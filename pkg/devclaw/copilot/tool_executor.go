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

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
)

// toolNameSanitizer replaces any character not in [a-zA-Z0-9_-] with "_".
var toolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ctxKeySessionID is the context key for passing session ID through
// the context chain, ensuring goroutine-safe isolation.
type ctxKeySessionID struct{}

// ctxKeyDeliveryTarget is the context key for passing the delivery target
// (channel + chatID) separately from the opaque session ID.
type ctxKeyDeliveryTarget struct{}

// ctxKeyCallerLevel is the context key for passing caller access level
// per-request, avoiding the global shared state race condition.
type ctxKeyCallerLevel struct{}

// ctxKeyCallerJID is the context key for passing caller JID per-request.
type ctxKeyCallerJID struct{}

// ctxKeyToolProfile is the context key for passing the active tool profile.
type ctxKeyToolProfile struct{}

// ctxKeyVaultReader is the context key for passing the vault reader.
type ctxKeyVaultReader struct{}

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

// ContextWithCaller returns a new context carrying the caller's access level and JID.
// This replaces the global SetCallerContext/SetSessionContext pattern, making
// tool security checks goroutine-safe (context per request).
func ContextWithCaller(ctx context.Context, level AccessLevel, jid string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyCallerLevel{}, level)
	ctx = context.WithValue(ctx, ctxKeyCallerJID{}, jid)
	return ctx
}

// CallerLevelFromContext extracts the caller access level from context.
// Falls back to AccessNone if not set.
func CallerLevelFromContext(ctx context.Context) AccessLevel {
	if v, ok := ctx.Value(ctxKeyCallerLevel{}).(AccessLevel); ok {
		return v
	}
	return AccessNone
}

// CallerJIDFromContext extracts the caller JID from context.
func CallerJIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCallerJID{}).(string); ok {
		return v
	}
	return ""
}

// ContextWithToolProfile returns a new context carrying a tool profile.
// The profile is used for CheckWithProfile to apply allow/deny lists.
func ContextWithToolProfile(ctx context.Context, profile *ToolProfile) context.Context {
	return context.WithValue(ctx, ctxKeyToolProfile{}, profile)
}

// ToolProfileFromContext extracts the tool profile from context.
// Returns nil if no profile is set.
func ToolProfileFromContext(ctx context.Context) *ToolProfile {
	if v, ok := ctx.Value(ctxKeyToolProfile{}).(*ToolProfile); ok {
		return v
	}
	return nil
}

// ContextWithVaultReader returns a new context carrying a vault reader.
func ContextWithVaultReader(ctx context.Context, vr skills.VaultReader) context.Context {
	return context.WithValue(ctx, ctxKeyVaultReader{}, vr)
}

// VaultReaderFromContext extracts the vault reader from context.
// Returns nil if not set.
func VaultReaderFromContext(ctx context.Context) skills.VaultReader {
	if v, ok := ctx.Value(ctxKeyVaultReader{}).(skills.VaultReader); ok {
		return v
	}
	return nil
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
const ctxKeyProgress = "devclaw.progress_sender"

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
	Content    string // Main content (used for LLM context when ForLLM is empty)
	Error      error

	// Extended fields for dual output (PicoClaw-inspired)
	ForLLM   string // Technical content for LLM reasoning (if empty, Content is used)
	ForUser  string // Friendly message to show user immediately
	IsAsync  bool   // Tool is running in background, result comes later
	IsSilent bool   // Don't notify user about this result
}

// DualToolResult creates a ToolResult with separate content for LLM and user.
// This is the recommended way to return results that have both technical details
// and user-friendly messages.
func DualToolResult(forLLM, forUser string) *ToolResult {
	return &ToolResult{
		Content: forLLM, // Content is always set for backwards compatibility
		ForLLM:  forLLM,
		ForUser: forUser,
	}
}

// SilentResult creates a ToolResult that doesn't notify the user.
// Useful for background operations or when the result should only be
// used for LLM reasoning.
func SilentResult(content string) *ToolResult {
	return &ToolResult{
		Content:  content,
		ForLLM:   content,
		IsSilent: true,
	}
}

// AsyncResult creates a ToolResult indicating the tool is running in background.
// The actual result will be delivered via callback or follow-up message.
func AsyncResult(message string) *ToolResult {
	return &ToolResult{
		Content:  message,
		ForLLM:   message,
		ForUser:  message,
		IsAsync:  true,
	}
}

// ErrorResult creates a ToolResult from an error.
func ErrorResult(err error) *ToolResult {
	errMsg := err.Error()
	return &ToolResult{
		Content:  errMsg,
		ForLLM:   errMsg,
		ForUser:  "An error occurred. Please try again.",
		Error:    err,
	}
}

// GetForLLM returns the content to use for LLM context.
// Returns ForLLM if set, otherwise Content.
func (r *ToolResult) GetForLLM() string {
	if r.ForLLM != "" {
		return r.ForLLM
	}
	return r.Content
}

// GetForUser returns the content to show the user.
// Returns ForUser if set, otherwise GetForLLM().
func (r *ToolResult) GetForUser() string {
	if r.ForUser != "" {
		return r.ForUser
	}
	return r.GetForLLM()
}

// ─────────────────────────────────────────────────────────────────────────────
// Contextual Tool Helpers
// ─────────────────────────────────────────────────────────────────────────────

// ContextualTool is an interface for tools that need delivery context.
// Tools can implement this interface to receive channel/chatID context.
// The executor checks for this interface and calls SetDeliveryTarget before
// executing the tool handler.
//
// Note: In DevClaw, handlers are functions (not objects), so this interface
// is typically implemented by a wrapper struct. For simple cases, tools can
// use GetDeliveryTarget(ctx) directly to extract context from the context.Context.
//
// Example with wrapper:
//
//	type contextualHandler struct {
//		fn      ToolHandlerFunc
//		channel string
//		chatID  string
//	}
//
//	func (h *contextualHandler) SetDeliveryTarget(channel, chatID string) {
//		h.channel = channel
//		h.chatID = chatID
//	}
//
//	func (h *contextualHandler) Call(ctx context.Context, args map[string]any) (any, error) {
//		// Use h.channel and h.chatID
//		return h.fn(ctx, args)
//	}
type ContextualTool interface {
	SetDeliveryTarget(channel, chatID string)
}

// GetDeliveryTarget is a convenience function that extracts the delivery target
// from the context. Tools should use this to get channel/chatID context.
//
// Example:
//
//	func myToolHandler(ctx context.Context, args map[string]any) (any, error) {
//		channel, chatID := GetDeliveryTarget(ctx)
//		if channel == "" {
//			return nil, fmt.Errorf("no channel context")
//		}
//		// Use channel and chatID...
//	}
func GetDeliveryTarget(ctx context.Context) (channel, chatID string) {
	dt := DeliveryTargetFromContext(ctx)
	return dt.Channel, dt.ChatID
}

// ─────────────────────────────────────────────────────────────────────────────
// Async Tool Support
// ─────────────────────────────────────────────────────────────────────────────

// AsyncCompleteCallback is called when an async tool completes execution.
// The result contains the final output that should be delivered.
type AsyncCompleteCallback func(result *ToolResult)

// AsyncToolConfig holds configuration for async tool execution.
type AsyncToolConfig struct {
	// Label is a human-readable description of the async task.
	Label string

	// OnComplete is called when the async task finishes.
	OnComplete AsyncCompleteCallback

	// Timeout is the maximum duration for the async task.
	// Default is 5 minutes if not set.
	Timeout time.Duration
}

// RunAsync executes a function in the background and calls the callback when done.
// This is a helper for tools that need to run long operations asynchronously.
//
// The function returns immediately with an AsyncResult. The actual work happens
// in a background goroutine, and the callback is invoked when complete.
//
// The context is propagated to the async function, so cancellation of the parent
// context will also cancel the async operation.
//
// Example:
//
//	func myAsyncTool(ctx context.Context, args map[string]any) (any, error) {
//		config := AsyncToolConfig{
//			Label: "processing files",
//			OnComplete: func(result *ToolResult) {
//				// Handle completion - e.g., send to user
//			},
//		}
//
//		RunAsync(ctx, config, func(ctx context.Context) *ToolResult {
//			// Long-running operation
//			return &ToolResult{Content: "done"}
//		})
//
//		return AsyncResult("Started processing files..."), nil
//	}
func RunAsync(ctx context.Context, config AsyncToolConfig, fn func(ctx context.Context) *ToolResult) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	go func() {
		// Derive from original context to respect cancellation
		asyncCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		result := fn(asyncCtx)

		if config.OnComplete != nil {
			// Use background context if original was cancelled
			callbackCtx := context.Background()
			select {
			case <-ctx.Done():
				// Original context cancelled, use background for callback
			default:
				callbackCtx = ctx
			}
			_ = callbackCtx // Callback doesn't need context currently

			config.OnComplete(result)
		}
	}()
}

// sequentialTools are tools that must not run in parallel (shared state).
var sequentialTools = map[string]bool{
	"bash": true, "write_file": true, "edit_file": true,
	"ssh": true, "scp": true, "exec": true, "set_env": true,
}

// ToolHook is a callback that runs before or after tool execution.
// Before hooks can modify args or block execution by returning an error.
// After hooks can observe/log the result but cannot modify it.
type ToolHook struct {
	// Name identifies this hook for logging and debugging.
	Name string

	// BeforeToolCall is called before the tool handler executes.
	// Return modified args (or original), or an error to block execution.
	// If blocked is true, the tool is not executed and blockReason is returned.
	BeforeToolCall func(toolName string, args map[string]any) (modifiedArgs map[string]any, blocked bool, blockReason string)

	// AfterToolCall is called after the tool handler executes (success or error).
	AfterToolCall func(toolName string, args map[string]any, result string, err error)
}

// ToolExecutor manages tool registration and dispatches tool calls.
type ToolExecutor struct {
	tools       map[string]*registeredTool
	timeout     time.Duration
	bashTimeout time.Duration // timeout for bash/ssh/scp/exec (default: 5min)
	logger      *slog.Logger
	guard       *ToolGuard
	mu          sync.RWMutex

	// vault is the optional vault reader for checking skill setup
	vault skills.VaultReader

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

	// hooks holds registered before/after tool execution hooks.
	hooks []*ToolHook

	// abortCh is closed when an abort is requested, signaling all running
	// tools to stop as soon as possible. Each run creates a fresh channel.
	abortCh   chan struct{}
	abortOnce sync.Once
}

// NewToolExecutor creates a new empty tool executor.
func NewToolExecutor(logger *slog.Logger) *ToolExecutor {
	return &ToolExecutor{
		tools:        make(map[string]*registeredTool),
		timeout:      DefaultToolTimeout,
		bashTimeout:  5 * time.Minute,
		logger:       logger.With("component", "tool_executor"),
		callerLevel:  AccessOwner, // Default to owner for CLI usage.
		parallel:     true,
		maxParallel:  5,
		abortCh:      make(chan struct{}),
	}
}

// ResetAbort creates a fresh abort channel for a new run.
func (e *ToolExecutor) ResetAbort() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.abortCh = make(chan struct{})
	e.abortOnce = sync.Once{}
}

// Abort signals all running tools to stop. Safe to call multiple times.
func (e *ToolExecutor) Abort() {
	e.abortOnce.Do(func() {
		close(e.abortCh)
	})
}

// IsAborted returns true if an abort has been signaled.
func (e *ToolExecutor) IsAborted() bool {
	select {
	case <-e.abortCh:
		return true
	default:
		return false
	}
}

// AbortCh returns the abort channel for tools to select on.
func (e *ToolExecutor) AbortCh() <-chan struct{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.abortCh
}

// SetGuard configures the security guard for tool execution.
func (e *ToolExecutor) SetGuard(guard *ToolGuard) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.guard = guard
}

// RegisterHook adds a before/after tool execution hook.
// Hooks are called in registration order. Multiple hooks can be registered.
func (e *ToolExecutor) RegisterHook(hook *ToolHook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hooks = append(e.hooks, hook)
	e.logger.Info("tool hook registered", "hook", hook.Name)
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

// Configure applies ToolExecutorConfig (parallel, max_parallel, timeouts).
func (e *ToolExecutor) Configure(cfg ToolExecutorConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.parallel = cfg.Parallel
	e.maxParallel = cfg.MaxParallel
	if e.maxParallel <= 0 {
		e.maxParallel = 5
	}
	if cfg.DefaultTimeoutSeconds > 0 {
		e.timeout = time.Duration(cfg.DefaultTimeoutSeconds) * time.Second
	}
	if cfg.BashTimeoutSeconds > 0 {
		e.bashTimeout = time.Duration(cfg.BashTimeoutSeconds) * time.Second
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

// SetVault sets the vault reader for skill setup checking.
func (e *ToolExecutor) SetVault(vault skills.VaultReader) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vault = vault
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

// ─────────────────────────────────────────────────────────────────────────────
// Skill Setup Checking
// ─────────────────────────────────────────────────────────────────────────────

// VaultReaderAdapter adapts a Vault to implement skills.VaultReader.
type VaultReaderAdapter struct {
	getKey func(key string) (string, error)
	hasKey func(key string) bool
}

// NewVaultReaderAdapter creates a VaultReader from getter functions.
func NewVaultReaderAdapter(getKey func(key string) (string, error), hasKey func(key string) bool) *VaultReaderAdapter {
	return &VaultReaderAdapter{getKey: getKey, hasKey: hasKey}
}

// Get returns the value for a key.
func (v *VaultReaderAdapter) Get(key string) (string, error) {
	if v.getKey == nil {
		return "", fmt.Errorf("vault not configured")
	}
	return v.getKey(key)
}

// Has returns true if the key exists.
func (v *VaultReaderAdapter) Has(key string) bool {
	if v.hasKey == nil {
		return false
	}
	return v.hasKey(key)
}

// CheckSkillSetup checks if a skill is properly configured.
// Returns setup status and nil if check was performed, or error if skill doesn't support setup checking.
func CheckSkillSetup(skill skills.Skill, vault skills.VaultReader) (*skills.SetupStatus, error) {
	checker, ok := skill.(skills.SkillSetupChecker)
	if !ok {
		return nil, fmt.Errorf("skill does not support setup checking")
	}
	status := checker.CheckSetup(vault)
	return &status, nil
}

// SkillNeedsSetup returns true if a skill requires configuration that is missing.
func SkillNeedsSetup(skill skills.Skill, vault skills.VaultReader) bool {
	status, err := CheckSkillSetup(skill, vault)
	if err != nil {
		return false // Skill doesn't need setup
	}
	return !status.IsComplete
}

// FormatSetupPrompt creates a user-friendly prompt for missing configuration.
func FormatSetupPrompt(skillName string, status *skills.SetupStatus) string {
	if status == nil || status.IsComplete {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ Skill **%s** needs configuration before use.\n\n", skillName))

	// Build details from MissingRequirements if available
	if len(status.MissingRequirements) > 0 {
		sb.WriteString("Required credentials:\n\n")
		for i, req := range status.MissingRequirements {
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, req.Name))
			sb.WriteString(fmt.Sprintf("   - Vault key: `%s`\n", req.Key))
			if req.Description != "" {
				sb.WriteString(fmt.Sprintf("   - %s\n", req.Description))
			}
			if req.Example != "" {
				sb.WriteString(fmt.Sprintf("   - Example: `%s`\n", req.Example))
			}
			if req.EnvVar != "" {
				sb.WriteString(fmt.Sprintf("   - Or set env var: `%s`\n", req.EnvVar))
			}
			sb.WriteString("\n")
		}
	} else if status.Message != "" {
		sb.WriteString(status.Message)
		sb.WriteString("\n")
	}

	sb.WriteString("To configure, provide the values and I'll save them to the vault with the correct keys.\n")
	sb.WriteString("Example: 'My API key is abc123 and my token is xyz789'\n")

	return sb.String()
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
	// Prefer per-request context (goroutine-safe) over global shared state.
	callerLevel := CallerLevelFromContext(ctx)
	callerJID := CallerJIDFromContext(ctx)
	if callerLevel == AccessNone {
		// Fallback to global state for backward compatibility (CLI, tests).
		callerLevel = e.callerLevel
		callerJID = e.callerJID
	}
	e.mu.RUnlock()

	if !ok {
		result.Content = formatToolError(name, fmt.Errorf("unknown tool %q", name))
		result.Error = fmt.Errorf("unknown tool: %s", name)
		e.logger.Warn("unknown tool called", "name", name)
		return result
	}

	// Parse arguments from JSON string.
	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		result.Content = formatToolError(name, fmt.Errorf("error parsing arguments: %w", err))
		result.Error = err
		e.logger.Warn("tool argument parse error", "name", name, "error", err)
		return result
	}

	// Security check: verify the caller has permission.
	var check ToolCheckResult
	if guard != nil {
		// Extract profile from context (workspace may override global profile).
		profile := ToolProfileFromContext(ctx)
		check = guard.CheckWithProfile(name, callerLevel, args, profile)
		if !check.Allowed {
			result.Content = formatToolError(name, fmt.Errorf("access denied: %s", check.Reason))
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
	// immediately (non-blocking) and run the tool in the
	// background once approved. The result is sent to the user via ProgressSender.
	if check.RequiresConfirmation {
		e.mu.RLock()
		req := e.confirmationRequester
		e.mu.RUnlock()

		// Use per-request context (goroutine-safe) for session/caller,
		// falling back to global state for backward compatibility.
		sessionID := SessionIDFromContext(ctx)
		callerJID := CallerJIDFromContext(ctx)
		if sessionID == "" || callerJID == "" {
			e.mu.RLock()
			if sessionID == "" {
				sessionID = e.sessionID
			}
			if callerJID == "" {
				callerJID = e.callerJID
			}
			e.mu.RUnlock()
		}

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
	// Give bash/ssh/scp longer timeouts (configurable via bash_timeout_seconds).
	if name == "bash" || name == "ssh" || name == "scp" || name == "exec" {
		timeout = e.bashTimeout
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

	// Propagate VaultReader to the tool context for skill setup checking.
	if e.vault != nil {
		execCtx = ContextWithVaultReader(execCtx, e.vault)
	} else if vr := VaultReaderFromContext(ctx); vr != nil {
		execCtx = ContextWithVaultReader(execCtx, vr)
	}

	defer cancel()

	// ── Before-tool hooks ──
	e.mu.RLock()
	hooks := e.hooks
	e.mu.RUnlock()

	for _, hook := range hooks {
		if hook.BeforeToolCall != nil {
			modArgs, blocked, reason := hook.BeforeToolCall(name, args)
			if blocked {
				result.Content = formatToolError(name, fmt.Errorf("blocked by hook %q: %s", hook.Name, reason))
				result.Error = fmt.Errorf("blocked by hook: %s", reason)
				e.logger.Info("tool blocked by before-hook",
					"tool", name, "hook", hook.Name, "reason", reason)
				return result
			}
			if modArgs != nil {
				args = modArgs
			}
		}
	}

	e.logger.Debug("executing tool", "name", name, "args_keys", mapKeys(args))

	// Progress heartbeat is handled by the ProgressSender cooldown in assistant.go.
	// Individual tools (e.g. claude-code) send their own progress; we don't add
	// a redundant generic heartbeat to avoid flooding the user.
	progressDone := make(chan struct{})

	start := time.Now()
	output, err := tool.Handler(execCtx, args)
	close(progressDone)
	duration := time.Since(start)

	// ── After-tool hooks ──
	resultStr := ""
	if err != nil {
		resultStr = fmt.Sprintf("Error: %v", err)
	} else {
		resultStr = formatToolOutput(output)
	}
	for _, hook := range hooks {
		if hook.AfterToolCall != nil {
			hook.AfterToolCall(name, args, resultStr, err)
		}
	}

	if err != nil {
		// Structured JSON error result ({ status, tool, error }) for parseable LLM retry logic.
		// This makes tool errors parseable by the LLM for better retry logic.
		result.Content = formatToolError(name, err)
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
	result.Content = resultStr

	// ── Tool result size guard ──
	// Cap oversized results proactively to prevent context overflow.
	if len(result.Content) > HardMaxToolResultChars {
		original := len(result.Content)
		result.Content = result.Content[:HardMaxToolResultChars] + "\n\n... [truncated: result was " +
			fmt.Sprintf("%d", original) + " chars, capped at " + fmt.Sprintf("%d", HardMaxToolResultChars) + "]"
		e.logger.Warn("tool result truncated by size guard",
			"name", name,
			"original_chars", original,
			"capped_at", HardMaxToolResultChars,
		)
	}

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

// HardMaxToolResultChars is the absolute maximum size for a tool result.
// Results exceeding this are truncated before entering the conversation
// to prevent context overflow.
const HardMaxToolResultChars = 400_000

// formatToolError creates a structured JSON error result.
// This format is more parseable by the LLM than plain "Error: ..." text.
func formatToolError(toolName string, err error) string {
	errMsg := err.Error()
	if len(errMsg) > 2000 {
		errMsg = errMsg[:2000] + "... (truncated)"
	}
	b, _ := json.Marshal(map[string]string{
		"status": "error",
		"tool":   toolName,
		"error":  errMsg,
	})
	return string(b)
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
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
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
	// Check if skill needs setup validation
	checker, needsSetupCheck := skill.(skills.SkillSetupChecker)
	meta := skill.Metadata()

	if tool.Handler != nil {
		// Skill tool has an explicit handler — wrap with setup check.
		return func(ctx context.Context, args map[string]any) (any, error) {
			// Check setup before executing
			if needsSetupCheck {
				vault := VaultReaderFromContext(ctx)
				status := checker.CheckSetup(vault)
				if !status.IsComplete {
					// Return setup instructions as a dual result
					return DualToolResult(
						fmt.Sprintf("[SETUP_REQUIRED] Skill '%s' needs configuration: %s", meta.Name, status.Message),
						FormatSetupPrompt(meta.Name, &status),
					), nil
				}
			}
			return tool.Handler(ctx, args)
		}
	}

	// Fallback: call the skill's Execute method with the input arg.
	return func(ctx context.Context, args map[string]any) (any, error) {
		// Check setup before executing
		if needsSetupCheck {
			vault := VaultReaderFromContext(ctx)
			status := checker.CheckSetup(vault)
			if !status.IsComplete {
				// Return setup instructions as a dual result
				return DualToolResult(
					fmt.Sprintf("[SETUP_REQUIRED] Skill '%s' needs configuration: %s", meta.Name, status.Message),
					FormatSetupPrompt(meta.Name, &status),
				), nil
			}
		}

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
				return fmt.Sprintf("%s: %s...", toolName, sanitizeForMarkdown(cmd[:80]))
			}
			return toolName + ": " + sanitizeForMarkdown(cmd)
		}
	case "write_file", "edit_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return toolName + " " + path
		}
	case "ssh":
		if host, ok := args["host"].(string); ok {
			return "ssh " + sanitizeForMarkdown(host)
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
