package copilot

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// mockReasoningExecutor provides a deterministic executor for reasoning tests
type mockReasoningExecutor struct{}

func (e *mockReasoningExecutor) Execute(ctx context.Context, tools []ToolCall) []ToolResult {
	var results []ToolResult
	for _, t := range tools {
		results = append(results, ToolResult{
			ToolCallID: t.ID,
			Name:       t.Function.Name,
			Content:    "mock output for " + t.Function.Name,
		})
	}
	return results
}

func TestAgentReasoningEnforcement(t *testing.T) {
	// Initialize a logger that discards or logs to stdout for debugging
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// The sequence of responses we want the mock LLM to return
	responses := []struct {
		content   string
		toolCalls []ToolCall
	}{
		{
			// First turn: The LLM forgets to close the think tag.
			content: "Let me think about this.\n<think>\nI need to call a tool, but I won't close my think tag.",
			toolCalls: []ToolCall{
				{
					ID: "call_1",
					Function: FunctionCall{
						Name:      "test_tool",
						Arguments: `{"arg": "value"}`,
					},
				},
			},
		},
		{
			// Second turn: The LLM is prompted to close the tag and does so.
			content: "</think>\nOkay, I have finished thinking. Now I will call the tool.",
			toolCalls: []ToolCall{
				{
					ID: "call_2",
					Function: FunctionCall{
						Name:      "test_tool",
						Arguments: `{"arg": "correct"}`,
					},
				},
			},
		},
		{
			// Third turn: Final reply after seeing the tool outputs.
			content:   "All done!",
			toolCalls: nil,
		},
	}

	callCount := 0
	var lastMessages []chatMessage

	// Create a mock HTTP server to simulate the OpenAI API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		// Decode the request to capture the messages for assertions
		var reqBody struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
			lastMessages = append(lastMessages, reqBody.Messages...)
		}

		if callCount >= len(responses) {
			t.Fatalf("LLM called more times than expected (%d)", callCount+1)
		}

		resp := responses[callCount]
		callCount++

		// Format the response as an OpenAI-compatible completion
		var toolsJson []map[string]any
		for _, tc := range resp.toolCalls {
			toolsJson = append(toolsJson, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				},
			})
		}

		openAIResp := map[string]any{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":       "assistant",
						"content":    resp.content,
						"tool_calls": toolsJson,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResp)
	}))
	defer server.Close()

	// Initialize the agent pointing to the mock server
	cfg := &Config{
		Name:  "DevClawTest",
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  server.URL,
			APIKey:   "test-key",
		},
	}
	llm := NewLLMClient(cfg, logger)

	executor := NewToolExecutor(logger)
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  []byte(`{}`),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		return "tool output", nil
	})
	// We can manually populate the executor or just use our mockReasoningExecutor type
	// But executor in AgentRun is a *ToolExecutor. Let's see if we can just register a tool.
	// We don't really need the tool's output to make the LLM return the next response in our mock,
	// because our mock just yields the predefined sequence regardless of the input message content!
	// So we can let the agent execute the real ToolExecutor which will probably fail if tool is not registered.
	// Actually, an unrecognized tool returns an error tool result, which is fine because the LLM is mocked
	// and will just return the next predetermined response!
	// Let's just create an empty ToolExecutor.

	agent := NewAgentRun(llm, executor, logger)
	// Disable reflection prompt injection to keep test deterministic
	agent.reflectionOn = false

	history := []ConversationEntry{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	finalResp, metrics, err := agent.RunWithUsage(ctx, "system prompt", history, "test request message")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}

	if finalResp != "All done!" {
		t.Errorf("Expected final response 'All done!', got '%s'", finalResp)
	}

	if metrics == nil {
		t.Fatal("Expected metrics but got nil")
	}

	if callCount != 3 {
		t.Errorf("Expected 3 LLM calls, but got %d", callCount)
	}

	// Check if the actual conversation history contains the system prompt about closing the tag
	if len(lastMessages) < 5 {
		t.Errorf("Expected at least 5 messages in last llm call, got %d", len(lastMessages))
	} else {
		foundWarning := false
		for _, msg := range lastMessages {
			if msg.Role == "user" {
				if content, ok := msg.Content.(string); ok && strings.Contains(content, "You opened a <think> tag but did not close it") {
					foundWarning = true
					break
				}
			}
		}
		if !foundWarning {
			t.Errorf("Expected the system warning about unclosed <think> tag, but it was not found in the messages sent to LLM.")
		}
	}
}
