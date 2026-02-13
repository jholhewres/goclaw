// Package copilot – keyring.go provides secure credential storage using the
// operating system's native keyring (Linux: Secret Service/GNOME Keyring,
// macOS: Keychain, Windows: Credential Manager).
//
// Priority for resolving secrets:
//  1. Encrypted vault (.goclaw.vault — AES-256-GCM + Argon2, requires master password)
//  2. OS keyring (encrypted by the OS, requires user session)
//  3. Environment variable (GOCLAW_API_KEY, OPENAI_API_KEY, etc.)
//  4. .env file (loaded by godotenv)
//  5. config.yaml value (least secure — plaintext on disk)
package copilot

import (
	"fmt"
	"log/slog"

	"github.com/zalando/go-keyring"
)

const (
	// keyringService is the service name used in the OS keyring.
	keyringService = "goclaw"

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
	testKey := "__goclaw_test__"
	if err := keyring.Set(keyringService, testKey, "test"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, testKey)
	return true
}

// ResolveAPIKey resolves the API key using the priority chain:
// vault → keyring → env var → config value.
// Also updates the config in-place with the resolved value.
// If a vault exists but is locked, it prompts for the master password.
func ResolveAPIKey(cfg *Config, logger *slog.Logger) {
	// 1. Try encrypted vault first (most secure — password-protected).
	vault := NewVault(VaultFile)
	if vault.Exists() {
		if !vault.IsUnlocked() {
			password, err := ReadPassword("Vault password: ")
			if err != nil {
				logger.Warn("failed to read vault password", "error", err)
			} else if err := vault.Unlock(password); err != nil {
				logger.Warn("failed to unlock vault", "error", err)
			}
		}

		if vault.IsUnlocked() {
			if val, err := vault.Get(keyringAPIKey); err == nil && val != "" {
				cfg.API.APIKey = val
				logger.Debug("API key loaded from encrypted vault")
				vault.Lock()
				return
			}
			vault.Lock()
		}
	}

	// 2. Try OS keyring (encrypted by the OS).
	if val := GetKeyring(keyringAPIKey); val != "" {
		cfg.API.APIKey = val
		logger.Debug("API key loaded from OS keyring")
		return
	}

	// 3. If config already has a resolved value (from env expansion), keep it.
	if cfg.API.APIKey != "" && !IsEnvReference(cfg.API.APIKey) {
		logger.Debug("API key loaded from config/env")
		return
	}

	logger.Warn("no API key found. Set one with: copilot config set-key or copilot config vault-set")
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
