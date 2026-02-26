// Package whatsapp – health.go implements proactive health monitoring for
// the WhatsApp connection to detect and recover from silent disconnects.
package whatsapp

import (
	"context"
	"time"
)

// HealthMonitorConfig configures proactive connection health monitoring.
type HealthMonitorConfig struct {
	// Enabled turns on proactive health monitoring.
	Enabled bool `yaml:"enabled"`

	// CheckInterval is how often to perform health checks.
	// Default: 30s
	CheckInterval time.Duration `yaml:"check_interval"`

	// MaxSilentDuration is the maximum time without any activity before
	// a health check is considered failed.
	// Default: 5m
	MaxSilentDuration time.Duration `yaml:"max_silent_duration"`

	// ForceReconnectAfter is the maximum silent duration before forcing
	// a preventive reconnection, regardless of client.IsConnected() result.
	// This handles "half-open" TCP connections where the socket appears
	// connected but is actually dead.
	// Default: 30m (0 = disabled)
	ForceReconnectAfter time.Duration `yaml:"force_reconnect_after"`
}

// DefaultHealthMonitorConfig returns sensible defaults.
func DefaultHealthMonitorConfig() HealthMonitorConfig {
	return HealthMonitorConfig{
		Enabled:             true,
		CheckInterval:       30 * time.Second,
		MaxSilentDuration:   5 * time.Minute,
		ForceReconnectAfter: 15 * time.Minute,
	}
}

// StartHealthMonitor starts the proactive health monitoring goroutine.
// It runs until the context is cancelled.
func (w *WhatsApp) StartHealthMonitor(ctx context.Context, cfg HealthMonitorConfig) {
	if !cfg.Enabled {
		return
	}

	// Apply defaults.
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.MaxSilentDuration <= 0 {
		cfg.MaxSilentDuration = 5 * time.Minute
	}
	// ForceReconnectAfter default is 30m (set in DefaultHealthMonitorConfig).
	// No need to validate - 0 means disabled.

	go func() {
		ticker := time.NewTicker(cfg.CheckInterval)
		defer ticker.Stop()

		w.logger.Info("whatsapp health monitor started",
			"check_interval", cfg.CheckInterval,
			"max_silent", cfg.MaxSilentDuration,
			"force_reconnect_after", cfg.ForceReconnectAfter)

		for {
			select {
			case <-ctx.Done():
				w.logger.Info("whatsapp health monitor stopped")
				return
			case <-ticker.C:
				w.performHealthCheck(cfg)
			}
		}
	}()

	w.StartPinger(ctx)
}

// StartPinger sends periodic presence updates to keep connection alive.
func (w *WhatsApp) StartPinger(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()

		w.logger.Info("whatsapp pinger started", "interval", "2m")

		for {
			select {
			case <-ctx.Done():
				w.logger.Info("whatsapp pinger stopped")
				return
			case <-ticker.C:
				if w.getState() == StateConnected {
					// Send presence to keep connection alive
					if err := w.SendPresence(ctx, true); err != nil {
						w.logger.Warn("pinger: failed to send presence", "error", err)
					} else {
						w.logger.Info("pinger: sent presence update")
						// Update last activity time
						w.UpdateLastMsgTime()
					}
				}
			}
		}
	}()
}

// performHealthCheck checks connection health and takes action if needed.
func (w *WhatsApp) performHealthCheck(cfg HealthMonitorConfig) {
	state := w.getState()

	// Skip if not connected.
	if state != StateConnected {
		return
	}

	// Check if we've been silent too long.
	lastMsg := w.getLastMsgTime()
	silentDuration := time.Since(lastMsg)

	if silentDuration > cfg.MaxSilentDuration {
		w.logger.Warn("whatsapp: connection silent for too long",
			"silent_duration", silentDuration,
			"max_silent", cfg.MaxSilentDuration)

		// Check if client is actually responsive.
		if w.client != nil {
			// Try to get connection state from client.
			// If IsConnected returns false but we think we're connected, there's a mismatch.
			if !w.client.IsConnected() {
				w.logger.Error("whatsapp: client reports disconnected but state is connected",
					"state", state)

				// Trigger reconnection.
				w.setState(StateReconnecting)
				w.connected.Store(false)
				go w.attemptReconnect()
				return
			}
		}

		// Check if we should force a preventive reconnection.
		// This handles "half-open" TCP connections where the socket appears
		// connected but is actually dead.
		if cfg.ForceReconnectAfter > 0 && silentDuration > cfg.ForceReconnectAfter {
			w.logger.Warn("whatsapp: forcing preventive reconnection due to excessive silence",
				"silent_duration", silentDuration,
				"force_reconnect_after", cfg.ForceReconnectAfter)

			w.setState(StateReconnecting)
			w.connected.Store(false)
			go w.attemptReconnect()
			return
		}

		// Note: whatsmeow handles its own keepalive pings internally.
		// If we're silent but client reports connected, the connection is likely fine.
		// We just log a warning for visibility.
		w.logger.Debug("whatsapp: silent connection, client still reports connected")
	}
}

// getLastMsgTime returns the time of the last message or activity.
func (w *WhatsApp) getLastMsgTime() time.Time {
	if v := w.lastMsg.Load(); v != nil {
		return v.(time.Time)
	}
	return time.Time{}
}

// UpdateLastMsgTime updates the last activity timestamp.
// This should be called whenever we receive a message or other activity.
func (w *WhatsApp) UpdateLastMsgTime() {
	w.lastMsg.Store(time.Now())
}
