package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
	"github.com/spf13/cobra"
)

// newSetupCmd creates the `copilot setup` command for interactive configuration.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard",
		Long: `Starts an interactive wizard to create your initial config.yaml.
Uses arrow keys (↑/↓) to select options. Navigate with Tab/Shift+Tab.
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

// ── Provider / URL mapping ──

type providerDef struct {
	key   string
	label string
	url   string
}

var knownProviders = []providerDef{
	{"openai", "OpenAI", "https://api.openai.com/v1"},
	{"zai", "Z.AI (API)", "https://api.z.ai/api/paas/v4"},
	{"zai-coding", "Z.AI (Coding)", "https://api.z.ai/api/coding/paas/v4"},
	{"zai-anthropic", "Z.AI (Anthropic proxy)", "https://api.z.ai/api/anthropic"},
	{"anthropic", "Anthropic", "https://api.anthropic.com/v1"},
}

// providerForModel returns the best-match provider key for a model ID.
func providerForModel(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-"):
		return "openai"
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "glm-"):
		return "zai"
	default:
		return "openai"
	}
}

func providerURL(key string) string {
	for _, p := range knownProviders {
		if p.key == key {
			return p.url
		}
	}
	return "https://api.openai.com/v1"
}

// runInteractiveSetup guides the user through config creation step by step.
func runInteractiveSetup() error {
	cfg := copilot.DefaultConfig()
	keyStorage := storageNone

	// ── Variables to bind ──
	var (
		name      = cfg.Name
		trigger   = cfg.Trigger
		owner     string
		policy    = string(cfg.Access.DefaultPolicy)
		modelID   = cfg.Model
		provider  string
		language  = cfg.Language
		timezone  = cfg.Timezone
		instruct  = cfg.Instructions
		groups    = true
		dms       = true
		doAdvance bool
	)

	// ── Banner ──
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       GoClaw Copilot — Setup Wizard          ║")
	fmt.Println("║   Use ↑/↓ arrows to select, Enter to confirm ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// ═══════════════════════════════════════════════
	// Group 1: Identity
	// ═══════════════════════════════════════════════
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Assistant name").
				Description("The name shown in responses").
				Placeholder(cfg.Name).
				Value(&name),

			huh.NewInput().
				Title("Trigger keyword").
				Description("Keyword that activates the bot (e.g. @copilot)").
				Placeholder(cfg.Trigger).
				Value(&trigger),

			huh.NewInput().
				Title("Owner phone number").
				Description("Full number with country code, no + or spaces (e.g. 5511999998888)").
				Validate(func(s string) error {
					n := normalizePhone(s)
					if len(n) < 10 {
						return fmt.Errorf("number too short — include country code")
					}
					return nil
				}).
				Value(&owner),

			huh.NewSelect[string]().
				Title("Access policy").
				Description("How to handle unknown contacts").
				Options(
					huh.NewOption("deny — silently ignore (recommended)", "deny"),
					huh.NewOption("allow — respond to everyone", "allow"),
					huh.NewOption("ask — send a one-time access request", "ask"),
				).
				Value(&policy),
		).Title("Identity & Access"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	cfg.Name = name
	if trigger != "" {
		cfg.Trigger = trigger
	}
	cfg.Access.Owners = []string{normalizePhone(owner)}
	cfg.Access.DefaultPolicy = copilot.AccessPolicy(policy)

	// ═══════════════════════════════════════════════
	// Group 2: Model selection
	// ═══════════════════════════════════════════════
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("LLM Model").
				Description("Use ↑/↓ to browse, type to filter").
				Height(15).
				Options(
					// OpenAI
					huh.NewOption("GPT-5 Mini         — fast, cost-effective (default)", "gpt-5-mini"),
					huh.NewOption("GPT-5              — latest OpenAI flagship", "gpt-5"),
					huh.NewOption("GPT-4.5 Preview    — enhanced reasoning", "gpt-4.5-preview"),
					huh.NewOption("GPT-4o             — great all-around", "gpt-4o"),
					huh.NewOption("GPT-4o Mini        — fast and cheap", "gpt-4o-mini"),
					// Anthropic
					huh.NewOption("Claude Opus 4.6    — most capable Anthropic", "claude-opus-4.6"),
					huh.NewOption("Claude Opus 4.5    — previous flagship", "claude-opus-4.5"),
					huh.NewOption("Claude Sonnet 4.5  — balanced performance", "claude-sonnet-4.5"),
					// GLM
					huh.NewOption("GLM-5              — most capable GLM (Z.AI)", "glm-5"),
					huh.NewOption("GLM-4.7            — balanced capability", "glm-4.7"),
					huh.NewOption("GLM-4.7 Flash      — fast, low cost", "glm-4.7-flash"),
					huh.NewOption("GLM-4.7 FlashX     — fast + extended context", "glm-4.7-flashx"),
				).
				Value(&modelID),
		).Title("Model"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}
	cfg.Model = modelID

	// Auto-detect best provider from model.
	autoProvider := providerForModel(modelID)
	provider = autoProvider

	// ═══════════════════════════════════════════════
	// Group 3: API provider (pre-selected from model)
	// ═══════════════════════════════════════════════
	// Build options with the auto-detected one first.
	providerOpts := make([]huh.Option[string], 0, len(knownProviders)+1)
	for _, p := range knownProviders {
		label := fmt.Sprintf("%-22s — %s", p.label, p.url)
		providerOpts = append(providerOpts, huh.NewOption(label, p.key))
	}
	providerOpts = append(providerOpts, huh.NewOption("Custom URL             — enter your own", "custom"))

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("API Provider").
				Description(fmt.Sprintf("Auto-detected: %s (%s). Change if needed.", autoProvider, providerURL(autoProvider))).
				Height(9).
				Options(providerOpts...).
				Value(&provider),
		).Title("API Configuration"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if provider == "custom" {
		customURL := ""
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Custom API URL").
					Description("Enter your OpenAI-compatible endpoint").
					Placeholder("https://my-api.example.com/v1").
					Value(&customURL),
			),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		cfg.API.BaseURL = customURL
		cfg.API.Provider = ""
	} else {
		cfg.API.BaseURL = providerURL(provider)
		cfg.API.Provider = provider
	}

	// ═══════════════════════════════════════════════
	// Group 4: API key + vault
	// ═══════════════════════════════════════════════
	fmt.Println()
	fmt.Println("  Your API key will be encrypted with AES-256-GCM and stored in a")
	fmt.Println("  password-protected vault. Even with filesystem access, nobody can")
	fmt.Println("  read it without your master password.")
	fmt.Println()

	apiKey, vaultErr := copilot.ReadPassword("  API key (hidden): ")
	if vaultErr != nil {
		// Fallback: use huh input.
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("API Key").
					Description("Leave empty to configure later").
					EchoMode(huh.EchoModePassword).
					Value(&apiKey),
			),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
	}

	if apiKey != "" {
		keyStorage = setupVault(apiKey)
		if keyStorage == storageNone {
			fmt.Println("  [!] Could not store API key securely.")
			fmt.Println("  Set it later with: copilot config vault-init && copilot config vault-set")
		}
	} else {
		fmt.Println("  Skipped. Set later with: copilot config vault-init && copilot config vault-set")
	}
	cfg.API.APIKey = "${GOCLAW_API_KEY}"

	// ═══════════════════════════════════════════════
	// Group 5: Language, Timezone, Instructions, WhatsApp
	// ═══════════════════════════════════════════════
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Response language").
				Placeholder(cfg.Language).
				Value(&language),

			huh.NewInput().
				Title("Timezone").
				Placeholder(cfg.Timezone).
				Value(&timezone),

			huh.NewInput().
				Title("System instructions").
				Description("Base system prompt. Press Enter to keep default.").
				Placeholder("(keep default)").
				Value(&instruct),
		).Title("Preferences"),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Respond in WhatsApp groups?").
				Affirmative("Yes").
				Negative("No").
				Value(&groups),

			huh.NewConfirm().
				Title("Respond in WhatsApp DMs?").
				Affirmative("Yes").
				Negative("No").
				Value(&dms),

			huh.NewConfirm().
				Title("Configure advanced settings?").
				Description("Fallback models, gateway, heartbeat, security, autonomy, media, logging").
				Affirmative("Yes").
				Negative("No (use defaults)").
				Value(&doAdvance),
		).Title("WhatsApp & Advanced"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if language != "" {
		cfg.Language = language
	}
	if timezone != "" {
		cfg.Timezone = timezone
	}
	if instruct != "" {
		cfg.Instructions = instruct
	}
	cfg.Channels.WhatsApp.RespondToGroups = groups
	cfg.Channels.WhatsApp.RespondToDMs = dms

	// ═══════════════════════════════════════════════
	// Group 6: Advanced settings (optional)
	// ═══════════════════════════════════════════════
	if doAdvance {
		if err := setupAdvanced(cfg); err != nil {
			return err
		}
	}

	// ═══════════════════════════════════════════════
	// Group 7: Channel Setup (Telegram, Discord, Slack)
	// ═══════════════════════════════════════════════
	var (
		setupTelegram    bool
		telegramToken    string
		setupDiscord     bool
		discordToken     string
		setupSlack       bool
		slackBotToken    string
		slackAppToken    string
	)

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Setup Telegram bot?").
				Description("Connect to Telegram via Bot API. Get a token from @BotFather.").
				Affirmative("Yes").
				Negative("Skip").
				Value(&setupTelegram),

			huh.NewConfirm().
				Title("Setup Discord bot?").
				Description("Connect to Discord via Bot Gateway. Get a token from Discord Developer Portal.").
				Affirmative("Yes").
				Negative("Skip").
				Value(&setupDiscord),

			huh.NewConfirm().
				Title("Setup Slack bot?").
				Description("Connect to Slack via Socket Mode. Requires Bot Token + App Token.").
				Affirmative("Yes").
				Negative("Skip").
				Value(&setupSlack),
		).Title("Messaging Channels"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if setupTelegram {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Telegram Bot Token").
					Description("From @BotFather: /newbot → copy token").
					Placeholder("123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11").
					EchoMode(huh.EchoModePassword).
					Value(&telegramToken),
			).Title("Telegram Setup"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		if telegramToken != "" {
			cfg.Channels.Telegram.Token = telegramToken
		}
	}

	if setupDiscord {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Discord Bot Token").
					Description("From Discord Developer Portal → Bot → Token").
					Placeholder("MTIzNDU2Nzg5MDEy...").
					EchoMode(huh.EchoModePassword).
					Value(&discordToken),
			).Title("Discord Setup"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		if discordToken != "" {
			cfg.Channels.Discord.Token = discordToken
		}
	}

	if setupSlack {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Slack Bot Token").
					Description("Bot User OAuth Token (starts with xoxb-)").
					Placeholder("xoxb-...").
					EchoMode(huh.EchoModePassword).
					Value(&slackBotToken),
				huh.NewInput().
					Title("Slack App Token").
					Description("App-Level Token for Socket Mode (starts with xapp-)").
					Placeholder("xapp-...").
					EchoMode(huh.EchoModePassword).
					Value(&slackAppToken),
			).Title("Slack Setup"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		if slackBotToken != "" {
			cfg.Channels.Slack.BotToken = slackBotToken
			cfg.Channels.Slack.AppToken = slackAppToken
		}
	}

	// ═══════════════════════════════════════════════
	// Group 8: TTS Configuration
	// ═══════════════════════════════════════════════
	var (
		setupTTS    bool
		ttsProvider = "auto"
		ttsVoice    = "nova"
		ttsMode     = "off"
	)

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Setup Text-to-Speech (TTS)?").
				Description("Generate audio responses as voice messages.").
				Affirmative("Yes").
				Negative("Skip").
				Value(&setupTTS),
		).Title("Text-to-Speech"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if setupTTS {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("TTS Provider").
					Options(
						huh.NewOption("auto — OpenAI with Edge TTS fallback (recommended)", "auto"),
						huh.NewOption("openai — high quality, paid", "openai"),
						huh.NewOption("edge — free Microsoft voices, good quality", "edge"),
					).
					Value(&ttsProvider),

				huh.NewSelect[string]().
					Title("Voice").
					Description("For OpenAI: alloy, echo, fable, onyx, nova, shimmer. For Edge: varies by language.").
					Options(
						huh.NewOption("nova — warm female (OpenAI)", "nova"),
						huh.NewOption("alloy — neutral (OpenAI)", "alloy"),
						huh.NewOption("echo — male (OpenAI)", "echo"),
						huh.NewOption("shimmer — expressive female (OpenAI)", "shimmer"),
						huh.NewOption("onyx — deep male (OpenAI)", "onyx"),
						huh.NewOption("fable — storytelling (OpenAI)", "fable"),
					).
					Value(&ttsVoice),

				huh.NewSelect[string]().
					Title("Auto mode").
					Description("When to auto-generate audio responses").
					Options(
						huh.NewOption("off — only when requested via /tts command", "off"),
						huh.NewOption("always — always generate audio alongside text", "always"),
						huh.NewOption("inbound — only when user sends a voice message", "inbound"),
					).
					Value(&ttsMode),
			).Title("TTS Settings"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}

		cfg.TTS.Enabled = true
		cfg.TTS.Provider = ttsProvider
		cfg.TTS.Voice = ttsVoice
		cfg.TTS.AutoMode = ttsMode
		if ttsProvider == "auto" || ttsProvider == "edge" {
			cfg.TTS.EdgeVoice = "pt-BR-FranciscaNeural"
		}
	}

	// ═══════════════════════════════════════════════
	// Group 9: Web UI Configuration
	// ═══════════════════════════════════════════════
	var (
		setupWebUI    bool
		webUIAddress  = ":8090"
		webUIAuthToken string
	)

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Web Dashboard?").
				Description("Browser-based UI to manage sessions, usage, skills, and config.").
				Affirmative("Yes").
				Negative("Skip").
				Value(&setupWebUI),
		).Title("Web Dashboard"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if setupWebUI {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Web UI listen address").
					Placeholder(":8090").
					Value(&webUIAddress),
				huh.NewInput().
					Title("Auth token (recommended)").
					Description("Bearer token for dashboard authentication. Leave empty for no auth.").
					EchoMode(huh.EchoModePassword).
					Value(&webUIAuthToken),
			).Title("Web UI Settings"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}

		cfg.WebUI.Enabled = true
		if webUIAddress != "" {
			cfg.WebUI.Address = webUIAddress
		}
		cfg.WebUI.AuthToken = webUIAuthToken
	}

	// ═══════════════════════════════════════════════
	// Group 10: Default skills installation
	// ═══════════════════════════════════════════════
	defaults := skills.DefaultSkills()
	skillOpts := make([]huh.Option[string], 0, len(defaults))
	for _, t := range defaults {
		skillOpts = append(skillOpts, huh.NewOption(t.Label, t.Name))
	}

	var selectedSkills []string
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Install default skills").
				Description("Select skills to install (Space to toggle, Enter to confirm)").
				Options(skillOpts...).
				Value(&selectedSkills),
		).Title("Skills"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if len(selectedSkills) > 0 {
		installEmbeddedSkills(selectedSkills)
	}

	// ═══════════════════════════════════════════════
	// Summary + Confirm
	// ═══════════════════════════════════════════════
	printSummary(cfg, keyStorage, selectedSkills)

	var doSave bool
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Save configuration to config.yaml?").
				Affirmative("Yes, save").
				Negative("Cancel").
				Value(&doSave),
		),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if !doSave {
		fmt.Println("  Setup cancelled.")
		return nil
	}

	// Check existing file.
	target := "config.yaml"
	if _, statErr := os.Stat(target); statErr == nil {
		var overwrite bool
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("%s already exists. Overwrite?", target)).
					Affirmative("Yes, overwrite").
					Negative("Cancel").
					Value(&overwrite),
			),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Println("  Setup cancelled. Existing file kept.")
			return nil
		}
	}

	if err := copilot.SaveConfigToFile(cfg, target); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Println("  ✓ config.yaml created successfully!")
	fmt.Println()

	// Security summary.
	switch keyStorage {
	case storageVault:
		fmt.Println("  Security:")
		fmt.Println("    • API key encrypted in vault (AES-256-GCM + Argon2id)")
		fmt.Println("    • No plaintext secrets anywhere on disk")
	case storageKeyring:
		fmt.Println("  Security:")
		fmt.Println("    • API key stored in OS keyring")
	default:
		fmt.Println("  Security:")
		fmt.Println("    • No API key configured yet")
		fmt.Println("    • Run: copilot config vault-init && copilot config vault-set")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    1. copilot serve")
	fmt.Println("    2. Enter your vault password when prompted")
	fmt.Println("    3. Connect your messaging platform(s)")
	fmt.Println()
	fmt.Println("  Shell completion (optional):")
	fmt.Println("    bash:       eval \"$(copilot completion bash)\"")
	fmt.Println("    zsh:        copilot completion zsh > \"${fpath[1]}/_copilot\"")
	fmt.Println("    fish:       copilot completion fish | source")
	fmt.Println("    powershell: copilot completion powershell | Out-String | Invoke-Expression")
	fmt.Println()

	return nil
}

// setupAdvanced runs the interactive advanced configuration form.
func setupAdvanced(cfg *copilot.Config) error {
	var (
		fallbackInput  string
		gatewayEnabled bool
		gatewayAddr    = cfg.Gateway.Address
		gatewayToken   string
		hbEnabled      bool
		hbChannel      = "whatsapp"
		hbChatID       string
		allowDestr     bool
		allowSudo      bool
		confirmTools   = true
		maxTurns       = fmt.Sprintf("%d", cfg.Agent.MaxTurns)
		maxCont        = fmt.Sprintf("%d", cfg.Agent.MaxContinuations)
		subEnabled     = true
		maxConcurrent  = fmt.Sprintf("%d", cfg.Subagents.MaxConcurrent)
		visionEnabled  = cfg.Media.VisionEnabled
		audioEnabled   = cfg.Media.TranscriptionEnabled
		logLevel       = cfg.Logging.Level
	)

	// ── Part A: Fallback + Gateway ──
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Fallback models").
				Description("Comma-separated list of models to try if primary fails (Enter to skip)").
				Placeholder("gpt-4o,glm-4.7-flash,claude-sonnet-4.5").
				Value(&fallbackInput),

			huh.NewConfirm().
				Title("Enable HTTP API Gateway?").
				Description("OpenAI-compatible REST API (chat completions, sessions, webhooks)").
				Affirmative("Yes").
				Negative("No").
				Value(&gatewayEnabled),
		).Title("Fallback & Gateway"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if fallbackInput != "" {
		parts := strings.Split(fallbackInput, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.Fallback.Models = append(cfg.Fallback.Models, p)
			}
		}
	}

	if gatewayEnabled {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Gateway listen address").
					Placeholder(cfg.Gateway.Address).
					Value(&gatewayAddr),
				huh.NewInput().
					Title("Gateway auth token (optional)").
					Description("Bearer token for API authentication. Leave empty for no auth.").
					EchoMode(huh.EchoModePassword).
					Value(&gatewayToken),
			).Title("Gateway Settings"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		cfg.Gateway.Enabled = true
		if gatewayAddr != "" {
			cfg.Gateway.Address = gatewayAddr
		}
		cfg.Gateway.AuthToken = gatewayToken
	}

	// ── Part B: Heartbeat + Security ──
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable heartbeat?").
				Description("Proactive checks for tasks, reminders, and scheduled actions").
				Affirmative("Yes").
				Negative("No").
				Value(&hbEnabled),
		).Title("Heartbeat"),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Allow owner to run destructive commands?").
				Description("rm -rf, mkfs, dd, etc. — blocked by default").
				Affirmative("Yes").
				Negative("No (recommended)").
				Value(&allowDestr),

			huh.NewConfirm().
				Title("Allow owner/admin to use sudo?").
				Description("Blocked by default for safety").
				Affirmative("Yes").
				Negative("No (recommended)").
				Value(&allowSudo),

			huh.NewConfirm().
				Title("Require confirmation for dangerous tools?").
				Description("bash, ssh, scp, write_file will ask before executing").
				Affirmative("Yes (recommended)").
				Negative("No").
				Value(&confirmTools),
		).Title("Security"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if hbEnabled {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Heartbeat channel").
					Options(
						huh.NewOption("WhatsApp", "whatsapp"),
						huh.NewOption("CLI", "cli"),
					).
					Value(&hbChannel),
				huh.NewInput().
					Title("Chat ID for heartbeat messages").
					Description("Your phone number for WhatsApp, or session ID for CLI").
					Value(&hbChatID),
			).Title("Heartbeat Settings"),
		).WithTheme(huh.ThemeDracula()).Run()
		if err != nil {
			return err
		}
		cfg.Heartbeat.Enabled = true
		cfg.Heartbeat.Channel = hbChannel
		cfg.Heartbeat.ChatID = normalizePhone(hbChatID)
	}

	cfg.Security.ToolGuard.AllowDestructive = allowDestr
	cfg.Security.ToolGuard.AllowSudo = allowSudo
	if confirmTools {
		cfg.Security.ToolGuard.RequireConfirmation = []string{"bash", "ssh", "scp", "write_file"}
	} else {
		cfg.Security.ToolGuard.RequireConfirmation = []string{}
	}

	// ── Part C: Autonomy + Media + Logging ──
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Max turns per request").
				Description("How many reasoning steps the agent can take (default: 25)").
				Placeholder("25").
				Value(&maxTurns),

			huh.NewInput().
				Title("Max auto-continuations").
				Description("Auto-continue when agent is still working (default: 2, 0=disable)").
				Placeholder("2").
				Value(&maxCont),

			huh.NewConfirm().
				Title("Enable subagent orchestration?").
				Description("Allow the agent to spawn child agents for parallel tasks").
				Affirmative("Yes").
				Negative("No").
				Value(&subEnabled),
		).Title("Agent Autonomy"),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable vision (image understanding)?").
				Affirmative("Yes").
				Negative("No").
				Value(&visionEnabled),

			huh.NewConfirm().
				Title("Enable audio transcription (Whisper)?").
				Affirmative("Yes").
				Negative("No").
				Value(&audioEnabled),

			huh.NewSelect[string]().
				Title("Log level").
				Options(
					huh.NewOption("info — standard (recommended)", "info"),
					huh.NewOption("debug — verbose", "debug"),
					huh.NewOption("warn — warnings and errors only", "warn"),
					huh.NewOption("error — errors only", "error"),
				).
				Value(&logLevel),
		).Title("Media & Logging"),
	).WithTheme(huh.ThemeDracula()).Run()
	if err != nil {
		return err
	}

	if n := parseInt(maxTurns, cfg.Agent.MaxTurns); n > 0 {
		cfg.Agent.MaxTurns = n
	}
	if n := parseInt(maxCont, cfg.Agent.MaxContinuations); n >= 0 {
		cfg.Agent.MaxContinuations = n
	}
	cfg.Subagents.Enabled = subEnabled
	if subEnabled {
		if n := parseInt(maxConcurrent, cfg.Subagents.MaxConcurrent); n > 0 {
			cfg.Subagents.MaxConcurrent = n
		}
	}
	cfg.Media.VisionEnabled = visionEnabled
	cfg.Media.TranscriptionEnabled = audioEnabled
	cfg.Logging.Level = logLevel

	return nil
}

// printSummary displays the configuration summary.
func printSummary(cfg *copilot.Config, keyStorage storageMethod, installedSkills []string) {
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("  Configuration summary:")
	fmt.Println("─────────────────────────────────────────────")
	fmt.Printf("  Name:       %s\n", cfg.Name)
	fmt.Printf("  Trigger:    %s\n", cfg.Trigger)
	if len(cfg.Access.Owners) > 0 {
		fmt.Printf("  Owner:      %s\n", cfg.Access.Owners[0])
	}
	fmt.Printf("  Policy:     %s\n", cfg.Access.DefaultPolicy)
	fmt.Printf("  Model:      %s\n", cfg.Model)
	fmt.Printf("  API URL:    %s\n", cfg.API.BaseURL)
	if cfg.API.Provider != "" {
		fmt.Printf("  Provider:   %s\n", cfg.API.Provider)
	}
	switch keyStorage {
	case storageVault:
		fmt.Println("  API key:    **** (encrypted vault)")
	case storageKeyring:
		fmt.Println("  API key:    **** (OS keyring)")
	default:
		fmt.Println("  API key:    (not set — configure later)")
	}
	fmt.Printf("  Language:   %s\n", cfg.Language)
	fmt.Printf("  Timezone:   %s\n", cfg.Timezone)
	fmt.Printf("  Groups:     %v\n", cfg.Channels.WhatsApp.RespondToGroups)
	fmt.Printf("  DMs:        %v\n", cfg.Channels.WhatsApp.RespondToDMs)
	if len(cfg.Fallback.Models) > 0 {
		fmt.Printf("  Fallback:   %s\n", strings.Join(cfg.Fallback.Models, " → "))
	}
	if cfg.Gateway.Enabled {
		fmt.Printf("  Gateway:    %s (auth: %v)\n", cfg.Gateway.Address, cfg.Gateway.AuthToken != "")
	}
	if cfg.Heartbeat.Enabled {
		fmt.Printf("  Heartbeat:  every %s, hours %d-%d\n",
			cfg.Heartbeat.Interval, cfg.Heartbeat.ActiveStart, cfg.Heartbeat.ActiveEnd)
	}
	if cfg.Security.ToolGuard.AllowDestructive {
		fmt.Println("  Destructive: allowed (owner)")
	}
	if cfg.Security.ToolGuard.AllowSudo {
		fmt.Println("  Sudo:       allowed (owner/admin)")
	}
	fmt.Printf("  Agent:      max %d turns, %d auto-continues\n", cfg.Agent.MaxTurns, cfg.Agent.MaxContinuations)
	if cfg.Subagents.Enabled {
		fmt.Printf("  Subagents:  enabled (max %d concurrent)\n", cfg.Subagents.MaxConcurrent)
	}
	fmt.Printf("  Vision:     %v\n", cfg.Media.VisionEnabled)
	fmt.Printf("  Audio:      %v\n", cfg.Media.TranscriptionEnabled)
	fmt.Printf("  Log level:  %s\n", cfg.Logging.Level)

	// Channels
	var chans []string
	chans = append(chans, "whatsapp")
	if cfg.Channels.Telegram.Token != "" {
		chans = append(chans, "telegram")
	}
	if cfg.Channels.Discord.Token != "" {
		chans = append(chans, "discord")
	}
	if cfg.Channels.Slack.BotToken != "" {
		chans = append(chans, "slack")
	}
	fmt.Printf("  Channels:   %s\n", strings.Join(chans, ", "))

	// TTS
	if cfg.TTS.Enabled {
		fmt.Printf("  TTS:        %s (voice: %s, mode: %s)\n", cfg.TTS.Provider, cfg.TTS.Voice, cfg.TTS.AutoMode)
	}
	// Web UI
	if cfg.WebUI.Enabled {
		fmt.Printf("  Web UI:     %s (auth: %v)\n", cfg.WebUI.Address, cfg.WebUI.AuthToken != "")
	}

	if len(installedSkills) > 0 {
		fmt.Printf("  Skills:     %s\n", strings.Join(installedSkills, ", "))
	}
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println()
}

// parseInt parses a string as int, returning fallback on failure.
func parseInt(s string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return fallback
	}
	return n
}

// setupVault creates the encrypted vault and stores the API key in it.
// Returns the storage method used.
func setupVault(apiKey string) storageMethod {
	fmt.Println()
	fmt.Println("  Creating encrypted vault...")
	fmt.Println("  Choose a master password (minimum 8 characters).")
	fmt.Println("  This password is NEVER stored — only you know it.")
	fmt.Println()

	password, err := copilot.ReadPassword("  Master password: ")
	if err != nil {
		fmt.Printf("  [!] Failed to read password: %v\n", err)
		return tryKeyringFallback(apiKey)
	}
	if len(password) < 8 {
		fmt.Println("  [!] Password too short (minimum 8 characters).")
		return tryKeyringFallback(apiKey)
	}

	confirm, err := copilot.ReadPassword("  Confirm password: ")
	if err != nil || password != confirm {
		fmt.Println("  [!] Passwords don't match.")
		return tryKeyringFallback(apiKey)
	}

	vault := copilot.NewVault(copilot.VaultFile)

	// Remove existing vault if present (fresh setup).
	if vault.Exists() {
		_ = os.Remove(copilot.VaultFile)
		vault = copilot.NewVault(copilot.VaultFile)
	}

	if err := vault.Create(password); err != nil {
		fmt.Printf("  [!] Vault creation failed: %v\n", err)
		return tryKeyringFallback(apiKey)
	}

	if err := vault.Set("api_key", apiKey); err != nil {
		fmt.Printf("  [!] Failed to store key in vault: %v\n", err)
		vault.Lock()
		return tryKeyringFallback(apiKey)
	}

	vault.Lock()
	fmt.Println()
	fmt.Println("  ✓ API key encrypted and stored in vault.")
	return storageVault
}

// tryKeyringFallback attempts to store the API key in the OS keyring
// as a fallback when vault creation fails.
func tryKeyringFallback(apiKey string) storageMethod {
	if copilot.KeyringAvailable() {
		fmt.Println("  Trying OS keyring as fallback...")
		if err := copilot.StoreKeyring("api_key", apiKey); err == nil {
			fmt.Println("  API key stored in OS keyring.")
			return storageKeyring
		}
	}
	return storageNone
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
