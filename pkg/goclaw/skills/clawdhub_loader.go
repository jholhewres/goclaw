// Package skills – clawdhub_loader.go implements a SkillLoader that reads
// OpenClaw/ClawdHub SKILL.md format and converts them into GoClaw skills.
//
// ClawdHub skills use SKILL.md with YAML frontmatter:
//
//	---
//	name: my-skill
//	description: "What this skill does"
//	metadata: { "openclaw": { "emoji": "...", "requires": { "bins": [...] } } }
//	---
//	# Skill Title
//	Instructions for the agent...
//
// The loader parses frontmatter, validates requirements, resolves {baseDir}
// references, and wraps the skill as a ScriptSkill for GoClaw execution.
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ClawdHubLoader loads skills from directories using the SKILL.md format.
type ClawdHubLoader struct {
	// dirs is the list of directories to scan for skills.
	dirs []string

	// logger for operational messages.
	logger *slog.Logger
}

// ClawdHubSkillDef holds the parsed SKILL.md frontmatter.
type ClawdHubSkillDef struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Homepage    string                 `yaml:"homepage"`
	Metadata    map[string]interface{} `yaml:"metadata"`

	// Parsed from metadata.openclaw
	OpenClaw *OpenClawMeta

	// Body is the markdown content after frontmatter (instructions).
	Body string

	// Dir is the absolute path to the skill directory.
	Dir string
}

// OpenClawMeta holds the openclaw-specific metadata.
type OpenClawMeta struct {
	Emoji    string          `json:"emoji"`
	Always   bool            `json:"always"`
	OS       []string        `json:"os"`
	Requires OpenClawRequire `json:"requires"`
	Install  []InstallSpec   `json:"install"`
}

// OpenClawRequire defines runtime requirements.
type OpenClawRequire struct {
	Bins    []string `json:"bins"`
	AnyBins []string `json:"anyBins"`
	Env     []string `json:"env"`
	Config  []string `json:"config"`
}

// InstallSpec describes how to install a dependency.
type InstallSpec struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"` // brew, apt, node, go, uv, download
	Formula string   `json:"formula"`
	Package string   `json:"package"`
	Bins    []string `json:"bins"`
	Label   string   `json:"label"`
	OS      []string `json:"os"`
}

// NewClawdHubLoader creates a loader that scans the given directories.
func NewClawdHubLoader(dirs []string, logger *slog.Logger) *ClawdHubLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClawdHubLoader{dirs: dirs, logger: logger}
}

// Load scans all configured directories and returns found skills.
func (l *ClawdHubLoader) Load(ctx context.Context) ([]Skill, error) {
	var skills []Skill

	for _, dir := range l.dirs {
		found, err := l.loadDir(ctx, dir)
		if err != nil {
			l.logger.Warn("clawdhub: error loading directory",
				"dir", dir, "error", err)
			continue
		}
		skills = append(skills, found...)
	}

	l.logger.Info("clawdhub: loaded skills",
		"count", len(skills),
		"dirs", len(l.dirs))

	return skills, nil
}

// Source returns the loader source identifier.
func (l *ClawdHubLoader) Source() string {
	return "clawdhub"
}

// ---------- Parsing ----------

// loadDir scans a directory for SKILL.md files.
func (l *ClawdHubLoader) loadDir(_ context.Context, dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")

		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue // No SKILL.md, skip.
		}

		def, err := l.parseSkillMD(skillFile, skillDir)
		if err != nil {
			l.logger.Warn("clawdhub: error parsing skill",
				"path", skillFile, "error", err)
			continue
		}

		// Check requirements before registering.
		if !l.checkRequirements(def) {
			l.logger.Debug("clawdhub: skill requirements not met",
				"name", def.Name, "dir", skillDir)
			continue
		}

		// Convert to GoClaw skill.
		skill := NewScriptSkill(def)
		skills = append(skills, skill)

		l.logger.Debug("clawdhub: loaded skill",
			"name", def.Name, "dir", skillDir)
	}

	return skills, nil
}

// parseSkillMD reads and parses a SKILL.md file.
func (l *ClawdHubLoader) parseSkillMD(path, dir string) (*ClawdHubSkillDef, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	text := string(content)

	// Parse YAML frontmatter.
	def, body, err := parseFrontmatter(text)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	def.Body = body
	def.Dir = dir

	// Parse openclaw metadata if present.
	if meta, ok := def.Metadata["openclaw"]; ok {
		ocMeta, err := parseOpenClawMeta(meta)
		if err != nil {
			l.logger.Warn("clawdhub: error parsing openclaw metadata",
				"name", def.Name, "error", err)
		} else {
			def.OpenClaw = ocMeta
		}
	}

	return def, nil
}

// parseFrontmatter extracts YAML frontmatter from a markdown file.
// Returns the parsed definition, remaining body, and any error.
func parseFrontmatter(text string) (*ClawdHubSkillDef, string, error) {
	text = strings.TrimSpace(text)

	if !strings.HasPrefix(text, "---") {
		return nil, "", fmt.Errorf("no YAML frontmatter found")
	}

	// Find closing ---
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, "", fmt.Errorf("unclosed YAML frontmatter")
	}

	frontmatter := strings.TrimSpace(rest[:idx])
	body := strings.TrimSpace(rest[idx+4:])

	def := &ClawdHubSkillDef{
		Metadata: make(map[string]interface{}),
	}

	// Parse YAML line by line (simple parser for flat keys).
	// The metadata field contains inline JSON which standard YAML
	// parsers handle, but we do a lightweight parse here.
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		// Remove surrounding quotes.
		value = strings.Trim(value, `"'`)

		switch key {
		case "name":
			def.Name = value
		case "description":
			def.Description = value
		case "homepage":
			def.Homepage = value
		case "metadata":
			// metadata is inline JSON.
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(value), &meta); err != nil {
				// Try to find the JSON object spanning multiple lines.
				jsonStr := extractJSONBlock(frontmatter, "metadata")
				if jsonStr != "" {
					if err := json.Unmarshal([]byte(jsonStr), &meta); err == nil {
						def.Metadata = meta
					}
				}
			} else {
				def.Metadata = meta
			}
		}
	}

	if def.Name == "" {
		return nil, "", fmt.Errorf("skill name is required")
	}

	return def, body, nil
}

// extractJSONBlock tries to extract a JSON object that spans multiple
// lines in the frontmatter, starting from a given key.
func extractJSONBlock(frontmatter, key string) string {
	lines := strings.Split(frontmatter, "\n")
	var collecting bool
	var depth int
	var buf strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !collecting {
			if strings.HasPrefix(trimmed, key+":") {
				// Start collecting from after the key.
				after := strings.TrimPrefix(trimmed, key+":")
				after = strings.TrimSpace(after)
				if after != "" {
					buf.WriteString(after)
					depth += strings.Count(after, "{") - strings.Count(after, "}")
					collecting = true
				}
				continue
			}
		}

		if collecting {
			buf.WriteString(trimmed)
			depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
			if depth <= 0 {
				break
			}
		}
	}

	result := buf.String()
	// Clean up YAML-style trailing commas that JSON doesn't allow.
	result = strings.ReplaceAll(result, ",}", "}")
	result = strings.ReplaceAll(result, ",]", "]")
	return result
}

// parseOpenClawMeta converts the openclaw metadata map to a typed struct.
func parseOpenClawMeta(meta interface{}) (*OpenClawMeta, error) {
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	var oc OpenClawMeta
	if err := json.Unmarshal(data, &oc); err != nil {
		return nil, err
	}
	return &oc, nil
}

// ---------- Requirement Checking ----------

// checkRequirements validates that the skill's dependencies are met.
func (l *ClawdHubLoader) checkRequirements(def *ClawdHubSkillDef) bool {
	if def.OpenClaw == nil {
		return true // No requirements specified.
	}

	oc := def.OpenClaw

	// If "always", skip requirement checks.
	if oc.Always {
		return true
	}

	// Check OS requirement.
	if len(oc.OS) > 0 && !l.checkOS(oc.OS) {
		return false
	}

	// Check required binaries (all must exist).
	for _, bin := range oc.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			l.logger.Debug("clawdhub: missing required binary",
				"skill", def.Name, "bin", bin)
			return false
		}
	}

	// Check anyBins (at least one must exist).
	if len(oc.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range oc.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			l.logger.Debug("clawdhub: none of required binaries found",
				"skill", def.Name, "bins", oc.Requires.AnyBins)
			return false
		}
	}

	// Check required environment variables.
	for _, env := range oc.Requires.Env {
		if os.Getenv(env) == "" {
			l.logger.Debug("clawdhub: missing required env var",
				"skill", def.Name, "env", env)
			return false
		}
	}

	return true
}

// checkOS validates the current OS against allowed platforms.
func (l *ClawdHubLoader) checkOS(allowed []string) bool {
	// OpenClaw uses: darwin, linux, win32
	// Map Go's runtime.GOOS equivalents.
	currentOS := goosToOpenClaw()
	for _, os := range allowed {
		if os == currentOS {
			return true
		}
	}
	return false
}

// goosToOpenClaw maps Go's runtime.GOOS to OpenClaw's OS identifiers.
func goosToOpenClaw() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "win32"
	default:
		return "linux"
	}
}
