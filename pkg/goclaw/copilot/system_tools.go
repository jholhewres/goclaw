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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
)

// RegisterSystemTools registers all built-in system tools in the executor.
// These are core tools available regardless of which skills are loaded.
func RegisterSystemTools(executor *ToolExecutor, sandboxRunner *sandbox.Runner, memStore *memory.FileStore, sched *scheduler.Scheduler, dataDir string) {
	registerWebSearchTool(executor)
	registerWebFetchTool(executor)
	registerFileTools(executor, dataDir)

	if sandboxRunner != nil {
		registerExecTool(executor, sandboxRunner)
	}

	if memStore != nil {
		registerMemoryTools(executor, memStore)
	}

	if sched != nil {
		registerCronTools(executor, sched)
	}
}

// ---------- Web Fetch Tool ----------

func registerWebSearchTool(executor *ToolExecutor) {
	client := &http.Client{Timeout: 15 * time.Second}

	executor.Register(
		MakeToolDefinition("web_search", "Search the web using DuckDuckGo. Returns search results with titles, URLs, and snippets.", map[string]any{
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

			// Use DuckDuckGo HTML search.
			searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s",
				strings.ReplaceAll(query, " ", "+"))

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

			// Extract results from DuckDuckGo HTML.
			results := extractDDGResults(html)
			if len(results) == 0 {
				return fmt.Sprintf("No results found for: %s", query), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))
			for i, r := range results {
				if i >= 8 {
					break
				}
				sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.title, r.url, r.snippet))
			}
			return sb.String(), nil
		},
	)
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

func registerWebFetchTool(executor *ToolExecutor) {
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

// ---------- Exec Tool ----------

func registerExecTool(executor *ToolExecutor, runner *sandbox.Runner) {
	executor.Register(
		MakeToolDefinition("exec", "Execute a shell command in a sandboxed environment. Use for running scripts, system commands, etc.", map[string]any{
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

// ---------- File Tools ----------

func registerFileTools(executor *ToolExecutor, dataDir string) {
	if dataDir == "" {
		dataDir = "./data"
	}

	// read_file
	executor.Register(
		MakeToolDefinition("read_file", "Read the contents of a file from the workspace data directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path within the data directory",
				},
			},
			"required": []string{"path"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			relPath, _ := args["path"].(string)
			if relPath == "" {
				return nil, fmt.Errorf("path is required")
			}

			// Prevent path traversal.
			cleaned := filepath.Clean(relPath)
			if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
				return nil, fmt.Errorf("invalid path: must be relative and within data directory")
			}

			fullPath := filepath.Join(dataDir, cleaned)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("reading file: %w", err)
			}

			text := string(content)
			if len(text) > 20000 {
				text = text[:20000] + "\n... [truncated]"
			}
			return text, nil
		},
	)

	// write_file
	executor.Register(
		MakeToolDefinition("write_file", "Write content to a file in the workspace data directory. Creates directories if needed.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path within the data directory",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file",
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "If true, append to file instead of overwriting. Default: false",
				},
			},
			"required": []string{"path", "content"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			relPath, _ := args["path"].(string)
			content, _ := args["content"].(string)
			appendMode, _ := args["append"].(bool)

			if relPath == "" {
				return nil, fmt.Errorf("path is required")
			}

			cleaned := filepath.Clean(relPath)
			if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
				return nil, fmt.Errorf("invalid path: must be relative and within data directory")
			}

			fullPath := filepath.Join(dataDir, cleaned)

			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return nil, fmt.Errorf("creating directory: %w", err)
			}

			var err error
			if appendMode {
				f, openErr := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if openErr != nil {
					return nil, fmt.Errorf("opening file: %w", openErr)
				}
				_, err = f.WriteString(content)
				f.Close()
			} else {
				err = os.WriteFile(fullPath, []byte(content), 0o644)
			}
			if err != nil {
				return nil, fmt.Errorf("writing file: %w", err)
			}

			return fmt.Sprintf("Written %d bytes to %s", len(content), relPath), nil
		},
	)

	// list_files
	executor.Register(
		MakeToolDefinition("list_files", "List files in a directory within the workspace data directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative directory path (default: root of data dir)",
				},
			},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			relPath, _ := args["path"].(string)
			if relPath == "" {
				relPath = "."
			}

			cleaned := filepath.Clean(relPath)
			if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
				return nil, fmt.Errorf("invalid path")
			}

			fullPath := filepath.Join(dataDir, cleaned)
			entries, err := os.ReadDir(fullPath)
			if err != nil {
				return nil, fmt.Errorf("reading directory: %w", err)
			}

			var items []map[string]any
			for _, e := range entries {
				info, _ := e.Info()
				item := map[string]any{
					"name":  e.Name(),
					"is_dir": e.IsDir(),
				}
				if info != nil {
					item["size"] = info.Size()
					item["modified"] = info.ModTime().Format(time.RFC3339)
				}
				items = append(items, item)
			}

			result, _ := json.MarshalIndent(items, "", "  ")
			return string(result), nil
		},
	)
}

// ---------- Memory Tools ----------

func registerMemoryTools(executor *ToolExecutor, store *memory.FileStore) {
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
			return fmt.Sprintf("Saved to memory: %s", content), nil
		},
	)

	// memory_search
	executor.Register(
		MakeToolDefinition("memory_search", "Search long-term memory for relevant facts, preferences, or past events.", map[string]any{
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
		func(_ context.Context, args map[string]any) (any, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return nil, fmt.Errorf("query is required")
			}

			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}

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
		func(_ context.Context, args map[string]any) (any, error) {
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

			return fmt.Sprintf("Job '%s' scheduled: %s (%s)", id, schedule, jobType), nil
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
