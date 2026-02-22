// Package copilot – git_tools.go implements native Git tools that provide
// structured JSON output for the agent, enabling better decision-making
// without parsing raw text. Uses os/exec to call git directly.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ---------- Git Data Types ----------

type gitStatusResult struct {
	Branch    string   `json:"branch"`
	Ahead     int      `json:"ahead"`
	Behind    int      `json:"behind"`
	Staged    []string `json:"staged"`
	Unstaged  []string `json:"unstaged"`
	Untracked []string `json:"untracked"`
	Conflicts []string `json:"conflicts"`
}

type gitLogEntry struct {
	Hash         string `json:"hash"`
	Author       string `json:"author"`
	Date         string `json:"date"`
	Message      string `json:"message"`
	FilesChanged int    `json:"files_changed"`
}

type gitBranchInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Remote  string `json:"remote,omitempty"`
}

type gitStashEntry struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// ---------- Git Helpers ----------

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("git %s: %s", args[0], result)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return result, nil
}

func runGitDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("git %s: %s", args[0], result)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return result, nil
}

func parseGitStatus() (*gitStatusResult, error) {
	result := &gitStatusResult{}

	// Branch name
	branch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	result.Branch = branch

	// Ahead/behind
	abOut, _ := runGit("rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if parts := strings.Fields(abOut); len(parts) == 2 {
		result.Ahead, _ = strconv.Atoi(parts[0])
		result.Behind, _ = strconv.Atoi(parts[1])
	}

	// Porcelain status
	status, err := runGit("status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(status, "\n") {
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		file := strings.TrimSpace(line[3:])

		// Conflicts
		if (x == 'U' || y == 'U') || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			result.Conflicts = append(result.Conflicts, file)
			continue
		}

		// Staged
		if x != ' ' && x != '?' {
			result.Staged = append(result.Staged, file)
		}

		// Unstaged
		if y != ' ' && y != '?' {
			result.Unstaged = append(result.Unstaged, file)
		}

		// Untracked
		if x == '?' && y == '?' {
			result.Untracked = append(result.Untracked, file)
		}
	}

	return result, nil
}

// ---------- Tool Registration ----------

// RegisterGitTools registers native Git tools in the executor.
func RegisterGitTools(executor *ToolExecutor) {
	// git_status
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_status",
			Description: "Get structured Git repository status: branch, ahead/behind, staged/unstaged/untracked files, and conflicts.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		st, err := parseGitStatus()
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(st, "", "  ")
		return string(data), nil
	})

	// git_diff
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_diff",
			Description: "Show Git diff with context. Supports --staged, specific paths, and --stat mode.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"staged": map[string]any{"type": "boolean", "description": "Show staged changes (--cached)"},
					"path":   map[string]any{"type": "string", "description": "Limit diff to specific file or directory"},
					"stat":   map[string]any{"type": "boolean", "description": "Show diffstat summary instead of full diff"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		gitArgs := []string{"diff"}
		if staged, _ := args["staged"].(bool); staged {
			gitArgs = append(gitArgs, "--cached")
		}
		if stat, _ := args["stat"].(bool); stat {
			gitArgs = append(gitArgs, "--stat")
		}
		if path, ok := args["path"].(string); ok && path != "" {
			gitArgs = append(gitArgs, "--", path)
		}

		out, err := runGit(gitArgs...)
		if err != nil {
			return nil, err
		}
		if out == "" {
			return "No changes.", nil
		}

		// Truncate very long diffs
		const maxLen = 8000
		if len(out) > maxLen {
			out = out[:maxLen] + "\n\n... (truncated, use path filter for specific files)"
		}
		return out, nil
	})

	// git_log
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_log",
			Description: "Get structured Git log: hash, author, date, message, files changed. Returns JSON array.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"count":  map[string]any{"type": "integer", "description": "Number of commits (default: 10, max: 50)"},
					"author": map[string]any{"type": "string", "description": "Filter by author name/email"},
					"path":   map[string]any{"type": "string", "description": "Filter by file path"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		count := 10
		if v, ok := args["count"].(float64); ok {
			count = int(v)
			if count > 50 {
				count = 50
			}
		}

		gitArgs := []string{"log", fmt.Sprintf("-%d", count), "--format=%H|%an|%aI|%s", "--shortstat"}
		if author, ok := args["author"].(string); ok && author != "" {
			gitArgs = append(gitArgs, "--author="+author)
		}
		if path, ok := args["path"].(string); ok && path != "" {
			gitArgs = append(gitArgs, "--", path)
		}

		out, err := runGit(gitArgs...)
		if err != nil {
			return nil, err
		}

		var entries []gitLogEntry
		lines := strings.Split(out, "\n")
		for i := 0; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 4)
			if len(parts) != 4 {
				continue
			}

			entry := gitLogEntry{
				Hash:    parts[0][:8],
				Author:  parts[1],
				Date:    parts[2],
				Message: parts[3],
			}

			// Parse shortstat on next non-empty line
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next == "" {
					i++
					continue
				}
				if strings.Contains(next, "file") && strings.Contains(next, "changed") {
					fields := strings.Fields(next)
					if len(fields) > 0 {
						entry.FilesChanged, _ = strconv.Atoi(fields[0])
					}
					i++
				}
				break
			}

			entries = append(entries, entry)
		}

		data, _ := json.MarshalIndent(entries, "", "  ")
		return string(data), nil
	})

	// git_commit
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_commit",
			Description: "Create a Git commit with the staged changes. Requires a commit message.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string", "description": "Commit message"},
					"add_all": map[string]any{"type": "boolean", "description": "Stage all modified files before committing (-a)"},
				},
				"required": []string{"message"},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		message, _ := args["message"].(string)
		if message == "" {
			return nil, fmt.Errorf("commit message is required")
		}

		if addAll, _ := args["add_all"].(bool); addAll {
			if _, err := runGit("add", "-A"); err != nil {
				return nil, fmt.Errorf("staging files: %w", err)
			}
		}

		out, err := runGit("commit", "-m", message)
		if err != nil {
			return nil, err
		}
		return out, nil
	})

	// git_branch
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_branch",
			Description: "List, create, switch, or delete Git branches. Returns structured JSON for list.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"list", "create", "switch", "delete"}, "description": "Action to perform"},
					"name":   map[string]any{"type": "string", "description": "Branch name (for create/switch/delete)"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)
		name, _ := args["name"].(string)

		switch action {
		case "list":
			out, err := runGit("branch", "-a", "--format=%(HEAD) %(refname:short) %(upstream:short)")
			if err != nil {
				return nil, err
			}
			var branches []gitBranchInfo
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				current := strings.HasPrefix(line, "* ")
				line = strings.TrimPrefix(line, "* ")
				fields := strings.Fields(line)
				b := gitBranchInfo{Name: fields[0], Current: current}
				if len(fields) > 1 {
					b.Remote = fields[1]
				}
				branches = append(branches, b)
			}
			data, _ := json.MarshalIndent(branches, "", "  ")
			return string(data), nil

		case "create":
			if name == "" {
				return nil, fmt.Errorf("branch name required")
			}
			out, err := runGit("checkout", "-b", name)
			if err != nil {
				return nil, err
			}
			return out, nil

		case "switch":
			if name == "" {
				return nil, fmt.Errorf("branch name required")
			}
			out, err := runGit("checkout", name)
			if err != nil {
				return nil, err
			}
			return out, nil

		case "delete":
			if name == "" {
				return nil, fmt.Errorf("branch name required")
			}
			out, err := runGit("branch", "-d", name)
			if err != nil {
				return nil, err
			}
			return out, nil

		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	})

	// git_stash
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_stash",
			Description: "Manage Git stash: save, pop, list, or drop stashed changes.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":  map[string]any{"type": "string", "enum": []string{"save", "pop", "list", "drop"}, "description": "Stash action"},
					"message": map[string]any{"type": "string", "description": "Message for save action"},
					"index":   map[string]any{"type": "integer", "description": "Stash index for pop/drop"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)

		switch action {
		case "save":
			gitArgs := []string{"stash", "push"}
			if msg, ok := args["message"].(string); ok && msg != "" {
				gitArgs = append(gitArgs, "-m", msg)
			}
			return runGit(gitArgs...)

		case "pop":
			gitArgs := []string{"stash", "pop"}
			if idx, ok := args["index"].(float64); ok {
				gitArgs = append(gitArgs, fmt.Sprintf("stash@{%d}", int(idx)))
			}
			return runGit(gitArgs...)

		case "list":
			out, err := runGit("stash", "list", "--format=%gd|%gs")
			if err != nil {
				return nil, err
			}
			if out == "" {
				return "No stashes.", nil
			}
			var stashes []gitStashEntry
			for _, line := range strings.Split(out, "\n") {
				parts := strings.SplitN(line, "|", 2)
				if len(parts) != 2 {
					continue
				}
				idx := 0
				fmt.Sscanf(parts[0], "stash@{%d}", &idx)
				stashes = append(stashes, gitStashEntry{Index: idx, Message: parts[1]})
			}
			data, _ := json.MarshalIndent(stashes, "", "  ")
			return string(data), nil

		case "drop":
			gitArgs := []string{"stash", "drop"}
			if idx, ok := args["index"].(float64); ok {
				gitArgs = append(gitArgs, fmt.Sprintf("stash@{%d}", int(idx)))
			}
			return runGit(gitArgs...)

		default:
			return nil, fmt.Errorf("unknown stash action: %s", action)
		}
	})

	// git_blame
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "git_blame",
			Description: "Show Git blame for a file or line range, showing who last modified each line.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":       map[string]any{"type": "string", "description": "File path to blame"},
					"start_line": map[string]any{"type": "integer", "description": "Start line (optional, for range)"},
					"end_line":   map[string]any{"type": "integer", "description": "End line (optional, for range)"},
				},
				"required": []string{"file"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		file, _ := args["file"].(string)
		if file == "" {
			return nil, fmt.Errorf("file path is required")
		}

		gitArgs := []string{"blame", "--porcelain"}
		if start, ok := args["start_line"].(float64); ok {
			end := start
			if e, ok2 := args["end_line"].(float64); ok2 {
				end = e
			}
			gitArgs = append(gitArgs, fmt.Sprintf("-L%d,%d", int(start), int(end)))
		}
		gitArgs = append(gitArgs, file)

		out, err := runGit(gitArgs...)
		if err != nil {
			return nil, err
		}

		// Truncate if too long
		const maxLen = 6000
		if len(out) > maxLen {
			out = out[:maxLen] + "\n\n... (truncated, use line range for specific sections)"
		}
		return out, nil
	})
}
