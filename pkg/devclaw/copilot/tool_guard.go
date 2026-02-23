// Package copilot – tool_guard.go implements a security layer that controls
// which tools can be used, by whom, and under what conditions.
//
// Security features:
//   - Tool-level access control (owner/admin/user)
//   - Destructive command detection and blocking
//   - Sensitive path protection
//   - SSH host allowlist
//   - Full audit logging of every tool execution
//   - Configurable confirmation for dangerous operations
package copilot

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ToolPermission defines which access level is required for a tool.
type ToolPermission string

const (
	PermOwner  ToolPermission = "owner"  // Only owner can use.
	PermAdmin  ToolPermission = "admin"  // Admin and owner.
	PermUser   ToolPermission = "user"   // Any authorized user.
	PermPublic ToolPermission = "public" // No restriction (used for read-only tools).
)

// ToolGuardConfig configures the security guard for tools.
type ToolGuardConfig struct {
	// Enable turns on the tool security guard (default: true).
	Enabled bool `yaml:"enabled"`

	// AuditLog path for recording all tool executions.
	AuditLogPath string `yaml:"audit_log"`

	// Profile selects a predefined tool profile.
	// Options: minimal, coding, messaging, full, or custom profile name.
	// Empty = use ToolPermissions directly (backward compatibility).
	Profile string `yaml:"profile"`

	// CustomProfiles allows defining custom tool profiles.
	CustomProfiles map[string]ToolProfile `yaml:"custom_profiles"`

	// ToolPermissions overrides per-tool permission levels.
	// key = tool name, value = "owner"/"admin"/"user"/"public".
	ToolPermissions map[string]string `yaml:"tool_permissions"`

	// AllowDestructive enables destructive commands (rm -rf /, mkfs, dd, etc)
	// for the owner. When false (default), these are blocked for everyone.
	// When true, owner can run them; non-owners are still blocked.
	AllowDestructive bool `yaml:"allow_destructive"`

	// AllowSudo allows sudo commands. When false (default), sudo is blocked
	// for non-owners. When true, owner and admin can use sudo.
	AllowSudo bool `yaml:"allow_sudo"`

	// AllowReboot allows shutdown/reboot/halt commands. Default: false.
	AllowReboot bool `yaml:"allow_reboot"`

	// DangerousCommands are additional regex patterns for commands that should
	// be blocked. These are added ON TOP of the defaults (not replacing them).
	// To disable all defaults, set allow_destructive: true.
	DangerousCommands []string `yaml:"dangerous_commands"`

	// ProtectedPaths are file paths that cannot be read or written by non-owners.
	// Supports glob patterns. If empty, defaults are used.
	ProtectedPaths []string `yaml:"protected_paths"`

	// SSHAllowedHosts restricts which hosts can be connected via SSH.
	// Empty list = any host allowed (no restriction). Use "*" explicitly to allow all.
	SSHAllowedHosts []string `yaml:"ssh_allowed_hosts"`

	// BlockSudo blocks sudo commands for non-owners (default: true).
	// Deprecated: use AllowSudo instead. Kept for backward compatibility.
	BlockSudo bool `yaml:"block_sudo"`

	// AutoApprove lists tools that can execute without any permission check,
	// even for regular users. Use with caution. Example: ["web_search", "memory"]
	AutoApprove []string `yaml:"auto_approve"`

	// RequireConfirmation lists tools that require the user to confirm via
	// the chat before executing. The agent will ask "Confirm: <action>?" and
	// wait for approval. Example: ["bash", "ssh", "scp", "write_file"]
	RequireConfirmation []string `yaml:"require_confirmation"`

	// DestructiveProtection configures rate limiting and batch detection for
	// destructive tools like cron_remove, vault_delete, etc.
	DestructiveProtection DestructiveToolsConfig `yaml:"destructive_protection"`
}

// DefaultToolGuardConfig returns safe defaults for the tool security guard.
// Owners have full access by default - adjust if you need stricter security.
func DefaultToolGuardConfig() ToolGuardConfig {
	return ToolGuardConfig{
		Enabled:          true,
		AuditLogPath:     "./data/audit.log",
		BlockSudo:        false, // Deprecated: use AllowSudo instead
		AllowDestructive: true,  // Allow owners to run chown, chmod, rm -rf, etc.
		AllowSudo:        true,  // Allow owners to run sudo (bash is already restricted to owner)
		AllowReboot:      false, // Don't allow reboot by default
		ToolPermissions: map[string]string{
			// System tools with machine access.
			"bash":         "owner",
			"ssh":          "owner",
			"scp":          "owner",
			"exec":         "admin",
			"set_env":      "owner",
			// File tools.
			"write_file":   "admin",
			"edit_file":    "admin",
			"read_file":    "user",
			"list_files":   "user",
			"search_files": "user",
			"glob_files":   "user",
			// Skill management.
			"install_skill": "admin",
			"remove_skill":  "admin",
			"init_skill":    "admin",
			"edit_skill":    "admin",
			"add_script":    "admin",
			"search_skills": "user",
			"list_skills":   "user",
			"test_skill":    "user",
			// Memory.
			"memory": "user",
			// Scheduler.
			"cron_add":    "admin",
			"cron_list":   "user",
			"cron_remove": "admin",
			// Web.
			"web_search": "user",
			"web_fetch":  "user",
		},
	}
}

// ── Tool Groups ──
// Groups can be used in Allow/Deny lists with "group:" prefix.
// Example: deny: ["group:sessions", "group:runtime"]

// ToolGroups maps group names to tool name lists.
// Allows policy management at a higher level than individual tools.
var ToolGroups = map[string][]string{
	"group:memory":    {"memory"},
	"group:web":       {"web_search", "web_fetch"},
	"group:fs":        {"read_file", "write_file", "edit_file", "list_files", "search_files", "glob_files"},
	"group:runtime":   {"bash", "exec", "ssh", "scp", "set_env"},
	"group:subagents": {"spawn_subagent", "list_subagents", "wait_subagent", "stop_subagent"},
	"group:skills":    {"install_skill", "remove_skill", "search_skills", "list_skills", "test_skill", "edit_skill", "add_script", "init_skill", "skill_defaults_list", "skill_defaults_install"},
	"group:scheduler": {"cron_add", "cron_list", "cron_remove"},
	"group:vault":     {"vault_save", "vault_get", "vault_list", "vault_delete"},
	"group:media":     {"describe_image", "transcribe_audio", "image-gen_generate_image"},
	"group:teams": {
		"team_manage",
		"team_agent",
		"team_task",
		"team_memory",
		"team_comm",
	},
}

// ExpandToolGroups expands group references (e.g. "group:memory") into
// individual tool names. Non-group entries are passed through as-is.
func ExpandToolGroups(names []string) []string {
	var result []string
	for _, name := range names {
		if tools, ok := ToolGroups[name]; ok {
			result = append(result, tools...)
		} else {
			result = append(result, name)
		}
	}
	return result
}

// ToolGuard enforces security policies on tool execution.
type ToolGuard struct {
	cfg       ToolGuardConfig
	logger    *slog.Logger
	auditFile *os.File

	// SQLite audit logger (optional; when set, replaces the file-based audit).
	sqliteAudit *SQLiteAuditLogger

	// Destructive tool tracker for rate limiting and batch detection.
	destructiveTracker *DestructiveTracker

	// Compiled patterns.
	dangerousPatterns   []*regexp.Regexp
	defaultPatternCount []bool // tracks which indices are default patterns
	protectedPaths      []string

	mu sync.Mutex
}

// NewToolGuard creates and initializes a tool security guard.
func NewToolGuard(cfg ToolGuardConfig, logger *slog.Logger) *ToolGuard {
	if logger == nil {
		logger = slog.Default()
	}

	guard := &ToolGuard{
		cfg:    cfg,
		logger: logger.With("component", "tool_guard"),
	}

	// Initialize destructive tool tracker.
	guard.destructiveTracker = NewDestructiveTracker(cfg.DestructiveProtection, logger)

	// Compile dangerous command patterns.
	guard.compileDangerousPatterns()

	// Set protected paths.
	guard.initProtectedPaths()

	// Open audit log.
	if cfg.AuditLogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.AuditLogPath), 0o755); err == nil {
			f, err := os.OpenFile(cfg.AuditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				logger.Warn("cannot open audit log", "path", cfg.AuditLogPath, "error", err)
			} else {
				guard.auditFile = f
			}
		}
	}

	logger.Info("tool guard initialized",
		"enabled", cfg.Enabled,
		"audit_log", cfg.AuditLogPath,
		"ssh_hosts", len(cfg.SSHAllowedHosts),
		"block_sudo", cfg.BlockSudo,
	)

	return guard
}

// CheckResult holds the result of a tool access check.
type ToolCheckResult struct {
	Allowed               bool
	Reason                string
	RequiresConfirmation  bool // true if tool needs user approval before execution
}

// CheckWithProfile evaluates tool access considering a profile's allow/deny lists.
// The profile check runs before the standard permission checks.
// If no profile is provided (nil), delegates directly to Check().
func (g *ToolGuard) CheckWithProfile(toolName string, callerLevel AccessLevel, args map[string]any, profile *ToolProfile) ToolCheckResult {
	// If no profile, use standard check.
	if profile == nil {
		return g.Check(toolName, callerLevel, args)
	}

	// Get all known tools for expansion.
	allTools := g.GetAllToolNames()

	// Create a profile checker.
	checker := NewProfileChecker(profile.Allow, profile.Deny, allTools)

	// Check deny first (takes precedence).
	if checker.IsDenied(toolName) {
		return ToolCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("tool '%s' is denied by profile '%s'", toolName, profile.Name),
		}
	}

	// Check allow list.
	if !checker.IsAllowed(toolName) {
		return ToolCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("tool '%s' is not in profile '%s' allow list", toolName, profile.Name),
		}
	}

	// Profile allows it, continue with standard permission checks.
	return g.Check(toolName, callerLevel, args)
}

// GetAllToolNames returns all known tool names from permissions and groups.
func (g *ToolGuard) GetAllToolNames() []string {
	seen := make(map[string]bool)
	var tools []string

	// Add tools from permissions config.
	for name := range g.cfg.ToolPermissions {
		if !seen[name] {
			seen[name] = true
			tools = append(tools, name)
		}
	}

	// Add tools from groups.
	for _, groupTools := range ToolGroups {
		for _, name := range groupTools {
			if !seen[name] {
				seen[name] = true
				tools = append(tools, name)
			}
		}
	}

	return tools
}

// GetActiveProfile returns the active profile based on config.
// Returns nil if no profile is configured or if profile is not found.
func (g *ToolGuard) GetActiveProfile() *ToolProfile {
	if g.cfg.Profile == "" {
		return nil
	}
	return GetProfile(g.cfg.Profile, g.cfg.CustomProfiles)
}

// Check evaluates whether a tool call is permitted for the given access level.
func (g *ToolGuard) Check(toolName string, callerLevel AccessLevel, args map[string]any) ToolCheckResult {
	if !g.cfg.Enabled {
		return ToolCheckResult{Allowed: true}
	}

	// 0. Check auto-approve list (bypass all checks).
	for _, name := range g.cfg.AutoApprove {
		if name == toolName {
			return ToolCheckResult{Allowed: true}
		}
	}

	// 0.5. Check destructive tool protection (rate limiting, batch detection).
	if g.destructiveTracker != nil && g.destructiveTracker.IsDestructive(toolName) {
		destructResult := g.destructiveTracker.Check(toolName)
		if !destructResult.Allowed {
			return ToolCheckResult{
				Allowed: false,
				Reason:  destructResult.Reason,
			}
		}
		// Record the call for rate limiting (even if blocked later by permissions).
		defer g.destructiveTracker.RecordCall(toolName)

		// If batch warning, inject into reason (but don't block).
		if destructResult.BatchWarning != "" {
			g.logger.Warn("destructive batch detected",
				"tool", toolName,
				"warning", destructResult.BatchWarning)
		}

		// If requires interactive confirmation, set flag.
		if destructResult.RequiresUserInput {
			return ToolCheckResult{
				Allowed:              true,
				RequiresConfirmation: true,
				Reason:               destructResult.BatchWarning,
			}
		}
	}

	// Check if tool requires confirmation (after permission checks pass).
	// Owner-level callers skip confirmation entirely — they are trusted.
	// For bash/exec, read-only commands (ls, cat, curl, etc.) also skip confirmation.
	requiresConfirmation := false
	if callerLevel != AccessOwner {
		for _, name := range g.cfg.RequireConfirmation {
			if name == toolName {
				requiresConfirmation = true
				break
			}
		}
		if requiresConfirmation && (toolName == "bash" || toolName == "exec") {
			command, _ := args["command"].(string)
			if isReadOnlyCommand(command) {
				requiresConfirmation = false
			}
		}
	}

	// 1. Check tool-level permission.
	permResult := g.checkToolPermission(toolName, callerLevel)
	if !permResult.Allowed {
		return permResult
	}

	// 2. For bash/exec, check command safety.
	if toolName == "bash" || toolName == "exec" {
		command, _ := args["command"].(string)
		if result := g.checkCommandSafety(command, callerLevel); !result.Allowed {
			return result
		}
	}

	// 3. For SSH, check host allowlist.
	if toolName == "ssh" || toolName == "scp" {
		host, _ := args["host"].(string)
		if host == "" {
			// For scp, extract host from source or destination.
			src, _ := args["source"].(string)
			dst, _ := args["destination"].(string)
			host = extractSSHHost(src)
			if host == "" {
				host = extractSSHHost(dst)
			}
		}
		if result := g.checkSSHHost(host); !result.Allowed {
			return result
		}
	}

	// 4. For file operations, check protected paths.
	if toolName == "read_file" || toolName == "write_file" || toolName == "edit_file" {
		path, _ := args["path"].(string)
		if result := g.checkPathSafety(path, callerLevel, toolName); !result.Allowed {
			return result
		}
	}

	return ToolCheckResult{Allowed: true, RequiresConfirmation: requiresConfirmation}
}

// SetSQLiteAudit configures a SQLite-backed audit logger. When set, audit
// records go to the database instead of the text file.
func (g *ToolGuard) SetSQLiteAudit(a *SQLiteAuditLogger) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sqliteAudit = a
}

// SQLiteAudit returns the SQLite audit logger (may be nil).
func (g *ToolGuard) SQLiteAudit() *SQLiteAuditLogger {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sqliteAudit
}

// AuditLog records a tool execution to the audit log.
func (g *ToolGuard) AuditLog(toolName string, callerJID string, callerLevel AccessLevel, args map[string]any, allowed bool, result string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Sanitize args for logging (remove large content).
	sanitizedArgs := make(map[string]any)
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > 200 {
			sanitizedArgs[k] = s[:200] + "...[truncated]"
		} else {
			sanitizedArgs[k] = v
		}
	}

	argsSummary := fmt.Sprintf("%v", sanitizedArgs)
	resultSummary := result
	if !allowed {
		resultSummary = "BLOCKED:" + result
	} else if len(resultSummary) > 200 {
		resultSummary = resultSummary[:200] + "...[truncated]"
	}

	entry := fmt.Sprintf("[%s] tool=%s caller=%s level=%s allowed=%v args=%s result=%s",
		time.Now().Format("2006-01-02 15:04:05"),
		toolName, callerJID, callerLevel, allowed, argsSummary, resultSummary)

	g.logger.Info("tool execution", "entry", entry)

	// Write to SQLite if configured, otherwise fall back to text file.
	if g.sqliteAudit != nil {
		g.sqliteAudit.Log(toolName, callerJID, string(callerLevel), allowed, argsSummary, resultSummary)
	} else if g.auditFile != nil {
		_, _ = g.auditFile.WriteString(entry + "\n")
	}
}

// Close closes the audit log file.
func (g *ToolGuard) Close() {
	if g.auditFile != nil {
		g.auditFile.Close()
	}
}

// UpdateConfig updates the tool guard config from hot-reload. Re-compiles
// dangerous patterns and protected paths. The audit log file is not changed
// (requires restart to change audit log path).
func (g *ToolGuard) UpdateConfig(cfg ToolGuardConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.cfg = cfg
	g.dangerousPatterns = nil
	g.defaultPatternCount = nil
	g.compileDangerousPatterns()
	g.initProtectedPaths()

	g.logger.Info("tool guard config hot-reloaded",
		"enabled", cfg.Enabled,
		"ssh_hosts", len(cfg.SSHAllowedHosts),
	)
}

// ---------- Internal checks ----------

// checkToolPermission verifies the caller has the required permission level.
func (g *ToolGuard) checkToolPermission(toolName string, callerLevel AccessLevel) ToolCheckResult {
	required := PermUser // Default: any user.

	if perm, ok := g.cfg.ToolPermissions[toolName]; ok {
		required = ToolPermission(perm)
	}

	if hasPermission(callerLevel, required) {
		return ToolCheckResult{Allowed: true}
	}

	return ToolCheckResult{
		Allowed: false,
		Reason:  fmt.Sprintf("tool '%s' requires %s access (you have %s)", toolName, required, callerLevel),
	}
}

// checkCommandSafety inspects a bash/exec command for dangerous patterns.
func (g *ToolGuard) checkCommandSafety(command string, callerLevel AccessLevel) ToolCheckResult {
	if command == "" {
		return ToolCheckResult{Allowed: true}
	}

	// --- Sudo check ---
	isSudo := strings.Contains(command, "sudo ") || strings.HasPrefix(command, "sudo")
	if isSudo {
		if g.cfg.AllowSudo {
			// AllowSudo: owner and admin can use sudo.
			if callerLevel != AccessOwner && callerLevel != AccessAdmin {
				return ToolCheckResult{
					Allowed: false,
					Reason:  "sudo commands require at least admin access",
				}
			}
		} else if g.cfg.BlockSudo {
			// Legacy BlockSudo: only owner can use.
			if callerLevel != AccessOwner {
				return ToolCheckResult{
					Allowed: false,
					Reason:  "sudo commands are disabled in config (allow_sudo: false)",
				}
			}
		}
	}

	// --- Reboot/shutdown check ---
	for _, kw := range []string{"shutdown", "reboot", "poweroff", "halt"} {
		if strings.Contains(command, kw) {
			if !g.cfg.AllowReboot {
				return ToolCheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("'%s' is blocked (allow_reboot: false in config)", kw),
				}
			}
			// Even if allowed, require owner.
			if callerLevel != AccessOwner {
				return ToolCheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("'%s' requires owner access", kw),
				}
			}
		}
	}

	// --- Destructive command patterns ---
	for i, pat := range g.dangerousPatterns {
		if pat.MatchString(command) {
			// If allow_destructive is on, owner is permitted.
			if g.cfg.AllowDestructive && callerLevel == AccessOwner {
				g.logger.Warn("destructive command allowed via config",
					"command", command,
					"pattern", pat.String(),
				)
				continue
			}
			// Custom patterns (appended after defaults) always block non-owner.
			if !g.cfg.AllowDestructive || callerLevel != AccessOwner {
				label := "safety rule"
				if i < len(g.defaultPatternCount) {
					label = "default safety rule"
				}
				return ToolCheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("command blocked by %s: %s (set allow_destructive: true to override)", label, pat.String()),
				}
			}
		}
	}

	return ToolCheckResult{Allowed: true}
}

// checkSSHHost verifies the host is in the allowlist (if configured).
func (g *ToolGuard) checkSSHHost(host string) ToolCheckResult {
	if len(g.cfg.SSHAllowedHosts) == 0 {
		// No allowlist = all hosts allowed.
		return ToolCheckResult{Allowed: true}
	}

	// Extract hostname (strip user@).
	if idx := strings.Index(host, "@"); idx >= 0 {
		host = host[idx+1:]
	}

	for _, allowed := range g.cfg.SSHAllowedHosts {
		if allowed == "*" {
			return ToolCheckResult{Allowed: true}
		}
		// Support wildcard subdomains: *.example.com.
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) || host == allowed[2:] {
				return ToolCheckResult{Allowed: true}
			}
		}
		if host == allowed {
			return ToolCheckResult{Allowed: true}
		}
	}

	return ToolCheckResult{
		Allowed: false,
		Reason:  fmt.Sprintf("SSH host '%s' not in allowed list. Configure security.ssh_allowed_hosts.", host),
	}
}

// checkPathSafety verifies the path is not protected.
func (g *ToolGuard) checkPathSafety(path string, callerLevel AccessLevel, toolName string) ToolCheckResult {
	if path == "" {
		return ToolCheckResult{Allowed: true}
	}

	// Owner has no path restrictions.
	if callerLevel == AccessOwner {
		return ToolCheckResult{Allowed: true}
	}

	// Resolve to absolute path.
	absPath := path
	if !filepath.IsAbs(path) {
		absPath, _ = filepath.Abs(path)
	}

	for _, protected := range g.protectedPaths {
		// Check exact match or prefix match.
		if absPath == protected || strings.HasPrefix(absPath, protected+"/") {
			// Allow reading some protected paths but not writing.
			if toolName == "read_file" && callerLevel == AccessAdmin {
				continue
			}
			return ToolCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("path '%s' is protected and requires owner access", path),
			}
		}

		// Glob match.
		if matched, _ := filepath.Match(protected, absPath); matched {
			return ToolCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("path '%s' matches protected pattern '%s'", path, protected),
			}
		}
	}

	return ToolCheckResult{Allowed: true}
}

// compileDangerousPatterns compiles the dangerous command regex patterns.
func (g *ToolGuard) compileDangerousPatterns() {
	// Default dangerous patterns (always compiled).
	// Note: shutdown/reboot/halt are handled separately by AllowReboot check.
	defaultPatterns := []string{
		`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/`, // rm -rf /
		`\bmkfs\b`,                              // format filesystem
		`\bdd\s+.*of=/dev/`,                     // dd to device
		`>\s*/dev/sd`,                           // overwrite device
		`\bchmod\s+(-R\s+)?777\s+/`,            // chmod 777 /
		`\bchown\s+(-R\s+)?.*\s+/`,             // chown / recursively
		`:\(\)\{\s*:\|:&\s*\};:`,               // fork bomb
		`\biptables\s+-F`,                       // flush firewall
		`\bufw\s+disable`,                       // disable firewall
		`\bpasswd\b`,                            // change password
		`\buserdel\b`,                           // delete user
		`\bgroupdel\b`,                          // delete group
		`DROP\s+DATABASE`,                       // drop database (SQL)
		`DROP\s+TABLE`,                          // drop table
		`TRUNCATE\s+TABLE`,                      // truncate table
	}

	// Compile default patterns.
	for _, p := range defaultPatterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			g.logger.Warn("invalid default dangerous pattern", "pattern", p, "error", err)
			continue
		}
		g.dangerousPatterns = append(g.dangerousPatterns, re)
		g.defaultPatternCount = append(g.defaultPatternCount, true)
	}

	// Compile custom patterns from config (appended after defaults).
	for _, p := range g.cfg.DangerousCommands {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			g.logger.Warn("invalid custom dangerous pattern", "pattern", p, "error", err)
			continue
		}
		g.dangerousPatterns = append(g.dangerousPatterns, re)
		g.defaultPatternCount = append(g.defaultPatternCount, false)
	}
}

// initProtectedPaths sets up the list of protected filesystem paths.
func (g *ToolGuard) initProtectedPaths() {
	g.protectedPaths = g.cfg.ProtectedPaths
	if len(g.protectedPaths) == 0 {
		home, _ := os.UserHomeDir()

		g.protectedPaths = []string{
			// SSH keys and config.
			filepath.Join(home, ".ssh"),
			// GPG keys.
			filepath.Join(home, ".gnupg"),
			// DevClaw secrets.
			filepath.Join(home, ".devclaw.vault"),
			".devclaw.vault",
			".env",
			// System sensitive paths.
			"/etc/shadow",
			"/etc/sudoers",
			"/etc/ssl/private",
			// Cloud credentials.
			filepath.Join(home, ".aws/credentials"),
			filepath.Join(home, ".config/gcloud"),
			filepath.Join(home, ".kube/config"),
			filepath.Join(home, ".docker/config.json"),
			// Browser data.
			filepath.Join(home, ".mozilla"),
			filepath.Join(home, ".config/google-chrome"),
		}
	}
}

// hasPermission checks if a caller's level meets the required permission.
func hasPermission(callerLevel AccessLevel, required ToolPermission) bool {
	switch required {
	case PermPublic:
		return true
	case PermUser:
		return callerLevel == AccessOwner || callerLevel == AccessAdmin || callerLevel == AccessUser
	case PermAdmin:
		return callerLevel == AccessOwner || callerLevel == AccessAdmin
	case PermOwner:
		return callerLevel == AccessOwner
	}
	return false
}

// extractSSHHost extracts the hostname from an scp-style path (user@host:/path).
func extractSSHHost(s string) string {
	if idx := strings.Index(s, ":"); idx > 0 {
		return s[:idx]
	}
	return ""
}

// safeCommandPrefixes lists command prefixes that are read-only / side-effect-free.
// If the first command in a pipeline starts with one of these, the bash call
// skips the RequireConfirmation prompt because it can't modify the system.
var safeCommandPrefixes = []string{
	"ls", "cat", "head", "tail", "less", "more",
	"grep", "rg", "awk", "sed", // sed is read-only when not using -i
	"wc", "sort", "uniq", "cut", "tr", "tee",
	"find", "which", "whereis", "type", "file",
	"echo", "printf", "date", "cal", "uptime",
	"whoami", "id", "hostname", "uname",
	"env", "printenv", "set",
	"pwd", "realpath", "dirname", "basename",
	"df", "du", "free", "top", "ps", "pgrep",
	"curl", "wget", "dig", "nslookup", "ping", "traceroute",
	"stat", "md5sum", "sha256sum", "sha1sum",
	"diff", "cmp", "comm",
	"jq", "yq", "python3 -c", "python -c", "node -e",
	"go version", "go env", "git status", "git log", "git diff", "git show", "git branch",
	"pm2 list", "pm2 status", "pm2 logs",
	"docker ps", "docker images", "docker logs",
	"tree", "wc",
}

// isReadOnlyCommand returns true if the bash command is read-only (no side effects).
// This is used to skip the RequireConfirmation prompt for safe commands.
func isReadOnlyCommand(command string) bool {
	if command == "" {
		return false
	}

	// Normalize: trim whitespace and handle common prefixes.
	cmd := strings.TrimSpace(command)

	// If the command contains write-indicating operators, it's not read-only.
	// Check for output redirection (>, >>), but allow pipes (|).
	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		// Skip content inside single/double quotes.
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			for i < len(cmd) && cmd[i] != quote {
				if cmd[i] == '\\' {
					i++ // skip escaped char
				}
				i++
			}
			continue
		}
		// Output redirection → not read-only.
		if ch == '>' {
			return false
		}
	}

	// Split by pipe and check the first command (the one that "does" the work).
	// Also split by && and ; to check all commands in a chain.
	parts := splitCommandChain(cmd)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !matchesSafePrefix(part) {
			return false
		}
	}

	return true
}

// splitCommandChain splits a command string by &&, ;, and || operators.
// Pipe (|) is NOT a split point because piped commands form a single pipeline.
func splitCommandChain(cmd string) []string {
	var parts []string
	var current strings.Builder
	inQuote := byte(0)

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]

		// Track quotes.
		if inQuote != 0 {
			current.WriteByte(ch)
			if ch == inQuote && (i == 0 || cmd[i-1] != '\\') {
				inQuote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inQuote = ch
			current.WriteByte(ch)
			continue
		}

		// Split on && or ||.
		if i < len(cmd)-1 && ((ch == '&' && cmd[i+1] == '&') || (ch == '|' && cmd[i+1] == '|')) {
			parts = append(parts, current.String())
			current.Reset()
			i++ // skip second char
			continue
		}
		// Split on ;.
		if ch == ';' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// matchesSafePrefix checks if a command starts with a known read-only prefix.
func matchesSafePrefix(cmd string) bool {
	cmd = strings.TrimSpace(cmd)

	// Strip leading env vars (e.g. "VAR=val command").
	for {
		if idx := strings.Index(cmd, "="); idx > 0 && idx < strings.Index(cmd+" ", " ") {
			// This looks like VAR=val, skip it.
			rest := cmd[idx+1:]
			spaceIdx := strings.IndexByte(rest, ' ')
			if spaceIdx < 0 {
				break // VAR=val with no command after
			}
			cmd = strings.TrimSpace(rest[spaceIdx:])
		} else {
			break
		}
	}

	// Pipe: for each segment in a pipeline, the last consumer determines safety,
	// but since we split by &&/; we get full pipelines. Each segment in a pipe
	// is read-only if the first command is read-only, so check just the first.
	pipeSegments := strings.SplitN(cmd, "|", 2)
	first := strings.TrimSpace(pipeSegments[0])

	for _, prefix := range safeCommandPrefixes {
		if first == prefix ||
			strings.HasPrefix(first, prefix+" ") ||
			strings.HasPrefix(first, prefix+"\t") {
			return true
		}
	}
	return false
}
