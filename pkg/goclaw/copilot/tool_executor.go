// Package copilot – tool_executor.go manages a registry of callable tools
// and dispatches tool calls from the LLM to the appropriate handlers.
// Tools can be registered from skills, system built-ins, or plugins.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// toolNameSanitizer replaces any character not in [a-zA-Z0-9_-] with "_".
var toolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ctxKeySessionID is the context key for passing session ID through
// the context chain, ensuring goroutine-safe isolation.
type ctxKeySessionID struct{}

// ctxKeyDeliveryTarget is the context key for passing the delivery target
// (channel + chatID) separately from the opaque session ID.
type ctxKeyDeliveryTarget struct{}

// DeliveryTarget holds the channel and chatID for message delivery.
type DeliveryTarget struct {
	Channel string
	ChatID  string
}

// ContextWithSession returns a new context carrying the given session ID.
func ContextWithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ctxKeySessionID{}, sessionID)
}

// ContextWithDelivery returns a new context carrying the delivery target.
// This is used by tools like cron_add to know where to deliver scheduled messages.
func ContextWithDelivery(ctx context.Context, channel, chatID string) context.Context {
	return context.WithValue(ctx, ctxKeyDeliveryTarget{}, DeliveryTarget{
		Channel: channel,
		ChatID:  chatID,
	})
}

// SessionIDFromContext extracts the session ID from a context.
// Returns empty string if not set.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySessionID{}).(string); ok {
		return v
	}
	return ""
}

// DeliveryTargetFromContext extracts the delivery target from a context.
// Returns empty DeliveryTarget if not set.
func DeliveryTargetFromContext(ctx context.Context) DeliveryTarget {
	if v, ok := ctx.Value(ctxKeyDeliveryTarget{}).(DeliveryTarget); ok {
		return v
	}
	return DeliveryTarget{}
}

// ProgressSender sends intermediate progress messages to the user during
// long-running tool execution (e.g. claude-code). Called by tools that want
// to give real-time feedback without waiting for the full result.
type ProgressSender func(ctx context.Context, message string)

// ctxKeyProgress is a string-based context key for ProgressSender.
// Using a well-known string ensures cross-package matching (skills package
// uses the same key to extract the sender without importing copilot).
const ctxKeyProgress = "goclaw.progress_sender"

// ContextWithProgressSender returns a context carrying a ProgressSender callback.
func ContextWithProgressSender(ctx context.Context, fn ProgressSender) context.Context {
	return context.WithValue(ctx, ctxKeyProgress, fn)
}

// ProgressSenderFromContext extracts the ProgressSender from context.
// Returns nil if not set.
func ProgressSenderFromContext(ctx context.Context) ProgressSender {
	if fn, ok := ctx.Value(ctxKeyProgress).(ProgressSender); ok {
		return fn
	}
	return nil
}

const (
	// DefaultToolTimeout is the maximum time a single tool execution can take.
	DefaultToolTimeout = 30 * time.Second
)

// ToolHandlerFunc is the signature for tool execution handlers.
// Receives parsed arguments and returns the result or an error.
type ToolHandlerFunc func(ctx context.Context, args map[string]any) (any, error)

// registeredTool bundles a tool definition with its handler.
type registeredTool struct {
	Definition ToolDefinition
	Handler    ToolHandlerFunc
}

// ToolResult holds the output of a single tool execution.
type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	Error      error
}

// sequentialTools are tools that must not run in parallel (shared state).
var sequentialTools = map[string]bool{
	"bash": true, "write_file": true, "edit_file": true,
	"ssh": true, "scp": true, "exec": true, "set_env": true,
}

// ToolExecutor manages tool registration and dispatches tool calls.
type ToolExecutor struct {
	tools   map[string]*registeredTool
	timeout time.Duration
	logger  *slog.Logger
	guard   *ToolGuard
	mu      sync.RWMutex

	// toolDefsCache caches the slice of ToolDefinitions so we don't rebuild
	// it on every Tools() call. Invalidated when a new tool is registered.
	toolDefsCache []ToolDefinition
	toolDefsDirty bool

	// parallel enables concurrent execution of independent tools.
	parallel    bool
	maxParallel int

	// callerLevel is the access level of the current caller.
	// Set per-request via SetCallerContext before Execute.
	callerLevel AccessLevel
	callerJID   string

	// sessionID is set per-request for approval matching (channel:chatID).
	sessionID string

	// confirmationRequester is called when a tool requires user approval.
	// If nil, tools requiring confirmation are denied.
	confirmationRequester func(sessionID, callerJID, toolName string, args map[string]any) (approved bool, err error)
}

// NewToolExecutor creates a new empty tool executor.
func NewToolExecutor(logger *slog.Logger) *ToolExecutor {
	return &ToolExecutor{
		tools:        make(map[string]*registeredTool),
		timeout:      DefaultToolTimeout,
		logger:       logger.With("component", "tool_executor"),
		callerLevel:  AccessOwner, // Default to owner for CLI usage.
		parallel:     true,
		maxParallel:  5,
	}
}

// SetGuard configures the security guard for tool execution.
func (e *ToolExecutor) SetGuard(guard *ToolGuard) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.guard = guard
}

// Guard returns the configured ToolGuard (may be nil).
func (e *ToolExecutor) Guard() *ToolGuard {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.guard
}

// UpdateGuardConfig updates the tool guard config (for hot-reload).
func (e *ToolExecutor) UpdateGuardConfig(cfg ToolGuardConfig) {
	e.mu.Lock()
	guard := e.guard
	e.mu.Unlock()
	if guard != nil {
		guard.UpdateConfig(cfg)
	}
}

// SetCallerContext sets the access level and JID for the current caller.
// Must be called before Execute() in the message handling flow.
func (e *ToolExecutor) SetCallerContext(level AccessLevel, jid string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callerLevel = level
	e.callerJID = jid
}

// SetSessionContext sets the session ID for approval matching (channel:chatID).
// Must be set before Execute() when using approval flow.
func (e *ToolExecutor) SetSessionContext(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionID = sessionID
}

// SessionContext returns the current session ID (format: "channel:chatID").
func (e *ToolExecutor) SessionContext() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sessionID
}

// SetConfirmationRequester sets the callback for tools requiring user approval.
// When a tool is in RequireConfirmation list, this callback is invoked.
func (e *ToolExecutor) SetConfirmationRequester(fn func(sessionID, callerJID, toolName string, args map[string]any) (bool, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.confirmationRequester = fn
}

// Configure applies ToolExecutorConfig (parallel, max_parallel).
func (e *ToolExecutor) Configure(cfg ToolExecutorConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.parallel = cfg.Parallel
	e.maxParallel = cfg.MaxParallel
	if e.maxParallel <= 0 {
		e.maxParallel = 5
	}
}

// Register adds a tool with its definition and handler.
// If a tool with the same name already exists, it is overwritten.
func (e *ToolExecutor) Register(def ToolDefinition, handler ToolHandlerFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := def.Function.Name
	e.tools[name] = &registeredTool{
		Definition: def,
		Handler:    handler,
	}
	e.toolDefsDirty = true // Invalidate cache.

	e.logger.Debug("tool registered", "name", name)
}

// RegisterSkillTools registers all tools exposed by a skill.
// Tool names are prefixed with the skill name to avoid collisions.
// Names are sanitized to match OpenAI's pattern: ^[a-zA-Z0-9_-]+$
func (e *ToolExecutor) RegisterSkillTools(skill skills.Skill) {
	meta := skill.Metadata()
	for _, tool := range skill.Tools() {
		// Build the full tool name: skill_name_tool_name
		// Use underscore separator (dots are rejected by OpenAI).
		fullName := sanitizeToolName(meta.Name + "_" + tool.Name)

		def := SkillToolToDefinition(fullName, tool)
		handler := makeSkillToolHandler(skill, tool)

		e.Register(def, handler)
	}
}

// sanitizeToolName ensures a tool name matches OpenAI's required pattern
// ^[a-zA-Z0-9_-]+$ by replacing invalid characters with underscores.
func sanitizeToolName(name string) string {
	// Replace dots and other invalid chars with underscores.
	name = toolNameSanitizer.ReplaceAllString(name, "_")
	// Collapse multiple underscores.
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	// Trim leading/trailing underscores.
	name = strings.Trim(name, "_")
	return name
}

// Tools returns all registered tool definitions for the LLM.
// Uses a cached slice that is rebuilt only when tools are added/removed.
func (e *ToolExecutor) Tools() []ToolDefinition {
	e.mu.RLock()
	if !e.toolDefsDirty && e.toolDefsCache != nil {
		result := e.toolDefsCache
		e.mu.RUnlock()
		return result
	}
	e.mu.RUnlock()

	// Upgrade to write lock to rebuild cache.
	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-check after acquiring write lock.
	if !e.toolDefsDirty && e.toolDefsCache != nil {
		return e.toolDefsCache
	}

	defs := make([]ToolDefinition, 0, len(e.tools))
	for _, t := range e.tools {
		defs = append(defs, t.Definition)
	}
	e.toolDefsCache = defs
	e.toolDefsDirty = false
	return defs
}

// ToolNames returns the names of all registered tools.
func (e *ToolExecutor) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		names = append(names, name)
	}
	return names
}

// HasTool checks if a tool is registered by name.
func (e *ToolExecutor) HasTool(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.tools[name]
	return ok
}

// Execute dispatches a batch of tool calls to their registered handlers.
// Each tool is executed with a per-tool timeout.
// When Parallel is true and no sequential tools are in the batch, runs concurrently.
// Returns results in the same order as the input calls.
func (e *ToolExecutor) Execute(ctx context.Context, calls []ToolCall) []ToolResult {
	e.mu.RLock()
	parallel := e.parallel
	maxParallel := e.maxParallel
	e.mu.RUnlock()

	if !parallel || len(calls) <= 1 || e.hasSequentialTool(calls) {
		return e.executeSequential(ctx, calls)
	}
	return e.executeParallel(ctx, calls, maxParallel)
}

func (e *ToolExecutor) hasSequentialTool(calls []ToolCall) bool {
	for _, c := range calls {
		if sequentialTools[c.Function.Name] {
			return true
		}
	}
	return false
}

func (e *ToolExecutor) executeSequential(ctx context.Context, calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, call := range calls {
		results[i] = e.executeSingle(ctx, call)
	}
	return results
}

func (e *ToolExecutor) executeParallel(ctx context.Context, calls []ToolCall, maxParallel int) []ToolResult {
	results := make([]ToolResult, len(calls))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = e.executeSingle(ctx, tc)
		}(i, call)
	}

	wg.Wait()
	return results
}

// executeSingle runs a single tool call and returns the result.
// If a ToolGuard is configured, it checks permissions before executing.
func (e *ToolExecutor) executeSingle(ctx context.Context, call ToolCall) ToolResult {
	name := call.Function.Name
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       name,
	}

	e.mu.RLock()
	tool, ok := e.tools[name]
	guard := e.guard
	callerLevel := e.callerLevel
	callerJID := e.callerJID
	e.mu.RUnlock()

	if !ok {
		result.Content = fmt.Sprintf("Error: unknown tool %q", name)
		result.Error = fmt.Errorf("unknown tool: %s", name)
		e.logger.Warn("unknown tool called", "name", name)
		return result
	}

	// Parse arguments from JSON string.
	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		result.Content = fmt.Sprintf("Error parsing arguments: %v", err)
		result.Error = err
		e.logger.Warn("tool argument parse error", "name", name, "error", err)
		return result
	}

	// Security check: verify the caller has permission.
	var check ToolCheckResult
	if guard != nil {
		check = guard.Check(name, callerLevel, args)
		if !check.Allowed {
			result.Content = fmt.Sprintf("Access denied: %s", check.Reason)
			result.Error = fmt.Errorf("access denied: %s", check.Reason)
			e.logger.Warn("tool blocked by guard",
				"name", name,
				"caller", callerJID,
				"level", callerLevel,
				"reason", check.Reason,
			)
			guard.AuditLog(name, callerJID, callerLevel, args, false, check.Reason)
			return result
		}
	}

	// Confirmation flow: if tool requires approval, return "approval-pending"
	// immediately (non-blocking, OpenClaw-style) and run the tool in the
	// background once approved. The result is sent to the user via ProgressSender.
	if check.RequiresConfirmation {
		e.mu.RLock()
		req := e.confirmationRequester
		sessionID := e.sessionID
		callerJID := e.callerJID
		e.mu.RUnlock()

		if req == nil {
			result.Content = "Tool requires confirmation but no approval handler is configured."
			result.Error = fmt.Errorf("confirmation required but no handler")
			return result
		}

		desc := formatApprovalSummary(name, args)

		// Return immediately — the agent loop continues without blocking.
		result.Content = fmt.Sprintf(
			"⚠️ Approval required: %s\nStatus: pending. Waiting for user to approve. "+
				"The command will execute in the background once approved and the result will be sent to the user.",
			desc,
		)

		// Fire-and-forget: handle approval + execution asynchronously.
		progressSend := ProgressSenderFromContext(ctx)
		go func() {
			approved, err := req(sessionID, callerJID, name, args)
			if err != nil {
				e.logger.Warn("async approval error", "tool", name, "error", err)
				if progressSend != nil {
					progressSend(context.Background(),
						fmt.Sprintf("⏱️ Approval for `%s` timed out. Command was not executed.", desc))
				}
				return
			}
			if !approved {
				e.logger.Info("async tool denied", "tool", name, "session", sessionID)
				if guard != nil {
					guard.AuditLog(name, callerJID, callerLevel, args, false, "DENIED_BY_USER")
				}
				if progressSend != nil {
					progressSend(context.Background(),
						fmt.Sprintf("❌ `%s` was denied by user.", desc))
				}
				return
			}

			// Approved — execute the tool now.
			e.logger.Info("async approval granted, executing", "tool", name)
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			output, execErr := tool.Handler(bgCtx, args)
			if execErr != nil {
				e.logger.Warn("async tool execution failed", "tool", name, "error", execErr)
				if guard != nil {
					guard.AuditLog(name, callerJID, callerLevel, args, true, "ERROR: "+execErr.Error())
				}
				if progressSend != nil {
					progressSend(context.Background(),
						fmt.Sprintf("⚠️ `%s` failed: %v", desc, execErr))
				}
				return
			}

			outputStr := formatToolOutput(output)
			e.logger.Info("async tool executed", "tool", name, "output_len", len(outputStr))
			if guard != nil {
				guard.AuditLog(name, callerJID, callerLevel, args, true, outputStr)
			}

			// Send result to the user via their channel.
			if progressSend != nil {
				msg := fmt.Sprintf("✅ `%s` completed:\n\n%s", desc, outputStr)
				if len(msg) > 4000 {
					msg = msg[:4000] + "\n... (truncated)"
				}
				progressSend(context.Background(), msg)
			}
		}()

		return result
	}

	// Execute with timeout.
	timeout := e.timeout
	// Give bash/ssh/scp longer timeouts.
	if name == "bash" || name == "ssh" || name == "scp" || name == "exec" {
		timeout = 5 * time.Minute
	}
	// Claude Code manages its own internal timeout (default 15min);
	// give the executor wrapper enough headroom.
	if name == "claude-code_execute" {
		timeout = 20 * time.Minute
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)

	// Propagate ProgressSender to the tool context so long-running tools
	// can send intermediate feedback to the user.
	if ps := ProgressSenderFromContext(ctx); ps != nil {
		execCtx = ContextWithProgressSender(execCtx, ps)
	}
	defer cancel()

	e.logger.Debug("executing tool", "name", name, "args_keys", mapKeys(args))

	start := time.Now()
	output, err := tool.Handler(execCtx, args)
	duration := time.Since(start)

	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.Error = err
		e.logger.Warn("tool execution failed",
			"name", name,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		if guard != nil {
			guard.AuditLog(name, callerJID, callerLevel, args, true, "ERROR: "+err.Error())
		}
		return result
	}

	// Serialize output to string.
	result.Content = formatToolOutput(output)

	e.logger.Info("tool executed",
		"name", name,
		"duration_ms", duration.Milliseconds(),
		"output_len", len(result.Content),
	)

	// Audit log successful execution.
	if guard != nil {
		guard.AuditLog(name, callerJID, callerLevel, args, true, result.Content)
	}

	return result
}

// ---------- Conversion Helpers ----------

// SkillToolToDefinition converts a skills.Tool into an OpenAI ToolDefinition.
func SkillToolToDefinition(name string, tool skills.Tool) ToolDefinition {
	props := make(map[string]any)
	required := make([]string, 0)

	for _, p := range tool.Parameters {
		prop := map[string]any{
			"type":        p.Type,
			"description": p.Description,
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		props[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	schemaJSON, _ := json.Marshal(schema)

	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        name,
			Description: tool.Description,
			Parameters:  schemaJSON,
		},
	}
}

// MakeToolDefinition creates a ToolDefinition from name, description, and a
// parameter schema map (matching JSON Schema format).
// The name is automatically sanitized to match OpenAI's pattern.
func MakeToolDefinition(name, description string, params map[string]any) ToolDefinition {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if params != nil {
		schema = params
	}

	schemaJSON, _ := json.Marshal(schema)

	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        sanitizeToolName(name),
			Description: description,
			Parameters:  schemaJSON,
		},
	}
}

// makeSkillToolHandler creates a ToolHandlerFunc that delegates to a skill's tool handler.
func makeSkillToolHandler(skill skills.Skill, tool skills.Tool) ToolHandlerFunc {
	if tool.Handler != nil {
		// Skill tool has an explicit handler — use it directly.
		return ToolHandlerFunc(tool.Handler)
	}

	// Fallback: call the skill's Execute method with the input arg.
	return func(ctx context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		if input == "" {
			// Try to serialize all args as the input.
			b, _ := json.Marshal(args)
			input = string(b)
		}
		return skill.Execute(ctx, input)
	}
}

// ---------- Internal Helpers ----------

// parseToolArgs parses JSON-encoded tool arguments into a map.
func parseToolArgs(raw string) (map[string]any, error) {
	if raw == "" || raw == "{}" {
		return map[string]any{}, nil
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("invalid JSON arguments: %w", err)
	}
	return args, nil
}

// formatToolOutput converts tool output to a string suitable for the LLM.
func formatToolOutput(output any) string {
	if output == nil {
		return "OK"
	}

	switch v := output.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case error:
		return fmt.Sprintf("Error: %v", v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// formatApprovalSummary builds a short summary of the tool + args for approval messages.
func formatApprovalSummary(toolName string, args map[string]any) string {
	switch toolName {
	case "bash", "exec":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			if len(cmd) > 80 {
				return fmt.Sprintf("%s: %s...", toolName, cmd[:80])
			}
			return toolName + ": " + cmd
		}
	case "write_file", "edit_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return toolName + " " + path
		}
	case "ssh":
		if host, ok := args["host"].(string); ok {
			return "ssh " + host
		}
	case "scp":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		return fmt.Sprintf("scp %s → %s", src, dst)
	}
	return toolName
}

// mapKeys returns the keys of a map for logging.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
