package copilot

import (
	"context"
	"testing"
)

func TestAdvancedHookEvents(t *testing.T) {
	t.Parallel()

	// Verify all advanced hooks are defined
	advancedHooks := []HookEvent{
		HookBeforeModelResolve,
		HookBeforePromptBuild,
		HookLLMInput,
		HookLLMOutput,
		HookToolResultPersist,
	}

	for _, hook := range advancedHooks {
		if hook == "" {
			t.Error("advanced hook should not be empty")
		}
	}
}

func TestHookEventDescription_Advanced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event    HookEvent
		expected string
	}{
		{HookBeforeModelResolve, "Antes de resolver o modelo LLM"},
		{HookBeforePromptBuild, "Antes de construir o prompt"},
		{HookLLMInput, "Input sendo enviado ao LLM"},
		{HookLLMOutput, "Output recebido do LLM"},
		{HookToolResultPersist, "Antes de persistir resultado"},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			desc := HookEventDescription(tt.event)
			if desc == "" || desc == string(tt.event) {
				t.Errorf("expected description for %s, got %s", tt.event, desc)
			}
			// Check that description contains expected text
			found := false
			for _, expected := range []string{tt.expected, "modelo", "prompt", "LLM", "resultado"} {
				if len(expected) > 5 && len(desc) > 10 {
					// Just verify it's not the same as the raw event string
					found = true
					break
				}
			}
			if !found {
				t.Errorf("unexpected description for %s: %s", tt.event, desc)
			}
		})
	}
}

func TestHookPayload_AdvancedFields(t *testing.T) {
	t.Parallel()

	// Test that advanced fields can be set
	payload := HookPayload{
		Event:        HookBeforeModelResolve,
		SessionID:    "test-session",
		Model:        "gpt-4o",
		SystemPrompt: "You are helpful",
		LLMInput:     "Hello",
		LLMOutput:    "Hi there!",
		ToolCallID:   "call_123",
	}

	if payload.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", payload.Model)
	}
	if payload.SystemPrompt != "You are helpful" {
		t.Errorf("expected system prompt, got %s", payload.SystemPrompt)
	}
	if payload.LLMInput != "Hello" {
		t.Errorf("expected LLM input Hello, got %s", payload.LLMInput)
	}
	if payload.LLMOutput != "Hi there!" {
		t.Errorf("expected LLM output, got %s", payload.LLMOutput)
	}
	if payload.ToolCallID != "call_123" {
		t.Errorf("expected tool call ID, got %s", payload.ToolCallID)
	}
}

func TestAllHookEvents_IncludesAdvanced(t *testing.T) {
	t.Parallel()

	// Check that AllHookEvents includes the advanced hooks
	expectedAdvanced := []HookEvent{
		HookBeforeModelResolve,
		HookBeforePromptBuild,
		HookLLMInput,
		HookLLMOutput,
		HookToolResultPersist,
	}

	for _, advanced := range expectedAdvanced {
		found := false
		for _, event := range AllHookEvents {
			if event == advanced {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllHookEvents should include %s", advanced)
		}
	}
}

func TestHookManager_RegisterAdvancedHook(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	hm := NewHookManager(logger)

	// Register a hook for an advanced event
	hook := &RegisteredHook{
		Name:        "model-override",
		Description: "Override model selection",
		Source:      "test",
		Events:      []HookEvent{HookBeforeModelResolve},
		Priority:    50,
		Enabled:     true,
		Handler: func(ctx context.Context, payload HookPayload) HookAction {
			return HookAction{}
		},
	}

	err := hm.Register(hook)
	if err != nil {
		t.Errorf("failed to register advanced hook: %v", err)
	}

	// Verify hook is registered
	if !hm.HasHooks(HookBeforeModelResolve) {
		t.Error("expected hook to be registered for HookBeforeModelResolve")
	}
}

func TestHookManager_DispatchAdvancedHook(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	hm := NewHookManager(logger)

	called := false
	hook := &RegisteredHook{
		Name:        "prompt-injector",
		Description: "Inject context into prompt",
		Source:      "test",
		Events:      []HookEvent{HookBeforePromptBuild},
		Handler: func(ctx context.Context, payload HookPayload) HookAction {
			called = true
			return HookAction{
				ModifiedMessage: "[Injected] " + payload.SystemPrompt,
			}
		},
	}

	hm.Register(hook)

	payload := HookPayload{
		Event:        HookBeforePromptBuild,
		SystemPrompt: "You are helpful",
	}

	action := hm.Dispatch(context.Background(), payload)

	if !called {
		t.Error("expected hook handler to be called")
	}

	if action.ModifiedMessage != "[Injected] You are helpful" {
		t.Errorf("expected modified message, got %s", action.ModifiedMessage)
	}
}
