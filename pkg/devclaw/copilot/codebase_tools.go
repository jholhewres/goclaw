// Package copilot – codebase_tools.go implements codebase analysis tools:
// file tree indexing, code search (ripgrep), symbol extraction, and
// Cursor rules generator.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---------- Data Types ----------

type fileTreeNode struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"` // file, dir
	Size     int64           `json:"size,omitempty"`
	Children []*fileTreeNode `json:"children,omitempty"`
}

type codeSearchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// ---------- Tool Registration ----------

// RegisterCodebaseTools registers codebase analysis tools in the executor.
func RegisterCodebaseTools(executor *ToolExecutor) {
	// codebase_index
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "codebase_index",
			Description: "Generate a tree view of the project directory structure. Respects .gitignore and skips common noise (node_modules, .git, vendor, etc.).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "Root path (default: current directory)"},
					"max_depth": map[string]any{"type": "integer", "description": "Maximum depth (default: 4)"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		root := "."
		if v, ok := args["path"].(string); ok && v != "" {
			root = v
		}
		maxDepth := 4
		if v, ok := args["max_depth"].(float64); ok {
			maxDepth = int(v)
		}

		tree := buildTree(root, 0, maxDepth)
		if tree == nil {
			return "Path not found or empty.", nil
		}

		data, _ := json.MarshalIndent(tree, "", "  ")
		return string(data), nil
	})

	// code_search
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "code_search",
			Description: "Search for patterns in code using ripgrep. Returns file, line number, and matching content. Falls back to grep if rg is not installed.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":   map[string]any{"type": "string", "description": "Search pattern (regex supported)"},
					"path":      map[string]any{"type": "string", "description": "Directory or file to search (default: current directory)"},
					"file_type": map[string]any{"type": "string", "description": "Filter by file type (e.g. 'go', 'ts', 'py')"},
					"max_count": map[string]any{"type": "integer", "description": "Maximum number of matches (default: 50)"},
				},
				"required": []string{"pattern"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		pattern, _ := args["pattern"].(string)
		searchPath := "."
		if v, ok := args["path"].(string); ok && v != "" {
			searchPath = v
		}
		maxCount := 50
		if v, ok := args["max_count"].(float64); ok {
			maxCount = int(v)
		}

		rgArgs := []string{"-n", "--no-heading", "--color=never", "-m", fmt.Sprintf("%d", maxCount)}
		if ft, ok := args["file_type"].(string); ok && ft != "" {
			rgArgs = append(rgArgs, "-t", ft)
		}
		rgArgs = append(rgArgs, pattern, searchPath)

		cmd := exec.Command("rg", rgArgs...)
		out, err := cmd.CombinedOutput()
		result := strings.TrimSpace(string(out))

		if err != nil && result == "" {
			// Try grep as fallback
			grepArgs := []string{"-rn", "--include=*", pattern, searchPath}
			cmd = exec.Command("grep", grepArgs...)
			out, err = cmd.CombinedOutput()
			result = strings.TrimSpace(string(out))
			if err != nil && result == "" {
				return "No matches found.", nil
			}
		}

		if result == "" {
			return "No matches found.", nil
		}

		// Truncate
		const maxLen = 6000
		if len(result) > maxLen {
			result = result[:maxLen] + "\n\n... (truncated, narrow your search)"
		}

		return result, nil
	})

	// code_symbols
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "code_symbols",
			Description: "Extract symbol definitions from a file: functions, classes, types, interfaces. Uses pattern matching for common languages.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file": map[string]any{"type": "string", "description": "File to extract symbols from"},
				},
				"required": []string{"file"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		file, _ := args["file"].(string)
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading file: %w", err)
		}

		ext := filepath.Ext(file)
		symbols := extractSymbols(string(content), ext)

		if len(symbols) == 0 {
			return "No symbols found.", nil
		}

		data, _ := json.MarshalIndent(symbols, "", "  ")
		return string(data), nil
	})

	// cursor_rules_generate
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "cursor_rules_generate",
			Description: "Analyze the codebase and generate a .cursor/rules/*.mdc file with project conventions, patterns, and coding standards.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string", "description": "Rule name (e.g. 'go-conventions', 'react-patterns')"},
					"scope": map[string]any{"type": "string", "description": "Glob pattern for which files this rule applies to (e.g. '**/*.go', 'web/src/**/*.tsx')"},
					"focus": map[string]any{"type": "string", "description": "What to focus on: 'patterns', 'conventions', 'architecture', or 'all'"},
				},
				"required": []string{"name"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		name, _ := args["name"].(string)
		scope, _ := args["scope"].(string)
		focus, _ := args["focus"].(string)
		if focus == "" {
			focus = "all"
		}

		// Gather project info
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", strings.ReplaceAll(name, "-", " ")))

		if scope != "" {
			sb.WriteString(fmt.Sprintf("Scope: `%s`\n\n", scope))
		}

		// Detect stack
		sb.WriteString("## Stack Detection\n\n")

		stackFiles := map[string]string{
			"go.mod":         "Go project",
			"package.json":   "Node.js/JavaScript project",
			"requirements.txt": "Python project",
			"Cargo.toml":     "Rust project",
			"pom.xml":        "Java (Maven) project",
			"build.gradle":   "Java (Gradle) project",
			"Gemfile":        "Ruby project",
			"composer.json":  "PHP project",
		}

		for file, desc := range stackFiles {
			if _, err := os.Stat(file); err == nil {
				sb.WriteString(fmt.Sprintf("- Detected: %s (`%s`)\n", desc, file))
			}
		}

		sb.WriteString(fmt.Sprintf("\n## Focus: %s\n\n", focus))
		sb.WriteString("(This is a generated template. Customize based on your project's actual conventions.)\n\n")
		sb.WriteString("## Conventions\n\n")
		sb.WriteString("- TODO: Add project-specific conventions\n\n")
		sb.WriteString("## Patterns\n\n")
		sb.WriteString("- TODO: Add common patterns used in this codebase\n\n")
		sb.WriteString("## Do NOT\n\n")
		sb.WriteString("- TODO: Add anti-patterns and things to avoid\n")

		// Write to .cursor/rules/
		rulesDir := filepath.Join(".cursor", "rules")
		if err := os.MkdirAll(rulesDir, 0755); err != nil {
			return nil, fmt.Errorf("creating rules directory: %w", err)
		}

		filename := filepath.Join(rulesDir, name+".mdc")
		if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
			return nil, fmt.Errorf("writing rules file: %w", err)
		}

		return fmt.Sprintf("Generated cursor rules file: %s\n\nCustomize the TODO sections with your project's actual conventions.", filename), nil
	})
}

// buildTree creates a file tree node recursively.
func buildTree(path string, depth, maxDepth int) *fileTreeNode {
	if depth >= maxDepth {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	node := &fileTreeNode{
		Name: filepath.Base(path),
		Type: "file",
	}

	if info.IsDir() {
		node.Type = "dir"
		entries, err := os.ReadDir(path)
		if err != nil {
			return node
		}

		for _, e := range entries {
			name := e.Name()
			if shouldSkip(name) {
				continue
			}
			child := buildTree(filepath.Join(path, name), depth+1, maxDepth)
			if child != nil {
				node.Children = append(node.Children, child)
			}
		}
	} else {
		node.Size = info.Size()
	}

	return node
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"__pycache__": true, ".next": true, ".nuxt": true,
	"dist": true, "build": true, ".venv": true,
	"target": true, ".idea": true, ".vscode": true,
}

func shouldSkip(name string) bool {
	if skipDirs[name] {
		return true
	}
	if strings.HasPrefix(name, ".") && name != ".cursor" && name != ".github" {
		return true
	}
	return false
}

type symbolInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // function, type, interface, class, method, const
	Line int    `json:"line"`
}

func extractSymbols(content, ext string) []symbolInfo {
	var symbols []symbolInfo
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch ext {
		case ".go":
			if strings.HasPrefix(trimmed, "func ") {
				name := extractGoFuncName(trimmed)
				if name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "function", Line: i + 1})
				}
			} else if strings.HasPrefix(trimmed, "type ") {
				name := extractGoTypeName(trimmed)
				if name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "type", Line: i + 1})
				}
			}

		case ".ts", ".tsx", ".js", ".jsx":
			if strings.Contains(trimmed, "function ") || strings.Contains(trimmed, "const ") || strings.Contains(trimmed, "export ") {
				if name := extractJSSymbol(trimmed); name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "function", Line: i + 1})
				}
			} else if strings.Contains(trimmed, "interface ") || strings.Contains(trimmed, "type ") {
				if name := extractJSType(trimmed); name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "type", Line: i + 1})
				}
			} else if strings.Contains(trimmed, "class ") {
				if name := extractClassName(trimmed); name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "class", Line: i + 1})
				}
			}

		case ".py":
			if strings.HasPrefix(trimmed, "def ") {
				name := extractPyFunc(trimmed)
				if name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "function", Line: i + 1})
				}
			} else if strings.HasPrefix(trimmed, "class ") {
				name := extractClassName(trimmed)
				if name != "" {
					symbols = append(symbols, symbolInfo{Name: name, Kind: "class", Line: i + 1})
				}
			}
		}
	}

	return symbols
}

func extractGoFuncName(line string) string {
	line = strings.TrimPrefix(line, "func ")
	if paren := strings.Index(line, "("); paren > 0 {
		name := line[:paren]
		// Method receiver
		if strings.HasPrefix(name, "(") {
			if close := strings.Index(name, ")"); close > 0 {
				rest := strings.TrimSpace(name[close+1:])
				if space := strings.IndexAny(rest, "( "); space > 0 {
					return rest[:space]
				}
				return rest
			}
		}
		return name
	}
	return ""
}

func extractGoTypeName(line string) string {
	line = strings.TrimPrefix(line, "type ")
	if space := strings.IndexAny(line, " {"); space > 0 {
		return line[:space]
	}
	return ""
}

func extractJSSymbol(line string) string {
	line = strings.TrimPrefix(line, "export ")
	line = strings.TrimPrefix(line, "default ")
	line = strings.TrimPrefix(line, "async ")

	if strings.HasPrefix(line, "function ") {
		line = strings.TrimPrefix(line, "function ")
		if paren := strings.Index(line, "("); paren > 0 {
			return line[:paren]
		}
	} else if strings.HasPrefix(line, "const ") {
		line = strings.TrimPrefix(line, "const ")
		if eq := strings.IndexAny(line, " =:"); eq > 0 {
			return line[:eq]
		}
	}
	return ""
}

func extractJSType(line string) string {
	line = strings.TrimPrefix(line, "export ")
	if strings.HasPrefix(line, "interface ") {
		line = strings.TrimPrefix(line, "interface ")
	} else if strings.HasPrefix(line, "type ") {
		line = strings.TrimPrefix(line, "type ")
	}
	if space := strings.IndexAny(line, " {<="); space > 0 {
		return line[:space]
	}
	return ""
}

func extractClassName(line string) string {
	idx := strings.Index(line, "class ")
	if idx < 0 {
		return ""
	}
	rest := line[idx+6:]
	if space := strings.IndexAny(rest, " ({:"); space > 0 {
		return rest[:space]
	}
	return strings.TrimSpace(rest)
}

func extractPyFunc(line string) string {
	line = strings.TrimPrefix(line, "def ")
	if paren := strings.Index(line, "("); paren > 0 {
		return line[:paren]
	}
	return ""
}
