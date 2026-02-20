package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
	"gopkg.in/yaml.v3"
)

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
		secrets := make(map[string]string)
		if setup.APIKey != "" {
			secrets["api_key"] = setup.APIKey
		}
		if setup.WebuiPassword != "" {
			secrets["webui_token"] = setup.WebuiPassword
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
	// HuggingFace uses a different API format - model is in the URL path
	if strings.ToLower(provider) == "huggingface" {
		return testHuggingFaceConnection(baseURL, apiKey, model)
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

// generateConfigYAML builds a config.yaml from the wizard data.
func generateConfigYAML(s *SetupRequest) string {
	var b strings.Builder

	b.WriteString("# DevClaw — generated by setup wizard\n")
	b.WriteString("# See configs/devclaw.example.yaml for full reference.\n\n")

	// ── Assistant ──
	b.WriteString("# ── Assistant ──\n")
	fmt.Fprintf(&b, "name: %q\n", s.Name)
	fmt.Fprintf(&b, "trigger: \"@%s\"\n", strings.ToLower(strings.ReplaceAll(s.Name, " ", "")))
	fmt.Fprintf(&b, "model: %q\n", s.Model)
	fmt.Fprintf(&b, "timezone: %q\n", s.Timezone)
	fmt.Fprintf(&b, "language: %q\n", s.Language)
	b.WriteString("instructions: |\n")
	b.WriteString("  You are a helpful personal assistant.\n")
	b.WriteString("  Be concise and practical in your responses.\n\n")

	// ── API ──
	b.WriteString("# ── API Provider ──\n")
	b.WriteString("api:\n")
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = providerBaseURL(s.Provider)
	}
	fmt.Fprintf(&b, "  base_url: %q\n", baseURL)
	b.WriteString("  api_key: \"${DEVCLAW_API_KEY}\"\n")
	if s.Provider != "" && s.Provider != "custom" {
		fmt.Fprintf(&b, "  provider: %q\n", s.Provider)
	}
	b.WriteString("\n")

	// ── Access ──
	b.WriteString("# ── Access Control ──\n")
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
		fmt.Fprintf(&b, "  owners: [%q]\n\n", jid)
	} else {
		b.WriteString("  owners: []\n\n")
	}

	// ── Channels ──
	b.WriteString("# ── Channels ──\n")
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

	// ── Memory ──
	b.WriteString("# ── Memory ──\n")
	b.WriteString("memory:\n")
	b.WriteString("  type: sqlite\n")
	b.WriteString("  path: \"./data/memory.db\"\n")
	b.WriteString("  max_messages: 100\n\n")

	// ── Security ──
	b.WriteString("# ── Security ──\n")
	b.WriteString("security:\n")
	b.WriteString("  max_input_length: 4096\n")
	b.WriteString("  rate_limit: 30\n\n")

	// ── Skills ──
	b.WriteString("# ── Skills ──\n")
	b.WriteString("skills:\n")
	// Go-native builtins (have actual tool implementations in Go).
	goNativeSet := map[string]bool{
		"calculator": true, "web-fetch": true, "datetime": true,
		"image-gen": true, "claude-code": true, "project-manager": true,
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
		builtins = []string{"calculator"}
	}
	fmt.Fprintf(&b, "  builtin: [%s]\n", strings.Join(builtins, ", "))
	b.WriteString("  clawdhub_dirs: [\"./skills\"]\n")
	b.WriteString("\n")

	// ── Scheduler ──
	b.WriteString("# ── Scheduler ──\n")
	b.WriteString("scheduler:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  storage: \"./data/scheduler.db\"\n\n")

	// ── WebUI ──
	b.WriteString("# ── Web UI ──\n")
	b.WriteString("webui:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  address: \":8090\"\n")
	if s.WebuiPassword != "" {
		// Reference the vault secret — never store the password in plain text.
		b.WriteString("  auth_token: \"${DEVCLAW_WEBUI_TOKEN}\"\n")
	}
	b.WriteString("\n")

	// ── Logging ──
	b.WriteString("# ── Logging ──\n")
	b.WriteString("logging:\n")
	b.WriteString("  level: info\n")
	b.WriteString("  format: json\n")

	return b.String()
}
