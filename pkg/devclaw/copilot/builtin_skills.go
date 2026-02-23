// Package copilot – builtin_skills.go provides embedded skills that are included
// in the binary and loaded into the system prompt.
package copilot

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"log/slog"
)

//go:embed builtin/skills/*/SKILL.md
var builtinSkillsFS embed.FS

// BuiltinSkill represents a built-in skill loaded from the embedded filesystem.
type BuiltinSkill struct {
	Name        string
	Description string
	Content     string
	Trigger     string // automatic, manual
}

// BuiltinSkills holds all loaded built-in skills.
type BuiltinSkills struct {
	skills map[string]*BuiltinSkill
	mu     sync.RWMutex
	logger *slog.Logger
}

// builtinSkillsGlobal is the global instance of loaded skills.
var builtinSkillsGlobal *BuiltinSkills
var builtinSkillsOnce sync.Once

// LoadBuiltinSkills loads all built-in skills from the embedded filesystem.
// Uses singleton pattern to load only once.
func LoadBuiltinSkills(logger *slog.Logger) *BuiltinSkills {
	builtinSkillsOnce.Do(func() {
		builtinSkillsGlobal = &BuiltinSkills{
			skills: make(map[string]*BuiltinSkill),
			logger: logger,
		}
		builtinSkillsGlobal.load()
	})
	return builtinSkillsGlobal
}

// GetBuiltinSkills returns the global builtin skills instance.
// Returns nil if LoadBuiltinSkills hasn't been called yet.
func GetBuiltinSkills() *BuiltinSkills {
	return builtinSkillsGlobal
}

// load reads all SKILL.md files from the embedded filesystem.
func (bs *BuiltinSkills) load() {
	// Read the builtin/skills directory
	entries, err := fs.ReadDir(builtinSkillsFS, "builtin/skills")
	if err != nil {
		if bs.logger != nil {
			bs.logger.Warn("failed to read builtin skills directory", "error", err)
		}
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillPath := fmt.Sprintf("builtin/skills/%s/SKILL.md", skillName)

		content, err := fs.ReadFile(builtinSkillsFS, skillPath)
		if err != nil {
			if bs.logger != nil {
				bs.logger.Debug("skipping skill, file not found", "skill", skillName, "path", skillPath)
			}
			continue
		}

		skill := bs.parseSkill(skillName, string(content))
		bs.skills[skillName] = skill

		if bs.logger != nil {
			bs.logger.Debug("loaded builtin skill", "name", skillName, "description", skill.Description)
		}
	}

	if bs.logger != nil {
		bs.logger.Info("builtin skills loaded", "count", len(bs.skills))
	}
}

// parseSkill parses a SKILL.md file and extracts metadata.
func (bs *BuiltinSkills) parseSkill(name, content string) *BuiltinSkill {
	skill := &BuiltinSkill{
		Name:    name,
		Content: content,
		Trigger: "automatic", // default
	}

	// Parse YAML frontmatter
	if strings.HasPrefix(content, "---") {
		end := strings.Index(content[3:], "---")
		if end > 0 {
			frontmatter := content[3 : end+3]
			body := content[end+6:]

			// Extract metadata
			lines := strings.Split(frontmatter, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
					// Remove quotes if present
					skill.Description = strings.Trim(skill.Description, "\"'")
				} else if strings.HasPrefix(line, "trigger:") {
					skill.Trigger = strings.TrimSpace(strings.TrimPrefix(line, "trigger:"))
				}
			}

			// Use body without frontmatter
			skill.Content = strings.TrimSpace(body)
		}
	}

	return skill
}

// Get returns a skill by name.
func (bs *BuiltinSkills) Get(name string) *BuiltinSkill {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.skills[name]
}

// All returns all loaded skills.
func (bs *BuiltinSkills) All() map[string]*BuiltinSkill {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make(map[string]*BuiltinSkill, len(bs.skills))
	for k, v := range bs.skills {
		result[k] = v
	}
	return result
}

// Names returns all skill names.
func (bs *BuiltinSkills) Names() []string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	names := make([]string, 0, len(bs.skills))
	for name := range bs.skills {
		names = append(names, name)
	}
	return names
}

// FormatForPrompt formats all skills for inclusion in the system prompt.
// Only includes skills with trigger="automatic".
func (bs *BuiltinSkills) FormatForPrompt() string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var sb strings.Builder
	for name, skill := range bs.skills {
		if skill.Trigger != "automatic" {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n## %s\n\n", strings.Title(name)))
		if skill.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", skill.Description))
		}
		sb.WriteString(skill.Content)
		sb.WriteString("\n\n---\n")
	}

	return sb.String()
}

// FormatSkillForPrompt formats a single skill for the prompt.
func (bs *BuiltinSkills) FormatSkillForPrompt(name string) string {
	skill := bs.Get(name)
	if skill == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(skill.Name)))
	if skill.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", skill.Description))
	}
	sb.WriteString(skill.Content)
	return sb.String()
}
