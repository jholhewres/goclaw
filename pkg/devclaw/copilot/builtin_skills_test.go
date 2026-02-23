package copilot

import (
	"strings"
	"testing"
)

func TestLoadBuiltinSkills(t *testing.T) {
	skills := LoadBuiltinSkills(nil)

	if skills == nil {
		t.Fatal("LoadBuiltinSkills returned nil")
	}

	// Check that memory and teams skills are loaded
	names := skills.Names()
	if len(names) < 2 {
		t.Errorf("expected at least 2 builtin skills, got %d: %v", len(names), names)
	}

	// Check memory skill
	memorySkill := skills.Get("memory")
	if memorySkill == nil {
		t.Error("expected memory skill to be loaded")
	} else {
		if memorySkill.Name != "memory" {
			t.Errorf("expected skill name 'memory', got %q", memorySkill.Name)
		}
		if memorySkill.Description == "" {
			t.Error("expected memory skill to have a description")
		}
		if !strings.Contains(memorySkill.Content, "action=") {
			t.Error("expected memory skill content to contain tool usage examples")
		}
	}

	// Check teams skill
	teamsSkill := skills.Get("teams")
	if teamsSkill == nil {
		t.Error("expected teams skill to be loaded")
	} else {
		if teamsSkill.Name != "teams" {
			t.Errorf("expected skill name 'teams', got %q", teamsSkill.Name)
		}
		if !strings.Contains(teamsSkill.Content, "team_manage") {
			t.Error("expected teams skill content to contain tool references")
		}
	}
}

func TestBuiltinSkills_FormatForPrompt(t *testing.T) {
	skills := LoadBuiltinSkills(nil)

	prompt := skills.FormatForPrompt()
	if prompt == "" {
		t.Error("expected non-empty prompt from FormatForPrompt")
	}

	// Should contain both skills
	if !strings.Contains(prompt, "Memory") {
		t.Error("expected prompt to contain Memory section")
	}
	if !strings.Contains(prompt, "Teams") {
		t.Error("expected prompt to contain Teams section")
	}
}

func TestBuiltinSkills_FormatSkillForPrompt(t *testing.T) {
	skills := LoadBuiltinSkills(nil)

	// Test specific skill formatting
	memoryPrompt := skills.FormatSkillForPrompt("memory")
	if memoryPrompt == "" {
		t.Error("expected non-empty prompt for memory skill")
	}
	if !strings.Contains(memoryPrompt, "Memory") {
		t.Error("expected memory prompt to contain 'Memory'")
	}

	// Test non-existent skill
	nonExistent := skills.FormatSkillForPrompt("nonexistent")
	if nonExistent != "" {
		t.Error("expected empty string for non-existent skill")
	}
}

func TestBuiltinSkills_ParseFrontmatter(t *testing.T) {
	// Test that frontmatter is correctly parsed
	skills := LoadBuiltinSkills(nil)

	memorySkill := skills.Get("memory")
	if memorySkill == nil {
		t.Fatal("memory skill not loaded")
	}

	// Check trigger is parsed
	if memorySkill.Trigger != "automatic" {
		t.Errorf("expected trigger 'automatic', got %q", memorySkill.Trigger)
	}

	// Check description is parsed and not empty
	if memorySkill.Description == "" {
		t.Error("expected description to be parsed from frontmatter")
	}

	// Check that content doesn't include frontmatter
	if strings.HasPrefix(memorySkill.Content, "---") {
		t.Error("expected content to not include frontmatter")
	}
}
