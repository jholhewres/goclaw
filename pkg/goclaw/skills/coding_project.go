// Package skills – coding_project.go implements the "project-manager" built-in skill.
// Provides tools for registering, activating, scanning, and inspecting
// development projects. This is the foundation for all coding skills.
package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectProvider is the interface that the ProjectManager (in copilot pkg)
// must implement for coding skills to access project context.
// This avoids an import cycle between skills ↔ copilot.
type ProjectProvider interface {
	// Register adds or updates a project.
	Register(p *ProjectInfo) error
	// Remove removes a project by ID.
	Remove(id string) error
	// Get returns a project by ID.
	Get(id string) *ProjectInfo
	// List returns all registered projects.
	List() []*ProjectInfo
	// Activate sets the active project for a session.
	Activate(sessionKey, projectID string) error
	// ActiveProject returns the active project for a session.
	ActiveProject(sessionKey string) *ProjectInfo
	// ScanDirectory scans for projects in a directory.
	ScanDirectory(root string) ([]*ProjectInfo, error)
}

// ProjectInfo holds project metadata shared between copilot and skills packages.
type ProjectInfo struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	RootPath      string `yaml:"root_path"`
	Language      string `yaml:"language"`
	Framework     string `yaml:"framework"`
	GitRemote     string `yaml:"git_remote,omitempty"`
	BuildCmd      string `yaml:"build_cmd,omitempty"`
	TestCmd       string `yaml:"test_cmd,omitempty"`
	LintCmd       string `yaml:"lint_cmd,omitempty"`
	StartCmd      string `yaml:"start_cmd,omitempty"`
	DeployCmd     string `yaml:"deploy_cmd,omitempty"`
	DockerCompose string `yaml:"docker_compose,omitempty"`
}

// projectManagerSkill implements the project management skill.
type projectManagerSkill struct {
	provider ProjectProvider
}

// NewProjectManagerSkill creates the project-manager skill.
// provider will be set via Init or direct injection from the assistant.
func NewProjectManagerSkill(provider ProjectProvider) Skill {
	return &projectManagerSkill{provider: provider}
}

func (s *projectManagerSkill) Metadata() Metadata {
	return Metadata{
		Name:        "project-manager",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Manage development projects: register, activate, scan, inspect, and navigate project workspaces",
		Category:    "development",
		Tags:        []string{"project", "workspace", "development", "coding"},
	}
}

func (s *projectManagerSkill) Tools() []Tool {
	return []Tool{
		{
			Name:        "list",
			Description: "List all registered development projects with language, framework, and path.",
			Parameters:  []ToolParameter{},
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				projects := s.provider.List()
				if len(projects) == 0 {
					return "No projects registered. Use project-manager_scan or project-manager_register to add projects.", nil
				}
				var b strings.Builder
				for _, p := range projects {
					fw := ""
					if p.Framework != "" {
						fw = "/" + p.Framework
					}
					b.WriteString(fmt.Sprintf("• %s — %s (%s%s)\n", p.ID, p.RootPath, p.Language, fw))
				}
				return b.String(), nil
			},
		},
		{
			Name:        "register",
			Description: "Register a development project by path. Auto-detects language, framework, build/test/lint commands.",
			Parameters: []ToolParameter{
				{Name: "path", Type: "string", Description: "Absolute path to the project root directory", Required: true},
				{Name: "id", Type: "string", Description: "Optional project ID (slug). Auto-generated from directory name if empty."},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				path, _ := args["path"].(string)
				if path == "" {
					return nil, fmt.Errorf("path is required")
				}
				// Expand ~
				if strings.HasPrefix(path, "~/") {
					home, _ := os.UserHomeDir()
					path = filepath.Join(home, path[2:])
				}
				id, _ := args["id"].(string)
				if id == "" {
					id = sanitizeID(filepath.Base(path))
				}
				p := &ProjectInfo{ID: id, RootPath: path}
				if err := s.provider.Register(p); err != nil {
					return nil, err
				}
				// Re-fetch to get auto-detected fields.
				p = s.provider.Get(id)
				return fmt.Sprintf("✓ Project %q registered:\n  Path: %s\n  Language: %s\n  Framework: %s\n  Build: %s\n  Test: %s\n  Lint: %s",
					p.ID, p.RootPath, p.Language, p.Framework, p.BuildCmd, p.TestCmd, p.LintCmd), nil
			},
		},
		{
			Name:        "activate",
			Description: "Activate a project for the current session. All code/git/deploy tools will operate in this project's directory.",
			Parameters: []ToolParameter{
				{Name: "project_id", Type: "string", Description: "Project ID to activate", Required: true},
				{Name: "session_key", Type: "string", Description: "Session key (auto-provided by system)"},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				projectID, _ := args["project_id"].(string)
				sessionKey, _ := args["session_key"].(string)
				if projectID == "" {
					return nil, fmt.Errorf("project_id is required")
				}
				if err := s.provider.Activate(sessionKey, projectID); err != nil {
					return nil, err
				}
				p := s.provider.Get(projectID)
				fw := ""
				if p.Framework != "" {
					fw = " | Framework: " + p.Framework
				}
				return fmt.Sprintf("✓ Project %q activated.\n  Path: %s\n  Language: %s%s", p.ID, p.RootPath, p.Language, fw), nil
			},
		},
		{
			Name:        "scan",
			Description: "Scan a directory to discover projects (subdirectories with .git, go.mod, package.json, etc.). Optionally auto-register found projects.",
			Parameters: []ToolParameter{
				{Name: "directory", Type: "string", Description: "Directory to scan (e.g. ~/Workspace, /home/user/projects)", Required: true},
				{Name: "register", Type: "boolean", Description: "Auto-register all found projects. Default: false (dry run)."},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				dir, _ := args["directory"].(string)
				if dir == "" {
					return nil, fmt.Errorf("directory is required")
				}
				if strings.HasPrefix(dir, "~/") {
					home, _ := os.UserHomeDir()
					dir = filepath.Join(home, dir[2:])
				}
				doReg, _ := args["register"].(bool)

				found, err := s.provider.ScanDirectory(dir)
				if err != nil {
					return nil, err
				}
				if len(found) == 0 {
					return fmt.Sprintf("No projects found in %s", dir), nil
				}

				var b strings.Builder
				b.WriteString(fmt.Sprintf("Found %d projects in %s:\n\n", len(found), dir))
				for _, p := range found {
					b.WriteString(fmt.Sprintf("• %s — %s (%s/%s)\n", p.ID, p.RootPath, p.Language, p.Framework))
					if doReg {
						if err := s.provider.Register(p); err != nil {
							b.WriteString(fmt.Sprintf("  ⚠ Error: %v\n", err))
						} else {
							b.WriteString("  ✓ Registered\n")
						}
					}
				}
				if !doReg {
					b.WriteString("\nSet register=true to auto-register all.")
				}
				return b.String(), nil
			},
		},
		{
			Name:        "info",
			Description: "Get detailed info about a project: language, framework, git, build/test commands, file count.",
			Parameters: []ToolParameter{
				{Name: "project_id", Type: "string", Description: "Project ID. If empty, uses active project."},
				{Name: "session_key", Type: "string", Description: "Session key for resolving active project."},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				p := s.resolveProject(args)
				if p == nil {
					return nil, fmt.Errorf("no active project. Use project-manager_activate first")
				}
				var b strings.Builder
				b.WriteString(fmt.Sprintf("Project: %s (%s)\n", p.Name, p.ID))
				b.WriteString(fmt.Sprintf("Path: %s\n", p.RootPath))
				b.WriteString(fmt.Sprintf("Language: %s\n", p.Language))
				if p.Framework != "" {
					b.WriteString(fmt.Sprintf("Framework: %s\n", p.Framework))
				}
				if p.GitRemote != "" {
					b.WriteString(fmt.Sprintf("Git: %s\n", p.GitRemote))
				}
				b.WriteString(fmt.Sprintf("Build: %s\n", p.BuildCmd))
				b.WriteString(fmt.Sprintf("Test: %s\n", p.TestCmd))
				b.WriteString(fmt.Sprintf("Lint: %s\n", p.LintCmd))
				if p.StartCmd != "" {
					b.WriteString(fmt.Sprintf("Start: %s\n", p.StartCmd))
				}
				if p.DockerCompose != "" {
					b.WriteString(fmt.Sprintf("Docker: %s\n", p.DockerCompose))
				}

				// Count files.
				var fileCount, dirCount int
				_ = filepath.WalkDir(p.RootPath, func(_ string, d os.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					if d.IsDir() {
						name := d.Name()
						if name == ".git" || name == "node_modules" || name == "vendor" || name == "__pycache__" || name == ".next" {
							return filepath.SkipDir
						}
						dirCount++
					} else {
						fileCount++
					}
					return nil
				})
				b.WriteString(fmt.Sprintf("Files: %d files in %d dirs\n", fileCount, dirCount))
				return b.String(), nil
			},
		},
		{
			Name:        "tree",
			Description: "Show the file tree of a project, respecting .gitignore. Great for understanding project structure.",
			Parameters: []ToolParameter{
				{Name: "project_id", Type: "string", Description: "Project ID. If empty, uses active project."},
				{Name: "session_key", Type: "string", Description: "Session key for resolving active project."},
				{Name: "path", Type: "string", Description: "Subdirectory relative to project root. Empty = root."},
				{Name: "depth", Type: "integer", Description: "Max depth. Default: 3."},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				p := s.resolveProject(args)
				if p == nil {
					return nil, fmt.Errorf("no active project. Use project-manager_activate first")
				}

				subPath, _ := args["path"].(string)
				root := p.RootPath
				if subPath != "" {
					root = filepath.Join(root, subPath)
				}

				depthF, _ := args["depth"].(float64)
				maxDepth := int(depthF)
				if maxDepth <= 0 {
					maxDepth = 3
				}

				var b strings.Builder
				b.WriteString(fmt.Sprintf("📁 %s/\n", filepath.Base(root)))
				buildFileTree(&b, root, "", 0, maxDepth)

				result := b.String()
				if len(result) > 8000 {
					result = result[:8000] + "\n... (truncated — use depth or path to narrow down)"
				}
				return result, nil
			},
		},
	}
}

func (s *projectManagerSkill) SystemPrompt() string {
	return `You have project management tools. When the user mentions a project or workspace:
1. Check if it's registered with project-manager_list
2. If not, use project-manager_scan to find it or project-manager_register to add it
3. Always activate a project before using code/git/dev tools
4. The active project context is shared with git, code, and dev skills`
}

func (s *projectManagerSkill) Triggers() []string {
	return []string{"project", "workspace", "open project", "switch project", "my projects"}
}

func (s *projectManagerSkill) Init(_ context.Context, _ map[string]any) error { return nil }

func (s *projectManagerSkill) Execute(_ context.Context, input string) (string, error) {
	return "", fmt.Errorf("use specific tools: project-manager_list, project-manager_register, project-manager_activate")
}

func (s *projectManagerSkill) Shutdown() error { return nil }

// resolveProject resolves a project from args (project_id or session active).
func (s *projectManagerSkill) resolveProject(args map[string]any) *ProjectInfo {
	if id, _ := args["project_id"].(string); id != "" {
		return s.provider.Get(id)
	}
	if key, _ := args["session_key"].(string); key != "" {
		return s.provider.ActiveProject(key)
	}
	return nil
}

// ── Tree helpers ──

func buildFileTree(b *strings.Builder, dir, prefix string, depth, maxDepth int) {
	if depth >= maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var visible []os.DirEntry
	for _, e := range entries {
		if !shouldSkipEntry(e.Name(), e.IsDir()) {
			visible = append(visible, e)
		}
	}

	for i, e := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		if e.IsDir() {
			b.WriteString(fmt.Sprintf("%s%s📁 %s/\n", prefix, connector, e.Name()))
			buildFileTree(b, filepath.Join(dir, e.Name()), prefix+childPrefix, depth+1, maxDepth)
		} else {
			b.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, e.Name()))
		}
	}
}

func shouldSkipEntry(name string, isDir bool) bool {
	skip := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "__pycache__": true,
		".venv": true, ".idea": true, ".vscode": true, ".DS_Store": true,
		"dist": true, ".next": true, ".nuxt": true, "target": true,
		"coverage": true, ".cache": true, ".turbo": true,
	}
	if skip[name] {
		return true
	}
	// Skip hidden files (but allow .env, .gitignore, .github, .editorconfig).
	if strings.HasPrefix(name, ".") && len(name) > 1 {
		allowed := map[string]bool{
			".env": true, ".gitignore": true, ".dockerignore": true,
			".github": true, ".editorconfig": true, ".eslintrc.json": true,
			".prettierrc": true,
		}
		return !allowed[name]
	}
	return false
}

func sanitizeID(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}
