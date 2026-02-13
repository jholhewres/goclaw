// Package copilot – loader.go handles loading configuration from YAML files
// with secure credential management via environment variables and .env files.
package copilot

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} or $VAR_NAME in config values.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Z_][A-Z0-9_]*)`)

// LoadConfigFromFile reads and parses a YAML configuration file.
// Automatically loads .env files and expands environment variables.
func LoadConfigFromFile(path string) (*Config, error) {
	// Load .env files (silently ignore if not found).
	loadEnvFiles()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables in YAML before parsing.
	expanded := expandEnvVars(string(data))

	cfg, err := ParseConfig([]byte(expanded))
	if err != nil {
		return nil, err
	}

	// Resolve secrets from environment (override empty/placeholder values).
	resolveSecrets(cfg)

	// Check file permissions and warn if too open.
	checkFilePermissions(path)

	return cfg, nil
}

// ParseConfig parses YAML bytes into a Config.
// Starts with defaults and overlays values from the YAML.
func ParseConfig(data []byte) (*Config, error) {
	cfg := DefaultConfig()

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("mapping config: %w", err)
	}

	return cfg, nil
}

// SaveConfigToFile writes a Config as YAML to the specified path.
// Secrets are replaced with environment variable references.
func SaveConfigToFile(cfg *Config, path string) error {
	// Create a copy to sanitize before writing.
	sanitized := *cfg
	sanitized.API.APIKey = sanitizeSecret(cfg.API.APIKey, "GOCLAW_API_KEY")

	data, err := yaml.Marshal(&sanitized)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Write with restricted permissions (owner read/write only).
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// FindConfigFile searches for config files in standard locations.
func FindConfigFile() string {
	candidates := []string{
		"config.yaml",
		"config.yml",
		"copilot.yaml",
		"copilot.yml",
		"configs/config.yaml",
		"configs/copilot.yaml",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// AuditSecrets checks for hardcoded secrets and logs warnings.
// Should be called on startup to alert the user.
func AuditSecrets(cfg *Config, logger *slog.Logger) {
	if cfg.API.APIKey != "" && !IsEnvReference(cfg.API.APIKey) {
		if looksLikeRealKey(cfg.API.APIKey) {
			logger.Warn("API key appears to be hardcoded in config. "+
				"Use environment variable GOCLAW_API_KEY instead.",
				"hint", "Set 'api_key: ${GOCLAW_API_KEY}' in config.yaml")
		}
	}
}

// ---------- Internal ----------

// loadEnvFiles loads .env files from standard locations.
func loadEnvFiles() {
	envFiles := []string{
		".env",
		".env.local",
	}

	for _, f := range envFiles {
		// godotenv.Load does NOT overwrite existing env vars.
		_ = godotenv.Load(f)
	}
}

// expandEnvVars replaces ${VAR} and $VAR references in a string
// with their environment variable values.
func expandEnvVars(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR} or $VAR.
		var varName string
		if strings.HasPrefix(match, "${") {
			varName = match[2 : len(match)-1]
		} else {
			varName = match[1:]
		}

		if val, ok := os.LookupEnv(varName); ok {
			return val
		}

		// Return original if env var not set (allows placeholder to remain).
		return match
	})
}

// resolveSecrets fills in config secrets from environment variables
// when the config value is empty or a placeholder.
func resolveSecrets(cfg *Config) {
	// API key: GOCLAW_API_KEY or OPENAI_API_KEY.
	if cfg.API.APIKey == "" || IsEnvReference(cfg.API.APIKey) {
		if key := os.Getenv("GOCLAW_API_KEY"); key != "" {
			cfg.API.APIKey = key
		} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			cfg.API.APIKey = key
		} else if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			cfg.API.APIKey = key
		}
	}
}

// sanitizeSecret replaces a real secret with an env var reference
// for safe storage in config files.
func sanitizeSecret(value, envVar string) string {
	if value == "" || IsEnvReference(value) {
		return value
	}
	// If the env var is already set with this value, use the reference.
	if os.Getenv(envVar) == value {
		return "${" + envVar + "}"
	}
	// Return as-is (user explicitly put it in config).
	return value
}

// IsEnvReference checks if a string is an environment variable reference.
func IsEnvReference(s string) bool {
	return strings.HasPrefix(s, "${") || strings.HasPrefix(s, "$")
}

// looksLikeRealKey heuristically checks if a string looks like a real API key
// (not a placeholder or env var reference).
func looksLikeRealKey(s string) bool {
	if IsEnvReference(s) {
		return false
	}
	// OpenAI keys start with "sk-"
	if strings.HasPrefix(s, "sk-") {
		return true
	}
	// Anthropic keys start with "sk-ant-"
	if strings.HasPrefix(s, "sk-ant-") {
		return true
	}
	// Generic: long alphanumeric strings are likely real keys.
	if len(s) > 20 {
		return true
	}
	return false
}

// checkFilePermissions warns if config file is world-readable.
func checkFilePermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	mode := info.Mode().Perm()
	// Warn if group or others can read (on Unix).
	if mode&0o044 != 0 {
		slog.Warn("config file has open permissions, consider restricting",
			"path", path,
			"current", fmt.Sprintf("%04o", mode),
			"recommended", "0600",
			"fix", fmt.Sprintf("chmod 600 %s", path),
		)
	}
}
