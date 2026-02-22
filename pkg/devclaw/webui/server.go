// Package webui implements the DevClaw web dashboard.
// Serves a React SPA (embedded via embed.FS) with a JSON API backend.
// Chat streaming uses Server-Sent Events (SSE) for real-time token delivery.
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// StreamEvent is a typed SSE event sent to the frontend.
type StreamEvent struct {
	Type string `json:"type"` // delta, tool_use, tool_result, done, error
	Data any    `json:"data"`
}

// RunHandle represents an active agent run that can stream events and be aborted.
type RunHandle struct {
	RunID     string
	SessionID string
	Events    chan StreamEvent // Backend pushes events here; handler writes SSE.
	Cancel    context.CancelFunc
}

// AssistantAPI defines the interface the web UI uses to access assistant state.
// This avoids a direct dependency on the copilot package.
type AssistantAPI interface {
	// GetConfig returns the current config as a map.
	GetConfigMap() map[string]any

	// UpdateConfigMap updates config fields and persists to disk.
	UpdateConfigMap(updates map[string]any) error

	// ListSessions returns active session metadata.
	ListSessions() []SessionInfo

	// GetSessionMessages returns messages for a session.
	GetSessionMessages(sessionID string) []MessageInfo

	// GetUsageGlobal returns global token usage stats.
	GetUsageGlobal() UsageInfo

	// GetChannelHealth returns health of all channels.
	GetChannelHealth() []ChannelHealthInfo

	// GetSchedulerJobs returns all scheduler jobs.
	GetSchedulerJobs() []JobInfo

	// ListSkills returns available skills.
	ListSkills() []SkillInfo

	// ToggleSkill enables or disables a skill by name.
	ToggleSkill(name string, enabled bool) error

	// SendChatMessage sends a message and blocks until the full response is ready.
	// Used as fallback when streaming is not available.
	SendChatMessage(sessionID, content string) (string, error)

	// StartChatStream starts an agent run with streaming.
	// Returns a RunHandle with an event channel and cancel function.
	// The caller is responsible for reading from Events until it's closed.
	StartChatStream(ctx context.Context, sessionID, content string) (*RunHandle, error)

	// AbortRun cancels an active agent run by session ID.
	AbortRun(sessionID string) bool

	// DeleteSession removes a session.
	DeleteSession(sessionID string) error

	// Security
	GetAuditLog(limit int) []AuditEntry
	GetAuditCount() int
	GetToolGuardStatus() ToolGuardStatus
	UpdateToolGuard(update ToolGuardStatus) error
	GetVaultStatus() VaultStatus
	GetSecurityStatus() SecurityStatus

	// Domain & Network
	GetDomainConfig() DomainConfigInfo
	UpdateDomainConfig(update DomainConfigUpdate) error

	// Webhooks
	ListWebhooks() []WebhookInfo
	CreateWebhook(url string, events []string) (WebhookInfo, error)
	DeleteWebhook(id string) error
	ToggleWebhook(id string, active bool) error
	GetValidWebhookEvents() []string

	// Hooks (lifecycle)
	ListHooks() []HookInfo
	ToggleHook(name string, enabled bool) error
	UnregisterHook(name string) error
	GetHookEvents() []HookEventInfo

	// MCP Servers
	ListMCPServers() []MCPServerInfo
	CreateMCPServer(name, command string, args []string, env map[string]string) error
	UpdateMCPServer(name string, enabled bool) error
	DeleteMCPServer(name string) error
	StartMCPServer(name string) error
	StopMCPServer(name string) error

	// Database
	GetDatabaseStatus() DatabaseStatusInfo
}

// SessionInfo contains session metadata for the UI.
type SessionInfo struct {
	ID            string    `json:"id"`
	Channel       string    `json:"channel"`
	ChatID        string    `json:"chat_id"`
	MessageCount  int       `json:"message_count"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// MessageInfo contains a single message for display.
type MessageInfo struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// UsageInfo contains token usage statistics.
type UsageInfo struct {
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCost         float64 `json:"total_cost"`
	RequestCount      int64   `json:"request_count"`
}

// ChannelHealthInfo contains channel health for display.
type ChannelHealthInfo struct {
	Name       string    `json:"name"`
	Connected  bool      `json:"connected"`
	ErrorCount int       `json:"error_count"`
	LastMsgAt  time.Time `json:"last_msg_at"`
}

// JobInfo contains scheduler job info for display.
type JobInfo struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Type      string    `json:"type"`
	Command   string    `json:"command"`
	Enabled   bool      `json:"enabled"`
	RunCount  int       `json:"run_count"`
	LastRunAt time.Time `json:"last_run_at"`
	LastError string    `json:"last_error"`
}

// SkillInfo contains skill info for display.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	ToolCount   int    `json:"tool_count"`
}

// HookInfo contains lifecycle hook metadata for the UI.
type HookInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Events      []string `json:"events"`
	Priority    int      `json:"priority"`
	Enabled     bool     `json:"enabled"`
}

// HookEventInfo describes a supported hook event.
type HookEventInfo struct {
	Event       string   `json:"event"`
	Description string   `json:"description"`
	Hooks       []string `json:"hooks"` // names of hooks subscribed to this event
}

// MCPServerInfo contains MCP server info for the UI.
type MCPServerInfo struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
	Status  string            `json:"status"` // running, stopped, error
	Error   string            `json:"error,omitempty"`
}

// DatabaseStatusInfo contains database health status for the UI.
type DatabaseStatusInfo struct {
	Name           string `json:"name"`
	Healthy        bool   `json:"healthy"`
	Latency        int64  `json:"latency"` // ms
	Version        string `json:"version"`
	OpenConns      int    `json:"open_connections"`
	InUse          int    `json:"in_use"`
	Idle           int    `json:"idle"`
	WaitCount      int    `json:"wait_count"`
	WaitDuration   int64  `json:"wait_duration"` // ms
	MaxOpenConns   int    `json:"max_open_conns"`
	Error          string `json:"error,omitempty"`
}

// WebhookInfo contains webhook metadata for the UI.
type WebhookInfo struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// DomainConfigInfo contains domain/network configuration for the UI.
type DomainConfigInfo struct {
	// WebUI settings
	WebuiAddress   string `json:"webui_address"`
	WebuiAuthToken bool   `json:"webui_auth_configured"` // never expose the actual token

	// Gateway API settings
	GatewayEnabled   bool     `json:"gateway_enabled"`
	GatewayAddress   string   `json:"gateway_address"`
	GatewayAuthToken bool     `json:"gateway_auth_configured"`
	CORSOrigins      []string `json:"cors_origins"`

	// Tailscale settings
	TailscaleEnabled  bool   `json:"tailscale_enabled"`
	TailscaleServe    bool   `json:"tailscale_serve"`
	TailscaleFunnel   bool   `json:"tailscale_funnel"`
	TailscalePort     int    `json:"tailscale_port"`
	TailscaleHostname string `json:"tailscale_hostname"`
	TailscaleURL      string `json:"tailscale_url"` // resolved URL if active

	// Computed URLs
	WebuiURL   string `json:"webui_url"`
	GatewayURL string `json:"gateway_url"`
	PublicURL  string `json:"public_url"` // tailscale funnel URL if active
}

// DomainConfigUpdate contains the mutable domain/network fields from the UI.
type DomainConfigUpdate struct {
	WebuiAddress     *string  `json:"webui_address,omitempty"`
	WebuiAuthToken   *string  `json:"webui_auth_token,omitempty"`
	GatewayEnabled   *bool    `json:"gateway_enabled,omitempty"`
	GatewayAddress   *string  `json:"gateway_address,omitempty"`
	GatewayAuthToken *string  `json:"gateway_auth_token,omitempty"`
	CORSOrigins      []string `json:"cors_origins,omitempty"`
	TailscaleEnabled *bool    `json:"tailscale_enabled,omitempty"`
	TailscaleServe   *bool    `json:"tailscale_serve,omitempty"`
	TailscaleFunnel  *bool    `json:"tailscale_funnel,omitempty"`
	TailscalePort    *int     `json:"tailscale_port,omitempty"`
}

// Config holds web UI configuration.
type Config struct {
	// Enabled turns the web UI on/off.
	Enabled bool `yaml:"enabled"`

	// Address is the listen address (default: ":8090").
	Address string `yaml:"address"`

	// AuthToken is the Bearer token for authentication (empty = no auth).
	AuthToken string `yaml:"auth_token"`
}

// Server is the web UI HTTP server.
type Server struct {
	cfg    Config
	api    AssistantAPI
	logger *slog.Logger
	server *http.Server

	// activeStreams tracks SSE connections waiting for events by runID.
	activeStreams   map[string]*RunHandle
	activeStreamMu sync.Mutex

	// setupMode is true when the server runs without a full config (setup wizard only).
	setupMode bool

	// onSetupDone is called when the setup wizard completes (optional callback).
	onSetupDone func()

	// onVaultInit is called during setup finalize to create the encrypted vault.
	// Receives (masterPassword, secrets map[name]value) and returns error.
	onVaultInit func(password string, secrets map[string]string) error

	// mediaAPI provides media upload/download operations (optional).
	mediaAPI MediaAPI
}

// New creates a new web UI server.
func New(cfg Config, api AssistantAPI, logger *slog.Logger) *Server {
	if cfg.Address == "" {
		cfg.Address = ":8090"
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		cfg:            cfg,
		api:            api,
		logger:         logger.With("component", "webui"),
		activeStreams:   make(map[string]*RunHandle),
	}
}

// SetSetupMode enables setup-only mode (no assistant, only setup + auth endpoints).
func (s *Server) SetSetupMode(enabled bool) { s.setupMode = enabled }

// OnSetupDone registers a callback invoked when the setup wizard finishes.
func (s *Server) OnSetupDone(fn func()) { s.onSetupDone = fn }

// OnVaultInit registers a callback to create the encrypted vault during setup.
func (s *Server) OnVaultInit(fn func(password string, secrets map[string]string) error) {
	s.onVaultInit = fn
}

// SetMediaAPI sets the media API for file upload/download operations.
func (s *Server) SetMediaAPI(api MediaAPI) {
	s.mediaAPI = api
}

// Start begins serving the web UI.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// ── Public routes (no auth required) ──
	mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("/api/setup/", s.handleAPISetup)

	// ── Protected routes (require auth, require assistant) ──
	mux.HandleFunc("/api/dashboard", s.authMiddleware(s.requireAssistant(s.handleAPIDashboard)))
	mux.HandleFunc("/api/sessions", s.authMiddleware(s.requireAssistant(s.handleAPISessions)))
	mux.HandleFunc("/api/sessions/", s.authMiddleware(s.requireAssistant(s.handleAPISessionDetail)))
	mux.HandleFunc("/api/skills", s.authMiddleware(s.requireAssistant(s.handleAPISkills)))
	mux.HandleFunc("/api/skills/", s.authMiddleware(s.requireAssistant(s.handleAPISkillsAction)))
	mux.HandleFunc("/api/channels", s.authMiddleware(s.requireAssistant(s.handleAPIChannels)))
	mux.HandleFunc("/api/channels/whatsapp/", s.authMiddleware(s.requireAssistant(s.handleAPIWhatsAppQR)))
	mux.HandleFunc("/api/config", s.authMiddleware(s.requireAssistant(s.handleAPIConfig)))
	mux.HandleFunc("/api/domain", s.authMiddleware(s.requireAssistant(s.handleAPIDomain)))
	mux.HandleFunc("/api/webhooks", s.authMiddleware(s.requireAssistant(s.handleAPIWebhooks)))
	mux.HandleFunc("/api/webhooks/", s.authMiddleware(s.requireAssistant(s.handleAPIWebhookByID)))
	mux.HandleFunc("/api/hooks", s.authMiddleware(s.requireAssistant(s.handleAPIHooks)))
	mux.HandleFunc("/api/hooks/", s.authMiddleware(s.requireAssistant(s.handleAPIHookByName)))
	mux.HandleFunc("/api/usage", s.authMiddleware(s.requireAssistant(s.handleAPIUsage)))
	mux.HandleFunc("/api/jobs", s.authMiddleware(s.requireAssistant(s.handleAPIJobs)))
	mux.HandleFunc("/api/security/", s.authMiddleware(s.requireAssistant(s.handleAPISecurity)))
	mux.HandleFunc("/api/security", s.authMiddleware(s.requireAssistant(s.handleAPISecurity)))
	mux.HandleFunc("/api/chat/", s.authMiddleware(s.requireAssistant(s.handleAPIChat)))

	// MCP Servers
	mux.HandleFunc("/api/mcp/servers", s.authMiddleware(s.requireAssistant(s.handleAPIMCPServers)))
	mux.HandleFunc("/api/mcp/servers/", s.authMiddleware(s.requireAssistant(s.handleAPIMCPServerByName)))

	// Database
	mux.HandleFunc("/api/database/status", s.authMiddleware(s.requireAssistant(s.handleAPIDatabaseStatus)))

	// Media routes (if media service is configured)
	if s.mediaAPI != nil {
		mux.HandleFunc("/api/media", s.authMiddleware(s.requireAssistant(s.handleAPIMedia)))
		mux.HandleFunc("/api/media/", s.authMiddleware(s.requireAssistant(s.handleAPIMediaByID)))
	}

	// ── SPA (React) fallback ──
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		s.logger.Warn("SPA dist not found, serving API only", "error", err)
	} else {
		mux.Handle("/", newSPAFileServer(sub))
	}

	s.server = &http.Server{
		Addr:         s.cfg.Address,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disabled for SSE streams (long-lived connections)
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Info("web UI starting", "address", s.cfg.Address)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("web UI server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the web UI server.
func (s *Server) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
		s.logger.Info("web UI stopped")
	}
}

// registerRun stores a run handle so the SSE endpoint can find it.
func (s *Server) registerRun(handle *RunHandle) {
	s.activeStreamMu.Lock()
	s.activeStreams[handle.RunID] = handle
	s.activeStreamMu.Unlock()
}

// unregisterRun removes a run handle.
func (s *Server) unregisterRun(runID string) {
	s.activeStreamMu.Lock()
	delete(s.activeStreams, runID)
	s.activeStreamMu.Unlock()
}

// getRun looks up an active run by ID.
func (s *Server) getRun(runID string) *RunHandle {
	s.activeStreamMu.Lock()
	defer s.activeStreamMu.Unlock()
	return s.activeStreams[runID]
}

// ── Middleware ──

// authMiddleware validates the bearer token if configured.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthToken == "" {
			next(w, r)
			return
		}

		token := extractToken(r)
		if !compareTokens(token, s.cfg.AuthToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return
		}

		next(w, r)
	}
}

// requireAssistant rejects requests when the server is in setup-only mode.
func (s *Server) requireAssistant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.setupMode || s.api == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "server is in setup mode — complete the setup wizard first",
			})
			return
		}
		next(w, r)
	}
}

// corsMiddleware adds CORS headers for development (Vite dev server on :3000).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── JSON helpers ──

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeSSE writes a named SSE event to the response writer.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
	flusher.Flush()
}

// ── Template helpers (kept for backward compat) ──

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen-3] + "..."
}
