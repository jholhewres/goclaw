package copilot

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltInProfiles_Exist(t *testing.T) {
	// Verify all built-in profiles exist.
	expectedProfiles := []string{"minimal", "coding", "messaging", "team", "full"}

	for _, name := range expectedProfiles {
		profile, ok := BuiltInProfiles[name]
		if !ok {
			t.Errorf("expected built-in profile '%s' to exist", name)
			continue
		}
		if profile.Name != name {
			t.Errorf("profile '%s' has wrong Name field: %s", name, profile.Name)
		}
		if profile.Description == "" {
			t.Errorf("profile '%s' has empty description", name)
		}
	}
}

func TestBuiltInProfiles_Minimal(t *testing.T) {
	profile := BuiltInProfiles["minimal"]

	// Minimal should allow web and memory groups.
	allowMap := make(map[string]bool)
	for _, item := range profile.Allow {
		allowMap[item] = true
	}

	if !allowMap["group:web"] {
		t.Error("minimal profile should allow group:web")
	}
	if !allowMap["group:memory"] {
		t.Error("minimal profile should allow group:memory")
	}

	// Minimal should deny runtime and write operations.
	denyMap := make(map[string]bool)
	for _, item := range profile.Deny {
		denyMap[item] = true
	}

	if !denyMap["group:runtime"] {
		t.Error("minimal profile should deny group:runtime")
	}
	if !denyMap["write_file"] {
		t.Error("minimal profile should deny write_file")
	}
}

func TestBuiltInProfiles_Coding(t *testing.T) {
	profile := BuiltInProfiles["coding"]

	// Coding should allow fs, web, memory, bash, git, docker.
	allowMap := make(map[string]bool)
	for _, item := range profile.Allow {
		allowMap[item] = true
	}

	if !allowMap["group:fs"] {
		t.Error("coding profile should allow group:fs")
	}
	if !allowMap["bash"] {
		t.Error("coding profile should allow bash")
	}
	if !allowMap["git_*"] {
		t.Error("coding profile should allow git_*")
	}

	// Coding should deny ssh, scp.
	denyMap := make(map[string]bool)
	for _, item := range profile.Deny {
		denyMap[item] = true
	}

	if !denyMap["ssh"] {
		t.Error("coding profile should deny ssh")
	}
	if !denyMap["scp"] {
		t.Error("coding profile should deny scp")
	}
}

func TestBuiltInProfiles_Full(t *testing.T) {
	profile := BuiltInProfiles["full"]

	// Full profile should allow all tools.
	if len(profile.Allow) != 1 || profile.Allow[0] != "*" {
		t.Errorf("full profile should allow '*', got %v", profile.Allow)
	}

	// Full profile should have empty deny list.
	if len(profile.Deny) != 0 {
		t.Errorf("full profile should have empty deny list, got %v", profile.Deny)
	}
}

func TestResolveProfile_BuiltIn(t *testing.T) {
	allow, deny := ResolveProfile("coding", nil)

	if len(allow) == 0 {
		t.Error("coding profile should have allow list")
	}
	if len(deny) == 0 {
		t.Error("coding profile should have deny list")
	}
}

func TestResolveProfile_Custom(t *testing.T) {
	customProfiles := map[string]ToolProfile{
		"custom": {
			Name:        "custom",
			Description: "Custom test profile",
			Allow:       []string{"read_file"},
			Deny:        []string{"bash"},
		},
	}

	allow, deny := ResolveProfile("custom", customProfiles)

	if len(allow) != 1 || allow[0] != "read_file" {
		t.Errorf("expected allow ['read_file'], got %v", allow)
	}
	if len(deny) != 1 || deny[0] != "bash" {
		t.Errorf("expected deny ['bash'], got %v", deny)
	}
}

func TestResolveProfile_NotFound(t *testing.T) {
	allow, deny := ResolveProfile("nonexistent", nil)

	if allow != nil {
		t.Errorf("expected nil allow for nonexistent profile, got %v", allow)
	}
	if deny != nil {
		t.Errorf("expected nil deny for nonexistent profile, got %v", deny)
	}
}

func TestGetProfile_BuiltIn(t *testing.T) {
	profile := GetProfile("minimal", nil)

	if profile == nil {
		t.Fatal("expected profile, got nil")
	}
	if profile.Name != "minimal" {
		t.Errorf("expected name 'minimal', got %s", profile.Name)
	}
}

func TestGetProfile_Custom(t *testing.T) {
	customProfiles := map[string]ToolProfile{
		"test": {
			Name:        "test",
			Description: "Test profile",
			Allow:       []string{"*"},
			Deny:        []string{},
		},
	}

	profile := GetProfile("test", customProfiles)

	if profile == nil {
		t.Fatal("expected profile, got nil")
	}
	if profile.Name != "test" {
		t.Errorf("expected name 'test', got %s", profile.Name)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	profile := GetProfile("nonexistent", nil)

	if profile != nil {
		t.Errorf("expected nil for nonexistent profile, got %v", profile)
	}
}

func TestListProfiles(t *testing.T) {
	customProfiles := map[string]ToolProfile{
		"custom1": {Name: "custom1"},
		"custom2": {Name: "custom2"},
	}

	profiles := ListProfiles(customProfiles)

	// Should have 5 built-in + 2 custom = 7 profiles.
	if len(profiles) != 7 {
		t.Errorf("expected 7 profiles, got %d: %v", len(profiles), profiles)
	}

	// Verify built-in profiles are included.
	profileMap := make(map[string]bool)
	for _, p := range profiles {
		profileMap[p] = true
	}

	for _, builtIn := range []string{"minimal", "coding", "messaging", "team", "full"} {
		if !profileMap[builtIn] {
			t.Errorf("expected built-in profile '%s' in list", builtIn)
		}
	}
}

func TestExpandProfileList_Wildcard(t *testing.T) {
	allTools := []string{"git_status", "git_commit", "git_push", "bash", "read_file"}

	items := []string{"git_*"}
	expanded := ExpandProfileList(items, allTools)

	if len(expanded) != 3 {
		t.Errorf("expected 3 expanded items for 'git_*', got %d: %v", len(expanded), expanded)
	}

	expandedMap := make(map[string]bool)
	for _, t := range expanded {
		expandedMap[t] = true
	}

	if !expandedMap["git_status"] {
		t.Error("expected git_status to be expanded")
	}
	if !expandedMap["git_commit"] {
		t.Error("expected git_commit to be expanded")
	}
	if !expandedMap["git_push"] {
		t.Error("expected git_push to be expanded")
	}
}

func TestExpandProfileList_Group(t *testing.T) {
	items := []string{"group:memory"}
	expanded := ExpandProfileList(items, nil)

	// Should expand to memory tools.
	if len(expanded) == 0 {
		t.Error("expected group:memory to expand to tools")
	}

	expandedMap := make(map[string]bool)
	for _, t := range expanded {
		expandedMap[t] = true
	}

	// Verify memory tool is included.
	if !expandedMap["memory"] {
		t.Error("expected memory from group:memory")
	}
}

func TestExpandProfileList_AllTools(t *testing.T) {
	allTools := []string{"bash", "read_file", "write_file"}

	items := []string{"*"}
	expanded := ExpandProfileList(items, allTools)

	if len(expanded) != 3 {
		t.Errorf("expected 3 expanded items for '*', got %d", len(expanded))
	}
}

func TestExpandProfileList_DirectToolName(t *testing.T) {
	items := []string{"bash", "read_file"}
	expanded := ExpandProfileList(items, nil)

	if len(expanded) != 2 {
		t.Errorf("expected 2 items, got %d", len(expanded))
	}
	if expanded[0] != "bash" {
		t.Errorf("expected first item 'bash', got %s", expanded[0])
	}
	if expanded[1] != "read_file" {
		t.Errorf("expected second item 'read_file', got %s", expanded[1])
	}
}

func TestProfileChecker_IsDenied(t *testing.T) {
	allTools := []string{"bash", "ssh", "read_file", "write_file"}

	allow := []string{"bash", "read_file"}
	deny := []string{"ssh"}

	checker := NewProfileChecker(allow, deny, allTools)

	if !checker.IsDenied("ssh") {
		t.Error("ssh should be denied")
	}
	if checker.IsDenied("bash") {
		t.Error("bash should not be denied")
	}
	if checker.IsDenied("read_file") {
		t.Error("read_file should not be denied")
	}
}

func TestProfileChecker_IsAllowed(t *testing.T) {
	allTools := []string{"bash", "ssh", "read_file", "write_file"}

	allow := []string{"bash", "read_file"}
	deny := []string{}

	checker := NewProfileChecker(allow, deny, allTools)

	if !checker.IsAllowed("bash") {
		t.Error("bash should be allowed")
	}
	if !checker.IsAllowed("read_file") {
		t.Error("read_file should be allowed")
	}
	if checker.IsAllowed("ssh") {
		t.Error("ssh should not be allowed (not in allow list)")
	}
}

func TestProfileChecker_IsAllowed_EmptyAllowList(t *testing.T) {
	allTools := []string{"bash", "ssh"}

	// Empty allow list = all allowed.
	checker := NewProfileChecker(nil, nil, allTools)

	if !checker.IsAllowed("bash") {
		t.Error("empty allow list should allow all tools")
	}
	if !checker.IsAllowed("ssh") {
		t.Error("empty allow list should allow all tools")
	}
}

func TestProfileChecker_Check_DenyTakesPrecedence(t *testing.T) {
	allTools := []string{"bash"}

	// Tool is in both allow and deny - deny wins.
	allow := []string{"bash"}
	deny := []string{"bash"}

	checker := NewProfileChecker(allow, deny, allTools)

	allowed, reason := checker.Check("bash")

	if allowed {
		t.Error("bash should be denied (deny takes precedence)")
	}
	if reason != "denied by profile" {
		t.Errorf("expected 'denied by profile', got %s", reason)
	}
}

func TestProfileChecker_Check_Allowed(t *testing.T) {
	allTools := []string{"bash", "read_file"}

	allow := []string{"bash", "read_file"}
	deny := []string{}

	checker := NewProfileChecker(allow, deny, allTools)

	allowed, reason := checker.Check("bash")

	if !allowed {
		t.Errorf("bash should be allowed, reason: %s", reason)
	}
}

func TestProfileChecker_Check_NotAllowed(t *testing.T) {
	allTools := []string{"bash", "ssh"}

	allow := []string{"bash"}
	deny := []string{}

	checker := NewProfileChecker(allow, deny, allTools)

	allowed, reason := checker.Check("ssh")

	if allowed {
		t.Error("ssh should not be allowed")
	}
	if reason != "not in profile allow list" {
		t.Errorf("expected 'not in profile allow list', got %s", reason)
	}
}

func TestMatchesPattern_Exact(t *testing.T) {
	if !MatchesPattern("bash", "bash") {
		t.Error("exact match should return true")
	}
	if MatchesPattern("bash", "bash_extra") {
		t.Error("different tool should not match")
	}
}

func TestMatchesPattern_Wildcard(t *testing.T) {
	if !MatchesPattern("git_status", "git_*") {
		t.Error("git_status should match git_*")
	}
	if !MatchesPattern("git_commit", "git_*") {
		t.Error("git_commit should match git_*")
	}
	if MatchesPattern("bash", "git_*") {
		t.Error("bash should not match git_*")
	}
}

func TestMatchesPattern_Glob(t *testing.T) {
	// Test simple glob patterns.
	if !MatchesPattern("test_bash", "test_*") {
		t.Error("test_bash should match test_*")
	}
}

// ========== Helper function to create ToolDefinition for tests ==========

func makeTestTool(name, description string) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        name,
			Description: description,
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}
}

// ========== Tests for InferToolCategory ==========

func TestInferToolCategory(t *testing.T) {
	tests := []struct {
		toolName string
		expected string
	}{
		// Filesystem
		{"read_file", "Filesystem"},
		{"write_file", "Filesystem"},
		{"edit_file", "Filesystem"},
		{"list_files", "Filesystem"},
		{"glob_files", "Filesystem"},
		{"search_files", "Filesystem"},

		// Execution
		{"bash", "Execution"},
		{"exec", "Execution"},
		{"ssh", "Execution"},
		{"scp", "Execution"},
		{"set_env", "Execution"},

		// Web
		{"web_search", "Web"},
		{"web_fetch", "Web"},

		// Memory
		{"memory", "Memory"},

		// Scheduling
		{"cron_add", "Scheduling"},
		{"cron_list", "Scheduling"},
		{"cron_remove", "Scheduling"},

		// Vault
		{"vault_save", "Vault"},
		{"vault_get", "Vault"},
		{"vault_list", "Vault"},
		{"vault_delete", "Vault"},

		// Agents
		{"sessions_list", "Agents"},
		{"sessions_send", "Agents"},
		{"spawn_subagent", "Agents"},
		{"list_subagents", "Agents"},

		// Git
		{"git_status", "Git"},
		{"git_commit", "Git"},
		{"git", "Git"},

		// Containers
		{"docker_ps", "Containers"},
		{"docker_run", "Containers"},
		{"kubectl_get", "Containers"},
		{"kubernetes_deploy", "Containers"},

		// Cloud
		{"aws_s3_ls", "Cloud"},
		{"gcloud_compute", "Cloud"},
		{"azure_vm", "Cloud"},
		{"terraform_apply", "Cloud"},

		// Team
		{"team_manage", "Team"},
		{"team_agent", "Team"},
		{"team_task", "Team"},

		// Skills
		{"install_skill", "Skills"},
		{"list_skills", "Skills"},
		{"search_skills", "Skills"},

		// Media
		{"describe_image", "Media"},
		{"transcribe_audio", "Media"},

		// Capabilities
		{"list_capabilities", "Capabilities"},

		// Other
		{"unknown_tool", "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := InferToolCategory(tt.toolName)
			if result != tt.expected {
				t.Errorf("InferToolCategory(%q) = %q, want %q", tt.toolName, result, tt.expected)
			}
		})
	}
}

// ========== Tests for CategorizeTools ==========

func TestCategorizeTools(t *testing.T) {
	tools := []ToolDefinition{
		makeTestTool("read_file", "Read file"),
		makeTestTool("bash", "Run command"),
		makeTestTool("web_search", "Search web"),
		makeTestTool("memory", "Memory operations"),
	}

	result := CategorizeTools(tools)

	// Verify categories exist
	if len(result["Filesystem"]) == 0 {
		t.Error("Expected Filesystem category to have tools")
	}
	if len(result["Execution"]) == 0 {
		t.Error("Expected Execution category to have tools")
	}
	if len(result["Web"]) == 0 {
		t.Error("Expected Web category to have tools")
	}
	if len(result["Memory"]) == 0 {
		t.Error("Expected Memory category to have tools")
	}
}

// ========== Tests for CategorizeToolNames ==========

func TestCategorizeToolNames(t *testing.T) {
	names := []string{"read_file", "write_file", "bash", "web_search"}

	result := CategorizeToolNames(names)

	if len(result["Filesystem"]) != 2 {
		t.Errorf("Expected 2 Filesystem tools, got %d", len(result["Filesystem"]))
	}
	if len(result["Execution"]) != 1 {
		t.Errorf("Expected 1 Execution tool, got %d", len(result["Execution"]))
	}
	if len(result["Web"]) != 1 {
		t.Errorf("Expected 1 Web tool, got %d", len(result["Web"]))
	}
}

// ========== Tests for FormatToolsForPrompt ==========

func TestFormatToolsForPrompt(t *testing.T) {
	tools := []ToolDefinition{
		makeTestTool("read_file", "Read file contents from disk"),
		makeTestTool("bash", "Run shell commands"),
	}

	result := FormatToolsForPrompt(tools, 60)

	// Verify structure
	if !strings.Contains(result, "### Filesystem") {
		t.Error("Expected Filesystem category header")
	}
	if !strings.Contains(result, "### Execution") {
		t.Error("Expected Execution category header")
	}
	if !strings.Contains(result, "read_file:") {
		t.Error("Expected read_file in output")
	}
	if !strings.Contains(result, "bash:") {
		t.Error("Expected bash in output")
	}
}

func TestFormatToolsForPrompt_Truncation(t *testing.T) {
	longDesc := "This is a very long description that should be truncated because it exceeds the maximum description length that we set for the prompt output and it keeps going on and on"
	tools := []ToolDefinition{
		makeTestTool("long_tool", longDesc),
	}

	result := FormatToolsForPrompt(tools, 30)

	// Verify truncation occurred
	if !strings.Contains(result, "...") && strings.Contains(result, longDesc) {
		t.Error("Expected description to be truncated")
	}
}

// ========== Tests for FormatToolNamesForPrompt ==========

func TestFormatToolNamesForPrompt(t *testing.T) {
	names := []string{"read_file", "bash", "web_search"}

	result := FormatToolNamesForPrompt(names)

	if !strings.Contains(result, "### Filesystem") {
		t.Error("Expected Filesystem category header")
	}
	if !strings.Contains(result, "### Execution") {
		t.Error("Expected Execution category header")
	}
	if !strings.Contains(result, "### Web") {
		t.Error("Expected Web category header")
	}
}
