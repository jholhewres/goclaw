package commands

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels/discord"
	slackchan "github.com/jholhewres/goclaw/pkg/goclaw/channels/slack"
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/telegram"
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/whatsapp"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/jholhewres/goclaw/pkg/goclaw/gateway"
	"github.com/jholhewres/goclaw/pkg/goclaw/plugins"
	"github.com/jholhewres/goclaw/pkg/goclaw/webui"
	"github.com/spf13/cobra"
)

// newServeCmd creates the `copilot serve` command that starts the daemon.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the daemon with messaging channels",
		Long: `Start GoClaw Copilot as a daemon service, connecting to enabled
channels (WhatsApp, Discord, Telegram) and processing messages.

Examples:
  copilot serve
  copilot serve --channel whatsapp
  copilot serve --config ./config.yaml`,
		RunE: runServe,
	}

	cmd.Flags().StringSlice("channel", nil, "channels to enable (whatsapp, discord, telegram)")
	return cmd
}

func runServe(cmd *cobra.Command, _ []string) error {
	// ── Load config ──
	cfg, configPath, err := resolveConfig(cmd)
	if err != nil {
		return err
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
	if shouldEnable("whatsapp", channelFilter, true) {
		wa := whatsapp.New(cfg.Channels.WhatsApp, logger)
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

	// ── Start ──
	if err := assistant.Start(ctx); err != nil {
		return fmt.Errorf("failed to start: %w", err)
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

	// ── Start Web UI if enabled ──
	var webServer *webui.Server
	if cfg.WebUI.Enabled {
		adapter := buildWebUIAdapter(assistant, cfg)
		webServer = webui.New(cfg.WebUI, adapter, logger)
		if err := webServer.Start(ctx); err != nil {
			logger.Error("failed to start web UI", "error", err)
		} else {
			logger.Info("web UI running", "address", cfg.WebUI.Address)
		}
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
	logger.Info("GoClaw Copilot running. Press Ctrl+C to stop.",
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

	// No config file — offer interactive setup before connecting.
	fmt.Println()
	fmt.Println("No configuration file found.")
	fmt.Println("GoClaw requires a config.yaml before connecting to WhatsApp.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Run interactive setup now? (y/n) [y]: ")
	answer := strings.TrimSpace(readInput(reader))

	if answer != "" && strings.ToLower(answer) != "y" {
		fmt.Println()
		fmt.Println("Run 'copilot setup' or 'copilot config init' to create the configuration.")
		return nil, "", fmt.Errorf("configuration required before starting")
	}

	// Run the interactive setup wizard.
	if err := runInteractiveSetup(); err != nil {
		return nil, "", fmt.Errorf("setup: %w", err)
	}

	// Try loading the freshly created config.
	if found := copilot.FindConfigFile(); found != "" {
		cfg, err := copilot.LoadConfigFromFile(found)
		if err != nil {
			return nil, "", fmt.Errorf("loading config from %s: %w", found, err)
		}
		slog.Info("config loaded after setup", "path", found)
		return cfg, found, nil
	}

	return nil, "", fmt.Errorf("setup completado mas config.yaml não encontrado")
}

// readInput reads a line from stdin (used by resolveConfig prompt).
func readInput(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return line
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

// buildWebUIAdapter creates the adapter that bridges the Assistant to the WebUI.
func buildWebUIAdapter(assistant *copilot.Assistant, cfg *copilot.Config) *webui.AssistantAdapter {
	return &webui.AssistantAdapter{
		GetConfigMapFn: func() map[string]any {
			return map[string]any{
				"name":     cfg.Name,
				"trigger":  cfg.Trigger,
				"model":    cfg.Model,
				"language": cfg.Language,
				"timezone": cfg.Timezone,
				"provider": cfg.API.Provider,
				"base_url": cfg.API.BaseURL,
			}
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
					Enabled:     true,
				}
			}
			return result
		},
		SendChatMessageFn: func(sessionID, content string) (string, error) {
			// TODO: implement proper chat via webui channel
			return "Web UI chat coming soon", nil
		},
	}
}
