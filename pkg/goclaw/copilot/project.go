// Package copilot – project.go implements the ProjectManager for managing
// development projects. Provides project registration, activation, auto-detection
// of language/framework, and per-session project context.
//
// A "project" maps to a filesystem directory (typically a git repo root) with
// associated metadata like language, framework, build/test/lint commands, and
// MCP server configurations.
package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Project represents a registered development project.
type Project struct {
	// ID is the unique project identifier (slug, e.g. "goclaw", "my-saas").
	ID string `yaml:"id"`

	// Name is the human-readable project name.
	Name string `yaml:"name"`

	// RootPath is the absolute path to the project root directory.
	RootPath string `yaml:"root_path"`

	// Language is the primary programming language (auto-detected or manual).
	Language string `yaml:"language"`

	// Framework is the detected/configured framework (e.g. "laravel", "next", "gin").
	Framework string `yaml:"framework"`

	// GitRemote is the primary git remote URL (auto-detected from origin).
	GitRemote string `yaml:"git_remote,omitempty"`

	// BuildCmd is the command to build the project.
	BuildCmd string `yaml:"build_cmd,omitempty"`

	// TestCmd is the command to run tests.
	TestCmd string `yaml:"test_cmd,omitempty"`

	// LintCmd is the command to run the linter.
	LintCmd string `yaml:"lint_cmd,omitempty"`

	// StartCmd is the command to start the dev server.
	StartCmd string `yaml:"start_cmd,omitempty"`

	// DeployCmd is the command to deploy the project.
	DeployCmd string `yaml:"deploy_cmd,omitempty"`

	// DockerCompose is the path to docker-compose.yml (if present).
	DockerCompose string `yaml:"docker_compose,omitempty"`

	// EnvFile is the path to the .env file (if present).
	EnvFile string `yaml:"env_file,omitempty"`

	// MCPServers lists MCP server configurations for this project.
	MCPServers []MCPServerConfig `yaml:"mcp_servers,omitempty"`
}

// MCPServerConfig holds configuration for an MCP server associated with a project.
type MCPServerConfig struct {
	Name      string            `yaml:"name"`
	Transport string            `yaml:"transport"` // "stdio", "sse", "streamable-http"
	Command   string            `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
	URL       string            `yaml:"url,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

// ProjectManager manages registered projects and per-session active project.
type ProjectManager struct {
	mu       sync.RWMutex
	projects map[string]*Project // id → project
	active   map[string]string   // sessionKey → project ID
	dataFile string              // path to projects.yaml
}

// NewProjectManager creates a new ProjectManager, loading from disk if available.
func NewProjectManager(dataDir string) *ProjectManager {
	pm := &ProjectManager{
		projects: make(map[string]*Project),
		active:   make(map[string]string),
		dataFile: filepath.Join(dataDir, "projects.yaml"),
	}
	_ = pm.load()
	return pm
}

// Register adds or updates a project.
func (pm *ProjectManager) Register(p *Project) error {
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	if p.RootPath == "" {
		return fmt.Errorf("project root_path is required")
	}

	// Normalize path.
	absPath, err := filepath.Abs(p.RootPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	p.RootPath = absPath

	// Verify directory exists.
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("directory not found: %s", absPath)
	}

	// Auto-detect if not set.
	if p.Language == "" {
		p.Language = detectLanguage(absPath)
	}
	if p.Framework == "" {
		p.Framework = detectFramework(absPath, p.Language)
	}
	if p.BuildCmd == "" {
		p.BuildCmd = defaultBuildCmd(p.Language, p.Framework)
	}
	if p.TestCmd == "" {
		p.TestCmd = defaultTestCmd(p.Language, p.Framework)
	}
	if p.LintCmd == "" {
		p.LintCmd = defaultLintCmd(p.Language, p.Framework)
	}
	if p.StartCmd == "" {
		p.StartCmd = defaultStartCmd(p.Language, p.Framework)
	}
	if p.Name == "" {
		p.Name = filepath.Base(absPath)
	}

	// Auto-detect git remote.
	if p.GitRemote == "" {
		p.GitRemote = detectGitRemote(absPath)
	}

	// Auto-detect docker-compose.
	if p.DockerCompose == "" {
		for _, f := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
			if _, err := os.Stat(filepath.Join(absPath, f)); err == nil {
				p.DockerCompose = f
				break
			}
		}
	}

	// Auto-detect .env file.
	if p.EnvFile == "" {
		if _, err := os.Stat(filepath.Join(absPath, ".env")); err == nil {
			p.EnvFile = ".env"
		}
	}

	pm.mu.Lock()
	pm.projects[p.ID] = p
	pm.mu.Unlock()

	return pm.save()
}

// Remove removes a project by ID.
func (pm *ProjectManager) Remove(id string) error {
	pm.mu.Lock()
	delete(pm.projects, id)
	pm.mu.Unlock()
	return pm.save()
}

// Get returns a project by ID.
func (pm *ProjectManager) Get(id string) *Project {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.projects[id]
}

// List returns all registered projects sorted by name.
func (pm *ProjectManager) List() []*Project {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	list := make([]*Project, 0, len(pm.projects))
	for _, p := range pm.projects {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

// Activate sets the active project for a session.
func (pm *ProjectManager) Activate(sessionKey, projectID string) error {
	pm.mu.RLock()
	_, ok := pm.projects[projectID]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("project %q not found", projectID)
	}

	pm.mu.Lock()
	pm.active[sessionKey] = projectID
	pm.mu.Unlock()
	return nil
}

// ActiveProject returns the active project for a session, or nil.
func (pm *ProjectManager) ActiveProject(sessionKey string) *Project {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	id, ok := pm.active[sessionKey]
	if !ok {
		return nil
	}
	return pm.projects[id]
}

// FindByPath finds a project whose root matches the given path.
func (pm *ProjectManager) FindByPath(path string) *Project {
	abs, _ := filepath.Abs(path)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.projects {
		if p.RootPath == abs || strings.HasPrefix(abs, p.RootPath+"/") {
			return p
		}
	}
	return nil
}

// ScanDirectory scans a directory for projects (subdirectories with .git or known markers).
func (pm *ProjectManager) ScanDirectory(root string) ([]*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var found []*Project
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dirPath := filepath.Join(abs, entry.Name())

		// Check if it's a project (has .git, go.mod, package.json, etc.).
		if !isProjectDir(dirPath) {
			continue
		}

		id := sanitizeProjectID(entry.Name())
		lang := detectLanguage(dirPath)
		framework := detectFramework(dirPath, lang)

		p := &Project{
			ID:        id,
			Name:      entry.Name(),
			RootPath:  dirPath,
			Language:  lang,
			Framework: framework,
			BuildCmd:  defaultBuildCmd(lang, framework),
			TestCmd:   defaultTestCmd(lang, framework),
			LintCmd:   defaultLintCmd(lang, framework),
			StartCmd:  defaultStartCmd(lang, framework),
			GitRemote: detectGitRemote(dirPath),
		}
		found = append(found, p)
	}

	return found, nil
}

// ── Persistence ──

func (pm *ProjectManager) load() error {
	data, err := os.ReadFile(pm.dataFile)
	if err != nil {
		return nil // file doesn't exist yet
	}
	var projects []*Project
	if err := yaml.Unmarshal(data, &projects); err != nil {
		return err
	}
	pm.mu.Lock()
	for _, p := range projects {
		pm.projects[p.ID] = p
	}
	pm.mu.Unlock()
	return nil
}

func (pm *ProjectManager) save() error {
	pm.mu.RLock()
	list := make([]*Project, 0, len(pm.projects))
	for _, p := range pm.projects {
		list = append(list, p)
	}
	pm.mu.RUnlock()

	data, err := yaml.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pm.dataFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pm.dataFile, data, 0o644)
}

// ── Detection Helpers ──

func isProjectDir(path string) bool {
	markers := []string{
		".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml",
		"requirements.txt", "composer.json", "Gemfile", "pom.xml",
		"build.gradle", "CMakeLists.txt", "Makefile", ".project",
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(path, m)); err == nil {
			return true
		}
	}
	return false
}

func detectLanguage(path string) string {
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "go"},
		{"package.json", "javascript"},
		{"tsconfig.json", "typescript"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
		{"setup.py", "python"},
		{"composer.json", "php"},
		{"Gemfile", "ruby"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
		{"build.gradle.kts", "kotlin"},
		{"Package.swift", "swift"},
		{"CMakeLists.txt", "cpp"},
		{"mix.exs", "elixir"},
		{"pubspec.yaml", "dart"},
	}

	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(path, c.file)); err == nil {
			return c.lang
		}
	}
	return "unknown"
}

func detectFramework(path, lang string) string {
	switch lang {
	case "php":
		if _, err := os.Stat(filepath.Join(path, "artisan")); err == nil {
			return "laravel"
		}
		if _, err := os.Stat(filepath.Join(path, "symfony.lock")); err == nil {
			return "symfony"
		}
	case "javascript", "typescript":
		if data, err := os.ReadFile(filepath.Join(path, "package.json")); err == nil {
			s := string(data)
			switch {
			case strings.Contains(s, "\"next\""):
				return "next"
			case strings.Contains(s, "\"nuxt\""):
				return "nuxt"
			case strings.Contains(s, "\"react\""):
				return "react"
			case strings.Contains(s, "\"vue\""):
				return "vue"
			case strings.Contains(s, "\"svelte\""):
				return "svelte"
			case strings.Contains(s, "\"express\""):
				return "express"
			case strings.Contains(s, "\"nestjs\"") || strings.Contains(s, "\"@nestjs/core\""):
				return "nest"
			case strings.Contains(s, "\"astro\""):
				return "astro"
			}
		}
	case "python":
		if _, err := os.Stat(filepath.Join(path, "manage.py")); err == nil {
			return "django"
		}
		if data, err := os.ReadFile(filepath.Join(path, "pyproject.toml")); err == nil {
			s := string(data)
			switch {
			case strings.Contains(s, "fastapi"):
				return "fastapi"
			case strings.Contains(s, "flask"):
				return "flask"
			}
		}
	case "go":
		if _, err := os.Stat(filepath.Join(path, "cmd")); err == nil {
			if data, err := os.ReadFile(filepath.Join(path, "go.mod")); err == nil {
				s := string(data)
				switch {
				case strings.Contains(s, "github.com/gin-gonic/gin"):
					return "gin"
				case strings.Contains(s, "github.com/gofiber/fiber"):
					return "fiber"
				case strings.Contains(s, "github.com/labstack/echo"):
					return "echo"
				}
			}
		}
	case "ruby":
		if _, err := os.Stat(filepath.Join(path, "config/routes.rb")); err == nil {
			return "rails"
		}
	case "rust":
		if data, err := os.ReadFile(filepath.Join(path, "Cargo.toml")); err == nil {
			s := string(data)
			if strings.Contains(s, "actix-web") {
				return "actix"
			}
			if strings.Contains(s, "axum") {
				return "axum"
			}
		}
	case "dart":
		if _, err := os.Stat(filepath.Join(path, "lib/main.dart")); err == nil {
			return "flutter"
		}
	}
	return ""
}

func detectGitRemote(path string) string {
	data, err := os.ReadFile(filepath.Join(path, ".git/config"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.Contains(line, "[remote \"origin\"]") && i+1 < len(lines) {
			for j := i + 1; j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "["); j++ {
				if strings.Contains(lines[j], "url = ") {
					parts := strings.SplitN(lines[j], "= ", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return ""
}

func defaultBuildCmd(lang, framework string) string {
	switch lang {
	case "go":
		return "go build ./..."
	case "rust":
		return "cargo build"
	case "java":
		return "mvn package -DskipTests"
	case "kotlin":
		return "gradle build"
	case "typescript", "javascript":
		switch framework {
		case "next":
			return "npm run build"
		default:
			return "npm run build"
		}
	case "php":
		if framework == "laravel" {
			return "composer install && npm run build"
		}
	case "python":
		return "pip install -e ."
	case "dart":
		return "flutter build"
	}
	return ""
}

func defaultTestCmd(lang, framework string) string {
	switch lang {
	case "go":
		return "go test ./..."
	case "rust":
		return "cargo test"
	case "java":
		return "mvn test"
	case "kotlin":
		return "gradle test"
	case "typescript", "javascript":
		return "npm test"
	case "php":
		if framework == "laravel" {
			return "php artisan test"
		}
		return "vendor/bin/phpunit"
	case "python":
		return "pytest"
	case "ruby":
		if framework == "rails" {
			return "rails test"
		}
		return "bundle exec rspec"
	case "dart":
		return "flutter test"
	}
	return ""
}

func defaultLintCmd(lang, framework string) string {
	switch lang {
	case "go":
		return "go vet ./..."
	case "rust":
		return "cargo clippy"
	case "typescript", "javascript":
		return "npx eslint ."
	case "php":
		return "vendor/bin/phpstan analyse"
	case "python":
		return "ruff check ."
	case "ruby":
		return "rubocop"
	case "dart":
		return "flutter analyze"
	}
	return ""
}

func defaultStartCmd(lang, framework string) string {
	switch framework {
	case "next":
		return "npm run dev"
	case "nuxt":
		return "npm run dev"
	case "react", "vue", "svelte", "astro":
		return "npm run dev"
	case "express", "nest":
		return "npm run start:dev"
	case "laravel":
		return "php artisan serve"
	case "django":
		return "python manage.py runserver"
	case "fastapi":
		return "uvicorn main:app --reload"
	case "flask":
		return "flask run --reload"
	case "rails":
		return "rails server"
	case "gin", "fiber", "echo":
		return "go run ./cmd/..."
	case "flutter":
		return "flutter run"
	}

	switch lang {
	case "go":
		return "go run ."
	case "python":
		return "python main.py"
	case "typescript", "javascript":
		return "npm start"
	}
	return ""
}

func sanitizeProjectID(name string) string {
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
