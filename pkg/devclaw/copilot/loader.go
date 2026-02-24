// Package copilot – loader.go handles loading configuration from YAML files
// with secure credential management via environment variables and .env files.
package copilot

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// envVarPattern matches environment variable patterns in config values:
//   - ${VAR_NAME}         - simple variable
//   - ${VAR_NAME:-default} - default value if not set
//   - ${VAR_NAME:?error}   - error message if not set
//   - $VAR_NAME           - bare variable (no default/error support)
//
// Capture groups:
//   - Group 1: Variable name (for ${} syntax)
//   - Group 2: Modifier type ("-" for default, "?" for error)
//   - Group 3: Default value or error message
//   - Group 4: Variable name (for bare $VAR syntax)
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::(-|\?)([^}]*))?\}|\$([A-Z_][A-Z0-9_]*)`)

// LoadConfigFromFile reads and parses a YAML configuration file.
// Automatically loads .env files and expands environment variables.
// Returns an error if any ${VAR:?error} pattern has its variable unset.
func LoadConfigFromFile(path string) (*Config, error) {
	// Load .env files (silently ignore if not found).
	loadEnvFiles()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables in YAML before parsing.
	// This validates ${VAR:?error} patterns and returns error if required vars are missing.
	expanded, err := expandEnvVarsWithValidation(string(data))
	if err != nil {
		return nil, fmt.Errorf("expanding environment variables: %w", err)
	}

	cfg, err := ParseConfig([]byte(expanded))
	if err != nil {
		return nil, err
	}

	// Resolve secrets from environment (override empty/placeholder values).
	resolveSecrets(cfg)

	// Resolve relative paths based on config file location.
	resolveRelativePaths(cfg, path)

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

	// YAML unmarshal zeros bool fields when absent. Merge with defaults so
	// vision/transcription are enabled out of the box, and partial media
	// sections (e.g. only vision_model) don't accidentally disable features.
	if _, hasMedia := raw["media"]; !hasMedia {
		cfg.Media = DefaultMediaConfig()
	} else {
		defaults := DefaultMediaConfig()
		mediaMap, _ := raw["media"].(map[string]any)
		if _, set := mediaMap["vision_enabled"]; !set {
			cfg.Media.VisionEnabled = defaults.VisionEnabled
		}
		if _, set := mediaMap["transcription_enabled"]; !set {
			cfg.Media.TranscriptionEnabled = defaults.TranscriptionEnabled
		}
	}

	return cfg, nil
}

// SaveConfigToFile writes a Config as YAML to the specified path.
// Secrets are replaced with environment variable references.
// Creates a backup (.bak) of the existing file before overwriting to prevent
// data loss from crashes or invalid writes.
func SaveConfigToFile(cfg *Config, path string) error {
	// Create a copy to sanitize before writing.
	sanitized := *cfg

	// Sanitize API key - try provider-specific env var first, then fallback.
	// The vault injects keys as OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.
	apiKeyEnvVar := GetProviderKeyName(cfg.API.Provider)
	sanitized.API.APIKey = sanitizeSecretWithFallback(cfg.API.APIKey, apiKeyEnvVar, "DEVCLAW_API_KEY")
	sanitized.Media.TranscriptionAPIKey = sanitizeSecret(cfg.Media.TranscriptionAPIKey, "DEVCLAW_TRANSCRIPTION_API_KEY")

	data, err := yaml.Marshal(&sanitized)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Validate the marshaled YAML is parseable before writing (sanity check).
	var check map[string]any
	if err := yaml.Unmarshal(data, &check); err != nil {
		return fmt.Errorf("config validation failed (refusing to write corrupt data): %w", err)
	}

	// Backup existing file before overwriting.
	if _, err := os.Stat(path); err == nil {
		bakPath := path + ".bak"
		if existing, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(bakPath, existing, 0o600)
		}
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
		"devclaw.yaml",
		"devclaw.yml",
		"configs/config.yaml",
		"configs/devclaw.yaml",
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
				"Use environment variable DEVCLAW_API_KEY instead.",
				"hint", "Set 'api_key: ${DEVCLAW_API_KEY}' in config.yaml")
		}
	}
}

// ---------- Internal ----------

// loadEnvFiles loads .env files from standard locations.
// By default, godotenv does NOT overwrite existing env vars.
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

// loadEnvFilesWithOverride loads .env files and OVERRITES existing env vars.
// This is used for hot-reloading credentials without restart.
func loadEnvFilesWithOverride() error {
	envFiles := []string{
		".env",
		".env.local",
	}

	for _, f := range envFiles {
		// Read and parse the file manually to allow override.
		data, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading %s: %w", f, err)
		}

		// Parse the env file.
		envMap, err := godotenv.Parse(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", f, err)
		}

		// Set each variable, overriding existing values.
		for key, value := range envMap {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
		}
	}
	return nil
}

// ReloadEnvFiles forces a reload of .env files with override.
// Returns the number of variables loaded.
func ReloadEnvFiles() (int, error) {
	envFiles := []string{".env.local", ".env"} // .env.local takes precedence
	loaded := 0

	for _, f := range envFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, fmt.Errorf("reading %s: %w", f, err)
		}

		envMap, err := godotenv.Parse(bytes.NewReader(data))
		if err != nil {
			return 0, fmt.Errorf("parsing %s: %w", f, err)
		}

		for key, value := range envMap {
			if err := os.Setenv(key, value); err != nil {
				return 0, fmt.Errorf("setting %s: %w", key, err)
			}
			loaded++
		}
	}

	return loaded, nil
}

// expandEnvVars replaces ${VAR}, ${VAR:-default}, ${VAR:?error}, and $VAR
// references in a string with their environment variable values.
//
// Supported patterns:
//   - ${VAR}           - use VAR value, keep placeholder if unset
//   - ${VAR:-default}  - use VAR value, or "default" if unset
//   - ${VAR:?error}    - use VAR value, or return error message if unset
//   - $VAR             - use VAR value, keep placeholder if unset
//
// The ${VAR:?error} pattern is handled specially: if the variable is unset,
// the function returns the original match prefixed with "ERROR:" to signal
// an error condition that should be caught during validation.
func expandEnvVars(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Use FindStringSubmatch to get capture groups.
		// Groups: 1=varName, 2=modifierType(-|?), 3=value, 4=bareVar
		submatches := envVarPattern.FindStringSubmatch(match)

		var varName, modifierType, modifierValue, bareVar string
		if len(submatches) >= 2 {
			varName = submatches[1] // ${VAR...} syntax
		}
		if len(submatches) >= 3 {
			modifierType = submatches[2] // "-" or "?"
		}
		if len(submatches) >= 4 {
			modifierValue = submatches[3] // default value or error message
		}
		if len(submatches) >= 5 {
			bareVar = submatches[4] // $VAR syntax
		}

		// Handle bare $VAR syntax.
		if bareVar != "" {
			if val, ok := os.LookupEnv(bareVar); ok {
				return val
			}
			return match // Keep placeholder if unset.
		}

		// Handle ${VAR} syntax with optional modifier.
		if varName != "" {
			if val, ok := os.LookupEnv(varName); ok {
				return val
			}

			// Variable not set - check for modifier.
			if modifierType != "" {
				// Check for :?error pattern.
				if modifierType == "?" {
					// Return error indicator. The caller can detect this
					// by checking for the ERROR: prefix.
					errorMsg := modifierValue
					if errorMsg == "" {
						errorMsg = "required environment variable not set"
					}
					return "ERROR:" + varName + ":" + errorMsg
				}
				// It's a :-default pattern.
				return modifierValue
			}
			// No modifier, keep placeholder.
			return match
		}

		return match
	})
}

// expandEnvVarsWithValidation is like expandEnvVars but returns an error
// if any ${VAR:?error} pattern has its variable unset.
func expandEnvVarsWithValidation(input string) (string, error) {
	result := expandEnvVars(input)
	if strings.Contains(result, "ERROR:") {
		// Extract the error details.
		// Format: ERROR:VAR_NAME:error message
		// The error message can contain any characters including spaces and colons.
		idx := strings.Index(result, "ERROR:")
		rest := result[idx+6:] // Skip "ERROR:"
		// Find the colon after VAR_NAME.
		colonIdx := strings.Index(rest, ":")
		if colonIdx == -1 {
			return "", fmt.Errorf("config error: malformed error marker")
		}
		varName := rest[:colonIdx]
		errorMsg := rest[colonIdx+1:]
		if errorMsg == "" {
			errorMsg = "required environment variable not set"
		}
		return "", fmt.Errorf("config error: %s - %s", varName, errorMsg)
	}
	return result, nil
}

// resolveSecrets fills in config secrets from environment variables
// when the config value is empty or a placeholder.
func resolveSecrets(cfg *Config) {
	// API key: DEVCLAW_API_KEY or OPENAI_API_KEY.
	if cfg.API.APIKey == "" || IsEnvReference(cfg.API.APIKey) {
		if key := os.Getenv("DEVCLAW_API_KEY"); key != "" {
			cfg.API.APIKey = key
		} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			cfg.API.APIKey = key
		} else if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			cfg.API.APIKey = key
		}
	}
}

// resolveRelativePaths converts relative paths to absolute paths based on
// the config file's directory. This ensures paths work correctly regardless
// of the current working directory when devclaw is started.
func resolveRelativePaths(cfg *Config, configPath string) {
	// Get the directory containing the config file.
	configDir := filepath.Dir(configPath)

	// Resolve skills directories.
	for i, dir := range cfg.Skills.ClawdHubDirs {
		cfg.Skills.ClawdHubDirs[i] = resolvePathFromConfig(dir, configDir)
	}

	// Resolve memory path.
	if cfg.Memory.Path != "" {
		cfg.Memory.Path = resolvePathFromConfig(cfg.Memory.Path, configDir)
	}

	// Resolve scheduler storage path.
	if cfg.Scheduler.Storage != "" {
		cfg.Scheduler.Storage = resolvePathFromConfig(cfg.Scheduler.Storage, configDir)
	}

	// Resolve plugins directory.
	if cfg.Plugins.Dir != "" {
		cfg.Plugins.Dir = resolvePathFromConfig(cfg.Plugins.Dir, configDir)
	}
}

// resolvePathFromConfig converts a path to absolute, resolving relative paths
// against the config file's directory. Expands ~ to home directory.
func resolvePathFromConfig(path, configDir string) string {
	if path == "" {
		return path
	}

	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}

	// If already absolute, return as-is.
	if filepath.IsAbs(path) {
		return path
	}

	// Make relative path absolute based on config directory.
	return filepath.Join(configDir, path)
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

// sanitizeSecretWithFallback tries multiple env vars before returning the value.
// This handles the case where the vault injects OPENAI_API_KEY but the config
// might reference DEVCLAW_API_KEY.
func sanitizeSecretWithFallback(value, primaryEnvVar, fallbackEnvVar string) string {
	if value == "" || IsEnvReference(value) {
		return value
	}
	// Try primary env var first (e.g., OPENAI_API_KEY)
	if os.Getenv(primaryEnvVar) == value {
		return "${" + primaryEnvVar + "}"
	}
	// Try fallback env var (e.g., DEVCLAW_API_KEY)
	if os.Getenv(fallbackEnvVar) == value {
		return "${" + fallbackEnvVar + "}"
	}
	// If primary env var exists (vault injected it after UI save), use the reference.
	// This handles the case where user saved via UI, vault stored it, but value doesn't match
	// (e.g., value was modified or this is a new key being set).
	if os.Getenv(primaryEnvVar) != "" {
		return "${" + primaryEnvVar + "}"
	}
	// Fallback env var exists - use its reference
	if os.Getenv(fallbackEnvVar) != "" {
		return "${" + fallbackEnvVar + "}"
	}
	// Value doesn't match any env var - clear it to force vault lookup
	// This prevents hardcoded keys from being saved to config
	return ""
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
