// Package copilot – destructive_tracker.go implements rate limiting and batch
// detection for destructive tools to prevent accidental mass deletions.
package copilot

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// DestructiveToolsConfig configures protection for destructive operations.
type DestructiveToolsConfig struct {
	// Enabled turns on destructive tool protection (default: true).
	Enabled bool `yaml:"enabled"`

	// Tools lists tool names that are considered destructive.
	// Default: ["cron_remove", "vault_delete", "sessions_delete"]
	Tools []string `yaml:"tools"`

	// RateLimitPerMinute is the maximum number of calls per tool per minute.
	// Default: 3
	RateLimitPerMinute int `yaml:"rate_limit_per_minute"`

	// BatchThreshold is the number of consecutive calls that trigger a warning.
	// Default: 3
	BatchThreshold int `yaml:"batch_threshold"`

	// RequireInteractiveConfirmation forces the agent to ask user before
	// executing destructive operations, rather than using a confirm parameter.
	// Default: false (uses confirm parameter)
	RequireInteractiveConfirmation bool `yaml:"require_interactive_confirmation"`

	// CooldownSeconds is the minimum time between destructive operations.
	// Default: 5
	CooldownSeconds int `yaml:"cooldown_seconds"`
}

// DefaultDestructiveToolsConfig returns safe defaults.
func DefaultDestructiveToolsConfig() DestructiveToolsConfig {
	return DestructiveToolsConfig{
		Enabled:                        true,
		Tools:                          []string{"cron_remove", "vault_delete", "sessions_delete"},
		RateLimitPerMinute:             3,
		BatchThreshold:                 3,
		RequireInteractiveConfirmation: false,
		CooldownSeconds:                5,
	}
}

// DestructiveCheckResult holds the result of a destructive tool check.
type DestructiveCheckResult struct {
	Allowed           bool
	Reason            string
	RequiresUserInput bool
	BatchWarning      string
	CooldownRemaining time.Duration
}

// DestructiveTracker tracks and limits destructive tool calls.
type DestructiveTracker struct {
	config    DestructiveToolsConfig
	logger    *slog.Logger
	callTimes map[string][]time.Time // tool -> timestamps of recent calls

	// Consecutive call tracking (reset when a different tool is called)
	lastDestructiveTool string
	consecutiveCount    int

	// Cooldown tracking
	lastDestructiveTime time.Time

	mu sync.Mutex
}

// NewDestructiveTracker creates a new tracker with the given config.
func NewDestructiveTracker(cfg DestructiveToolsConfig, logger *slog.Logger) *DestructiveTracker {
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults.
	if cfg.RateLimitPerMinute <= 0 {
		cfg.RateLimitPerMinute = 3
	}
	if cfg.BatchThreshold <= 0 {
		cfg.BatchThreshold = 3
	}
	if cfg.CooldownSeconds <= 0 {
		cfg.CooldownSeconds = 5
	}
	if len(cfg.Tools) == 0 {
		cfg.Tools = []string{"cron_remove", "vault_delete", "sessions_delete"}
	}

	return &DestructiveTracker{
		config:    cfg,
		logger:    logger.With("component", "destructive_tracker"),
		callTimes: make(map[string][]time.Time),
	}
}

// IsDestructive checks if a tool is in the destructive list.
func (d *DestructiveTracker) IsDestructive(toolName string) bool {
	for _, t := range d.config.Tools {
		if t == toolName {
			return true
		}
	}
	return false
}

// Check evaluates whether a destructive tool call should be allowed.
func (d *DestructiveTracker) Check(toolName string) DestructiveCheckResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.config.Enabled {
		return DestructiveCheckResult{Allowed: true}
	}

	now := time.Now()
	result := DestructiveCheckResult{Allowed: true}

	// 1. Check cooldown.
	if !d.lastDestructiveTime.IsZero() {
		cooldown := time.Duration(d.config.CooldownSeconds) * time.Second
		elapsed := now.Sub(d.lastDestructiveTime)
		if elapsed < cooldown {
			result.Allowed = false
			result.Reason = fmt.Sprintf("cooldown active: %v remaining", cooldown-elapsed)
			result.CooldownRemaining = cooldown - elapsed
			return result
		}
	}

	// 2. Check rate limit.
	recentCalls := d.getRecentCalls(toolName, now)
	if len(recentCalls) >= d.config.RateLimitPerMinute {
		result.Allowed = false
		result.Reason = fmt.Sprintf("rate limit exceeded: %d calls in the last minute (max: %d)",
			len(recentCalls), d.config.RateLimitPerMinute)
		return result
	}

	// 3. Check for batch pattern (consecutive calls to same destructive tool).
	if toolName == d.lastDestructiveTool {
		d.consecutiveCount++
		if d.consecutiveCount >= d.config.BatchThreshold {
			result.BatchWarning = fmt.Sprintf(
				"DESTRUCTIVE BATCH WARNING: You have called '%s' %d times consecutively. "+
					"Please confirm with the user before proceeding with more deletions.",
				toolName, d.consecutiveCount)

			if d.config.RequireInteractiveConfirmation {
				result.RequiresUserInput = true
			}
		}
	} else {
		d.lastDestructiveTool = toolName
		d.consecutiveCount = 1
	}

	return result
}

// RecordCall records a destructive tool call for rate limiting.
func (d *DestructiveTracker) RecordCall(toolName string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Add to call history.
	d.callTimes[toolName] = append(d.callTimes[toolName], now)

	// Update cooldown.
	d.lastDestructiveTime = now

	// Clean old entries (keep only last minute).
	d.cleanOldEntries(toolName, now)

	d.logger.Debug("destructive tool call recorded",
		"tool", toolName,
		"consecutive", d.consecutiveCount,
		"recent_calls", len(d.callTimes[toolName]))
}

// getRecentCalls returns calls within the last minute.
func (d *DestructiveTracker) getRecentCalls(toolName string, now time.Time) []time.Time {
	cutoff := now.Add(-time.Minute)
	var recent []time.Time
	for _, t := range d.callTimes[toolName] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	return recent
}

// cleanOldEntries removes calls older than a minute.
func (d *DestructiveTracker) cleanOldEntries(toolName string, now time.Time) {
	cutoff := now.Add(-time.Minute)
	var recent []time.Time
	for _, t := range d.callTimes[toolName] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	d.callTimes[toolName] = recent
}

// Reset resets the tracker state (useful for testing or manual override).
func (d *DestructiveTracker) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.callTimes = make(map[string][]time.Time)
	d.lastDestructiveTool = ""
	d.consecutiveCount = 0
	d.lastDestructiveTime = time.Time{}
}

// Stats returns current statistics for monitoring.
func (d *DestructiveTracker) Stats() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	stats := map[string]any{
		"enabled":               d.config.Enabled,
		"last_destructive_tool": d.lastDestructiveTool,
		"consecutive_count":     d.consecutiveCount,
		"tools":                 d.config.Tools,
	}

	// Add recent call counts per tool.
	recentCalls := make(map[string]int)
	for tool := range d.callTimes {
		recentCalls[tool] = len(d.getRecentCalls(tool, now))
	}
	stats["recent_calls"] = recentCalls

	if !d.lastDestructiveTime.IsZero() {
		cooldown := time.Duration(d.config.CooldownSeconds) * time.Second
		remaining := cooldown - now.Sub(d.lastDestructiveTime)
		if remaining > 0 {
			stats["cooldown_remaining"] = remaining.String()
		}
	}

	return stats
}
