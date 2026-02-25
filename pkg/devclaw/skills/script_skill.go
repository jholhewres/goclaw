// Package skills – script_skill.go wraps ClawdHub-format skills
// (Python, Node.js, Shell scripts) as DevClaw Skill implementations.
//
// A ScriptSkill:
//   - Exposes the SKILL.md body as the system prompt
//   - Discovers scripts in the skill's scripts/ directory
//   - Delegates execution to the sandbox.Runner
//   - Replaces {baseDir} with the skill's directory path
//   - Extracts trigger phrases from the SKILL.md body
package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/sandbox"
)

// ScriptSkill wraps a ClawdHub skill definition as a DevClaw Skill.
type ScriptSkill struct {
	def      *ClawdHubSkillDef
	meta     Metadata
	scripts  []SkillScript
	triggers []string
	runner   *sandbox.Runner
}

// SkillScript represents an executable script in the skill directory.
type SkillScript struct {
	Name    string
	Path    string
	Runtime sandbox.Runtime
}

// NewScriptSkill creates a DevClaw skill from a ClawdHub definition.
func NewScriptSkill(def *ClawdHubSkillDef) *ScriptSkill {
	s := &ScriptSkill{
		def: def,
		meta: Metadata{
			Name:        def.Name,
			Description: def.Description,
			Author:      "clawdhub",
			Category:    "community",
		},
	}

	// Set emoji as tag if available.
	if def.OpenClaw != nil && def.OpenClaw.Emoji != "" {
		s.meta.Tags = append(s.meta.Tags, "emoji:"+def.OpenClaw.Emoji)
	}

	// Discover scripts in the skill directory.
	s.scripts = discoverScripts(def.Dir)

	// Extract triggers from the body (lines starting with keywords).
	s.triggers = extractTriggers(def.Body, def.Name)

	return s
}

// ---------- Skill Interface ----------

// Metadata returns the skill metadata.
func (s *ScriptSkill) Metadata() Metadata {
	return s.meta
}

// Tools returns the tools exposed by this skill.
// Each discovered script becomes a tool.
func (s *ScriptSkill) Tools() []Tool {
	tools := make([]Tool, 0, len(s.scripts)+1)

	// Main "execute" tool that runs the skill with free-form input.
	tools = append(tools, Tool{
		Name:        "execute",
		Description: fmt.Sprintf("Execute the %s skill with the given input", s.def.Name),
		Parameters: []ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Input for the skill",
				Required:    true,
			},
		},
	})

	// Each script becomes a tool.
	for _, script := range s.scripts {
		tools = append(tools, Tool{
			Name:        "run_" + sanitizeToolName(script.Name),
			Description: fmt.Sprintf("Run %s (%s)", script.Name, script.Runtime),
			Parameters: []ToolParameter{
				{
					Name:        "args",
					Type:        "string",
					Description: "Command-line arguments for the script",
				},
				{
					Name:        "stdin",
					Type:        "string",
					Description: "Standard input for the script",
				},
			},
		})
	}

	return tools
}

// SystemPrompt returns the SKILL.md body with {baseDir} resolved.
// For prompt-only skills (no scripts), adds a notice about available tools.
func (s *ScriptSkill) SystemPrompt() string {
	body := s.def.Body

	// Replace {baseDir} with the actual skill directory.
	body = strings.ReplaceAll(body, "{baseDir}", s.def.Dir)

	// Check if this is a prompt-only skill (no executable scripts)
	if len(s.scripts) == 0 {
		// Add notice about how to use this skill
		notice := `

---
**Skill Notice:** This skill provides instructions only and has no executable scripts.

- If the skill references specific tools, use the ` + "`bash`" + ` tool to run CLI commands
- If the skill mentions external CLIs (e.g., gog, gh, aws), ensure they are installed: ` + "`which <cli-name>`" + `
- If no CLI is mentioned and no tools are available, inform the user that this skill requires additional setup
`

		body += notice
	}

	return body
}

// Triggers returns phrases that should activate this skill.
func (s *ScriptSkill) Triggers() []string {
	return s.triggers
}

// Init initializes the skill. Sets the sandbox runner.
func (s *ScriptSkill) Init(_ context.Context, config map[string]any) error {
	// If a sandbox.Runner is provided via config, use it.
	if runner, ok := config["_sandbox_runner"].(*sandbox.Runner); ok {
		s.runner = runner
	}
	return nil
}

// Execute runs the skill's primary operation.
// For script skills, this means finding the most appropriate script
// and running it through the sandbox.
func (s *ScriptSkill) Execute(ctx context.Context, input string) (string, error) {
	if s.runner == nil {
		return "", fmt.Errorf("sandbox runner not configured for skill %s", s.def.Name)
	}

	// If there's exactly one script, run it.
	if len(s.scripts) == 1 {
		return s.runScript(ctx, s.scripts[0], input)
	}

	// If there are multiple scripts, try to find one matching the input.
	for _, script := range s.scripts {
		if strings.Contains(strings.ToLower(input), strings.ToLower(script.Name)) {
			return s.runScript(ctx, script, input)
		}
	}

	// Default: run the first script.
	if len(s.scripts) > 0 {
		return s.runScript(ctx, s.scripts[0], input)
	}

	// No scripts — this is a prompt-only skill.
	return fmt.Sprintf("[%s] This skill provides instructions only. Use the system prompt for guidance.", s.def.Name), nil
}

// Shutdown releases resources.
func (s *ScriptSkill) Shutdown() error {
	return nil
}

// ---------- SkillSetupChecker Interface ----------

// RequiredConfig returns the configuration requirements for this skill.
func (s *ScriptSkill) RequiredConfig() []ConfigRequirement {
	if s.def == nil {
		return nil
	}
	return s.def.ConfigRequirements
}

// CheckSetup verifies if all required configuration is present.
func (s *ScriptSkill) CheckSetup(vault VaultReader) SetupStatus {
	reqs := s.RequiredConfig()

	// Check for required binaries first
	var missingBins []string
	if s.def.OpenClaw != nil {
		for _, bin := range s.def.OpenClaw.Requires.Bins {
			if _, err := exec.LookPath(bin); err != nil {
				missingBins = append(missingBins, bin)
			}
		}
	}

	// Check for prompt-only skills without required tools
	isPromptOnly := len(s.scripts) == 0

	// Build status
	var missing []ConfigRequirement
	var optionalMissing []ConfigRequirement

	for _, req := range reqs {
		// Check vault first
		hasInVault := vault != nil && vault.Has(req.Key)

		// Also check environment variable as fallback
		hasInEnv := req.EnvVar != "" && os.Getenv(req.EnvVar) != ""

		if !hasInVault && !hasInEnv {
			if req.Required {
				missing = append(missing, req)
			} else {
				optionalMissing = append(optionalMissing, req)
			}
		}
	}

	// If there are missing binaries, add them as setup requirements
	if len(missingBins) > 0 {
		return SetupStatus{
			IsComplete: false,
			MissingRequirements: missing,
			Message: fmt.Sprintf("Skill '%s' requires external tools that are not installed:\n\nMissing binaries: %s\n\nInstall them before using this skill.",
				s.meta.Name, strings.Join(missingBins, ", ")),
		}
	}

	// If prompt-only with no config requirements, it's complete but note it
	if isPromptOnly && len(reqs) == 0 && len(missingBins) == 0 {
		return SetupStatus{
			IsComplete: true,
			Message:    fmt.Sprintf("Skill '%s' is a prompt-only skill (instructions provided, no executable scripts)", s.meta.Name),
		}
	}

	if len(missing) == 0 {
		msg := fmt.Sprintf("Skill '%s' is properly configured", s.meta.Name)
		if len(optionalMissing) > 0 {
			msg += fmt.Sprintf(" (%d optional settings not configured)", len(optionalMissing))
		}
		return SetupStatus{
			IsComplete:         true,
			OptionalMissing:    optionalMissing,
			Message:            msg,
		}
	}

	// Build helpful message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Skill '%s' needs configuration:\n", s.meta.Name))
	for i, req := range missing {
		sb.WriteString(fmt.Sprintf("\n%d. **%s**\n", i+1, req.Name))
		if req.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", req.Description))
		}
		if req.Example != "" {
			sb.WriteString(fmt.Sprintf("   Example: `%s`\n", req.Example))
		}
		if req.EnvVar != "" {
			sb.WriteString(fmt.Sprintf("   Or set env: `%s`\n", req.EnvVar))
		}
	}

	return SetupStatus{
		IsComplete:         false,
		MissingRequirements: missing,
		OptionalMissing:    optionalMissing,
		Message:            sb.String(),
	}
}

// ---------- Script Execution ----------

// runScript executes a specific script through the sandbox.
func (s *ScriptSkill) runScript(ctx context.Context, script SkillScript, input string) (string, error) {
	result, err := s.runner.Run(ctx, &sandbox.ExecRequest{
		Runtime:  script.Runtime,
		Script:   script.Path,
		Args:     parseArgs(input),
		SkillDir: s.def.Dir,
	})
	if err != nil {
		return "", fmt.Errorf("running %s: %w", script.Name, err)
	}

	if result.Killed {
		return "", fmt.Errorf("script killed: %s", result.KillReason)
	}

	output := result.Stdout
	if result.ExitCode != 0 {
		output = fmt.Sprintf("Exit code %d\nStdout: %s\nStderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	// List output files if any.
	if len(result.OutputFiles) > 0 {
		output += "\n\nOutput files:\n"
		for _, f := range result.OutputFiles {
			output += "  - " + f + "\n"
		}
	}

	return output, nil
}

// RunScriptByName runs a specific script by name (used by tool handlers).
func (s *ScriptSkill) RunScriptByName(ctx context.Context, name, args, stdin string) (string, error) {
	for _, script := range s.scripts {
		if sanitizeToolName(script.Name) == name || script.Name == name {
			result, err := s.runner.Run(ctx, &sandbox.ExecRequest{
				Runtime:  script.Runtime,
				Script:   script.Path,
				Args:     parseArgs(args),
				Stdin:    stdin,
				SkillDir: s.def.Dir,
			})
			if err != nil {
				return "", err
			}
			if result.Killed {
				return "", fmt.Errorf("script killed: %s", result.KillReason)
			}
			return result.Stdout, nil
		}
	}
	return "", fmt.Errorf("script %q not found in skill %s", name, s.def.Name)
}

// Scripts returns the list of discovered scripts.
func (s *ScriptSkill) Scripts() []SkillScript {
	return s.scripts
}

// ---------- Discovery ----------

// discoverScripts finds all executable scripts in the skill directory.
func discoverScripts(dir string) []SkillScript {
	var scripts []SkillScript

	// Check scripts/ subdirectory.
	scriptsDir := filepath.Join(dir, "scripts")
	if entries, err := os.ReadDir(scriptsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(scriptsDir, e.Name())
			runtime := sandbox.DetectRuntime(e.Name())
			if runtime != "" {
				scripts = append(scripts, SkillScript{
					Name:    e.Name(),
					Path:    path,
					Runtime: runtime,
				})
			}
		}
	}

	// Also check src/ for Python packages.
	srcDir := filepath.Join(dir, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		// Look for __main__.py or main.py.
		for _, name := range []string{"__main__.py", "main.py"} {
			path := filepath.Join(srcDir, name)
			if _, err := os.Stat(path); err == nil {
				scripts = append(scripts, SkillScript{
					Name:    name,
					Path:    path,
					Runtime: sandbox.RuntimePython,
				})
				break
			}
		}
	}

	return scripts
}

// extractTriggers derives trigger phrases from the skill name and body.
func extractTriggers(body, name string) []string {
	triggers := []string{name}

	// Add name parts (e.g., "web-search" → "web search", "search").
	parts := strings.Split(name, "-")
	if len(parts) > 1 {
		triggers = append(triggers, strings.Join(parts, " "))
	}

	// Extract keywords from headings in the body.
	headingRe := regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	matches := headingRe.FindAllStringSubmatch(body, 10)
	for _, m := range matches {
		heading := strings.TrimSpace(m[1])
		if len(heading) > 3 && len(heading) < 50 {
			triggers = append(triggers, strings.ToLower(heading))
		}
	}

	return triggers
}

// ---------- Helpers ----------

// sanitizeToolName converts a filename to a valid tool name.
func sanitizeToolName(name string) string {
	// Remove extension.
	name = strings.TrimSuffix(name, filepath.Ext(name))
	// Replace non-alphanumeric with underscore.
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	name = re.ReplaceAllString(name, "_")
	// Remove leading/trailing underscores.
	name = strings.Trim(name, "_")
	return strings.ToLower(name)
}

// parseArgs splits a string into command-line arguments.
// Handles basic quoting (single and double quotes).
func parseArgs(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	var inQuote rune

	for _, r := range input {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = r
		case r == ' ' || r == '\t':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}
