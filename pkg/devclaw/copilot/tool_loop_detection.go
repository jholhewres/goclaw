// Package copilot – tool_loop_detection.go detects when the agent enters a
// tool call loop (repeating the same call with no progress) and triggers
// circuit breakers to prevent infinite loops.
//
// Four detectors:
//   - Generic repeat: same tool+args hash repeated N times
//   - Ping-pong: alternating between two tool calls
//   - Known no-progress poll: tools that poll external state without progress
//   - Global circuit breaker: total no-progress calls across all patterns
package copilot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// ToolLoopConfig configures tool loop detection thresholds.
type ToolLoopConfig struct {
	// Enabled turns loop detection on (default: true).
	Enabled bool `yaml:"enabled"`

	// HistorySize is how many recent tool calls to track (default: 30).
	HistorySize int `yaml:"history_size"`

	// WarningThreshold triggers a warning injected into the conversation (default: 8).
	WarningThreshold int `yaml:"warning_threshold"`

	// CriticalThreshold triggers a strong nudge to stop (default: 15).
	CriticalThreshold int `yaml:"critical_threshold"`

	// CircuitBreakerThreshold force-stops the agent run (default: 25).
	CircuitBreakerThreshold int `yaml:"circuit_breaker_threshold"`

	// GlobalCircuitBreaker is the max total no-progress calls before hard stop (default: 30).
	GlobalCircuitBreaker int `yaml:"global_circuit_breaker"`

	// ProgressDetection enables content-based progress analysis (default: true).
	ProgressDetection bool `yaml:"progress_detection"`
}

// DefaultToolLoopConfig returns sensible defaults.
func DefaultToolLoopConfig() ToolLoopConfig {
	return ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
		GlobalCircuitBreaker:    30,
		ProgressDetection:     true,
	}
}

// LoopSeverity represents the level of loop detection.
type LoopSeverity int

const (
	LoopNone     LoopSeverity = iota
	LoopWarning               // Agent should be nudged
	LoopCritical              // Agent should be strongly nudged
	LoopBreaker               // Agent run should be terminated
)

// LoopDetectionResult is the outcome of a loop check.
type LoopDetectionResult struct {
	Severity LoopSeverity
	Message  string // Injected into the conversation as a system hint
	Streak   int    // Number of consecutive repeats detected
	Pattern  string // "repeat", "ping-pong", "known_poll", "global_breaker", or ""
}

// toolCallEntry records a single tool call in the history ring buffer.
type toolCallEntry struct {
	hash           string
	name           string
	progress       bool   // whether this call made progress (output changed from previous)
	errorMsg       string // last error message for this call (for strategy detection)
	hasProgress    bool   // detected progress indicator in output
	exitCode       int    // for command-based tools
	outputHash     string // hash of output for comparison
}

// knownNoProgressTools are tools that frequently poll external state without
// making progress. These get hard-blocked earlier than generic repeats.
var knownNoProgressTools = map[string]map[string]bool{
	"process":   {"poll": true, "log": true},
	"cron_list": {"": true},
}

// destructiveTools are tools that can cause data loss or irreversible changes.
// These get additional batch detection to prevent accidental mass operations.
// Note: Dispatcher tools (team_agent, team_manage, team_task) are NOT included
// because they have multiple actions, not all destructive. The DestructiveTracker
// handles those with action-specific checking.
var destructiveTools = map[string]bool{
	"cron_remove":     true,
	"vault_delete":    true,
	"sessions_delete": true,
}

// ToolLoopDetector tracks tool call history and detects loops.
type ToolLoopDetector struct {
	config          ToolLoopConfig
	history         []toolCallEntry
	noProgressCount int // total calls without progress across all tools
	lastOutputHash  string
	lastErrorMsg    string         // last error message seen
	sameErrorCount  int            // consecutive calls with same error
	warningBucket   map[string]int // tool → warning count (coalesce repeated warnings)
	logger          *slog.Logger

	// Destructive batch tracking
	destructiveStreak     int    // consecutive destructive tool calls
	lastDestructiveTool   string // last destructive tool called
	destructiveBatchCount int    // total destructive calls in session
}

// NewToolLoopDetector creates a new detector with the given config.
func NewToolLoopDetector(cfg ToolLoopConfig, logger *slog.Logger) *ToolLoopDetector {
	if cfg.HistorySize <= 0 {
		cfg.HistorySize = 30
	}
	if cfg.WarningThreshold <= 0 {
		cfg.WarningThreshold = 8
	}
	if cfg.CriticalThreshold <= 0 {
		cfg.CriticalThreshold = 15
	}
	if cfg.CircuitBreakerThreshold <= 0 {
		cfg.CircuitBreakerThreshold = 25
	}
	if cfg.GlobalCircuitBreaker <= 0 {
		cfg.GlobalCircuitBreaker = 30
	}
	// Ensure thresholds are ordered.
	if cfg.CriticalThreshold <= cfg.WarningThreshold {
		cfg.CriticalThreshold = cfg.WarningThreshold + 1
	}
	if cfg.CircuitBreakerThreshold <= cfg.CriticalThreshold {
		cfg.CircuitBreakerThreshold = cfg.CriticalThreshold + 1
	}

	return &ToolLoopDetector{
		config:        cfg,
		history:       make([]toolCallEntry, 0, cfg.HistorySize),
		warningBucket: make(map[string]int),
		logger:        logger,
	}
}

// RecordToolOutcome records the result of a tool call for progress tracking.
// Call this after tool execution with the output to determine if the agent
// is making progress. An empty or identical output signals no progress.
func (d *ToolLoopDetector) RecordToolOutcome(output string) {
	h := hashOutput(output)
	outputChanged := h != d.lastOutputHash && output != ""
	hasProgressIndicators := detectProgressIndicators(output)

	// Progress is determined by: output changed OR contains success indicators
	madeProgress := outputChanged || hasProgressIndicators

	if !madeProgress {
		d.noProgressCount++
	} else {
		d.noProgressCount = 0
	}

	// Track repeated errors (same error = stuck strategy).
	errorMsg := extractErrorMessage(output)
	if errorMsg != "" {
		if errorMsg == d.lastErrorMsg {
			d.sameErrorCount++
		} else {
			d.sameErrorCount = 1 // First occurrence of this error
			d.lastErrorMsg = errorMsg
		}
	}
	// Don't reset counter on success - we want to catch alternating error/success patterns too

	// Update the last history entry with all progress indicators.
	if len(d.history) > 0 {
		d.history[len(d.history)-1].progress = madeProgress
		d.history[len(d.history)-1].errorMsg = errorMsg
		d.history[len(d.history)-1].hasProgress = hasProgressIndicators
		d.history[len(d.history)-1].outputHash = h
	}

	d.lastOutputHash = h
}

// extractErrorMessage extracts a normalized error message from tool output.
// Returns empty string if no error detected.
func extractErrorMessage(output string) string {
	lower := strings.ToLower(output)

	// Common error patterns
	patterns := []string{
		"error:", "failed", "cannot", "not found", "404", "500",
		"exception:", "panic:", "fatal:", "errno", "enoent",
	}

	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			// Extract first 100 chars after the error marker for comparison
			idx := strings.Index(lower, pattern)
			if idx >= 0 {
				end := idx + 100
				if end > len(output) {
					end = len(output)
				}
				return strings.TrimSpace(output[idx:end])
			}
		}
	}

	return ""
}

// detectProgressIndicators analyzes output for signs of actual progress.
// Returns true if output contains indicators of successful work being done.
func detectProgressIndicators(output string) bool {
	if output == "" {
		return false
	}

	lower := strings.ToLower(output)

	// Success patterns that indicate actual progress
	successPatterns := []string{
		"created", "written", "saved", "deleted", "moved",
		"success", "completed", "installed", "updated",
		"modified", "changed", "added", "removed",
		"ok", "done", "finished", "committed",
		"executed", "ran", "started", "deployed",
	}

	for _, pattern := range successPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// RecordAndCheck records a tool call and checks for loops.
// Returns a result indicating the severity (if any).
func (d *ToolLoopDetector) RecordAndCheck(toolName string, args map[string]any) LoopDetectionResult {
	if !d.config.Enabled {
		return LoopDetectionResult{Severity: LoopNone}
	}

	hash := hashToolCall(toolName, args)
	entry := toolCallEntry{hash: hash, name: toolName}

	// Append to history (ring buffer).
	d.history = append(d.history, entry)
	if len(d.history) > d.config.HistorySize {
		d.history = d.history[len(d.history)-d.config.HistorySize:]
	}

	// 1. Global circuit breaker: total no-progress calls.
	if d.noProgressCount >= d.config.GlobalCircuitBreaker {
		d.logger.Error("global circuit breaker triggered",
			"tool", toolName, "no_progress_count", d.noProgressCount)
		return LoopDetectionResult{
			Severity: LoopBreaker,
			Message: fmt.Sprintf(
				"GLOBAL CIRCUIT BREAKER: %d consecutive tool calls with no progress. "+
					"This run is being terminated. Try a completely different approach.",
				d.noProgressCount),
			Streak:  d.noProgressCount,
			Pattern: "global_breaker",
		}
	}

	// 1b. Strategy loop detection: same error repeated = stuck strategy.
	// This catches cases where the agent tries different tools but gets the same error,
	// indicating a fundamental misunderstanding of the problem.
	if d.sameErrorCount >= 5 {
		d.logger.Warn("strategy loop detected (same error repeated)",
			"tool", toolName, "same_error_count", d.sameErrorCount, "error", d.lastErrorMsg)
		return LoopDetectionResult{
			Severity: LoopCritical,
			Message: fmt.Sprintf(
				"STRATEGY LOOP DETECTED: You've gotten the same error %d times with different approaches. "+
					"This means your understanding of the problem is incorrect. STOP and investigate: "+
					"read documentation, check architecture, verify assumptions. Error: %s",
				d.sameErrorCount, truncateStr(d.lastErrorMsg, 100)),
			Streak:  d.sameErrorCount,
			Pattern: "strategy_loop",
		}
	}

	// 2. Known no-progress poll detection (hard-block earlier).
	if d.isKnownNoProgressCall(toolName, args) {
		pollStreak := d.getRepeatStreak(hash)
		if pollStreak >= 5 {
			d.logger.Warn("known no-progress poll loop detected",
				"tool", toolName, "streak", pollStreak)
			return LoopDetectionResult{
				Severity: LoopCritical,
				Message: fmt.Sprintf(
					"BLOCKED: '%s' has been called %d times polling the same resource with no change. "+
						"Stop polling and try a different approach (e.g. re-read with smaller chunks, check a different resource).",
					toolName, pollStreak),
				Streak:  pollStreak,
				Pattern: "known_poll",
			}
		}
	}

	// 2b. Destructive batch detection: BLOCK on consecutive destructive calls.
	// Destructive operations (vault_delete, cron_remove, sessions_delete) should
	// be limited to prevent accidental mass deletions. We use LoopBreaker (hard stop)
	// because LoopCritical (warning) was insufficient - agents ignored it and continued.
	if destructiveTools[toolName] {
		d.destructiveBatchCount++
		if toolName == d.lastDestructiveTool {
			d.destructiveStreak++
			if d.destructiveStreak >= 3 {
				d.logger.Error("destructive batch blocked - too many consecutive deletions",
					"tool", toolName, "streak", d.destructiveStreak, "total", d.destructiveBatchCount)
				return LoopDetectionResult{
					Severity: LoopBreaker,
					Message: fmt.Sprintf(
						"DESTRUCTIVE BATCH BLOCKED: You have called '%s' %d times consecutively. "+
							"This run is being terminated to prevent accidental mass deletion. "+
							"If you really need to delete more items, please start a new conversation and proceed carefully. "+
							"Total destructive calls this session: %d",
						toolName, d.destructiveStreak, d.destructiveBatchCount),
					Streak:  d.destructiveStreak,
					Pattern: "destructive_batch",
				}
			}
		} else {
			d.destructiveStreak = 1
			d.lastDestructiveTool = toolName
		}
	} else {
		// Non-destructive tool resets the consecutive count.
		d.destructiveStreak = 0
	}

	// 3. Check generic repeat and ping-pong patterns.
	repeatStreak := d.getRepeatStreak(hash)
	pingPongStreak := d.getPingPongStreak(hash)

	// Use the worst streak.
	streak := repeatStreak
	pattern := "repeat"
	if pingPongStreak > streak {
		streak = pingPongStreak
		pattern = "ping-pong"
	}

	if streak >= d.config.CircuitBreakerThreshold {
		d.logger.Error("tool loop circuit breaker triggered",
			"tool", toolName, "streak", streak, "pattern", pattern)
		return LoopDetectionResult{
			Severity: LoopBreaker,
			Message: fmt.Sprintf(
				"CIRCUIT BREAKER: You have called '%s' %d times with the same arguments and no progress. "+
					"This run is being terminated. The approach is not working — you need a fundamentally different strategy.",
				toolName, streak),
			Streak:  streak,
			Pattern: pattern,
		}
	}

	if streak >= d.config.CriticalThreshold {
		d.logger.Warn("tool loop critical threshold reached",
			"tool", toolName, "streak", streak, "pattern", pattern)
		return LoopDetectionResult{
			Severity: LoopCritical,
			Message: fmt.Sprintf(
				"CRITICAL: You have repeated '%s' %d times with no progress. STOP this approach immediately. "+
					"Explain to the user what you tried and ask for guidance. Do NOT call this tool again with the same arguments.",
				toolName, streak),
			Streak:  streak,
			Pattern: pattern,
		}
	}

	if streak >= d.config.WarningThreshold {
		// Coalesce repeated warnings into buckets to reduce spam.
		d.warningBucket[toolName]++
		warnCount := d.warningBucket[toolName]
		if warnCount == 1 || warnCount%3 == 0 {
			d.logger.Warn("tool loop warning threshold reached",
				"tool", toolName, "streak", streak, "pattern", pattern, "warn_count", warnCount)
			return LoopDetectionResult{
				Severity: LoopWarning,
				Message: fmt.Sprintf(
					"WARNING: You have called '%s' %d times with similar arguments. This may indicate a loop. "+
						"Consider a different approach or ask the user for help.",
					toolName, streak),
				Streak:  streak,
				Pattern: pattern,
			}
		}
	}

	return LoopDetectionResult{Severity: LoopNone}
}

// Reset clears the history (e.g. for a new run).
func (d *ToolLoopDetector) Reset() {
	d.history = d.history[:0]
	d.noProgressCount = 0
	d.lastOutputHash = ""
	d.lastErrorMsg = ""
	d.sameErrorCount = 0
	d.warningBucket = make(map[string]int)
}

// isKnownNoProgressCall checks if a tool call matches known poll patterns.
func (d *ToolLoopDetector) isKnownNoProgressCall(toolName string, args map[string]any) bool {
	actions, ok := knownNoProgressTools[toolName]
	if !ok {
		return false
	}
	// Check if the action argument matches a known poll action.
	if action, ok := args["action"].(string); ok {
		return actions[action]
	}
	return false
}

// getRepeatStreak counts consecutive identical tool calls from the end.
func (d *ToolLoopDetector) getRepeatStreak(currentHash string) int {
	streak := 0
	for i := len(d.history) - 1; i >= 0; i-- {
		if d.history[i].hash == currentHash {
			streak++
		} else {
			break
		}
	}
	return streak
}

// getPingPongStreak detects alternating A-B-A-B patterns.
func (d *ToolLoopDetector) getPingPongStreak(currentHash string) int {
	if len(d.history) < 3 {
		return 0
	}

	// The current call is already appended, so check the pattern from the end.
	// Pattern: ...A, B, A, B, A (current = A)
	otherHash := ""
	streak := 1

	for i := len(d.history) - 2; i >= 0; i-- {
		h := d.history[i].hash
		if streak == 1 {
			// First step back: should be different from current.
			if h == currentHash {
				return 0
			}
			otherHash = h
			streak++
		} else if streak%2 == 0 {
			// Even positions: should match current.
			if h != currentHash {
				break
			}
			streak++
		} else {
			// Odd positions: should match other.
			if h != otherHash {
				break
			}
			streak++
		}
	}

	// Ping-pong streak is the number of pairs (each pair = 2 calls).
	return streak / 2
}

// hashToolCall creates a stable hash of tool name + args for comparison.
func hashToolCall(name string, args map[string]any) string {
	// Normalize: sort keys, marshal to JSON.
	data, err := json.Marshal(args)
	if err != nil {
		data = []byte(fmt.Sprintf("%v", args))
	}

	// For bash commands, also normalize whitespace.
	key := name + ":" + string(data)
	if name == "bash" || name == "ssh" {
		key = strings.Join(strings.Fields(key), " ")
	}

	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:8])
}

// hashOutput creates a hash of tool output for progress tracking.
func hashOutput(output string) string {
	if output == "" {
		return ""
	}
	h := sha256.Sum256([]byte(output))
	return fmt.Sprintf("%x", h[:8])
}
