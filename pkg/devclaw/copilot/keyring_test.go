package copilot

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestInjectVaultSecrets_PopulatesConfig(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	// Store keys with provider-specific names
	v.Set("OPENAI_API_KEY", "sk-test-openai-key")
	v.Set("DEVCLAW_WEBUI_TOKEN", "my-webui-password")
	v.Set("custom_key", "custom-value")

	cfg := DefaultConfig()
	cfg.API.Provider = "openai" // Set provider so it finds the right key
	logger := slog.Default()

	injectVaultSecrets(v, cfg, logger)

	// Check config was populated from provider-specific key
	if cfg.API.APIKey != "sk-test-openai-key" {
		t.Errorf("API.APIKey = %q, want %q", cfg.API.APIKey, "sk-test-openai-key")
	}
	if cfg.WebUI.AuthToken != "my-webui-password" {
		t.Errorf("WebUI.AuthToken = %q, want %q", cfg.WebUI.AuthToken, "my-webui-password")
	}

	// Check env vars were injected by InjectProviderKeys
	if got := os.Getenv("OPENAI_API_KEY"); got != "sk-test-openai-key" {
		t.Errorf("env OPENAI_API_KEY = %q, want %q", got, "sk-test-openai-key")
	}
	if got := os.Getenv("DEVCLAW_WEBUI_TOKEN"); got != "my-webui-password" {
		t.Errorf("env DEVCLAW_WEBUI_TOKEN = %q, want %q", got, "my-webui-password")
	}
}

func TestInjectVaultSecrets_DifferentProviders(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		vaultKey       string
		expectedAPIKey string
	}{
		{"openai", "openai", "OPENAI_API_KEY", "sk-openai-test"},
		{"anthropic", "anthropic", "ANTHROPIC_API_KEY", "sk-ant-test"},
		{"google", "google", "GOOGLE_API_KEY", "ai-google-test"},
		{"groq", "groq", "GROQ_API_KEY", "gsk-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			vaultPath := filepath.Join(dir, "test.vault")
			v := NewVault(vaultPath)
			if err := v.Create("pass"); err != nil {
				t.Fatal(err)
			}

			v.Set(tt.vaultKey, tt.expectedAPIKey)

			cfg := DefaultConfig()
			cfg.API.Provider = tt.provider
			logger := slog.Default()

			injectVaultSecrets(v, cfg, logger)

			if cfg.API.APIKey != tt.expectedAPIKey {
				t.Errorf("API.APIKey = %q, want %q", cfg.API.APIKey, tt.expectedAPIKey)
			}
		})
	}
}

func TestInjectVaultSecrets_UnknownKeysInjectedAsEnvVars(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}
	v.Set("CUSTOM_SECRET", "custom-value")

	cfg := DefaultConfig()
	logger := slog.Default()

	// Clean up env before test.
	os.Unsetenv("CUSTOM_SECRET")

	injectVaultSecrets(v, cfg, logger)

	// With InjectProviderKeys, ALL keys are injected as env vars with their original names
	if got := os.Getenv("CUSTOM_SECRET"); got != "custom-value" {
		t.Errorf("CUSTOM_SECRET = %q, want %q", got, "custom-value")
	}
}

func TestInjectVaultSecrets_EmptyVault(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	logger := slog.Default()

	// Should not panic or error with empty vault.
	injectVaultSecrets(v, cfg, logger)

	if cfg.API.APIKey != "" {
		t.Errorf("expected empty API key, got %q", cfg.API.APIKey)
	}
}
