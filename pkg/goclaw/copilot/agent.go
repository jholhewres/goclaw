// Package copilot – agent.go implements the agentic loop that orchestrates
// LLM calls with tool execution. The agent iterates: call LLM → if tool_calls
// → execute tools → append results → call LLM again, until the LLM produces
// a final text response or the turn limit is reached.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	// DefaultMaxTurns is the maximum number of LLM round-trips in a single agent run.
	DefaultMaxTurns = 10

	// DefaultTurnTimeout is the timeout for a single LLM call within the loop.
	DefaultTurnTimeout = 60 * time.Second
)

// AgentRun encapsulates a single agent execution with its dependencies.
type AgentRun struct {
	llm         *LLMClient
	executor    *ToolExecutor
	maxTurns    int
	turnTimeout time.Duration
	logger      *slog.Logger
}

// NewAgentRun creates a new agent runner.
func NewAgentRun(llm *LLMClient, executor *ToolExecutor, logger *slog.Logger) *AgentRun {
	return &AgentRun{
		llm:         llm,
		executor:    executor,
		maxTurns:    DefaultMaxTurns,
		turnTimeout: DefaultTurnTimeout,
		logger:      logger.With("component", "agent"),
	}
}

// Run executes the agent loop: builds the initial message list from conversation
// history, then iterates LLM calls and tool executions until a final response
// is produced or the turn limit is exhausted.
func (a *AgentRun) Run(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, error) {
	// Build initial messages from history.
	messages := a.buildMessages(systemPrompt, history, userMessage)

	// Collect tool definitions from the executor.
	tools := a.executor.Tools()

	a.logger.Debug("agent run started",
		"history_entries", len(history),
		"tools_available", len(tools),
		"max_turns", a.maxTurns,
	)

	// If no tools are registered, do a single completion and return.
	if len(tools) == 0 {
		resp, err := a.llm.CompleteWithTools(ctx, messages, nil)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}

	// Agent loop: LLM call → tool execution → repeat.
	for turn := 1; turn <= a.maxTurns; turn++ {
		a.logger.Debug("agent turn",
			"turn", turn,
			"messages", len(messages),
		)

		// Call LLM with a per-turn timeout.
		turnCtx, cancel := context.WithTimeout(ctx, a.turnTimeout)
		resp, err := a.llm.CompleteWithTools(turnCtx, messages, tools)
		cancel()

		if err != nil {
			return "", fmt.Errorf("LLM call failed (turn %d): %w", turn, err)
		}

		// No tool calls → final response.
		if len(resp.ToolCalls) == 0 {
			a.logger.Info("agent completed",
				"turns", turn,
				"response_len", len(resp.Content),
			)
			return resp.Content, nil
		}

		// Append assistant message with tool calls to the conversation.
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute all requested tool calls.
		a.logger.Info("executing tool calls",
			"count", len(resp.ToolCalls),
			"turn", turn,
		)

		results := a.executor.Execute(ctx, resp.ToolCalls)

		// Append each tool result as a message.
		for _, result := range results {
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
		}
	}

	// If we exhausted all turns, make one final call without tools
	// to get a summary response.
	a.logger.Warn("agent reached max turns, requesting summary",
		"max_turns", a.maxTurns,
	)

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: "[System: You have used all available tool calls. Please provide your best response with the information gathered so far.]",
	})

	resp, err := a.llm.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("final summary call failed: %w", err)
	}

	return resp.Content, nil
}

// buildMessages converts conversation history into the chat message format.
func (a *AgentRun) buildMessages(systemPrompt string, history []ConversationEntry, userMessage string) []chatMessage {
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

	return messages
}
