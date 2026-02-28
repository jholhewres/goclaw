package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// HubAdapter implements OAuthManager by delegating to an OAuth Hub instance.
// It maps DevClaw provider names (e.g. "google-gmail") to Hub connection IDs.
type HubAdapter struct {
	mu     sync.RWMutex
	hubURL string
	apiKey string
	client *http.Client
	logger *slog.Logger

	// connectionCache maps provider names to connection IDs
	connectionCache map[string]string
	cacheExpiry     time.Time

	// tokenCache stores recently fetched access tokens
	tokenCache map[string]*cachedToken
}

type cachedToken struct {
	cred      *OAuthCredential
	expiresAt time.Time
}

// HubAdapterConfig configures the Hub adapter.
type HubAdapterConfig struct {
	HubURL      string
	APIKey      string
	APIKeyEnvVar string
	Logger      *slog.Logger
}

// NewHubAdapter creates a new OAuth Hub adapter.
func NewHubAdapter(cfg HubAdapterConfig) (*HubAdapter, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		envVar := cfg.APIKeyEnvVar
		if envVar == "" {
			envVar = "OAUTH_HUB_API_KEY"
		}
		apiKey = os.Getenv(envVar)
	}

	if cfg.HubURL == "" {
		return nil, fmt.Errorf("oauth hub URL is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("oauth hub API key is required (set %s or configure api_key)", cfg.APIKeyEnvVar)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &HubAdapter{
		hubURL: strings.TrimRight(cfg.HubURL, "/"),
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:          logger.With("component", "oauth-hub-adapter"),
		connectionCache: make(map[string]string),
		tokenCache:      make(map[string]*cachedToken),
	}, nil
}

// GetValidToken returns a valid access token for the given provider.
// It calls the Hub's /api/v1/token/get endpoint.
func (h *HubAdapter) GetValidToken(provider string) (*OAuthCredential, error) {
	h.mu.RLock()
	if cached, ok := h.tokenCache[provider]; ok {
		if time.Now().Before(cached.expiresAt.Add(-5 * time.Minute)) {
			h.mu.RUnlock()
			return cached.cred, nil
		}
	}
	h.mu.RUnlock()

	// Resolve provider to connection ID
	connectionID, err := h.resolveConnectionID(provider)
	if err != nil {
		return nil, fmt.Errorf("no hub connection for provider %s: %w", provider, err)
	}

	// Request token from Hub
	body := fmt.Sprintf(`{"connection_id":%q}`, connectionID)
	resp, err := h.doRequest("POST", "/api/v1/token/get", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("hub token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, h.readError(resp)
	}

	var tokenResp hubTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode hub token response: %w", err)
	}

	cred := &OAuthCredential{
		Provider:    provider,
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	// Cache the token
	h.mu.Lock()
	h.tokenCache[provider] = &cachedToken{
		cred:      cred,
		expiresAt: cred.ExpiresAt,
	}
	h.mu.Unlock()

	h.logger.Debug("obtained token from hub",
		"provider", provider,
		"connection_id", connectionID,
		"expires_in", tokenResp.ExpiresIn)

	return cred, nil
}

// SaveCredential is a no-op for Hub mode — the Hub manages token storage.
func (h *HubAdapter) SaveCredential(_ *OAuthCredential) error {
	return nil
}

// resolveConnectionID maps a DevClaw provider name to a Hub connection ID.
func (h *HubAdapter) resolveConnectionID(provider string) (string, error) {
	h.mu.RLock()
	if id, ok := h.connectionCache[provider]; ok && time.Now().Before(h.cacheExpiry) {
		h.mu.RUnlock()
		return id, nil
	}
	h.mu.RUnlock()

	// Refresh connections from Hub
	if err := h.refreshConnections(); err != nil {
		return "", err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if id, ok := h.connectionCache[provider]; ok {
		return id, nil
	}

	return "", fmt.Errorf("no active connection found for provider %q in hub", provider)
}

// refreshConnections fetches the connection list from the Hub.
func (h *HubAdapter) refreshConnections() error {
	resp, err := h.doRequest("GET", "/api/v1/connections", nil)
	if err != nil {
		return fmt.Errorf("failed to list hub connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return h.readError(resp)
	}

	var connections []hubConnection
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return fmt.Errorf("failed to decode hub connections: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Clear old cache
	h.connectionCache = make(map[string]string)

	for _, conn := range connections {
		if conn.Status != "connected" {
			continue
		}

		// Map Hub connection to DevClaw provider names.
		// Hub uses "google" provider + "gmail" service → DevClaw uses "google-gmail"
		providerKey := conn.Provider + "-" + conn.Service
		h.connectionCache[providerKey] = conn.ID

		// Also store by service name alone for convenience
		h.connectionCache[conn.Service] = conn.ID

		// Also store full provider name for LLM OAuth providers
		// (e.g. "gemini", "chatgpt" are stored as-is in Hub)
		if conn.Service == conn.Provider {
			h.connectionCache[conn.Provider] = conn.ID
		}
	}

	h.cacheExpiry = time.Now().Add(2 * time.Minute)
	return nil
}

// doRequest performs an authenticated HTTP request to the Hub.
func (h *HubAdapter) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := h.hubURL + path

	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return h.client.Do(req)
}

// readError extracts an error message from a non-200 Hub response.
func (h *HubAdapter) readError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var errBody struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &errBody) == nil && errBody.Error != "" {
		return fmt.Errorf("hub error (%d): %s", resp.StatusCode, errBody.Error)
	}
	return fmt.Errorf("hub error (%d): %s", resp.StatusCode, string(data))
}

// Hub API response types

type hubTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type hubConnection struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Service   string `json:"service"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	Scopes    string `json:"scopes"`
}
