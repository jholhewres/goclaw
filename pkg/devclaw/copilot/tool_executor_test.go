package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
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

func TestToolResultDual(t *testing.T) {
	t.Run("DualToolResult creates result with separate content", func(t *testing.T) {
		result := DualToolResult(
			"Technical details: 15 results found for 'golang'",
			"Encontrei 15 resultados!",
		)

		if result.ForLLM != "Technical details: 15 results found for 'golang'" {
			t.Errorf("expected ForLLM to be technical, got %q", result.ForLLM)
		}
		if result.ForUser != "Encontrei 15 resultados!" {
			t.Errorf("expected ForUser to be friendly, got %q", result.ForUser)
		}
		if result.Content != result.ForLLM {
			t.Error("expected Content to equal ForLLM for backwards compatibility")
		}
	})

	t.Run("GetForLLM returns ForLLM when set", func(t *testing.T) {
		result := &ToolResult{
			Content: "fallback",
			ForLLM:  "technical content",
		}

		if result.GetForLLM() != "technical content" {
			t.Errorf("expected GetForLLM to return ForLLM, got %q", result.GetForLLM())
		}
	})

	t.Run("GetForLLM returns Content when ForLLM is empty", func(t *testing.T) {
		result := &ToolResult{
			Content: "fallback content",
		}

		if result.GetForLLM() != "fallback content" {
			t.Errorf("expected GetForLLM to return Content, got %q", result.GetForLLM())
		}
	})

	t.Run("GetForUser returns ForUser when set", func(t *testing.T) {
		result := &ToolResult{
			Content:  "technical",
			ForLLM:   "technical",
			ForUser:  "friendly message",
		}

		if result.GetForUser() != "friendly message" {
			t.Errorf("expected GetForUser to return ForUser, got %q", result.GetForUser())
		}
	})

	t.Run("GetForUser falls back to ForLLM", func(t *testing.T) {
		result := &ToolResult{
			Content: "technical",
			ForLLM:  "technical content",
		}

		if result.GetForUser() != "technical content" {
			t.Errorf("expected GetForUser to fall back to ForLLM, got %q", result.GetForUser())
		}
	})
}

func TestSilentResult(t *testing.T) {
	result := SilentResult("background operation started")

	if result.Content != "background operation started" {
		t.Errorf("expected Content, got %q", result.Content)
	}
	if !result.IsSilent {
		t.Error("expected IsSilent to be true")
	}
	if result.ForLLM != result.Content {
		t.Error("expected ForLLM to equal Content")
	}
}

func TestAsyncResult(t *testing.T) {
	result := AsyncResult("Task started in background (ID: abc123)")

	if result.Content != "Task started in background (ID: abc123)" {
		t.Errorf("expected Content, got %q", result.Content)
	}
	if !result.IsAsync {
		t.Error("expected IsAsync to be true")
	}
	if result.ForUser != result.Content {
		t.Error("expected ForUser to equal Content for async results")
	}
}

func TestErrorResult(t *testing.T) {
	t.Run("creates result with error", func(t *testing.T) {
		err := fmt.Errorf("something went wrong")
		result := ErrorResult(err)

		if result.Error != err {
			t.Error("expected Error to be the original error")
		}
		if result.Content != "something went wrong" {
			t.Errorf("expected Content to be error message, got %q", result.Content)
		}
		if result.ForLLM != "something went wrong" {
			t.Errorf("expected ForLLM to be error message, got %q", result.ForLLM)
		}
		if result.ForUser != "An error occurred. Please try again." {
			t.Errorf("expected ForUser to be friendly message, got %q", result.ForUser)
		}
	})

	t.Run("GetForLLM returns technical error", func(t *testing.T) {
		err := fmt.Errorf("technical error details")
		result := ErrorResult(err)

		if result.GetForLLM() != "technical error details" {
			t.Errorf("expected GetForLLM to return technical error, got %q", result.GetForLLM())
		}
	})

	t.Run("GetForUser returns friendly message", func(t *testing.T) {
		err := fmt.Errorf("technical error details")
		result := ErrorResult(err)

		if result.GetForUser() != "An error occurred. Please try again." {
			t.Errorf("expected GetForUser to return friendly message, got %q", result.GetForUser())
		}
	})
}

func TestToolResultFlags(t *testing.T) {
	t.Run("IsAsync flag", func(t *testing.T) {
		result := &ToolResult{IsAsync: true}
		if !result.IsAsync {
			t.Error("expected IsAsync to be true")
		}
	})

	t.Run("IsSilent flag", func(t *testing.T) {
		result := &ToolResult{IsSilent: true}
		if !result.IsSilent {
			t.Error("expected IsSilent to be true")
		}
	})

	t.Run("combined flags", func(t *testing.T) {
		result := &ToolResult{
			Content:  "test",
			ForLLM:   "technical",
			ForUser:  "friendly",
			IsAsync:  true,
			IsSilent: true,
		}

		if !result.IsAsync || !result.IsSilent {
			t.Error("expected both flags to be true")
		}
	})
}

func TestGetDeliveryTarget(t *testing.T) {
	t.Run("extracts channel and chatID from context", func(t *testing.T) {
		ctx := ContextWithDelivery(context.Background(), "telegram", "chat123")
		channel, chatID := GetDeliveryTarget(ctx)

		if channel != "telegram" {
			t.Errorf("expected channel 'telegram', got %q", channel)
		}
		if chatID != "chat123" {
			t.Errorf("expected chatID 'chat123', got %q", chatID)
		}
	})

	t.Run("returns empty strings when not set", func(t *testing.T) {
		ctx := context.Background()
		channel, chatID := GetDeliveryTarget(ctx)

		if channel != "" {
			t.Errorf("expected empty channel, got %q", channel)
		}
		if chatID != "" {
			t.Errorf("expected empty chatID, got %q", chatID)
		}
	})
}

// mockContextualTool is a test implementation of ContextualTool
type mockContextualTool struct {
	channel string
	chatID  string
}

func (m *mockContextualTool) SetDeliveryTarget(channel, chatID string) {
	m.channel = channel
	m.chatID = chatID
}

func TestContextualToolInterface(t *testing.T) {
	t.Run("SetDeliveryTarget sets values", func(t *testing.T) {
		tool := &mockContextualTool{}
		tool.SetDeliveryTarget("whatsapp", "5511999999999")

		if tool.channel != "whatsapp" {
			t.Errorf("expected channel 'whatsapp', got %q", tool.channel)
		}
		if tool.chatID != "5511999999999" {
			t.Errorf("expected chatID '5511999999999', got %q", tool.chatID)
		}
	})

	t.Run("interface can be checked with assertion", func(t *testing.T) {
		var handler any = &mockContextualTool{}

		_, ok := handler.(ContextualTool)
		if !ok {
			t.Error("expected mockContextualTool to implement ContextualTool")
		}
	})
}

func TestRunAsync(t *testing.T) {
	t.Run("executes function in background", func(t *testing.T) {
		completed := make(chan *ToolResult, 1)

		config := AsyncToolConfig{
			Label: "test task",
			OnComplete: func(result *ToolResult) {
				completed <- result
			},
			Timeout: 1 * time.Second,
		}

		RunAsync(context.Background(), config, func(ctx context.Context) *ToolResult {
			return &ToolResult{Content: "task completed"}
		})

		// Wait for completion
		select {
		case result := <-completed:
			if result.Content != "task completed" {
				t.Errorf("expected 'task completed', got %q", result.Content)
			}
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for async task to complete")
		}
	})

	t.Run("uses default timeout when not specified", func(t *testing.T) {
		config := AsyncToolConfig{
			Label:     "test task",
			OnComplete: func(result *ToolResult) {},
		}

		// Default timeout should be 5 minutes
		if config.Timeout != 0 {
			t.Error("expected default timeout to be 0 (will use 5m default)")
		}
	})

	t.Run("callback can be nil", func(t *testing.T) {
		// This should not panic
		config := AsyncToolConfig{
			Label:     "test task",
			OnComplete: nil,
			Timeout:   100 * time.Millisecond,
		}

		RunAsync(context.Background(), config, func(ctx context.Context) *ToolResult {
			return &ToolResult{Content: "done"}
		})

		// Give it time to complete
		time.Sleep(150 * time.Millisecond)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		completed := make(chan *ToolResult, 1)
		cancelled := make(chan struct{})

		ctx, cancel := context.WithCancel(context.Background())

		config := AsyncToolConfig{
			Label: "cancellable task",
			OnComplete: func(result *ToolResult) {
				completed <- result
			},
			Timeout: 5 * time.Second,
		}

		RunAsync(ctx, config, func(ctx context.Context) *ToolResult {
			// Simulate work that checks for cancellation
			select {
			case <-ctx.Done():
				close(cancelled)
				return &ToolResult{Content: "cancelled", Error: ctx.Err()}
			case <-time.After(2 * time.Second):
				return &ToolResult{Content: "completed"}
			}
		})

		// Cancel after a short delay
		time.Sleep(100 * time.Millisecond)
		cancel()

		// Verify cancellation was detected
		select {
		case <-cancelled:
			// Good - task was cancelled
		case <-time.After(1 * time.Second):
			t.Error("expected task to be cancelled")
		}

		// Callback should still be called even when cancelled
		select {
		case result := <-completed:
			if result.Content != "cancelled" {
				t.Errorf("expected 'cancelled', got %q", result.Content)
			}
		case <-time.After(1 * time.Second):
			t.Error("expected callback to be called even after cancellation")
		}
	})
}

func TestAsyncToolConfig(t *testing.T) {
	t.Run("config holds label", func(t *testing.T) {
		config := AsyncToolConfig{Label: "my task"}
		if config.Label != "my task" {
			t.Errorf("expected label 'my task', got %q", config.Label)
		}
	})

	t.Run("config holds timeout", func(t *testing.T) {
		config := AsyncToolConfig{Timeout: 30 * time.Second}
		if config.Timeout != 30*time.Second {
			t.Errorf("expected 30s timeout, got %v", config.Timeout)
		}
	})
}

// mockVaultReader is a test implementation of skills.VaultReader
type mockVaultReader struct {
	keys map[string]string
}

func (m *mockVaultReader) Get(key string) (string, error) {
	if v, ok := m.keys[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("key not found")
}

func (m *mockVaultReader) Has(key string) bool {
	_, ok := m.keys[key]
	return ok
}

func TestVaultReaderAdapter(t *testing.T) {
	t.Run("Get returns value", func(t *testing.T) {
		adapter := NewVaultReaderAdapter(
			func(key string) (string, error) { return "value-" + key, nil },
			func(key string) bool { return key == "exists" },
		)

		val, err := adapter.Get("test")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if val != "value-test" {
			t.Errorf("expected 'value-test', got %q", val)
		}
	})

	t.Run("Has returns true for existing key", func(t *testing.T) {
		adapter := NewVaultReaderAdapter(
			func(key string) (string, error) { return "", nil },
			func(key string) bool { return key == "exists" },
		)

		if !adapter.Has("exists") {
			t.Error("expected Has to return true")
		}
		if adapter.Has("notexists") {
			t.Error("expected Has to return false")
		}
	})
}

func TestContextWithVaultReader(t *testing.T) {
	t.Run("sets and gets vault reader", func(t *testing.T) {
		reader := &mockVaultReader{keys: map[string]string{"test": "value"}}
		ctx := ContextWithVaultReader(context.Background(), reader)

		got := VaultReaderFromContext(ctx)
		if got == nil {
			t.Fatal("expected non-nil vault reader")
		}
		if !got.Has("test") {
			t.Error("expected vault reader to have 'test' key")
		}
	})

	t.Run("returns nil when not set", func(t *testing.T) {
		got := VaultReaderFromContext(context.Background())
		if got != nil {
			t.Error("expected nil vault reader")
		}
	})
}

func TestFormatSetupPrompt(t *testing.T) {
	t.Run("returns empty for complete setup", func(t *testing.T) {
		status := &skills.SetupStatus{IsComplete: true}
		prompt := FormatSetupPrompt("test", status)
		if prompt != "" {
			t.Errorf("expected empty prompt, got %q", prompt)
		}
	})

	t.Run("returns prompt for missing config", func(t *testing.T) {
		status := &skills.SetupStatus{
			IsComplete: false,
			MissingRequirements: []skills.ConfigRequirement{
				{Key: "API_KEY", Name: "API Key", Description: "Your API key", Example: "abc123"},
			},
			Message: "Missing API_KEY",
		}
		prompt := FormatSetupPrompt("my-skill", status)

		if !strings.Contains(prompt, "my-skill") {
			t.Error("expected prompt to contain skill name")
		}
		if !strings.Contains(prompt, "API Key") {
			t.Error("expected prompt to contain config name")
		}
		if !strings.Contains(prompt, "abc123") {
			t.Error("expected prompt to contain example")
		}
		if !strings.Contains(prompt, "Example:") {
			t.Error("expected prompt to contain 'Example:' label")
		}
	})
}

func TestAsyncCompleteCallback(t *testing.T) {
	t.Run("callback receives result", func(t *testing.T) {
		var receivedResult *ToolResult

		callback := AsyncCompleteCallback(func(result *ToolResult) {
			receivedResult = result
		})

		testResult := &ToolResult{
			Content:  "test output",
			ForUser:  "User message",
			IsAsync:  true,
			IsSilent: false,
		}

		callback(testResult)

		if receivedResult == nil {
			t.Fatal("expected callback to receive result")
		}
		if receivedResult.Content != "test output" {
			t.Errorf("expected 'test output', got %q", receivedResult.Content)
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
