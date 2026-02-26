// Package copilot – hooks.go implements a lifecycle hook system. Hooks allow
// external code to observe and optionally modify agent behavior at well-defined
// points in the lifecycle.
//
// Hook events include:
//
//	SessionStart      — A new session is created or restored.
//	SessionEnd        — A session is about to be pruned/deleted.
//	UserPromptSubmit  — User message received, before processing.
//	PreToolUse        — Before a tool is called (can block/modify).
//	PostToolUse       — After a tool returns (observe/log).
//	AgentStart        — Agent loop is about to begin.
//	AgentStop         — Agent loop finished (normal or error).
//	SubagentStart     — A subagent has been spawned.
//	SubagentStop      — A subagent has finished.
//	PreCompact        — Before session compaction.
//	PostCompact       — After session compaction.
//	MemorySave        — A memory was saved.
//	MemoryRecall      — Memories were recalled for prompt.
//	Notification      — An outbound notification/message is being sent.
//	Heartbeat         — Periodic heartbeat tick.
//	Error             — An unrecoverable error occurred.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// HookEvent identifies the lifecycle point at which a hook fires.
type HookEvent string

const (
	HookSessionStart      HookEvent = "session_start"
	HookSessionEnd        HookEvent = "session_end"
	HookUserPromptSubmit  HookEvent = "user_prompt_submit"
	HookPreToolUse        HookEvent = "pre_tool_use"
	HookPostToolUse       HookEvent = "post_tool_use"
	HookAgentStart        HookEvent = "agent_start"
	HookAgentStop         HookEvent = "agent_stop"
	HookSubagentStart     HookEvent = "subagent_start"
	HookSubagentStop      HookEvent = "subagent_stop"
	HookPreCompact        HookEvent = "pre_compact"
	HookPostCompact       HookEvent = "post_compact"
	HookMemorySave        HookEvent = "memory_save"
	HookMemoryRecall      HookEvent = "memory_recall"
	HookNotification      HookEvent = "notification"
	HookHeartbeat         HookEvent = "heartbeat"
	HookError             HookEvent = "error"
	HookUserJoin          HookEvent = "user_join"
	HookUserLeave         HookEvent = "user_leave"
	HookChannelConnect    HookEvent = "channel_connect"
	HookChannelDisconnect HookEvent = "channel_disconnect"

	// Advanced hooks inspired by OpenClaw
	HookBeforeModelResolve HookEvent = "before_model_resolve" // Override provider/model selection
	HookBeforePromptBuild  HookEvent = "before_prompt_build"  // Inject/modify system prompt
	HookLLMInput           HookEvent = "llm_input"            // Modify prompt before sending to LLM
	HookLLMOutput          HookEvent = "llm_output"          // Modify LLM response
	HookToolResultPersist  HookEvent = "tool_result_persist" // Transform tool result before persisting
)

// AllHookEvents lists every supported hook event for discovery/documentation.
var AllHookEvents = []HookEvent{
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

// HooksConfig holds all hook configuration.
type HooksConfig struct {
	// Enabled turns the hooks system on/off.
	Enabled bool `yaml:"enabled"`

	// Webhooks is the list of external webhook configurations.
	Webhooks []WebhookConfig `yaml:"webhooks"`

	// Handlers is the list of internal hook handlers.
	Handlers []HandlerConfig `yaml:"handlers"`
}

// HandlerConfig configures an internal hook handler.
type HandlerConfig struct {
	// Event is the hook event to listen to.
	Event string `yaml:"event"`

	// Action is the action to take (notify_admins, send_message).
	Action string `yaml:"action"`

	// Template is a Go template for the action output.
	Template string `yaml:"template"`

	// Enabled controls whether this handler is active.
	Enabled bool `yaml:"enabled"`
}

// HookPayload carries contextual data for a hook invocation.
// Fields are populated based on the event type; unused fields are zero-valued.
type HookPayload struct {
	// Event is the hook event type.
	Event HookEvent

	// SessionID is the session this event relates to (if applicable).
	SessionID string

	// Channel is the originating channel (if applicable).
	Channel string

	// ToolName is the tool being called (PreToolUse/PostToolUse).
	ToolName string

	// ToolArgs are the tool arguments (PreToolUse only).
	ToolArgs map[string]any

	// ToolResult is the tool output (PostToolUse only).
	ToolResult string

	// Message is a human-readable description or the user message content.
	Message string

	// Error is set for HookError events.
	Error error

	// Extra holds arbitrary key-value data for extensibility.
	Extra map[string]any

	// Advanced hook fields
	// Model is the LLM model being used (HookBeforeModelResolve).
	Model string

	// SystemPrompt is the system prompt being built (HookBeforePromptBuild).
	SystemPrompt string

	// LLMInput is the input being sent to the LLM (HookLLMInput).
	LLMInput string

	// LLMOutput is the response from the LLM (HookLLMOutput).
	LLMOutput string

	// ToolCallID identifies the tool call for persistence hooks.
	ToolCallID string
}

// HookAction is the result returned by a hook handler.
type HookAction struct {
	// Block prevents the operation from proceeding (PreToolUse, UserPromptSubmit).
	Block bool

	// Reason explains why the operation was blocked.
	Reason string

	// ModifiedArgs replaces ToolArgs if non-nil (PreToolUse only).
	ModifiedArgs map[string]any

	// ModifiedMessage replaces the user message if non-empty (UserPromptSubmit).
	ModifiedMessage string
}

// HookHandler processes a hook event and returns an action.
// Handlers should be fast and non-blocking. For async work, spawn a goroutine.
type HookHandler func(ctx context.Context, payload HookPayload) HookAction

// RegisteredHook associates a handler with metadata.
type RegisteredHook struct {
	// Name identifies this hook for logging.
	Name string

	// Description provides a human-readable summary of this hook.
	Description string

	// Source indicates where the hook came from (e.g. "system", "plugin:github", "skill:monitor").
	Source string

	// Events lists which events this hook subscribes to.
	Events []HookEvent

	// Priority controls execution order (lower = earlier). Default: 100.
	Priority int

	// Enabled controls whether this hook is active. Default: true.
	Enabled bool

	// Handler is the callback function.
	Handler HookHandler
}

// HookSummary is a serializable representation of a registered hook (no handler).
type HookSummary struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Source      string      `json:"source"`
	Events      []HookEvent `json:"events"`
	Priority    int         `json:"priority"`
	Enabled     bool        `json:"enabled"`
}

// HookManager manages lifecycle hook registration and dispatch.
type HookManager struct {
	mu     sync.RWMutex
	hooks  map[HookEvent][]*RegisteredHook
	logger *slog.Logger
}

// NewHookManager creates a new hook manager.
func NewHookManager(logger *slog.Logger) *HookManager {
	if logger == nil {
		logger = slog.Default()
	}
	hooks := make(map[HookEvent][]*RegisteredHook, len(AllHookEvents))
	for _, ev := range AllHookEvents {
		hooks[ev] = nil
	}
	return &HookManager{
		hooks:  hooks,
		logger: logger.With("component", "hooks"),
	}
}

// Register adds a hook handler for the specified events.
func (hm *HookManager) Register(hook *RegisteredHook) error {
	if hook == nil || hook.Handler == nil {
		return fmt.Errorf("hook and handler must not be nil")
	}
	if len(hook.Events) == 0 {
		return fmt.Errorf("hook must subscribe to at least one event")
	}
	if hook.Priority == 0 {
		hook.Priority = 100
	}
	// Default: enabled.
	if !hook.Enabled {
		hook.Enabled = true
	}

	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Deduplicate events to prevent double-registration.
	seen := make(map[HookEvent]bool, len(hook.Events))
	for _, ev := range hook.Events {
		if seen[ev] {
			continue
		}
		seen[ev] = true
	}

	for ev := range seen {
		list := hm.hooks[ev]
		// Insert in priority order.
		inserted := false
		for i, existing := range list {
			if hook.Priority < existing.Priority {
				list = append(list[:i], append([]*RegisteredHook{hook}, list[i:]...)...)
				inserted = true
				break
			}
		}
		if !inserted {
			list = append(list, hook)
		}
		hm.hooks[ev] = list
	}

	hm.logger.Info("hook registered",
		"name", hook.Name,
		"events", hook.Events,
		"priority", hook.Priority,
	)
	return nil
}

// Dispatch fires all hooks for the given event and returns the combined action.
// For blocking events (PreToolUse, UserPromptSubmit), the first Block=true stops
// dispatch and returns immediately. For non-blocking events, all hooks run.
func (hm *HookManager) Dispatch(ctx context.Context, payload HookPayload) HookAction {
	hm.mu.RLock()
	hooks := hm.hooks[payload.Event]
	hm.mu.RUnlock()

	if len(hooks) == 0 {
		return HookAction{}
	}

	var combined HookAction

	for _, hook := range hooks {
		if !hook.Enabled {
			continue
		}

		action, panicked := func(h *RegisteredHook) (a HookAction, didPanic bool) {
			defer func() {
				if r := recover(); r != nil {
					hm.logger.Error("hook panicked in dispatch",
						"hook", h.Name, "event", payload.Event, "panic", r)
					didPanic = true
				}
			}()
			return h.Handler(ctx, payload), false
		}(hook)

		if panicked {
			continue
		}

		// Blocking events: first block wins.
		if action.Block {
			hm.logger.Info("hook blocked operation",
				"hook", hook.Name,
				"event", payload.Event,
				"reason", action.Reason,
			)
			return action
		}

		// Merge modifications: last non-nil wins.
		if action.ModifiedArgs != nil {
			combined.ModifiedArgs = action.ModifiedArgs
			payload.ToolArgs = action.ModifiedArgs // Chain to next hook.
		}
		if action.ModifiedMessage != "" {
			combined.ModifiedMessage = action.ModifiedMessage
			payload.Message = action.ModifiedMessage
		}
	}

	return combined
}

// DispatchAsync fires all hooks for the event without waiting for them.
// Use for non-critical observe-only events (PostToolUse, Notification, etc.).
func (hm *HookManager) DispatchAsync(payload HookPayload) {
	hm.mu.RLock()
	hooks := hm.hooks[payload.Event]
	hm.mu.RUnlock()

	if len(hooks) == 0 {
		return
	}

	go func() {
		ctx := context.Background()
		for _, hook := range hooks {
			if !hook.Enabled {
				continue
			}
			func(h *RegisteredHook) {
				defer func() {
					if r := recover(); r != nil {
						hm.logger.Error("hook panicked in async dispatch",
							"hook", h.Name, "event", payload.Event, "panic", r)
					}
				}()
				h.Handler(ctx, payload)
			}(hook)
		}
	}()
}

// HasHooks returns true if any hooks are registered for the given event.
func (hm *HookManager) HasHooks(event HookEvent) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return len(hm.hooks[event]) > 0
}

// HookCount returns the total number of registered hooks.
func (hm *HookManager) HookCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	total := 0
	for _, list := range hm.hooks {
		total += len(list)
	}
	return total
}

// ListHooks returns all registered hooks grouped by event.
func (hm *HookManager) ListHooks() map[HookEvent][]string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	result := make(map[HookEvent][]string)
	for ev, hooks := range hm.hooks {
		if len(hooks) > 0 {
			names := make([]string, len(hooks))
			for i, h := range hooks {
				names[i] = h.Name
			}
			result[ev] = names
		}
	}
	return result
}

// ListDetailed returns a deduplicated list of all registered hooks with metadata.
func (hm *HookManager) ListDetailed() []HookSummary {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	seen := make(map[string]bool)
	var result []HookSummary
	for _, hooks := range hm.hooks {
		for _, h := range hooks {
			if seen[h.Name] {
				continue
			}
			seen[h.Name] = true
			result = append(result, HookSummary{
				Name:        h.Name,
				Description: h.Description,
				Source:      h.Source,
				Events:      h.Events,
				Priority:    h.Priority,
				Enabled:     h.Enabled,
			})
		}
	}
	return result
}

// SetEnabled enables or disables a hook by name.
func (hm *HookManager) SetEnabled(name string, enabled bool) bool {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	found := false
	for _, hooks := range hm.hooks {
		for _, h := range hooks {
			if h.Name == name {
				h.Enabled = enabled
				found = true
			}
		}
	}
	if found {
		hm.logger.Info("hook toggled", "name", name, "enabled", enabled)
	}
	return found
}

// Unregister removes all registrations for a hook by name.
func (hm *HookManager) Unregister(name string) bool {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	found := false
	for ev, hooks := range hm.hooks {
		filtered := hooks[:0]
		for _, h := range hooks {
			if h.Name == name {
				found = true
				continue
			}
			filtered = append(filtered, h)
		}
		hm.hooks[ev] = filtered
	}
	if found {
		hm.logger.Info("hook unregistered", "name", name)
	}
	return found
}

// HookEventDescription returns a human-readable description for a hook event.
func HookEventDescription(ev HookEvent) string {
	switch ev {
	case HookSessionStart:
		return "Sessão criada ou restaurada"
	case HookSessionEnd:
		return "Sessão encerrada ou removida"
	case HookUserPromptSubmit:
		return "Mensagem do usuário recebida (antes do processamento)"
	case HookPreToolUse:
		return "Antes de chamar uma ferramenta (pode bloquear/modificar)"
	case HookPostToolUse:
		return "Após o retorno de uma ferramenta"
	case HookAgentStart:
		return "Loop do agente iniciando"
	case HookAgentStop:
		return "Loop do agente finalizado"
	case HookSubagentStart:
		return "Subagente iniciado"
	case HookSubagentStop:
		return "Subagente finalizado"
	case HookPreCompact:
		return "Antes da compactação de sessão"
	case HookPostCompact:
		return "Após a compactação de sessão"
	case HookMemorySave:
		return "Memória salva"
	case HookMemoryRecall:
		return "Memórias recuperadas para o prompt"
	case HookNotification:
		return "Notificação/mensagem sendo enviada"
	case HookHeartbeat:
		return "Tick periódico do heartbeat"
	case HookError:
		return "Erro irrecuperável ocorreu"
	case HookUserJoin:
		return "Usuário aprovado/adicionado"
	case HookUserLeave:
		return "Usuário removido/bloqueado"
	case HookChannelConnect:
		return "Canal conectado"
	case HookChannelDisconnect:
		return "Canal desconectado"
	// Advanced hooks
	case HookBeforeModelResolve:
		return "Antes de resolver o modelo LLM (permite override)"
	case HookBeforePromptBuild:
		return "Antes de construir o prompt do sistema (permite injetar contexto)"
	case HookLLMInput:
		return "Input sendo enviado ao LLM (permite modificar)"
	case HookLLMOutput:
		return "Output recebido do LLM (permite modificar)"
	case HookToolResultPersist:
		return "Antes de persistir resultado da ferramenta (permite transformar)"
	default:
		return string(ev)
	}
}
