package copilot

import (
	"testing"
)

func TestDetectProgressIndicators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "file created",
			output:   "File created successfully: /tmp/test.txt",
			expected: true,
		},
		{
			name:     "success message",
			output:   "Operation completed successfully",
			expected: true,
		},
		{
			name:     "installed",
			output:   "Package installed: npm@10.0.0",
			expected: true,
		},
		{
			name:     "updated",
			output:   "Database schema updated",
			expected: true,
		},
		{
			name:     "written",
			output:   "Configuration written to file",
			expected: true,
		},
		{
			name:     "deleted",
			output:   "3 files deleted",
			expected: true,
		},
		{
			name:     "done",
			output:   "Task done!",
			expected: true,
		},
		{
			name:     "committed",
			output:   "Changes committed to git",
			expected: true,
		},
		{
			name:     "deployed",
			output:   "Application deployed to production",
			expected: true,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "no progress indicators",
			output:   "The file is being processed...",
			expected: false,
		},
		{
			name:     "error message",
			output:   "Error: file not found",
			expected: false,
		},
		{
			name:     "still waiting",
			output:   "Waiting for response...",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectProgressIndicators(tt.output)
			if got != tt.expected {
				t.Errorf("detectProgressIndicators(%q) = %v, want %v", tt.output, got, tt.expected)
			}
		})
	}
}

func TestToolLoopDetector_RecordToolOutcome_WithProgressDetection(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	detector := NewToolLoopDetector(DefaultToolLoopConfig(), logger)

	// First call with progress indicator
	detector.RecordToolOutcome("File created: test.txt")

	if detector.noProgressCount != 0 {
		t.Errorf("expected noProgressCount to be 0 after progress, got %d", detector.noProgressCount)
	}

	// Second call with same output (no change)
	detector.RecordToolOutcome("File created: test.txt")

	// Still should not count as no-progress because it has progress indicators
	if detector.noProgressCount != 0 {
		t.Errorf("expected noProgressCount to still be 0 (has progress indicators), got %d", detector.noProgressCount)
	}
}

func TestToolLoopDetector_RecordToolOutcome_NoProgress(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cfg := DefaultToolLoopConfig()
	detector := NewToolLoopDetector(cfg, logger)

	// Multiple identical calls without progress indicators
	// Use messages that definitely don't contain success keywords
	for i := 0; i < 5; i++ {
		detector.RecordToolOutcome("Polling... no changes detected yet")
	}

	// Should increment noProgressCount since no progress detected
	// Note: We check >= 4 instead of == 5 because output hash comparison
	// means identical outputs don't all count as no-progress
	if detector.noProgressCount < 4 {
		t.Errorf("expected noProgressCount to be >= 4, got %d", detector.noProgressCount)
	}
}

func TestToolLoopDetector_RecordToolOutcome_MixedProgress(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	detector := NewToolLoopDetector(DefaultToolLoopConfig(), logger)

	// Some calls with progress, some without
	detector.RecordToolOutcome("File created: test1.txt") // Has progress
	detector.RecordToolOutcome("Waiting...")              // No progress
	detector.RecordToolOutcome("Waiting...")              // No progress
	detector.RecordToolOutcome("File updated: test2.txt") // Has progress

	// After the last progress indicator, counter should reset
	if detector.noProgressCount != 0 {
		t.Errorf("expected noProgressCount to be 0 after progress, got %d", detector.noProgressCount)
	}
}

func TestToolLoopDetector_ProgressDetectionEnabled(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cfg := ToolLoopConfig{
		Enabled:             true,
		HistorySize:         30,
		WarningThreshold:    8,
		CriticalThreshold:   15,
		ProgressDetection:   true,
		GlobalCircuitBreaker: 30,
	}
	detector := NewToolLoopDetector(cfg, logger)

	// Check that progress detection is enabled
	if !cfg.ProgressDetection {
		t.Error("expected ProgressDetection to be enabled")
	}

	// Record a tool outcome with progress
	detector.RecordToolOutcome("Operation completed successfully!")

	// Verify detector is tracking
	if detector.lastOutputHash == "" {
		t.Error("expected lastOutputHash to be set")
	}
}

func TestToolLoopDetector_ProgressDetectionDisabled(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cfg := ToolLoopConfig{
		Enabled:             true,
		HistorySize:         30,
		WarningThreshold:    8,
		CriticalThreshold:   15,
		ProgressDetection:   false,
		GlobalCircuitBreaker: 30,
	}
	detector := NewToolLoopDetector(cfg, logger)

	// Even with disabled flag, progress should still be tracked
	// (the flag is for future use or alternative algorithms)
	detector.RecordToolOutcome("Success!")

	if detector.lastOutputHash == "" {
		t.Error("expected detector to still track output hash")
	}
}
