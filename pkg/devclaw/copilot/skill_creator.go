// Package copilot – skill_creator.go implements tools that allow the agent
// to create, edit, and manage skills via chat. Skills are created as
// ClawdHub-compatible SKILL.md files in the workspace skills directory.
//
// The agent can use these tools to:
//   - Initialize a new skill with a SKILL.md template
//   - Edit an existing skill's instructions
//   - Add scripts (Python, Node, Shell) to a skill
//   - List installed skills
//   - Test a skill by executing it
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
)

// RegisterSkillCreatorTools registers a single "skill_manage" dispatcher tool
// that consolidates init, edit, add_script, list, test, install, search,
// defaults_list, defaults_install, remove actions.
func RegisterSkillCreatorTools(executor *ToolExecutor, registry *skills.Registry, skillsDir string, skillDB *SkillDB, logger *slog.Logger) {
	if skillsDir == "" {
		skillsDir = "./skills"
	}
	if logger == nil {
		logger = slog.Default()
	}

	installer := skills.NewInstaller(skillsDir, logger)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"init", "edit", "add_script", "list", "test", "install", "search", "defaults_list", "defaults_install", "remove"},
				"description": "Action: init (create skill), edit (modify SKILL.md), add_script, list, test, install (from ClawHub/GitHub/URL), search (ClawHub), defaults_list, defaults_install, remove",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name (for init/edit/test/remove)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Skill description (for init)",
			},
			"instructions": map[string]any{
				"type":        "string",
				"description": "Agent instructions markdown (for init)",
			},
			"emoji": map[string]any{
				"type":        "string",
				"description": "Skill emoji (for init)",
			},
			"with_database": map[string]any{
				"type":        "boolean",
				"description": "Create database table (for init)",
			},
			"database_table": map[string]any{
				"type":        "string",
				"description": "Database table name (for init, default: 'data')",
			},
			"database_schema": map[string]any{
				"type":        "object",
				"description": "Column definitions (for init with_database)",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"content": map[string]any{
				"type":        "string",
				"description": "New SKILL.md content (for edit) or script source (for add_script)",
			},
			"skill_name": map[string]any{
				"type":        "string",
				"description": "Target skill (for add_script)",
			},
			"script_name": map[string]any{
				"type":        "string",
				"description": "Script filename (for add_script)",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Test input (for test)",
			},
			"source": map[string]any{
				"type":        "string",
				"description": "Install source: ClawHub slug, GitHub URL, or local path (for install)",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for search)",
			},
			"names": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Skill names to install (for defaults_install). Use [\"all\"] for all.",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("skill_manage",
			"Manage skills: init, edit, add_script, list, test, install, search, defaults_list, defaults_install, remove.",
			schema),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			// Write actions require admin level (they create files/directories).
			// Read-only actions (list, search, test, defaults_list) are safe for any user.
			if isSkillWriteAction(action) {
				level := CallerLevelFromContext(ctx)
				if level != AccessOwner && level != AccessAdmin {
					return nil, fmt.Errorf("action %q requires admin access (current: %s). Ask an admin to perform this action", action, level)
				}
			}

			switch action {
			case "init":
				return handleSkillInit(registry, skillsDir, skillDB, args)
			case "edit":
				return handleSkillEdit(skillsDir, args)
			case "add_script":
				return handleSkillAddScript(skillsDir, args)
			case "list":
				return handleSkillList(registry, skillsDir)
			case "test":
				return handleSkillTest(ctx, registry, args)
			case "install":
				return handleSkillInstall(ctx, installer, registry, args)
			case "search":
				return handleSkillSearch(args)
			case "defaults_list":
				return handleSkillDefaultsList(skillsDir)
			case "defaults_install":
				return handleSkillDefaultsInstall(ctx, registry, skillsDir, args)
			case "remove":
				return handleSkillRemove(registry, skillsDir, args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func handleSkillInit(registry *skills.Registry, skillsDir string, skillDB *SkillDB, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	instructions, _ := args["instructions"].(string)
	emoji, _ := args["emoji"].(string)
	withDatabase, _ := args["with_database"].(bool)
	databaseTable, _ := args["database_table"].(string)
	databaseSchemaRaw, _ := args["database_schema"].(map[string]any)

	if name == "" || description == "" {
		return nil, fmt.Errorf("name and description are required for init action")
	}

	displayName := name
	name = sanitizeSkillName(name)
	dbSkillName := strings.ReplaceAll(name, "-", "_")

	if withDatabase && skillDB != nil {
		existingTables, err := skillDB.ListTables("")
		if err == nil {
			for _, t := range existingTables {
				if t.SkillName == dbSkillName {
					existingSkillName := strings.ReplaceAll(t.SkillName, "_", "-")
					return nil, fmt.Errorf("skill name collision: '%s' would conflict with existing skill '%s' in database", name, existingSkillName)
				}
			}
		}
	}

	if _, exists := registry.Get(name); exists {
		return nil, fmt.Errorf("skill '%s' already exists. Use action=edit to modify it", name)
	}

	skillDir := filepath.Join(skillsDir, name)
	if _, err := os.Stat(skillDir); err == nil {
		return nil, fmt.Errorf("skill directory '%s' already exists. Use action=edit to modify it", skillDir)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating skill directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		return nil, fmt.Errorf("creating scripts directory: %w", err)
	}

	var dbInfo string
	if withDatabase && skillDB != nil {
		if databaseTable == "" {
			databaseTable = "data"
		}
		databaseSchema := make(map[string]string)
		for k, v := range databaseSchemaRaw {
			if vs, ok := v.(string); ok {
				databaseSchema[k] = vs
			}
		}
		err := skillDB.CreateTable(dbSkillName, databaseTable, displayName, description, databaseSchema)
		if err != nil {
			os.RemoveAll(skillDir)
			return nil, fmt.Errorf("creating database table: %w", err)
		}
		dbInfo = fmt.Sprintf("\n\nDatabase table '%s_%s' created for storing data.", dbSkillName, databaseTable)
	}

	if instructions == "" {
		instructions = fmt.Sprintf("# %s\n\nDescribe how the agent should use this skill.", name)
	}

	if withDatabase && skillDB != nil {
		dbInstructions := fmt.Sprintf(`

## Database

This skill has a database table for storing structured data.

**IMPORTANT:**
- Always use skill_name="%s" (underscores, not hyphens)
- Use action="query" to LIST data (when user asks to "list", "show", "what are")
- NEVER show tool syntax in chat - respond naturally

### The skill_db Tool

`+"```"+`
# LIST records (use this when user asks to "list" or "show")
skill_db(action="query", skill_name="%s", table_name="%s")

# ADD a record
skill_db(action="insert", skill_name="%s", table_name="%s", data={"title": "Example"})

# FILTER records
skill_db(action="query", skill_name="%s", table_name="%s", where={"status": "active"})

# UPDATE a record
skill_db(action="update", skill_name="%s", table_name="%s", row_id="ID", data={"status": "done"})

# DELETE a record
skill_db(action="delete", skill_name="%s", table_name="%s", row_id="ID")
`+"```"+`

### Quick Reference
| User asks... | Use action=... |
|--------------|----------------|
| "list my X" / "show X" | query |
| "save/add/create X" | insert |
| "update/change X" | update |
| "delete/remove X" | delete |
`, dbSkillName, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable)
		instructions += dbInstructions
	}

	metadata := map[string]any{
		"openclaw": map[string]any{"emoji": emoji, "always": false},
		"database": withDatabase,
	}
	metaJSON, _ := json.Marshal(metadata)

	skillMD := fmt.Sprintf("---\nname: %s\ndescription: \"%s\"\nmetadata: %s\n---\n%s\n", name, description, string(metaJSON), instructions)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
		return nil, fmt.Errorf("writing SKILL.md: %w", err)
	}

	return fmt.Sprintf("Skill '%s' created at %s%s\n\nTo add scripts: use action=add_script.\nTo test: use action=test.", name, skillDir, dbInfo), nil
}

func handleSkillEdit(skillsDir string, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)
	if name == "" || content == "" {
		return nil, fmt.Errorf("name and content are required for edit action")
	}
	skillFile := filepath.Join(skillsDir, sanitizeSkillName(name), "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill '%s' not found at %s", name, skillFile)
	}
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("writing SKILL.md: %w", err)
	}
	return fmt.Sprintf("Skill '%s' updated.", name), nil
}

func handleSkillAddScript(skillsDir string, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	scriptName, _ := args["script_name"].(string)
	content, _ := args["content"].(string)
	if skillName == "" || scriptName == "" || content == "" {
		return nil, fmt.Errorf("skill_name, script_name, and content are required for add_script action")
	}
	scriptsDir := filepath.Join(skillsDir, sanitizeSkillName(skillName), "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating scripts directory: %w", err)
	}
	scriptPath := filepath.Join(scriptsDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return nil, fmt.Errorf("writing script: %w", err)
	}
	return fmt.Sprintf("Script '%s' added to skill '%s'.", scriptName, skillName), nil
}

func handleSkillList(registry *skills.Registry, skillsDir string) (any, error) {
	allSkills := registry.List()
	if len(allSkills) == 0 {
		return "No skills installed.", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Installed skills (%d):\n\n", len(allSkills))
	for _, meta := range allSkills {
		fmt.Fprintf(&sb, "- **%s** v%s by %s\n  %s\n  Category: %s, Tags: %s\n",
			meta.Name, meta.Version, meta.Author, meta.Description,
			meta.Category, strings.Join(meta.Tags, ", "))
	}
	userSkills := listUserSkillDirs(skillsDir)
	if len(userSkills) > 0 {
		fmt.Fprintf(&sb, "\nUser skills directory (%d):\n", len(userSkills))
		for _, name := range userSkills {
			fmt.Fprintf(&sb, "- %s\n", name)
		}
	}
	return sb.String(), nil
}

func handleSkillTest(ctx context.Context, registry *skills.Registry, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	input, _ := args["input"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for test action")
	}
	skill, ok := registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("skill '%s' not found in registry", name)
	}
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := skill.Execute(testCtx, input)
	if err != nil {
		return nil, fmt.Errorf("skill execution failed: %w", err)
	}
	return fmt.Sprintf("Skill '%s' test result:\n\n%s", name, result), nil
}

func handleSkillInstall(ctx context.Context, installer *skills.Installer, registry *skills.Registry, args map[string]any) (any, error) {
	source, _ := args["source"].(string)
	if source == "" {
		return nil, fmt.Errorf("source is required for install action")
	}
	installCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := installer.Install(installCtx, source)
	if err != nil {
		return nil, fmt.Errorf("install failed: %w", err)
	}
	reloadCtx, reloadCancel := context.WithTimeout(ctx, 10*time.Second)
	defer reloadCancel()
	reloaded, reloadErr := registry.Reload(reloadCtx)
	reloadMsg := ""
	if reloadErr != nil {
		reloadMsg = fmt.Sprintf("\nWarning: skill catalog refresh failed: %v", reloadErr)
	} else {
		reloadMsg = fmt.Sprintf("\nSkill catalog refreshed (%d skills loaded).", reloaded)
	}
	status := "installed"
	if !result.IsNew {
		status = "updated"
	}
	return fmt.Sprintf("Skill '%s' %s successfully.\nPath: %s\nSource: %s%s",
		result.Name, status, result.Path, result.Source, reloadMsg), nil
}

func handleSkillSearch(args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required for search action")
	}
	client := skills.NewClawHubClient("")
	result, err := client.Search(query, 10)
	if err != nil {
		return nil, fmt.Errorf("ClawHub search failed: %w", err)
	}
	if len(result.Results) == 0 {
		return fmt.Sprintf("No skills found for %q on ClawHub.", query), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "ClawHub results for %q (%d found):\n\n", query, len(result.Results))
	for _, s := range result.Results {
		fmt.Fprintf(&sb, "- **%s** (%s)\n  %s\n  Score: %.2f\n  Install: skill_manage action=install source=%q\n\n",
			s.DisplayName, s.Slug, s.Summary, s.Score, s.Slug)
	}
	return sb.String(), nil
}

func handleSkillDefaultsList(skillsDir string) (any, error) {
	defaults := skills.DefaultSkills()
	installed := listUserSkillDirs(skillsDir)
	installedSet := make(map[string]bool, len(installed))
	for _, n := range installed {
		installedSet[n] = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Default skills available (%d):\n\n", len(defaults))
	for _, d := range defaults {
		status := "not installed"
		if installedSet[d.Name] {
			status = "installed"
		}
		fmt.Fprintf(&sb, "- **%s** — %s [%s]\n", d.Name, d.Description, status)
	}
	sb.WriteString("\nUse action=defaults_install with names to install. Pass [\"all\"] for all.")
	return sb.String(), nil
}

func handleSkillDefaultsInstall(ctx context.Context, registry *skills.Registry, skillsDir string, args map[string]any) (any, error) {
	rawNames, _ := args["names"].([]any)
	if len(rawNames) == 0 {
		return nil, fmt.Errorf("names is required for defaults_install action: pass skill names or [\"all\"]")
	}
	var names []string
	for _, v := range rawNames {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	if len(names) == 1 && strings.ToLower(names[0]) == "all" {
		names = skills.DefaultSkillNames()
	}
	installed, skipped, failed := skills.InstallDefaultSkills(skillsDir, names)
	reloadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	reloaded, reloadErr := registry.Reload(reloadCtx)
	reloadMsg := ""
	if reloadErr != nil {
		reloadMsg = fmt.Sprintf("\nWarning: catalog refresh failed: %v", reloadErr)
	} else {
		reloadMsg = fmt.Sprintf("\nSkill catalog refreshed (%d skills loaded).", reloaded)
	}
	var sb strings.Builder
	sb.WriteString("Default skills installation complete.\n")
	fmt.Fprintf(&sb, "  Installed: %d\n", installed)
	if skipped > 0 {
		fmt.Fprintf(&sb, "  Already existed: %d\n", skipped)
	}
	if failed > 0 {
		fmt.Fprintf(&sb, "  Failed: %d\n", failed)
	}
	sb.WriteString(reloadMsg)
	return sb.String(), nil
}

func handleSkillRemove(registry *skills.Registry, skillsDir string, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for remove action")
	}
	targetDir := filepath.Join(skillsDir, sanitizeSkillName(name))
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill '%s' not found at %s", name, targetDir)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, fmt.Errorf("removing skill: %w", err)
	}
	registry.Remove(name)
	return fmt.Sprintf("Skill '%s' removed successfully.", name), nil
}

// sanitizeSkillName normalizes a skill name to filesystem-safe format.
func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	// Remove anything that's not alphanumeric or hyphen.
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// listUserSkillDirs lists skill directories in the user skills folder.
func listUserSkillDirs(skillsDir string) []string {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names
}

// isSkillWriteAction returns true for actions that create, modify, or delete
// files on disk. These require admin access to prevent privilege escalation
// in restricted profiles (e.g., messaging channels).
func isSkillWriteAction(action string) bool {
	switch action {
	case "init", "edit", "add_script", "install", "defaults_install", "remove":
		return true
	default:
		return false
	}
}
