package copilot

import (
	"testing"
)

func TestIdentityConfig_IsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		if !(IdentityConfig{}).IsEmpty() {
			t.Error("expected IsEmpty to be true")
		}
	})

	t.Run("name set", func(t *testing.T) {
		t.Parallel()
		if (IdentityConfig{Name: "Aria"}).IsEmpty() {
			t.Error("expected IsEmpty to be false")
		}
	})

	t.Run("only emoji", func(t *testing.T) {
		t.Parallel()
		if (IdentityConfig{Emoji: "X"}).IsEmpty() {
			t.Error("expected IsEmpty to be false")
		}
	})
}

func TestIdentityConfig_EffectiveName(t *testing.T) {
	t.Parallel()

	t.Run("identity name set", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{Name: "Aria"}
		if got := id.EffectiveName("DevClaw"); got != "Aria" {
			t.Errorf("EffectiveName = %q, want Aria", got)
		}
	})

	t.Run("fallback to default", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{}
		if got := id.EffectiveName("DevClaw"); got != "DevClaw" {
			t.Errorf("EffectiveName = %q, want DevClaw", got)
		}
	})
}

func TestParseIdentityFile(t *testing.T) {
	t.Parallel()

	t.Run("key value format", func(t *testing.T) {
		t.Parallel()
		content := `name: Aria
emoji: X
theme: helpful hacker
creature: fox
vibe: Friendly and curious`
		id := ParseIdentityFile(content)
		if id.Name != "Aria" {
			t.Errorf("Name = %q", id.Name)
		}
		if id.Emoji != "X" {
			t.Errorf("Emoji = %q", id.Emoji)
		}
		if id.Theme != "helpful hacker" {
			t.Errorf("Theme = %q", id.Theme)
		}
		if id.Creature != "fox" {
			t.Errorf("Creature = %q", id.Creature)
		}
		if id.Vibe != "Friendly and curious" {
			t.Errorf("Vibe = %q", id.Vibe)
		}
	})

	t.Run("markdown header format", func(t *testing.T) {
		t.Parallel()
		content := `# Name
Aria

# Theme
helpful hacker

# Vibe
Always cheerful and supportive`
		id := ParseIdentityFile(content)
		if id.Name != "Aria" {
			t.Errorf("Name = %q", id.Name)
		}
		if id.Theme != "helpful hacker" {
			t.Errorf("Theme = %q", id.Theme)
		}
		if id.Vibe != "Always cheerful and supportive" {
			t.Errorf("Vibe = %q", id.Vibe)
		}
	})

	t.Run("extra lines become vibe", func(t *testing.T) {
		t.Parallel()
		content := `name: Bot
I am a friendly assistant who loves to help.`
		id := ParseIdentityFile(content)
		if id.Name != "Bot" {
			t.Errorf("Name = %q", id.Name)
		}
		if id.Vibe != "I am a friendly assistant who loves to help." {
			t.Errorf("Vibe = %q", id.Vibe)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		t.Parallel()
		id := ParseIdentityFile("")
		if !id.IsEmpty() {
			t.Errorf("expected empty identity, got %+v", id)
		}
	})

	t.Run("with yaml frontmatter separator", func(t *testing.T) {
		t.Parallel()
		content := `---
name: Aria
theme: helpful
---`
		id := ParseIdentityFile(content)
		if id.Name != "Aria" {
			t.Errorf("Name = %q", id.Name)
		}
		if id.Theme != "helpful" {
			t.Errorf("Theme = %q", id.Theme)
		}
	})

	t.Run("personality alias", func(t *testing.T) {
		t.Parallel()
		content := "personality: cheerful coder"
		id := ParseIdentityFile(content)
		if id.Theme != "cheerful coder" {
			t.Errorf("Theme = %q, want cheerful coder", id.Theme)
		}
	})

	t.Run("mascot alias", func(t *testing.T) {
		t.Parallel()
		content := "mascot: owl"
		id := ParseIdentityFile(content)
		if id.Creature != "owl" {
			t.Errorf("Creature = %q, want owl", id.Creature)
		}
	})
}

func TestResolveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("config only", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Name: "LegacyBot",
			Identity: IdentityConfig{
				Theme: "helpful",
			},
		}
		id := ResolveIdentity(cfg, nil, "")
		if id.Name != "LegacyBot" {
			t.Errorf("Name = %q, want LegacyBot", id.Name)
		}
		if id.Theme != "helpful" {
			t.Errorf("Theme = %q, want helpful", id.Theme)
		}
	})

	t.Run("identity name overrides legacy name", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Name:     "OldName",
			Identity: IdentityConfig{Name: "NewName"},
		}
		id := ResolveIdentity(cfg, nil, "")
		if id.Name != "NewName" {
			t.Errorf("Name = %q, want NewName", id.Name)
		}
	})

	t.Run("identity file overrides config", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Identity: IdentityConfig{Name: "ConfigName", Theme: "config-theme"},
		}
		fileContent := "name: FileName\ncreature: fox"
		id := ResolveIdentity(cfg, nil, fileContent)
		if id.Name != "FileName" {
			t.Errorf("Name = %q, want FileName", id.Name)
		}
		if id.Creature != "fox" {
			t.Errorf("Creature = %q, want fox", id.Creature)
		}
		// Theme should be inherited from config since file doesn't set it.
		if id.Theme != "config-theme" {
			t.Errorf("Theme = %q, want config-theme", id.Theme)
		}
	})

	t.Run("agent profile overrides all", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Identity: IdentityConfig{Name: "ConfigBot", Theme: "config-theme"},
		}
		profile := &AgentProfileConfig{
			Identity: &IdentityConfig{Name: "AgentBot", Emoji: "!"},
		}
		id := ResolveIdentity(cfg, profile, "name: FileBot")
		if id.Name != "AgentBot" {
			t.Errorf("Name = %q, want AgentBot", id.Name)
		}
		if id.Emoji != "!" {
			t.Errorf("Emoji = %q, want !", id.Emoji)
		}
		// Theme inherited from config (not overridden).
		if id.Theme != "config-theme" {
			t.Errorf("Theme = %q, want config-theme", id.Theme)
		}
	})

	t.Run("default fallback name", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{}
		id := ResolveIdentity(cfg, nil, "")
		if id.Name != "DevClaw" {
			t.Errorf("Name = %q, want DevClaw", id.Name)
		}
	})

	t.Run("backward compat - only legacy Name", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Name: "MyBot"}
		id := ResolveIdentity(cfg, nil, "")
		if id.Name != "MyBot" {
			t.Errorf("Name = %q, want MyBot", id.Name)
		}
	})
}

func TestValidateIdentity(t *testing.T) {
	t.Parallel()

	t.Run("valid identity", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{
			Name:  "Aria",
			Emoji: "X",
			Theme: "helpful hacker",
		}
		warnings := ValidateIdentity(id)
		if len(warnings) != 0 {
			t.Errorf("expected no warnings, got %v", warnings)
		}
	})

	t.Run("long name", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{
			Name: "This is a very long name that exceeds the fifty character limit for identity names",
		}
		warnings := ValidateIdentity(id)
		if len(warnings) == 0 {
			t.Error("expected warning for long name")
		}
	})

	t.Run("placeholder name", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{Name: "Your Name"}
		warnings := ValidateIdentity(id)
		if len(warnings) == 0 {
			t.Error("expected warning for placeholder name")
		}
	})

	t.Run("long theme", func(t *testing.T) {
		t.Parallel()
		longTheme := ""
		for i := 0; i < 210; i++ {
			longTheme += "x"
		}
		id := IdentityConfig{Theme: longTheme}
		warnings := ValidateIdentity(id)
		if len(warnings) == 0 {
			t.Error("expected warning for long theme")
		}
	})

	t.Run("long emoji", func(t *testing.T) {
		t.Parallel()
		id := IdentityConfig{Emoji: "this is not an emoji"}
		warnings := ValidateIdentity(id)
		if len(warnings) == 0 {
			t.Error("expected warning for long emoji")
		}
	})

	t.Run("empty identity", func(t *testing.T) {
		t.Parallel()
		warnings := ValidateIdentity(IdentityConfig{})
		if len(warnings) != 0 {
			t.Errorf("expected no warnings for empty identity, got %v", warnings)
		}
	})
}
