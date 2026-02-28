// Package security – audit.go implements security auditing for DevClaw configuration.
// Checks for common misconfigurations, exposed secrets, and security gaps.
package security

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Severity levels for audit findings.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// SecurityChecks is the list of all security check functions run during an audit.
var SecurityChecks = []func(AuditOptions) *AuditFinding{
	checkVaultNotConfigured,
	checkRawAPIKeys,
	checkConfigPermissions,
	checkSessionsPermissions,
	checkGatewayBindNoAuth,
	checkCORSOpen,
	checkSudoAllowed,
	checkSSRFDisabled,
	checkEmbeddingNoKey,
	checkVaultFilePermissions,
}

// AuditFinding represents a single security finding.
type AuditFinding struct {
	CheckID     string `json:"check_id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation"`
}

// AuditReport is the result of a security audit.
type AuditReport struct {
	Timestamp    time.Time      `json:"timestamp"`
	TotalChecks  int            `json:"total_checks"`
	CriticalCount int           `json:"critical_count"`
	WarningCount  int           `json:"warning_count"`
	InfoCount     int           `json:"info_count"`
	Findings     []AuditFinding `json:"findings"`
}

// Summary returns a human-readable summary line.
func (r *AuditReport) Summary() string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("Security audit passed: %d checks, no findings.", r.TotalChecks)
	}
	return fmt.Sprintf("Security audit: %d checks, %d critical, %d warnings, %d info.",
		r.TotalChecks, r.CriticalCount, r.WarningCount, r.InfoCount)
}

// AuditOptions configures which checks to run.
type AuditOptions struct {
	ConfigPath     string            // Path to config.yaml
	SessionsDir    string            // Path to sessions directory
	VaultPath      string            // Path to .devclaw.vault
	VaultConfigured bool             // Whether vault is initialized
	APIKey         string            // Current API key value (for plaintext check)
	Provider       string            // Current LLM provider
	GatewayEnabled bool              // Whether HTTP gateway is enabled
	GatewayBind    string            // Gateway bind address
	GatewayAuth    bool              // Whether gateway auth is configured
	CORSOrigins    []string          // CORS allowed origins
	SSRFEnabled    bool              // Whether SSRF guard is enabled
	SudoAllowed    bool              // Whether sudo is allowed in exec tools
	EmbeddingProvider string         // Embedding provider name
	EmbeddingAPIKey   string         // Embedding API key
	ExtraChecks    map[string]string // Additional key-value pairs for custom checks
}

// RunSecurityAudit executes all security checks and returns a report.
func RunSecurityAudit(opts AuditOptions) *AuditReport {
	report := &AuditReport{
		Timestamp: time.Now(),
	}

	report.TotalChecks = len(SecurityChecks)

	for _, check := range SecurityChecks {
		if finding := check(opts); finding != nil {
			report.Findings = append(report.Findings, *finding)
			switch finding.Severity {
			case SeverityCritical:
				report.CriticalCount++
			case SeverityWarning:
				report.WarningCount++
			case SeverityInfo:
				report.InfoCount++
			}
		}
	}

	return report
}

// ---------- Individual Checks ----------

func checkVaultNotConfigured(opts AuditOptions) *AuditFinding {
	if opts.VaultConfigured {
		return nil
	}
	return &AuditFinding{
		CheckID:     "vault.not_configured",
		Severity:    SeverityWarning,
		Title:       "Vault not configured",
		Detail:      "The encrypted vault is not initialized. API keys may be stored in plaintext config or environment variables.",
		Remediation: "Run 'devclaw vault init' to create an encrypted vault for storing secrets.",
	}
}

func checkRawAPIKeys(opts AuditOptions) *AuditFinding {
	// Check both the main LLM API key and the embedding API key.
	keys := []struct {
		key      string
		label    string
	}{
		{opts.APIKey, opts.Provider + " LLM"},
		{opts.EmbeddingAPIKey, opts.EmbeddingProvider + " embedding"},
	}
	for _, k := range keys {
		if k.key == "" {
			continue
		}
		if strings.HasPrefix(k.key, "vault:") || strings.HasPrefix(k.key, "${") {
			continue
		}
		if looksLikeAPIKey(k.key) {
			return &AuditFinding{
				CheckID:     "config.raw_api_keys",
				Severity:    SeverityCritical,
				Title:       "API key in plaintext",
				Detail:      fmt.Sprintf("The %s API key appears to be stored in plaintext configuration.", k.label),
				Remediation: "Store API keys in the vault: 'devclaw vault set API_KEY <key>'",
			}
		}
	}
	return nil
}

// looksLikeAPIKey checks if a string looks like a real API key using
// known prefixes and entropy heuristics.
func looksLikeAPIKey(s string) bool {
	lower := strings.ToLower(s)

	// Skip test keys.
	if strings.HasPrefix(lower, "sk_test") || strings.HasPrefix(lower, "test_") ||
		strings.HasPrefix(lower, "pk_test") || strings.HasPrefix(lower, "rk_test") {
		return false
	}

	// Known API key prefixes (minimum 15 chars to avoid false positives on short values).
	if len(s) >= 15 {
		knownPrefixes := []string{
			"sk-", "pk-", "ak-", "key-", "api-",
			"ghp_", "gho_", "ghs_", "ghu_",
			"xoxb-", "xoxp-", "xoxs-",
			"ATATT3",
			"AIzaSy", // Google API keys
			"sk-ant-", "sk-proj-",
		}
		for _, prefix := range knownPrefixes {
			if strings.HasPrefix(s, prefix) {
				return true
			}
		}
	}

	// High-entropy heuristic: long strings with mixed character classes.
	if len(s) >= 32 {
		var hasUpper, hasLower, hasDigit bool
		for _, c := range s {
			switch {
			case c >= 'A' && c <= 'Z':
				hasUpper = true
			case c >= 'a' && c <= 'z':
				hasLower = true
			case c >= '0' && c <= '9':
				hasDigit = true
			}
		}
		if hasUpper && hasLower && hasDigit {
			return true
		}
	}

	return false
}

func checkConfigPermissions(opts AuditOptions) *AuditFinding {
	if opts.ConfigPath == "" {
		return nil
	}
	info, err := os.Stat(opts.ConfigPath)
	if err != nil {
		return nil
	}
	perm := info.Mode().Perm()
	// Check if world-readable (others have read permission).
	if perm&0004 != 0 {
		return &AuditFinding{
			CheckID:     "fs.config_permissions",
			Severity:    SeverityWarning,
			Title:       "Config file is world-readable",
			Detail:      fmt.Sprintf("Config file %s has permissions %o. It may contain sensitive data.", opts.ConfigPath, perm),
			Remediation: fmt.Sprintf("chmod 600 %s", opts.ConfigPath),
		}
	}
	return nil
}

func checkSessionsPermissions(opts AuditOptions) *AuditFinding {
	if opts.SessionsDir == "" {
		return nil
	}
	info, err := os.Stat(opts.SessionsDir)
	if err != nil {
		return nil
	}
	perm := info.Mode().Perm()
	if perm&0004 != 0 {
		return &AuditFinding{
			CheckID:     "fs.sessions_permissions",
			Severity:    SeverityWarning,
			Title:       "Sessions directory is world-readable",
			Detail:      fmt.Sprintf("Sessions directory %s has permissions %o. Session data may contain sensitive conversations.", opts.SessionsDir, perm),
			Remediation: fmt.Sprintf("chmod 700 %s", opts.SessionsDir),
		}
	}
	return nil
}

func checkGatewayBindNoAuth(opts AuditOptions) *AuditFinding {
	if !opts.GatewayEnabled {
		return nil
	}
	// Check if gateway is bound to all interfaces without auth.
	bind := opts.GatewayBind
	if bind == "" {
		return nil
	}
	exposedToAll := strings.HasPrefix(bind, "0.0.0.0") || strings.HasPrefix(bind, ":")
	if exposedToAll && !opts.GatewayAuth {
		return &AuditFinding{
			CheckID:     "gateway.bind_no_auth",
			Severity:    SeverityCritical,
			Title:       "Gateway exposed without authentication",
			Detail:      fmt.Sprintf("Gateway is bound to %s without authentication. Anyone on the network can access it.", bind),
			Remediation: "Configure gateway authentication or bind to localhost (127.0.0.1).",
		}
	}
	return nil
}

func checkCORSOpen(opts AuditOptions) *AuditFinding {
	if !opts.GatewayEnabled || len(opts.CORSOrigins) == 0 {
		return nil
	}
	for _, origin := range opts.CORSOrigins {
		if origin == "*" {
			return &AuditFinding{
				CheckID:     "gateway.cors_open",
				Severity:    SeverityWarning,
				Title:       "CORS allows all origins",
				Detail:      "Gateway CORS is configured to allow all origins (*). This may expose the API to cross-origin attacks.",
				Remediation: "Restrict CORS origins to your specific domains.",
			}
		}
	}
	return nil
}

func checkSudoAllowed(opts AuditOptions) *AuditFinding {
	if !opts.SudoAllowed {
		return nil
	}
	return &AuditFinding{
		CheckID:     "tools.sudo_allowed",
		Severity:    SeverityWarning,
		Title:       "Sudo execution allowed",
		Detail:      "The agent is allowed to run commands with sudo. This grants root access to tool executions.",
		Remediation: "Disable sudo in tools configuration unless absolutely required.",
	}
}

func checkSSRFDisabled(opts AuditOptions) *AuditFinding {
	if opts.SSRFEnabled {
		return nil
	}
	return &AuditFinding{
		CheckID:     "ssrf.disabled",
		Severity:    SeverityWarning,
		Title:       "SSRF guard disabled",
		Detail:      "Server-Side Request Forgery protection is disabled. The agent may be tricked into making requests to internal services.",
		Remediation: "Enable SSRF guard in the security configuration.",
	}
}

func checkEmbeddingNoKey(opts AuditOptions) *AuditFinding {
	if opts.EmbeddingProvider == "" || opts.EmbeddingProvider == "none" {
		return nil
	}
	if opts.EmbeddingAPIKey != "" {
		return nil
	}
	return &AuditFinding{
		CheckID:     "embedding.no_key",
		Severity:    SeverityInfo,
		Title:       "Embedding provider configured without API key",
		Detail:      fmt.Sprintf("Embedding provider '%s' is configured but no API key is set. Semantic search will not work.", opts.EmbeddingProvider),
		Remediation: "Set the embedding API key in the vault or environment.",
	}
}

func checkVaultFilePermissions(opts AuditOptions) *AuditFinding {
	if opts.VaultPath == "" {
		return nil
	}
	info, err := os.Stat(opts.VaultPath)
	if err != nil {
		return nil // Vault file doesn't exist, covered by vault.not_configured.
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return &AuditFinding{
			CheckID:     "fs.vault_permissions",
			Severity:    SeverityCritical,
			Title:       "Vault file has open permissions",
			Detail:      fmt.Sprintf("Vault file %s has permissions %o. It should only be readable by the owner.", opts.VaultPath, perm),
			Remediation: fmt.Sprintf("chmod 600 %s", opts.VaultPath),
		}
	}
	return nil
}
