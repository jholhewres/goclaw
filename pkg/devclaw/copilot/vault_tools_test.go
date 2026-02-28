package copilot

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// TestVaultDispatcherWithNilVault verifies that the vault dispatcher gracefully handles nil vault.
func TestVaultDispatcherWithNilVault(t *testing.T) {
	executor := NewToolExecutor(slog.Default())

	// Register vault dispatcher with nil vault
	RegisterVaultDispatcher(executor, nil)

	tool, ok := executor.tools["vault"]
	if !ok {
		t.Fatal("vault tool not registered")
	}

	t.Run("status with nil vault", func(t *testing.T) {
		result, err := tool.Handler(context.Background(), map[string]any{
			"action": "status",
		})
		if err != nil {
			t.Fatalf("vault status should not error with nil vault: %v", err)
		}

		status, ok := result.(map[string]any)
		if !ok {
			t.Fatal("vault status should return map[string]any")
		}

		if status["available"].(bool) {
			t.Error("vault status should show available=false with nil vault")
		}
		if status["exists"].(bool) {
			t.Error("vault status should show exists=false with nil vault")
		}
		if !status["locked"].(bool) {
			t.Error("vault status should show locked=true with nil vault")
		}

		// Verify unlock_methods is present
		methods, ok := status["unlock_methods"].([]string)
		if !ok || len(methods) == 0 {
			t.Error("vault status should include unlock_methods")
		}
	})

	t.Run("save with nil vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "save",
			"name":   "TEST_KEY",
			"value":  "test_value",
		})

		if err == nil {
			t.Error("vault save should error with nil vault")
		}

		if err != nil && !strings.Contains(err.Error(), "vault not available") {
			t.Errorf("error should mention 'vault not available', got: %v", err)
		}
	})

	t.Run("get with nil vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "get",
			"name":   "TEST_KEY",
		})

		if err == nil {
			t.Error("vault get should error with nil vault")
		}
	})

	t.Run("list with nil vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "list",
		})

		if err == nil {
			t.Error("vault list should error with nil vault")
		}
	})

	t.Run("delete with nil vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "delete",
			"name":   "TEST_KEY",
		})

		if err == nil {
			t.Error("vault delete should error with nil vault")
		}
	})
}

// TestVaultDispatcherWithLockedVault verifies that the vault dispatcher gracefully handles locked vault.
func TestVaultDispatcherWithLockedVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	// Create vault (unlocked by default) then lock it
	vault := NewVault(vaultPath)
	if err := vault.Create("test-password"); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}
	// Explicitly lock the vault for testing
	vault.Lock()

	executor := NewToolExecutor(slog.Default())
	RegisterVaultDispatcher(executor, vault)

	tool := executor.tools["vault"]

	t.Run("status with locked vault", func(t *testing.T) {
		result, err := tool.Handler(context.Background(), map[string]any{
			"action": "status",
		})
		if err != nil {
			t.Fatalf("vault status should not error: %v", err)
		}

		status := result.(map[string]any)
		if status["available"].(bool) {
			t.Error("vault status should show available=false when locked")
		}
		if !status["exists"].(bool) {
			t.Error("vault status should show exists=true")
		}
		if !status["locked"].(bool) {
			t.Error("vault status should show locked=true")
		}
	})

	t.Run("save with locked vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "save",
			"name":   "TEST_KEY",
			"value":  "test_value",
		})

		if err == nil {
			t.Error("vault save should error with locked vault")
		}

		if err != nil && !strings.Contains(err.Error(), "vault is locked") {
			t.Errorf("error should mention 'vault is locked', got: %v", err)
		}

		// Should provide unlock instructions
		if err != nil && !strings.Contains(err.Error(), "DEVCLAW_VAULT_PASSWORD") {
			t.Errorf("error should mention DEVCLAW_VAULT_PASSWORD, got: %v", err)
		}
	})

	t.Run("get with locked vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "get",
			"name":   "TEST_KEY",
		})

		if err == nil {
			t.Error("vault get should error with locked vault")
		}
	})

	t.Run("list with locked vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "list",
		})

		if err == nil {
			t.Error("vault list should error with locked vault")
		}
	})

	t.Run("delete with locked vault", func(t *testing.T) {
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "delete",
			"name":   "TEST_KEY",
		})

		if err == nil {
			t.Error("vault delete should error with locked vault")
		}
	})
}

// TestVaultDispatcherWithUnlockedVault verifies that the vault dispatcher works when unlocked.
func TestVaultDispatcherWithUnlockedVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	// Create and unlock vault
	vault := NewVault(vaultPath)
	if err := vault.Create("test-password"); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}
	if err := vault.Unlock("test-password"); err != nil {
		t.Fatalf("failed to unlock vault: %v", err)
	}

	executor := NewToolExecutor(slog.Default())
	RegisterVaultDispatcher(executor, vault)

	tool := executor.tools["vault"]

	t.Run("status with unlocked vault", func(t *testing.T) {
		result, err := tool.Handler(context.Background(), map[string]any{
			"action": "status",
		})
		if err != nil {
			t.Fatalf("vault status should not error: %v", err)
		}

		status := result.(map[string]any)
		if !status["available"].(bool) {
			t.Error("vault status should show available=true when unlocked")
		}
		if !status["exists"].(bool) {
			t.Error("vault status should show exists=true")
		}
		if status["locked"].(bool) {
			t.Error("vault status should show locked=false when unlocked")
		}
	})

	t.Run("save and get", func(t *testing.T) {
		// Save
		result, err := tool.Handler(context.Background(), map[string]any{
			"action": "save",
			"name":   "MY_API_KEY",
			"value":  "sk-test12345",
		})
		if err != nil {
			t.Fatalf("vault save should succeed when unlocked: %v", err)
		}

		if !strings.Contains(result.(string), "saved") {
			t.Errorf("expected success message, got: %v", result)
		}

		// Get
		val, err := tool.Handler(context.Background(), map[string]any{
			"action": "get",
			"name":   "MY_API_KEY",
		})
		if err != nil {
			t.Fatalf("vault get should succeed: %v", err)
		}

		if val != "sk-test12345" {
			t.Errorf("expected 'sk-test12345', got: %v", val)
		}
	})

	t.Run("list shows secrets", func(t *testing.T) {
		result, err := tool.Handler(context.Background(), map[string]any{
			"action": "list",
		})
		if err != nil {
			t.Fatalf("vault list should succeed: %v", err)
		}

		if !strings.Contains(result.(string), "MY_API_KEY") {
			t.Errorf("vault list should include MY_API_KEY, got: %v", result)
		}
	})

	t.Run("delete removes secret", func(t *testing.T) {
		// Delete
		_, err := tool.Handler(context.Background(), map[string]any{
			"action": "delete",
			"name":   "MY_API_KEY",
		})
		if err != nil {
			t.Fatalf("vault delete should succeed: %v", err)
		}

		// Verify it's gone
		result, _ := tool.Handler(context.Background(), map[string]any{
			"action": "get",
			"name":   "MY_API_KEY",
		})

		if !strings.Contains(result.(string), "not found") {
			t.Errorf("expected 'not found' after delete, got: %v", result)
		}
	})
}

// TestVaultStatusJSONStructure verifies that vault status returns properly structured JSON.
func TestVaultStatusJSONStructure(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	vault := NewVault(vaultPath)
	vault.Create("password")
	vault.Unlock("password")
	vault.Set("test_key", "test_value")

	executor := NewToolExecutor(slog.Default())
	RegisterVaultDispatcher(executor, vault)

	tool := executor.tools["vault"]
	result, err := tool.Handler(context.Background(), map[string]any{
		"action": "status",
	})
	if err != nil {
		t.Fatalf("vault status failed: %v", err)
	}

	status := result.(map[string]any)

	// Verify all expected fields exist
	requiredFields := []string{"available", "exists", "locked", "secret_count", "path", "message"}
	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("vault status missing field: %s", field)
		}
	}

	// Verify types
	if _, ok := status["available"].(bool); !ok {
		t.Error("available should be bool")
	}
	if _, ok := status["exists"].(bool); !ok {
		t.Error("exists should be bool")
	}
	if _, ok := status["locked"].(bool); !ok {
		t.Error("locked should be bool")
	}
	if _, ok := status["secret_count"].(int); !ok {
		t.Error("secret_count should be int")
	}
	if _, ok := status["path"].(string); !ok {
		t.Error("path should be string")
	}
	if _, ok := status["message"].(string); !ok {
		t.Error("message should be string")
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(status)
	if err != nil {
		t.Errorf("vault status result should be JSON serializable: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Errorf("vault status JSON should be decodable: %v", err)
	}
}

// TestVaultStatusWithNonExistentVault verifies status when vault file doesn't exist.
func TestVaultStatusWithNonExistentVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "nonexistent.vault")

	// Point to non-existent vault
	vault := NewVault(vaultPath)

	executor := NewToolExecutor(slog.Default())
	RegisterVaultDispatcher(executor, vault)

	tool := executor.tools["vault"]
	result, err := tool.Handler(context.Background(), map[string]any{
		"action": "status",
	})
	if err != nil {
		t.Fatalf("vault status should not error: %v", err)
	}

	status := result.(map[string]any)

	if status["exists"].(bool) {
		t.Error("exists should be false for non-existent vault")
	}
	if status["available"].(bool) {
		t.Error("available should be false when vault doesn't exist")
	}
}
