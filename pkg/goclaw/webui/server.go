// Package webui implements a web-based dashboard for GoClaw.
// Uses Go templates + HTMX + Tailwind CSS for a reactive server-side UI
// with zero JavaScript build step.
package webui

import (
	"context"
	"crypto/subtle"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

// AssistantAPI defines the interface the web UI uses to access assistant state.
// This avoids a direct dependency on the copilot package.
type AssistantAPI interface {
	// GetConfig returns the current config as a map.
	GetConfigMap() map[string]any

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

	// SendChatMessage sends a message through the webui channel.
	SendChatMessage(sessionID, content string) (string, error)
}

// SessionInfo contains session metadata for the UI.
type SessionInfo struct {
	ID            string
	Channel       string
	ChatID        string
	MessageCount  int
	LastMessageAt time.Time
	CreatedAt     time.Time
}

// MessageInfo contains a single message for display.
type MessageInfo struct {
	Role      string // "user" or "assistant"
	Content   string
	Timestamp time.Time
}

// UsageInfo contains token usage statistics.
type UsageInfo struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCost         float64
	RequestCount      int64
}

// ChannelHealthInfo contains channel health for display.
type ChannelHealthInfo struct {
	Name       string
	Connected  bool
	ErrorCount int
	LastMsgAt  time.Time
}

// JobInfo contains scheduler job info for display.
type JobInfo struct {
	ID        string
	Schedule  string
	Type      string
	Command   string
	Enabled   bool
	RunCount  int
	LastRunAt time.Time
	LastError string
}

// SkillInfo contains skill info for display.
type SkillInfo struct {
	Name        string
	Description string
	Enabled     bool
	ToolCount   int
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
	cfg       Config
	api       AssistantAPI
	logger    *slog.Logger
	templates *template.Template
	server    *http.Server
}

// New creates a new web UI server.
func New(cfg Config, api AssistantAPI, logger *slog.Logger) *Server {
	if cfg.Address == "" {
		cfg.Address = ":8090"
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:    cfg,
		api:    api,
		logger: logger.With("component", "webui"),
	}

	// Parse templates.
	funcMap := template.FuncMap{
		"timeAgo":   timeAgo,
		"truncate":  truncate,
		"hasPrefix": strings.HasPrefix,
		"lower":     strings.ToLower,
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		logger.Error("failed to parse web UI templates", "error", err)
		// Create a minimal fallback.
		tmpl = template.Must(template.New("error").Parse("<html><body>Template error: {{.Error}}</body></html>"))
	}
	s.templates = tmpl

	return s
}

// Start begins serving the web UI.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Static assets (embedded).
	staticFS, err := fs.Sub(templateFS, "templates")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// Page routes.
	mux.HandleFunc("/", s.authMiddleware(s.handleDashboard))
	mux.HandleFunc("/chat", s.authMiddleware(s.handleChat))
	mux.HandleFunc("/sessions", s.authMiddleware(s.handleSessions))
	mux.HandleFunc("/sessions/", s.authMiddleware(s.handleSessionDetail))
	mux.HandleFunc("/config", s.authMiddleware(s.handleConfig))
	mux.HandleFunc("/skills", s.authMiddleware(s.handleSkills))
	mux.HandleFunc("/usage", s.authMiddleware(s.handleUsage))
	mux.HandleFunc("/jobs", s.authMiddleware(s.handleJobs))

	// HTMX API partials.
	mux.HandleFunc("/api/chat/send", s.authMiddleware(s.handleChatSend))

	s.server = &http.Server{
		Addr:         s.cfg.Address,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
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

// authMiddleware validates the bearer token if configured.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthToken == "" {
			next(w, r)
			return
		}

		// Check bearer token in header or query param.
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		// Also check cookie.
		if token == "" {
			if cookie, err := r.Cookie("goclaw_token"); err == nil {
				token = cookie.Value
			}
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AuthToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// renderTemplate renders an HTML template with the given data.
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("template render error", "template", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ---------- Template helpers ----------

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
