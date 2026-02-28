// Package copilot – identity.go resolves the assistant's identity from
// multiple sources: agent profile, IDENTITY.md file, config, or defaults.
package copilot

import (
	"strings"
)

// ResolveIdentity returns the effective identity by merging sources in priority order:
//  1. AgentProfile.Identity (if agent routing matched)
//  2. IDENTITY.md content (parsed for structured fields)
//  3. Config.Identity
//  4. Fallback: Config.Name
//
// Fields are merged individually: a higher-priority source only overrides
// fields it actually sets (non-empty), so a profile can override just the
// name while inheriting the theme from config.
func ResolveIdentity(cfg *Config, agentProfile *AgentProfileConfig, identityFileContent string) IdentityConfig {
	// Start with the global config identity.
	result := cfg.Identity

	// If the global config has no name but the legacy Name field is set, use it.
	if result.Name == "" && cfg.Name != "" {
		result.Name = cfg.Name
	}

	// Merge from IDENTITY.md if present.
	if identityFileContent != "" {
		parsed := ParseIdentityFile(identityFileContent)
		result = mergeIdentity(result, parsed)
	}

	// Merge from agent profile (highest priority).
	if agentProfile != nil && agentProfile.Identity != nil {
		result = mergeIdentity(result, *agentProfile.Identity)
	}

	// Final fallback: ensure there's always a name.
	if result.Name == "" {
		result.Name = "DevClaw"
	}

	return result
}

// mergeIdentity overlays non-empty fields from overlay onto base.
func mergeIdentity(base, overlay IdentityConfig) IdentityConfig {
	if overlay.Name != "" {
		base.Name = overlay.Name
	}
	if overlay.Emoji != "" {
		base.Emoji = overlay.Emoji
	}
	if overlay.Theme != "" {
		base.Theme = overlay.Theme
	}
	if overlay.Avatar != "" {
		base.Avatar = overlay.Avatar
	}
	if overlay.Vibe != "" {
		base.Vibe = overlay.Vibe
	}
	if overlay.Creature != "" {
		base.Creature = overlay.Creature
	}
	return base
}

// ParseIdentityFile extracts structured identity fields from an IDENTITY.md file.
// Supports two formats:
//   - YAML-like "key: value" lines (e.g. "name: Aria")
//   - Markdown headers followed by content (e.g. "# Name\nAria")
//
// Unrecognized lines are collected as the vibe if no explicit vibe is set.
func ParseIdentityFile(content string) IdentityConfig {
	var id IdentityConfig
	var extraLines []string

	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || line == "---" {
			continue
		}

		// Try "key: value" format.
		if key, val, ok := parseKeyValue(line); ok {
			setIdentityField(&id, key, val)
			continue
		}

		// Try markdown header: "# Key" followed by value on next line.
		if strings.HasPrefix(line, "#") {
			header := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if nextLine != "" && !strings.HasPrefix(nextLine, "#") {
					if setIdentityField(&id, header, nextLine) {
						i++ // skip the value line
						continue
					}
				}
			}
		}

		// Collect unrecognized lines as potential vibe content.
		if !strings.HasPrefix(line, "#") {
			extraLines = append(extraLines, line)
		}
	}

	// If no explicit vibe was set, use collected extra lines.
	if id.Vibe == "" && len(extraLines) > 0 {
		id.Vibe = strings.Join(extraLines, " ")
	}

	return id
}

// parseKeyValue splits "key: value" lines. Returns false if not in that format.
func parseKeyValue(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 1 || idx >= len(line)-1 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if value == "" {
		return "", "", false
	}
	return key, value, true
}

// setIdentityField maps a key name to the appropriate IdentityConfig field.
// Returns true if the key was recognized.
func setIdentityField(id *IdentityConfig, key, value string) bool {
	switch strings.ToLower(key) {
	case "name":
		id.Name = value
	case "emoji":
		id.Emoji = value
	case "theme", "personality":
		id.Theme = value
	case "avatar":
		id.Avatar = value
	case "vibe", "style", "tone":
		id.Vibe = value
	case "creature", "mascot":
		id.Creature = value
	default:
		return false
	}
	return true
}

// ValidateIdentity checks for common issues in an identity configuration.
// Returns a list of warnings (empty = valid).
func ValidateIdentity(id IdentityConfig) []string {
	var warnings []string

	if id.Name != "" {
		if len(id.Name) > 50 {
			warnings = append(warnings, "name is too long (max 50 chars)")
		}
		placeholders := []string{"your name", "assistant name", "bot name", "change me"}
		lower := strings.ToLower(id.Name)
		for _, p := range placeholders {
			if lower == p {
				warnings = append(warnings, "name looks like a placeholder: "+id.Name)
				break
			}
		}
	}

	if id.Theme != "" && len(id.Theme) > 200 {
		warnings = append(warnings, "theme is too long (max 200 chars)")
	}

	if id.Vibe != "" && len(id.Vibe) > 500 {
		warnings = append(warnings, "vibe is too long (max 500 chars)")
	}

	if id.Emoji != "" {
		// Basic check: emojis are typically 1-4 runes.
		runes := []rune(id.Emoji)
		if len(runes) > 8 {
			warnings = append(warnings, "emoji looks too long — use a single emoji")
		}
	}

	return warnings
}
