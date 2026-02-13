// Package copilot – heartbeat.go implements a periodic heartbeat that checks
// for pending scheduled jobs, reads HEARTBEAT.md for custom checklists,
// and triggers proactive agent turns that can send messages to channels.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

// HeartbeatConfig configures the heartbeat system.
type HeartbeatConfig struct {
	// Enabled turns the heartbeat on/off.
	Enabled bool `yaml:"enabled"`

	// Interval is the time between heartbeat ticks.
	// Default: 30 minutes.
	Interval time.Duration `yaml:"interval"`

	// ActiveStart is the earliest hour the heartbeat runs (e.g., 9 for 9 AM).
	ActiveStart int `yaml:"active_start"`

	// ActiveEnd is the latest hour the heartbeat runs (e.g., 22 for 10 PM).
	ActiveEnd int `yaml:"active_end"`

	// Channel is the default channel to send proactive messages to.
	Channel string `yaml:"channel"`

	// ChatID is the default chat to send proactive messages to.
	ChatID string `yaml:"chat_id"`

	// WorkspaceDir is the workspace directory where HEARTBEAT.md is located.
	WorkspaceDir string `yaml:"workspace_dir"`
}

// DefaultHeartbeatConfig returns sensible defaults for the heartbeat.
func DefaultHeartbeatConfig() HeartbeatConfig {
	return HeartbeatConfig{
		Enabled:      false,
		Interval:     30 * time.Minute,
		ActiveStart:  9,
		ActiveEnd:    22,
		WorkspaceDir: ".",
	}
}

// Heartbeat runs periodic checks and proactive behavior.
type Heartbeat struct {
	config    HeartbeatConfig
	assistant *Assistant
	logger    *slog.Logger
	cancel    context.CancelFunc
}

// NewHeartbeat creates a new heartbeat instance.
func NewHeartbeat(cfg HeartbeatConfig, assistant *Assistant, logger *slog.Logger) *Heartbeat {
	return &Heartbeat{
		config:    cfg,
		assistant: assistant,
		logger:    logger.With("component", "heartbeat"),
	}
}

// Start begins the heartbeat loop in a background goroutine.
func (h *Heartbeat) Start(ctx context.Context) {
	if !h.config.Enabled {
		h.logger.Info("heartbeat disabled")
		return
	}

	hbCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	interval := h.config.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	h.logger.Info("heartbeat started",
		"interval", interval.String(),
		"active_hours", fmt.Sprintf("%02d:00-%02d:00", h.config.ActiveStart, h.config.ActiveEnd),
		"channel", h.config.Channel,
	)

	go h.loop(hbCtx, interval)
}

// Stop shuts down the heartbeat.
func (h *Heartbeat) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// loop is the main heartbeat goroutine.
func (h *Heartbeat) loop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.tick(ctx)
		case <-ctx.Done():
			h.logger.Info("heartbeat stopped")
			return
		}
	}
}

// tick performs a single heartbeat check.
func (h *Heartbeat) tick(ctx context.Context) {
	now := time.Now()
	hour := now.Hour()

	// Check if we're in active hours.
	if hour < h.config.ActiveStart || hour >= h.config.ActiveEnd {
		h.logger.Debug("heartbeat: outside active hours, skipping")
		return
	}

	h.logger.Debug("heartbeat tick", "time", now.Format("15:04"))

	// Build the heartbeat prompt.
	prompt := h.buildHeartbeatPrompt(now)

	// Run an agent turn with the heartbeat prompt.
	session := h.assistant.sessionStore.GetOrCreate("heartbeat", "main")
	systemPrompt := h.assistant.promptComposer.Compose(session, prompt)

	agent := NewAgentRun(h.assistant.llmClient, h.assistant.toolExecutor, h.logger)

	turnCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	response, err := agent.Run(turnCtx, systemPrompt, session.RecentHistory(5), prompt)
	if err != nil {
		h.logger.Error("heartbeat agent turn failed", "error", err)
		return
	}

	// Save to session.
	session.AddMessage(prompt, response)

	// If the response is just HEARTBEAT_OK or empty, skip delivery.
	trimmed := strings.TrimSpace(response)
	if trimmed == "" || strings.EqualFold(trimmed, "HEARTBEAT_OK") || strings.EqualFold(trimmed, "NO_REPLY") {
		h.logger.Debug("heartbeat: nothing to deliver")
		return
	}

	// Deliver proactive message to configured channel.
	if h.config.Channel != "" && h.config.ChatID != "" {
		outMsg := &channels.OutgoingMessage{Content: response}
		if err := h.assistant.channelMgr.Send(ctx, h.config.Channel, h.config.ChatID, outMsg); err != nil {
			h.logger.Error("heartbeat: failed to deliver message", "error", err)
		} else {
			h.logger.Info("heartbeat: proactive message delivered",
				"channel", h.config.Channel,
				"response_len", len(response),
			)
		}
	}
}

// buildHeartbeatPrompt builds the prompt for a heartbeat turn.
// Reads HEARTBEAT.md if it exists, otherwise uses a default prompt.
func (h *Heartbeat) buildHeartbeatPrompt(now time.Time) string {
	// Try to read HEARTBEAT.md from workspace.
	heartbeatFile := filepath.Join(h.config.WorkspaceDir, "HEARTBEAT.md")
	content, err := os.ReadFile(heartbeatFile)
	if err == nil && len(content) > 0 {
		return fmt.Sprintf("[HEARTBEAT at %s]\n\n%s\n\nIf there is nothing to do, respond with HEARTBEAT_OK.",
			now.Format("2006-01-02 15:04"), strings.TrimSpace(string(content)))
	}

	// Default heartbeat prompt.
	return fmt.Sprintf(`[HEARTBEAT at %s]

Check if there are any pending reminders, scheduled tasks, or proactive actions to take.
Review recent memory for anything time-sensitive.

If there is nothing to do, respond with HEARTBEAT_OK.
If there is something to communicate to the user, write a concise message.`, now.Format("2006-01-02 15:04"))
}
