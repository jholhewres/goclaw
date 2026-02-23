// Package copilot – agent.go implements the agentic loop that orchestrates
// LLM calls with tool execution. The agent iterates: call LLM → if tool_calls
// → execute tools → append results → call LLM again, until the LLM produces
// a final text response with no tool calls.
//
// Architecture:
//   - No fixed max turns — the loop runs until the LLM stops calling tools.
//   - Single run timeout (default: 600s = 10min) controls the whole run.
//   - Per-LLM-call safety timeout (5min) prevents individual hung requests.
//   - Reflection nudge every 15 turns for budget awareness.
//   - Auto-compaction on context overflow (up to 3 attempts).
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// DefaultRunTimeout is the maximum duration for an entire agent run.
	// Set to 20 minutes to accommodate coding tasks that invoke Claude Code CLI
	// (which itself can take 5-15 minutes for complex projects).
	// This is the PRIMARY timeout — no per-turn limit.
	DefaultRunTimeout = 1200 * time.Second

	// DefaultLLMCallTimeout is the safety-net timeout for a single LLM API call.
	// This only prevents hung HTTP connections — it should be generous enough
	// that even large contexts complete. 5 minutes covers worst-case scenarios.
	DefaultLLMCallTimeout = 5 * time.Minute

	// reflectionInterval is how often (in turns) the agent receives a budget nudge.
	// Reduced from 15 to 5 to catch stuck patterns earlier.
	reflectionInterval = 5

	// DefaultMaxCompactionAttempts is how many times to retry after context overflow compaction.
	DefaultMaxCompactionAttempts = 3
)

// AgentConfig holds configurable agent loop parameters.
type AgentConfig struct {
	// RunTimeoutSeconds is the max seconds for the entire agent run (default: 600).
	// One timer for the whole run, not per-turn.
	RunTimeoutSeconds int `yaml:"run_timeout_seconds"`

	// LLMCallTimeoutSeconds is the safety-net timeout per individual LLM call
	// (default: 300). Only catches hung connections — not the primary timeout.
	LLMCallTimeoutSeconds int `yaml:"llm_call_timeout_seconds"`

	// MaxTurns is a soft safety limit on LLM round-trips (default: 0 = unlimited).
	// When > 0, the agent will request a summary after this many turns.
	MaxTurns int `yaml:"max_turns"`

	// MaxContinuations is how many auto-continue rounds are allowed when
	// MaxTurns is hit and the agent is still using tools.
	// Only relevant when MaxTurns > 0. Default: 2.
	MaxContinuations int `yaml:"max_continuations"`

	// ReflectionEnabled enables periodic budget awareness nudges (default: true).
	ReflectionEnabled bool `yaml:"reflection_enabled"`

	// MaxCompactionAttempts is how many times to retry after context overflow (default: 3).
	MaxCompactionAttempts int `yaml:"max_compaction_attempts"`

	// ToolLoop configures tool loop detection thresholds.
	ToolLoop ToolLoopConfig `yaml:"tool_loop"`
}

// DefaultAgentConfig returns sensible defaults for agent autonomy.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		RunTimeoutSeconds:     int(DefaultRunTimeout / time.Second),
		LLMCallTimeoutSeconds: int(DefaultLLMCallTimeout / time.Second),
		MaxTurns:              0, // Unlimited
		MaxContinuations:      2,
		ReflectionEnabled:     true,
		MaxCompactionAttempts: DefaultMaxCompactionAttempts,
	}
}

// AgentRun encapsulates a single agent execution with its dependencies.
type AgentRun struct {
	llm                   *LLMClient
	executor              *ToolExecutor
	runTimeout            time.Duration // Total run timeout (default: 600s)
	llmCallTimeout        time.Duration // Per-LLM-call safety timeout (default: 5min)
	maxTurns              int           // 0 = unlimited
	reflectionOn          bool
	maxCompactionAttempts int
	streamCallback        StreamCallback
	modelOverride         string                             // When set, use this model instead of default.
	usageRecorder         func(model string, usage LLMUsage) // Called after each successful LLM response.

	// interruptCh receives follow-up user messages that should be injected into
	// the active agent loop. Between turns, the agent drains this channel and
	// appends the messages to the conversation before the next LLM call.
	// This enables Claude Code-style live message injection.
	interruptCh <-chan string

	// onBeforeToolExec is called right before tool execution starts.
	// Used to flush any buffered stream text so the user sees the LLM's
	// intermediate reasoning before tools run.
	onBeforeToolExec func()

	// onToolResult is called after each tool execution completes.
	// Used to auto-send media (e.g. generated images) to the channel.
	onToolResult func(name string, result ToolResult)

	// loopDetector tracks tool call history and detects repetitive patterns.
	loopDetector *ToolLoopDetector

	logger *slog.Logger
}

// NewAgentRun creates a new agent runner.
func NewAgentRun(llm *LLMClient, executor *ToolExecutor, logger *slog.Logger) *AgentRun {
	return &AgentRun{
		llm:                   llm,
		executor:              executor,
		runTimeout:            DefaultRunTimeout,
		llmCallTimeout:        DefaultLLMCallTimeout,
		maxTurns:              0, // Unlimited
		reflectionOn:          true,
		maxCompactionAttempts: DefaultMaxCompactionAttempts,
		logger:                logger.With("component", "agent"),
	}
}

// NewAgentRunWithConfig creates a new agent runner with explicit configuration.
func NewAgentRunWithConfig(llm *LLMClient, executor *ToolExecutor, cfg AgentConfig, logger *slog.Logger) *AgentRun {
	ar := NewAgentRun(llm, executor, logger)
	if cfg.RunTimeoutSeconds > 0 {
		ar.runTimeout = time.Duration(cfg.RunTimeoutSeconds) * time.Second
	}
	if cfg.LLMCallTimeoutSeconds > 0 {
		ar.llmCallTimeout = time.Duration(cfg.LLMCallTimeoutSeconds) * time.Second
	}
	if cfg.MaxTurns >= 0 {
		ar.maxTurns = cfg.MaxTurns // 0 = unlimited
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

// SetOnBeforeToolExec sets a callback fired right before tool execution starts
// in the agent loop. Used by the block streamer to flush buffered text so the
// user sees intermediate reasoning before tools run.
func (a *AgentRun) SetOnBeforeToolExec(fn func()) {
	a.onBeforeToolExec = fn
}

// SetOnToolResult sets a callback fired after each tool execution completes.
// Used to auto-send media (e.g. generated images) to the channel.
func (a *AgentRun) SetOnToolResult(fn func(name string, result ToolResult)) {
	a.onToolResult = fn
}

// SetLoopDetector sets the tool loop detector for this run.
func (a *AgentRun) SetLoopDetector(d *ToolLoopDetector) {
	a.loopDetector = d
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
//
// Architecture:
//   - The loop runs until the LLM produces a response with no tool calls.
//   - A single run-level timeout controls the entire execution (default: 600s).
//   - Individual LLM calls have a safety-net timeout (5min) to catch hung connections.
//   - No fixed turn limit — the agent keeps going as long as it has tools to call.
func (a *AgentRun) RunWithUsage(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, *LLMUsage, error) {
	// ── Run-level timeout (single timer for the whole run) ──
	runCtx, runCancel := context.WithTimeout(ctx, a.runTimeout)
	defer runCancel()

	runStart := time.Now()

	// Build initial messages from history.
	messages := a.buildMessages(systemPrompt, history, userMessage)

	// Collect tool definitions from the executor, filtered by profile if present.
	allTools := a.executor.Tools()
	var tools []ToolDefinition

	profile := ToolProfileFromContext(runCtx)
	if profile != nil {
		// Filter tools by profile
		allToolNames := a.executor.ToolNames()
		checker := NewProfileChecker(profile.Allow, profile.Deny, allToolNames)
		for _, tool := range allTools {
			if allowed, _ := checker.Check(tool.Function.Name); allowed {
				tools = append(tools, tool)
			}
		}
		a.logger.Debug("tools filtered by profile",
			"profile", profile.Name,
			"total_tools", len(allTools),
			"allowed_tools", len(tools),
		)
	} else {
		tools = allTools
	}

	// Limit tools to 128 for OpenAI API compatibility
	const maxTools = 128
	if len(tools) > maxTools {
		a.logger.Warn("too many tools, truncating to max",
			"total", len(tools),
			"max", maxTools,
		)
		tools = tools[:maxTools]
	}

	a.logger.Debug("agent run started",
		"history_entries", len(history),
		"tools_available", len(tools),
		"run_timeout_s", int(a.runTimeout.Seconds()),
		"max_turns", a.maxTurns,
	)

	// If no tools are registered, do a single completion and return.
	if len(tools) == 0 {
		resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, nil)
		if err != nil {
			return "", nil, err
		}
		var totalUsage LLMUsage
		a.accumulateUsage(&totalUsage, resp)
		return resp.Content, &totalUsage, nil
	}

	var totalUsage LLMUsage
	totalTurns := 0

	// Progress cooldown: avoid flooding the user with tool progress messages.
	// Short 3s cooldown for faster feedback while avoiding message spam.
	const progressCooldown = 3 * time.Second
	var lastProgressAt time.Time

	// ── Main agent loop ──
	// Loop until: (1) LLM produces no tool calls, (2) run timeout fires, or
	// (3) optional soft turn limit is hit. No fixed turn limit by default.
	for {
		totalTurns++
		turnStart := time.Now()

		a.logger.Debug("agent turn start",
			"turn", totalTurns,
			"messages", len(messages),
			"run_elapsed_s", int(time.Since(runStart).Seconds()),
		)

		// ── Soft turn limit (optional, 0 = disabled) ──
		if a.maxTurns > 0 && totalTurns > a.maxTurns {
			a.logger.Warn("agent reached soft turn limit, requesting summary",
				"total_turns", totalTurns,
				"max_turns", a.maxTurns,
			)
			messages = append(messages, chatMessage{
				Role: "user",
				Content: "[System: You have used many turns. " +
					"Please provide your best response with the information gathered so far.]",
			})
			resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, nil)
			if err != nil {
				return "", nil, fmt.Errorf("final summary call failed: %w", err)
			}
			a.accumulateUsage(&totalUsage, resp)
			return resp.Content, &totalUsage, nil
		}

		// ── Run timeout check ──
		if runCtx.Err() != nil {
			return "", &totalUsage, fmt.Errorf("agent run timeout (%s) after %d turns: %w",
				a.runTimeout, totalTurns, runCtx.Err())
		}

		// ── Interrupt injection ──
		// Check for follow-up user messages sent while the agent was working.
		if totalTurns > 1 {
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

		// ── Proactive context pruning ──
		// Trim old tool results to prevent context bloat:
		//   - Soft trim: tool results older than 5 turns → truncated to 500 chars
		//   - Hard trim: tool results older than 10 turns → removed entirely
		// This reduces the need for full compaction and keeps the context lean.
		if totalTurns > 5 {
			messages = a.pruneOldToolResults(messages, totalTurns)
		}

		// Inject reflection nudge periodically so the agent is aware of duration.
		// More aggressive messaging to catch stuck patterns early.
		if a.reflectionOn && totalTurns > 1 && totalTurns%reflectionInterval == 0 {
			elapsed := time.Since(runStart).Seconds()
			remaining := a.runTimeout.Seconds() - elapsed
			messages = append(messages, chatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"[System: Turn %d checkpoint (%.0fs elapsed, ~%.0fs remaining). "+
						"If you're stuck or repeating the same approach, STOP and investigate the root cause. "+
						"Don't repeat failed approaches — think before acting.]",
					totalTurns, elapsed, remaining,
				),
			})
		}

		// ── Call LLM ──
		llmStart := time.Now()
		resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, tools)
		llmDuration := time.Since(llmStart)
		if err != nil {
			// If the parent/run context was cancelled, propagate immediately.
			if runCtx.Err() != nil {
				// Distinguish user abort from run timeout.
				if ctx.Err() != nil {
					return "", &totalUsage, fmt.Errorf("agent cancelled by user: %w", ctx.Err())
				}
				return "", &totalUsage, fmt.Errorf("agent run timeout (%s) at turn %d: %w",
					a.runTimeout, totalTurns, runCtx.Err())
			}

			// Timeout or transient error on a later turn: try compacting
			// the context and retrying once before giving up.
			errStr := err.Error()
			isTimeout := strings.Contains(errStr, "deadline exceeded") || strings.Contains(errStr, "context canceled")
			if isTimeout && totalTurns > 2 && len(messages) > 10 {
				a.logger.Warn("LLM call timed out, compacting context and retrying",
					"turn", totalTurns,
					"messages_before", len(messages),
					"llm_ms", llmDuration.Milliseconds(),
				)
				messages = a.compactMessages(messages, 12)
				messages = a.truncateToolResults(messages, 1500)

				// Retry the LLM call with compacted context.
				llmStart = time.Now()
				resp, err = a.doLLMCallWithOverflowRetry(runCtx, messages, tools)
				llmDuration = time.Since(llmStart)
			}

			if err != nil {
				return "", &totalUsage, fmt.Errorf("LLM call failed (turn %d, llm_ms=%d): %w",
					totalTurns, llmDuration.Milliseconds(), err)
			}
		}
		a.accumulateUsage(&totalUsage, resp)

		a.logger.Info("LLM call complete",
			"turn", totalTurns,
			"llm_ms", llmDuration.Milliseconds(),
			"tool_calls", len(resp.ToolCalls),
			"prompt_tokens", resp.Usage.PromptTokens,
			"completion_tokens", resp.Usage.CompletionTokens,
		)

		// ── No tool calls → final response ──
		if len(resp.ToolCalls) == 0 {
			a.logger.Info("agent completed",
				"total_turns", totalTurns,
				"response_len", len(resp.Content),
				"run_elapsed_ms", time.Since(runStart).Milliseconds(),
			)
			return resp.Content, &totalUsage, nil
		}

		// Append assistant message with tool calls to the conversation.
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// ── Tool Loop Detection ──
		// Record tool calls and check for repetitive patterns before execution.
		// Warnings/criticals are deferred until AFTER tool results to maintain
		// valid message ordering (assistant→tool→user, not assistant→user→tool).
		var loopWarning string
		if a.loopDetector != nil {
			for _, tc := range resp.ToolCalls {
				args, _ := parseToolArgs(tc.Function.Arguments)
				result := a.loopDetector.RecordAndCheck(tc.Function.Name, args)

				switch result.Severity {
				case LoopBreaker:
					a.logger.Error("tool loop circuit breaker",
						"tool", tc.Function.Name, "streak", result.Streak, "pattern", result.Pattern)
					return result.Message, &totalUsage, nil

				case LoopCritical, LoopWarning:
					loopWarning = result.Message
				}
			}
		}

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

		// Flush any buffered stream text before tools start — ensures the user
		// sees the LLM's intermediate reasoning/thoughts immediately.
		if a.onBeforeToolExec != nil {
			a.onBeforeToolExec()
		}

		// Send progress to the user so they see what the agent is doing
		// while tools execute (especially for long-running tools).
		// When a stream callback (BlockStreamer) is active, it already delivers
		// the LLM's text progressively — sending resp.Content via ProgressSender
		// would duplicate messages. Only send tool descriptions as progress.
		if ps := ProgressSenderFromContext(runCtx); ps != nil {
			now := time.Now()
			if now.Sub(lastProgressAt) >= progressCooldown {
				var progressMsg string

				if a.streamCallback != nil {
					// BlockStreamer is active and already sent resp.Content
					// to the channel — only send a tool description as progress.
					progressMsg = formatToolProgressMessage(resp.ToolCalls)
				} else if resp.Content != "" && len(resp.Content) < 1000 {
					// No streamer — send the LLM's own text as progress.
					progressMsg = resp.Content
				} else if resp.Content != "" {
					progressMsg = resp.Content[:500] + "..."
				} else {
					progressMsg = formatToolProgressMessage(resp.ToolCalls)
				}

				if progressMsg != "" {
					ps(runCtx, progressMsg)
					lastProgressAt = now
				}
			}
		}

		results := a.executor.Execute(runCtx, resp.ToolCalls)

		a.logger.Info("tool calls complete",
			"count", len(results),
			"tools_ms", time.Since(toolStart).Milliseconds(),
			"turn_ms", time.Since(turnStart).Milliseconds(),
		)

		// Append each tool result as a message.
		// Classify recoverable errors: the model should retry silently without
		// the user seeing transient failures.
		for _, result := range results {
			content := result.Content
			if result.Error != nil && isRecoverableToolError(content) {
				a.logger.Debug("recoverable tool error (model should retry)",
					"tool", result.Name,
					"error_preview", truncateStr(content, 80),
				)
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: result.ToolCallID,
			})

			// Track tool output for progress-aware loop detection.
			if a.loopDetector != nil {
				a.loopDetector.RecordToolOutcome(content)
			}

			// Notify hook (e.g. auto-send media for generate_image).
			if a.onToolResult != nil && result.Error == nil {
				a.onToolResult(result.Name, result)
			}
		}

		// Inject deferred loop warning AFTER tool results (valid message order:
		// assistant→tool→user). This ensures providers that validate message
		// sequences don't reject the request.
		if loopWarning != "" {
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "[System] " + loopWarning,
			})
		}
	}
}

// formatToolProgressMessage creates a clean, concise, user-facing message about
// what the agent is doing. Designed for chat apps (WhatsApp, Telegram).
// Unlike step-by-step output, this shows a single summarized line.
// Format: emoji + label + optional detail.
func formatToolProgressMessage(toolCalls []ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	// For a single tool call, show a concise description.
	if len(toolCalls) == 1 {
		name := toolCalls[0].Function.Name
		args, _ := parseToolArgs(toolCalls[0].Function.Arguments)
		return describeToolAction(name, args)
	}

	// For multiple parallel tool calls, summarize them.
	// Show the most interesting one (longest description) and count the rest.
	var best string
	count := 0
	for _, tc := range toolCalls {
		name := tc.Function.Name
		args, _ := parseToolArgs(tc.Function.Arguments)
		desc := describeToolAction(name, args)
		if desc != "" {
			count++
			if len(desc) > len(best) {
				best = desc
			}
		}
	}

	if count == 0 {
		return ""
	}
	if count == 1 {
		return best
	}
	return fmt.Sprintf("%s (+%d)", best, count-1)
}

// describeToolAction returns a human-friendly, emoji-prefixed description
// of a tool call. Empty string means "skip this tool in progress output".
func describeToolAction(name string, args map[string]any) string {
	switch name {
	// ── Shell / commands ──
	case "bash", "exec":
		cmd, _ := args["command"].(string)
		if cmd == "" {
			return "💻 Executando comando..."
		}
		if len(cmd) > 60 {
			cmd = cmd[:60] + "..."
		}
		return "💻 `" + cmd + "`"

	// ── File operations ──
	case "read_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "📖 Lendo " + shortPath(p)
		}
		return "📖 Lendo arquivo..."

	case "write_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✍️ Escrevendo " + shortPath(p)
		}
		return "✍️ Escrevendo arquivo..."

	case "edit_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✏️ Editando " + shortPath(p)
		}
		return "✏️ Editando arquivo..."

	case "list_files", "glob_files":
		p, _ := args["path"].(string)
		if p == "" {
			p, _ = args["pattern"].(string)
		}
		if p != "" {
			return "📂 Listando " + shortPath(p)
		}
		return "📂 Listando arquivos..."

	case "search_files":
		q, _ := args["query"].(string)
		if q == "" {
			q, _ = args["pattern"].(string)
		}
		if q != "" {
			return "🔎 Buscando: " + q
		}
		return "🔎 Buscando nos arquivos..."

	// ── Web ──
	case "web_search", "brave-search_execute", "brave-search_run_search":
		q, _ := args["query"].(string)
		if q != "" {
			if len(q) > 60 {
				q = q[:60] + "..."
			}
			return "🔍 Pesquisando: " + q
		}
		return "🔍 Pesquisando na web..."

	case "web_fetch", "web-fetch_fetch_url":
		u, _ := args["url"].(string)
		if u != "" {
			if len(u) > 55 {
				u = u[:55] + "..."
			}
			return "🌐 Acessando " + u
		}
		return "🌐 Acessando página..."

	// ── Memory ──
	case "memory":
		action, _ := args["action"].(string)
		switch action {
		case "save":
			return "💾 Saving to memory..."
		case "search":
			q, _ := args["query"].(string)
			if q != "" {
				return "🧠 Recalling: " + q
			}
			return "🧠 Searching memory..."
		case "list", "index":
			return "🧠 Organizing memories..."
		default:
			return "🧠 Memory..."
		}

	// ── Remote ──
	case "ssh":
		host, _ := args["host"].(string)
		cmd, _ := args["command"].(string)
		if host != "" && cmd != "" {
			if len(cmd) > 40 {
				cmd = cmd[:40] + "..."
			}
			return "🔗 " + host + ": `" + cmd + "`"
		}
		if host != "" {
			return "🔗 Conectando em " + host + "..."
		}
		return "🔗 Conectando via SSH..."

	case "scp":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		if src != "" && dst != "" {
			return "📤 Transferindo " + shortPath(src) + " → " + shortPath(dst)
		}
		return "📤 Transferindo arquivo..."

	// ── Coding ──
	case "claude-code_execute":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 55 {
				p = p[:55] + "..."
			}
			return "🤖 Codificando: " + p
		}
		return "🤖 Executando Claude Code..."
	case "claude-code_check":
		return "🤖 Verificando Claude Code..."

	// ── Images ──
	case "describe_image":
		return "👁️ Analisando imagem..."
	case "image-gen_generate_image":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 50 {
				p = p[:50] + "..."
			}
			return "🎨 Gerando imagem: " + p
		}
		return "🎨 Gerando imagem..."

	// ── Audio ──
	case "transcribe_audio":
		return "🎤 Transcrevendo áudio..."

	// ── Scheduler ──
	case "cron_add":
		return "⏰ Criando agendamento..."
	case "cron_list":
		return "⏰ Listando agendamentos..."
	case "cron_remove":
		return "⏰ Removendo agendamento..."

	// ── Vault ──
	case "vault_save":
		return "🔐 Salvando no cofre..."
	case "vault_get":
		return "🔐 Buscando no cofre..."
	case "vault_list":
		return "🔐 Listando cofre..."

	// ── Skills ──
	case "install_skill":
		s, _ := args["name"].(string)
		if s != "" {
			return "📦 Instalando skill: " + s
		}
		return "📦 Instalando skill..."
	case "list_skills", "search_skills":
		return "📋 Listando skills..."

	// ── Subagents ──
	case "spawn_subagent":
		label, _ := args["label"].(string)
		if label == "" {
			label, _ = args["task"].(string)
			if len(label) > 40 {
				label = label[:40] + "..."
			}
		}
		if label != "" {
			return "🧵 Iniciando subagente: " + label
		}
		return "🧵 Iniciando subagente..."
	case "list_subagents":
		return "🧵 Verificando subagentes..."
	case "wait_subagent":
		return "⏳ Aguardando subagente..."
	case "stop_subagent":
		return "🛑 Parando subagente..."

	// ── Project Manager ──
	case "project-manager_activate":
		p, _ := args["name"].(string)
		if p != "" {
			return "📁 Ativando projeto: " + p
		}
		return "📁 Ativando projeto..."
	case "project-manager_list":
		return "📁 Listando projetos..."
	case "project-manager_scan", "project-manager_tree":
		return "📁 Escaneando projeto..."
	case "project-manager_register":
		return "📁 Registrando projeto..."

	// ── Calculator / DateTime ──
	case "calculator_calculate":
		return "" // silent — too trivial
	case "datetime_current_time":
		return "" // silent

	default:
		// For skill tools (prefixed), make it cleaner.
		if strings.Contains(name, "_execute") {
			skillName := strings.TrimSuffix(name, "_execute")
			skillName = strings.ReplaceAll(skillName, "_", " ")
			skillName = strings.ReplaceAll(skillName, "-", " ")
			return "⚡ Executando " + skillName + "..."
		}
		if strings.Contains(name, "_run_") {
			parts := strings.SplitN(name, "_run_", 2)
			skillName := strings.ReplaceAll(parts[0], "-", " ")
			action := strings.ReplaceAll(parts[1], "_", " ")
			return "⚡ " + skillName + ": " + action + "..."
		}
		return "⚙️ " + strings.ReplaceAll(name, "_", " ") + "..."
	}
}

// shortPath returns the last 2 segments of a path for display.
// "/home/user/projects/app/src/index.ts" → "src/index.ts"
func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

// isRecoverableToolError checks if a tool error is likely transient or due to
// incorrect parameters, so the model should retry without surfacing it to the user.
// Classifies errors that the model can recover from by retrying or adjusting parameters.
func isRecoverableToolError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	patterns := []string{
		"required",       // "path is required", "prompt is required"
		"missing",        // "missing parameter"
		"not found",      // "file not found" (model can fix path)
		"invalid",        // "invalid argument"
		"parsing",        // "error parsing arguments"
		"no such file",   // fs errors
		"does not exist", // resource not found
		"permission denied",
		"timed out", // transient timeout
		"connection refused",
		"empty", // "command is empty"
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// truncateStr truncates a string to n characters for logging.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

// pruneOldToolResults implements proactive context trimming.
// Tool results are tagged with their turn number. Older results are progressively
// truncated or removed to keep the context lean without waiting for overflow.
func (a *AgentRun) pruneOldToolResults(messages []chatMessage, currentTurn int) []chatMessage {
	const (
		softTrimAge   = 5   // Turns before soft trim (truncate to 500 chars)
		hardTrimAge   = 10  // Turns before hard trim (remove entirely)
		softTrimChars = 500 // Max chars after soft trim
	)

	// Estimate the "turn" of each message based on position. Tool messages
	// between two assistant messages belong to the same turn.
	msgCount := len(messages)
	if msgCount < 10 {
		return messages
	}

	// Count tool result messages from the end to estimate age.
	toolResultCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			toolResultCount++
		}
	}

	if toolResultCount < softTrimAge {
		return messages // Not enough tool results to prune.
	}

	// Walk through messages, trimming old tool results.
	result := make([]chatMessage, 0, len(messages))
	toolIdx := 0
	for _, m := range messages {
		if m.Role == "tool" {
			age := toolResultCount - toolIdx
			toolIdx++

			if age > hardTrimAge {
				// Hard trim: skip this tool result entirely.
				result = append(result, chatMessage{
					Role:       m.Role,
					Content:    "[tool result removed — too old]",
					ToolCallID: m.ToolCallID,
				})
				continue
			}

			if age > softTrimAge {
				// Soft trim: truncate to 500 chars.
				if s, ok := m.Content.(string); ok && len(s) > softTrimChars {
					m.Content = s[:softTrimChars] + "... [truncated — old result]"
				}
			}
		}
		result = append(result, m)
	}

	return result
}

// doLLMCallWithOverflowRetry runs the LLM call and retries with compaction on context overflow.
// The per-call timeout is a safety net (llmCallTimeout, default 5min) — the primary timeout
// is the run-level context passed in ctx.
//
// Compaction strategy:
//  1. First attempt: truncate oversized tool results (>4K chars).
//  2. Second attempt: compact messages (keep last N) + truncate tool results harder.
//  3. Third attempt: aggressive compaction (keep fewer messages).
func (a *AgentRun) doLLMCallWithOverflowRetry(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	toolResultTruncated := false
	keepRecent := 20

	for attempt := 0; attempt < a.maxCompactionAttempts; attempt++ {
		// Use the shorter of: run context deadline or llmCallTimeout safety net.
		callCtx, cancel := context.WithTimeout(ctx, a.llmCallTimeout)
		var resp *LLMResponse
		var err error
		if a.streamCallback != nil {
			resp, err = a.llm.CompleteWithToolsStreamUsingModel(callCtx, a.modelOverride, messages, tools, a.streamCallback)
		} else {
			resp, err = a.llm.CompleteWithFallbackUsingModel(callCtx, a.modelOverride, messages, tools)
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

		a.logger.Info("context overflow detected",
			"attempt", attempt+1,
			"max_attempts", a.maxCompactionAttempts,
			"messages_before", len(messages),
		)

		// ── Compaction strategy ──
		// Step 1: Try truncating oversized tool results first (cheap operation).
		if !toolResultTruncated {
			if hasOversizedToolResults(messages, 4000) {
				a.logger.Info("truncating oversized tool results before compaction")
				messages = a.truncateToolResults(messages, 4000)
				toolResultTruncated = true
				continue // Retry without compacting messages.
			}
		}

		// Step 2+3: Compact messages (keep system + last N).
		a.logger.Info("compacting messages",
			"keep_recent", keepRecent,
			"messages_before", len(messages),
		)
		messages = a.compactMessages(messages, keepRecent)
		messages = a.truncateToolResults(messages, 2000)

		// Next attempt: keep fewer messages.
		keepRecent -= 5
		if keepRecent < 6 {
			keepRecent = 6
		}
	}

	return nil, fmt.Errorf("context overflow: compacted %d times but still exceeded context limit", a.maxCompactionAttempts)
}

// hasOversizedToolResults checks if any tool result message exceeds maxLen.
func hasOversizedToolResults(messages []chatMessage, maxLen int) bool {
	for _, m := range messages {
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok && len(s) > maxLen {
				return true
			}
		}
	}
	return false
}
