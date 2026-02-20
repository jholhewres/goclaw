package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels/discord"
	slackchan "github.com/jholhewres/devclaw/pkg/devclaw/channels/slack"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/telegram"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/whatsapp"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/jholhewres/devclaw/pkg/devclaw/gateway"
	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
	"github.com/jholhewres/devclaw/pkg/devclaw/webui"
	"github.com/spf13/cobra"
)

// newServeCmd creates the `devclaw serve` command that starts the daemon.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the daemon with messaging channels",
		Long: `Start DevClaw as a daemon service, connecting to enabled
channels (WhatsApp, Discord, Telegram) and processing messages.

Examples:
  devclaw serve
  devclaw serve --channel whatsapp
  devclaw serve --config ./config.yaml`,
		RunE: runServe,
	}

	cmd.Flags().StringSlice("channel", nil, "channels to enable (whatsapp, discord, telegram)")
	return cmd
}

func runServe(cmd *cobra.Command, _ []string) error {
	// ── Load config ──
	cfg, configPath, err := resolveConfig(cmd)
	if err != nil {
		// No config? Start in web setup mode.
		return runWebSetupMode()
	}

	// ── Configure logger ──
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	logLevel := slog.LevelInfo
	if verbose || cfg.Logging.Level == "debug" {
		logLevel = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler)

	// ── Resolve secrets ──
	// Audit BEFORE resolving — checks the raw config values for hardcoded keys.
	copilot.AuditSecrets(cfg, logger)
	// Resolve from vault → keyring → env → config.
	// Returns unlocked vault (if available) for agent vault tools.
	vault := copilot.ResolveAPIKey(cfg, logger)

	// ── Create assistant ──
	assistant := copilot.New(cfg, logger)
	if vault != nil {
		assistant.SetVault(vault)
	}

	// ── Create context ──
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Register channels ──
	channelFilter, _ := cmd.Flags().GetStringSlice("channel")

	// WhatsApp (core channel).
	var wa *whatsapp.WhatsApp
	if shouldEnable("whatsapp", channelFilter, true) {
		wa = whatsapp.New(cfg.Channels.WhatsApp, logger)
		if err := assistant.ChannelManager().Register(wa); err != nil {
			logger.Error("failed to register WhatsApp", "error", err)
		} else {
			logger.Info("WhatsApp channel registered")
		}
	}

	// Telegram (core channel).
	if shouldEnable("telegram", channelFilter, false) && cfg.Channels.Telegram.Token != "" {
		tg := telegram.New(cfg.Channels.Telegram, logger)
		if err := assistant.ChannelManager().Register(tg); err != nil {
			logger.Error("failed to register Telegram", "error", err)
		} else {
			logger.Info("Telegram channel registered")
		}
	}

	// Slack (core channel).
	if shouldEnable("slack", channelFilter, false) && cfg.Channels.Slack.BotToken != "" {
		sl := slackchan.New(cfg.Channels.Slack, logger)
		if err := assistant.ChannelManager().Register(sl); err != nil {
			logger.Error("failed to register Slack", "error", err)
		} else {
			logger.Info("Slack channel registered")
		}
	}

	// Discord (core channel).
	if shouldEnable("discord", channelFilter, false) && cfg.Channels.Discord.Token != "" {
		dc := discord.New(cfg.Channels.Discord, logger)
		if err := assistant.ChannelManager().Register(dc); err != nil {
			logger.Error("failed to register Discord", "error", err)
		} else {
			logger.Info("Discord channel registered")
		}
	}

	// Load plugins (other channels).
	pluginLoader := plugins.NewLoader(cfg.Plugins, logger)
	if err := pluginLoader.LoadAll(ctx); err != nil {
		logger.Error("failed to load plugins", "error", err)
	} else if pluginLoader.Count() > 0 {
		if err := pluginLoader.RegisterChannels(assistant.ChannelManager()); err != nil {
			logger.Error("failed to register plugin channels", "error", err)
		}
	}

	// ── Start Web UI first (independent of channels) ──
	var webServer *webui.Server
	var adapter *webui.AssistantAdapter
	if cfg.WebUI.Enabled {
		adapter = buildWebUIAdapter(assistant, cfg, wa, configPath)
		webServer = webui.New(cfg.WebUI, adapter, logger)
		if err := webServer.Start(ctx); err != nil {
			logger.Error("failed to start web UI", "error", err)
		} else {
			logger.Info("web UI running", "address", cfg.WebUI.Address)
		}
	}

	// ── Start assistant (channels, scheduler, heartbeat, etc.) ──
	if err := assistant.Start(ctx); err != nil {
		logger.Warn("assistant started with warnings", "error", err)
		logger.Info("channels pending — connect via web UI", "url", fmt.Sprintf("http://localhost%s/channels", cfg.WebUI.Address))
	}

	// ── Start gateway if enabled ──
	var gw *gateway.Gateway
	if cfg.Gateway.Enabled {
		gw = gateway.New(assistant, cfg.Gateway, logger)
		if err := gw.Start(ctx); err != nil {
			logger.Error("failed to start gateway", "error", err)
		} else {
			logger.Info("gateway running", "address", cfg.Gateway.Address)
		}
	}

	// ── Wire webhook management to WebUI adapter ──
	if webServer != nil {
		wireWebhookAdapter(adapter, gw)
	}

	// ── Start config watcher for hot-reload ──
	if configPath != "" {
		watcher := copilot.NewConfigWatcher(
			configPath,
			5*time.Second,
			assistant.ApplyConfigUpdate,
			logger,
		)
		go watcher.Start(ctx)
		logger.Info("config watcher started", "path", configPath)
	}

	// ── Wait for shutdown ──
	logger.Info("DevClaw Copilot running. Press Ctrl+C to stop.",
		"name", cfg.Name,
		"trigger", cfg.Trigger,
		"policy", cfg.Access.DefaultPolicy,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutdown signal received, stopping...")

	// Graceful shutdown with timeout.
	done := make(chan struct{})
	go func() {
		pluginLoader.Shutdown()
		if gw != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = gw.Stop(shutdownCtx)
			cancel()
		}
		if webServer != nil {
			webServer.Stop()
		}
		assistant.Stop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(10 * time.Second):
		logger.Warn("shutdown timed out after 10s, forcing exit")
	}

	return nil
}

// resolveConfig loads config from file, runs interactive setup if missing.
// Returns (config, configPath, error). configPath is empty if config came from discovery without a known path.
func resolveConfig(cmd *cobra.Command) (*copilot.Config, string, error) {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")

	// Try explicit path first.
	if configPath != "" {
		cfg, err := copilot.LoadConfigFromFile(configPath)
		if err != nil {
			return nil, "", fmt.Errorf("loading config: %w", err)
		}
		return cfg, configPath, nil
	}

	// Auto-discover config file.
	if found := copilot.FindConfigFile(); found != "" {
		cfg, err := copilot.LoadConfigFromFile(found)
		if err != nil {
			return nil, "", fmt.Errorf("loading config from %s: %w", found, err)
		}
		slog.Info("config loaded", "path", found)
		return cfg, found, nil
	}

	// No config file found — the caller (runServe) will fall back to
	// web setup mode. CLI setup is still available via `copilot setup`.
	return nil, "", fmt.Errorf("no configuration file found")
}

// runWebSetupMode starts a minimal webui server in setup-only mode.
// Blocks until the setup wizard completes or the user cancels.
func runWebSetupMode() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	fmt.Println()
	fmt.Println("  ╭──────────────────────────────────────────────╮")
	fmt.Println("  │  🐾 DevClaw — First Run Setup                 │")
	fmt.Println("  │                                              │")
	fmt.Println("  │  No config.yaml found.                       │")
	fmt.Println("  │  Starting web setup wizard...                │")
	fmt.Println("  │                                              │")
	fmt.Println("  │  Open:  http://localhost:8090/setup           │")
	fmt.Println("  ╰──────────────────────────────────────────────╯")
	fmt.Println()

	setupDone := make(chan struct{})

	// Start a webui server in setup-only mode (no assistant needed).
	webuiCfg := webui.Config{
		Enabled: true,
		Address: ":8090",
	}
	webServer := webui.New(webuiCfg, nil, logger)
	webServer.SetSetupMode(true)
	webServer.OnSetupDone(func() {
		close(setupDone)
	})
	webServer.OnVaultInit(func(password string, secrets map[string]string) error {
		vault := copilot.NewVault(copilot.VaultFile)
		if vault.Exists() {
			// Vault already exists — unlock and add secrets.
			if err := vault.Unlock(password); err != nil {
				return fmt.Errorf("failed to unlock existing vault: %w", err)
			}
		} else {
			if err := vault.Create(password); err != nil {
				return fmt.Errorf("failed to create vault: %w", err)
			}
		}
		for name, value := range secrets {
			if err := vault.Set(name, value); err != nil {
				return fmt.Errorf("failed to store %s in vault: %w", name, err)
			}
		}
		return nil
	})

	if err := webServer.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start setup server: %w", err)
	}

	// Wait for setup completion or interrupt.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-setupDone:
		webServer.Stop()
		fmt.Println()
		fmt.Println("Setup complete! config.yaml saved.")
		fmt.Println("Restarting...")
		return nil
	case <-sigChan:
		webServer.Stop()
		return nil
	}
}

// shouldEnable checks if a channel should be enabled.
func shouldEnable(name string, filter []string, defaultEnabled bool) bool {
	if len(filter) == 0 {
		return defaultEnabled
	}
	for _, f := range filter {
		if f == name {
			return true
		}
	}
	return false
}

// wireWebhookAdapter connects webhook management functions to the WebUI adapter.
// Called after the gateway is created (may be nil if gateway is disabled).
func wireWebhookAdapter(adapter *webui.AssistantAdapter, gw *gateway.Gateway) {
	if gw == nil {
		adapter.ListWebhooksFn = func() []webui.WebhookInfo { return nil }
		adapter.CreateWebhookFn = func(string, []string) (webui.WebhookInfo, error) {
			return webui.WebhookInfo{}, fmt.Errorf("Gateway API is not enabled")
		}
		adapter.DeleteWebhookFn = func(string) error {
			return fmt.Errorf("Gateway API is not enabled")
		}
		adapter.ToggleWebhookFn = func(string, bool) error {
			return fmt.Errorf("Gateway API is not enabled")
		}
		adapter.GetValidWebhookEventsFn = func() []string { return gateway.ValidWebhookEvents }
		return
	}

	adapter.ListWebhooksFn = func() []webui.WebhookInfo {
		entries := gw.ListWebhooks()
		result := make([]webui.WebhookInfo, len(entries))
		for i, e := range entries {
			result[i] = webui.WebhookInfo{
				ID:        e.ID,
				URL:       e.URL,
				Events:    e.Events,
				Active:    e.Active,
				CreatedAt: e.CreatedAt,
			}
		}
		return result
	}
	adapter.CreateWebhookFn = func(url string, events []string) (webui.WebhookInfo, error) {
		entry, err := gw.AddWebhook(url, events)
		if err != nil {
			return webui.WebhookInfo{}, err
		}
		return webui.WebhookInfo{
			ID:        entry.ID,
			URL:       entry.URL,
			Events:    entry.Events,
			Active:    entry.Active,
			CreatedAt: entry.CreatedAt,
		}, nil
	}
	adapter.DeleteWebhookFn = func(id string) error {
		if !gw.DeleteWebhook(id) {
			return fmt.Errorf("webhook %q not found", id)
		}
		return nil
	}
	adapter.ToggleWebhookFn = func(id string, active bool) error {
		if !gw.ToggleWebhook(id, active) {
			return fmt.Errorf("webhook %q not found", id)
		}
		return nil
	}
	adapter.GetValidWebhookEventsFn = func() []string {
		return gateway.ValidWebhookEvents
	}
}

// buildWebUIAdapter creates the adapter that bridges the Assistant to the WebUI.
func buildWebUIAdapter(assistant *copilot.Assistant, cfg *copilot.Config, wa *whatsapp.WhatsApp, configPath string) *webui.AssistantAdapter {
	adapter := &webui.AssistantAdapter{
		GetConfigMapFn: func() map[string]any {
			media := cfg.Media.Effective()
			return map[string]any{
				"name":               cfg.Name,
				"trigger":            cfg.Trigger,
				"model":              cfg.Model,
				"language":           cfg.Language,
				"timezone":           cfg.Timezone,
				"provider":           cfg.API.Provider,
				"base_url":           cfg.API.BaseURL,
				"api_key_configured": cfg.API.APIKey != "",
				"media": map[string]any{
					"vision_enabled":          media.VisionEnabled,
					"vision_model":            media.VisionModel,
					"vision_detail":           media.VisionDetail,
					"transcription_enabled":   media.TranscriptionEnabled,
					"transcription_model":     media.TranscriptionModel,
					"transcription_base_url":  media.TranscriptionBaseURL,
					"transcription_api_key":   media.TranscriptionAPIKey != "",
					"transcription_language":  media.TranscriptionLanguage,
				},
			}
		},
		UpdateConfigMapFn: func(updates map[string]any) error {
			// Update provider & model.
			if v, ok := updates["provider"].(string); ok && v != "" {
				cfg.API.Provider = v
			}
			if v, ok := updates["model"].(string); ok && v != "" {
				cfg.Model = v
			}
			if v, ok := updates["base_url"].(string); ok {
				cfg.API.BaseURL = v
			}
			if v, ok := updates["api_key"].(string); ok && v != "" {
				cfg.API.APIKey = v
			}

			// Update media config.
			if mediaRaw, ok := updates["media"]; ok {
				if mediaMap, ok := mediaRaw.(map[string]any); ok {
					media := cfg.Media
					if v, ok := mediaMap["vision_enabled"].(bool); ok {
						media.VisionEnabled = v
					}
					if v, ok := mediaMap["vision_model"].(string); ok {
						media.VisionModel = v
					}
					if v, ok := mediaMap["vision_detail"].(string); ok {
						media.VisionDetail = v
					}
					if v, ok := mediaMap["transcription_enabled"].(bool); ok {
						media.TranscriptionEnabled = v
					}
					if v, ok := mediaMap["transcription_model"].(string); ok {
						media.TranscriptionModel = v
					}
					if v, ok := mediaMap["transcription_base_url"].(string); ok {
						media.TranscriptionBaseURL = v
					}
					if v, ok := mediaMap["transcription_api_key"].(string); ok && v != "" {
						media.TranscriptionAPIKey = v
					}
					if v, ok := mediaMap["transcription_language"].(string); ok {
						media.TranscriptionLanguage = v
					}
					cfg.Media = media
					assistant.UpdateMediaConfig(media)
				}
			}
			savePath := configPath
			if savePath == "" {
				savePath = "config.yaml"
			}
			return copilot.SaveConfigToFile(cfg, savePath)
		},
		ListSessionsFn: func() []webui.SessionInfo {
			sessions := assistant.SessionStore().ListSessions()
			result := make([]webui.SessionInfo, len(sessions))
			for i, s := range sessions {
				result[i] = webui.SessionInfo{
					ID:            s.ID,
					Channel:       s.Channel,
					ChatID:        s.ChatID,
					MessageCount:  s.MessageCount,
					CreatedAt:     s.CreatedAt,
					LastMessageAt: s.LastActiveAt,
				}
			}
			return result
		},
		GetSessionMessagesFn: func(sessionID string) []webui.MessageInfo {
			session := assistant.SessionStore().GetByID(sessionID)
			if session == nil {
				return nil
			}
			entries := session.RecentHistory(50)
			result := make([]webui.MessageInfo, 0, len(entries)*2)
			for _, e := range entries {
				result = append(result, webui.MessageInfo{
					Role:      "user",
					Content:   e.UserMessage,
					Timestamp: e.Timestamp,
				})
				if e.AssistantResponse != "" {
					result = append(result, webui.MessageInfo{
						Role:      "assistant",
						Content:   e.AssistantResponse,
						Timestamp: e.Timestamp,
					})
				}
			}
			return result
		},
		GetUsageGlobalFn: func() webui.UsageInfo {
			usage := assistant.UsageTracker().GetGlobal()
			if usage == nil {
				return webui.UsageInfo{}
			}
			return webui.UsageInfo{
				TotalInputTokens:  usage.PromptTokens,
				TotalOutputTokens: usage.CompletionTokens,
				TotalCost:         usage.EstimatedCostUSD,
				RequestCount:      usage.Requests,
			}
		},
		GetChannelHealthFn: func() []webui.ChannelHealthInfo {
			healthMap := assistant.ChannelManager().HealthAll()
			result := make([]webui.ChannelHealthInfo, 0, len(healthMap))
			for name, h := range healthMap {
				result = append(result, webui.ChannelHealthInfo{
					Name:       name,
					Connected:  h.Connected,
					ErrorCount: h.ErrorCount,
					LastMsgAt:  h.LastMessageAt,
				})
			}
			return result
		},
		GetSchedulerJobsFn: func() []webui.JobInfo {
			sched := assistant.Scheduler()
			if sched == nil {
				return nil
			}
			jobs := sched.List()
			result := make([]webui.JobInfo, len(jobs))
			for i, j := range jobs {
				var lastRun time.Time
				if j.LastRunAt != nil {
					lastRun = *j.LastRunAt
				}
				result[i] = webui.JobInfo{
					ID:        j.ID,
					Schedule:  j.Schedule,
					Type:      j.Type,
					Command:   j.Command,
					Enabled:   j.Enabled,
					RunCount:  j.RunCount,
					LastRunAt: lastRun,
					LastError: j.LastError,
				}
			}
			return result
		},
		ListSkillsFn: func() []webui.SkillInfo {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return nil
			}
			metas := reg.List()
			result := make([]webui.SkillInfo, len(metas))
			for i, m := range metas {
				result[i] = webui.SkillInfo{
					Name:        m.Name,
					Description: m.Description,
					Enabled:     reg.IsEnabled(m.Name),
				}
			}
			return result
		},
		ToggleSkillFn: func(name string, enabled bool) error {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return fmt.Errorf("skill registry not available")
			}
			if enabled {
				return reg.Enable(name)
			}
			return reg.Disable(name)
		},
		SendChatMessageFn: func(sessionID, content string) (string, error) {
			session := assistant.SessionStore().GetOrCreate("webui", sessionID)
			prompt := assistant.ComposePrompt(session, content)
			resp := assistant.ExecuteAgent(context.Background(), prompt, session, content)
			session.AddMessage(content, resp)
			return resp, nil
		},
		StartChatStreamFn: func(_ context.Context, sessionID, content string) (*webui.RunHandle, error) {
			session := assistant.SessionStore().GetOrCreate("webui", sessionID)
			prompt := assistant.ComposePrompt(session, content)

			runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
			events := make(chan webui.StreamEvent, 256)

			// FIX: Use context.Background() — the agent must outlive the POST /send
			// request. The context is cancelled by handle.Cancel() when the SSE
			// client disconnects or the user aborts.
			runCtx, cancel := context.WithCancel(context.Background())

			handle := &webui.RunHandle{
				RunID:     runID,
				SessionID: sessionID,
				Events:    events,
				Cancel:    cancel,
			}

			// Run the agent in a goroutine, streaming events via the channel.
			go func() {
				defer close(events)
				defer cancel() // Ensure context is always cleaned up.

				history := session.RecentHistory(10)
				agent := copilot.NewAgentRunWithConfig(
					assistant.LLMClient(),
					assistant.ToolExecutor(),
					cfg.Agent,
					slog.Default(),
				)

				// Propagate caller context via context.Context (goroutine-safe).
				runCtx = copilot.ContextWithCaller(runCtx, copilot.AccessOwner, "webui")
				runCtx = copilot.ContextWithSession(runCtx, sessionID)

				// Stream text tokens to the SSE channel.
				agent.SetStreamCallback(func(chunk string) {
					select {
					case events <- webui.StreamEvent{
						Type: "delta",
						Data: map[string]string{"content": chunk},
					}:
					case <-runCtx.Done():
					}
				})

				// Record token usage.
				if assistant.UsageTracker() != nil {
					agent.SetUsageRecorder(func(model string, usage copilot.LLMUsage) {
						assistant.UsageTracker().Record(session.ID, model, usage)
					})
				}

				resp, usage, err := agent.RunWithUsage(runCtx, prompt, history, content)
				if err != nil {
					// FIX: Non-blocking sends to avoid deadlock when client is gone.
					if runCtx.Err() != nil {
						select {
						case events <- webui.StreamEvent{
							Type: "error",
							Data: map[string]string{"message": "Execution cancelled"},
						}:
						default:
						}
						return
					}
					select {
					case events <- webui.StreamEvent{
						Type: "error",
						Data: map[string]string{"message": err.Error()},
					}:
					case <-runCtx.Done():
					}
					return
				}

				// Persist the conversation.
				session.AddMessage(content, resp)

				if usage != nil {
					session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
				}

				// Send done event with usage stats (non-blocking).
				usageData := map[string]int{"input_tokens": 0, "output_tokens": 0}
				if usage != nil {
					usageData["input_tokens"] = usage.PromptTokens
					usageData["output_tokens"] = usage.CompletionTokens
				}
				select {
				case events <- webui.StreamEvent{
					Type: "done",
					Data: map[string]any{"usage": usageData},
				}:
				case <-runCtx.Done():
				}
			}()

			return handle, nil
		},
		AbortRunFn: func(sessionID string) bool {
			// First try to stop via the assistant's active runs (channel-driven).
			if assistant.StopActiveRun("default", "webui:"+sessionID) {
				return true
			}
			// Web UI runs are cancelled via RunHandle.Cancel() in the SSE handler.
			// This path is a fallback — the primary abort is via the webui server
			// which calls handle.Cancel() directly.
			return false
		},
		DeleteSessionFn: func(sessionID string) error {
			deleted := assistant.SessionStore().Delete("webui", sessionID)
			if !deleted {
				// Try with the raw ID (might include channel prefix already).
				parts := strings.SplitN(sessionID, ":", 2)
				if len(parts) == 2 {
					assistant.SessionStore().Delete(parts[0], parts[1])
				}
			}
			return nil
		},
	}

	// ── Security: Audit Log ──
	adapter.GetAuditLogFn = func(limit int) []webui.AuditEntry {
		guard := assistant.ToolExecutor().Guard()
		if guard == nil {
			return nil
		}
		audit := guard.SQLiteAudit()
		if audit == nil {
			return nil
		}
		records := audit.RecentRecords(limit)
		entries := make([]webui.AuditEntry, len(records))
		for i, r := range records {
			entries[i] = webui.AuditEntry{
				ID:            r.ID,
				Tool:          r.Tool,
				Caller:        r.Caller,
				Level:         r.Level,
				Allowed:       r.Allowed,
				ArgsSummary:   r.ArgsSummary,
				ResultSummary: r.ResultSummary,
				CreatedAt:     r.CreatedAt,
			}
		}
		return entries
	}
	adapter.GetAuditCountFn = func() int {
		guard := assistant.ToolExecutor().Guard()
		if guard == nil {
			return 0
		}
		audit := guard.SQLiteAudit()
		if audit == nil {
			return 0
		}
		return audit.Count()
	}

	// ── Security: Tool Guard ──
	adapter.GetToolGuardStatusFn = func() webui.ToolGuardStatus {
		gc := cfg.Security.ToolGuard
		return webui.ToolGuardStatus{
			Enabled:             gc.Enabled,
			AllowDestructive:    gc.AllowDestructive,
			AllowSudo:           gc.AllowSudo,
			AllowReboot:         gc.AllowReboot,
			AutoApprove:         gc.AutoApprove,
			RequireConfirmation: gc.RequireConfirmation,
			ProtectedPaths:      gc.ProtectedPaths,
			SSHAllowedHosts:     gc.SSHAllowedHosts,
			DangerousCommands:   gc.DangerousCommands,
			ToolPermissions:     gc.ToolPermissions,
		}
	}
	adapter.UpdateToolGuardFn = func(update webui.ToolGuardStatus) error {
		cfg.Security.ToolGuard.AllowDestructive = update.AllowDestructive
		cfg.Security.ToolGuard.AllowSudo = update.AllowSudo
		cfg.Security.ToolGuard.AllowReboot = update.AllowReboot
		if update.AutoApprove != nil {
			cfg.Security.ToolGuard.AutoApprove = update.AutoApprove
		}
		if update.RequireConfirmation != nil {
			cfg.Security.ToolGuard.RequireConfirmation = update.RequireConfirmation
		}
		if update.ProtectedPaths != nil {
			cfg.Security.ToolGuard.ProtectedPaths = update.ProtectedPaths
		}
		if update.SSHAllowedHosts != nil {
			cfg.Security.ToolGuard.SSHAllowedHosts = update.SSHAllowedHosts
		}
		// Apply hot-reload to the running tool guard.
		assistant.ToolExecutor().UpdateGuardConfig(cfg.Security.ToolGuard)
		return nil
	}

	// ── Security: Vault ──
	adapter.GetVaultStatusFn = func() webui.VaultStatus {
		v := assistant.Vault()
		if v == nil {
			return webui.VaultStatus{Exists: false}
		}
		status := webui.VaultStatus{
			Exists:   v.Exists(),
			Unlocked: v.IsUnlocked(),
		}
		if v.IsUnlocked() {
			status.Keys = v.List()
			if status.Keys == nil {
				status.Keys = []string{}
			}
		}
		return status
	}

	// ── Security: Overview ──
	adapter.GetSecurityStatusFn = func() webui.SecurityStatus {
		s := webui.SecurityStatus{
			GatewayAuthConfigured: cfg.Gateway.AuthToken != "",
			WebUIAuthConfigured:   cfg.WebUI.AuthToken != "",
			ToolGuardEnabled:      cfg.Security.ToolGuard.Enabled,
		}
		if v := assistant.Vault(); v != nil {
			s.VaultExists = v.Exists()
			s.VaultUnlocked = v.IsUnlocked()
		}
		if guard := assistant.ToolExecutor().Guard(); guard != nil {
			if audit := guard.SQLiteAudit(); audit != nil {
				s.AuditEntryCount = audit.Count()
			}
		}
		return s
	}

	// ── Hooks (Lifecycle) ──
	adapter.ListHooksFn = func() []webui.HookInfo {
		hm := assistant.HookManager()
		if hm == nil {
			return nil
		}
		summaries := hm.ListDetailed()
		result := make([]webui.HookInfo, len(summaries))
		for i, s := range summaries {
			events := make([]string, len(s.Events))
			for j, ev := range s.Events {
				events[j] = string(ev)
			}
			result[i] = webui.HookInfo{
				Name:        s.Name,
				Description: s.Description,
				Source:      s.Source,
				Events:      events,
				Priority:    s.Priority,
				Enabled:     s.Enabled,
			}
		}
		return result
	}
	adapter.ToggleHookFn = func(name string, enabled bool) error {
		hm := assistant.HookManager()
		if hm == nil {
			return fmt.Errorf("hook manager not available")
		}
		if !hm.SetEnabled(name, enabled) {
			return fmt.Errorf("hook %q not found", name)
		}
		return nil
	}
	adapter.UnregisterHookFn = func(name string) error {
		hm := assistant.HookManager()
		if hm == nil {
			return fmt.Errorf("hook manager not available")
		}
		if !hm.Unregister(name) {
			return fmt.Errorf("hook %q not found", name)
		}
		return nil
	}
	adapter.GetHookEventsFn = func() []webui.HookEventInfo {
		hm := assistant.HookManager()
		if hm == nil {
			return nil
		}
		hooksByEvent := hm.ListHooks()
		result := make([]webui.HookEventInfo, 0, len(copilot.AllHookEvents))
		for _, ev := range copilot.AllHookEvents {
			names := hooksByEvent[ev]
			if names == nil {
				names = []string{}
			}
			result = append(result, webui.HookEventInfo{
				Event:       string(ev),
				Description: copilot.HookEventDescription(ev),
				Hooks:       names,
			})
		}
		return result
	}

	// Wire up WhatsApp QR callbacks if WhatsApp channel is available.
	if wa != nil {
		adapter.GetWhatsAppStatusFn = func() webui.WhatsAppStatus {
			return webui.WhatsAppStatus{
				Connected: wa.IsConnected(),
				NeedsQR:   wa.NeedsQR(),
			}
		}
		adapter.SubscribeWhatsAppQRFn = func() (chan webui.WhatsAppQREvent, func()) {
			ch, unsub := wa.SubscribeQR()
			// Bridge whatsapp.QREvent → webui.WhatsAppQREvent
			out := make(chan webui.WhatsAppQREvent, 8)
			go func() {
				defer close(out)
				for evt := range ch {
					out <- webui.WhatsAppQREvent{
						Type:    evt.Type,
						Code:    evt.Code,
						Message: evt.Message,
					}
				}
			}()
			return out, unsub
		}
		adapter.RequestWhatsAppQRFn = func() error {
			return wa.RequestNewQR(context.Background())
		}
	}

	return adapter
}
