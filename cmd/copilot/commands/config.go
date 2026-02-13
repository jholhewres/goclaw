package commands

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newConfigCmd creates the `copilot config` command.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage assistant configuration",
		Long: `Manage GoClaw Copilot configuration.

Examples:
  copilot config init
  copilot config show
  copilot config validate`,
	}

	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigShowCmd(),
		newConfigValidateCmd(),
		newConfigSetKeyCmd(),
		newConfigDeleteKeyCmd(),
		newConfigKeyStatusCmd(),
		newVaultInitCmd(),
		newVaultSetCmd(),
		newVaultStatusCmd(),
		newVaultChangePasswordCmd(),
	)

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default config.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			target := "config.yaml"

			// Check if already exists.
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("config.yaml already exists. Remove it first or edit it directly")
			}

			// Write default config.
			cfg := copilot.DefaultConfig()
			if err := copilot.SaveConfigToFile(cfg, target); err != nil {
				return err
			}

			fmt.Printf("Created %s with default configuration.\n", target)
			fmt.Println("\nNext steps:")
			fmt.Println("  1. Edit config.yaml and set your phone number in access.owners")
			fmt.Println("  2. Run: copilot serve")
			fmt.Println("  3. Scan the QR code with WhatsApp")
			return nil
		},
	}
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, path, err := loadConfig(cmd)
			if err != nil {
				return err
			}

			fmt.Printf("# Loaded from: %s\n\n", path)

			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	}
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, path, err := loadConfig(cmd)
			if err != nil {
				return err
			}

			fmt.Printf("Config: %s\n", path)
			fmt.Printf("  Name:      %s\n", cfg.Name)
			fmt.Printf("  Model:     %s\n", cfg.Model)
			fmt.Printf("  Trigger:   %s\n", cfg.Trigger)
			fmt.Printf("  Language:  %s\n", cfg.Language)
			fmt.Printf("  Policy:    %s\n", cfg.Access.DefaultPolicy)
			fmt.Printf("  Owners:    %d\n", len(cfg.Access.Owners))
			fmt.Printf("  Admins:    %d\n", len(cfg.Access.Admins))
			fmt.Printf("  Users:     %d\n", len(cfg.Access.AllowedUsers))

			wsCount := len(cfg.Workspaces.Workspaces)
			fmt.Printf("  Workspaces: %d\n", wsCount)
			for _, ws := range cfg.Workspaces.Workspaces {
				fmt.Printf("    - %s (%s): %d members, %d groups\n",
					ws.ID, ws.Name, len(ws.Members), len(ws.Groups))
			}

			fmt.Println("\nConfiguration is valid.")
			return nil
		},
	}
}

// newConfigSetKeyCmd stores the API key in the OS keyring.
func newConfigSetKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-key",
		Short: "Store API key in OS keyring (encrypted)",
		Long: `Securely stores your API key in the operating system's native keyring.
This is the most secure option — the key is encrypted by the OS
and never stored as plaintext on disk.

Linux:   GNOME Keyring / KDE Wallet / Secret Service
macOS:   Keychain
Windows: Credential Manager

Examples:
  copilot config set-key`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !copilot.KeyringAvailable() {
				fmt.Println("OS keyring is not available on this system.")
				fmt.Println("Make sure you have a keyring service running:")
				fmt.Println("  Linux:   gnome-keyring-daemon or kwallet")
				fmt.Println("  macOS:   Keychain (built-in)")
				fmt.Println("  Windows: Credential Manager (built-in)")
				return fmt.Errorf("keyring not available")
			}

			reader := bufio.NewReader(os.Stdin)

			// Check if key already exists.
			if existing := copilot.GetKeyring("api_key"); existing != "" {
				masked := existing[:4] + "****" + existing[max(4, len(existing)-4):]
				fmt.Printf("API key already in keyring: %s\n", masked)
				fmt.Print("Overwrite? (y/n) [n]: ")
				if ans := strings.TrimSpace(readKeyLine(reader)); strings.ToLower(ans) != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			fmt.Print("Enter API key: ")
			key := strings.TrimSpace(readKeyLine(reader))
			if key == "" {
				return fmt.Errorf("no key provided")
			}

			logger := slog.Default()
			if err := copilot.MigrateKeyToKeyring(key, logger); err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("API key stored in OS keyring (encrypted).")
			fmt.Println()
			fmt.Println("You can now safely remove it from other locations:")
			fmt.Println("  - Delete the GOCLAW_API_KEY line from .env")
			fmt.Println("  - Set api_key: \"\" in config.yaml")
			fmt.Println()
			fmt.Println("The keyring is checked first, before .env or config.yaml.")

			return nil
		},
	}
}

// newConfigDeleteKeyCmd removes the API key from the OS keyring.
func newConfigDeleteKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-key",
		Short: "Remove API key from OS keyring",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := copilot.DeleteKeyring("api_key"); err != nil {
				return fmt.Errorf("deleting from keyring: %w", err)
			}
			fmt.Println("API key removed from OS keyring.")
			return nil
		},
	}
}

// newConfigKeyStatusCmd shows where the API key is stored.
func newConfigKeyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "key-status",
		Short: "Show where the API key is loaded from",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("API key resolution order:")
			fmt.Println()

			// 1. Encrypted vault.
			vault := copilot.NewVault(copilot.VaultFile)
			if vault.Exists() {
				fmt.Println("  1. [OK] Encrypted vault: .goclaw.vault (AES-256-GCM, locked)")
			} else {
				fmt.Println("  1. [--] Encrypted vault: (not created)")
			}

			// 2. Keyring.
			if copilot.KeyringAvailable() {
				if val := copilot.GetKeyring("api_key"); val != "" {
					masked := val[:min(4, len(val))] + "****" + val[max(0, len(val)-4):]
					fmt.Printf("  2. [OK] OS keyring:     %s\n", masked)
				} else {
					fmt.Println("  2. [--] OS keyring:     (not set)")
				}
			} else {
				fmt.Println("  2. [!!] OS keyring:     (not available)")
			}

			// 3. Environment variable.
			if val := os.Getenv("GOCLAW_API_KEY"); val != "" {
				masked := val[:min(4, len(val))] + "****" + val[max(0, len(val)-4):]
				fmt.Printf("  3. [OK] GOCLAW_API_KEY: %s\n", masked)
			} else {
				fmt.Println("  3. [--] GOCLAW_API_KEY: (not set)")
			}

			if val := os.Getenv("OPENAI_API_KEY"); val != "" {
				fmt.Println("  4. [OK] OPENAI_API_KEY: (set, fallback)")
			} else {
				fmt.Println("  4. [--] OPENAI_API_KEY: (not set)")
			}

			fmt.Println()
			fmt.Println("Recommendation: use 'copilot config vault-init' + 'vault-set' for maximum security.")
			fmt.Println("The encrypted vault is the only method that protects against filesystem access.")

			return nil
		},
	}
}

// readKeyLine reads a line for the config key commands.
func readKeyLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return line
}

// ---------- Vault commands ----------

// newVaultInitCmd creates the encrypted vault with a master password.
func newVaultInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault-init",
		Short: "Create an encrypted vault for secrets",
		Long: `Creates a new encrypted vault file (.goclaw.vault) protected by a master password.

The vault uses AES-256-GCM encryption with Argon2id key derivation.
Even with filesystem access, secrets cannot be read without the password.

The password is NEVER stored — it exists only in your memory.

Examples:
  copilot config vault-init`,
		RunE: func(_ *cobra.Command, _ []string) error {
			vault := copilot.NewVault(copilot.VaultFile)

			if vault.Exists() {
				return fmt.Errorf("vault already exists at %s. Delete it first to recreate", copilot.VaultFile)
			}

			fmt.Println("Creating encrypted vault...")
			fmt.Println()
			fmt.Println("Choose a strong master password. This password is NEVER stored.")
			fmt.Println("If you forget it, the vault contents are permanently lost.")
			fmt.Println()

			password, err := copilot.ReadPassword("Master password: ")
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			if len(password) < 8 {
				return fmt.Errorf("password too short (minimum 8 characters)")
			}

			confirm, err := copilot.ReadPassword("Confirm password: ")
			if err != nil {
				return fmt.Errorf("reading confirmation: %w", err)
			}

			if password != confirm {
				return fmt.Errorf("passwords do not match")
			}

			if err := vault.Create(password); err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("Encrypted vault created at .goclaw.vault")
			fmt.Println()
			fmt.Println("Next: store your API key with 'copilot config vault-set'")

			return nil
		},
	}
}

// newVaultSetCmd stores a secret in the encrypted vault.
func newVaultSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault-set",
		Short: "Store API key in the encrypted vault",
		Long: `Encrypts and stores your API key in the vault (.goclaw.vault).
Requires the master password to unlock the vault.

After storing, you can safely delete the .env file:
  rm .env

Examples:
  copilot config vault-set`,
		RunE: func(_ *cobra.Command, _ []string) error {
			vault := copilot.NewVault(copilot.VaultFile)

			if !vault.Exists() {
				return fmt.Errorf("no vault found. Run 'copilot config vault-init' first")
			}

			// Unlock vault.
			password, err := copilot.ReadPassword("Master password: ")
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}

			if err := vault.Unlock(password); err != nil {
				return err
			}
			defer vault.Lock()

			// Check if key already exists.
			if existing, _ := vault.Get("api_key"); existing != "" {
				masked := existing[:min(4, len(existing))] + "****"
				fmt.Printf("API key already in vault: %s\n", masked)

				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Overwrite? (y/n) [n]: ")
				if ans := strings.TrimSpace(readKeyLine(reader)); strings.ToLower(ans) != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Read the API key (hidden input).
			apiKey, err := copilot.ReadPassword("API key: ")
			if err != nil {
				return fmt.Errorf("reading API key: %w", err)
			}
			if apiKey == "" {
				return fmt.Errorf("no key provided")
			}

			if err := vault.Set("api_key", apiKey); err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("API key encrypted and stored in vault.")
			fmt.Println()
			fmt.Println("You can now safely remove plaintext copies:")
			fmt.Println("  rm .env                          # delete .env file")
			fmt.Println("  copilot config delete-key        # remove from OS keyring")
			fmt.Println()
			fmt.Println("On startup, GoClaw will ask for your master password to decrypt.")

			return nil
		},
	}
}

// newVaultStatusCmd shows vault status and stored keys.
func newVaultStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault-status",
		Short: "Show encrypted vault status",
		RunE: func(_ *cobra.Command, _ []string) error {
			vault := copilot.NewVault(copilot.VaultFile)

			fmt.Println("Encrypted vault status:")
			fmt.Println()

			if !vault.Exists() {
				fmt.Println("  Status: NOT CREATED")
				fmt.Println()
				fmt.Println("  Run 'copilot config vault-init' to create one.")
				return nil
			}

			fmt.Printf("  File:   %s\n", copilot.VaultFile)
			fmt.Println("  Status: LOCKED (encrypted)")
			fmt.Println()

			fmt.Print("Unlock to see stored keys? (y/n) [n]: ")
			reader := bufio.NewReader(os.Stdin)
			if ans := strings.TrimSpace(readKeyLine(reader)); strings.ToLower(ans) != "y" {
				return nil
			}

			password, err := copilot.ReadPassword("Master password: ")
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}

			if err := vault.Unlock(password); err != nil {
				return err
			}
			defer vault.Lock()

			keys, err := vault.Keys()
			if err != nil {
				return err
			}

			if len(keys) == 0 {
				fmt.Println("  No secrets stored yet.")
				fmt.Println("  Run 'copilot config vault-set' to add your API key.")
			} else {
				fmt.Printf("  Stored keys (%d):\n", len(keys))
				for _, k := range keys {
					val, _ := vault.Get(k)
					masked := ""
					if len(val) > 4 {
						masked = val[:4] + "****"
					} else {
						masked = "****"
					}
					fmt.Printf("    - %s: %s\n", k, masked)
				}
			}

			return nil
		},
	}
}

// newVaultChangePasswordCmd changes the vault master password.
func newVaultChangePasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault-change-password",
		Short: "Change the vault master password",
		Long: `Re-encrypts all vault entries with a new master password.
Requires the current password to unlock first.

Examples:
  copilot config vault-change-password`,
		RunE: func(_ *cobra.Command, _ []string) error {
			vault := copilot.NewVault(copilot.VaultFile)

			if !vault.Exists() {
				return fmt.Errorf("no vault found. Run 'copilot config vault-init' first")
			}

			// Unlock with current password.
			current, err := copilot.ReadPassword("Current password: ")
			if err != nil {
				return err
			}
			if err := vault.Unlock(current); err != nil {
				return err
			}

			// Get new password.
			newPass, err := copilot.ReadPassword("New password: ")
			if err != nil {
				return err
			}
			if len(newPass) < 8 {
				vault.Lock()
				return fmt.Errorf("password too short (minimum 8 characters)")
			}

			confirm, err := copilot.ReadPassword("Confirm new password: ")
			if err != nil {
				return err
			}
			if newPass != confirm {
				vault.Lock()
				return fmt.Errorf("passwords do not match")
			}

			if err := vault.ChangePassword(newPass); err != nil {
				return err
			}

			vault.Lock()

			fmt.Println()
			fmt.Println("Vault password changed. All secrets re-encrypted with the new password.")

			return nil
		},
	}
}

// loadConfig loads the config from the --config flag or auto-discovers it.
func loadConfig(cmd *cobra.Command) (*copilot.Config, string, error) {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")

	if configPath == "" {
		configPath = copilot.FindConfigFile()
	}

	if configPath == "" {
		return nil, "", fmt.Errorf("no config file found.\nRun 'copilot config init' to create one, or use --config <path>")
	}

	cfg, err := copilot.LoadConfigFromFile(configPath)
	if err != nil {
		return nil, configPath, fmt.Errorf("loading config from %s: %w", configPath, err)
	}

	return cfg, configPath, nil
}
