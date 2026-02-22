// Package copilot – plugin_system.go implements an extensible plugin system
// that supports HTTP-based plugins (GitHub, Jira, Sentry, etc.) with
// webhook integration and tool registration.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------- Plugin Types ----------

// Plugin represents an installed plugin with its tools and hooks.
type Plugin struct {
	Name        string       `json:"name" yaml:"name"`
	Version     string       `json:"version" yaml:"version"`
	Description string       `json:"description" yaml:"description"`
	Author      string       `json:"author" yaml:"author"`
	Enabled     bool         `json:"enabled" yaml:"enabled"`
	Config      map[string]any `json:"config" yaml:"config"`
	Tools       []PluginTool `json:"tools" yaml:"tools"`
	Webhooks    []WebhookDef `json:"webhooks" yaml:"webhooks"`
}

// PluginTool describes a tool provided by a plugin.
type PluginTool struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description" yaml:"description"`
	Endpoint    string         `json:"endpoint" yaml:"endpoint"` // HTTP endpoint for the tool
	Method      string         `json:"method" yaml:"method"`     // GET, POST, etc.
	Headers     map[string]string `json:"headers" yaml:"headers"`
	Parameters  map[string]any `json:"parameters" yaml:"parameters"`
}

// WebhookDef describes an incoming webhook configuration.
type WebhookDef struct {
	Path      string            `json:"path" yaml:"path"`
	Secret    string            `json:"secret" yaml:"secret"`
	Events    []string          `json:"events" yaml:"events"`
	Headers   map[string]string `json:"headers" yaml:"headers"`
}

// ---------- Plugin Manager ----------

// PluginManager manages installed plugins and their lifecycle.
type PluginManager struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
	logger  interface{ Printf(string, ...any) }
}

// NewPluginManager creates a new plugin manager.
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: make(map[string]*Plugin),
	}
}

// Install adds a plugin to the manager.
func (pm *PluginManager) Install(p *Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[p.Name]; exists {
		return fmt.Errorf("plugin %q already installed", p.Name)
	}
	p.Enabled = true
	pm.plugins[p.Name] = p
	return nil
}

// Uninstall removes a plugin.
func (pm *PluginManager) Uninstall(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[name]; !exists {
		return fmt.Errorf("plugin %q not installed", name)
	}
	delete(pm.plugins, name)
	return nil
}

// Get returns a plugin by name.
func (pm *PluginManager) Get(name string) (*Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.plugins[name]
	return p, ok
}

// List returns all installed plugins.
func (pm *PluginManager) List() []*Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*Plugin, 0, len(pm.plugins))
	for _, p := range pm.plugins {
		result = append(result, p)
	}
	return result
}

// Enable enables a plugin.
func (pm *PluginManager) Enable(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	p.Enabled = true
	return nil
}

// Disable disables a plugin.
func (pm *PluginManager) Disable(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	p.Enabled = false
	return nil
}

// ---------- Webhook Dispatcher ----------

// WebhookDispatcher handles incoming webhook events from external services.
type WebhookDispatcher struct {
	mu       sync.RWMutex
	handlers map[string][]WebhookHandler
}

// WebhookHandler processes a webhook event.
type WebhookHandler func(ctx context.Context, event string, payload []byte) error

// NewWebhookDispatcher creates a new webhook dispatcher.
func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{
		handlers: make(map[string][]WebhookHandler),
	}
}

// On registers a handler for a webhook event type.
func (wd *WebhookDispatcher) On(event string, handler WebhookHandler) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	wd.handlers[event] = append(wd.handlers[event], handler)
}

// Dispatch sends a webhook event to all registered handlers.
func (wd *WebhookDispatcher) Dispatch(ctx context.Context, event string, payload []byte) error {
	wd.mu.RLock()
	handlers := wd.handlers[event]
	wildcard := wd.handlers["*"]
	wd.mu.RUnlock()

	all := append(handlers, wildcard...)
	for _, h := range all {
		if err := h(ctx, event, payload); err != nil {
			return fmt.Errorf("webhook handler error for %s: %w", event, err)
		}
	}
	return nil
}

// ---------- Built-in Plugin Definitions ----------

// GitHubPlugin returns a pre-configured GitHub integration plugin.
func GitHubPlugin(token string) *Plugin {
	return &Plugin{
		Name:        "github",
		Version:     "1.0.0",
		Description: "GitHub integration: issues, PRs, repos, actions",
		Author:      "devclaw",
		Enabled:     true,
		Config:      map[string]any{"token": token},
		Tools: []PluginTool{
			{Name: "github_issues", Description: "List, create, or update GitHub issues", Endpoint: "https://api.github.com", Method: "GET"},
			{Name: "github_prs", Description: "List, create, or review pull requests", Endpoint: "https://api.github.com", Method: "GET"},
			{Name: "github_actions", Description: "List and trigger GitHub Actions workflows", Endpoint: "https://api.github.com", Method: "GET"},
			{Name: "github_repos", Description: "Search and manage repositories", Endpoint: "https://api.github.com", Method: "GET"},
		},
		Webhooks: []WebhookDef{
			{Path: "/webhooks/github", Events: []string{"push", "pull_request", "issues", "workflow_run"}},
		},
	}
}

// JiraPlugin returns a pre-configured Jira integration plugin.
func JiraPlugin(baseURL, token string) *Plugin {
	return &Plugin{
		Name:        "jira",
		Version:     "1.0.0",
		Description: "Jira integration: issues, sprints, boards",
		Author:      "devclaw",
		Enabled:     true,
		Config:      map[string]any{"base_url": baseURL, "token": token},
		Tools: []PluginTool{
			{Name: "jira_issues", Description: "Search, create, or update Jira issues", Endpoint: baseURL + "/rest/api/3", Method: "GET"},
			{Name: "jira_sprint", Description: "Get current sprint and board info", Endpoint: baseURL + "/rest/agile/1.0", Method: "GET"},
			{Name: "jira_transition", Description: "Transition issue status", Endpoint: baseURL + "/rest/api/3", Method: "POST"},
		},
		Webhooks: []WebhookDef{
			{Path: "/webhooks/jira", Events: []string{"jira:issue_created", "jira:issue_updated", "sprint_started", "sprint_closed"}},
		},
	}
}

// SentryPlugin returns a pre-configured Sentry integration plugin.
func SentryPlugin(dsn, token string) *Plugin {
	return &Plugin{
		Name:        "sentry",
		Version:     "1.0.0",
		Description: "Sentry integration: errors, releases, performance",
		Author:      "devclaw",
		Enabled:     true,
		Config:      map[string]any{"dsn": dsn, "token": token},
		Tools: []PluginTool{
			{Name: "sentry_issues", Description: "List recent Sentry issues and errors", Endpoint: "https://sentry.io/api/0", Method: "GET"},
			{Name: "sentry_releases", Description: "List Sentry releases and deploys", Endpoint: "https://sentry.io/api/0", Method: "GET"},
		},
		Webhooks: []WebhookDef{
			{Path: "/webhooks/sentry", Events: []string{"error", "issue", "alert_rule_action"}},
		},
	}
}

// ---------- Tool Registration ----------

// RegisterPluginTools registers plugin management tools in the executor.
func RegisterPluginTools(executor *ToolExecutor, pm *PluginManager) {
	// plugin_list
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_list",
			Description: "List all installed plugins with their status and available tools.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		plugins := pm.List()
		if len(plugins) == 0 {
			return "No plugins installed.", nil
		}

		data, _ := json.MarshalIndent(plugins, "", "  ")
		return string(data), nil
	})

	// plugin_install
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_install",
			Description: "Install a built-in plugin (github, jira, sentry). Requires appropriate API tokens.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":     map[string]any{"type": "string", "enum": []string{"github", "jira", "sentry"}, "description": "Plugin to install"},
					"token":    map[string]any{"type": "string", "description": "API token for the service"},
					"base_url": map[string]any{"type": "string", "description": "Base URL (required for Jira)"},
				},
				"required": []string{"name", "token"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		name, _ := args["name"].(string)
		token, _ := args["token"].(string)
		baseURL, _ := args["base_url"].(string)

		var plugin *Plugin
		switch name {
		case "github":
			plugin = GitHubPlugin(token)
		case "jira":
			if baseURL == "" {
				return nil, fmt.Errorf("base_url is required for Jira plugin")
			}
			plugin = JiraPlugin(baseURL, token)
		case "sentry":
			plugin = SentryPlugin("", token)
		default:
			return nil, fmt.Errorf("unknown plugin: %s", name)
		}

		if err := pm.Install(plugin); err != nil {
			return nil, err
		}

		return fmt.Sprintf("Plugin %q installed successfully with %d tools.", name, len(plugin.Tools)), nil
	})

	// plugin_call
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_call",
			Description: "Execute a plugin tool by calling its HTTP endpoint.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plugin": map[string]any{"type": "string", "description": "Plugin name"},
					"tool":   map[string]any{"type": "string", "description": "Tool name within the plugin"},
					"path":   map[string]any{"type": "string", "description": "API path to append to endpoint"},
					"body":   map[string]any{"type": "string", "description": "Request body (JSON)"},
				},
				"required": []string{"plugin", "tool"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		pluginName, _ := args["plugin"].(string)
		toolName, _ := args["tool"].(string)
		apiPath, _ := args["path"].(string)
		body, _ := args["body"].(string)

		p, ok := pm.Get(pluginName)
		if !ok {
			return nil, fmt.Errorf("plugin %q not installed", pluginName)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("plugin %q is disabled", pluginName)
		}

		var tool *PluginTool
		for i := range p.Tools {
			if p.Tools[i].Name == toolName {
				tool = &p.Tools[i]
				break
			}
		}
		if tool == nil {
			return nil, fmt.Errorf("tool %q not found in plugin %q", toolName, pluginName)
		}

		url := tool.Endpoint
		if apiPath != "" {
			url = strings.TrimRight(url, "/") + "/" + strings.TrimLeft(apiPath, "/")
		}

		var bodyReader io.Reader
		method := tool.Method
		if body != "" {
			bodyReader = strings.NewReader(body)
			if method == "GET" {
				method = "POST"
			}
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		// Add auth headers
		if token, ok := p.Config["token"].(string); ok {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/json")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		result := strings.TrimSpace(string(respBody))

		const maxLen = 6000
		if len(result) > maxLen {
			result = result[:maxLen] + "\n... (truncated)"
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, result)
		}

		return result, nil
	})
}
