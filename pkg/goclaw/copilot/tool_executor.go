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

// ToolExecutor manages tool registration and dispatches tool calls.
type ToolExecutor struct {
	tools   map[string]*registeredTool
	timeout time.Duration
	logger  *slog.Logger
	mu      sync.RWMutex
}

// NewToolExecutor creates a new empty tool executor.
func NewToolExecutor(logger *slog.Logger) *ToolExecutor {
	return &ToolExecutor{
		tools:   make(map[string]*registeredTool),
		timeout: DefaultToolTimeout,
		logger:  logger.With("component", "tool_executor"),
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
func (e *ToolExecutor) Tools() []ToolDefinition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	defs := make([]ToolDefinition, 0, len(e.tools))
	for _, t := range e.tools {
		defs = append(defs, t.Definition)
	}
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
// Returns results in the same order as the input calls.
func (e *ToolExecutor) Execute(ctx context.Context, calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))

	// Execute tools sequentially to avoid overwhelming resources.
	// Future: add concurrency option for independent tools.
	for i, call := range calls {
		results[i] = e.executeSingle(ctx, call)
	}

	return results
}

// executeSingle runs a single tool call and returns the result.
func (e *ToolExecutor) executeSingle(ctx context.Context, call ToolCall) ToolResult {
	name := call.Function.Name
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       name,
	}

	e.mu.RLock()
	tool, ok := e.tools[name]
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

	// Execute with timeout.
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
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
		return result
	}

	// Serialize output to string.
	result.Content = formatToolOutput(output)

	e.logger.Info("tool executed",
		"name", name,
		"duration_ms", duration.Milliseconds(),
		"output_len", len(result.Content),
	)

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

// mapKeys returns the keys of a map for logging.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
