package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
	"github.com/jholhewres/devclaw/pkg/devclaw/oauth/providers"
	"github.com/jholhewres/devclaw/pkg/devclaw/paths"
)

// OAuthAPI provides OAuth operations for the web UI.
type OAuthAPI interface {
	// TokenManager returns the OAuth token manager
	GetTokenManager() *oauth.TokenManager
}

// oauthFlow tracks active OAuth flows for callback handling.
type oauthFlow struct {
	state     string
	pkce      *oauth.PKCEPair
	provider  string
	expiresAt time.Time
	result    chan oauthFlowResult
}

type oauthFlowResult struct {
	cred *oauth.OAuthCredential
	err  error
}

// OAuthHandlers manages OAuth-related HTTP handlers.
type OAuthHandlers struct {
	tokenManager *oauth.TokenManager
	logger       *slog.Logger

	flowsMu sync.RWMutex
	flows   map[string]*oauthFlow // state -> flow

	dataDir string
}

// NewOAuthHandlers creates new OAuth handlers.
func NewOAuthHandlers(dataDir string, logger *slog.Logger) (*OAuthHandlers, error) {
	tm, err := oauth.NewTokenManager(dataDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create token manager: %w", err)
	}

	// Register providers
	tm.RegisterProvider(providers.NewGeminiProvider())
	tm.RegisterProvider(providers.NewChatGPTProvider())
	tm.RegisterProvider(providers.NewQwenProvider())
	tm.RegisterProvider(providers.NewMiniMaxProvider())

	// Start auto-refresh
	tm.StartAutoRefresh()

	return &OAuthHandlers{
		tokenManager: tm,
		logger:       logger.With("component", "oauth-handlers"),
		flows:        make(map[string]*oauthFlow),
		dataDir:      dataDir,
	}, nil
}

// TokenManager returns the token manager.
func (h *OAuthHandlers) TokenManager() *oauth.TokenManager {
	return h.tokenManager
}

// Stop stops the OAuth handlers.
func (h *OAuthHandlers) Stop() {
	if h.tokenManager != nil {
		h.tokenManager.Stop()
	}
}

// RegisterRoutes registers OAuth routes on the mux.
func (h *OAuthHandlers) RegisterRoutes(mux *http.ServeMux, authMiddleware func(http.HandlerFunc) http.HandlerFunc) {
	// Public routes (for OAuth callbacks)
	mux.HandleFunc("/api/oauth/callback", h.handleOAuthCallback)

	// Protected routes
	mux.HandleFunc("/api/oauth/providers", authMiddleware(h.handleListProviders))
	mux.HandleFunc("/api/oauth/status", authMiddleware(h.handleOAuthStatus))
	mux.HandleFunc("/api/oauth/start/", authMiddleware(h.handleOAuthStart))
	mux.HandleFunc("/api/oauth/refresh/", authMiddleware(h.handleOAuthRefresh))
	mux.HandleFunc("/api/oauth/logout/", authMiddleware(h.handleOAuthLogout))

	// Google OAuth credentials configuration (HTML page + API)
	mux.HandleFunc("/oauth/google/setup", authMiddleware(h.handleGoogleSetupPage))
	mux.HandleFunc("/api/oauth/google/credentials", authMiddleware(h.handleGoogleCredentials))
}

// OAuthProviderInfo contains provider info for the UI.
type OAuthProviderInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	FlowType    string `json:"flow_type"` // "pkce" or "device_code"
	Experimental bool   `json:"experimental,omitempty"`
}

// handleListProviders returns available OAuth providers.
func (h *OAuthHandlers) handleListProviders(w http.ResponseWriter, r *http.Request) {
	hasGoogleCreds := getGoogleClientID() != ""

	providers := []OAuthProviderInfo{
		{ID: "gemini", Label: "Google Gemini", FlowType: "pkce"},
		{ID: "chatgpt", Label: "ChatGPT/Codex", FlowType: "pkce", Experimental: true},
		{ID: "qwen", Label: "Qwen Portal", FlowType: "device_code"},
		{ID: "minimax", Label: "MiniMax Portal", FlowType: "device_code"},
	}

	// Add Google Workspace providers if credentials are configured
	if hasGoogleCreds {
		providers = append(providers,
			OAuthProviderInfo{ID: "google-gmail", Label: "Gmail", FlowType: "pkce"},
			OAuthProviderInfo{ID: "google-calendar", Label: "Google Calendar", FlowType: "pkce"},
			OAuthProviderInfo{ID: "google-drive", Label: "Google Drive", FlowType: "pkce"},
		)
	} else {
		// Show Google Workspace as manual setup required
		providers = append(providers,
			OAuthProviderInfo{ID: "google-workspace", Label: "Google Workspace (Manual)", FlowType: "manual"},
		)
	}

	writeOAuthJSON(w, http.StatusOK, providers)
}

// handleOAuthStatus returns OAuth status for all providers.
func (h *OAuthHandlers) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	status := h.tokenManager.GetStatus()
	writeOAuthJSON(w, http.StatusOK, status)
}

// OAuthStartResponse is returned when starting an OAuth flow.
type OAuthStartResponse struct {
	FlowType    string `json:"flow_type"`
	AuthURL     string `json:"auth_url,omitempty"`     // For PKCE flow
	Provider    string `json:"provider"`
	UserCode    string `json:"user_code,omitempty"`    // For device code flow
	VerifyURL   string `json:"verify_url,omitempty"`   // For device code flow
	ExpiresIn   int    `json:"expires_in,omitempty"`   // For device code flow
	Experimental bool   `json:"experimental,omitempty"` // Warning flag
}

// handleOAuthStart starts an OAuth flow for a provider.
func (h *OAuthHandlers) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/start/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	ctx := r.Context()

	switch provider {
	case "gemini":
		h.startPKCEFlow(ctx, w, r, provider, providers.NewGeminiProvider())
	case "chatgpt":
		h.startPKCEFlow(ctx, w, r, provider, providers.NewChatGPTProvider())
	case "qwen":
		h.startDeviceCodeFlow(ctx, w, r, provider, providers.NewQwenProvider())
	case "minimax":
		region := r.URL.Query().Get("region")
		if region == "" {
			region = "global"
		}
		h.startDeviceCodeFlow(ctx, w, r, provider,
			providers.NewMiniMaxProvider(providers.WithMiniMaxRegion(region)))
	case "google", "google-gmail", "google-calendar", "google-drive", "google-sheets",
		"google-docs", "google-slides", "google-contacts", "google-tasks", "google-people":
		// Google Workspace services - use PKCE OAuth flow
		h.startGoogleWorkspacePKCEFlow(ctx, w, r, provider)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unknown provider: "+provider)
	}
}

// PKCEProvider is the interface for PKCE-based OAuth providers.
type PKCEProvider interface {
	Name() string
	Label() string
	AuthURL(state, challenge string) string
	ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error)
	RedirectPort() int
}

func (h *OAuthHandlers) startPKCEFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string, p PKCEProvider) {
	// Generate PKCE
	pkce, err := oauth.GeneratePKCE()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to generate PKCE: "+err.Error())
		return
	}

	// Generate state
	state := generateState()

	// Store flow for callback
	flow := &oauthFlow{
		state:     state,
		pkce:      pkce,
		provider:  provider,
		expiresAt: time.Now().Add(10 * time.Minute),
		result:    make(chan oauthFlowResult, 1),
	}

	h.flowsMu.Lock()
	h.flows[state] = flow
	h.flowsMu.Unlock()

	// Cleanup old flows
	go h.cleanupExpiredFlows()

	// Build auth URL
	authURL := p.AuthURL(state, pkce.Challenge)

	// Response
	resp := OAuthStartResponse{
		FlowType: "pkce",
		AuthURL:  authURL,
		Provider: provider,
	}

	if provider == "chatgpt" {
		resp.Experimental = true
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// DeviceCodeProvider is the interface for device code OAuth providers.
type DeviceCodeProvider interface {
	Name() string
	StartDeviceFlow(ctx context.Context) (*oauth.DeviceCodeResponse, error)
	PollForToken(ctx context.Context, deviceCode string, interval time.Duration) (*oauth.OAuthCredential, error)
}

func (h *OAuthHandlers) startDeviceCodeFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string, p DeviceCodeProvider) {
	// Start device code flow
	deviceResp, err := p.StartDeviceFlow(ctx)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to start device code flow: "+err.Error())
		return
	}

	// Response
	resp := OAuthStartResponse{
		FlowType:  "device_code",
		Provider:  provider,
		UserCode:  deviceResp.UserCode,
		VerifyURL: deviceResp.VerificationURI,
		ExpiresIn: deviceResp.ExpiresIn,
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// startGoogleWorkspacePKCEFlow starts a PKCE OAuth flow for Google Workspace services.
// If no client ID is configured, returns manual setup instructions.
func (h *OAuthHandlers) startGoogleWorkspacePKCEFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string) {
	// Get client ID from environment or config
	clientID := getGoogleClientID()
	clientSecret := getGoogleClientSecret()

	if clientID == "" {
		// Fall back to manual instructions
		h.startGoogleWorkspaceManualFlow(ctx, w, r, provider)
		return
	}

	// Create provider based on service
	var p PKCEProvider
	switch provider {
	case "google-gmail":
		p = providers.NewGmailProvider(
			providers.WithGoogleClientID(clientID),
			providers.WithGoogleClientSecret(clientSecret),
		)
	case "google-calendar":
		p = providers.NewCalendarProvider(
			providers.WithGoogleClientID(clientID),
			providers.WithGoogleClientSecret(clientSecret),
		)
	case "google-drive":
		p = providers.NewDriveProvider(
			providers.WithGoogleClientID(clientID),
			providers.WithGoogleClientSecret(clientSecret),
		)
	default:
		// Generic Google provider with full scopes
		p = providers.NewGoogleProvider(
			providers.WithGoogleClientID(clientID),
			providers.WithGoogleClientSecret(clientSecret),
			providers.WithGoogleService(getServiceFromProvider(provider)),
			providers.WithGoogleName(provider),
		)
	}

	h.startPKCEFlow(ctx, w, r, provider, p)
}

// getServiceFromProvider extracts service name from provider string
func getServiceFromProvider(provider string) string {
	parts := strings.SplitN(provider, "-", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return "full"
}

// startGoogleWorkspaceManualFlow provides instructions for manual OAuth setup.
func (h *OAuthHandlers) startGoogleWorkspaceManualFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string) {
	// Map provider to service name
	serviceMap := map[string]string{
		"google":          "gmail,calendar,drive",
		"google-gmail":    "gmail",
		"google-calendar": "calendar",
		"google-drive":    "drive",
		"google-sheets":   "sheets",
		"google-docs":     "docs",
		"google-slides":   "slides",
		"google-contacts": "contacts",
		"google-tasks":    "tasks",
		"google-people":   "people",
	}

	services := serviceMap[provider]
	if services == "" {
		services = "gmail,calendar,drive"
	}

	// Return instructions for manual setup
	resp := map[string]any{
		"flow_type":    "manual",
		"provider":     provider,
		"instructions": "Google Workspace OAuth requires client credentials.",
		"setup_steps": []string{
			"1. Create OAuth credentials in Google Cloud Console",
			"2. Set GOOGLE_CLIENT_ID environment variable",
			"3. (Optional) Set GOOGLE_CLIENT_SECRET for web application credentials",
			"4. Restart DevClaw and try again",
		},
		"alternative": "Or use gogcli: go install github.com/steipete/gogcli@latest && gog auth add you@gmail.com --services " + services,
		"docs_url":    "https://console.cloud.google.com/apis/credentials",
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// getGoogleClientID returns the Google OAuth client ID from environment or stored config.
func getGoogleClientID() string {
	// Check environment variables first
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	if id := os.Getenv("GOOGLE_OAUTH_CLIENT_ID"); id != "" {
		return id
	}
	// Check stored config
	if creds := loadGoogleCredentials(); creds != nil {
		return creds.ClientID
	}
	return ""
}

// getGoogleClientSecret returns the Google OAuth client secret from environment or stored config.
func getGoogleClientSecret() string {
	if secret := os.Getenv("GOOGLE_CLIENT_SECRET"); secret != "" {
		return secret
	}
	if secret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"); secret != "" {
		return secret
	}
	// Check stored config
	if creds := loadGoogleCredentials(); creds != nil {
		return creds.ClientSecret
	}
	return ""
}

// GoogleCredentials represents stored Google OAuth credentials.
type GoogleCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// googleCredentialsPath returns the path to the Google credentials file.
func googleCredentialsPath() string {
	return fmt.Sprintf("%s/oauth/google_credentials.json", paths.ResolveDataDir())
}

// loadGoogleCredentials loads stored Google OAuth credentials.
func loadGoogleCredentials() *GoogleCredentials {
	path := googleCredentialsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var creds GoogleCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil
	}
	return &creds
}

// saveGoogleCredentials saves Google OAuth credentials.
func saveGoogleCredentials(creds *GoogleCredentials) error {
	path := googleCredentialsPath()
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// handleGoogleCredentials handles GET/POST for Google OAuth credentials.
// GET - returns current status (without secret)
// POST - saves new credentials
// DELETE - removes stored credentials
func (h *OAuthHandlers) handleGoogleCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return current status
		creds := loadGoogleCredentials()
		status := map[string]interface{}{
			"configured": creds != nil && creds.ClientID != "",
		}
		if creds != nil {
			status["client_id"] = creds.ClientID
			status["has_secret"] = creds.ClientSecret != ""
		}
		// Also check environment variables
		status["env_client_id"] = os.Getenv("GOOGLE_CLIENT_ID") != "" || os.Getenv("GOOGLE_OAUTH_CLIENT_ID") != ""
		writeOAuthJSON(w, http.StatusOK, status)

	case http.MethodPost:
		// Save new credentials
		var creds GoogleCredentials
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if creds.ClientID == "" {
			writeOAuthError(w, http.StatusBadRequest, "client_id is required")
			return
		}
		if err := saveGoogleCredentials(&creds); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to save credentials: "+err.Error())
			return
		}
		writeOAuthJSON(w, http.StatusOK, map[string]string{
			"status":    "saved",
			"client_id": creds.ClientID,
		})

	case http.MethodDelete:
		// Remove stored credentials
		path := googleCredentialsPath()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			writeOAuthError(w, http.StatusInternalServerError, "failed to remove credentials: "+err.Error())
			return
		}
		writeOAuthJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGoogleSetupPage serves an HTML page for configuring Google OAuth credentials.
func (h *OAuthHandlers) handleGoogleSetupPage(w http.ResponseWriter, r *http.Request) {
	// Check if this is a form submission
	if r.Method == http.MethodPost {
		clientID := r.FormValue("client_id")
		clientSecret := r.FormValue("client_secret")

		if clientID == "" {
			h.renderGoogleSetupPage(w, "", "", "Client ID is required")
			return
		}

		creds := &GoogleCredentials{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}
		if err := saveGoogleCredentials(creds); err != nil {
			h.renderGoogleSetupPage(w, clientID, clientSecret, "Failed to save: "+err.Error())
			return
		}

		// Success - redirect to OAuth providers page or show success
		h.renderGoogleSetupSuccess(w, clientID)
		return
	}

	// Check for delete action
	if r.Method == http.MethodDelete || r.URL.Query().Get("delete") == "1" {
		path := googleCredentialsPath()
		os.Remove(path)
		h.renderGoogleSetupPage(w, "", "", "")
		return
	}

	// GET - show the form
	creds := loadGoogleCredentials()
	var clientID, clientSecret string
	if creds != nil {
		clientID = creds.ClientID
		clientSecret = creds.ClientSecret
	}
	h.renderGoogleSetupPage(w, clientID, clientSecret, "")
}

func (h *OAuthHandlers) renderGoogleSetupPage(w http.ResponseWriter, clientID, clientSecret, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	hasEnvCreds := os.Getenv("GOOGLE_CLIENT_ID") != "" || os.Getenv("GOOGLE_OAUTH_CLIENT_ID") != ""

	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>Google OAuth Setup - DevClaw</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; background: #f5f5f5; }
		.container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
		h1 { margin: 0 0 8px 0; color: #333; }
		h2 { margin: 0 0 8px 0; color: #333; font-size: 18px; }
		p { color: #666; margin: 0 0 20px 0; }
		.form-group { margin-bottom: 16px; }
		label { display: block; margin-bottom: 6px; font-weight: 500; color: #333; }
		input[type="text"], input[type="password"] { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; box-sizing: border-box; }
		input:focus { outline: none; border-color: #4285f4; box-shadow: 0 0 0 2px rgba(66,133,244,0.2); }
		.hint { font-size: 12px; color: #888; margin-top: 4px; }
		.error { background: #fef2f2; border: 1px solid #fecaca; color: #dc2626; padding: 12px; border-radius: 4px; margin-bottom: 16px; }
		.success { background: #f0fdf4; border: 1px solid #bbf7d0; color: #16a34a; padding: 12px; border-radius: 4px; margin-bottom: 16px; }
		.info { background: #eff6ff; border: 1px solid #bfdbfe; color: #2563eb; padding: 12px; border-radius: 4px; margin-bottom: 16px; font-size: 14px; }
		.info code { background: rgba(0,0,0,0.1); padding: 2px 6px; border-radius: 3px; }
		.buttons { display: flex; gap: 10px; margin-top: 24px; }
		button { padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; font-weight: 500; }
		.btn-primary { background: #4285f4; color: white; }
		.btn-primary:hover { background: #3367d6; }
		.btn-secondary { background: #e5e7eb; color: #374151; }
		.btn-secondary:hover { background: #d1d5db; }
		.btn-danger { background: #ef4444; color: white; }
		.btn-danger:hover { background: #dc2626; }
		a { color: #4285f4; }
		.status { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; margin-left: 8px; }
		.status-configured { background: #dcfce7; color: #16a34a; }
		.status-env { background: #fef3c7; color: #d97706; }
		.status-none { background: #f3f4f6; color: #6b7280; }
		.divider { border-top: 1px solid #e5e7eb; margin: 24px 0; }
	</style>
</head>
<body>
	<div class="container">
		<h1>Google OAuth Setup</h1>
		<p>Configure your Google OAuth credentials to enable Gmail, Calendar, and Drive integration.</p>
`)

	if errorMsg != "" {
		fmt.Fprintf(w, `		<div class="error">%s</div>
`, errorMsg)
	}

	if hasEnvCreds {
		fmt.Fprint(w, `		<div class="info">
			<strong>Environment variables detected.</strong><br>
			Google OAuth credentials are configured via environment variables (GOOGLE_CLIENT_ID).
			You can still save credentials here to override them.
		</div>
`)
	}

	fmt.Fprint(w, `
		<form method="post">
			<div class="form-group">
				<label for="client_id">Client ID</label>
				<input type="text" id="client_id" name="client_id" value="`)
	fmt.Fprint(w, clientID)
	fmt.Fprint(w, `" placeholder="xxx.apps.googleusercontent.com">
				<p class="hint">From Google Cloud Console > APIs & Services > Credentials</p>
			</div>

			<div class="form-group">
				<label for="client_secret">Client Secret (optional)</label>
				<input type="password" id="client_secret" name="client_secret" value="`)
	fmt.Fprint(w, clientSecret)
	fmt.Fprint(w, `" placeholder="GOCSPX-xxx">
				<p class="hint">Required for Web Application credentials, optional for Desktop apps</p>
			</div>

			<div class="buttons">
				<button type="submit" class="btn-primary">Save Credentials</button>
				<a href="/settings/oauth" class="btn-secondary" style="text-decoration:none; padding: 10px 20px; display: inline-block;">Cancel</a>
			</div>
		</form>

		<div class="divider"></div>

		<h2>Setup Instructions</h2>
		<ol style="color: #666; line-height: 1.8;">
			<li>Go to <a href="https://console.cloud.google.com/apis/credentials" target="_blank">Google Cloud Console</a></li>
			<li>Create a new project or select existing one</li>
			<li>Click "Create Credentials" > "OAuth client ID"</li>
			<li>Choose "Desktop app" or "Web application"</li>
			<li>Add redirect URI: <code>http://localhost:8086/oauth/callback</code></li>
			<li>Copy the Client ID and paste above</li>
		</ol>
	</div>
</body>
</html>`)
}

func (h *OAuthHandlers) renderGoogleSetupSuccess(w http.ResponseWriter, clientID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>Google OAuth Setup - DevClaw</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; background: #f5f5f5; }
		.container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); text-align: center; }
		h1 { color: #16a34a; margin-bottom: 8px; }
		p { color: #666; margin-bottom: 20px; }
		.success-icon { font-size: 48px; margin-bottom: 16px; }
		.btn { display: inline-block; padding: 12px 24px; background: #4285f4; color: white; text-decoration: none; border-radius: 4px; font-weight: 500; }
		.btn:hover { background: #3367d6; }
	</style>
</head>
<body>
	<div class="container">
		<div class="success-icon">✓</div>
		<h1>Credentials Saved!</h1>
		<p>Your Google OAuth credentials have been saved successfully.</p>
		<p style="font-size: 14px; color: #888;">Client ID: `)
	fmt.Fprint(w, clientID[:min(30, len(clientID))])
	if len(clientID) > 30 {
		fmt.Fprint(w, "...")
	}
	fmt.Fprint(w, `</p>
		<a href="/settings/oauth" class="btn">Go to OAuth Settings</a>
	</div>
</body>
</html>`)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleOAuthCallback handles OAuth callbacks.
func (h *OAuthHandlers) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for error
	if err := query.Get("error"); err != "" {
		writeOAuthError(w, http.StatusBadRequest, "OAuth error: "+err)
		return
	}

	// Get code and state
	code := query.Get("code")
	state := query.Get("state")

	if code == "" || state == "" {
		writeOAuthError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	// Find flow
	h.flowsMu.RLock()
	flow, ok := h.flows[state]
	h.flowsMu.RUnlock()

	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	// Exchange code for token
	var cred *oauth.OAuthCredential
	var err error

	ctx := r.Context()

	switch flow.provider {
	case "gemini":
		p := providers.NewGeminiProvider()
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "chatgpt":
		p := providers.NewChatGPTProvider()
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "google-gmail":
		p := providers.NewGmailProvider(
			providers.WithGoogleClientID(getGoogleClientID()),
			providers.WithGoogleClientSecret(getGoogleClientSecret()),
		)
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "google-calendar":
		p := providers.NewCalendarProvider(
			providers.WithGoogleClientID(getGoogleClientID()),
			providers.WithGoogleClientSecret(getGoogleClientSecret()),
		)
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "google-drive":
		p := providers.NewDriveProvider(
			providers.WithGoogleClientID(getGoogleClientID()),
			providers.WithGoogleClientSecret(getGoogleClientSecret()),
		)
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "google", "google-sheets", "google-docs", "google-slides",
		"google-contacts", "google-tasks", "google-people":
		p := providers.NewGoogleProvider(
			providers.WithGoogleClientID(getGoogleClientID()),
			providers.WithGoogleClientSecret(getGoogleClientSecret()),
			providers.WithGoogleService(getServiceFromProvider(flow.provider)),
			providers.WithGoogleName(flow.provider),
		)
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	default:
		err = fmt.Errorf("unknown provider: %s", flow.provider)
	}

	if err != nil {
		// Send error response
		select {
		case flow.result <- oauthFlowResult{err: err}:
		default:
		}

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<html><body><h1>Authentication Failed</h1><p>%s</p></body></html>`, err.Error())
		return
	}

	// Save credential
	if err := h.tokenManager.SaveCredential(cred); err != nil {
		select {
		case flow.result <- oauthFlowResult{err: err}:
		default:
		}

		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `<html><body><h1>Error</h1><p>Failed to save credential: %s</p></body></html>`, err.Error())
		return
	}

	// Cleanup flow
	h.flowsMu.Lock()
	delete(h.flows, state)
	h.flowsMu.Unlock()

	// Send success result
	select {
	case flow.result <- oauthFlowResult{cred: cred}:
	default:
	}

	// Success page
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>Authentication Successful</title>
	<style>
		body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
		.container { text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
		h1 { color: #22c55e; margin-bottom: 0.5rem; }
		p { color: #666; }
	</style>
</head>
<body>
	<div class="container">
		<h1>✓ Authentication Successful</h1>
		<p>You can close this window and return to DevClaw.</p>
	</div>
	<script>
		if (window.opener) {
			window.opener.postMessage({ type: 'oauth-success', provider: '`+flow.provider+`' }, '*');
		}
	</script>
</body>
</html>`)
}

// handleOAuthRefresh manually refreshes a token.
func (h *OAuthHandlers) handleOAuthRefresh(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/refresh/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	cred, err := h.tokenManager.Refresh(provider)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to refresh token: "+err.Error())
		return
	}

	writeOAuthJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   cred.Provider,
		"email":      cred.Email,
		"expires_at": cred.ExpiresAt,
	})
}

// handleOAuthLogout removes OAuth credentials.
func (h *OAuthHandlers) handleOAuthLogout(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/logout/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	if err := h.tokenManager.DeleteCredential(provider); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to logout: "+err.Error())
		return
	}

	writeOAuthJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// cleanupExpiredFlows removes expired OAuth flows.
func (h *OAuthHandlers) cleanupExpiredFlows() {
	h.flowsMu.Lock()
	defer h.flowsMu.Unlock()

	now := time.Now()
	for state, flow := range h.flows {
		if flow.expiresAt.Before(now) {
			delete(h.flows, state)
		}
	}
}

// Helper functions

func generateState() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// writeOAuthJSON writes JSON response (renamed to avoid conflict with server.go)
func writeOAuthJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeOAuthError(w http.ResponseWriter, status int, message string) {
	writeOAuthJSON(w, status, map[string]string{"error": message})
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

// GetDataDir returns the default data directory.
func GetDataDir() (string, error) {
	return "./data", nil
}
