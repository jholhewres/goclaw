package copilot

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestNewHookManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	hm := NewHookManager(logger)
	if hm == nil {
		t.Fatal("expected hook manager, got nil")
	}

	if hm.HookCount() != 0 {
		t.Errorf("expected 0 hooks, got %d", hm.HookCount())
	}
}

func TestHookManager_Register(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	tests := []struct {
		name    string
		hook    *RegisteredHook
		wantErr bool
	}{
		{
			name: "valid hook",
			hook: &RegisteredHook{
				Name:        "test-hook",
				Events:      []HookEvent{HookSessionStart},
				Handler:     func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
			},
			wantErr: false,
		},
		{
			name:    "nil hook",
			hook:    nil,
			wantErr: true,
		},
		{
			name: "nil handler",
			hook: &RegisteredHook{
				Name:   "no-handler",
				Events: []HookEvent{HookSessionStart},
			},
			wantErr: true,
		},
		{
			name: "no events",
			hook: &RegisteredHook{
				Name:    "no-events",
				Handler: func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := hm.Register(tt.hook)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHookManager_Dispatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	called := false
	hook := &RegisteredHook{
		Name:   "test-dispatch",
		Events: []HookEvent{HookSessionStart},
		Handler: func(ctx context.Context, p HookPayload) HookAction {
			called = true
			return HookAction{}
		},
	}

	if err := hm.Register(hook); err != nil {
		t.Fatalf("failed to register hook: %v", err)
	}

	action := hm.Dispatch(context.Background(), HookPayload{Event: HookSessionStart})

	if !called {
		t.Error("expected hook to be called")
	}
	if action.Block {
		t.Error("expected no block")
	}
}

func TestHookManager_Dispatch_Blocking(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	// Register a hook that blocks.
	blockingHook := &RegisteredHook{
		Name:     "blocking-hook",
		Events:   []HookEvent{HookPreToolUse},
		Priority: 10, // Run first
		Handler: func(ctx context.Context, p HookPayload) HookAction {
			return HookAction{Block: true, Reason: "blocked for testing"}
		},
	}

	// Register a hook that should not run.
	secondHookCalled := false
	secondHook := &RegisteredHook{
		Name:     "second-hook",
		Events:   []HookEvent{HookPreToolUse},
		Priority: 20, // Run second
		Handler: func(ctx context.Context, p HookPayload) HookAction {
			secondHookCalled = true
			return HookAction{}
		},
	}

	if err := hm.Register(blockingHook); err != nil {
		t.Fatalf("failed to register blocking hook: %v", err)
	}
	if err := hm.Register(secondHook); err != nil {
		t.Fatalf("failed to register second hook: %v", err)
	}

	action := hm.Dispatch(context.Background(), HookPayload{Event: HookPreToolUse})

	if !action.Block {
		t.Error("expected action to be blocked")
	}
	if action.Reason != "blocked for testing" {
		t.Errorf("reason = %q, want %q", action.Reason, "blocked for testing")
	}
	if secondHookCalled {
		t.Error("second hook should not have been called after block")
	}
}

func TestHookManager_Dispatch_ModifyArgs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	modifyingHook := &RegisteredHook{
		Name:   "modifying-hook",
		Events: []HookEvent{HookPreToolUse},
		Handler: func(ctx context.Context, p HookPayload) HookAction {
			return HookAction{
				ModifiedArgs: map[string]any{"modified": true},
			}
		},
	}

	if err := hm.Register(modifyingHook); err != nil {
		t.Fatalf("failed to register hook: %v", err)
	}

	action := hm.Dispatch(context.Background(), HookPayload{
		Event:   HookPreToolUse,
		ToolArgs: map[string]any{"original": true},
	})

	if action.ModifiedArgs == nil {
		t.Fatal("expected modified args")
	}
	if action.ModifiedArgs["modified"] != true {
		t.Error("expected modified arg to be set")
	}
}

func TestHookManager_HasHooks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	if hm.HasHooks(HookSessionStart) {
		t.Error("expected no hooks initially")
	}

	hook := &RegisteredHook{
		Name:   "test-has-hooks",
		Events: []HookEvent{HookSessionStart},
		Handler: func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
	}
	hm.Register(hook)

	if !hm.HasHooks(HookSessionStart) {
		t.Error("expected hooks for session_start")
	}
	if hm.HasHooks(HookSessionEnd) {
		t.Error("expected no hooks for session_end")
	}
}

func TestHookManager_SetEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	hook := &RegisteredHook{
		Name:    "test-toggle",
		Events:  []HookEvent{HookSessionStart},
		Enabled: true,
		Handler: func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
	}
	hm.Register(hook)

	// Disable the hook.
	if !hm.SetEnabled("test-toggle", false) {
		t.Error("expected to find hook")
	}

	// Verify it's disabled.
	hooks := hm.ListDetailed()
	for _, h := range hooks {
		if h.Name == "test-toggle" && h.Enabled {
			t.Error("expected hook to be disabled")
		}
	}

	// Re-enable.
	hm.SetEnabled("test-toggle", true)

	// Non-existent hook.
	if hm.SetEnabled("nonexistent", true) {
		t.Error("expected not to find nonexistent hook")
	}
}

func TestHookManager_Unregister(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	hook := &RegisteredHook{
		Name:   "test-unregister",
		Events: []HookEvent{HookSessionStart},
		Handler: func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
	}
	hm.Register(hook)

	if !hm.HasHooks(HookSessionStart) {
		t.Error("expected hooks after registration")
	}

	if !hm.Unregister("test-unregister") {
		t.Error("expected to find and unregister hook")
	}

	if hm.HasHooks(HookSessionStart) {
		t.Error("expected no hooks after unregister")
	}

	// Unregister non-existent.
	if hm.Unregister("nonexistent") {
		t.Error("expected not to find nonexistent hook")
	}
}

func TestHookManager_ListDetailed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	hm := NewHookManager(logger)

	hook1 := &RegisteredHook{
		Name:        "hook-one",
		Description: "First hook",
		Source:      "test",
		Events:      []HookEvent{HookSessionStart},
		Handler:     func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
	}
	hook2 := &RegisteredHook{
		Name:        "hook-two",
		Description: "Second hook",
		Source:      "test",
		Events:      []HookEvent{HookSessionEnd},
		Handler:     func(ctx context.Context, p HookPayload) HookAction { return HookAction{} },
	}

	hm.Register(hook1)
	hm.Register(hook2)

	list := hm.ListDetailed()
	if len(list) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(list))
	}

	// Check that descriptions are present.
	names := make(map[string]bool)
	for _, h := range list {
		names[h.Name] = true
		if h.Description == "" {
			t.Errorf("hook %s has no description", h.Name)
		}
	}

	if !names["hook-one"] || !names["hook-two"] {
		t.Error("missing expected hooks in list")
	}
}

func TestHookEventDescription(t *testing.T) {
	tests := []struct {
		event HookEvent
		want  string
	}{
		{HookSessionStart, "Sessão criada ou restaurada"},
		{HookSessionEnd, "Sessão encerrada ou removida"},
		{HookPreToolUse, "Antes de chamar uma ferramenta (pode bloquear/modificar)"},
		{HookPostToolUse, "Após o retorno de uma ferramenta"},
		{HookError, "Erro irrecuperável ocorreu"},
		{HookUserJoin, "Usuário aprovado/adicionado"},
		{HookChannelConnect, "Canal conectado"},
		{HookEvent("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := HookEventDescription(tt.event)
			if got != tt.want {
				t.Errorf("HookEventDescription(%q) = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}

func TestAllHookEvents(t *testing.T) {
	// Verify that AllHookEvents contains all expected events.
	expectedEvents := []HookEvent{
		HookSessionStart, HookSessionEnd,
		HookUserPromptSubmit,
		HookPreToolUse, HookPostToolUse,
		HookAgentStart, HookAgentStop,
		HookSubagentStart, HookSubagentStop,
		HookPreCompact, HookPostCompact,
		HookMemorySave, HookMemoryRecall,
		HookNotification,
		HookHeartbeat,
		HookError,
		HookUserJoin, HookUserLeave,
		HookChannelConnect, HookChannelDisconnect,
		// Advanced hooks
		HookBeforeModelResolve,
		HookBeforePromptBuild,
		HookLLMInput,
		HookLLMOutput,
		HookToolResultPersist,
	}

	if len(AllHookEvents) != len(expectedEvents) {
		t.Errorf("AllHookEvents has %d events, want %d", len(AllHookEvents), len(expectedEvents))
	}

	eventSet := make(map[HookEvent]bool)
	for _, ev := range AllHookEvents {
		eventSet[ev] = true
	}

	for _, expected := range expectedEvents {
		if !eventSet[expected] {
			t.Errorf("missing event in AllHookEvents: %s", expected)
		}
	}
}
