package copilot

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestToolExecutorNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("creates executor with defaults", func(t *testing.T) {
		exec := NewToolExecutor(logger)

		if exec == nil {
			t.Fatal("expected non-nil executor")
		}
		if exec.timeout != DefaultToolTimeout {
			t.Errorf("expected default timeout %v, got %v", DefaultToolTimeout, exec.timeout)
		}
		if exec.bashTimeout != 5*time.Minute {
			t.Errorf("expected bash timeout 5m, got %v", exec.bashTimeout)
		}
		if len(exec.tools) != 0 {
			t.Errorf("expected empty tools map, got %d", len(exec.tools))
		}
	})
}

func TestToolExecutorAbort(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	exec := NewToolExecutor(logger)

	t.Run("initial state not aborted", func(t *testing.T) {
		if exec.IsAborted() {
			t.Error("expected not aborted initially")
		}
	})

	t.Run("abort signals stop", func(t *testing.T) {
		exec.Abort()
		if !exec.IsAborted() {
			t.Error("expected aborted after Abort()")
		}
	})

	t.Run("reset creates fresh channel", func(t *testing.T) {
		exec.ResetAbort()
		if exec.IsAborted() {
			t.Error("expected not aborted after ResetAbort")
		}
	})
}

func TestToolExecutorAbortCh(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	exec := NewToolExecutor(logger)

	t.Run("channel not closed initially", func(t *testing.T) {
		ch := exec.AbortCh()
		select {
		case <-ch:
			t.Error("expected channel to be open")
		default:
			// Good - channel is open
		}
	})

	t.Run("channel closed after abort", func(t *testing.T) {
		exec.Abort()
		ch := exec.AbortCh()
		select {
		case <-ch:
			// Good - channel is closed
		default:
			t.Error("expected channel to be closed after abort")
		}
	})
}

func TestToolExecutorGuard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	exec := NewToolExecutor(logger)

	t.Run("nil guard initially", func(t *testing.T) {
		guard := exec.Guard()
		if guard != nil {
			t.Error("expected nil guard initially")
		}
	})

	t.Run("SetGuard configures guard", func(t *testing.T) {
		newGuard := NewToolGuard(ToolGuardConfig{Enabled: true}, logger)
		exec.SetGuard(newGuard)
		guard := exec.Guard()
		if guard == nil {
			t.Error("expected non-nil guard after SetGuard")
		}
	})
}

func TestToolExecutorRegisterHook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	exec := NewToolExecutor(logger)

	t.Run("registers hooks", func(t *testing.T) {
		hook := &ToolHook{
			Name: "test-hook",
			BeforeToolCall: func(toolName string, args map[string]any) (map[string]any, bool, string) {
				return args, false, ""
			},
		}

		exec.RegisterHook(hook)

		if len(exec.hooks) != 1 {
			t.Errorf("expected 1 hook, got %d", len(exec.hooks))
		}
	})
}

func TestContextFunctions(t *testing.T) {
	ctx := context.Background()

	t.Run("ContextWithSession and SessionIDFromContext", func(t *testing.T) {
		ctx := ContextWithSession(context.Background(), "test-session-123")
		sessionID := SessionIDFromContext(ctx)

		if sessionID != "test-session-123" {
			t.Errorf("expected 'test-session-123', got %q", sessionID)
		}
	})

	t.Run("SessionIDFromContext returns empty if not set", func(t *testing.T) {
		sessionID := SessionIDFromContext(ctx)
		if sessionID != "" {
			t.Errorf("expected empty string, got %q", sessionID)
		}
	})

	t.Run("ContextWithDelivery and DeliveryTargetFromContext", func(t *testing.T) {
		ctx := ContextWithDelivery(context.Background(), "whatsapp", "5511999999999")
		target := DeliveryTargetFromContext(ctx)

		if target.Channel != "whatsapp" {
			t.Errorf("expected channel 'whatsapp', got %q", target.Channel)
		}
		if target.ChatID != "5511999999999" {
			t.Errorf("expected chatID '5511999999999', got %q", target.ChatID)
		}
	})

	t.Run("DeliveryTargetFromContext returns empty if not set", func(t *testing.T) {
		target := DeliveryTargetFromContext(ctx)
		if target.Channel != "" || target.ChatID != "" {
			t.Errorf("expected empty delivery target, got %+v", target)
		}
	})

	t.Run("ContextWithCaller and CallerLevelFromContext", func(t *testing.T) {
		ctx := ContextWithCaller(context.Background(), AccessAdmin, "user@domain.com")
		level := CallerLevelFromContext(ctx)
		jid := CallerJIDFromContext(ctx)

		if level != AccessAdmin {
			t.Errorf("expected AccessAdmin, got %v", level)
		}
		if jid != "user@domain.com" {
			t.Errorf("expected 'user@domain.com', got %q", jid)
		}
	})

	t.Run("CallerLevelFromContext returns AccessNone if not set", func(t *testing.T) {
		level := CallerLevelFromContext(ctx)
		if level != AccessNone {
			t.Errorf("expected AccessNone, got %v", level)
		}
	})

	t.Run("CallerJIDFromContext returns empty if not set", func(t *testing.T) {
		jid := CallerJIDFromContext(ctx)
		if jid != "" {
			t.Errorf("expected empty string, got %q", jid)
		}
	})
}

func TestContextWithToolProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("sets and gets tool profile", func(t *testing.T) {
		profile := &ToolProfile{
			Name:        "test-profile",
			Description: "Test profile",
			Allow:       []string{"group:memory"},
		}

		ctx := ContextWithToolProfile(context.Background(), profile)
		got := ToolProfileFromContext(ctx)

		if got == nil {
			t.Fatal("expected non-nil profile")
		}
		if got.Name != "test-profile" {
			t.Errorf("expected name 'test-profile', got %q", got.Name)
		}
	})

	t.Run("returns nil if not set", func(t *testing.T) {
		got := ToolProfileFromContext(ctx)
		if got != nil {
			t.Error("expected nil profile")
		}
	})
}

func TestContextWithProgressSender(t *testing.T) {
	ctx := context.Background()

	t.Run("sets and gets progress sender", func(t *testing.T) {
		called := false
		sender := func(ctx context.Context, message string) {
			called = true
		}

		ctx := ContextWithProgressSender(context.Background(), sender)
		got := ProgressSenderFromContext(ctx)

		if got == nil {
			t.Fatal("expected non-nil sender")
		}

		got(ctx, "test")
		if !called {
			t.Error("expected sender to be called")
		}
	})

	t.Run("returns nil if not set", func(t *testing.T) {
		got := ProgressSenderFromContext(ctx)
		if got != nil {
			t.Error("expected nil sender")
		}
	})
}

func TestToolNameSanitizer(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"valid_name", "valid_name"},
		{"valid-name-123", "valid-name-123"},
		{"invalid name", "invalid_name"},
		{"invalid@name!", "invalid_name_"},
		{"with/slash", "with_slash"},
		{"with.dot", "with_dot"},
		{"UPPERCASE", "UPPERCASE"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toolNameSanitizer.ReplaceAllString(tt.input, "_")
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDeliveryTarget(t *testing.T) {
	t.Run("struct holds channel and chatID", func(t *testing.T) {
		target := DeliveryTarget{
			Channel: "whatsapp",
			ChatID:  "5511999999999",
		}

		if target.Channel != "whatsapp" {
			t.Errorf("expected channel 'whatsapp', got %q", target.Channel)
		}
		if target.ChatID != "5511999999999" {
			t.Errorf("expected chatID '5511999999999', got %q", target.ChatID)
		}
	})
}

func TestToolResult(t *testing.T) {
	t.Run("struct holds tool execution result", func(t *testing.T) {
		result := ToolResult{
			ToolCallID: "call-123",
			Name:       "bash",
			Content:    "output",
			Error:      nil,
		}

		if result.ToolCallID != "call-123" {
			t.Errorf("expected ToolCallID 'call-123', got %q", result.ToolCallID)
		}
		if result.Name != "bash" {
			t.Errorf("expected Name 'bash', got %q", result.Name)
		}
		if result.Content != "output" {
			t.Errorf("expected Content 'output', got %q", result.Content)
		}
	})
}

func TestToolHook(t *testing.T) {
	t.Run("BeforeToolCall can modify args", func(t *testing.T) {
		hook := &ToolHook{
			Name: "test-hook",
			BeforeToolCall: func(toolName string, args map[string]any) (map[string]any, bool, string) {
				args["modified"] = true
				return args, false, ""
			},
		}

		args := map[string]any{"original": true}
		modified, blocked, reason := hook.BeforeToolCall("test", args)

		if blocked {
			t.Error("expected not blocked")
		}
		if reason != "" {
			t.Errorf("expected empty reason, got %q", reason)
		}
		if !modified["modified"].(bool) {
			t.Error("expected args to be modified")
		}
	})

	t.Run("BeforeToolCall can block execution", func(t *testing.T) {
		hook := &ToolHook{
			Name: "blocking-hook",
			BeforeToolCall: func(toolName string, args map[string]any) (map[string]any, bool, string) {
				return args, true, "blocked for testing"
			},
		}

		args := map[string]any{}
		_, blocked, reason := hook.BeforeToolCall("test", args)

		if !blocked {
			t.Error("expected to be blocked")
		}
		if reason != "blocked for testing" {
			t.Errorf("expected reason 'blocked for testing', got %q", reason)
		}
	})

	t.Run("AfterToolCall observes result", func(t *testing.T) {
		observed := false
		hook := &ToolHook{
			Name: "observing-hook",
			AfterToolCall: func(toolName string, args map[string]any, result string, err error) {
				observed = true
			},
		}

		hook.AfterToolCall("test", nil, "result", nil)

		if !observed {
			t.Error("expected AfterToolCall to be called")
		}
	})
}
