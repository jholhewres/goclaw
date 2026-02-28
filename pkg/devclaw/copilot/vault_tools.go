// Package copilot – vault_tools.go implements the vault dispatcher tool.
// Uses dispatcher pattern to consolidate 5 vault tools into 1.
package copilot

import (
	"context"
	"fmt"
	"strings"
)

// RegisterVaultDispatcher registers a single "vault" dispatcher tool that
// replaces the individual vault_status, vault_save, vault_get, vault_list,
// vault_delete tools.
func RegisterVaultDispatcher(executor *ToolExecutor, vault *Vault) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"status", "save", "get", "list", "delete"},
				"description": "Action: status (check vault state), save (store secret), get (retrieve), list (show names), delete (remove)",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Secret name/key (for save/get/delete)",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Secret value (for save only)",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("vault",
			"Manage encrypted secrets vault (AES-256-GCM). Actions: status, save, get, list, delete. Always check status first.",
			schema),
		func(_ context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			switch action {
			case "status":
				return handleVaultStatus(vault)
			case "save":
				return handleVaultSave(vault, args)
			case "get":
				return handleVaultGet(vault, args)
			case "list":
				return handleVaultList(vault)
			case "delete":
				return handleVaultDelete(vault, args)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: status, save, get, list, delete)", action)
			}
		},
	)
}

func handleVaultStatus(vault *Vault) (any, error) {
	status := map[string]any{
		"unlock_methods": []string{
			"Set DEVCLAW_VAULT_PASSWORD environment variable",
			"Run 'devclaw vault unlock'",
		},
	}

	if vault == nil {
		status["available"] = false
		status["exists"] = false
		status["locked"] = true
		status["secret_count"] = 0
		status["path"] = ""
		status["message"] = "Vault not configured. Run 'devclaw vault init' to create one."
		return status, nil
	}

	exists := vault.Exists()
	unlocked := vault.IsUnlocked()
	status["available"] = unlocked
	status["exists"] = exists
	status["locked"] = !unlocked
	status["path"] = vault.Path()

	if unlocked {
		keys, _ := vault.Keys()
		status["secret_count"] = len(keys)
		status["message"] = fmt.Sprintf("Vault unlocked with %d secrets.", len(keys))
	} else if exists {
		status["secret_count"] = 0
		status["message"] = "Vault exists but is locked. Unlock with DEVCLAW_VAULT_PASSWORD or 'devclaw vault unlock'."
	} else {
		status["secret_count"] = 0
		status["message"] = "Vault not initialized. Run 'devclaw vault init' to create one."
	}

	return status, nil
}

func handleVaultSave(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	value, _ := args["value"].(string)
	if name == "" || value == "" {
		return nil, fmt.Errorf("name and value are required for save action")
	}
	if !vault.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked — set DEVCLAW_VAULT_PASSWORD or run 'devclaw vault unlock'")
	}
	if err := vault.Set(name, value); err != nil {
		return nil, fmt.Errorf("failed to save to vault: %w", err)
	}
	return fmt.Sprintf("Secret '%s' saved to encrypted vault.", name), nil
}

func handleVaultGet(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for get action")
	}
	val, err := vault.Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read from vault: %w", err)
	}
	if val == "" {
		return fmt.Sprintf("Secret '%s' not found in vault.", name), nil
	}
	return val, nil
}

func handleVaultList(vault *Vault) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	names, err := vault.Keys()
	if err != nil {
		return nil, fmt.Errorf("failed to list vault keys: %w", err)
	}
	if len(names) == 0 {
		return "Vault is empty.", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Vault contains %d secrets:\n", len(names))
	for _, name := range names {
		fmt.Fprintf(&sb, "- %s\n", name)
	}
	return sb.String(), nil
}

func handleVaultDelete(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for delete action")
	}
	if err := vault.Delete(name); err != nil {
		return nil, fmt.Errorf("failed to delete from vault: %w", err)
	}
	return fmt.Sprintf("Secret '%s' removed from vault.", name), nil
}
