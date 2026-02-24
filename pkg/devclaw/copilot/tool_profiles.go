// Package copilot – tool_profiles.go implements predefined tool permission profiles.
// Profiles simplify tool configuration by providing presets for common use cases.
package copilot

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ToolProfile defines a preset of allowed and denied tools.
type ToolProfile struct {
	// Name is the profile identifier (e.g., "minimal", "coding", "full").
	Name string `yaml:"name"`

	// Description explains what this profile is for.
	Description string `yaml:"description"`

	// Allow lists tools and groups that are permitted.
	// Supports: tool names, "group:name", wildcards like "git_*"
	// Empty means no allow list (use permission levels).
	Allow []string `yaml:"allow"`

	// Deny lists tools and groups that are always blocked.
	// Takes precedence over Allow.
	Deny []string `yaml:"deny"`
}

// BuiltInProfiles provides predefined tool profiles for common use cases.
var BuiltInProfiles = map[string]ToolProfile{
	"minimal": {
		Name:        "minimal",
		Description: "Basic queries only - read-only access",
		Allow: []string{
			"group:web",
			"group:memory",
			"read_file",
			"list_files",
			"search_files",
			"glob_files",
		},
		Deny: []string{
			"group:runtime",
			"write_file",
			"edit_file",
			"group:skills",
			"group:scheduler",
			"group:vault",
			"group:subagents",
		},
	},
	"coding": {
		Name:        "coding",
		Description: "Software development - file access, git, docker, tests, reminders",
		Allow: []string{
			"group:fs",
			"group:web",
			"group:memory",
			"group:scheduler",
			"group:vault",
			"bash",
			"exec",
			"git_*",
			"docker_*",
			"test_*",
		},
		Deny: []string{
			"ssh",
			"scp",
		},
	},
	"messaging": {
		Name:        "messaging",
		Description: "Chat channel usage - web search, memory, reminders, skills, and vault",
		Allow: []string{
			"group:web",
			"group:memory",
			"group:scheduler",
			"group:vault",
			"group:skills",
			"list_skills",
			"search_skills",
		},
		Deny: []string{
			"group:runtime",
			"group:fs",
			"group:subagents",
			"bash",
			"exec",
		},
	},
	"team": {
		Name:        "team",
		Description: "Team agent - team tools, web, memory, scheduler, vault, skills",
		Allow: []string{
			"group:teams",
			"group:web",
			"group:memory",
			"group:scheduler",
			"group:vault",
			"group:skills",
			"read_file",
			"list_files",
			"search_files",
			"glob_files",
			"bash",
		},
		Deny: []string{
			"group:subagents",
			"ssh",
			"scp",
			"exec",
			"write_file",
			"edit_file",
		},
	},
	"full": {
		Name:        "full",
		Description: "Full access - all tools available (respect permissions)",
		Allow:       []string{"*"},
		Deny:        []string{},
	},
}

// ResolveProfile returns the allow and deny lists for a profile.
// Checks built-in profiles first, then custom profiles.
// Returns nil lists if profile not found.
func ResolveProfile(name string, customProfiles map[string]ToolProfile) (allow, deny []string) {
	// Check built-in profiles first
	if profile, ok := BuiltInProfiles[name]; ok {
		return profile.Allow, profile.Deny
	}

	// Check custom profiles
	if customProfiles != nil {
		if profile, ok := customProfiles[name]; ok {
			return profile.Allow, profile.Deny
		}
	}

	return nil, nil
}

// GetProfile returns a profile by name (built-in or custom).
func GetProfile(name string, customProfiles map[string]ToolProfile) *ToolProfile {
	if profile, ok := BuiltInProfiles[name]; ok {
		return &profile
	}
	if customProfiles != nil {
		if profile, ok := customProfiles[name]; ok {
			profile := profile // copy
			return &profile
		}
	}
	return nil
}

// ListProfiles returns all available profile names.
func ListProfiles(customProfiles map[string]ToolProfile) []string {
	names := make([]string, 0, len(BuiltInProfiles)+len(customProfiles))

	// Add built-in profiles
	for name := range BuiltInProfiles {
		names = append(names, name)
	}

	// Add custom profiles
	for name := range customProfiles {
		names = append(names, name)
	}

	return names
}

// ExpandProfileList expands a profile's allow/deny lists into tool names.
// Handles groups ("group:name") and wildcards ("git_*").
func ExpandProfileList(items []string, allTools []string) []string {
	var result []string

	for _, item := range items {
		// Wildcard pattern
		if strings.HasSuffix(item, "*") {
			prefix := strings.TrimSuffix(item, "*")
			for _, tool := range allTools {
				if strings.HasPrefix(tool, prefix) {
					result = append(result, tool)
				}
			}
			continue
		}

		// Group reference
		if strings.HasPrefix(item, "group:") {
			if tools, ok := ToolGroups[item]; ok {
				result = append(result, tools...)
			}
			continue
		}

		// Special case: "*" means all tools
		if item == "*" {
			result = append(result, allTools...)
			continue
		}

		// Direct tool name
		result = append(result, item)
	}

	return result
}

// ProfileChecker checks if tools are allowed/denied by a profile.
type ProfileChecker struct {
	allowSet map[string]bool
	denySet  map[string]bool
}

// NewProfileChecker creates a checker from allow/deny lists.
func NewProfileChecker(allow, deny []string, allTools []string) *ProfileChecker {
	pc := &ProfileChecker{
		allowSet: make(map[string]bool),
		denySet:  make(map[string]bool),
	}

	// Expand and populate deny set (deny takes precedence)
	expandedDeny := ExpandProfileList(deny, allTools)
	for _, tool := range expandedDeny {
		pc.denySet[tool] = true
	}

	// Expand and populate allow set
	expandedAllow := ExpandProfileList(allow, allTools)
	for _, tool := range expandedAllow {
		pc.allowSet[tool] = true
	}

	return pc
}

// IsDenied returns true if the tool is in the deny list.
func (pc *ProfileChecker) IsDenied(toolName string) bool {
	return pc.denySet[toolName]
}

// IsAllowed returns true if the tool is in the allow list.
// If allow list is empty, all tools are allowed (respecting deny).
func (pc *ProfileChecker) IsAllowed(toolName string) bool {
	// Empty allow list = all allowed
	if len(pc.allowSet) == 0 {
		return true
	}
	return pc.allowSet[toolName]
}

// Check returns whether a tool is permitted by the profile.
// Returns (allowed, reason) where reason explains why if not allowed.
func (pc *ProfileChecker) Check(toolName string) (allowed bool, reason string) {
	// Check deny first (takes precedence)
	if pc.IsDenied(toolName) {
		return false, "denied by profile"
	}

	// Check allow
	if !pc.IsAllowed(toolName) {
		return false, "not in profile allow list"
	}

	return true, ""
}

// MatchesPattern checks if a tool name matches a pattern.
// Supports glob-style wildcards: "git_*" matches "git_status", "git_commit", etc.
func MatchesPattern(toolName, pattern string) bool {
	// Exact match
	if pattern == toolName {
		return true
	}

	// Wildcard suffix
	if prefix, found := strings.CutSuffix(pattern, "*"); found {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}

	// Glob pattern (simple implementation)
	matched, err := filepath.Match(pattern, toolName)
	if err != nil {
		return false
	}
	return matched
}

// ---------- Tool Categorization for Prompt ----------

// InferToolCategory determines the category of a tool from its name.
// Used for grouping tools in the system prompt and list_capabilities output.
func InferToolCategory(name string) string {
	switch {
	// Filesystem operations
	case strings.Contains(name, "read") ||
		strings.Contains(name, "write") ||
		strings.Contains(name, "edit") ||
		strings.Contains(name, "list_files") ||
		strings.Contains(name, "glob") ||
		strings.Contains(name, "search_files"):
		return "Filesystem"

	// Shell/execution
	case name == "bash" ||
		name == "exec" ||
		name == "ssh" ||
		name == "scp" ||
		name == "set_env":
		return "Execution"

	// Web operations
	case strings.Contains(name, "web_") ||
		strings.Contains(name, "fetch"):
		return "Web"

	// Memory/knowledge
	case strings.Contains(name, "memory"):
		return "Memory"

	// Scheduling
	case strings.HasPrefix(name, "cron_"):
		return "Scheduling"

	// Vault/secrets
	case strings.HasPrefix(name, "vault_"):
		return "Vault"

	// Sessions/agents
	case strings.HasPrefix(name, "sessions_") ||
		strings.Contains(name, "subagent"):
		return "Agents"

	// Git/version control
	case strings.Contains(name, "git_") ||
		name == "git":
		return "Git"

	// Docker/containers
	case strings.Contains(name, "docker") ||
		strings.Contains(name, "kubectl") ||
		strings.Contains(name, "kubernetes"):
		return "Containers"

	// Cloud/infrastructure
	case strings.Contains(name, "aws_") ||
		strings.Contains(name, "gcloud_") ||
		strings.Contains(name, "azure_") ||
		strings.Contains(name, "terraform"):
		return "Cloud"

	// Development tools
	case strings.Contains(name, "claude-code") ||
		strings.Contains(name, "test") ||
		strings.Contains(name, "debug"):
		return "Development"

	// Team tools
	case strings.HasPrefix(name, "team_"):
		return "Team"

	// Skills management
	case strings.HasSuffix(name, "_skill") ||
		strings.Contains(name, "skill"):
		return "Skills"

	// Media
	case strings.Contains(name, "image") ||
		strings.Contains(name, "audio") ||
		strings.Contains(name, "video") ||
		strings.Contains(name, "transcribe"):
		return "Media"

	// Capabilities
	case name == "list_capabilities":
		return "Capabilities"

	default:
		return "Other"
	}
}

// CategorizeTools groups tool definitions by category for display purposes.
func CategorizeTools(tools []ToolDefinition) map[string][]ToolDefinition {
	categories := make(map[string][]ToolDefinition)
	for _, tool := range tools {
		cat := InferToolCategory(tool.Function.Name)
		categories[cat] = append(categories[cat], tool)
	}
	return categories
}

// CategorizeToolNames groups tool names by category.
func CategorizeToolNames(names []string) map[string][]string {
	categories := make(map[string][]string)
	for _, name := range names {
		cat := InferToolCategory(name)
		categories[cat] = append(categories[cat], name)
	}
	return categories
}

// FormatToolsForPrompt formats tools as a compact list for the system prompt.
// Groups by category and truncates descriptions to fit within budget.
func FormatToolsForPrompt(tools []ToolDefinition, maxDescLen int) string {
	categories := CategorizeTools(tools)

	// Sort categories for consistent output
	var cats []string
	for cat := range categories {
		cats = append(cats, cat)
	}
	// Sort alphabetically
	for i := 0; i < len(cats); i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[j] < cats[i] {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}

	var b strings.Builder
	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("\n### %s\n", cat))
		for _, tool := range categories[cat] {
			desc := tool.Function.Description
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}
			// Clean up description (remove newlines)
			desc = strings.ReplaceAll(desc, "\n", " ")
			b.WriteString(fmt.Sprintf("- %s: %s\n", tool.Function.Name, desc))
		}
	}

	return b.String()
}

// FormatToolNamesForPrompt formats tool names as a compact list for the system prompt.
// Groups by category for better readability.
func FormatToolNamesForPrompt(names []string) string {
	categories := CategorizeToolNames(names)

	// Sort categories
	var cats []string
	for cat := range categories {
		cats = append(cats, cat)
	}
	for i := 0; i < len(cats); i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[j] < cats[i] {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}

	var b strings.Builder
	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("\n### %s\n", cat))
		for _, name := range categories[cat] {
			b.WriteString(fmt.Sprintf("- %s\n", name))
		}
	}

	return b.String()
}
