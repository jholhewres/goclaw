package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
	"gopkg.in/yaml.v3"
)

// providerKeyNames maps provider IDs to their standard API key variable names.
// This is a copy of copilot.ProviderKeyNames to avoid import cycles.
var providerKeyNames = map[string]string{
	"openai":      "OPENAI_API_KEY",
	"anthropic":   "ANTHROPIC_API_KEY",
	"google":      "GOOGLE_API_KEY",
	"xai":         "XAI_API_KEY",
	"groq":        "GROQ_API_KEY",
	"zai":         "ZAI_API_KEY",
	"mistral":     "MISTRAL_API_KEY",
	"openrouter":  "OPENROUTER_API_KEY",
	"cerebras":    "CEREBRAS_API_KEY",
	"minimax":     "MINIMAX_API_KEY",
	"huggingface": "HUGGINGFACE_API_KEY",
	"deepseek":    "DEEPSEEK_API_KEY",
	"custom":      "CUSTOM_API_KEY",
}

// getProviderKeyName returns the standard API key variable name for a provider.
func getProviderKeyName(provider string) string {
	if name, ok := providerKeyNames[strings.ToLower(provider)]; ok {
		return name
	}
	return "API_KEY"
}

// SetupRequest contains all data from the setup wizard frontend.
type SetupRequest struct {
	Name          string          `json:"name"`
	Language      string          `json:"language"`
	Timezone      string          `json:"timezone"`
	Provider      string          `json:"provider"`
	APIKey        string          `json:"apiKey"`
	Model         string          `json:"model"`
	BaseURL       string          `json:"baseUrl"`
	OwnerPhone    string          `json:"ownerPhone"`
	WebuiPassword string          `json:"webuiPassword"`
	VaultPassword string          `json:"vaultPassword"`
	AccessMode    string          `json:"accessMode"`
	Channels      map[string]bool `json:"channels"`
	EnabledSkills []string        `json:"enabledSkills"`
}

// handleAPISetup routes setup-related requests.
func (s *Server) handleAPISetup(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/setup/")

	switch path {
	case "status":
		s.handleSetupStatus(w, r)
	case "test-provider":
		s.handleSetupTestProvider(w, r)
	case "finalize":
		s.handleSetupFinalize(w, r)
	case "skills":
		s.handleSetupSkills(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleSetupStatus reports whether the system is already configured.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	configured := configFileExists()
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   configured,
		"current_step": 0,
	})
}

// handleSetupTestProvider makes a real API call to verify provider credentials.
func (s *Server) handleSetupTestProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
		BaseURL  string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if body.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}

	baseURL := body.BaseURL
	if baseURL == "" {
		baseURL = providerBaseURL(body.Provider)
	}
	if err := testProviderConnection(body.Provider, baseURL, body.APIKey, body.Model); err != nil {
		s.logger.Debug("provider test failed", "provider", body.Provider, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleSetupFinalize generates config.yaml and .env from wizard data.
func (s *Server) handleSetupFinalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var setup SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&setup); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if setup.Name == "" {
		setup.Name = "DevClaw"
	}
	if setup.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}

	// Safety: don't overwrite an existing config unless in setup mode.
	if configFileExists() && !s.setupMode {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "config.yaml already exists — use /api/config to modify"})
		return
	}

	// Generate and write config.yaml.
	configYAML := generateConfigYAML(&setup)
	if err := os.WriteFile("config.yaml", []byte(configYAML), 0o600); err != nil {
		s.logger.Error("failed to write config.yaml", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write config: " + err.Error()})
		return
	}
	s.logger.Info("config.yaml written by setup wizard")

	// Create data directories.
	for _, dir := range []string{"./data", "./sessions/whatsapp", "./plugins", "./skills"} {
		os.MkdirAll(dir, 0o755)
	}

	// Install selected skills as SKILL.md files so ClawdHubLoader can find them.
	if len(setup.EnabledSkills) > 0 {
		s.installSetupSkills(setup.EnabledSkills)
	}

	// Initialize the encrypted vault. Prefer dedicated vault password;
	// fall back to webui password; then to a minimal default.
	vaultPassword := setup.VaultPassword
	if vaultPassword == "" {
		vaultPassword = setup.WebuiPassword
	}
	if vaultPassword == "" {
		vaultPassword = "devclaw-default"
	}

	vaultOK := false
	if s.onVaultInit != nil {
		// Store ALL secrets in the vault — never leave them in plain text.
		// Provider API keys use standard names (OPENAI_API_KEY, etc.)
		// DEVCLAW_API_KEY is for DevClaw gateway authentication.
		secrets := make(map[string]string)
		if setup.APIKey != "" {
			// Use provider-specific key name (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.)
			providerKey := getProviderKeyName(setup.Provider)
			secrets[providerKey] = setup.APIKey
		}
		if setup.WebuiPassword != "" {
			secrets["DEVCLAW_WEBUI_TOKEN"] = setup.WebuiPassword
		}

		if len(secrets) > 0 {
			if err := s.onVaultInit(vaultPassword, secrets); err != nil {
				s.logger.Warn("failed to create vault during setup", "error", err)
			} else {
				vaultOK = true
				s.logger.Info("vault created — secrets stored securely",
					"keys", len(secrets))
			}
		} else {
			vaultOK = true
		}
	}

	// Write .env with ONLY the vault password — used by Docker Compose
	// for variable interpolation. All other secrets live in the vault.
	envContent := "# DevClaw — generated by setup wizard\n# Only the vault password is stored here.\n# All secrets (API keys, tokens) are encrypted in .devclaw.vault.\nDEVCLAW_VAULT_PASSWORD=" + vaultPassword + "\n"
	if err := os.WriteFile(".env", []byte(envContent), 0o600); err != nil {
		s.logger.Error("failed to write .env", "error", err)
	}

	if !vaultOK {
		s.logger.Warn("vault creation failed — API key may not be available until vault is initialized")
	}

	// Signal setup completion (for setup-only mode).
	if s.onSetupDone != nil {
		go s.onSetupDone()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Configuration saved. Restarting server to apply changes.",
	})
}

// installSetupSkills installs selected skills as SKILL.md files into ./skills/.
// First tries from embedded defaults, then downloads from devclaw-skills on GitHub.
func (s *Server) installSetupSkills(selected []string) {
	const skillsDir = "./skills"
	const ghRaw = "https://raw.githubusercontent.com/jholhewres/devclaw-skills/master"

	// Go-native builtins that don't need SKILL.md files (they have Go implementations).
	goNative := map[string]bool{
		"calculator": true, "datetime": true,
	}

	// Build path lookup from the cached catalog (index.yaml paths).
	catalogPaths := s.buildCatalogPathMap()

	for _, name := range selected {
		if goNative[name] {
			continue
		}

		// Check if already installed.
		targetFile := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(targetFile); err == nil {
			s.logger.Debug("skill already installed", "name", name)
			continue
		}

		// Try embedded defaults first (starter pack skills).
		installed, err := skills.InstallDefaultSkill(skillsDir, name)
		if err == nil && installed {
			s.logger.Info("skill installed from defaults", "name", name)
			continue
		}

		// Download from GitHub using catalog path or fallback flat path.
		remotePath := "skills/" + name
		if p, ok := catalogPaths[name]; ok {
			remotePath = p
		}
		url := fmt.Sprintf("%s/%s/SKILL.md", ghRaw, remotePath)

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			s.logger.Warn("failed to download skill from GitHub", "name", name, "error", err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK || len(body) == 0 {
			s.logger.Warn("skill not found on GitHub", "name", name, "status", resp.StatusCode)
			continue
		}

		targetDir := filepath.Join(skillsDir, name)
		os.MkdirAll(targetDir, 0o755)
		if err := os.WriteFile(targetFile, body, 0o644); err != nil {
			s.logger.Warn("failed to write skill file", "name", name, "error", err)
			continue
		}
		s.logger.Info("skill installed from GitHub", "name", name)
	}
}

// buildCatalogPathMap fetches index.yaml from GitHub and builds a name→path map.
func (s *Server) buildCatalogPathMap() map[string]string {
	result := make(map[string]string)

	catalog := fetchSkillsCatalogFromGitHub(s.logger)
	for _, entry := range catalog {
		name, _ := entry["name"].(string)
		path, _ := entry["path"].(string)
		if name != "" && path != "" {
			result[name] = path
		}
	}
	return result
}

// handleSetupSkills returns the list of skills available for installation.
// First tries to fetch the catalog from devclaw-skills on GitHub; falls back to embedded defaults.
func (s *Server) handleSetupSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Build catalog from embedded defaults (always available, enriched with starter_pack info).
	defaults := skills.DefaultSkills()
	result := make([]map[string]any, 0, len(defaults))
	for _, sk := range defaults {
		cat := sk.Category
		if cat == "" {
			cat = "builtin"
		}
		result = append(result, map[string]any{
			"name":         sk.Name,
			"description":  sk.Description,
			"category":     cat,
			"starter_pack": sk.StarterPack,
			"enabled":      sk.StarterPack,
			"tool_count":   0,
		})
	}

	// Merge extra skills from devclaw-skills GitHub catalog (non-embedded).
	remote := fetchSkillsCatalogFromGitHub(s.logger)
	embeddedNames := make(map[string]bool, len(defaults))
	for _, sk := range defaults {
		embeddedNames[sk.Name] = true
	}
	for _, r := range remote {
		name, _ := r["name"].(string)
		if !embeddedNames[name] {
			r["starter_pack"] = false
			result = append(result, r)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// skillsCatalogEntry represents a single skill in the devclaw-skills index.yaml.
type skillsCatalogEntry struct {
	Path        string   `yaml:"path"`
	Version     string   `yaml:"version"`
	Category    string   `yaml:"category"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
}

// skillsCatalogFile represents the top-level structure of devclaw-skills index.yaml.
type skillsCatalogFile struct {
	Skills map[string]skillsCatalogEntry `yaml:"skills"`
}

// fetchSkillsCatalogFromGitHub fetches and parses the devclaw-skills index.yaml from GitHub.
func fetchSkillsCatalogFromGitHub(logger interface{ Debug(string, ...any) }) []map[string]any {
	const catalogURL = "https://raw.githubusercontent.com/jholhewres/devclaw-skills/master/index.yaml"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(catalogURL)
	if err != nil {
		if logger != nil {
			logger.Debug("failed to fetch skills catalog from GitHub", "error", err)
		}
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if logger != nil {
			logger.Debug("skills catalog returned non-200", "status", resp.StatusCode)
		}
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil
	}

	var catalog skillsCatalogFile
	if err := yaml.Unmarshal(body, &catalog); err != nil {
		if logger != nil {
			logger.Debug("failed to parse skills catalog YAML", "error", err)
		}
		return nil
	}

	result := make([]map[string]any, 0, len(catalog.Skills))
	for name, entry := range catalog.Skills {
		result = append(result, map[string]any{
			"name":        name,
			"description": entry.Description,
			"category":    entry.Category,
			"version":     entry.Version,
			"tags":        entry.Tags,
			"path":        entry.Path,
			"enabled":     false,
			"tool_count":  0,
		})
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(result, func(i, j int) bool {
		return result[i]["name"].(string) < result[j]["name"].(string)
	})

	return result
}

// ── Provider helpers ──

// providerBaseURL returns the API base URL for known providers.
func providerBaseURL(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "google":
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case "zai":
		return "https://api.z.ai/api/paas/v4"
	case "xai":
		return "https://api.x.ai/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "minimax":
		return "https://api.minimax.io/anthropic"
	case "ollama":
		return "http://localhost:11434/v1"
	case "huggingface":
		return "https://api-inference.huggingface.co/models"
	case "lmstudio":
		return "http://localhost:1234/v1"
	case "vllm":
		return "http://localhost:8000/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// testProviderConnection makes a minimal chat completion request to verify credentials.
func testProviderConnection(provider, baseURL, apiKey, model string) error {
	providerLower := strings.ToLower(provider)

	// HuggingFace uses a different API format - model is in the URL path
	if providerLower == "huggingface" {
		return testHuggingFaceConnection(baseURL, apiKey, model)
	}

	// Anthropic uses native Messages API format (different endpoint, headers, auth)
	if providerLower == "anthropic" || strings.Contains(baseURL, "anthropic.com") {
		return testAnthropicConnection(baseURL, apiKey, model, false)
	}

	// Z.AI Anthropic proxy uses Bearer auth instead of x-api-key
	if providerLower == "zai" && strings.Contains(baseURL, "anthropic") {
		return testAnthropicConnection(baseURL, apiKey, model, true)
	}

	// Newer OpenAI models (o1, o3, o4, gpt-5) require max_completion_tokens instead of max_tokens
	usesMaxCompletionTokens := strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") ||
		strings.HasPrefix(model, "gpt-5")

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
	}
	if usesMaxCompletionTokens {
		payload["max_completion_tokens"] = 5
	} else {
		payload["max_tokens"] = 5
	}
	jsonBody, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("falha ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("invalid API key")
	case 403:
		return fmt.Errorf("access denied — check your API key permissions")
	case 404:
		return fmt.Errorf("model '%s' not found", model)
	case 429:
		return fmt.Errorf("rate limit excedido — tente novamente em alguns segundos")
	default:
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("API retornou %d: %s", resp.StatusCode, msg)
	}
}

// testHuggingFaceConnection tests HuggingFace Inference API which uses a different format.
func testHuggingFaceConnection(baseURL, apiKey, model string) error {
	// HuggingFace uses model in the URL path: /models/{model_id}
	// The baseURL already contains the base, we append the model
	url := strings.TrimSuffix(baseURL, "/") + "/" + model

	payload := map[string]any{
		"inputs": "Hi",
	}
	jsonBody, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("invalid API key")
	case 403:
		return fmt.Errorf("access denied — check your API key permissions")
	case 404:
		return fmt.Errorf("model '%s' not found on HuggingFace", model)
	case 429:
		return fmt.Errorf("rate limit exceeded — try again in a few seconds")
	case 503:
		return fmt.Errorf("model is loading — try again in a few moments")
	default:
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, msg)
	}
}

// testAnthropicConnection tests Anthropic Messages API which uses a different format.
// useBearerAuth=true for Z.AI proxy, false for native Anthropic API.
func testAnthropicConnection(baseURL, apiKey, model string, useBearerAuth bool) error {
	// Anthropic uses /v1/messages endpoint (not /chat/completions)
	endpoint := strings.TrimSuffix(baseURL, "/") + "/messages"

	payload := map[string]any{
		"model":      model,
		"max_tokens": 10,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
	}
	jsonBody, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if apiKey != "" {
		if useBearerAuth {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else {
			req.Header.Set("x-api-key", apiKey)
		}
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	// Parse Anthropic error format
	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Errorf("%s", errResp.Error.Message)
	}

	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("invalid API key")
	case 403:
		return fmt.Errorf("access denied — check your API key permissions")
	case 404:
		return fmt.Errorf("model '%s' not found", model)
	case 429:
		return fmt.Errorf("rate limit exceeded — try again in a few seconds")
	case 500, 529:
		return fmt.Errorf("Anthropic service temporarily unavailable")
	default:
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, msg)
	}
}

// configFileExists checks if a config file exists at common paths.
func configFileExists() bool {
	candidates := []string{
		"config.yaml", "config.yml",
		"devclaw.yaml", "devclaw.yml",
		"configs/config.yaml", "configs/devclaw.yaml",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// detectChrome checks if Chrome/Chromium is available on the system.
func detectChrome() (string, bool) {
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
		if path, err := exec.LookPath(c); err == nil {
			return path, true
		}
	}
	return "", false
}

// generateConfigYAML builds a config.yaml from the wizard data.
func generateConfigYAML(s *SetupRequest) string {
	var b strings.Builder

	b.WriteString("# -----------------------------------------------------------\n")
	b.WriteString("# DevClaw - Configuration\n")
	b.WriteString("# Generated by setup wizard\n")
	b.WriteString("# -----------------------------------------------------------\n\n")

	// -- Assistant --
	b.WriteString("# -- Assistant ---------------------------------------------\n")
	fmt.Fprintf(&b, "name: %q\n", s.Name)
	fmt.Fprintf(&b, "trigger: \"@%s\"\n", strings.ToLower(strings.ReplaceAll(s.Name, " ", "")))
	fmt.Fprintf(&b, "model: %q\n", s.Model)
	fmt.Fprintf(&b, "timezone: %q\n", s.Timezone)
	fmt.Fprintf(&b, "language: %q\n", s.Language)
	b.WriteString("instructions: |\n")
	b.WriteString("  You are a helpful personal assistant.\n")
	b.WriteString("  Be concise and practical in your responses.\n")
	b.WriteString("  Respond in Portuguese when the user writes in Portuguese.\n\n")

	// -- API --
	b.WriteString("# -- API Provider ------------------------------------------\n")
	b.WriteString("# API keys are stored in the encrypted vault (.devclaw.vault)\n")
	b.WriteString("# and injected at runtime. Do NOT put keys here.\n")
	b.WriteString("api:\n")
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = providerBaseURL(s.Provider)
	}
	fmt.Fprintf(&b, "  base_url: %q\n", baseURL)
	if s.Provider != "" && s.Provider != "custom" {
		fmt.Fprintf(&b, "  provider: %q\n", s.Provider)
	}
	b.WriteString("\n")

	// -- Agent --
	b.WriteString("# -- Agent -------------------------------------------------\n")
	b.WriteString("agent:\n")
	b.WriteString("  max_iterations: 20\n")
	b.WriteString("  max_tool_calls: 50\n")
	b.WriteString("  timeout: 120s\n")
	b.WriteString("  stream_tokens: true\n\n")

	// -- Access --
	b.WriteString("# -- Access Control ----------------------------------------\n")
	b.WriteString("access:\n")
	switch s.AccessMode {
	case "relaxed":
		b.WriteString("  default_policy: allow\n")
	default:
		b.WriteString("  default_policy: deny\n")
	}
	if s.OwnerPhone != "" {
		// Format as WhatsApp JID for the access system.
		jid := s.OwnerPhone + "@s.whatsapp.net"
		fmt.Fprintf(&b, "  owners: [%q]\n", jid)
	} else {
		b.WriteString("  owners: []\n")
	}
	b.WriteString("  admins: []\n")
	b.WriteString("  allowed_users: []\n")
	b.WriteString("  blocked_users: []\n")
	b.WriteString("  pending_message: \"Access not authorized. Contact an admin.\"\n\n")

	// -- Workspaces --
	b.WriteString("# -- Workspaces --------------------------------------------\n")
	b.WriteString("workspaces:\n")
	b.WriteString("  default_workspace: \"default\"\n")
	b.WriteString("  workspaces:\n")
	b.WriteString("    - id: \"default\"\n")
	b.WriteString("      name: \"Default\"\n")
	b.WriteString("      description: \"Default workspace for all users\"\n")
	b.WriteString("      active: true\n\n")

	// -- Channels --
	b.WriteString("# -- Channels ----------------------------------------------\n")
	b.WriteString("channels:\n")
	if s.Channels["whatsapp"] {
		b.WriteString("  whatsapp:\n")
		b.WriteString("    session_dir: \"./sessions/whatsapp\"\n")
		fmt.Fprintf(&b, "    trigger: \"@%s\"\n", strings.ToLower(strings.ReplaceAll(s.Name, " ", "")))
		b.WriteString("    respond_to_groups: true\n")
		b.WriteString("    respond_to_dms: true\n")
		b.WriteString("    auto_read: true\n")
		b.WriteString("    send_typing: true\n")
		b.WriteString("    media_dir: \"./data/media\"\n")
		b.WriteString("    max_media_size_mb: 16\n")
	}
	if s.Channels["telegram"] {
		b.WriteString("  telegram:\n")
		b.WriteString("    token: \"${TELEGRAM_BOT_TOKEN}\"\n")
		b.WriteString("    respond_to_groups: true\n")
		b.WriteString("    respond_to_dms: true\n")
	}
	if s.Channels["discord"] {
		b.WriteString("  discord:\n")
		b.WriteString("    token: \"${DISCORD_BOT_TOKEN}\"\n")
		b.WriteString("    respond_to_dms: true\n")
	}
	if s.Channels["slack"] {
		b.WriteString("  slack:\n")
		b.WriteString("    bot_token: \"${SLACK_BOT_TOKEN}\"\n")
		b.WriteString("    app_token: \"${SLACK_APP_TOKEN}\"\n")
	}
	b.WriteString("\n")

	// -- Memory --
	b.WriteString("# -- Memory ------------------------------------------------\n")
	b.WriteString("memory:\n")
	b.WriteString("  type: \"file\"\n")
	b.WriteString("  path: \"./data/memory.db\"\n")
	b.WriteString("  max_messages: 100\n")
	b.WriteString("  compression_strategy: \"summarize\"\n\n")

	// -- Security --
	b.WriteString("# -- Security ----------------------------------------------\n")
	b.WriteString("security:\n")
	b.WriteString("  max_input_length: 4096\n")
	b.WriteString("  rate_limit: 30\n")
	b.WriteString("  enable_pii_detection: false\n")
	b.WriteString("  enable_url_validation: true\n")
	b.WriteString("  tool_guard:\n")
	b.WriteString("    enabled: true\n")
	switch s.AccessMode {
	case "relaxed":
		// Tudo liberado para owner
		b.WriteString("    allow_destructive: true\n")
		b.WriteString("    allow_sudo: true\n")
		b.WriteString("    allow_reboot: true\n")
		b.WriteString("    block_sudo: false\n")
	case "strict":
		// Balanceado - sudo ok, destrutivo não
		b.WriteString("    allow_destructive: false\n")
		b.WriteString("    allow_sudo: true\n")
		b.WriteString("    allow_reboot: false\n")
		b.WriteString("    block_sudo: false\n")
	default: // paranoid
		// Maximamente restritivo
		b.WriteString("    allow_destructive: false\n")
		b.WriteString("    allow_sudo: false\n")
		b.WriteString("    allow_reboot: false\n")
		b.WriteString("    block_sudo: true\n")
	}
	b.WriteString("\n")

	// -- Token Budget --
	b.WriteString("# -- Token Budget ------------------------------------------\n")
	b.WriteString("token_budget:\n")
	b.WriteString("  total: 128000\n")
	b.WriteString("  reserved: 4096\n")
	b.WriteString("  system: 500\n")
	b.WriteString("  skills: 2000\n")
	b.WriteString("  memory: 1000\n")
	b.WriteString("  history: 8000\n")
	b.WriteString("  tools: 4000\n\n")

	// -- Plugins --
	b.WriteString("# -- Plugins -----------------------------------------------\n")
	b.WriteString("plugins:\n")
	b.WriteString("  dir: \"./plugins\"\n\n")

	// -- Skills --
	b.WriteString("# -- Skills ------------------------------------------------\n")
	b.WriteString("skills:\n")
	// Go-native builtins (have actual tool implementations in Go).
	goNativeSet := map[string]bool{
		"calculator": true, "web-fetch": true, "datetime": true,
		"image-gen": true, "claude-code": true, "project-manager": true,
		"weather": true, "web-search": true,
	}
	var builtins []string
	for _, sk := range s.EnabledSkills {
		if goNativeSet[sk] {
			builtins = append(builtins, sk)
		}
		// Non-native skills are installed as SKILL.md files in ./skills/
		// and loaded automatically by ClawdHubLoader.
	}
	if len(builtins) == 0 {
		builtins = []string{"calculator", "web-search"}
	}
	fmt.Fprintf(&b, "  builtin: [%s]\n", strings.Join(builtins, ", "))
	b.WriteString("  clawdhub_dirs: [\"./skills\"]\n\n")

	// -- Scheduler --
	b.WriteString("# -- Scheduler ---------------------------------------------\n")
	b.WriteString("scheduler:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  storage: \"./data/scheduler.db\"\n\n")

	// -- WebUI --
	b.WriteString("# -- Web UI -----------------------------------------------\n")
	b.WriteString("webui:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  address: \"0.0.0.0:8090\"\n")
	if s.WebuiPassword != "" {
		b.WriteString("  auth_token: \"${DEVCLAW_WEBUI_TOKEN}\"\n")
	}
	b.WriteString("\n")

	// -- Logging --
	b.WriteString("# -- Logging -----------------------------------------------\n")
	b.WriteString("logging:\n")
	b.WriteString("  level: \"info\"\n")
	b.WriteString("  format: \"json\"\n\n")

	// -- Background Routines --
	b.WriteString("# -- Background Routines ------------------------------------\n")
	b.WriteString("routines:\n")
	b.WriteString("  metrics:\n")
	b.WriteString("    enabled: false\n")
	b.WriteString("    interval: 1m\n")
	b.WriteString("  memory_indexer:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("    interval: 5m\n\n")

	// -- Native Media --
	b.WriteString("# -- Native Media ------------------------------------------\n")
	b.WriteString("native_media:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  store:\n")
	b.WriteString("    base_dir: \"./data/media\"\n")
	b.WriteString("    temp_dir: \"./data/media/temp\"\n")
	b.WriteString("    max_file_size: 52428800\n")
	b.WriteString("  service:\n")
	b.WriteString("    max_image_size: 20971520\n")
	b.WriteString("    max_audio_size: 26214400\n")
	b.WriteString("    max_doc_size: 52428800\n")
	b.WriteString("    temp_ttl: \"24h\"\n")
	b.WriteString("    cleanup_enabled: true\n")
	b.WriteString("    cleanup_interval: \"1h\"\n")
	b.WriteString("  enrichment:\n")
	b.WriteString("    auto_enrich_images: true\n")
	b.WriteString("    auto_enrich_audio: true\n")
	b.WriteString("    auto_enrich_documents: true\n\n")

	// -- Database Hub --
	b.WriteString("# -- Database Hub -------------------------------------------\n")
	b.WriteString("database:\n")
	b.WriteString("  path: \"./data/devclaw.db\"\n")
	b.WriteString("  hub:\n")
	b.WriteString("    backend: \"sqlite\"\n")
	b.WriteString("    sqlite:\n")
	b.WriteString("      path: \"./data/devclaw.db\"\n")
	b.WriteString("      journal_mode: \"WAL\"\n")
	b.WriteString("      busy_timeout: 5000\n")
	b.WriteString("      foreign_keys: true\n")
	b.WriteString("    memory:\n")
	b.WriteString("      backend: \"sqlite\"\n")
	b.WriteString("      path: \"./data/memory.db\"\n\n")

	// -- Browser (auto-detect Chrome) --
	b.WriteString("# -- Browser Automation --------------------------------------\n")
	b.WriteString("# Chrome/Chromium is required for browser tools.\n")
	if chromePath, found := detectChrome(); found {
		b.WriteString("# Auto-detected Chrome installation.\n")
		fmt.Fprintf(&b, "browser:\n")
		fmt.Fprintf(&b, "  enabled: true\n")
		fmt.Fprintf(&b, "  chrome_path: %q\n", chromePath)
		b.WriteString("  headless: true\n")
		b.WriteString("  timeout_seconds: 30\n")
	} else {
		b.WriteString("# Chrome/Chromium not found. Browser tools disabled.\n")
		b.WriteString("# Install Chrome or set browser.chrome_path to enable.\n")
		b.WriteString("browser:\n")
		b.WriteString("  enabled: false\n")
	}

	return b.String()
}
