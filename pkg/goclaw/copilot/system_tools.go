// Package copilot – system_tools.go registers built-in tools that are always
// available to the agent, independent of skills. These tools provide core
// capabilities like shell execution, file I/O, memory operations, and scheduling.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
)

// RegisterSystemTools registers all built-in system tools in the executor.
// These are core tools available regardless of which skills are loaded.
// If ssrfGuard is non-nil, web_fetch will validate URLs against SSRF rules.
func RegisterSystemTools(executor *ToolExecutor, sandboxRunner *sandbox.Runner, memStore *memory.FileStore, sqliteStore *memory.SQLiteStore, memCfg MemoryConfig, sched *scheduler.Scheduler, dataDir string, ssrfGuard *security.SSRFGuard, vault *Vault, webSearchCfg WebSearchConfig) {
	registerWebSearchTool(executor, webSearchCfg)
	registerWebFetchTool(executor, ssrfGuard)
	registerFileTools(executor, dataDir)
	registerBashTool(executor)

	if sandboxRunner != nil {
		registerExecTool(executor, sandboxRunner)
	}

	if memStore != nil {
		registerMemoryTools(executor, memStore, sqliteStore, memCfg)
	}

	if sched != nil {
		registerCronTools(executor, sched)
	}

	if vault != nil && vault.IsUnlocked() {
		registerVaultTools(executor, vault)
	}
}

// ---------- Web Fetch Tool ----------

func registerWebSearchTool(executor *ToolExecutor, cfg WebSearchConfig) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Resolve Brave API key: config > env var.
	braveKey := cfg.BraveAPIKey
	if braveKey == "" {
		braveKey = os.Getenv("BRAVE_API_KEY")
	}

	// Auto-select provider: if brave key is available and configured, use Brave.
	provider := cfg.Provider
	if provider == "brave" && braveKey == "" {
		provider = "duckduckgo" // fallback if no API key
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 8
	}

	description := "Search the web and return results with titles, URLs, and snippets."
	if provider == "brave" {
		description = "Search the web using Brave Search. Returns results with titles, URLs, and descriptions."
	}

	executor.Register(
		MakeToolDefinition("web_search", description, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query",
				},
			},
			"required": []string{"query"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return nil, fmt.Errorf("query is required")
			}

			// Use Brave Search if configured and key is available.
			if provider == "brave" && braveKey != "" {
				return searchBrave(ctx, client, query, braveKey, maxResults)
			}

			// Fallback to DuckDuckGo HTML search.
			return searchDDG(ctx, client, query, maxResults)
		},
	)
}

// searchBrave queries the Brave Search API and returns formatted results.
func searchBrave(ctx context.Context, client *http.Client, query, apiKey string, maxResults int) (any, error) {
	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), maxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("brave search returned %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200*1024))

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing brave results: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return fmt.Sprintf("No results found for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))
	for i, r := range result.Web.Results {
		if i >= maxResults {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description))
	}
	return sb.String(), nil
}

// searchDDG queries DuckDuckGo HTML and returns formatted results.
func searchDDG(ctx context.Context, client *http.Client, query string, maxResults int) (any, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "GoClaw/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	html := string(body)

	results := extractDDGResults(html)
	if len(results) == 0 {
		return fmt.Sprintf("No results found for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))
	for i, r := range results {
		if i >= maxResults {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.title, r.url, r.snippet))
	}
	return sb.String(), nil
}

// ddgResult holds a single DuckDuckGo search result.
type ddgResult struct {
	title   string
	url     string
	snippet string
}

// extractDDGResults parses DuckDuckGo HTML for search results.
func extractDDGResults(html string) []ddgResult {
	var results []ddgResult

	// Find result blocks: <a class="result__a" href="...">Title</a>
	parts := strings.Split(html, "result__a")
	for _, part := range parts[1:] { // Skip the first split (before first match).
		var r ddgResult

		// Extract URL from href="..."
		hrefIdx := strings.Index(part, "href=\"")
		if hrefIdx >= 0 {
			urlStart := hrefIdx + 6
			urlEnd := strings.Index(part[urlStart:], "\"")
			if urlEnd > 0 {
				r.url = part[urlStart : urlStart+urlEnd]
				// DuckDuckGo wraps URLs in a redirect; try to extract the actual URL.
				if udIdx := strings.Index(r.url, "uddg="); udIdx >= 0 {
					r.url = r.url[udIdx+5:]
					if ampIdx := strings.Index(r.url, "&"); ampIdx >= 0 {
						r.url = r.url[:ampIdx]
					}
				}
			}
		}

		// Extract title from between > and </a>
		gtIdx := strings.Index(part, ">")
		if gtIdx >= 0 {
			closeIdx := strings.Index(part[gtIdx:], "</a>")
			if closeIdx > 0 {
				r.title = stripHTMLTags(part[gtIdx+1 : gtIdx+closeIdx])
			}
		}

		// Extract snippet from result__snippet
		snipIdx := strings.Index(part, "result__snippet")
		if snipIdx >= 0 {
			snipStart := strings.Index(part[snipIdx:], ">")
			if snipStart >= 0 {
				snipEnd := strings.Index(part[snipIdx+snipStart:], "</")
				if snipEnd > 0 {
					r.snippet = stripHTMLTags(part[snipIdx+snipStart+1 : snipIdx+snipStart+snipEnd])
				}
			}
		}

		if r.title != "" && r.url != "" {
			results = append(results, r)
		}
	}

	return results
}

// stripHTMLTags removes HTML tags from a string.
func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

func registerWebFetchTool(executor *ToolExecutor, ssrfGuard *security.SSRFGuard) {
	client := &http.Client{Timeout: 20 * time.Second}

	executor.Register(
		MakeToolDefinition("web_fetch", "Fetch content from a URL and return the text. Use for reading web pages, APIs, etc.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
			},
			"required": []string{"url"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return nil, fmt.Errorf("url is required")
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}

			if ssrfGuard != nil {
				if err := ssrfGuard.IsAllowed(url); err != nil {
					return nil, err
				}
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, fmt.Errorf("creating request: %w", err)
			}
			req.Header.Set("User-Agent", "GoClaw/1.0")
			req.Header.Set("Accept", "text/html,text/plain,application/json")

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetching URL: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
			content := string(body)
			if len(content) > 10000 {
				content = content[:10000] + "\n... [truncated]"
			}

			return fmt.Sprintf("Status: %d\nContent-Type: %s\n\n%s",
				resp.StatusCode, resp.Header.Get("Content-Type"), content), nil
		},
	)
}

// ---------- Exec Tool (sandboxed) ----------

func registerExecTool(executor *ToolExecutor, runner *sandbox.Runner) {
	executor.Register(
		MakeToolDefinition("exec", "Execute a shell command in a sandboxed environment. For full access, use the 'bash' tool instead.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
			},
			"required": []string{"command"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return nil, fmt.Errorf("command is required")
			}

			result, err := runner.RunShell(ctx, command, nil, "")
			if err != nil {
				return nil, fmt.Errorf("execution failed: %w", err)
			}

			output := result.Stdout
			if result.Stderr != "" {
				output += "\nSTDERR:\n" + result.Stderr
			}
			if result.ExitCode != 0 {
				output = fmt.Sprintf("Exit code: %d\n%s", result.ExitCode, output)
			}
			if result.Killed {
				output = fmt.Sprintf("Process killed: %s\n%s", result.KillReason, output)
			}

			return output, nil
		},
	)
}

// ---------- Bash Tool (full access, user environment) ----------

func registerBashTool(executor *ToolExecutor) {
	// Persistent shell state: tracks working directory between calls.
	shellState := &persistentShellState{
		cwd: "",
		env: map[string]string{},
	}

	// bash — full access command execution inheriting the user's environment.
	executor.Register(
		MakeToolDefinition("bash", "Execute a bash command with full system access. Inherits the user's complete environment (PATH, SSH keys, etc). Supports cd (persistent between calls), git, ssh, docker, package managers, builds, system administration, or any shell operation. The command runs directly on the host machine as the current user.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Bash command to execute. cd is tracked between calls.",
				},
				"working_dir": map[string]any{
					"type":        "string",
					"description": "Override working directory for this command",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default: 120, max: 600)",
				},
			},
			"required": []string{"command"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return nil, fmt.Errorf("command is required")
			}

			timeout := 120 * time.Second
			if t, ok := args["timeout_seconds"].(float64); ok && t > 0 {
				if t > 600 {
					t = 600
				}
				timeout = time.Duration(t) * time.Second
			}

			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Wrap in a login shell to inherit the user's full environment
			// (~/.bashrc, ~/.profile, SSH agent, etc).
			wrappedCmd := command

			// If we have a persistent cwd, prepend cd.
			wd := ""
			if w, ok := args["working_dir"].(string); ok && w != "" {
				wd = w
			} else if shellState.cwd != "" {
				wd = shellState.cwd
			}

			if wd != "" {
				wrappedCmd = fmt.Sprintf("cd %q && %s", wd, command)
			}

			// Append pwd capture to track cd.
			wrappedCmd += " ; __exit=$?; echo \"__GOCLAW_CWD=$(pwd)\"; exit $__exit"

			cmd := exec.CommandContext(cmdCtx, "bash", "-l", "-c", wrappedCmd)
			cmd.Env = os.Environ() // Inherit full user environment.

			// Add any extra env vars set via set_env.
			for k, v := range shellState.env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}

			out, err := cmd.CombinedOutput()
			output := string(out)

			// Extract and update persistent cwd.
			if idx := strings.LastIndex(output, "__GOCLAW_CWD="); idx >= 0 {
				cwdLine := output[idx+len("__GOCLAW_CWD="):]
				if nl := strings.Index(cwdLine, "\n"); nl >= 0 {
					shellState.cwd = strings.TrimSpace(cwdLine[:nl])
				} else {
					shellState.cwd = strings.TrimSpace(cwdLine)
				}
				// Remove the cwd marker from output.
				output = output[:idx]
			}

			output = strings.TrimRight(output, "\n ")

			// Truncate very long output.
			if len(output) > 50000 {
				output = output[:50000] + "\n... [truncated, output too long]"
			}

			if err != nil {
				if cmdCtx.Err() != nil {
					return fmt.Sprintf("Command timed out after %v.\n\nPartial output:\n%s", timeout, output), nil
				}
				return fmt.Sprintf("Exit code: non-zero\n%s", output), nil
			}

			if output == "" {
				output = "(no output)"
			}

			return output, nil
		},
	)

	// ssh — execute commands on remote machines via SSH.
	executor.Register(
		MakeToolDefinition("ssh", "Execute a command on a remote machine via SSH. Uses the user's SSH keys and config (~/.ssh/config). Supports any host configured in SSH config or direct user@host.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host": map[string]any{
					"type":        "string",
					"description": "SSH host (e.g. 'myserver', 'user@192.168.1.10', 'deploy@prod.example.com')",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute on the remote machine",
				},
				"port": map[string]any{
					"type":        "integer",
					"description": "SSH port (default: 22, or as configured in ~/.ssh/config)",
				},
				"identity_file": map[string]any{
					"type":        "string",
					"description": "Path to SSH private key (default: uses ssh-agent or ~/.ssh/id_*)",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default: 60)",
				},
			},
			"required": []string{"host", "command"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			host, _ := args["host"].(string)
			command, _ := args["command"].(string)
			if host == "" || command == "" {
				return nil, fmt.Errorf("host and command are required")
			}

			timeout := 60 * time.Second
			if t, ok := args["timeout_seconds"].(float64); ok && t > 0 {
				timeout = time.Duration(t) * time.Second
			}

			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			sshArgs := []string{
				"-o", "StrictHostKeyChecking=accept-new",
				"-o", "ConnectTimeout=10",
				"-o", "BatchMode=yes",
			}

			if port, ok := args["port"].(float64); ok && port > 0 {
				sshArgs = append(sshArgs, "-p", fmt.Sprintf("%d", int(port)))
			}

			if keyFile, ok := args["identity_file"].(string); ok && keyFile != "" {
				sshArgs = append(sshArgs, "-i", resolvePath(keyFile))
			}

			sshArgs = append(sshArgs, host, command)

			cmd := exec.CommandContext(cmdCtx, "ssh", sshArgs...)
			cmd.Env = os.Environ() // Inherit SSH agent, keys, etc.

			out, err := cmd.CombinedOutput()
			output := strings.TrimRight(string(out), "\n ")

			if len(output) > 50000 {
				output = output[:50000] + "\n... [truncated]"
			}

			if err != nil {
				if cmdCtx.Err() != nil {
					return fmt.Sprintf("SSH timed out after %v.\n\nPartial output:\n%s", timeout, output), nil
				}
				return fmt.Sprintf("SSH error: %v\n%s", err, output), nil
			}

			if output == "" {
				output = "(no output)"
			}

			return output, nil
		},
	)

	// scp — copy files to/from remote machines.
	executor.Register(
		MakeToolDefinition("scp", "Copy files between local machine and remote hosts via SCP/SFTP. Uses the user's SSH keys and config.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Source path. For remote: 'user@host:/path'. For local: '/local/path'",
				},
				"destination": map[string]any{
					"type":        "string",
					"description": "Destination path. For remote: 'user@host:/path'. For local: '/local/path'",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "Copy directories recursively. Default: false",
				},
			},
			"required": []string{"source", "destination"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			source, _ := args["source"].(string)
			dest, _ := args["destination"].(string)
			recursive, _ := args["recursive"].(bool)

			if source == "" || dest == "" {
				return nil, fmt.Errorf("source and destination are required")
			}

			cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			scpArgs := []string{
				"-o", "StrictHostKeyChecking=accept-new",
				"-o", "ConnectTimeout=10",
			}
			if recursive {
				scpArgs = append(scpArgs, "-r")
			}
			scpArgs = append(scpArgs, source, dest)

			cmd := exec.CommandContext(cmdCtx, "scp", scpArgs...)
			cmd.Env = os.Environ()

			out, err := cmd.CombinedOutput()
			output := strings.TrimRight(string(out), "\n ")

			if err != nil {
				return fmt.Sprintf("SCP error: %v\n%s", err, output), nil
			}

			return fmt.Sprintf("Copied: %s -> %s\n%s", source, dest, output), nil
		},
	)

	// set_env — set environment variables for subsequent bash/ssh calls.
	executor.Register(
		MakeToolDefinition("set_env", "Set an environment variable that persists across subsequent bash calls in this session.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Environment variable name",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Environment variable value",
				},
			},
			"required": []string{"name", "value"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			value, _ := args["value"].(string)
			if name == "" {
				return nil, fmt.Errorf("name is required")
			}

			shellState.env[name] = value
			return fmt.Sprintf("Set %s=%s", name, value), nil
		},
	)
}

// persistentShellState tracks state between bash tool calls.
type persistentShellState struct {
	cwd string            // Current working directory.
	env map[string]string  // Extra environment variables.
}

// ---------- File Tools (full filesystem access) ----------

func registerFileTools(executor *ToolExecutor, _ string) {
	// read_file — reads any file on the machine.
	executor.Register(
		MakeToolDefinition("read_file", "Read the contents of any file on the machine. Supports absolute and relative paths. Returns up to 100KB of text content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (absolute or relative)",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Line number to start reading from (1-based, default: 1)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of lines to return (default: all)",
				},
			},
			"required": []string{"path"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			filePath, _ := args["path"].(string)
			if filePath == "" {
				return nil, fmt.Errorf("path is required")
			}

			filePath = resolvePath(filePath)

			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading file: %w", err)
			}

			text := string(content)

			// Apply offset/limit if specified.
			offset := 0
			if o, ok := args["offset"].(float64); ok && o > 1 {
				offset = int(o) - 1 // Convert 1-based to 0-based.
			}

			limit := 0
			if l, ok := args["limit"].(float64); ok && l > 0 {
				limit = int(l)
			}

			if offset > 0 || limit > 0 {
				lines := strings.Split(text, "\n")
				if offset >= len(lines) {
					return "(offset beyond end of file)", nil
				}
				lines = lines[offset:]
				if limit > 0 && limit < len(lines) {
					lines = lines[:limit]
				}
				text = strings.Join(lines, "\n")
			}

			// Truncate for safety.
			if len(text) > 100000 {
				text = text[:100000] + "\n... [truncated at 100KB]"
			}

			return text, nil
		},
	)

	// write_file — writes to any file on the machine.
	executor.Register(
		MakeToolDefinition("write_file", "Write content to any file on the machine. Creates parent directories if needed. Supports absolute and relative paths.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (absolute or relative)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file",
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "If true, append to file instead of overwriting. Default: false",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "File permissions in octal (e.g. '0755' for executable). Default: '0644'",
				},
			},
			"required": []string{"path", "content"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			filePath, _ := args["path"].(string)
			content, _ := args["content"].(string)
			appendMode, _ := args["append"].(bool)

			if filePath == "" {
				return nil, fmt.Errorf("path is required")
			}

			filePath = resolvePath(filePath)

			// Parse file mode.
			fileMode := os.FileMode(0o644)
			if m, ok := args["mode"].(string); ok && m != "" {
				var parsed uint64
				_, err := fmt.Sscanf(m, "%o", &parsed)
				if err == nil {
					fileMode = os.FileMode(parsed)
				}
			}

			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
				return nil, fmt.Errorf("creating directory: %w", err)
			}

			var err error
			if appendMode {
				f, openErr := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
				if openErr != nil {
					return nil, fmt.Errorf("opening file: %w", openErr)
				}
				_, err = f.WriteString(content)
				f.Close()
			} else {
				err = os.WriteFile(filePath, []byte(content), fileMode)
			}
			if err != nil {
				return nil, fmt.Errorf("writing file: %w", err)
			}

			return fmt.Sprintf("Written %d bytes to %s", len(content), filePath), nil
		},
	)

	// edit_file — search-and-replace edit on any file.
	executor.Register(
		MakeToolDefinition("edit_file", "Edit a file by replacing a specific text occurrence. Finds old_text in the file and replaces it with new_text. Use for precise code modifications.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (absolute or relative)",
				},
				"old_text": map[string]any{
					"type":        "string",
					"description": "Exact text to find and replace (must be unique in the file)",
				},
				"new_text": map[string]any{
					"type":        "string",
					"description": "Text to replace old_text with",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "If true, replace all occurrences. Default: false (replace first only)",
				},
			},
			"required": []string{"path", "old_text", "new_text"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			filePath, _ := args["path"].(string)
			oldText, _ := args["old_text"].(string)
			newText, _ := args["new_text"].(string)
			replaceAll, _ := args["replace_all"].(bool)

			if filePath == "" || oldText == "" {
				return nil, fmt.Errorf("path and old_text are required")
			}

			filePath = resolvePath(filePath)

			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading file: %w", err)
			}

			text := string(content)
			if !strings.Contains(text, oldText) {
				return nil, fmt.Errorf("old_text not found in %s", filePath)
			}

			count := strings.Count(text, oldText)
			if !replaceAll && count > 1 {
				return nil, fmt.Errorf("old_text found %d times in file — provide more context to make it unique, or set replace_all=true", count)
			}

			var newContent string
			if replaceAll {
				newContent = strings.ReplaceAll(text, oldText, newText)
			} else {
				newContent = strings.Replace(text, oldText, newText, 1)
			}

			// Preserve original file permissions.
			info, _ := os.Stat(filePath)
			mode := os.FileMode(0o644)
			if info != nil {
				mode = info.Mode()
			}

			if err := os.WriteFile(filePath, []byte(newContent), mode); err != nil {
				return nil, fmt.Errorf("writing file: %w", err)
			}

			replaced := 1
			if replaceAll {
				replaced = count
			}
			return fmt.Sprintf("Replaced %d occurrence(s) in %s", replaced, filePath), nil
		},
	)

	// list_files — list any directory.
	executor.Register(
		MakeToolDefinition("list_files", "List files and directories at any path on the machine. Returns names, sizes, permissions, and modification times.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path (absolute or relative). Default: current directory",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "If true, list recursively (max 500 entries). Default: false",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to filter files (e.g. '*.go', '*.py')",
				},
			},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			dirPath, _ := args["path"].(string)
			if dirPath == "" {
				dirPath = "."
			}
			recursive, _ := args["recursive"].(bool)
			pattern, _ := args["pattern"].(string)

			dirPath = resolvePath(dirPath)

			if !recursive {
				entries, err := os.ReadDir(dirPath)
				if err != nil {
					return nil, fmt.Errorf("reading directory: %w", err)
				}

				var sb strings.Builder
				for _, e := range entries {
					info, _ := e.Info()
					prefix := "  "
					if e.IsDir() {
						prefix = "d "
					}
					size := int64(0)
					mod := ""
					if info != nil {
						size = info.Size()
						mod = info.ModTime().Format("2006-01-02 15:04")
					}
					name := e.Name()
					if pattern != "" {
						matched, _ := filepath.Match(pattern, name)
						if !matched {
							continue
						}
					}
					sb.WriteString(fmt.Sprintf("%s %8d  %s  %s\n", prefix, size, mod, name))
				}
				return sb.String(), nil
			}

			// Recursive listing.
			var sb strings.Builder
			count := 0
			_ = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil // Skip errors.
				}
				if count >= 500 {
					return filepath.SkipAll
				}

				// Skip hidden directories like .git.
				if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != dirPath {
					return filepath.SkipDir
				}

				rel, _ := filepath.Rel(dirPath, path)
				if rel == "." {
					return nil
				}

				if pattern != "" {
					matched, _ := filepath.Match(pattern, info.Name())
					if !matched && !info.IsDir() {
						return nil
					}
				}

				prefix := "  "
				if info.IsDir() {
					prefix = "d "
				}
				sb.WriteString(fmt.Sprintf("%s %8d  %s\n", prefix, info.Size(), rel))
				count++
				return nil
			})

			if count >= 500 {
				sb.WriteString("\n... [truncated at 500 entries]")
			}
			return sb.String(), nil
		},
	)

	// search_files — grep-like search across files.
	executor.Register(
		MakeToolDefinition("search_files", "Search for text patterns in files. Similar to grep. Searches recursively in the given directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Text or regex pattern to search for",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search in (default: current directory)",
				},
				"file_pattern": map[string]any{
					"type":        "string",
					"description": "Glob to filter files (e.g. '*.go', '*.py')",
				},
				"case_insensitive": map[string]any{
					"type":        "boolean",
					"description": "Case insensitive search. Default: false",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matching lines to return (default: 50)",
				},
			},
			"required": []string{"pattern"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return nil, fmt.Errorf("pattern is required")
			}

			searchDir, _ := args["path"].(string)
			if searchDir == "" {
				searchDir = "."
			}
			searchDir = resolvePath(searchDir)

			filePattern, _ := args["file_pattern"].(string)
			caseInsensitive, _ := args["case_insensitive"].(bool)

			maxResults := 50
			if m, ok := args["max_results"].(float64); ok && m > 0 {
				maxResults = int(m)
			}

			// Use ripgrep if available, otherwise grep.
			rgArgs := []string{"--no-heading", "--line-number", "--color=never"}
			if caseInsensitive {
				rgArgs = append(rgArgs, "-i")
			}
			if filePattern != "" {
				rgArgs = append(rgArgs, "-g", filePattern)
			}
			rgArgs = append(rgArgs, fmt.Sprintf("-m%d", maxResults))
			rgArgs = append(rgArgs, pattern, searchDir)

			cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			// Try ripgrep first.
			cmd := execCommandContext(cmdCtx, "rg", rgArgs...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				// Fallback to grep.
				grepArgs := []string{"-rn", "--color=never"}
				if caseInsensitive {
					grepArgs = append(grepArgs, "-i")
				}
				if filePattern != "" {
					grepArgs = append(grepArgs, "--include="+filePattern)
				}
				grepArgs = append(grepArgs, fmt.Sprintf("-m%d", maxResults))
				grepArgs = append(grepArgs, pattern, searchDir)

				cmd = execCommandContext(cmdCtx, "grep", grepArgs...)
				out, err = cmd.CombinedOutput()
				if err != nil && len(out) == 0 {
					return fmt.Sprintf("No matches found for %q in %s", pattern, searchDir), nil
				}
			}

			output := string(out)
			if len(output) > 50000 {
				output = output[:50000] + "\n... [truncated]"
			}
			if output == "" {
				return fmt.Sprintf("No matches found for %q in %s", pattern, searchDir), nil
			}
			return output, nil
		},
	)

	// glob_files — find files by glob pattern.
	executor.Register(
		MakeToolDefinition("glob_files", "Find files matching a glob pattern. Searches recursively from the given directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern (e.g. '**/*.go', 'src/**/*.ts', '*.py')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Base directory (default: current directory)",
				},
			},
			"required": []string{"pattern"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return nil, fmt.Errorf("pattern is required")
			}

			baseDir, _ := args["path"].(string)
			if baseDir == "" {
				baseDir = "."
			}
			baseDir = resolvePath(baseDir)

			// If pattern is relative, combine with base dir.
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join(baseDir, pattern)
			}

			matches, err := filepath.Glob(pattern)
			if err != nil {
				// filepath.Glob doesn't support **. Walk manually.
				matches = globRecursive(baseDir, args["pattern"].(string))
			}

			if len(matches) == 0 {
				return "No files found.", nil
			}

			if len(matches) > 200 {
				matches = matches[:200]
			}

			return strings.Join(matches, "\n"), nil
		},
	)
}

// resolvePath resolves a file path, expanding ~ and making relative paths absolute.
func resolvePath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// globRecursive implements a simple recursive glob supporting ** patterns.
func globRecursive(baseDir, pattern string) []string {
	var matches []string

	// Extract the file-level pattern (last component after any **/).
	fileGlob := pattern
	if idx := strings.LastIndex(pattern, "/"); idx >= 0 {
		fileGlob = pattern[idx+1:]
	}

	count := 0
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || count >= 200 {
			if count >= 200 {
				return filepath.SkipAll
			}
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories.
			if strings.HasPrefix(info.Name(), ".") && path != baseDir {
				return filepath.SkipDir
			}
			// Skip common non-useful dirs.
			name := info.Name()
			if name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		matched, _ := filepath.Match(fileGlob, info.Name())
		if matched {
			matches = append(matches, path)
			count++
		}
		return nil
	})

	return matches
}

// execCommandContext wraps exec.CommandContext for use in tools.
func execCommandContext(ctx context.Context, name string, args ...string) *osExecCmd {
	return &osExecCmd{cmd: exec.CommandContext(ctx, name, args...)}
}

// osExecCmd wraps exec.Cmd.
type osExecCmd struct {
	cmd *exec.Cmd
	Dir string
}

func (c *osExecCmd) CombinedOutput() ([]byte, error) {
	if c.Dir != "" {
		c.cmd.Dir = c.Dir
	}
	return c.cmd.CombinedOutput()
}

// ---------- Memory Tools ----------

func registerMemoryTools(executor *ToolExecutor, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig) {
	// memory_save
	executor.Register(
		MakeToolDefinition("memory_save", "Save an important fact, preference, or piece of information to long-term memory. Use this to remember things about the user or important context.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The fact or information to remember",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "Category: 'fact', 'preference', 'event', or 'summary'",
					"enum":        []string{"fact", "preference", "event", "summary"},
				},
			},
			"required": []string{"content"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			content, _ := args["content"].(string)
			if content == "" {
				return nil, fmt.Errorf("content is required")
			}
			category, _ := args["category"].(string)
			if category == "" {
				category = "fact"
			}

			err := store.Save(memory.Entry{
				Content:   content,
				Source:    "agent",
				Category:  category,
				Timestamp: time.Now(),
			})
			if err != nil {
				return nil, err
			}

			// Re-index the MEMORY.md file if SQLite memory is available.
			if sqliteStore != nil && cfg.Index.Auto {
				memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
				chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
				if chunkCfg.MaxTokens <= 0 {
					chunkCfg.MaxTokens = 500
				}
				go func() {
					_ = sqliteStore.IndexMemoryDir(context.Background(), memDir, chunkCfg)
				}()
			}

			return fmt.Sprintf("Saved to memory: %s", content), nil
		},
	)

	// memory_search — uses hybrid search (vector + BM25) when available,
	// falls back to substring matching.
	searchDesc := "Search long-term memory for relevant facts, preferences, or past events."
	if sqliteStore != nil {
		searchDesc += " Uses semantic search (vector + keyword hybrid) for best results."
	}
	executor.Register(
		MakeToolDefinition("memory_search", searchDesc, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query to find relevant memories",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 10)",
				},
			},
			"required": []string{"query"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return nil, fmt.Errorf("query is required")
			}

			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}

			// Try hybrid search first.
			if sqliteStore != nil {
				results, err := sqliteStore.HybridSearch(
					ctx, query, limit, cfg.Search.MinScore,
					cfg.Search.HybridWeightVector, cfg.Search.HybridWeightBM25,
				)
				if err == nil && len(results) > 0 {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("Found %d memories (semantic search):\n\n", len(results)))
					for _, r := range results {
						text := r.Text
						if len(text) > 500 {
							text = text[:500] + "..."
						}
						sb.WriteString(fmt.Sprintf("- [%s] (score: %.2f) %s\n", r.FileID, r.Score, text))
					}
					return sb.String(), nil
				}
			}

			// Fallback to substring search.
			entries, err := store.Search(query, limit)
			if err != nil {
				return nil, err
			}

			if len(entries) == 0 {
				return "No memories found matching the query.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(entries)))
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, e.Content))
			}
			return sb.String(), nil
		},
	)

	// memory_list
	executor.Register(
		MakeToolDefinition("memory_list", "List recent memories from long-term storage.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of entries to return (default: 20)",
				},
			},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			limit := 20
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}

			entries, err := store.GetRecent(limit)
			if err != nil {
				return nil, err
			}

			if len(entries) == 0 {
				return "No memories stored yet.", nil
			}

			var sb strings.Builder
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("- [%s] [%s] %s\n",
					e.Timestamp.Format("2006-01-02"),
					e.Category,
					e.Content))
			}
			return sb.String(), nil
		},
	)

	// memory_index — manually trigger re-indexing of memory files.
	if sqliteStore != nil {
		executor.Register(
			MakeToolDefinition("memory_index", "Manually re-index all memory files for semantic search. Run this after adding or modifying memory files.", map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
			func(ctx context.Context, _ map[string]any) (any, error) {
				memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
				chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
				if chunkCfg.MaxTokens <= 0 {
					chunkCfg.MaxTokens = 500
				}

				if err := sqliteStore.IndexMemoryDir(ctx, memDir, chunkCfg); err != nil {
					return nil, fmt.Errorf("indexing failed: %w", err)
				}

				return fmt.Sprintf("Memory index updated: %d files, %d chunks.",
					sqliteStore.FileCount(), sqliteStore.ChunkCount()), nil
			},
		)
	}
}

// ---------- Cron / Scheduler Tools ----------

func registerCronTools(executor *ToolExecutor, sched *scheduler.Scheduler) {
	// cron_add
	executor.Register(
		MakeToolDefinition("cron_add", "Schedule a recurring or one-time task. The agent will execute the command at the specified time.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for this job",
				},
				"schedule": map[string]any{
					"type":        "string",
					"description": "Cron expression (e.g. '0 9 * * *' for daily at 9AM), interval (e.g. '5m'), or time (e.g. '14:30')",
				},
				"type": map[string]any{
					"type":        "string",
					"description": "Schedule type: 'cron' (recurring), 'every' (interval), 'at' (one-shot)",
					"enum":        []string{"cron", "every", "at"},
				},
				"command": map[string]any{
					"type":        "string",
					"description": "The prompt/command to execute when the job fires",
				},
				"channel": map[string]any{
					"type":        "string",
					"description": "Target channel for the response (e.g. 'whatsapp')",
				},
				"chat_id": map[string]any{
					"type":        "string",
					"description": "Target chat/group ID for the response",
				},
			},
			"required": []string{"id", "schedule", "command"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			schedule, _ := args["schedule"].(string)
			jobType, _ := args["type"].(string)
			command, _ := args["command"].(string)
			channel, _ := args["channel"].(string)
			chatID, _ := args["chat_id"].(string)

			if id == "" || schedule == "" || command == "" {
				return nil, fmt.Errorf("id, schedule, and command are required")
			}
			if jobType == "" {
				jobType = "cron"
			}

			// Auto-fill channel/chatID from the context-propagated delivery target.
			// This is goroutine-safe: each agent run carries its own context
			// with the delivery target (channel + chatID) set separately from
			// the opaque session ID.
			if channel == "" || chatID == "" {
				dt := DeliveryTargetFromContext(ctx)
				if dt.Channel != "" && channel == "" {
					channel = dt.Channel
				}
				if dt.ChatID != "" && chatID == "" {
					chatID = dt.ChatID
				}
			}

			job := &scheduler.Job{
				ID:       id,
				Schedule: schedule,
				Type:     jobType,
				Command:  command,
				Channel:  channel,
				ChatID:   chatID,
				Enabled:  true,
			}

			if err := sched.Add(job); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Job '%s' scheduled: %s (%s) → %s:%s", id, schedule, jobType, channel, chatID), nil
		},
	)

	// cron_list
	executor.Register(
		MakeToolDefinition("cron_list", "List all scheduled jobs/tasks.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		func(_ context.Context, _ map[string]any) (any, error) {
			jobs := sched.List()
			if len(jobs) == 0 {
				return "No scheduled jobs.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Scheduled jobs (%d):\n\n", len(jobs)))
			for _, j := range jobs {
				status := "enabled"
				if !j.Enabled {
					status = "disabled"
				}
				sb.WriteString(fmt.Sprintf("- **%s** [%s] schedule=%s type=%s\n  Command: %s\n  Runs: %d",
					j.ID, status, j.Schedule, j.Type, j.Command, j.RunCount))
				if j.LastRunAt != nil {
					sb.WriteString(fmt.Sprintf("  Last run: %s", j.LastRunAt.Format("2006-01-02 15:04")))
				}
				if j.LastError != "" {
					sb.WriteString(fmt.Sprintf("  Last error: %s", j.LastError))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	)

	// cron_remove
	executor.Register(
		MakeToolDefinition("cron_remove", "Remove a scheduled job by its ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The job ID to remove",
				},
			},
			"required": []string{"id"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("id is required")
			}
			if err := sched.Remove(id); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Job '%s' removed.", id), nil
		},
	)
}

// ---------- Vault Tools ----------

func registerVaultTools(executor *ToolExecutor, vault *Vault) {
	// vault_save — store a secret in the encrypted vault.
	executor.Register(
		MakeToolDefinition("vault_save", "Store a secret (API key, token, password) in the encrypted vault. Secrets are encrypted with AES-256-GCM and persist across restarts. Use this whenever the user provides a credential or API key.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Secret name/key (e.g. 'weather_api_key', 'github_token')",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "The secret value to store",
				},
			},
			"required": []string{"name", "value"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			value, _ := args["value"].(string)
			if name == "" || value == "" {
				return nil, fmt.Errorf("name and value are required")
			}
			if err := vault.Set(name, value); err != nil {
				return nil, fmt.Errorf("failed to save to vault: %w", err)
			}
			return fmt.Sprintf("Secret '%s' saved to encrypted vault.", name), nil
		},
	)

	// vault_get — retrieve a secret from the encrypted vault.
	executor.Register(
		MakeToolDefinition("vault_get", "Retrieve a secret from the encrypted vault by name.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Secret name/key to retrieve",
				},
			},
			"required": []string{"name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("name is required")
			}
			val, err := vault.Get(name)
			if err != nil {
				return nil, fmt.Errorf("failed to read from vault: %w", err)
			}
			if val == "" {
				return fmt.Sprintf("Secret '%s' not found in vault.", name), nil
			}
			return val, nil
		},
	)

	// vault_list — list all secret names in the vault (without values).
	executor.Register(
		MakeToolDefinition("vault_list", "List all secret names stored in the encrypted vault. Returns only names, not values.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		func(_ context.Context, _ map[string]any) (any, error) {
			names := vault.List()
			if len(names) == 0 {
				return "Vault is empty.", nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Vault contains %d secrets:\n", len(names)))
			for _, name := range names {
				sb.WriteString(fmt.Sprintf("- %s\n", name))
			}
			return sb.String(), nil
		},
	)

	// vault_delete — remove a secret from the vault.
	executor.Register(
		MakeToolDefinition("vault_delete", "Remove a secret from the encrypted vault by name.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Secret name/key to delete",
				},
			},
			"required": []string{"name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("name is required")
			}
			if err := vault.Delete(name); err != nil {
				return nil, fmt.Errorf("failed to delete from vault: %w", err)
			}
			return fmt.Sprintf("Secret '%s' removed from vault.", name), nil
		},
	)
}
