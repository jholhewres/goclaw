// Package copilot – events.go implements an in-memory pub/sub event bus
// for agent lifecycle events. Replaces channel-based buffering with fan-out
// to multiple listeners, each receiving events via direct function call.
//
// Event streams:
//   - "lifecycle": run_start, run_end, abort
//   - "assistant": delta (text tokens), thinking_start, thinking_delta, thinking_end
//   - "tool": tool_use, tool_result
//   - "error": agent errors, LLM errors
package copilot

import (
	"sync"
	"sync/atomic"
	"time"
)

// AgentEvent represents a single typed event from an agent run.
type AgentEvent struct {
	RunID     string    `json:"run_id"`
	SessionID string    `json:"session_id"`
	Seq       int64     `json:"seq"`
	Stream    string    `json:"stream"` // lifecycle, assistant, tool, error
	Type      string    `json:"type"`   // delta, tool_use, tool_result, done, error, run_start, etc.
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// EventListener is a callback that receives agent events.
type EventListener func(event AgentEvent)

// EventBus is a thread-safe pub/sub hub for agent events.
// Subscribers receive events synchronously during Emit — callers should
// keep listener logic fast or dispatch to goroutines internally.
type EventBus struct {
	listeners sync.Map // listenerID (uint64) → EventListener
	nextID    atomic.Uint64
	seqByRun  sync.Map // runID → *atomic.Int64
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe registers a listener and returns an unsubscribe function.
// The listener is called synchronously for every emitted event.
func (eb *EventBus) Subscribe(fn EventListener) func() {
	id := eb.nextID.Add(1)
	eb.listeners.Store(id, fn)
	return func() { eb.listeners.Delete(id) }
}

// SubscribeRun registers a listener that only receives events for a specific run.
// Returns an unsubscribe function.
func (eb *EventBus) SubscribeRun(runID string, fn EventListener) func() {
	return eb.Subscribe(func(event AgentEvent) {
		if event.RunID == runID {
			fn(event)
		}
	})
}

// Emit sends an event to all registered listeners. The event's Seq field is
// auto-assigned using an atomic counter scoped to the run ID.
func (eb *EventBus) Emit(event AgentEvent) {
	// Auto-assign sequence number.
	seq := eb.getRunSeq(event.RunID)
	event.Seq = seq.Add(1)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Fan-out to all listeners (synchronous).
	eb.listeners.Range(func(_, value any) bool {
		if fn, ok := value.(EventListener); ok {
			fn(event)
		}
		return true
	})
}

// EmitDelta is a convenience method for emitting text delta events.
func (eb *EventBus) EmitDelta(runID, sessionID, content string) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "assistant",
		Type:      "delta",
		Data:      map[string]string{"content": content},
	})
}

// EmitThinkingStart emits a thinking_start event signalling the beginning of
// an extended thinking / reasoning block.
func (eb *EventBus) EmitThinkingStart(runID, sessionID string) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "assistant",
		Type:      "thinking_start",
		Data:      nil,
	})
}

// EmitThinkingDelta emits a thinking_delta event with a partial reasoning token.
func (eb *EventBus) EmitThinkingDelta(runID, sessionID, content string) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "assistant",
		Type:      "thinking_delta",
		Data:      map[string]string{"content": content},
	})
}

// EmitThinkingEnd emits a thinking_end event. Callers are responsible for
// deduplication — this method emits unconditionally. The recommended pattern
// is to track a thinkingActive bool and only call EmitThinkingEnd once.
func (eb *EventBus) EmitThinkingEnd(runID, sessionID string) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "assistant",
		Type:      "thinking_end",
		Data:      nil,
	})
}

// EmitToolUse emits a tool invocation event.
func (eb *EventBus) EmitToolUse(runID, sessionID, toolName string, input any) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "tool",
		Type:      "tool_use",
		Data:      map[string]any{"tool": toolName, "input": input},
	})
}

// EmitToolResult emits a tool result event.
func (eb *EventBus) EmitToolResult(runID, sessionID, toolName, output string, isError bool) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "tool",
		Type:      "tool_result",
		Data: map[string]any{
			"tool":     toolName,
			"output":   output,
			"is_error": isError,
		},
	})
}

// EmitDone emits a run completion event.
func (eb *EventBus) EmitDone(runID, sessionID string, usage any) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "lifecycle",
		Type:      "done",
		Data:      map[string]any{"usage": usage},
	})
}

// EmitError emits an error event.
func (eb *EventBus) EmitError(runID, sessionID, message string) {
	eb.Emit(AgentEvent{
		RunID:     runID,
		SessionID: sessionID,
		Stream:    "error",
		Type:      "error",
		Data:      map[string]string{"message": message},
	})
}

// CleanupRun removes the sequence counter for a completed run.
func (eb *EventBus) CleanupRun(runID string) {
	eb.seqByRun.Delete(runID)
}

// getRunSeq returns the sequence counter for a run, creating one if needed.
func (eb *EventBus) getRunSeq(runID string) *atomic.Int64 {
	if v, ok := eb.seqByRun.Load(runID); ok {
		return v.(*atomic.Int64)
	}
	seq := &atomic.Int64{}
	actual, _ := eb.seqByRun.LoadOrStore(runID, seq)
	return actual.(*atomic.Int64)
}
