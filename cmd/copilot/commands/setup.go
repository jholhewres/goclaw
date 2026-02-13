package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/spf13/cobra"
)

// newSetupCmd creates the `copilot setup` command for interactive configuration.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard",
		Long: `Starts an interactive wizard to create your initial config.yaml.
Asks for assistant name, owner phone number, model, language, and other essentials.
API keys are stored in an encrypted vault (AES-256-GCM) — never in plaintext.

Examples:
  copilot setup`,
		RunE: runSetup,
	}
}

// runSetup executes the interactive setup flow.
func runSetup(_ *cobra.Command, _ []string) error {
	return runInteractiveSetup()
}

// storageMethod tracks where the API key was stored during setup.
type storageMethod int

const (
	storageNone    storageMethod = iota
	storageVault                 // encrypted vault (.goclaw.vault)
	storageKeyring               // OS keyring
)

// runInteractiveSetup guides the user through config creation step by step.
func runInteractiveSetup() error {
	reader := bufio.NewReader(os.Stdin)
	cfg := copilot.DefaultConfig()
	keyStorage := storageNone

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║         GoClaw Copilot — Setup Wizard        ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// ── Step 1: Assistant name ──
	fmt.Printf("1. Assistant name [%s]: ", cfg.Name)
	if name := readLine(reader); name != "" {
		cfg.Name = name
	}

	// ── Step 2: Trigger keyword ──
	fmt.Printf("2. Trigger keyword [%s]: ", cfg.Trigger)
	if trigger := readLine(reader); trigger != "" {
		cfg.Trigger = trigger
	}

	// ── Step 3: Owner phone number ──
	fmt.Println()
	fmt.Println("   The owner has full control over the bot.")
	fmt.Println("   Use your phone number with country code, no +, spaces or dashes.")
	fmt.Println("   Example: 5511999998888")
	fmt.Println()
	for {
		fmt.Print("3. Your phone number (owner): ")
		owner := readLine(reader)
		if owner == "" {
			fmt.Println("   [!] Phone number is required. The bot needs at least one owner.")
			continue
		}
		owner = normalizePhone(owner)
		if len(owner) < 10 {
			fmt.Println("   [!] Number seems too short. Include the country code (e.g. 5511999998888).")
			continue
		}
		cfg.Access.Owners = []string{owner}
		break
	}

	// ── Step 4: Access policy ──
	fmt.Println()
	fmt.Println("   Access policy for unknown contacts:")
	fmt.Println("   deny  — silently ignore (recommended)")
	fmt.Println("   allow — respond to everyone")
	fmt.Println("   ask   — send a one-time access request message")
	fmt.Println()
	fmt.Printf("4. Access policy [%s]: ", cfg.Access.DefaultPolicy)
	if policy := readLine(reader); policy != "" {
		switch strings.ToLower(policy) {
		case "deny", "allow", "ask":
			cfg.Access.DefaultPolicy = copilot.AccessPolicy(strings.ToLower(policy))
		default:
			fmt.Println("   [!] Invalid value, using 'deny'.")
		}
	}

	// ── Step 5: API provider ──
	fmt.Println()
	fmt.Println("   API endpoint (OpenAI-compatible):")
	fmt.Println()
	fmt.Printf("5. API base URL [%s]: ", cfg.API.BaseURL)
	if url := readLine(reader); url != "" {
		cfg.API.BaseURL = url
	}

	// ── Step 6: API key + encrypted vault ──
	fmt.Println()
	fmt.Println("   Your API key will be encrypted with AES-256-GCM and stored in a")
	fmt.Println("   password-protected vault. Even with filesystem access, nobody can")
	fmt.Println("   read it without your master password.")
	fmt.Println()

	apiKey, err := copilot.ReadPassword("6. API key (hidden input): ")
	if err != nil {
		// Fallback to visible input if terminal password reading fails.
		fmt.Print("6. API key (or press Enter to skip): ")
		apiKey = readLine(reader)
	}

	if apiKey != "" {
		keyStorage = setupVault(apiKey)
		if keyStorage == storageNone {
			fmt.Println("   [!] Could not store the API key securely.")
			fmt.Println("   You can set it later with: copilot config vault-init && copilot config vault-set")
		}
	} else {
		fmt.Println("   Skipped. Set it later with: copilot config vault-init && copilot config vault-set")
	}

	// config.yaml never contains the real key.
	cfg.API.APIKey = "${GOCLAW_API_KEY}"

	// ── Step 7: Model (interactive numbered list) ──
	type modelOption struct {
		id   string
		name string
		desc string
	}

	models := []modelOption{
		// OpenAI
		{"gpt-5-mini", "GPT-5 Mini", "fast and cost-effective (default)"},
		{"gpt-5", "GPT-5", "latest OpenAI flagship"},
		{"gpt-4.5-preview", "GPT-4.5 Preview", "enhanced reasoning"},
		{"gpt-4o", "GPT-4o", "great all-around"},
		{"gpt-4o-mini", "GPT-4o Mini", "fast and cheap"},
		// Anthropic
		{"claude-opus-4.6", "Claude Opus 4.6", "most capable Anthropic"},
		{"claude-opus-4.5", "Claude Opus 4.5", "previous flagship"},
		{"claude-sonnet-4.5", "Claude Sonnet 4.5", "balanced performance"},
		// GLM (api.z.ai)
		{"glm-5", "GLM-5", "most capable GLM"},
		{"glm-4.7", "GLM-4.7", "balanced capability"},
		{"glm-4.7-flash", "GLM-4.7 Flash", "fast, low cost"},
		{"glm-4.7-flashx", "GLM-4.7 FlashX", "fast with extended context"},
	}

	defaultIdx := 0
	for i, m := range models {
		if m.id == cfg.Model {
			defaultIdx = i
			break
		}
	}

	fmt.Println()
	fmt.Println("7. Select LLM model:")
	fmt.Println()
	fmt.Println("   OpenAI:")
	for i := 0; i < 5; i++ {
		marker := "  "
		if i == defaultIdx {
			marker = " *"
		}
		fmt.Printf("   %s %2d) %-20s — %s\n", marker, i+1, models[i].id, models[i].desc)
	}
	fmt.Println()
	fmt.Println("   Anthropic:")
	for i := 5; i < 8; i++ {
		marker := "  "
		if i == defaultIdx {
			marker = " *"
		}
		fmt.Printf("   %s %2d) %-20s — %s\n", marker, i+1, models[i].id, models[i].desc)
	}
	fmt.Println()
	fmt.Println("   GLM (api.z.ai):")
	for i := 8; i < len(models); i++ {
		marker := "  "
		if i == defaultIdx {
			marker = " *"
		}
		fmt.Printf("   %s %2d) %-20s — %s\n", marker, i+1, models[i].id, models[i].desc)
	}
	fmt.Println()
	fmt.Printf("   Choose [1-%d] or type model name [%s]: ", len(models), cfg.Model)

	if input := readLine(reader); input != "" {
		if idx, err := fmt.Sscanf(input, "%d", new(int)); idx == 1 && err == nil {
			var num int
			fmt.Sscanf(input, "%d", &num)
			if num >= 1 && num <= len(models) {
				cfg.Model = models[num-1].id
			} else {
				fmt.Printf("   [!] Invalid number, keeping '%s'.\n", cfg.Model)
			}
		} else {
			cfg.Model = input
		}
	}

	// Auto-adjust API base URL based on model choice.
	if strings.HasPrefix(cfg.Model, "glm-") && cfg.API.BaseURL == "https://api.openai.com/v1" {
		cfg.API.BaseURL = "https://api.z.ai/api/anthropic"
		fmt.Printf("   API URL auto-set to %s for GLM models.\n", cfg.API.BaseURL)
	} else if strings.HasPrefix(cfg.Model, "claude-") && cfg.API.BaseURL == "https://api.openai.com/v1" {
		cfg.API.BaseURL = "https://api.anthropic.com/v1"
		fmt.Printf("   API URL auto-set to %s for Anthropic models.\n", cfg.API.BaseURL)
	}

	// ── Step 8: Language ──
	fmt.Printf("8. Response language [%s]: ", cfg.Language)
	if lang := readLine(reader); lang != "" {
		cfg.Language = lang
	}

	// ── Step 9: Timezone ──
	fmt.Printf("9. Timezone [%s]: ", cfg.Timezone)
	if tz := readLine(reader); tz != "" {
		cfg.Timezone = tz
	}

	// ── Step 10: System instructions ──
	fmt.Println()
	fmt.Println("   Base system instructions (system prompt).")
	fmt.Println("   Press Enter to keep the default.")
	fmt.Println()
	fmt.Printf("10. Instructions [default]: ")
	if instr := readLine(reader); instr != "" {
		cfg.Instructions = instr
	}

	// ── Step 11: WhatsApp settings ──
	fmt.Println()
	fmt.Println("   WhatsApp settings:")
	fmt.Println()

	fmt.Printf("   Respond in groups? (y/n) [y]: ")
	if g := readLine(reader); strings.ToLower(g) == "n" {
		cfg.Channels.WhatsApp.RespondToGroups = false
	}

	fmt.Printf("   Respond in DMs? (y/n) [y]: ")
	if d := readLine(reader); strings.ToLower(d) == "n" {
		cfg.Channels.WhatsApp.RespondToDMs = false
	}

	// ── Summary ──
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("  Configuration summary:")
	fmt.Println("─────────────────────────────────────────────")
	fmt.Printf("  Name:      %s\n", cfg.Name)
	fmt.Printf("  Trigger:   %s\n", cfg.Trigger)
	fmt.Printf("  Owner:     %s\n", cfg.Access.Owners[0])
	fmt.Printf("  Policy:    %s\n", cfg.Access.DefaultPolicy)
	fmt.Printf("  API URL:   %s\n", cfg.API.BaseURL)
	switch keyStorage {
	case storageVault:
		fmt.Println("  API key:   **** (encrypted vault)")
	case storageKeyring:
		fmt.Println("  API key:   **** (OS keyring)")
	default:
		fmt.Println("  API key:   (not set — configure later)")
	}
	fmt.Printf("  Model:     %s\n", cfg.Model)
	fmt.Printf("  Language:  %s\n", cfg.Language)
	fmt.Printf("  Timezone:  %s\n", cfg.Timezone)
	fmt.Printf("  Groups:    %v\n", cfg.Channels.WhatsApp.RespondToGroups)
	fmt.Printf("  DMs:       %v\n", cfg.Channels.WhatsApp.RespondToDMs)
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println()

	// ── Confirm and save ──
	target := "config.yaml"
	fmt.Printf("Save to %s? (y/n) [y]: ", target)
	if confirm := readLine(reader); strings.ToLower(confirm) == "n" {
		fmt.Println("Setup cancelled.")
		return nil
	}

	// Check if already exists.
	if _, err := os.Stat(target); err == nil {
		fmt.Printf("File %s already exists. Overwrite? (y/n) [n]: ", target)
		if overwrite := readLine(reader); strings.ToLower(overwrite) != "y" {
			fmt.Println("Setup cancelled. Existing file kept.")
			return nil
		}
	}

	if err := copilot.SaveConfigToFile(cfg, target); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nconfig.yaml created successfully!\n\n")

	fmt.Println("Security:")
	switch keyStorage {
	case storageVault:
		fmt.Println("  - API key encrypted in vault (AES-256-GCM + Argon2id)")
		fmt.Println("  - Even with filesystem access, it cannot be read without your password")
		fmt.Println("  - No plaintext secrets anywhere on disk")
	case storageKeyring:
		fmt.Println("  - API key encrypted in OS keyring")
	default:
		fmt.Println("  - No API key configured yet")
		fmt.Println("  - Run: copilot config vault-init && copilot config vault-set")
	}
	fmt.Println("  - config.yaml has no secrets (permissions: 600)")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Run: copilot serve")
	fmt.Println("  2. Enter your vault password when prompted")
	fmt.Println("  3. Scan the QR code with your WhatsApp")
	fmt.Println()

	return nil
}

// setupVault creates the encrypted vault and stores the API key in it.
// Returns the storage method used.
func setupVault(apiKey string) storageMethod {
	fmt.Println()
	fmt.Println("   Creating encrypted vault...")
	fmt.Println("   Choose a master password (minimum 8 characters).")
	fmt.Println("   This password is NEVER stored — only you know it.")
	fmt.Println()

	password, err := copilot.ReadPassword("   Master password: ")
	if err != nil {
		fmt.Printf("   [!] Failed to read password: %v\n", err)
		return tryKeyringFallback(apiKey)
	}
	if len(password) < 8 {
		fmt.Println("   [!] Password too short (minimum 8 characters).")
		return tryKeyringFallback(apiKey)
	}

	confirm, err := copilot.ReadPassword("   Confirm password: ")
	if err != nil || password != confirm {
		fmt.Println("   [!] Passwords don't match.")
		return tryKeyringFallback(apiKey)
	}

	vault := copilot.NewVault(copilot.VaultFile)

	// Remove existing vault if present (fresh setup).
	if vault.Exists() {
		_ = os.Remove(copilot.VaultFile)
		vault = copilot.NewVault(copilot.VaultFile)
	}

	if err := vault.Create(password); err != nil {
		fmt.Printf("   [!] Vault creation failed: %v\n", err)
		return tryKeyringFallback(apiKey)
	}

	if err := vault.Set("api_key", apiKey); err != nil {
		fmt.Printf("   [!] Failed to store key in vault: %v\n", err)
		vault.Lock()
		return tryKeyringFallback(apiKey)
	}

	vault.Lock()
	fmt.Println()
	fmt.Println("   API key encrypted and stored in vault.")
	return storageVault
}

// tryKeyringFallback attempts to store the API key in the OS keyring
// as a fallback when vault creation fails.
func tryKeyringFallback(apiKey string) storageMethod {
	if copilot.KeyringAvailable() {
		fmt.Println("   Trying OS keyring as fallback...")
		if err := copilot.StoreKeyring("api_key", apiKey); err == nil {
			fmt.Println("   API key stored in OS keyring.")
			return storageKeyring
		}
	}
	return storageNone
}

// readLine reads a single line from the reader, trimming whitespace.
func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// normalizePhone removes common phone number formatting characters.
func normalizePhone(phone string) string {
	phone = strings.ReplaceAll(phone, "+", "")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	return phone
}
