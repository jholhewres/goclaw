package copilot

import (
	"log/slog"
	"testing"
)

func newTestDetector(cfg ToolLoopConfig) *ToolLoopDetector {
	return NewToolLoopDetector(cfg, slog.Default())
}

func TestToolLoopDetector_NoLoopBeforeThreshold(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        5,
		CriticalThreshold:       10,
		CircuitBreakerThreshold: 15,
	})

	args := map[string]any{"command": "ls -la"}

	for i := 0; i < 4; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("expected LoopNone at iteration %d, got %d", i, r.Severity)
		}
	}
}

func TestToolLoopDetector_Warning(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "cat file.txt"}

	for i := 0; i < 2; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopWarning {
		t.Errorf("expected LoopWarning, got %d", r.Severity)
	}
	if r.Pattern != "repeat" {
		t.Errorf("expected pattern 'repeat', got %q", r.Pattern)
	}
	if r.Streak != 3 {
		t.Errorf("expected streak 3, got %d", r.Streak)
	}
}

func TestToolLoopDetector_Critical(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "curl http://example.com"}

	for i := 0; i < 5; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopCritical {
		t.Errorf("expected LoopCritical, got %d", r.Severity)
	}
}

func TestToolLoopDetector_CircuitBreaker(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "cat file.txt"}

	for i := 0; i < 9; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopBreaker {
		t.Errorf("expected LoopBreaker, got %d", r.Severity)
	}
	if r.Streak != 10 {
		t.Errorf("expected streak 10, got %d", r.Streak)
	}
}

func TestToolLoopDetector_DifferentArgsNoLoop(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	for i := 0; i < 20; i++ {
		args := map[string]any{"command": "echo " + string(rune('a'+i))}
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("expected LoopNone at iteration %d with unique args, got %d", i, r.Severity)
		}
	}
}

func TestToolLoopDetector_PingPong(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	argsA := map[string]any{"command": "cat a.txt"}
	argsB := map[string]any{"command": "cat b.txt"}

	// Build A-B-A-B-A-B pattern (6 calls = 3 pairs).
	var lastResult LoopDetectionResult
	for i := 0; i < 6; i++ {
		if i%2 == 0 {
			lastResult = d.RecordAndCheck("bash", argsA)
		} else {
			lastResult = d.RecordAndCheck("bash", argsB)
		}
	}

	if lastResult.Severity < LoopWarning {
		t.Errorf("expected at least LoopWarning after ping-pong pattern, got %d (streak=%d, pattern=%s)",
			lastResult.Severity, lastResult.Streak, lastResult.Pattern)
	}
}

func TestToolLoopDetector_Reset(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "ls"}

	for i := 0; i < 2; i++ {
		d.RecordAndCheck("bash", args)
	}

	d.Reset()

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopNone {
		t.Errorf("after Reset, expected LoopNone, got %d (streak=%d)", r.Severity, r.Streak)
	}
}

func TestToolLoopDetector_Disabled(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 false,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "ls"}

	for i := 0; i < 20; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("disabled detector should always return LoopNone, got %d", r.Severity)
		}
	}
}

func TestToolLoopDetector_HistoryRingBuffer(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             5,
		WarningThreshold:        4,
		CriticalThreshold:       8,
		CircuitBreakerThreshold: 12,
	})

	// Fill with 5 unique calls.
	for i := 0; i < 5; i++ {
		d.RecordAndCheck("bash", map[string]any{"i": i})
	}

	// Now repeat 3 times — history only holds 5, so streak can't hit 4.
	args := map[string]any{"command": "repeat"}
	for i := 0; i < 3; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("streak within history window should not trigger warning, got %d", r.Severity)
		}
	}
}

func TestToolLoopConfig_DefaultValues(t *testing.T) {
	t.Parallel()
	cfg := DefaultToolLoopConfig()

	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.HistorySize != 30 {
		t.Errorf("expected HistorySize 30, got %d", cfg.HistorySize)
	}
	if cfg.WarningThreshold != 8 {
		t.Errorf("expected WarningThreshold 8, got %d", cfg.WarningThreshold)
	}
	if cfg.CriticalThreshold != 15 {
		t.Errorf("expected CriticalThreshold 15, got %d", cfg.CriticalThreshold)
	}
	if cfg.CircuitBreakerThreshold != 25 {
		t.Errorf("expected CircuitBreakerThreshold 25, got %d", cfg.CircuitBreakerThreshold)
	}
}

func TestNewToolLoopDetector_NormalizesThresholds(t *testing.T) {
	t.Parallel()

	// Inverted thresholds should be corrected.
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        10,
		CriticalThreshold:       5,  // Less than warning.
		CircuitBreakerThreshold: 3,  // Less than critical.
	})

	if d.config.CriticalThreshold <= d.config.WarningThreshold {
		t.Errorf("CriticalThreshold (%d) should be > WarningThreshold (%d)",
			d.config.CriticalThreshold, d.config.WarningThreshold)
	}
	if d.config.CircuitBreakerThreshold <= d.config.CriticalThreshold {
		t.Errorf("CircuitBreakerThreshold (%d) should be > CriticalThreshold (%d)",
			d.config.CircuitBreakerThreshold, d.config.CriticalThreshold)
	}
}

func TestHashToolCall_Deterministic(t *testing.T) {
	t.Parallel()

	args := map[string]any{"command": "echo hello", "timeout": 30}
	h1 := hashToolCall("bash", args)
	h2 := hashToolCall("bash", args)

	if h1 != h2 {
		t.Errorf("hash should be deterministic: %q != %q", h1, h2)
	}

	h3 := hashToolCall("ssh", args)
	if h1 == h3 {
		t.Error("different tool names should produce different hashes")
	}
}

func TestHashToolCall_DifferentArgs(t *testing.T) {
	t.Parallel()

	h1 := hashToolCall("bash", map[string]any{"command": "ls"})
	h2 := hashToolCall("bash", map[string]any{"command": "pwd"})

	if h1 == h2 {
		t.Error("different args should produce different hashes")
	}
}

func TestToolLoopDetector_StrategyLoop(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
	})

	// Simulate agent trying different tools but getting the same error.
	// This indicates a stuck strategy (wrong understanding of the problem).
	sameError := "Error: Cannot GET /api/route - 404 Not Found"

	// Try bash (attempt 1)
	d.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/route"})
	d.RecordToolOutcome(sameError)

	// Try read_file (attempt 2)
	d.RecordAndCheck("read_file", map[string]any{"path": "dist/routes.js"})
	d.RecordToolOutcome(sameError)

	// Try bash again - rebuild (attempt 3)
	d.RecordAndCheck("bash", map[string]any{"command": "npm run build"})
	d.RecordToolOutcome(sameError)

	// Try bash - restart (attempt 4)
	d.RecordAndCheck("bash", map[string]any{"command": "pm2 restart app"})
	d.RecordToolOutcome(sameError)

	// Try bash - test again (attempt 5)
	d.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/route"})
	d.RecordToolOutcome(sameError)

	// Now sameErrorCount should be 5, next RecordAndCheck should trigger
	r := d.RecordAndCheck("bash", map[string]any{"command": "curl -v http://localhost:3000/api/route"})

	if r.Severity != LoopCritical {
		t.Errorf("expected LoopCritical for strategy loop, got %d (sameErrorCount=%d)", r.Severity, d.sameErrorCount)
	}
	if r.Pattern != "strategy_loop" {
		t.Errorf("expected pattern 'strategy_loop', got %q", r.Pattern)
	}
}

func TestExtractErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   bool // true if error should be detected
	}{
		{
			name:   "404 error",
			output: "HTTP/1.1 404 Not Found\nContent-Type: text/html",
			want:   true,
		},
		{
			name:   "error prefix",
			output: "Error: ENOENT: no such file or directory",
			want:   true,
		},
		{
			name:   "failed prefix",
			output: "Failed to connect to localhost:3000",
			want:   true,
		},
		{
			name:   "success output",
			output: "Build completed successfully\nOutput: dist/",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorMessage(tt.output)
			if tt.want && got == "" {
				t.Errorf("expected error to be detected in %q", tt.output)
			}
			if !tt.want && got != "" {
				t.Errorf("expected no error, but got %q from %q", got, tt.output)
			}
		})
	}
}

func TestToolLoopDetector_DestructiveBatch(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
	})

	args := map[string]any{"key": "test_key_"}

	// First two calls should not trigger
	r1 := d.RecordAndCheck("vault_delete", args)
	if r1.Severity != LoopNone {
		t.Errorf("first vault_delete should return LoopNone, got %d", r1.Severity)
	}

	r2 := d.RecordAndCheck("vault_delete", args)
	if r2.Severity != LoopNone {
		t.Errorf("second vault_delete should return LoopNone, got %d", r2.Severity)
	}

	// Third consecutive call to same destructive tool should trigger LoopBreaker
	r3 := d.RecordAndCheck("vault_delete", args)
	if r3.Severity != LoopBreaker {
		t.Errorf("third consecutive vault_delete should return LoopBreaker, got %d (pattern=%s)",
			r3.Severity, r3.Pattern)
	}
	if r3.Pattern != "destructive_batch" {
		t.Errorf("expected pattern 'destructive_batch', got %q", r3.Pattern)
	}
	if r3.Streak != 3 {
		t.Errorf("expected streak 3, got %d", r3.Streak)
	}
}

func TestToolLoopDetector_DestructiveBatch_ResetsOnOtherTool(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
	})

	args := map[string]any{"key": "test_key_"}

	// Two destructive calls
	d.RecordAndCheck("vault_delete", args)
	d.RecordAndCheck("vault_delete", args)

	// Non-destructive tool should reset the streak
	d.RecordAndCheck("bash", map[string]any{"command": "ls"})

	// Now destructive calls should start fresh
	r := d.RecordAndCheck("vault_delete", args)
	if r.Severity != LoopNone {
		t.Errorf("vault_delete after non-destructive should return LoopNone, got %d", r.Severity)
	}
}

func TestToolLoopDetector_DestructiveBatch_DifferentToolsReset(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
	})

	// Two vault_delete calls
	d.RecordAndCheck("vault_delete", map[string]any{"key": "a"})
	d.RecordAndCheck("vault_delete", map[string]any{"key": "b"})

	// Different destructive tool resets streak
	r := d.RecordAndCheck("cron_remove", map[string]any{"id": "1"})
	if r.Severity != LoopNone {
		t.Errorf("different destructive tool should reset streak, got %d", r.Severity)
	}
}
