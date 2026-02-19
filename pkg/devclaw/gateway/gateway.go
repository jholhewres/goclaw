// Package gateway provides an HTTP API gateway for DevClaw.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
)

// Gateway is the HTTP API gateway.
type Gateway struct {
	assistant   *copilot.Assistant
	config      copilot.GatewayConfig
	server      *http.Server
	logger      *slog.Logger
	startedAt   time.Time
	webhooks    []WebhookEntry
	webhooksMu  sync.Mutex
	webhookSeq  int
}

// WebhookEntry represents a registered outgoing webhook.
type WebhookEntry struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
}

// New creates a new Gateway.
func New(assistant *copilot.Assistant, cfg copilot.GatewayConfig, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Address == "" {
		cfg.Address = ":8085"
	}
	return &Gateway{
		assistant: assistant,
		config:   cfg,
		logger:   logger.With("component", "gateway"),
		webhooks: make([]WebhookEntry, 0),
	}
}

// Start starts the HTTP server.
func (g *Gateway) Start(ctx context.Context) error {
	g.startedAt = time.Now()
	mux := http.NewServeMux()

	// Health (always public)
	mux.HandleFunc("/health", g.handleHealth)

	// OpenAI-compatible chat
	mux.HandleFunc("/v1/chat/completions", g.handleChatCompletions)

	// API routes
	mux.HandleFunc("/api/sessions", g.handleListSessions)
	mux.HandleFunc("/api/sessions/", g.handleSessionByID)
	mux.HandleFunc("/api/usage", g.handleGlobalUsage)
	mux.HandleFunc("/api/usage/", g.handleSessionUsage)
	mux.HandleFunc("/api/status", g.handleStatus)
	mux.HandleFunc("/api/webhooks", g.handleWebhooks)
	mux.HandleFunc("/api/webhooks/", g.handleWebhookByID)

	handler := g.securityHeadersMiddleware(g.corsMiddleware(g.authMiddleware(mux)))
	g.server = &http.Server{
		Addr:    g.config.Address,
		Handler: handler,
	}

	// Warn when the gateway has no auth token and is bound to a non-loopback address.
	if g.config.AuthToken == "" {
		host, _, _ := net.SplitHostPort(g.config.Address)
		if host == "" {
			host = "0.0.0.0"
		}
		ip := net.ParseIP(host)
		isLoopback := ip != nil && ip.IsLoopback()
		isLocalName := host == "localhost"
		if !isLoopback && !isLocalName {
			g.logger.Warn("SECURITY: gateway has no auth token and is bound to a non-loopback address — anyone on the network can access the API",
				"address", g.config.Address)
		}
	}

	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			g.logger.Error("gateway server error", "error", err)
		}
	}()
	g.logger.Info("gateway started", "address", g.config.Address)
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (g *Gateway) Stop(ctx context.Context) error {
	if g.server == nil {
		return nil
	}
	g.logger.Info("gateway stopping...")
	return g.server.Shutdown(ctx)
}

// ListWebhooks returns all registered webhooks.
func (g *Gateway) ListWebhooks() []WebhookEntry {
	g.webhooksMu.Lock()
	defer g.webhooksMu.Unlock()
	result := make([]WebhookEntry, len(g.webhooks))
	copy(result, g.webhooks)
	return result
}

// AddWebhook registers a new webhook and returns the entry or an error.
// Returns an error if the URL targets a private/loopback address (SSRF guard).
func (g *Gateway) AddWebhook(webhookURL string, events []string) (WebhookEntry, error) {
	if err := validateWebhookURL(webhookURL); err != nil {
		return WebhookEntry{}, err
	}
	g.webhooksMu.Lock()
	defer g.webhooksMu.Unlock()
	g.webhookSeq++
	entry := WebhookEntry{
		ID:        fmt.Sprintf("wh_%d", g.webhookSeq),
		URL:       webhookURL,
		Events:    events,
		CreatedAt: time.Now(),
		Active:    true,
	}
	g.webhooks = append(g.webhooks, entry)
	return entry, nil
}

// securityHeadersMiddleware adds standard security headers to all responses.
func (g *Gateway) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

// validateWebhookURL rejects URLs that target private or loopback addresses
// to prevent Server-Side Request Forgery (SSRF) via outgoing webhooks.
func validateWebhookURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("webhook URL must use http or https scheme")
	}
	hostname := strings.ToLower(parsed.Hostname())
	ip := net.ParseIP(hostname)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("webhook URL targets a private or loopback address: %s", hostname)
		}
	} else {
		// Block well-known internal hostnames.
		for _, blocked := range []string{"localhost", "localhost.localdomain", "metadata.google.internal"} {
			if hostname == blocked {
				return fmt.Errorf("webhook URL targets a reserved hostname: %s", hostname)
			}
		}
	}
	return nil
}

// DeleteWebhook removes a webhook by ID.
func (g *Gateway) DeleteWebhook(id string) bool {
	g.webhooksMu.Lock()
	defer g.webhooksMu.Unlock()
	for i, wh := range g.webhooks {
		if wh.ID == id {
			g.webhooks = append(g.webhooks[:i], g.webhooks[i+1:]...)
			return true
		}
	}
	return false
}

// ToggleWebhook enables or disables a webhook by ID.
func (g *Gateway) ToggleWebhook(id string, active bool) bool {
	g.webhooksMu.Lock()
	defer g.webhooksMu.Unlock()
	for i := range g.webhooks {
		if g.webhooks[i].ID == id {
			g.webhooks[i].Active = active
			return true
		}
	}
	return false
}

// handleSessionByID routes to get, delete, or compact based on method and path.
func (g *Gateway) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if strings.HasSuffix(path, "/compact") {
		g.handleCompactSession(w, r)
		return
	}
	if path == "" {
		g.writeError(w, "session id required", 400)
		return
	}
	switch r.Method {
	case http.MethodGet:
		g.handleGetSessionPath(w, r, path)
	case http.MethodDelete:
		g.handleDeleteSessionPath(w, r, path)
	default:
		g.writeError(w, "method not allowed", 405)
	}
}

func (g *Gateway) handleGetSessionPath(w http.ResponseWriter, r *http.Request, id string) {
	session, ws := g.assistant.WorkspaceManager().GetSessionByID(id)
	if session == nil {
		g.writeError(w, "session not found", 404)
		return
	}
	history := session.RecentHistory(20)
	promptTokens, completionTokens, requests := session.GetTokenUsage()
	g.writeJSON(w, 200, map[string]any{
		"id":             session.ID,
		"channel":        session.Channel,
		"chat_id":        session.ChatID,
		"workspace":      ws.ID,
		"created_at":     session.CreatedAt,
		"last_active_at": session.LastActiveAt(),
		"history_len":    len(history),
		"history":        history,
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"requests":          requests,
		},
	})
}

func (g *Gateway) handleDeleteSessionPath(w http.ResponseWriter, r *http.Request, id string) {
	if !g.assistant.WorkspaceManager().DeleteSessionByID(id) {
		g.writeError(w, "session not found", 404)
		return
	}
	if g.assistant.UsageTracker() != nil {
		g.assistant.UsageTracker().ResetSession(id)
	}
	g.writeJSON(w, 200, map[string]string{"status": "deleted"})
}
