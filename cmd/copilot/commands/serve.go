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

	"github.com/jholhewres/goclaw/pkg/goclaw/channels/whatsapp"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/jholhewres/goclaw/pkg/goclaw/plugins"
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
	cfg, err := resolveConfig(cmd)
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
	copilot.ResolveAPIKey(cfg, logger)

	// ── Create assistant ──
	assistant := copilot.New(cfg, logger)

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

	// Load plugins (Discord, Telegram, etc.).
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
func resolveConfig(cmd *cobra.Command) (*copilot.Config, error) {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")

	// Try explicit path first.
	if configPath != "" {
		cfg, err := copilot.LoadConfigFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		return cfg, nil
	}

	// Auto-discover config file.
	if found := copilot.FindConfigFile(); found != "" {
		cfg, err := copilot.LoadConfigFromFile(found)
		if err != nil {
			return nil, fmt.Errorf("loading config from %s: %w", found, err)
		}
		slog.Info("config loaded", "path", found)
		return cfg, nil
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
		return nil, fmt.Errorf("configuration required before starting")
	}

	// Run the interactive setup wizard.
	if err := runInteractiveSetup(); err != nil {
		return nil, fmt.Errorf("setup: %w", err)
	}

	// Try loading the freshly created config.
	if found := copilot.FindConfigFile(); found != "" {
		cfg, err := copilot.LoadConfigFromFile(found)
		if err != nil {
			return nil, fmt.Errorf("loading config from %s: %w", found, err)
		}
		slog.Info("config loaded after setup", "path", found)
		return cfg, nil
	}

	return nil, fmt.Errorf("setup completado mas config.yaml não encontrado")
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
