// Package copilot – agent.go implements the agentic loop that orchestrates
// LLM calls with tool execution. The agent iterates: call LLM → if tool_calls
// → execute tools → append results → call LLM again, until the LLM produces
// a final text response or the turn limit is reached.
//
// Autonomy features:
//   - Configurable max turns (default: 25).
//   - Auto-continue: when turns are exhausted but the agent is mid-task, it
//     can automatically start a continuation run (up to MaxContinuations).
//   - Reflection: the agent periodically sees a "[System: N turns used of M]"
//     nudge so it can self-manage its budget.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// DefaultMaxTurns is the maximum number of LLM round-trips in a single agent run.
	DefaultMaxTurns = 25

	// DefaultTurnTimeout is the timeout for a single LLM call within the loop.
	// 60s covers cold starts and complex responses while cutting wait time on
	// hung requests. Most LLM calls complete in 5-30s. Individual tools (bash,
	// ssh) have their own longer timeouts.
	DefaultTurnTimeout = 60 * time.Second

	// DefaultMaxContinuations is how many times the agent can auto-continue
	// after exhausting its turn budget. 0 = no auto-continue.
	DefaultMaxContinuations = 2

	// reflectionInterval is how often (in turns) the agent receives a budget nudge.
	reflectionInterval = 8

	// DefaultMaxCompactionAttempts is how many times to retry after context overflow compaction.
	DefaultMaxCompactionAttempts = 3
)

// AgentConfig holds configurable agent loop parameters.
type AgentConfig struct {
	// MaxTurns is the max LLM round-trips per agent run (default: 25).
	MaxTurns int `yaml:"max_turns"`

	// TurnTimeoutSeconds is the max seconds per LLM call (default: 300).
	TurnTimeoutSeconds int `yaml:"turn_timeout_seconds"`

	// MaxContinuations is how many auto-continue rounds are allowed when
	// the agent exhausts its turn budget while still using tools.
	// 0 = disabled (agent must summarize). Default: 2.
	MaxContinuations int `yaml:"max_continuations"`

	// ReflectionEnabled enables periodic budget awareness nudges (default: true).
	ReflectionEnabled bool `yaml:"reflection_enabled"`

	// MaxCompactionAttempts is how many times to retry after context overflow (default: 3).
	MaxCompactionAttempts int `yaml:"max_compaction_attempts"`
}

// DefaultAgentConfig returns sensible defaults for agent autonomy.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		MaxTurns:              DefaultMaxTurns,
		TurnTimeoutSeconds:    int(DefaultTurnTimeout / time.Second),
		MaxContinuations:      DefaultMaxContinuations,
		ReflectionEnabled:     true,
		MaxCompactionAttempts: DefaultMaxCompactionAttempts,
	}
}

// AgentRun encapsulates a single agent execution with its dependencies.
type AgentRun struct {
	llm                   *LLMClient
	executor              *ToolExecutor
	maxTurns              int
	turnTimeout           time.Duration
	maxContinuations      int
	reflectionOn          bool
	maxCompactionAttempts int
	streamCallback        StreamCallback
	modelOverride         string   // When set, use this model instead of default.
	usageRecorder         func(model string, usage LLMUsage) // Called after each successful LLM response.

	// interruptCh receives follow-up user messages that should be injected into
	// the active agent loop. Between turns, the agent drains this channel and
	// appends the messages to the conversation before the next LLM call.
	// This enables Claude Code-style live message injection.
	interruptCh <-chan string

	logger *slog.Logger
}

// NewAgentRun creates a new agent runner.
func NewAgentRun(llm *LLMClient, executor *ToolExecutor, logger *slog.Logger) *AgentRun {
	return &AgentRun{
		llm:                   llm,
		executor:              executor,
		maxTurns:              DefaultMaxTurns,
		turnTimeout:           DefaultTurnTimeout,
		maxContinuations:      DefaultMaxContinuations,
		reflectionOn:          true,
		maxCompactionAttempts: DefaultMaxCompactionAttempts,
		logger:                logger.With("component", "agent"),
	}
}

// NewAgentRunWithConfig creates a new agent runner with explicit configuration.
func NewAgentRunWithConfig(llm *LLMClient, executor *ToolExecutor, cfg AgentConfig, logger *slog.Logger) *AgentRun {
	ar := NewAgentRun(llm, executor, logger)
	if cfg.MaxTurns > 0 {
		ar.maxTurns = cfg.MaxTurns
	}
	if cfg.TurnTimeoutSeconds > 0 {
		ar.turnTimeout = time.Duration(cfg.TurnTimeoutSeconds) * time.Second
	}
	if cfg.MaxContinuations >= 0 {
		ar.maxContinuations = cfg.MaxContinuations
	}
	ar.reflectionOn = cfg.ReflectionEnabled
	if cfg.MaxCompactionAttempts > 0 {
		ar.maxCompactionAttempts = cfg.MaxCompactionAttempts
	}
	return ar
}

// SetStreamCallback sets the callback for streaming text deltas.
// When set, the agent uses CompleteWithToolsStream; only text content is forwarded,
// tool calls are accumulated silently.
func (a *AgentRun) SetStreamCallback(cb StreamCallback) {
	a.streamCallback = cb
}

// SetModelOverride sets the model to use instead of the default.
// Empty string means use the LLM client's default.
func (a *AgentRun) SetModelOverride(model string) {
	a.modelOverride = model
}

// SetUsageRecorder sets a callback invoked after each successful LLM response.
func (a *AgentRun) SetUsageRecorder(fn func(model string, usage LLMUsage)) {
	a.usageRecorder = fn
}

// SetInterruptChannel sets the channel for receiving follow-up user messages
// during agent execution. Messages received on this channel are injected into
// the conversation between agent turns, allowing users to steer the agent
// mid-run (similar to Claude Code behavior).
func (a *AgentRun) SetInterruptChannel(ch <-chan string) {
	a.interruptCh = ch
}

// Run executes the agent loop: builds the initial message list from conversation
// history, then iterates LLM calls and tool executions until a final response
// is produced or the turn limit is exhausted.
//
// If auto-continue is enabled and the agent is still using tools when the
// budget runs out, it will automatically start a continuation round.
func (a *AgentRun) Run(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, error) {
	content, _, err := a.RunWithUsage(ctx, systemPrompt, history, userMessage)
	return content, err
}

// RunWithUsage is like Run but also returns aggregated token usage from all LLM calls.
func (a *AgentRun) RunWithUsage(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, *LLMUsage, error) {
	// Build initial messages from history.
	messages := a.buildMessages(systemPrompt, history, userMessage)

	// Collect tool definitions from the executor.
	tools := a.executor.Tools()

	a.logger.Debug("agent run started",
		"history_entries", len(history),
		"tools_available", len(tools),
		"max_turns", a.maxTurns,
		"max_continuations", a.maxContinuations,
	)

	// If no tools are registered, do a single completion and return.
	if len(tools) == 0 {
		resp, err := a.doLLMCallWithOverflowRetry(ctx, messages, nil)
		if err != nil {
			return "", nil, err
		}
		var totalUsage LLMUsage
		a.accumulateUsage(&totalUsage, resp)
		return resp.Content, &totalUsage, nil
	}

	var totalUsage LLMUsage
	totalTurns := 0
	continuations := 0

	for {
		// Agent loop: LLM call → tool execution → repeat.
		exhausted := false

		for turn := 1; turn <= a.maxTurns; turn++ {
			totalTurns++
			turnStart := time.Now()

			a.logger.Debug("agent turn start",
				"turn", totalTurns,
				"continuation", continuations,
				"messages", len(messages),
			)

			// ── Interrupt injection ──
			// Check for follow-up user messages sent while the agent was working.
			if turn > 1 {
				if interrupts := a.drainInterrupts(); len(interrupts) > 0 {
					for _, interrupt := range interrupts {
						messages = append(messages, chatMessage{
							Role:    "user",
							Content: "[Follow-up from user while processing]\n" + interrupt,
						})
					}
					a.logger.Info("injected interrupt messages into agent loop",
						"count", len(interrupts),
						"turn", totalTurns,
					)
				}
			}

			// Inject reflection nudge periodically.
			if a.reflectionOn && turn > 1 && turn%reflectionInterval == 0 {
				messages = append(messages, chatMessage{
					Role: "user",
					Content: fmt.Sprintf(
						"[System: %d/%d turns used. Plan your remaining work efficiently.]",
						turn, a.maxTurns,
					),
				})
			}

			// Call LLM (with overflow retry and per-turn timeout inside).
			llmStart := time.Now()
			resp, err := a.doLLMCallWithOverflowRetry(ctx, messages, tools)
			llmDuration := time.Since(llmStart)
			if err != nil {
				return "", nil, fmt.Errorf("LLM call failed (turn %d, llm_ms=%d): %w",
					totalTurns, llmDuration.Milliseconds(), err)
			}
			a.accumulateUsage(&totalUsage, resp)

			a.logger.Info("LLM call complete",
				"turn", totalTurns,
				"llm_ms", llmDuration.Milliseconds(),
				"tool_calls", len(resp.ToolCalls),
				"prompt_tokens", resp.Usage.PromptTokens,
				"completion_tokens", resp.Usage.CompletionTokens,
			)

			// No tool calls → final response.
			if len(resp.ToolCalls) == 0 {
				a.logger.Info("agent completed",
					"total_turns", totalTurns,
					"continuations", continuations,
					"response_len", len(resp.Content),
					"turn_ms", time.Since(turnStart).Milliseconds(),
				)
				return resp.Content, &totalUsage, nil
			}

			// Append assistant message with tool calls to the conversation.
			messages = append(messages, chatMessage{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			// Execute all requested tool calls.
			toolStart := time.Now()
			toolNames := make([]string, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				toolNames[i] = tc.Function.Name
			}
			a.logger.Info("executing tool calls",
				"count", len(resp.ToolCalls),
				"tools", strings.Join(toolNames, ","),
				"turn", totalTurns,
			)

			results := a.executor.Execute(ctx, resp.ToolCalls)

			a.logger.Info("tool calls complete",
				"count", len(results),
				"tools_ms", time.Since(toolStart).Milliseconds(),
				"turn_ms", time.Since(turnStart).Milliseconds(),
			)

			// Append each tool result as a message.
			for _, result := range results {
				messages = append(messages, chatMessage{
					Role:       "tool",
					Content:    result.Content,
					ToolCallID: result.ToolCallID,
				})
			}
		}

		exhausted = true

		// ── Auto-continue logic ──
		// If the agent is still actively using tools and we have continuation budget,
		// inject a continuation prompt and keep going.
		if exhausted && continuations < a.maxContinuations {
			continuations++

			a.logger.Info("agent auto-continuing",
				"continuation", continuations,
				"max_continuations", a.maxContinuations,
				"total_turns_so_far", totalTurns,
			)

			messages = append(messages, chatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"[System: Turn budget reached (%d turns). Auto-continuing (continuation %d/%d). "+
						"You have %d more turns. Wrap up your current task or continue working.]",
					a.maxTurns, continuations, a.maxContinuations, a.maxTurns,
				),
			})

			continue // Start another round of turns.
		}

		// No more continuations. Request a final summary.
		a.logger.Warn("agent reached max turns and continuations, requesting summary",
			"total_turns", totalTurns,
			"continuations", continuations,
		)

		messages = append(messages, chatMessage{
			Role: "user",
			Content: "[System: You have used all available turns and continuations. " +
				"Please provide your best response with the information gathered so far.]",
		})

		resp, err := a.doLLMCallWithOverflowRetry(ctx, messages, nil)
		if err != nil {
			return "", nil, fmt.Errorf("final summary call failed: %w", err)
		}
		a.accumulateUsage(&totalUsage, resp)
		return resp.Content, &totalUsage, nil
	}
}

// drainInterrupts reads all pending messages from the interrupt channel
// without blocking. Returns nil if no messages are available.
func (a *AgentRun) drainInterrupts() []string {
	if a.interruptCh == nil {
		return nil
	}
	var msgs []string
	for {
		select {
		case msg, ok := <-a.interruptCh:
			if !ok {
				return msgs // Channel closed.
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// accumulateUsage adds resp.Usage into total.
func (a *AgentRun) accumulateUsage(total *LLMUsage, resp *LLMResponse) {
	if resp == nil {
		return
	}
	total.PromptTokens += resp.Usage.PromptTokens
	total.CompletionTokens += resp.Usage.CompletionTokens
	total.TotalTokens += resp.Usage.TotalTokens
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

// isContextOverflow checks if an error indicates context length exceeded.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "context_length_exceeded") ||
		strings.Contains(s, "maximum context length") ||
		(strings.Contains(s, "400") && strings.Contains(s, "tokens"))
}

// compactMessages removes older messages to reduce context size.
// Keeps: system prompt (first), last N messages.
func (a *AgentRun) compactMessages(messages []chatMessage, keepRecent int) []chatMessage {
	if len(messages) <= keepRecent+1 {
		return messages
	}

	var result []chatMessage
	// Always keep the first message if it's system.
	if len(messages) > 0 && messages[0].Role == "system" {
		result = append(result, messages[0])
		// Keep last keepRecent from the rest.
		rest := messages[1:]
		if len(rest) <= keepRecent {
			result = append(result, rest...)
			return result
		}
		rest = rest[len(rest)-keepRecent:]
		result = append(result, rest...)
	} else {
		// No system message; keep last keepRecent.
		if len(messages) <= keepRecent {
			return messages
		}
		result = make([]chatMessage, keepRecent)
		copy(result, messages[len(messages)-keepRecent:])
	}
	return result
}

// truncateToolResults shortens tool result messages that exceed maxLen.
func (a *AgentRun) truncateToolResults(messages []chatMessage, maxLen int) []chatMessage {
	if maxLen <= 0 {
		maxLen = 2000
	}
	truncSuffix := "... [truncated]"
	keepChars := 1000
	if keepChars+len(truncSuffix) > maxLen {
		keepChars = maxLen - len(truncSuffix)
	}

	result := make([]chatMessage, len(messages))
	for i, m := range messages {
		result[i] = m
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok && len(s) > maxLen {
				result[i].Content = s[:keepChars] + truncSuffix
			}
		}
	}
	return result
}

// doLLMCallWithOverflowRetry runs the LLM call and retries with compaction on context overflow.
func (a *AgentRun) doLLMCallWithOverflowRetry(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	keepRecent := 20 // Start by keeping last 20 messages.
	for attempt := 0; attempt < a.maxCompactionAttempts; attempt++ {
		turnCtx, cancel := context.WithTimeout(ctx, a.turnTimeout)
		var resp *LLMResponse
		var err error
		if a.streamCallback != nil {
			resp, err = a.llm.CompleteWithToolsStreamUsingModel(turnCtx, a.modelOverride, messages, tools, a.streamCallback)
		} else {
			resp, err = a.llm.CompleteWithFallbackUsingModel(turnCtx, a.modelOverride, messages, tools)
		}
		cancel()

		if err == nil {
			if a.usageRecorder != nil && resp.Usage.TotalTokens > 0 {
				a.usageRecorder(resp.ModelUsed, resp.Usage)
			}
			return resp, nil
		}

		if !isContextOverflow(err) {
			return nil, err
		}

		a.logger.Info("context overflow detected, compacting and retrying",
			"attempt", attempt+1,
			"max_attempts", a.maxCompactionAttempts,
			"messages_before", len(messages),
			"keep_recent", keepRecent,
		)

		// Compact: keep system + last keepRecent.
		messages = a.compactMessages(messages, keepRecent)
		// Truncate long tool results.
		messages = a.truncateToolResults(messages, 2000)

		// Next attempt: keep fewer messages.
		keepRecent -= 5
		if keepRecent < 6 {
			keepRecent = 6
		}
	}

	return nil, fmt.Errorf("context overflow: compacted %d times but still exceeded context limit", a.maxCompactionAttempts)
}
