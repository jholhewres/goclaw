// Package copilot – llm.go implements the LLM client for chat completions
// with function calling / tool use support.
// Uses the OpenAI-compatible API format, which works with OpenAI, Anthropic
// proxies, GLM (api.z.ai), and any compatible endpoint.
package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ---------- Client ----------

// LLMClient handles communication with the LLM provider API.
type LLMClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewLLMClient creates a new LLM client from config.
func NewLLMClient(cfg *Config, logger *slog.Logger) *LLMClient {
	baseURL := cfg.API.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &LLMClient{
		baseURL: baseURL,
		apiKey:  cfg.API.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logger.With("component", "llm"),
	}
}

// ---------- Wire Types (OpenAI-compatible) ----------

// chatMessage represents a message in the OpenAI chat format.
// Supports user, system, assistant (with optional tool_calls), and tool result messages.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// chatRequest is the OpenAI-compatible chat completions request.
type chatRequest struct {
	Model    string           `json:"model"`
	Messages []chatMessage    `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

// chatResponse is the OpenAI-compatible chat completions response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// ---------- Tool Calling Types ----------

// ToolDefinition is an OpenAI-compatible tool definition for function calling.
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a callable function exposed to the LLM.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and serialized arguments from the LLM.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ---------- Response Types ----------

// LLMResponse holds the parsed response from a chat completion.
type LLMResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        LLMUsage
}

// LLMUsage holds token usage information from the API response.
type LLMUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ---------- Public Methods ----------

// Complete sends a simple chat completion request (no tools) and returns the text.
// Convenience wrapper around CompleteWithTools for non-agentic use cases.
func (c *LLMClient) Complete(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, error) {
	messages := make([]chatMessage, 0, len(history)*2+2)

	if systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, entry := range history {
		messages = append(messages, chatMessage{
			Role:    "user",
			Content: entry.UserMessage,
		})
		if entry.AssistantResponse != "" {
			messages = append(messages, chatMessage{
				Role:    "assistant",
				Content: entry.AssistantResponse,
			})
		}
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: userMessage,
	})

	resp, err := c.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// CompleteWithTools sends a chat completion request with optional tool definitions.
// Returns a structured response that may include tool calls the LLM wants to execute.
// If tools is nil/empty, behaves as a regular chat completion.
func (c *LLMClient) CompleteWithTools(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("API key not configured. Run 'copilot config set-key' or set GOCLAW_API_KEY")
	}

	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	c.logger.Debug("sending chat completion",
		"model", c.model,
		"messages", len(messages),
		"tools", len(tools),
		"endpoint", endpoint,
	)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("API error",
			"status", resp.StatusCode,
			"body", truncate(string(respBody), 500),
		)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from model")
	}

	choice := chatResp.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)

	c.logger.Info("chat completion done",
		"model", c.model,
		"duration_ms", duration.Milliseconds(),
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"finish_reason", choice.FinishReason,
		"tool_calls", len(choice.Message.ToolCalls),
	)

	return &LLMResponse{
		Content:      content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Usage: LLMUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}
