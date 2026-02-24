package commands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
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
	"github.com/jholhewres/devclaw/pkg/devclaw/media"
	"github.com/jholhewres/devclaw/pkg/devclaw/paths"
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
	// ── Ensure state directories exist ──
	if err := paths.EnsureStateDirs(); err != nil {
		return fmt.Errorf("failed to create state directories: %w", err)
	}

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

	// ── Run startup verification ──
	verifier := copilot.NewStartupVerifier(cfg, vault, logger)
	startupReport := verifier.RunAll()
	verifier.PrintReport(startupReport)
	if !startupReport.Healthy {
		logger.Error("startup verification failed, some required checks did not pass")
		return fmt.Errorf("startup verification failed")
	}

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
		// Use the main devclaw database for WhatsApp sessions.
		// This stores whatsmeow tables alongside other devclaw data.
		waCfg := cfg.Channels.WhatsApp
		if waCfg.DatabasePath == "" && cfg.Database.Path != "" {
			waCfg.DatabasePath = cfg.Database.Path
		}
		wa = whatsapp.New(waCfg, logger)
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

	// ── Wire media service to WebUI ──
	if webServer != nil {
		wireMediaAdapter(webServer, assistant, logger)
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
// After setup, it automatically reloads the process.
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
		fmt.Println("Reloading...")
		// Small delay to ensure server is fully stopped
		time.Sleep(500 * time.Millisecond)
		return reloadProcess()
	case <-sigChan:
		webServer.Stop()
		return nil
	}
}

// reloadProcess replaces the current process with a new instance.
// This is used after setup to start the service with the new config.
func reloadProcess() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get the original arguments (skip the program name)
	args := os.Args[1:]

	// Replace current process with a new instance
	err = syscall.Exec(executable, append([]string{executable}, args...), os.Environ())
	if err != nil {
		return fmt.Errorf("failed to reload process: %w", err)
	}

	return nil
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

// anySliceToStringSlice converts []any to []string.
func anySliceToStringSlice(items []any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
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

// wireMediaAdapter connects the MediaService to the WebUI server.
func wireMediaAdapter(webServer *webui.Server, assistant *copilot.Assistant, logger *slog.Logger) {
	mediaSvc := assistant.GetMediaService()
	if mediaSvc == nil {
		logger.Debug("native media service not available")
		return
	}

	adapter := &webui.MediaAdapter{
		UploadFn: func(r *http.Request, sessionID string) (string, string, string, int64, error) {
			// Parse multipart form
			if err := r.ParseMultipartForm(50 * 1024 * 1024); err != nil {
				return "", "", "", 0, fmt.Errorf("failed to parse form: %w", err)
			}

			file, header, err := r.FormFile("file")
			if err != nil {
				return "", "", "", 0, fmt.Errorf("no file provided: %w", err)
			}
			defer file.Close()

			// Read file data
			data, err := io.ReadAll(file)
			if err != nil {
				return "", "", "", 0, fmt.Errorf("failed to read file: %w", err)
			}

			// Upload to media service
			media, err := mediaSvc.Upload(r.Context(), media.UploadRequest{
				Data:      data,
				Filename:  header.Filename,
				Channel:   "ui",
				SessionID: sessionID,
				Temporary: r.FormValue("temporary") == "true",
			})
			if err != nil {
				return "", "", "", 0, err
			}

			return media.ID, string(media.Type), media.Filename, media.Size, nil
		},
		GetFn: func(mediaID string) ([]byte, string, string, error) {
			data, storedMedia, err := mediaSvc.Get(context.Background(), mediaID)
			if err != nil {
				return nil, "", "", err
			}
			return data, storedMedia.MimeType, storedMedia.Filename, nil
		},
		ListFn: func(sessionID string, mediaType string, limit int) ([]webui.MediaInfo, error) {
			medias, err := mediaSvc.List(context.Background(), media.ListFilter{
				SessionID: sessionID,
				Type:      media.MediaType(mediaType),
				Limit:     limit,
			})
			if err != nil {
				return nil, err
			}

			result := make([]webui.MediaInfo, len(medias))
			for i, m := range medias {
				result[i] = webui.MediaInfo{
					ID:        m.ID,
					Filename:  m.Filename,
					Type:      string(m.Type),
					Size:      m.Size,
					URL:       mediaSvc.URL(m.ID),
					CreatedAt: m.CreatedAt.Format(time.RFC3339),
				}
			}
			return result, nil
		},
		DeleteFn: func(mediaID string) error {
			return mediaSvc.Delete(context.Background(), mediaID)
		},
	}

	webServer.SetMediaAPI(adapter)
	logger.Info("media API wired to web UI")
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
				"params":             cfg.API.Params,
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
				"access": map[string]any{
					"default_policy":  cfg.Access.DefaultPolicy,
					"owners":          cfg.Access.Owners,
					"admins":          cfg.Access.Admins,
					"allowed_users":   cfg.Access.AllowedUsers,
					"blocked_users":   cfg.Access.BlockedUsers,
					"pending_message": cfg.Access.PendingMessage,
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
				// Store in vault if available and unlocked (preferred for security)
				if vault := assistant.Vault(); vault != nil && vault.IsUnlocked() {
					providerKey := copilot.GetProviderKeyName(cfg.API.Provider)
					if err := vault.Set(providerKey, v); err != nil {
						return fmt.Errorf("failed to store API key in vault: %w", err)
					}
					// Inject into current process environment for immediate use
					os.Setenv(providerKey, v)
				}
				// Set in config for immediate use (will be sanitized on save)
				cfg.API.APIKey = v
			}
			// Update API params (provider-specific settings like context1m, tool_stream).
			if paramsRaw, ok := updates["params"]; ok {
				if paramsMap, ok := paramsRaw.(map[string]any); ok {
					if cfg.API.Params == nil {
						cfg.API.Params = make(map[string]any)
					}
					maps.Copy(cfg.API.Params, paramsMap)
				}
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

			// Update access config.
			if accessRaw, ok := updates["access"]; ok {
				if accessMap, ok := accessRaw.(map[string]any); ok {
					if v, ok := accessMap["default_policy"].(string); ok {
						cfg.Access.DefaultPolicy = copilot.AccessPolicy(v)
					}
					if v, ok := accessMap["owners"].([]any); ok {
						cfg.Access.Owners = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["admins"].([]any); ok {
						cfg.Access.Admins = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["allowed_users"].([]any); ok {
						cfg.Access.AllowedUsers = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["blocked_users"].([]any); ok {
						cfg.Access.BlockedUsers = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["pending_message"].(string); ok {
						cfg.Access.PendingMessage = v
					}
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
					// Strip internal tags like [[reply_to_current]] before sending to UI
					cleanChunk := copilot.StripInternalTags(chunk)
					if cleanChunk == "" {
						return // Skip empty chunks after stripping
					}
					select {
					case events <- webui.StreamEvent{
						Type: "delta",
						Data: map[string]string{"content": cleanChunk},
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
			health := wa.Health()
			state := wa.GetState()
			status := webui.WhatsAppStatus{
				Connected:  wa.IsConnected(),
				State:      string(state),
				NeedsQR:    wa.NeedsQR(),
				ErrorCount: health.ErrorCount,
			}

			// Add details from health.
			if jid, ok := health.Details["jid"].(string); ok {
				status.Phone = jid
			}
			if platform, ok := health.Details["platform"].(string); ok {
				status.Platform = platform
			}
			if attempts, ok := health.Details["reconnect_attempts"].(int); ok {
				status.ReconnectAttempts = attempts
			}

			// Add human-readable message based on state.
			switch state {
			case "connected":
				status.Message = "Connected"
			case "disconnected":
				status.Message = "Disconnected"
			case "connecting":
				status.Message = "Connecting..."
			case "reconnecting":
				status.Message = fmt.Sprintf("Reconnecting (attempt %d)...", status.ReconnectAttempts)
			case "waiting_qr":
				status.Message = "Waiting for QR code scan"
			case "banned":
				status.Message = "Account temporarily banned"
			case "logging_out":
				status.Message = "Logging out..."
			}

			return status
		}
		adapter.SubscribeWhatsAppQRFn = func() (chan webui.WhatsAppQREvent, func()) {
			ch, unsub := wa.SubscribeQR()
			// Bridge whatsapp.QREvent → webui.WhatsAppQREvent
			out := make(chan webui.WhatsAppQREvent, 8)
			go func() {
				defer close(out)
				for evt := range ch {
					out <- webui.WhatsAppQREvent{
						Type:        evt.Type,
						Code:        evt.Code,
						Message:     evt.Message,
						ExpiresAt:   evt.ExpiresAt.Format(time.RFC3339),
						SecondsLeft: evt.SecondsLeft,
					}
				}
			}()
			return out, unsub
		}
		adapter.RequestWhatsAppQRFn = func() error {
			return wa.RequestNewQR(context.Background())
		}
	}

	// ── MCP Servers ──
	adapter.ListMCPServersFn = func() []webui.MCPServerInfo {
		mcpCfg := cfg.MCP
		if mcpCfg.Servers == nil {
			return nil
		}
		result := make([]webui.MCPServerInfo, 0, len(mcpCfg.Servers))
		for _, srv := range mcpCfg.Servers {
			env := make(map[string]string)
			for k, v := range srv.Env {
				// Mask sensitive values
				if strings.Contains(strings.ToLower(k), "token") ||
					strings.Contains(strings.ToLower(k), "key") ||
					strings.Contains(strings.ToLower(k), "secret") ||
					strings.Contains(strings.ToLower(k), "password") {
					env[k] = "***"
				} else {
					env[k] = v
				}
			}
			info := webui.MCPServerInfo{
				Name:    srv.Name,
				Command: srv.Command,
				Args:    srv.Args,
				Env:     env,
				Enabled: srv.Enabled,
				Status:  "stopped", // Default status, actual status requires runtime tracking
			}
			result = append(result, info)
		}
		return result
	}
	adapter.CreateMCPServerFn = func(name, command string, args []string, env map[string]string) error {
		newServer := copilot.ManagedMCPServerConfig{
			Name:     name,
			Type:     copilot.MCPTypeStdio,
			Command:  command,
			Args:     args,
			Env:      env,
			Enabled:  true,
			AutoStart: true,
		}
		cfg.MCP.Servers = append(cfg.MCP.Servers, newServer)
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.UpdateMCPServerFn = func(name string, enabled bool) error {
		found := false
		for i, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				cfg.MCP.Servers[i].Enabled = enabled
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.DeleteMCPServerFn = func(name string) error {
		found := false
		newServers := make([]copilot.ManagedMCPServerConfig, 0, len(cfg.MCP.Servers))
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				continue
			}
			newServers = append(newServers, srv)
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		cfg.MCP.Servers = newServers
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.StartMCPServerFn = func(name string) error {
		// MCP server start/stop requires runtime management
		// For now, we just validate the server exists
		found := false
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		// TODO: Implement actual start via MCP manager when available
		return nil
	}
	adapter.StopMCPServerFn = func(name string) error {
		// MCP server start/stop requires runtime management
		found := false
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		// TODO: Implement actual stop via MCP manager when available
		return nil
	}

	// ── Database Status ──
	adapter.GetDatabaseStatusFn = func() webui.DatabaseStatusInfo {
		dbCfg := cfg.Database.Effective()
		status := webui.DatabaseStatusInfo{
			Name:         string(dbCfg.Backend),
			Healthy:      true, // Assume healthy if we got here
			Latency:      1,    // Placeholder, actual value requires runtime check
			Version:      "1.0",
			MaxOpenConns: 25, // Default value
		}

		// Try to get actual database connection and stats
		// For SQLite, check if the database file exists
		if dbCfg.Backend == "sqlite" {
			if info, err := os.Stat(dbCfg.SQLite.Path); err == nil {
				status.Version = "3.x"
				status.OpenConns = 1
				_ = info // File exists, database is healthy
			} else {
				status.Healthy = false
				status.Error = err.Error()
			}
		}

		return status
	}

	return adapter
}
