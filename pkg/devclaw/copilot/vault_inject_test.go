package copilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVaultInjectProviderKeys(t *testing.T) {
	// Create a temporary vault
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, ".test.vault")

	vault := NewVault(vaultPath)

	// Create the vault with a password
	password := "test-password-123"
	if err := vault.Create(password); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	// Store some provider keys
	if err := vault.Set("OPENAI_API_KEY", "sk-test-openai"); err != nil {
		t.Fatalf("failed to set OPENAI_API_KEY: %v", err)
	}
	if err := vault.Set("ANTHROPIC_API_KEY", "sk-ant-test"); err != nil {
		t.Fatalf("failed to set ANTHROPIC_API_KEY: %v", err)
	}
	if err := vault.Set("DEVCLAW_WEBUI_TOKEN", "webui-token"); err != nil {
		t.Fatalf("failed to set DEVCLAW_WEBUI_TOKEN: %v", err)
	}

	// Clear any existing env vars
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("DEVCLAW_WEBUI_TOKEN")

	// Inject provider keys
	if err := vault.InjectProviderKeys(); err != nil {
		t.Fatalf("failed to inject provider keys: %v", err)
	}

	// Verify env vars are set
	if val := os.Getenv("OPENAI_API_KEY"); val != "sk-test-openai" {
		t.Errorf("OPENAI_API_KEY = %q, want %q", val, "sk-test-openai")
	}
	if val := os.Getenv("ANTHROPIC_API_KEY"); val != "sk-ant-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", val, "sk-ant-test")
	}
	if val := os.Getenv("DEVCLAW_WEBUI_TOKEN"); val != "webui-token" {
		t.Errorf("DEVCLAW_WEBUI_TOKEN = %q, want %q", val, "webui-token")
	}
}

func TestVaultInjectProviderKeysLocked(t *testing.T) {
	// Create a temporary vault
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, ".test.vault")

	vault := NewVault(vaultPath)

	// Create the vault with a password
	password := "test-password-123"
	if err := vault.Create(password); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	// Lock the vault
	vault.Lock()

	// Try to inject keys on locked vault - should fail
	err := vault.InjectProviderKeys()
	if err == nil {
		t.Error("expected error when injecting keys on locked vault")
	}
}

func TestVaultInjectProviderKeysEmpty(t *testing.T) {
	// Create a temporary vault
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, ".test.vault")

	vault := NewVault(vaultPath)

	// Create the vault with a password
	password := "test-password-123"
	if err := vault.Create(password); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	// Don't store any keys

	// Clear any existing env vars
	os.Unsetenv("OPENAI_API_KEY")

	// Inject provider keys (should succeed with no keys)
	if err := vault.InjectProviderKeys(); err != nil {
		t.Fatalf("failed to inject empty provider keys: %v", err)
	}

	// Verify env vars are not set
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		t.Errorf("OPENAI_API_KEY = %q, want empty", val)
	}
}
