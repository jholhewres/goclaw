// Package copilot – canvas_host.go implements an interactive HTML/JS canvas host.
// Allows the agent to generate
// HTML/JS content and serve it via a temporary local HTTP server for the user
// to interact with.
//
// Use cases:
//   - Data visualization (charts, graphs)
//   - Interactive prototypes
//   - Mini-apps (calculators, forms)
//   - Rich output that exceeds chat formatting
//
// Architecture:
//
//	Agent ──canvas_create──▶ CanvasHost ──HTTP──▶ user browser
//	Agent ──canvas_update──▶ CanvasHost (live-reload via SSE)
//	Agent ──canvas_list──▶ list of active canvases
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// CanvasConfig configures the canvas host.
type CanvasConfig struct {
	// Enabled turns canvas hosting on/off (default: true).
	Enabled bool `yaml:"enabled"`

	// BasePort is the starting port for canvas servers (default: 9100).
	// Each canvas gets its own port: BasePort, BasePort+1, etc.
	BasePort int `yaml:"base_port"`

	// MaxCanvases is the max number of simultaneous canvas servers (default: 5).
	MaxCanvases int `yaml:"max_canvases"`

	// TTLMinutes is how long a canvas stays alive without updates (default: 30).
	TTLMinutes int `yaml:"ttl_minutes"`
}

// DefaultCanvasConfig returns sensible defaults.
func DefaultCanvasConfig() CanvasConfig {
	return CanvasConfig{
		Enabled:     true,
		BasePort:    9100,
		MaxCanvases: 5,
		TTLMinutes:  30,
	}
}

// Canvas represents a single hosted HTML page with live-reload support.
type Canvas struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	HTML      string    `json:"-"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	server   *http.Server
	updateCh chan struct{} // Signal for SSE live-reload.
	mu       sync.RWMutex
}

// CanvasHost manages multiple canvas servers.
type CanvasHost struct {
	cfg      CanvasConfig
	logger   *slog.Logger
	canvases map[string]*Canvas
	nextPort int
	mu       sync.Mutex
}

// NewCanvasHost creates a new canvas host.
func NewCanvasHost(cfg CanvasConfig, logger *slog.Logger) *CanvasHost {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BasePort <= 0 {
		cfg.BasePort = 9100
	}
	if cfg.MaxCanvases <= 0 {
		cfg.MaxCanvases = 5
	}
	if cfg.TTLMinutes <= 0 {
		cfg.TTLMinutes = 30
	}
	return &CanvasHost{
		cfg:      cfg,
		logger:   logger.With("component", "canvas"),
		canvases: make(map[string]*Canvas),
		nextPort: cfg.BasePort,
	}
}

// Create creates and starts a new canvas with the given HTML content.
func (ch *CanvasHost) Create(id, title, html string) (*Canvas, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if len(ch.canvases) >= ch.cfg.MaxCanvases {
		// Evict oldest canvas.
		var oldestID string
		var oldestTime time.Time
		for cid, c := range ch.canvases {
			if oldestID == "" || c.UpdatedAt.Before(oldestTime) {
				oldestID = cid
				oldestTime = c.UpdatedAt
			}
		}
		if oldestID != "" {
			ch.stopCanvasLocked(oldestID)
		}
	}

	if _, exists := ch.canvases[id]; exists {
		ch.stopCanvasLocked(id)
	}

	port := ch.findFreePort()
	canvas := &Canvas{
		ID:        id,
		Title:     title,
		HTML:      html,
		Port:      port,
		URL:       fmt.Sprintf("http://localhost:%d", port),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		updateCh:  make(chan struct{}, 1),
	}

	// Build handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		canvas.mu.RLock()
		content := canvas.HTML
		canvas.mu.RUnlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")

		// Inject live-reload script.
		liveReload := `<script>
const es = new EventSource('/__canvas_events');
es.onmessage = function(e) { if (e.data === 'reload') location.reload(); };
es.onerror = function() { setTimeout(() => location.reload(), 2000); };
</script>`
		fmt.Fprint(w, content+liveReload)
	})

	mux.HandleFunc("/__canvas_events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		for {
			select {
			case <-canvas.updateCh:
				fmt.Fprint(w, "data: reload\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	canvas.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start the server.
	ln, err := net.Listen("tcp", canvas.server.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	go func() {
		if err := canvas.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			ch.logger.Error("canvas server error", "id", id, "error", err)
		}
	}()

	ch.canvases[id] = canvas
	ch.logger.Info("canvas created", "id", id, "port", port, "url", canvas.URL)
	return canvas, nil
}

// Update replaces the HTML content of an existing canvas and triggers live-reload.
func (ch *CanvasHost) Update(id, html string) error {
	ch.mu.Lock()
	canvas, ok := ch.canvases[id]
	if !ok {
		ch.mu.Unlock()
		return fmt.Errorf("canvas %q not found", id)
	}

	canvas.mu.Lock()
	canvas.HTML = html
	canvas.UpdatedAt = time.Now()
	canvas.mu.Unlock()

	// Signal live-reload while holding ch.mu to prevent Stop from closing
	// the channel concurrently (send on closed channel would panic).
	select {
	case canvas.updateCh <- struct{}{}:
	default:
	}
	ch.mu.Unlock()

	ch.logger.Debug("canvas updated", "id", id)
	return nil
}

// List returns metadata for all active canvases.
func (ch *CanvasHost) List() []*Canvas {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	result := make([]*Canvas, 0, len(ch.canvases))
	for _, c := range ch.canvases {
		result = append(result, c)
	}
	return result
}

// Stop stops a specific canvas server.
func (ch *CanvasHost) Stop(id string) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.stopCanvasLocked(id)
}

func (ch *CanvasHost) stopCanvasLocked(id string) error {
	canvas, ok := ch.canvases[id]
	if !ok {
		return fmt.Errorf("canvas %q not found", id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if canvas.server != nil {
		canvas.server.Shutdown(ctx)
	}
	close(canvas.updateCh)
	delete(ch.canvases, id)
	ch.logger.Info("canvas stopped", "id", id)
	return nil
}

// StopAll stops all canvas servers.
func (ch *CanvasHost) StopAll() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for id := range ch.canvases {
		ch.stopCanvasLocked(id)
	}
}

// findFreePort finds the next available port in the [BasePort, BasePort+100] range.
// Falls back to returning the starting port if all are in use (bounded loop).
func (ch *CanvasHost) findFreePort() int {
	start := ch.nextPort
	for {
		port := ch.nextPort
		ch.nextPort++
		if ch.nextPort > ch.cfg.BasePort+100 {
			ch.nextPort = ch.cfg.BasePort
		}
		// Check if port is in use by another canvas.
		inUse := false
		for _, c := range ch.canvases {
			if c.Port == port {
				inUse = true
				break
			}
		}
		if !inUse {
			return port
		}
		// Guard: wrapped around without finding a free port.
		if ch.nextPort == start {
			return port
		}
	}
}

// CleanupStale removes canvases that haven't been updated within TTL.
func (ch *CanvasHost) CleanupStale() int {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(ch.cfg.TTLMinutes) * time.Minute)
	removed := 0

	for id, c := range ch.canvases {
		if c.UpdatedAt.Before(cutoff) {
			ch.stopCanvasLocked(id)
			removed++
		}
	}
	return removed
}

// ─── Tool Registration ───

// RegisterCanvasTools registers canvas tools in the executor.
func RegisterCanvasTools(executor *ToolExecutor, canvasHost *CanvasHost, logger *slog.Logger) {
	if canvasHost == nil {
		return
	}

	// canvas_create
	executor.Register(
		MakeToolDefinition("canvas_create",
			"Create an interactive HTML/JS canvas that the user can view in their browser. "+
				"Use this for data visualizations, interactive prototypes, rich output, or any "+
				"content that benefits from a full HTML page. The canvas supports live-reload — "+
				"use canvas_update to push changes without requiring the user to refresh.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Unique identifier for this canvas (e.g. 'chart-sales', 'prototype-v1').",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Human-readable title shown to the user.",
					},
					"html": map[string]any{
						"type":        "string",
						"description": "Complete HTML content. Can include inline CSS/JS, external CDN libs, etc.",
					},
				},
				"required": []string{"id", "html"},
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			title, _ := args["title"].(string)
			html, _ := args["html"].(string)
			if id == "" || html == "" {
				return nil, fmt.Errorf("id and html are required")
			}
			if title == "" {
				title = id
			}

			canvas, err := canvasHost.Create(id, title, html)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf(
				"Canvas created!\n  ID: %s\n  Title: %s\n  URL: %s\n\n"+
					"The user can open this URL in their browser. Use canvas_update to push changes.",
				canvas.ID, canvas.Title, canvas.URL,
			), nil
		},
	)

	// canvas_update
	executor.Register(
		MakeToolDefinition("canvas_update",
			"Update the HTML content of an existing canvas. Triggers live-reload so "+
				"the user sees changes immediately without refreshing.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The canvas ID to update.",
					},
					"html": map[string]any{
						"type":        "string",
						"description": "New HTML content.",
					},
				},
				"required": []string{"id", "html"},
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			html, _ := args["html"].(string)
			if id == "" || html == "" {
				return nil, fmt.Errorf("id and html are required")
			}
			if err := canvasHost.Update(id, html); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Canvas %q updated. The page will auto-reload.", id), nil
		},
	)

	// canvas_list
	executor.Register(
		MakeToolDefinition("canvas_list",
			"List all active canvas hosts with their URLs and status.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			canvases := canvasHost.List()
			if len(canvases) == 0 {
				return "No active canvases.", nil
			}
			result := fmt.Sprintf("Active canvases (%d):\n", len(canvases))
			for _, c := range canvases {
				ago := time.Since(c.UpdatedAt).Round(time.Second)
				result += fmt.Sprintf("- %s: %s (updated %s ago)\n", c.ID, c.URL, ago)
			}
			return result, nil
		},
	)

	// canvas_stop
	executor.Register(
		MakeToolDefinition("canvas_stop",
			"Stop a canvas server by its ID.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The canvas ID to stop.",
					},
				},
				"required": []string{"id"},
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("id is required")
			}
			if err := canvasHost.Stop(id); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Canvas %q stopped.", id), nil
		},
	)

	logger.Info("canvas tools registered",
		"tools", []string{"canvas_create", "canvas_update", "canvas_list", "canvas_stop"},
	)
}
