package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jholhewres/devclaw/pkg/devclaw/mcp"
	"github.com/spf13/cobra"
)

// newMCPCmd creates the `devclaw mcp` command group for MCP server operations.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol server",
		Long:  `Run DevClaw as an MCP (Model Context Protocol) server for IDE integration.`,
	}

	cmd.AddCommand(newMCPServeCmd())
	return cmd
}

// newMCPServeCmd creates the `devclaw mcp serve` command.
func newMCPServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server over stdio",
		Long: `Start the MCP server using stdio transport (JSON-RPC 2.0 over stdin/stdout).
This is the standard way for IDEs to connect to DevClaw.

Add to your IDE configuration:

  Cursor/VSCode (.cursor/mcp.json or .vscode/mcp.json):
  {
    "mcpServers": {
      "devclaw": {
        "command": "devclaw",
        "args": ["mcp", "serve"]
      }
    }
  }

  Claude Code (.mcp.json):
  {
    "mcpServers": {
      "devclaw": {
        "command": "devclaw",
        "args": ["mcp", "serve"]
      }
    }
  }`,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

			server := mcp.New(logger)

			// TODO: register DevClaw tools into MCP server from assistant

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			logger.Info("starting MCP server on stdio")
			if err := server.ServeStdio(ctx); err != nil {
				return fmt.Errorf("MCP server error: %w", err)
			}

			return nil
		},
	}

	return cmd
}
