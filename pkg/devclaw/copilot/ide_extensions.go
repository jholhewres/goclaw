// Package copilot – ide_extensions.go provides configuration generators
// for IDE extensions: VSCode, JetBrains, and Neovim. These generate
// the necessary config files for connecting to DevClaw's MCP server.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IDEConfig holds configuration for IDE extension generation.
type IDEConfig struct {
	MCPPort   int    `yaml:"mcp_port" json:"mcp_port"`
	MCPHost   string `yaml:"mcp_host" json:"mcp_host"`
	Transport string `yaml:"transport" json:"transport"` // stdio, sse
}

// DefaultIDEConfig returns sensible defaults.
func DefaultIDEConfig() IDEConfig {
	return IDEConfig{
		MCPPort:   8091,
		MCPHost:   "localhost",
		Transport: "stdio",
	}
}

// RegisterIDETools registers IDE extension configuration tools.
func RegisterIDETools(executor *ToolExecutor) {
	// ide_configure
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "ide_configure",
			Description: "Generate IDE extension configuration for connecting to DevClaw. Supports VSCode, JetBrains (IntelliJ/GoLand/WebStorm), and Neovim.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ide":       map[string]any{"type": "string", "enum": []string{"vscode", "jetbrains", "neovim", "cursor"}, "description": "Target IDE"},
					"transport": map[string]any{"type": "string", "enum": []string{"stdio", "sse"}, "description": "MCP transport (default: stdio)"},
					"install":   map[string]any{"type": "boolean", "description": "Write config files to the appropriate location"},
				},
				"required": []string{"ide"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		ide, _ := args["ide"].(string)
		transport, _ := args["transport"].(string)
		if transport == "" {
				transport = "stdio"
		}
		install, _ := args["install"].(bool)

		config := generateIDEConfig(ide, transport)
		if config == "" {
			return nil, fmt.Errorf("unsupported IDE: %s", ide)
		}

		if install {
			path := writeIDEConfig(ide, config)
			if path != "" {
				return fmt.Sprintf("Configuration written to: %s\n\n%s", path, config), nil
			}
		}

		return config, nil
	})
}

func generateIDEConfig(ide, transport string) string {
	switch ide {
	case "vscode", "cursor":
		return generateVSCodeConfig(transport)
	case "jetbrains":
		return generateJetBrainsConfig(transport)
	case "neovim":
		return generateNeovimConfig(transport)
	default:
		return ""
	}
}

func generateVSCodeConfig(transport string) string {
	if transport == "stdio" {
		config := map[string]any{
			"mcpServers": map[string]any{
				"devclaw": map[string]any{
					"command": "devclaw",
					"args":    []string{"mcp", "serve"},
				},
			},
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		return fmt.Sprintf("// .vscode/mcp.json (or .cursor/mcp.json)\n%s", string(data))
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"devclaw": map[string]any{
				"url": "http://localhost:8091/sse",
			},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	return fmt.Sprintf("// .vscode/mcp.json (or .cursor/mcp.json)\n%s", string(data))
}

func generateJetBrainsConfig(transport string) string {
	var sb strings.Builder
	sb.WriteString("<!-- JetBrains IDE MCP Configuration -->\n")
	sb.WriteString("<!-- Place in: .idea/mcp.xml -->\n\n")
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString("<project version=\"4\">\n")
	sb.WriteString("  <component name=\"McpServers\">\n")

	if transport == "stdio" {
		sb.WriteString("    <server name=\"devclaw\" type=\"stdio\">\n")
		sb.WriteString("      <command>devclaw</command>\n")
		sb.WriteString("      <args>mcp serve</args>\n")
		sb.WriteString("    </server>\n")
	} else {
		sb.WriteString("    <server name=\"devclaw\" type=\"sse\">\n")
		sb.WriteString("      <url>http://localhost:8091/sse</url>\n")
		sb.WriteString("    </server>\n")
	}

	sb.WriteString("  </component>\n")
	sb.WriteString("</project>")
	return sb.String()
}

func generateNeovimConfig(transport string) string {
	var sb strings.Builder
	sb.WriteString("-- Neovim MCP Configuration (using nvim-mcp or similar plugin)\n")
	sb.WriteString("-- Add to your init.lua or nvim config:\n\n")

	if transport == "stdio" {
		sb.WriteString(`require("mcp").setup({
  servers = {
    devclaw = {
      command = "devclaw",
      args = { "mcp", "serve" },
    },
  },
})`)
	} else {
		sb.WriteString(`require("mcp").setup({
  servers = {
    devclaw = {
      url = "http://localhost:8091/sse",
      transport = "sse",
    },
  },
})`)
	}

	return sb.String()
}

func writeIDEConfig(ide, config string) string {
	var targetPath string

	switch ide {
	case "vscode":
		targetPath = filepath.Join(".vscode", "mcp.json")
	case "cursor":
		targetPath = filepath.Join(".cursor", "mcp.json")
	case "jetbrains":
		targetPath = filepath.Join(".idea", "mcp.xml")
	case "neovim":
		home, _ := os.UserHomeDir()
		targetPath = filepath.Join(home, ".config", "nvim", "lua", "devclaw-mcp.lua")
	default:
		return ""
	}

	dir := filepath.Dir(targetPath)
	os.MkdirAll(dir, 0755)

	// Strip comment lines for actual file content
	lines := strings.Split(config, "\n")
	var cleanLines []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "//") && !strings.HasPrefix(strings.TrimSpace(line), "<!--") && !strings.HasPrefix(strings.TrimSpace(line), "--") {
			cleanLines = append(cleanLines, line)
		}
	}

	os.WriteFile(targetPath, []byte(strings.Join(cleanLines, "\n")), 0644)
	return targetPath
}
