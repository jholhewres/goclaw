// Package sandbox – policy.go implements the security policy for
// script execution: command allowlisting, environment variable
// filtering, and script content scanning.
package sandbox

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Policy enforces security rules on script execution requests.
type Policy struct {
	cfg Config

	// allowedBins is the set of binaries considered safe to execute.
	allowedBins map[string]bool

	// blockedEnvSet is the set of environment variables to strip.
	blockedEnvSet map[string]bool

	// allowedEnvSet is the whitelist of allowed env vars. If empty,
	// all non-blocked vars are allowed.
	allowedEnvSet map[string]bool

	// scanRules are regex patterns for detecting dangerous content.
	scanRules []ScanRule
}

// ScanRule defines a pattern to detect in script content.
type ScanRule struct {
	Name     string
	Severity string // "critical", "warn"
	Pattern  *regexp.Regexp
	Message  string
}

// ScanResult reports a detected issue in script content.
type ScanResult struct {
	Rule     string
	Severity string
	Message  string
	Line     int
	Content  string
}

// NewPolicy creates a new Policy from the sandbox config.
func NewPolicy(cfg Config) *Policy {
	p := &Policy{
		cfg:           cfg,
		allowedBins:   defaultAllowedBins(),
		blockedEnvSet: make(map[string]bool),
		allowedEnvSet: make(map[string]bool),
		scanRules:     defaultScanRules(),
	}

	for _, env := range cfg.BlockedEnv {
		p.blockedEnvSet[env] = true
	}
	for _, env := range cfg.AllowedEnv {
		p.allowedEnvSet[env] = true
	}

	return p
}

// Validate checks whether an execution request is allowed.
func (p *Policy) Validate(req *ExecRequest) error {
	// For IsolationNone, we trust the skill — minimal checks.
	if req.Isolation == IsolationNone {
		return nil
	}

	// Check the script file exists and is readable.
	if req.Script != "" {
		info, err := os.Stat(req.Script)
		if err != nil {
			return fmt.Errorf("script not found: %s", req.Script)
		}
		if info.IsDir() {
			return fmt.Errorf("script path is a directory: %s", req.Script)
		}
	}

	return nil
}

// FilterEnv filters environment variables based on the policy.
// Returns a new map with only allowed variables.
func (p *Policy) FilterEnv(env map[string]string) map[string]string {
	filtered := make(map[string]string)

	for k, v := range env {
		// Always block dangerous variables (exact match).
		if p.blockedEnvSet[k] {
			continue
		}

		// Block dangerous variable prefixes (e.g. LD_*, DYLD_*).
		if hasBlockedPrefix(k) {
			continue
		}

		// If allowlist is configured, only allow listed vars.
		if len(p.allowedEnvSet) > 0 {
			if !p.allowedEnvSet[k] {
				continue
			}
		}

		filtered[k] = v
	}

	return filtered
}

// hasBlockedPrefix checks if an env var name matches any blocked prefix.
func hasBlockedPrefix(name string) bool {
	for _, prefix := range blockedEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ScanScript analyzes script content for dangerous patterns.
// Returns a list of findings. Critical findings should block execution.
func (p *Policy) ScanScript(content string) []ScanResult {
	var results []ScanResult
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		for _, rule := range p.scanRules {
			if rule.Pattern.MatchString(line) {
				results = append(results, ScanResult{
					Rule:     rule.Name,
					Severity: rule.Severity,
					Message:  rule.Message,
					Line:     i + 1,
					Content:  strings.TrimSpace(line),
				})
			}
		}
	}
	return results
}

// HasCritical checks if any scan result is critical severity.
func HasCritical(results []ScanResult) bool {
	for _, r := range results {
		if r.Severity == "critical" {
			return true
		}
	}
	return false
}

// IsEnvAllowed checks if an environment variable is allowed.
func (p *Policy) IsEnvAllowed(name string) bool {
	if p.blockedEnvSet[name] {
		return false
	}
	if len(p.allowedEnvSet) > 0 {
		return p.allowedEnvSet[name]
	}
	return true
}

// AddAllowedBin adds a binary to the safe execution list.
func (p *Policy) AddAllowedBin(bin string) {
	p.allowedBins[bin] = true
}

// IsBinAllowed checks if a binary is in the allowlist.
func (p *Policy) IsBinAllowed(bin string) bool {
	return p.allowedBins[bin]
}

// ---------- Defaults ----------

// defaultAllowedBins returns the default set of safe binaries.
// Modeled after a curated list of safe binaries.
func defaultAllowedBins() map[string]bool {
	return map[string]bool{
		// Interpreters
		"python3": true, "python": true,
		"node": true, "npx": true, "bun": true,
		"sh": true, "bash": true,
		// Package managers (for uv run, etc.)
		"uv": true, "pip": true, "npm": true,
		// Common safe utilities
		"jq": true, "yq": true,
		"grep": true, "rg": true,
		"cut": true, "sort": true, "uniq": true,
		"head": true, "tail": true,
		"tr": true, "wc": true,
		"cat": true, "echo": true, "printf": true,
		"date": true, "env": true,
		"curl": true, "wget": true,
		"base64": true, "sha256sum": true, "md5sum": true,
		// Version control
		"git": true, "gh": true,
		// Media
		"ffmpeg": true, "ffprobe": true,
		"convert": true, "identify": true,
		// Skill-specific CLIs
		"summarize": true, "gog": true, "op": true,
		"obsidian-cli": true, "tmux": true,
	}
}

// defaultScanRules returns patterns for detecting dangerous script content.
func defaultScanRules() []ScanRule {
	return []ScanRule{
		// -- Critical: Code injection --
		{
			Name:     "python-exec",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)\b(exec|eval)\s*\(`),
			Message:  "Dynamic code execution detected (exec/eval)",
		},
		{
			Name:     "python-subprocess-shell",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`subprocess\.(call|run|Popen)\s*\([^)]*shell\s*=\s*True`),
			Message:  "Subprocess with shell=True (command injection risk)",
		},
		{
			Name:     "node-eval",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`\b(eval|new\s+Function)\s*\(`),
			Message:  "Dynamic code execution (eval/new Function)",
		},
		{
			Name:     "node-child-process",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`require\s*\(\s*['"]child_process['"]\s*\)`),
			Message:  "Direct child_process import (exec/spawn)",
		},

		// -- Critical: Crypto mining --
		{
			Name:     "crypto-mining",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(stratum\+tcp|coinhive|xmrig|cryptonight|monero.*pool|mining.*pool)`),
			Message:  "Possible crypto mining detected",
		},

		// -- Critical: Reverse shell --
		{
			Name:     "reverse-shell",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(\/dev\/tcp\/|nc\s+-[a-z]*e|bash\s+-i\s+>&|python.*socket.*connect)`),
			Message:  "Possible reverse shell detected",
		},

		// -- Warn: Data exfiltration --
		{
			Name:     "exfiltration",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`(?i)(readFile|open\s*\([^)]*\/etc\/(passwd|shadow)|\.ssh\/)`),
			Message:  "Potential data exfiltration (sensitive file access)",
		},

		// -- Warn: Obfuscated code --
		{
			Name:     "obfuscation-hex",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`\\x[0-9a-fA-F]{2}(\\x[0-9a-fA-F]{2}){10,}`),
			Message:  "Possible obfuscated code (hex-encoded strings)",
		},
		{
			Name:     "obfuscation-base64-exec",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`(?i)(base64.*decode|atob)\s*\([^)]+\)\s*\)`),
			Message:  "Base64 decode + execute pattern",
		},

		// -- Warn: Suspicious network --
		{
			Name:     "suspicious-network",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`(?i)(websocket|ws://|wss://)\s*.*:\d{4,5}`),
			Message:  "Suspicious WebSocket connection to non-standard port",
		},

		// -- Warn: Environment manipulation --
		{
			Name:     "env-manipulation",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`(?i)(os\.environ|process\.env)\s*\[.*\]\s*=`),
			Message:  "Environment variable manipulation detected",
		},

		// -- Critical: Shell env var injection in Python/Node scripts --
		// Detects when LLM generates scripts that reference shell-style
		// environment variables (e.g. $DM_JSON, $TMPDIR), which indicates
		// the model confused shell with the target language.
		{
			Name:     "shell-env-injection",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`\$[A-Z_][A-Z0-9_]{2,}`),
			Message:  "Shell-style environment variable reference in script (possible language confusion)",
		},
	}
}

// defaultShellScanRules returns patterns specific to shell script scanning.
// These rules are separate from defaultScanRules because shell scripts use
// $VAR syntax legitimately (so the shell-env-injection rule does not apply).
func defaultShellScanRules() []ScanRule {
	return []ScanRule{
		// -- Critical: Reverse shell --
		{
			Name:     "shell-reverse-shell",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(\/dev\/tcp\/|nc\s+-[a-z]*e|bash\s+-i\s+>&|python.*socket.*connect)`),
			Message:  "Possible reverse shell detected",
		},

		// -- Critical: Output-redirect flags that write to arbitrary files.
		// sort -o, grep/jq -f (read filter from file, can be used to probe),
		// curl -o/-O and wget -O write attacker-controlled content to disk.
		{
			Name:     "shell-file-write-flag",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`\b(sort\s+.*-o|grep\s+.*-f\s+\S|jq\s+.*-f\s+\S|curl\s+.*-[oO]\s+\S|wget\s+.*-O\s+\S)`),
			Message:  "Tool invoked with flag that writes output to a file path (-o/-O/-f); potential file write outside temp directory",
		},

		// -- Critical: File-existence oracle via error messages.
		// Patterns like "test -e /etc/shadow && echo yes" or
		// "ls /home/user/.ssh/id_rsa" let scripts probe for sensitive files
		// even when they cannot read them.
		{
			Name:     "shell-file-existence-probe",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(test\s+-[efdrs]\s+\/(?:etc|root|home|proc|sys)|ls\s+\/(?:etc|root|home|proc|sys)|stat\s+\/(?:etc|root|home|proc|sys)|\[\s+-[efdrs]\s+\/(?:etc|root|home|proc|sys))`),
			Message:  "Possible file-existence probe on sensitive system path",
		},

		// -- Critical: Reading /etc/passwd, /etc/shadow, SSH keys --
		{
			Name:     "shell-sensitive-read",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(cat|head|tail|less|more|grep)\s+.*\/(?:etc\/(?:passwd|shadow|sudoers)|root\/|\.ssh\/)`),
			Message:  "Attempt to read sensitive system file",
		},

		// -- Critical: Crypto mining --
		{
			Name:     "shell-crypto-mining",
			Severity: "critical",
			Pattern:  regexp.MustCompile(`(?i)(stratum\+tcp|coinhive|xmrig|cryptonight|monero.*pool|mining.*pool)`),
			Message:  "Possible crypto mining detected",
		},

		// -- Warn: Obfuscated payload via base64 | eval --
		{
			Name:     "shell-base64-exec",
			Severity: "warn",
			Pattern:  regexp.MustCompile(`(?i)(base64\s+.*\|\s*(bash|sh|eval)|echo\s+[A-Za-z0-9+/]{20,}.*\|\s*(base64|bash|sh))`),
			Message:  "Base64-encoded payload piped to shell interpreter",
		},
	}
}

// ScanShellScript analyzes shell script content for dangerous patterns.
// Shell scripts are scanned with a separate rule set because $VAR syntax
// is valid in shell and should not trigger the shell-env-injection rule.
func (p *Policy) ScanShellScript(content string) []ScanResult {
	var results []ScanResult
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		for _, rule := range defaultShellScanRules() {
			if rule.Pattern.MatchString(line) {
				results = append(results, ScanResult{
					Rule:     rule.Name,
					Severity: rule.Severity,
					Message:  rule.Message,
					Line:     i + 1,
					Content:  strings.TrimSpace(line),
				})
			}
		}
	}
	return results
}
