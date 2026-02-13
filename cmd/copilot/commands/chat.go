package commands

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/spf13/cobra"
)

// newChatCmd creates the `copilot chat` command for interactive CLI conversations.
func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Chat with the assistant via terminal",
		Long: `Start a conversation with the assistant directly in the terminal.
Pass a message as argument for a single response, or run without arguments
for an interactive REPL session.

The CLI chat uses the same agent loop, tools, and skills as WhatsApp.

Examples:
  copilot chat "What time is it?"
  copilot chat                      # interactive mode`,
		Args: cobra.MaximumNArgs(1),
		RunE: runChat,
	}

	cmd.Flags().StringP("model", "m", "", "override the LLM model")
	return cmd
}

func runChat(cmd *cobra.Command, args []string) error {
	// ── Load config ──
	cfg, err := resolveConfig(cmd)
	if err != nil {
		return err
	}

	// Override model if flag is set.
	if model, _ := cmd.Flags().GetString("model"); model != "" {
		cfg.Model = model
	}

	// ── Configure logger (quiet for chat mode) ──
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	logLevel := slog.LevelWarn
	if verbose {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(handler)

	// ── Resolve secrets ──
	copilot.AuditSecrets(cfg, logger)
	copilot.ResolveAPIKey(cfg, logger)

	if cfg.API.APIKey == "" || copilot.IsEnvReference(cfg.API.APIKey) {
		return fmt.Errorf("no API key configured. Run: copilot config vault-set")
	}

	// ── Create and start assistant ──
	assistant := copilot.New(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := assistant.Start(ctx); err != nil {
		return fmt.Errorf("failed to start assistant: %w", err)
	}
	defer assistant.Stop()

	// ── Single message mode ──
	if len(args) > 0 {
		response := executeChat(assistant, args[0])
		fmt.Println(response)
		return nil
	}

	// ── Interactive REPL mode ──
	return runInteractiveChat(assistant, cfg)
}

// executeChat sends a message through the assistant and returns the response.
func executeChat(assistant *copilot.Assistant, message string) string {
	session := assistant.SessionStore().GetOrCreate("cli", "terminal")
	prompt := assistant.ComposePrompt(session, message)
	response := assistant.ExecuteAgent(context.Background(), prompt, session, message)
	session.AddMessage(message, response)
	return response
}

// runInteractiveChat runs an interactive REPL chat in the terminal.
func runInteractiveChat(assistant *copilot.Assistant, cfg *copilot.Config) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("  %s — CLI Chat\n", cfg.Name)
	fmt.Println("  Type your message and press Enter. Commands:")
	fmt.Println("    /quit    — exit")
	fmt.Println("    /clear   — reset conversation")
	fmt.Println("    /tools   — list available tools")
	fmt.Println("    /model   — show current model")
	fmt.Println()

	session := assistant.SessionStore().GetOrCreate("cli", "terminal")

	for {
		fmt.Print("you> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF (Ctrl+D).
			fmt.Println()
			return nil
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Handle commands.
		switch strings.ToLower(input) {
		case "/quit", "/exit", "/q":
			fmt.Println("Bye!")
			return nil

		case "/clear", "/reset":
			session = assistant.SessionStore().GetOrCreate("cli", fmt.Sprintf("terminal-%d", session.HistoryLen()))
			fmt.Println("  [conversation cleared]")
			fmt.Println()
			continue

		case "/tools":
			tools := assistant.ToolExecutor().ToolNames()
			fmt.Printf("  [%d tools available]\n", len(tools))
			for _, t := range tools {
				fmt.Printf("    - %s\n", t)
			}
			fmt.Println()
			continue

		case "/model":
			fmt.Printf("  Model: %s\n", cfg.Model)
			fmt.Printf("  API:   %s\n", cfg.API.BaseURL)
			fmt.Println()
			continue

		case "/help":
			fmt.Println("  /quit    — exit")
			fmt.Println("  /clear   — reset conversation")
			fmt.Println("  /tools   — list available tools")
			fmt.Println("  /model   — show current model")
			fmt.Println()
			continue
		}

		// Send to the agent.
		prompt := assistant.ComposePrompt(session, input)
		response := assistant.ExecuteAgent(context.Background(), prompt, session, input)
		session.AddMessage(input, response)

		fmt.Println()
		fmt.Printf("%s> %s\n", cfg.Name, response)
		fmt.Println()
	}
}
