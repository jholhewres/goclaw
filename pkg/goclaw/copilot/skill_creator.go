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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// RegisterSkillCreatorTools registers skill management tools in the executor.
// skillsDir is the workspace-level directory where user-created skills live.
func RegisterSkillCreatorTools(executor *ToolExecutor, registry *skills.Registry, skillsDir string) {
	if skillsDir == "" {
		skillsDir = "./skills"
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
			"type":       "object",
			"properties": map[string]any{},
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
