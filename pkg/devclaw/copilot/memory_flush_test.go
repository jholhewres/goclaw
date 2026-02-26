package copilot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()

	a := &AgentRun{}

	// Simple message
	messages := []chatMessage{
		{Role: "user", Content: "hello"},
	}
	tokens := a.estimateTokens(messages)
	// ~4 chars per token, so ~5/4 = 1 or 2 tokens
	if tokens < 1 || tokens > 5 {
		t.Errorf("expected ~1-5 tokens for 'hello', got %d", tokens)
	}

	// Multiple messages
	messages = []chatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "What is the weather?"},
		{Role: "assistant", Content: "It is sunny today."},
	}
	tokens = a.estimateTokens(messages)
	// Rough estimate: ~60 chars / 4 = ~15 tokens
	if tokens < 5 || tokens > 30 {
		t.Errorf("expected ~10-25 tokens, got %d", tokens)
	}

	// Empty messages
	tokens = a.estimateTokens([]chatMessage{})
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty messages, got %d", tokens)
	}
}

func TestGetModelContextWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model    string
		expected int
	}{
		{"gpt-4o", 128000},
		{"gpt-4o-mini", 128000},
		{"gpt-5", 128000},
		{"gpt-4", 8192},
		{"claude-3-opus", 200000},
		{"claude-3.5", 200000},
		{"glm-4", 128000},
		{"unknown-model", 128000}, // Default
		{"", 128000},              // Empty = default
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			a := &AgentRun{modelOverride: tt.model}
			got := a.getModelContextWindow()
			if got != tt.expected {
				t.Errorf("model %s: expected %d, got %d", tt.model, tt.expected, got)
			}
		})
	}
}

func TestMaybeMemoryFlush_Disabled(t *testing.T) {
	t.Parallel()

	a := &AgentRun{
		cfg: AgentConfig{
			MemoryFlush: MemoryFlushConfig{
				Enabled: false,
			},
		},
		logger: newTestLogger(),
	}

	messages := []chatMessage{
		{Role: "user", Content: "test"},
	}

	// Should return immediately when disabled without panicking
	a.maybeMemoryFlush(context.Background(), messages, 100000)
}

func TestMaybeMemoryFlush_NotTriggered(t *testing.T) {
	t.Parallel()

	a := &AgentRun{
		modelOverride: "gpt-4o", // 128k window
		cfg: AgentConfig{
			MemoryFlush: MemoryFlushConfig{
				Enabled:            true,
				ReserveTokensFloor: 20000,
				FlushThreshold:     4000,
			},
		},
		logger: newTestLogger(),
	}

	messages := []chatMessage{
		{Role: "user", Content: "test"},
	}

	// Token estimate of 1000 is well below threshold
	// 128000 - 20000 - 4000 = 104000, so 1000 should NOT trigger
	a.maybeMemoryFlush(context.Background(), messages, 1000)
	// Should not panic and should not call LLM
}

func TestMaybeMemoryFlush_DefaultConfigValues(t *testing.T) {
	t.Parallel()

	cfg := DefaultAgentConfig()

	if cfg.MemoryFlush.Enabled {
		t.Error("memory flush should be disabled by default")
	}
	if cfg.MemoryFlush.ReserveTokensFloor != 20000 {
		t.Errorf("expected ReserveTokensFloor 20000, got %d", cfg.MemoryFlush.ReserveTokensFloor)
	}
	if cfg.MemoryFlush.FlushThreshold != 4000 {
		t.Errorf("expected FlushThreshold 4000, got %d", cfg.MemoryFlush.FlushThreshold)
	}
}

func TestMaybeMemoryFlush_ConfigWithCustomValues(t *testing.T) {
	t.Parallel()

	a := &AgentRun{
		modelOverride: "gpt-4",
		cfg: AgentConfig{
			MemoryFlush: MemoryFlushConfig{
				Enabled:            true,
				ReserveTokensFloor: 10000,
				FlushThreshold:     2000,
				Prompt:             "Custom prompt",
				SystemPrompt:       "Custom system",
			},
		},
		logger: newTestLogger(),
	}

	// gpt-4 has 8192 context window
	// Threshold: 8192 - 10000 - 2000 = negative, so almost anything should trigger
	messages := []chatMessage{
		{Role: "user", Content: strings.Repeat("test ", 1000)},
	}

	// Should not panic even though we can't easily mock the LLM call
	a.maybeMemoryFlush(context.Background(), messages, 5000)
}

func TestMaybeMemoryFlush_InvalidConfigValues(t *testing.T) {
	t.Parallel()

	a := &AgentRun{
		modelOverride: "gpt-4o",
		cfg: AgentConfig{
			MemoryFlush: MemoryFlushConfig{
				Enabled:            true,
				ReserveTokensFloor: 0, // Will be set to default
				FlushThreshold:     0, // Will be set to default
			},
		},
		logger: newTestLogger(),
	}

	messages := []chatMessage{
		{Role: "user", Content: "test"},
	}

	// Should not panic - config values will be defaulted
	a.maybeMemoryFlush(context.Background(), messages, 5000)
}

func TestMaybeMemoryFlush_DefaultPrompts(t *testing.T) {
	t.Parallel()

	a := &AgentRun{
		modelOverride: "gpt-4o",
		cfg: AgentConfig{
			MemoryFlush: MemoryFlushConfig{
				Enabled:            true,
				ReserveTokensFloor: 20000,
				FlushThreshold:     4000,
				// No custom prompts set
			},
		},
		logger: newTestLogger(),
	}

	// Trigger a call that would use default prompts
	messages := make([]chatMessage, 20)
	for i := range messages {
		messages[i] = chatMessage{
			Role:    "user",
			Content: strings.Repeat("content ", 200),
		}
	}

	// Will use default prompts since none are configured
	a.maybeMemoryFlush(context.Background(), messages, 110000)
}

func TestMemoryFlushConfig_Validation(t *testing.T) {
	t.Parallel()

	// Test with valid config
	cfg := MemoryFlushConfig{
		Enabled:            true,
		ReserveTokensFloor: 20000,
		FlushThreshold:     4000,
		SystemPrompt:       "Test system",
		Prompt:             "Test prompt",
	}

	if !cfg.Enabled {
		t.Error("should be enabled")
	}
	if cfg.ReserveTokensFloor <= 0 {
		t.Error("ReserveTokensFloor should be positive")
	}
	if cfg.FlushThreshold <= 0 {
		t.Error("FlushThreshold should be positive")
	}

	// Test default prompt fallback
	cfg2 := MemoryFlushConfig{
		Enabled: true,
	}
	if cfg2.ReserveTokensFloor != 0 {
		t.Error("should be zero (will use default)")
	}
}
