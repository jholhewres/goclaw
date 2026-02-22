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

// RegisterSkillCreatorTools registers skill management tools in the executor.
// skillsDir is the workspace-level directory where user-created skills live.
func RegisterSkillCreatorTools(executor *ToolExecutor, registry *skills.Registry, skillsDir string, logger *slog.Logger) {
	if skillsDir == "" {
		skillsDir = "./skills"
	}
	if logger == nil {
		logger = slog.Default()
	}

	// init_skill
	executor.Register(
		MakeToolDefinition("init_skill", "Create a new skill with a SKILL.md template. The skill will be available for the agent to use after creation.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (lowercase, hyphens allowed, e.g. 'my-skill')",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Brief description of what the skill does",
				},
				"instructions": map[string]any{
					"type":        "string",
					"description": "Detailed instructions for the agent on how to use this skill (markdown)",
				},
				"emoji": map[string]any{
					"type":        "string",
					"description": "Optional emoji for the skill",
				},
			},
			"required": []string{"name", "description"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			description, _ := args["description"].(string)
			instructions, _ := args["instructions"].(string)
			emoji, _ := args["emoji"].(string)

			if name == "" || description == "" {
				return nil, fmt.Errorf("name and description are required")
			}

			// Sanitize name.
			name = sanitizeSkillName(name)

			// Create directory structure.
			skillDir := filepath.Join(skillsDir, name)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return nil, fmt.Errorf("creating skill directory: %w", err)
			}
			if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
				return nil, fmt.Errorf("creating scripts directory: %w", err)
			}

			// Build SKILL.md content.
			if instructions == "" {
				instructions = fmt.Sprintf("# %s\n\nDescribe how the agent should use this skill.", name)
			}

			metadata := map[string]any{
				"openclaw": map[string]any{
					"emoji":  emoji,
					"always": false,
				},
			}
			metaJSON, _ := json.Marshal(metadata)

			skillMD := fmt.Sprintf(`---
name: %s
description: "%s"
metadata: %s
---
%s
`, name, description, string(metaJSON), instructions)

			skillFile := filepath.Join(skillDir, "SKILL.md")
			if err := os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
				return nil, fmt.Errorf("writing SKILL.md: %w", err)
			}

			return fmt.Sprintf("Skill '%s' created at %s\n\nTo add scripts: use add_script tool.\nTo test: use test_skill tool.", name, skillDir), nil
		},
	)

	// edit_skill
	executor.Register(
		MakeToolDefinition("edit_skill", "Edit the SKILL.md instructions of an existing skill.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to edit",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "New full content for SKILL.md (including frontmatter)",
				},
			},
			"required": []string{"name", "content"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			content, _ := args["content"].(string)

			if name == "" || content == "" {
				return nil, fmt.Errorf("name and content are required")
			}

			skillFile := filepath.Join(skillsDir, sanitizeSkillName(name), "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				return nil, fmt.Errorf("skill '%s' not found at %s", name, skillFile)
			}

			if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
				return nil, fmt.Errorf("writing SKILL.md: %w", err)
			}

			return fmt.Sprintf("Skill '%s' updated.", name), nil
		},
	)

	// add_script
	executor.Register(
		MakeToolDefinition("add_script", "Add an executable script to a skill. Scripts are placed in the skill's scripts/ directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Skill to add the script to",
				},
				"script_name": map[string]any{
					"type":        "string",
					"description": "Script filename (e.g. 'main.py', 'fetch.js', 'run.sh')",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Script source code",
				},
			},
			"required": []string{"skill_name", "script_name", "content"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			scriptName, _ := args["script_name"].(string)
			content, _ := args["content"].(string)

			if skillName == "" || scriptName == "" || content == "" {
				return nil, fmt.Errorf("skill_name, script_name, and content are required")
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
		},
	)

	// list_skills
	executor.Register(
		MakeToolDefinition("list_skills", "List all installed skills (both built-in and user-created).", map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}),
		func(_ context.Context, _ map[string]any) (any, error) {
			allSkills := registry.List()

			if len(allSkills) == 0 {
				return "No skills installed.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Installed skills (%d):\n\n", len(allSkills)))

			for _, meta := range allSkills {
				sb.WriteString(fmt.Sprintf("- **%s** v%s by %s\n  %s\n  Category: %s, Tags: %s\n",
					meta.Name, meta.Version, meta.Author,
					meta.Description,
					meta.Category, strings.Join(meta.Tags, ", ")))
			}

			// Also list user-created skills not yet loaded.
			userSkills := listUserSkillDirs(skillsDir)
			if len(userSkills) > 0 {
				sb.WriteString(fmt.Sprintf("\nUser skills directory (%d):\n", len(userSkills)))
				for _, name := range userSkills {
					sb.WriteString(fmt.Sprintf("- %s\n", name))
				}
			}

			return sb.String(), nil
		},
	)

	// test_skill
	executor.Register(
		MakeToolDefinition("test_skill", "Test a skill by executing it with sample input.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to test",
				},
				"input": map[string]any{
					"type":        "string",
					"description": "Test input to send to the skill",
				},
			},
			"required": []string{"name", "input"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			input, _ := args["input"].(string)

			if name == "" {
				return nil, fmt.Errorf("name is required")
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
		},
	)

	// install_skill — install skills from ClawHub, GitHub, URL, or local path.
	installer := skills.NewInstaller(skillsDir, logger)

	executor.Register(
		MakeToolDefinition("install_skill", "Install a skill from ClawHub, GitHub, URL, or local path. Supports: ClawHub slugs (e.g. 'steipete/trello'), ClawHub URLs (https://clawhub.ai/user/skill), GitHub URLs (https://github.com/user/repo), HTTP URLs (zip or SKILL.md), and local paths.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Skill source: ClawHub slug (steipete/trello), GitHub URL, HTTP URL, or local path",
				},
			},
			"required": []string{"source"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			source, _ := args["source"].(string)
			if source == "" {
				return nil, fmt.Errorf("source is required")
			}

			installCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			result, err := installer.Install(installCtx, source)
			if err != nil {
				return nil, fmt.Errorf("install failed: %w", err)
			}

			// Hot-reload: reload the registry to pick up the new skill.
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
		},
	)

	// search_skills — search ClawHub for available skills.
	executor.Register(
		MakeToolDefinition("search_skills", "Search the ClawHub skill registry for available skills by keyword.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query (e.g. 'calendar', 'trello', 'github')",
				},
			},
			"required": []string{"query"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return nil, fmt.Errorf("query is required")
			}

			client := skills.NewClawHubClient("")
			result, err := client.Search(query, 10)
			if err != nil {
				return nil, fmt.Errorf("ClawHub search failed: %w", err)
			}

			if len(result.Skills) == 0 {
				return fmt.Sprintf("No skills found for %q on ClawHub.", query), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("ClawHub results for %q (%d found):\n\n", query, len(result.Skills)))
			for _, s := range result.Skills {
				sb.WriteString(fmt.Sprintf("- **%s** (%s)\n  %s\n  Stars: %d | Downloads: %d\n  Install: `devclaw skill install %s` or ask me to install it\n\n",
					s.Name, s.Slug, s.Description, s.Stars, s.Downloads, s.Slug))
			}
			return sb.String(), nil
		},
	)

	// skill_defaults_list — list available default skills.
	executor.Register(
		MakeToolDefinition("skill_defaults_list", "List all default skills bundled with DevClaw that can be installed instantly (no internet required).", map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}),
		func(_ context.Context, _ map[string]any) (any, error) {
			defaults := skills.DefaultSkills()
			installed := listUserSkillDirs(skillsDir)
			installedSet := make(map[string]bool, len(installed))
			for _, n := range installed {
				installedSet[n] = true
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Default skills available (%d):\n\n", len(defaults)))
			for _, d := range defaults {
				status := "not installed"
				if installedSet[d.Name] {
					status = "✓ installed"
				}
				sb.WriteString(fmt.Sprintf("- **%s** — %s [%s]\n", d.Name, d.Description, status))
			}
			sb.WriteString("\nUse skill_defaults_install to install one or more. Pass names: [\"web-search\",\"weather\"] or \"all\" for all.")
			return sb.String(), nil
		},
	)

	// skill_defaults_install — install default skills from the embedded catalog.
	executor.Register(
		MakeToolDefinition("skill_defaults_install", "Install one or more default skills bundled with DevClaw. Pass specific names or 'all' to install everything. No internet required.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"names": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Skill names to install, e.g. [\"web-search\",\"weather\"]. Use [\"all\"] to install all defaults.",
				},
			},
			"required": []string{"names"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			rawNames, _ := args["names"].([]any)
			if len(rawNames) == 0 {
				return nil, fmt.Errorf("names is required: pass skill names or [\"all\"]")
			}

			// Parse names.
			var names []string
			for _, v := range rawNames {
				if s, ok := v.(string); ok {
					names = append(names, s)
				}
			}

			// Handle "all".
			if len(names) == 1 && strings.ToLower(names[0]) == "all" {
				names = skills.DefaultSkillNames()
			}

			installed, skipped, failed := skills.InstallDefaultSkills(skillsDir, names)

			// Hot-reload the registry to pick up new skills.
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
			sb.WriteString(fmt.Sprintf("  Installed: %d\n", installed))
			if skipped > 0 {
				sb.WriteString(fmt.Sprintf("  Already existed: %d\n", skipped))
			}
			if failed > 0 {
				sb.WriteString(fmt.Sprintf("  Failed: %d\n", failed))
			}
			sb.WriteString(reloadMsg)
			return sb.String(), nil
		},
	)

	// remove_skill — remove an installed skill.
	executor.Register(
		MakeToolDefinition("remove_skill", "Remove an installed skill by name.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the skill to remove",
				},
			},
			"required": []string{"name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("name is required")
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
		},
	)
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
