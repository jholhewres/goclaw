// Package copilot – daemon_manager.go implements a process manager that lets
// the agent start, monitor, and control long-running background processes
// (dev servers, watchers, database engines, etc.) with ring-buffer output
// capture and health checking.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultRingSize = 500
	healthCheckFreq = 30 * time.Second
)

// Daemon represents a managed background process.
type Daemon struct {
	Label       string    `json:"label"`
	Command     string    `json:"command"`
	PID         int       `json:"pid"`
	Port        int       `json:"port,omitempty"`
	Status      string    `json:"status"` // running, stopped, failed
	StartedAt   time.Time `json:"started_at"`
	ExitCode    int       `json:"exit_code,omitempty"`
	Error       string    `json:"error,omitempty"`

	cmd        *exec.Cmd
	ringBuffer *ringBuffer
	cancel     context.CancelFunc
	done       chan struct{}
}

// DaemonManager manages a set of background daemons.
type DaemonManager struct {
	mu      sync.RWMutex
	daemons map[string]*Daemon
	stopCh  chan struct{}
}

// NewDaemonManager creates a new daemon manager.
func NewDaemonManager() *DaemonManager {
	dm := &DaemonManager{
		daemons: make(map[string]*Daemon),
		stopCh:  make(chan struct{}),
	}
	go dm.healthLoop()
	return dm
}

// StartDaemon starts a new background process.
func (dm *DaemonManager) StartDaemon(label, command string, port int, readyPattern string) (*Daemon, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if existing, ok := dm.daemons[label]; ok {
		if existing.Status == "running" {
			return nil, fmt.Errorf("daemon %q already running (PID %d)", label, existing.PID)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	rb := newRingBuffer(defaultRingSize)
	cmd.Stdout = rb
	cmd.Stderr = rb

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting daemon %q: %w", label, err)
	}

	d := &Daemon{
		Label:      label,
		Command:    command,
		PID:        cmd.Process.Pid,
		Port:       port,
		Status:     "running",
		StartedAt:  time.Now(),
		cmd:        cmd,
		ringBuffer: rb,
		cancel:     cancel,
		done:       make(chan struct{}),
	}

	// Wait for process exit in background.
	go func() {
		err := cmd.Wait()
		dm.mu.Lock()
		defer dm.mu.Unlock()
		d.Status = "stopped"
		if err != nil {
			d.Status = "failed"
			d.Error = err.Error()
		}
		if cmd.ProcessState != nil {
			d.ExitCode = cmd.ProcessState.ExitCode()
		}
		close(d.done)
	}()

	// Wait for ready pattern if specified.
	if readyPattern != "" {
		re, err := regexp.Compile(readyPattern)
		if err == nil {
			deadline := time.After(30 * time.Second)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
		waitLoop:
			for {
				select {
				case <-deadline:
					break waitLoop
				case <-ticker.C:
					if re.MatchString(rb.String()) {
						break waitLoop
					}
				case <-d.done:
					break waitLoop
				}
			}
		}
	}

	dm.daemons[label] = d
	return d, nil
}

// StopDaemon gracefully stops a daemon (SIGTERM). If force is true, uses SIGKILL.
func (dm *DaemonManager) StopDaemon(label string, force bool) error {
	dm.mu.Lock()
	d, ok := dm.daemons[label]
	dm.mu.Unlock()

	if !ok {
		return fmt.Errorf("daemon %q not found", label)
	}
	if d.Status != "running" {
		return fmt.Errorf("daemon %q is not running (status: %s)", label, d.Status)
	}

	if force {
		if d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
		}
	} else {
		d.cancel()
	}

	select {
	case <-d.done:
	case <-time.After(10 * time.Second):
		if d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
		}
	}

	return nil
}

// RestartDaemon stops and re-starts a daemon with the same config.
func (dm *DaemonManager) RestartDaemon(label string) (*Daemon, error) {
	dm.mu.RLock()
	d, ok := dm.daemons[label]
	dm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("daemon %q not found", label)
	}

	cmd := d.Command
	port := d.Port

	if d.Status == "running" {
		if err := dm.StopDaemon(label, false); err != nil {
			return nil, fmt.Errorf("stopping daemon for restart: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return dm.StartDaemon(label, cmd, port, "")
}

// GetLogs returns the last n lines from a daemon's output ring buffer.
func (dm *DaemonManager) GetLogs(label string, n int, filter string) (string, error) {
	dm.mu.RLock()
	d, ok := dm.daemons[label]
	dm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("daemon %q not found", label)
	}

	lines := d.ringBuffer.Lines()
	if n > 0 && n < len(lines) {
		lines = lines[len(lines)-n:]
	}

	if filter != "" {
		re, err := regexp.Compile(filter)
		if err != nil {
			return "", fmt.Errorf("invalid filter regex: %w", err)
		}
		var filtered []string
		for _, line := range lines {
			if re.MatchString(line) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}

	return strings.Join(lines, "\n"), nil
}

// List returns info about all managed daemons.
func (dm *DaemonManager) List() []Daemon {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]Daemon, 0, len(dm.daemons))
	for _, d := range dm.daemons {
		result = append(result, Daemon{
			Label:     d.Label,
			Command:   d.Command,
			PID:       d.PID,
			Port:      d.Port,
			Status:    d.Status,
			StartedAt: d.StartedAt,
			ExitCode:  d.ExitCode,
			Error:     d.Error,
		})
	}
	return result
}

// Shutdown stops all running daemons.
func (dm *DaemonManager) Shutdown() {
	close(dm.stopCh)
	dm.mu.RLock()
	labels := make([]string, 0)
	for label, d := range dm.daemons {
		if d.Status == "running" {
			labels = append(labels, label)
		}
	}
	dm.mu.RUnlock()

	for _, label := range labels {
		_ = dm.StopDaemon(label, false)
	}
}

func (dm *DaemonManager) healthLoop() {
	ticker := time.NewTicker(healthCheckFreq)
	defer ticker.Stop()

	for {
		select {
		case <-dm.stopCh:
			return
		case <-ticker.C:
			dm.cleanupDead()
		}
	}
}

func (dm *DaemonManager) cleanupDead() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for _, d := range dm.daemons {
		if d.Status == "running" && d.cmd.ProcessState != nil {
			d.Status = "stopped"
			d.ExitCode = d.cmd.ProcessState.ExitCode()
		}
	}
}

// ---------- Ring Buffer ----------

type ringBuffer struct {
	mu       sync.Mutex
	lines    []string
	maxLines int
	partial  strings.Builder
}

func newRingBuffer(maxLines int) *ringBuffer {
	return &ringBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.partial.Write(p)
	text := rb.partial.String()

	for {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			break
		}
		line := text[:idx]
		text = text[idx+1:]
		rb.lines = append(rb.lines, line)
		if len(rb.lines) > rb.maxLines {
			rb.lines = rb.lines[1:]
		}
	}

	rb.partial.Reset()
	rb.partial.WriteString(text)

	return len(p), nil
}

func (rb *ringBuffer) Lines() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	result := make([]string, len(rb.lines))
	copy(result, rb.lines)
	return result
}

func (rb *ringBuffer) String() string {
	return strings.Join(rb.Lines(), "\n")
}

// Ensure ringBuffer implements io.Writer.
var _ io.Writer = (*ringBuffer)(nil)

// ---------- Tool Registration ----------

// RegisterDaemonTools registers daemon management tools in the executor.
func RegisterDaemonTools(executor *ToolExecutor, dm *DaemonManager) {
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "start_daemon",
			Description: "Start a background process (dev server, watcher, database, etc.) and manage its lifecycle. Returns PID and status.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":       map[string]any{"type": "string", "description": "Shell command to run (e.g. 'npm run dev', 'python manage.py runserver')"},
					"label":         map[string]any{"type": "string", "description": "Unique label for this daemon (e.g. 'frontend', 'api', 'db')"},
					"port":          map[string]any{"type": "integer", "description": "Port the daemon listens on (for health checks)"},
					"ready_pattern": map[string]any{"type": "string", "description": "Regex pattern in stdout that indicates the daemon is ready (e.g. 'listening on port')"},
				},
				"required": []string{"command", "label"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		command, _ := args["command"].(string)
		label, _ := args["label"].(string)
		port, _ := args["port"].(float64)
		readyPattern, _ := args["ready_pattern"].(string)

		d, err := dm.StartDaemon(label, command, int(port), readyPattern)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("Daemon %q started (PID %d, port %d, status: %s)", d.Label, d.PID, d.Port, d.Status), nil
	})

	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "daemon_logs",
			Description: "Get the last N lines of output from a running daemon. Supports regex filtering.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label":  map[string]any{"type": "string", "description": "Daemon label"},
					"lines":  map[string]any{"type": "integer", "description": "Number of lines to return (default: 50)"},
					"filter": map[string]any{"type": "string", "description": "Regex filter to match lines"},
				},
				"required": []string{"label"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		label, _ := args["label"].(string)
		n := 50
		if v, ok := args["lines"].(float64); ok {
			n = int(v)
		}
		filter, _ := args["filter"].(string)
		return dm.GetLogs(label, n, filter)
	})

	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "daemon_list",
			Description: "List all managed daemons with their PID, status, port, and uptime.",
			Parameters: mustJSON(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		daemons := dm.List()
		if len(daemons) == 0 {
			return "No daemons running.", nil
		}
		data, _ := json.MarshalIndent(daemons, "", "  ")
		return string(data), nil
	})

	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "daemon_stop",
			Description: "Stop a running daemon. Uses graceful shutdown by default; set force=true for immediate kill.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label": map[string]any{"type": "string", "description": "Daemon label"},
					"force": map[string]any{"type": "boolean", "description": "Force kill (SIGKILL) instead of graceful stop"},
				},
				"required": []string{"label"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		label, _ := args["label"].(string)
		force, _ := args["force"].(bool)
		if err := dm.StopDaemon(label, force); err != nil {
			return nil, err
		}
		return fmt.Sprintf("Daemon %q stopped.", label), nil
	})

	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "daemon_restart",
			Description: "Restart a daemon with the same configuration. Stops the old process and starts a new one.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label": map[string]any{"type": "string", "description": "Daemon label"},
				},
				"required": []string{"label"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		label, _ := args["label"].(string)
		d, err := dm.RestartDaemon(label)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("Daemon %q restarted (new PID %d, status: %s)", d.Label, d.PID, d.Status), nil
	})
}

// mustJSON marshals a value to json.RawMessage, panicking on error.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return b
}
