// Package skills – coding_claude_code.go integrates with the Claude Code CLI
// to provide full-stack coding capabilities: code editing, review, commit, PR,
// deployment, testing, refactoring, and any development task.
//
// Instead of implementing many granular tools (git_status, code_read, etc.),
// this skill delegates everything to Claude Code, which has its own rich set
// of tools (Bash, Read, Edit, Grep, Glob, Write, etc.).
//
// Requirements:
//   - Claude Code CLI installed: npm install -g @anthropic-ai/claude-code
//   - Authenticated: claude setup-token or claude login
//   - The user must enable "claude-code" in skills.builtin config.
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// claudeCodeResult represents the JSON output from `claude -p --output-format json`.
type claudeCodeResult struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	SessionID string  `json:"session_id"`
	NumTurns  int     `json:"num_turns"`
	TotalCost float64 `json:"total_cost_usd"`
	Duration  int     `json:"duration_ms"`
	Errors    []any   `json:"errors"`
}

// claudeCodeSkill implements the claude-code skill.
type claudeCodeSkill struct {
	provider ProjectProvider

	// sessions maps GoClaw session key → last Claude Code session ID,
	// allowing multi-step coding tasks to be continued.
	sessions   map[string]string
	sessionsMu sync.RWMutex

	// Configurable defaults (can be overridden per call).
	defaultModel    string
	defaultBudget   float64
	skipPermissions bool
	timeout         time.Duration
}

// NewClaudeCodeSkill creates the claude-code skill.
// provider may be nil if project management is not configured.
func NewClaudeCodeSkill(provider ProjectProvider) Skill {
	// Model: empty means use Claude Code's own default (from its config).
	model := os.Getenv("GOCLAW_CLAUDE_CODE_MODEL")

	// Budget: 0 means no limit (same as interactive Claude Code).
	// Only set a limit if explicitly configured via env var.
	var budget float64
	if budgetStr := os.Getenv("GOCLAW_CLAUDE_CODE_BUDGET"); budgetStr != "" {
		if v, err := parseFloat(budgetStr); err == nil && v > 0 {
			budget = v
		}
	}

	timeoutMin := 15
	if v := os.Getenv("GOCLAW_CLAUDE_CODE_TIMEOUT_MIN"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			timeoutMin = n
		}
	}

	return &claudeCodeSkill{
		provider:        provider,
		sessions:        make(map[string]string),
		defaultModel:    model,
		defaultBudget:   budget,
		skipPermissions: true,
		timeout:         time.Duration(timeoutMin) * time.Minute,
	}
}

func (s *claudeCodeSkill) Metadata() Metadata {
	return Metadata{
		Name:        "claude-code",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Full-stack coding assistant powered by Claude Code CLI. Handles code editing, review, commit, PR, deploy, test, refactor — any development task.",
		Category:    "development",
		Tags: []string{
			"code", "git", "commit", "review", "deploy", "pr", "refactor",
			"programming", "claude-code", "backend", "frontend", "devops",
		},
	}
}

func (s *claudeCodeSkill) Tools() []Tool {
	return []Tool{
		{
			Name: "execute",
			Description: `Execute any coding task using Claude Code. Claude Code has full access to:
- Read, edit, create, search files (Read, Edit, Write, Grep, Glob)
- Run shell commands (Bash: git, npm, docker, make, etc.)
- Create commits, branches, PRs
- Run tests, lint, build
- Deploy, configure servers
- Multi-file refactoring, code review
Send clear, detailed instructions. The task runs in the active project directory.`,
			Parameters: []ToolParameter{
				{Name: "prompt", Type: "string", Description: "The coding task or instruction. Be specific and detailed.", Required: true},
				{Name: "project_id", Type: "string", Description: "Project ID to work on. Empty = active project."},
				{Name: "session_key", Type: "string", Description: "GoClaw session key (auto-provided by system)."},
				{Name: "continue_session", Type: "boolean", Description: "Continue the previous Claude Code session for multi-step tasks. Default: false."},
				{Name: "model", Type: "string", Description: "Claude model alias: 'sonnet', 'opus', 'haiku'. Empty = Claude Code's own default."},
				{Name: "max_budget", Type: "number", Description: "Max budget in USD. 0 or empty = no limit (normal Claude Code behavior)."},
				{Name: "allowed_tools", Type: "string", Description: "Restrict tools (e.g. 'Read,Grep,Glob' for read-only). Empty = all tools."},
				{Name: "add_dirs", Type: "string", Description: "Comma-separated additional directories Claude Code can access."},
				{Name: "permission_mode", Type: "string", Description: "Permission mode: 'default', 'plan' (read-only analysis), 'bypassPermissions'. Default: bypassPermissions."},
			},
			Handler: s.handleExecute,
		},
		{
			Name:        "check",
			Description: "Check if Claude Code CLI is installed, authenticated, and ready. Reports version, auth status, and available models.",
			Parameters:  []ToolParameter{},
			Handler:     s.handleCheck,
		},
	}
}

func (s *claudeCodeSkill) SystemPrompt() string {
	return `You have the claude-code skill which integrates with Claude Code CLI for advanced software development.

WHEN TO USE claude-code_execute:
- Code editing, creation, refactoring (any language/framework)
- Git operations: commit, branch, merge, rebase, PR
- Code review and analysis
- Running tests, lint, build
- Searching codebase (grep, find patterns)
- DevOps: Docker, deploy, server config
- Multi-file changes, large refactors

BEST PRACTICES:
1. Always activate a project first (project-manager_activate) so Claude Code runs in the right directory
2. For multi-step tasks, use continue_session=true to keep context between calls
3. Be specific in your prompts — Claude Code is powerful but needs clear instructions
4. For read-only analysis, set permission_mode="plan"
5. Claude Code has its own tools (Bash, Read, Edit, Grep, Glob, Write, etc.)
6. It runs with no budget limit by default, just like normal interactive Claude Code

DO NOT use this for non-coding tasks. For general questions, web search, etc. use the appropriate other skills.`
}

func (s *claudeCodeSkill) Triggers() []string {
	return []string{
		"code", "git", "commit", "push", "pull request", "PR", "branch",
		"diff", "merge", "deploy", "review", "refactor", "edit code",
		"create file", "test", "lint", "build", "docker", "server",
		"programming", "bug fix", "feature", "backend", "frontend",
	}
}

func (s *claudeCodeSkill) Init(_ context.Context, cfg map[string]any) error {
	if model, ok := cfg["claude_code_model"].(string); ok && model != "" {
		s.defaultModel = model
	}
	if budget, ok := cfg["claude_code_budget"].(float64); ok && budget > 0 {
		s.defaultBudget = budget
	}
	if skip, ok := cfg["claude_code_skip_permissions"].(bool); ok {
		s.skipPermissions = skip
	}
	return nil
}

func (s *claudeCodeSkill) Execute(ctx context.Context, input string) (string, error) {
	result, err := s.handleExecute(ctx, map[string]any{"prompt": input})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

func (s *claudeCodeSkill) Shutdown() error { return nil }

// ── Handlers ──

func (s *claudeCodeSkill) handleExecute(ctx context.Context, args map[string]any) (any, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Check if claude is available.
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("Claude Code CLI not found. Install: npm install -g @anthropic-ai/claude-code")
	}

	// Resolve working directory from project.
	var workDir string
	var projectCtx string
	if s.provider != nil {
		p := ccResolveProject(s.provider, args)
		if p != nil {
			workDir = p.RootPath
			projectCtx = buildProjectContext(p)
		}
	}

	// Build CLI arguments.
	cliArgs := []string{"-p", "--output-format", "json"}

	// Permission mode.
	permMode, _ := args["permission_mode"].(string)
	if permMode == "" && s.skipPermissions {
		permMode = "bypassPermissions"
	}
	if permMode != "" {
		cliArgs = append(cliArgs, "--permission-mode", permMode)
	}

	// Model.
	model, _ := args["model"].(string)
	if model == "" {
		model = s.defaultModel
	}
	if model != "" {
		cliArgs = append(cliArgs, "--model", model)
	}

	// Budget.
	budget := s.defaultBudget
	if b, ok := args["max_budget"].(float64); ok && b > 0 {
		budget = b
	}
	if budget > 0 {
		cliArgs = append(cliArgs, "--max-budget-usd", fmt.Sprintf("%.2f", budget))
	}

	// Session continuation.
	if cont, _ := args["continue_session"].(bool); cont {
		sessionKey, _ := args["session_key"].(string)
		s.sessionsMu.RLock()
		prevSessionID, hasPrev := s.sessions[sessionKey]
		s.sessionsMu.RUnlock()
		if hasPrev && prevSessionID != "" {
			cliArgs = append(cliArgs, "--resume", prevSessionID)
		} else {
			cliArgs = append(cliArgs, "--continue")
		}
	}

	// Allowed tools restriction.
	if tools, _ := args["allowed_tools"].(string); tools != "" {
		cliArgs = append(cliArgs, "--allowedTools", tools)
	}

	// Additional directories.
	if dirs, _ := args["add_dirs"].(string); dirs != "" {
		for _, d := range strings.Split(dirs, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				cliArgs = append(cliArgs, "--add-dir", d)
			}
		}
	}

	// Inject project context as append-system-prompt so Claude Code knows
	// about the project's language, framework, commands, etc.
	if projectCtx != "" {
		cliArgs = append(cliArgs, "--append-system-prompt", projectCtx)
	}

	// The prompt goes last.
	cliArgs = append(cliArgs, prompt)

	// Execute with timeout.
	execCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "claude", cliArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set HOME if not set (needed for claude auth).
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()

	// Parse JSON response.
	var result claudeCodeResult
	if jsonErr := json.Unmarshal(out, &result); jsonErr == nil {
		// Store session for continuation.
		if result.SessionID != "" {
			sessionKey, _ := args["session_key"].(string)
			if sessionKey != "" {
				s.sessionsMu.Lock()
				s.sessions[sessionKey] = result.SessionID
				s.sessionsMu.Unlock()
			}
		}

		// Check for errors.
		if result.IsError || result.Subtype == "error" {
			errMsg := result.Result
			if errMsg == "" {
				errMsg = fmt.Sprintf("Claude Code error (subtype: %s)", result.Subtype)
			}
			return nil, fmt.Errorf("%s", errMsg)
		}

		// Build response with metadata.
		response := result.Result
		if response == "" && result.Subtype == "error_max_budget_usd" {
			return nil, fmt.Errorf("Claude Code exceeded the budget limit of $%.2f", budget)
		}

		// Append cost/metadata footer.
		if result.TotalCost > 0 || result.NumTurns > 0 {
			meta := fmt.Sprintf("\n\n---\n💰 $%.4f | %d turns | %dms",
				result.TotalCost, result.NumTurns, result.Duration)
			if result.SessionID != "" {
				meta += fmt.Sprintf(" | session: %s", result.SessionID[:8])
			}
			response += meta
		}

		return ccTruncate(response, 15000), nil
	}

	// JSON parse failed — return raw output.
	if err != nil {
		raw := strings.TrimSpace(string(out))
		if raw != "" {
			return nil, fmt.Errorf("claude code: %s", ccTruncate(raw, 3000))
		}
		return nil, fmt.Errorf("claude code failed: %v", err)
	}

	return ccTruncate(string(out), 15000), nil
}

func (s *claudeCodeSkill) handleCheck(_ context.Context, _ map[string]any) (any, error) {
	// Check if claude is in PATH.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return map[string]any{
			"installed": false,
			"message":   "Claude Code CLI not found. Install with: npm install -g @anthropic-ai/claude-code",
			"docs":      "https://docs.anthropic.com/en/docs/claude-code",
		}, nil
	}

	// Get version.
	versionOut, _ := exec.Command("claude", "--version").CombinedOutput()
	version := strings.TrimSpace(string(versionOut))

	// Check auth by running doctor.
	doctorOut, doctorErr := exec.Command("claude", "doctor").CombinedOutput()
	authOK := doctorErr == nil
	doctorInfo := strings.TrimSpace(string(doctorOut))

	return map[string]any{
		"installed":     true,
		"path":          claudePath,
		"version":       version,
		"authenticated": authOK,
		"doctor":        ccTruncate(doctorInfo, 2000),
		"message":       fmt.Sprintf("Claude Code %s ready at %s", version, claudePath),
	}, nil
}

// ── Helpers ──

// ccResolveProject resolves a project from args (project_id or session active).
// Prefixed with "cc" to avoid conflict with the package-level resolveProject
// from other coding skill files during transition.
func ccResolveProject(provider ProjectProvider, args map[string]any) *ProjectInfo {
	if id, _ := args["project_id"].(string); id != "" {
		return provider.Get(id)
	}
	if key, _ := args["session_key"].(string); key != "" {
		return provider.ActiveProject(key)
	}
	return nil
}

// buildProjectContext creates a system prompt fragment with project metadata.
func buildProjectContext(p *ProjectInfo) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Active project: %s\n", p.Name))
	b.WriteString(fmt.Sprintf("Language: %s\n", p.Language))
	if p.Framework != "" {
		b.WriteString(fmt.Sprintf("Framework: %s\n", p.Framework))
	}
	if p.GitRemote != "" {
		b.WriteString(fmt.Sprintf("Git remote: %s\n", p.GitRemote))
	}
	if p.BuildCmd != "" {
		b.WriteString(fmt.Sprintf("Build command: %s\n", p.BuildCmd))
	}
	if p.TestCmd != "" {
		b.WriteString(fmt.Sprintf("Test command: %s\n", p.TestCmd))
	}
	if p.LintCmd != "" {
		b.WriteString(fmt.Sprintf("Lint command: %s\n", p.LintCmd))
	}
	if p.StartCmd != "" {
		b.WriteString(fmt.Sprintf("Start command: %s\n", p.StartCmd))
	}
	if p.DeployCmd != "" {
		b.WriteString(fmt.Sprintf("Deploy command: %s\n", p.DeployCmd))
	}
	return b.String()
}

// ccTruncate truncates a string to maxLen characters.
func ccTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// parseFloat is a simple float parser for env vars.
func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// parseInt is a simple int parser for env vars.
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
