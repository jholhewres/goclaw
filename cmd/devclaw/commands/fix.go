package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/spf13/cobra"
)

// newFixCmd creates the `devclaw fix` command that analyzes the last error
// or a specific file and suggests fixes.
func newFixCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix [file]",
		Short: "Analyze and fix errors",
		Long: `Analyze the last error or a specific file and suggest fixes.

Examples:
  devclaw fix                  # analyze last error from shell history
  devclaw fix main.go          # analyze errors in specific file
  npm run build 2>&1 | devclaw fix  # pipe build errors`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveConfig(cmd)
			if err != nil {
				return err
			}

			assistant, cleanup, err := quickAssistant(cfg, cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			var prompt string
			if len(args) > 0 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("reading file: %w", err)
				}
				prompt = fmt.Sprintf("Analyze this file for errors, bugs, or issues and suggest fixes:\n\nFile: %s\n```\n%s\n```", args[0], string(content))
			} else {
				prompt = "Analyze the last error I encountered and suggest a fix. Check recent shell history or logs for context."
			}

			response := executeChat(assistant, prompt)
			fmt.Println(response)
			return nil
		},
	}
	return cmd
}

// quickAssistant creates a minimal assistant for quick commands.
func quickAssistant(cfg *copilot.Config, cmd *cobra.Command) (*copilot.Assistant, func(), error) {
	logger := quietLogger()
	copilot.AuditSecrets(cfg, logger)
	vault := copilot.ResolveAPIKey(cfg, logger)

	if cfg.API.APIKey == "" || copilot.IsEnvReference(cfg.API.APIKey) {
		return nil, nil, fmt.Errorf("no API key configured. Run: devclaw config vault-set")
	}

	assistant := copilot.New(cfg, logger)
	if vault != nil {
		assistant.SetVault(vault)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = cmd.Root().Context()
	}
	if ctx == nil {
		var cancel func()
		ctx, cancel = signalContext()
		cleanup := func() {
			cancel()
			assistant.Stop()
		}
		if err := assistant.Start(ctx); err != nil {
			cancel()
			return nil, nil, err
		}
		return assistant, cleanup, nil
	}

	if err := assistant.Start(ctx); err != nil {
		return nil, nil, err
	}
	return assistant, func() { assistant.Stop() }, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func signalContext() (context.Context, func()) {
	return context.WithCancel(context.Background())
}
