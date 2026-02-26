package copilot

import (
	"testing"
)

func TestPromptMode_Constants(t *testing.T) {
	t.Parallel()

	if PromptModeFull != "full" {
		t.Errorf("expected PromptModeFull to be 'full', got %s", PromptModeFull)
	}
	if PromptModeMinimal != "minimal" {
		t.Errorf("expected PromptModeMinimal to be 'minimal', got %s", PromptModeMinimal)
	}
	if PromptModeNone != "none" {
		t.Errorf("expected PromptModeNone to be 'none', got %s", PromptModeNone)
	}
}

func TestPromptComposer_ComposeWithMode_Full(t *testing.T) {
	t.Parallel()

	// This test verifies that full mode doesn't panic
	// A full integration test would require more setup

	composer := NewPromptComposer(&Config{
		Name:         "Test",
		Instructions: "Test instructions",
	})

	session := &Session{
		ID: "test-session",
	}

	// Full mode should delegate to Compose
	// Just verify it doesn't panic
	_ = composer.ComposeWithMode(session, "test input", PromptModeFull)
}

func TestPromptComposer_ComposeWithMode_Minimal(t *testing.T) {
	t.Parallel()

	composer := NewPromptComposer(&Config{
		Name:         "Test",
		Instructions: "Test instructions for minimal mode",
	})

	session := &Session{
		ID:      "test-session",
		ChatID:  "test-chat",
		Channel: "test",
		config: SessionConfig{
			BusinessContext: "Test business context",
		},
	}

	result := composer.ComposeWithMode(session, "test input", PromptModeMinimal)

	// Minimal mode should include core layers
	if result == "" {
		t.Error("expected non-empty prompt in minimal mode")
	}
}

func TestPromptComposer_ComposeWithMode_None(t *testing.T) {
	t.Parallel()

	composer := NewPromptComposer(&Config{
		Name:         "Test",
		Instructions: "Short",
	})

	session := &Session{
		ID: "test-session",
	}

	result := composer.ComposeWithMode(session, "test input", PromptModeNone)

	// None mode should include at least core layers
	if result == "" {
		t.Error("expected non-empty prompt in none mode")
	}
}

func TestPromptComposer_ComposeWithMode_NoneLongInstructions(t *testing.T) {
	t.Parallel()

	// When instructions are long (>200 chars), they should NOT be included in None mode
	longInstructions := "This is a very long instruction that should not be included when using PromptModeNone because it would defeat the purpose of having a minimal prompt for simple tasks."

	composer := NewPromptComposer(&Config{
		Name:         "Test",
		Instructions: longInstructions,
	})

	session := &Session{
		ID: "test-session",
	}

	result := composer.ComposeWithMode(session, "test input", PromptModeNone)

	// Should still produce output
	if result == "" {
		t.Error("expected non-empty prompt even with long instructions")
	}
}

func TestPromptComposer_ComposeWithMode_Invalid(t *testing.T) {
	t.Parallel()

	composer := NewPromptComposer(&Config{
		Name: "Test",
	})

	session := &Session{
		ID: "test-session",
	}

	// Invalid/empty mode should default to core layers only
	result := composer.ComposeWithMode(session, "test input", "invalid")

	if result == "" {
		t.Error("expected non-empty prompt for invalid mode")
	}
}
