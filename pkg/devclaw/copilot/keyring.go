// Package copilot – keyring.go provides secure credential storage using the
// operating system's native keyring (Linux: Secret Service/GNOME Keyring,
// macOS: Keychain, Windows: Credential Manager).
//
// Priority for resolving secrets:
//  1. Encrypted vault (.devclaw.vault — AES-256-GCM + Argon2, requires master password)
//  2. OS keyring (encrypted by the OS, requires user session)
//  3. Environment variable (DEVCLAW_API_KEY, OPENAI_API_KEY, etc.)
//  4. .env file (loaded by godotenv)
//  5. config.yaml value (least secure — plaintext on disk)
package copilot

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	// keyringService is the service name used in the OS keyring.
	keyringService = "devclaw"

	// keyringAPIKey is the key name for the LLM API key.
	keyringAPIKey = "api_key"
)

// StoreKeyring saves a secret to the OS keyring.
func StoreKeyring(key, value string) error {
	return keyring.Set(keyringService, key, value)
}

// GetKeyring retrieves a secret from the OS keyring.
// Returns empty string if not found.
func GetKeyring(key string) string {
	val, err := keyring.Get(keyringService, key)
	if err != nil {
		return ""
	}
	return val
}

// DeleteKeyring removes a secret from the OS keyring.
func DeleteKeyring(key string) error {
	return keyring.Delete(keyringService, key)
}

// KeyringAvailable checks if the OS keyring is accessible.
func KeyringAvailable() bool {
	// Try a write+delete cycle with a test key.
	testKey := "__devclaw_test__"
	if err := keyring.Set(keyringService, testKey, "test"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, testKey)
	return true
}

// ResolveAPIKey resolves the API key using the priority chain:
// vault → keyring → env var → config value.
// Also updates the config in-place with the resolved value.
// If a vault exists but is locked, it prompts for the master password
// (or uses DEVCLAW_VAULT_PASSWORD env var for non-interactive mode).
// Returns the unlocked vault (or nil if unavailable) so it can be reused
// by the assistant for agent vault tools.
func ResolveAPIKey(cfg *Config, logger *slog.Logger) *Vault {
	// 1. Try encrypted vault first (most secure — password-protected).
	vault := NewVault(VaultFile)
	if vault.Exists() {
		if !vault.IsUnlocked() {
			// Try DEVCLAW_VAULT_PASSWORD env var first (for PM2, systemd, Docker).
			if envPass := os.Getenv("DEVCLAW_VAULT_PASSWORD"); envPass != "" {
				if err := vault.Unlock(envPass); err != nil {
					logger.Warn("failed to unlock vault with DEVCLAW_VAULT_PASSWORD", "error", err)
				} else {
					logger.Info("vault unlocked via DEVCLAW_VAULT_PASSWORD")
				}
			}
		}

		if !vault.IsUnlocked() {
			// Fall back to interactive prompt if stdin is a terminal.
			if term.IsTerminal(int(os.Stdin.Fd())) {
				password, err := ReadPassword("Vault password: ")
				if err != nil {
					logger.Warn("failed to read vault password", "error", err)
				} else if err := vault.Unlock(password); err != nil {
					logger.Warn("failed to unlock vault", "error", err)
				}
			} else {
				logger.Info("vault exists but skipping (non-interactive mode, no DEVCLAW_VAULT_PASSWORD), using env/config")
			}
		}

		if vault.IsUnlocked() {
			// Inject all vault secrets into the process environment so that
			// ${DEVCLAW_*} references in config.yaml resolve correctly.
			// This is the key design: .env only holds the vault password;
			// all other secrets live encrypted in the vault and are injected
			// at runtime.
			injectVaultSecrets(vault, cfg, logger)

			return vault
		}
	}

	// 2. Try OS keyring (encrypted by the OS).
	if val := GetKeyring(keyringAPIKey); val != "" {
		cfg.API.APIKey = val
		logger.Debug("API key loaded from OS keyring")
		return nil
	}

	// 3. If config already has a resolved value (from env expansion), keep it.
	if cfg.API.APIKey != "" && !IsEnvReference(cfg.API.APIKey) {
		logger.Debug("API key loaded from config/env")
		return nil
	}

	logger.Warn("no API key found. Set one with: devclaw config set-key or devclaw config vault-set")
	return nil
}

// injectVaultSecrets reads all secrets from the unlocked vault, sets them as
// environment variables, and resolves known config fields.
// Provider API keys (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.) are injected
// with their original names, allowing LLM clients to find them automatically.
func injectVaultSecrets(vault *Vault, cfg *Config, logger *slog.Logger) {
	// Inject all provider keys into environment variables.
	// This allows LLM clients to find their keys via standard names.
	if err := vault.InjectProviderKeys(); err != nil {
		logger.Warn("failed to inject provider keys", "error", err)
	}

	// Count how many keys were injected.
	keys := vault.List()
	injected := len(keys)

	// Resolve known config fields from the vault.
	// First try provider-specific key based on current provider.
	providerKey := GetProviderKeyName(cfg.API.Provider)
	if val, err := vault.Get(providerKey); err == nil && val != "" {
		cfg.API.APIKey = val
		logger.Debug("API key loaded from encrypted vault", "provider", cfg.API.Provider, "key", providerKey)
	}

	// Also check for DEVCLAW_API_KEY (for DevClaw gateway auth).
	if val, err := vault.Get("DEVCLAW_API_KEY"); err == nil && val != "" {
		os.Setenv("DEVCLAW_API_KEY", val)
		logger.Debug("DEVCLAW_API_KEY loaded from vault")
	}

	// WebUI auth token.
	if val, err := vault.Get("DEVCLAW_WEBUI_TOKEN"); err == nil && val != "" {
		cfg.WebUI.AuthToken = val
		logger.Debug("WebUI auth token loaded from encrypted vault")
	}

	if injected > 0 {
		logger.Info("vault secrets injected into process environment",
			"count", injected)
	}
}

// MigrateKeyToKeyring moves an API key from config/env to the OS keyring
// and clears it from the original location.
func MigrateKeyToKeyring(apiKey string, logger *slog.Logger) error {
	if err := StoreKeyring(keyringAPIKey, apiKey); err != nil {
		return fmt.Errorf("storing in keyring: %w", err)
	}
	logger.Info("API key stored in OS keyring",
		"service", keyringService,
		"hint", "You can now remove it from .env and config.yaml")
	return nil
}
