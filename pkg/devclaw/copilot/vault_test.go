package copilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVaultCreate(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	t.Run("creates new vault", func(t *testing.T) {
		err := vault.Create("test-password-123")
		if err != nil {
			t.Fatalf("failed to create vault: %v", err)
		}

		if !vault.Exists() {
			t.Error("vault should exist after creation")
		}
	})

	t.Run("cannot create if already exists", func(t *testing.T) {
		err := vault.Create("different-password")
		if err == nil {
			t.Error("expected error when creating existing vault")
		}
	})
}

func TestVaultUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Create vault first
	if err := vault.Create("correct-password"); err != nil {
		t.Fatalf("setup: failed to create vault: %v", err)
	}

	// Note: __verify__ entry is only created on first Set(), so we need to
	// set a value to enable password verification
	if err := vault.Unlock("correct-password"); err != nil {
		t.Fatalf("setup: failed to unlock for initial set: %v", err)
	}
	vault.Set("initial_key", "initial_value")
	vault.Lock()

	t.Run("unlocks with correct password", func(t *testing.T) {
		err := vault.Unlock("correct-password")
		if err != nil {
			t.Fatalf("failed to unlock: %v", err)
		}

		if !vault.IsUnlocked() {
			t.Error("vault should be unlocked")
		}
	})

	t.Run("fails with wrong password", func(t *testing.T) {
		vault.Lock()
		err := vault.Unlock("wrong-password")
		if err == nil {
			t.Error("expected error with wrong password")
		}

		if vault.IsUnlocked() {
			t.Error("vault should not be unlocked with wrong password")
		}
	})

	t.Run("fails if vault doesn't exist", func(t *testing.T) {
		nonExistent := NewVault(filepath.Join(tmpDir, "nonexistent.vault"))
		err := nonExistent.Unlock("any-password")
		if err == nil {
			t.Error("expected error when unlocking non-existent vault")
		}
	})
}

func TestVaultSetGet(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("sets and gets value", func(t *testing.T) {
		err := vault.Set("api_key", "secret-api-key-12345")
		if err != nil {
			t.Fatalf("failed to set: %v", err)
		}

		val, err := vault.Get("api_key")
		if err != nil {
			t.Fatalf("failed to get: %v", err)
		}

		if val != "secret-api-key-12345" {
			t.Errorf("expected 'secret-api-key-12345', got %q", val)
		}
	})

	t.Run("returns empty for non-existent key", func(t *testing.T) {
		val, err := vault.Get("nonexistent")
		if err != nil {
			t.Errorf("unexpected error for non-existent key: %v", err)
		}
		if val != "" {
			t.Errorf("expected empty string, got %q", val)
		}
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		vault.Set("key1", "value1")
		vault.Set("key1", "value2")

		val, _ := vault.Get("key1")
		if val != "value2" {
			t.Errorf("expected 'value2', got %q", val)
		}
	})

	t.Run("stores multiple keys", func(t *testing.T) {
		vault.Set("key_a", "value_a")
		vault.Set("key_b", "value_b")
		vault.Set("key_c", "value_c")

		keys, err := vault.Keys()
		if err != nil {
			t.Fatalf("failed to list keys: %v", err)
		}

		// Should have at least our keys (plus __verify__)
		if len(keys) < 3 {
			t.Errorf("expected at least 3 keys, got %d", len(keys))
		}
	})
}

func TestVaultLock(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	vault.Set("test_key", "test_value")

	t.Run("lock clears in-memory state", func(t *testing.T) {
		vault.Lock()

		if vault.IsUnlocked() {
			t.Error("vault should be locked")
		}
	})

	t.Run("cannot get after lock", func(t *testing.T) {
		_, err := vault.Get("test_key")
		if err == nil {
			t.Error("expected error when getting from locked vault")
		}
	})
}

func TestVaultPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	// Create and store values
	vault1 := NewVault(vaultPath)
	if err := vault1.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault1.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	vault1.Set("persistent_key", "persistent_value")
	vault1.Lock()

	t.Run("values persist across instances", func(t *testing.T) {
		vault2 := NewVault(vaultPath)
		if err := vault2.Unlock("password"); err != nil {
			t.Fatalf("failed to unlock: %v", err)
		}

		val, err := vault2.Get("persistent_key")
		if err != nil {
			t.Fatalf("failed to get: %v", err)
		}

		if val != "persistent_value" {
			t.Errorf("expected 'persistent_value', got %q", val)
		}
	})
}

func TestVaultDelete(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	vault.Set("to_delete", "value")

	t.Run("deletes existing key", func(t *testing.T) {
		err := vault.Delete("to_delete")
		if err != nil {
			t.Fatalf("failed to delete: %v", err)
		}

		val, err := vault.Get("to_delete")
		if err != nil {
			t.Errorf("unexpected error getting deleted key: %v", err)
		}
		if val != "" {
			t.Errorf("expected empty string for deleted key, got %q", val)
		}
	})

	t.Run("delete non-existent key succeeds silently", func(t *testing.T) {
		// Delete on non-existent key is a no-op in Go maps
		err := vault.Delete("nonexistent")
		if err != nil {
			t.Errorf("unexpected error deleting non-existent key: %v", err)
		}
	})
}

func TestVaultList(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("list returns stored keys", func(t *testing.T) {
		vault.Set("key1", "val1")
		vault.Set("key2", "val2")
		vault.Set("key3", "val3")

		keys := vault.List()

		// Check that our keys are in the list
		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}

		for _, expected := range []string{"key1", "key2", "key3"} {
			if !keyMap[expected] {
				t.Errorf("expected key %q in list", expected)
			}
		}
	})
}

func TestVaultFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("vault file has restricted permissions", func(t *testing.T) {
		info, err := os.Stat(vaultPath)
		if err != nil {
			t.Fatalf("failed to stat vault file: %v", err)
		}

		// Check that file is not world-readable
		mode := info.Mode()
		if mode&0077 != 0 {
			t.Errorf("vault file should have restricted permissions, got %v", mode)
		}
	})
}

func TestVaultChangePassword(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("old-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("old-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	vault.Set("test_key", "test_value")

	t.Run("change password and unlock with new", func(t *testing.T) {
		err := vault.ChangePassword("new-password")
		if err != nil {
			t.Fatalf("failed to change password: %v", err)
		}

		// Should still be unlocked after change
		if !vault.IsUnlocked() {
			t.Error("vault should still be unlocked after password change")
		}

		// Value should still be accessible
		val, err := vault.Get("test_key")
		if err != nil {
			t.Fatalf("failed to get after password change: %v", err)
		}
		if val != "test_value" {
			t.Errorf("expected 'test_value', got %q", val)
		}
	})

	t.Run("old password no longer works", func(t *testing.T) {
		vault.Lock()
		err := vault.Unlock("old-password")
		if err == nil {
			t.Error("expected error with old password")
		}
	})

	t.Run("new password works", func(t *testing.T) {
		err := vault.Unlock("new-password")
		if err != nil {
			t.Fatalf("failed to unlock with new password: %v", err)
		}
	})
}

func TestVaultKeyListing(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")
	vault := NewVault(vaultPath)

	// Setup
	if err := vault.Create("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := vault.Unlock("password"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("keys returns all stored keys", func(t *testing.T) {
		vault.Set("key1", "value1")
		vault.Set("key2", "value2")

		keys, err := vault.Keys()
		if err != nil {
			t.Fatalf("failed to get keys: %v", err)
		}

		// Check that our keys are present
		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}

		if !keyMap["key1"] || !keyMap["key2"] {
			t.Errorf("expected keys 'key1' and 'key2' in %v", keys)
		}
	})

	t.Run("keys empty when vault empty", func(t *testing.T) {
		emptyVault := NewVault(filepath.Join(tmpDir, "empty.vault"))
		if err := emptyVault.Create("password"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := emptyVault.Unlock("password"); err != nil {
			t.Fatalf("setup: %v", err)
		}

		keys, err := emptyVault.Keys()
		if err != nil {
			t.Fatalf("failed to get keys: %v", err)
		}

		// Should only have __verify__ key
		if len(keys) > 1 {
			t.Errorf("expected at most 1 key (verify), got %d: %v", len(keys), keys)
		}
	})
}
