// Package copilot – subagent.go implements a subagent system that allows the
// main agent to spawn independent child agents to handle tasks concurrently.
//
// Architecture:
//
//	Main Agent ──spawn_subagent──▶ SubagentManager ──goroutine──▶ Child AgentRun
//	                                   │                              │
//	                                   ▼                              ▼
//	                            SubagentRegistry           (runs with own session,
//	                            tracks runs + results       limited tools, separate
//	                                                        prompt, optional model)
//
// Subagents:
//   - Run in isolated goroutines with their own session context.
//   - Cannot spawn nested subagents (no recursion).
//   - Have a configurable subset of tools (deny list applied).
//   - Results are collected and can be polled or waited on.
//   - Are announced back to the parent session when complete.
package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// contextKeySpawnDepth is the context key for spawn depth tracking.
type contextKeySpawnDepth struct{}

// SpawnDepthFromContext retrieves the current spawn depth from context.
// Returns 0 if not set (main agent level).
func SpawnDepthFromContext(ctx context.Context) int {
	if v := ctx.Value(contextKeySpawnDepth{}); v != nil {
		if depth, ok := v.(int); ok {
			return depth
		}
	}
	return 0
}

// ContextWithSpawnDepth returns a new context with the spawn depth set.
func ContextWithSpawnDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, contextKeySpawnDepth{}, depth)
}

// ─── Configuration ───

// SubagentConfig configures the subagent system.
type SubagentConfig struct {
	// Enabled turns the subagent system on/off (default: true).
	Enabled bool `yaml:"enabled"`

	// MaxConcurrent is the max number of subagents running at the same time.
	MaxConcurrent int `yaml:"max_concurrent"`

	// MaxTurns is the max agent loop turns for each subagent (default: 15).
	MaxTurns int `yaml:"max_turns"`

	// TimeoutSeconds is the max execution time per subagent (default: 300 = 5min).
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// MaxSpawnDepth controls nested subagent spawning (default: 1 = no nesting).
	// Set to 2 to allow subagents to spawn their own children.
	// Higher values allow deeper nesting (e.g., 3 = sub-sub-subagents).
	MaxSpawnDepth int `yaml:"max_spawn_depth"`

	// MaxChildrenPerAgent limits how many children a single agent can spawn (default: 5).
	MaxChildrenPerAgent int `yaml:"max_children_per_agent"`

	// DeniedTools lists tool names that subagents cannot use.
	// Default: ["spawn_subagent", "list_subagents", "wait_subagent"]
	DeniedTools []string `yaml:"denied_tools"`

	// Model overrides the LLM model for subagents (empty = use parent model).
	Model string `yaml:"model"`
}

// DefaultSubagentDeniedTools lists tools subagents should not access.
// Note: spawn tools (spawn_subagent, list_subagents, wait_subagent, stop_subagent)
// are intentionally omitted — they are managed by depth-based logic in createChildExecutor.
var DefaultSubagentDeniedTools = []string{
	// Memory tool (subagents should not pollute parent's memory).
	"memory",
	// Scheduler dispatcher (subagents should not create/remove jobs).
	"scheduler",
	// Skill management dispatcher (subagents should not install/remove skills).
	"skill_manage",
}

// DefaultSubagentConfig returns safe defaults.
func DefaultSubagentConfig() SubagentConfig {
	return SubagentConfig{
		Enabled:             true,
		MaxConcurrent:       8,
		MaxTurns:            0,   // Unlimited (aligned with agent loop)
		TimeoutSeconds:      600, // 10 minutes — enough for research tasks that do many web searches
		MaxSpawnDepth:       1,   // No nesting by default (subagents cannot spawn children)
		MaxChildrenPerAgent: 5,   // Each agent can spawn up to 5 children
		DeniedTools:         DefaultSubagentDeniedTools,
	}
}

// ─── Subagent Run ───

// SubagentStatus represents the current state of a subagent run.
type SubagentStatus string

const (
	SubagentStatusRunning   SubagentStatus = "running"
	SubagentStatusCompleted SubagentStatus = "completed"
	SubagentStatusFailed    SubagentStatus = "failed"
	SubagentStatusTimeout   SubagentStatus = "timeout"
)

// SubagentRun tracks a single subagent execution.
type SubagentRun struct {
	// ID is a unique identifier for this run.
	ID string `json:"id"`

	// Label is a human-readable label for identification.
	Label string `json:"label"`

	// Task is the original task description given to the subagent.
	Task string `json:"task"`

	// Status is the current execution status.
	Status SubagentStatus `json:"status"`

	// Result is the final text response (set on completion).
	Result string `json:"result,omitempty"`

	// Error holds the error message if the subagent failed.
	Error string `json:"error,omitempty"`

	// Model is the LLM model used for this run.
	Model string `json:"model,omitempty"`

	// ParentSessionID is the session that spawned this subagent.
	ParentSessionID string `json:"parent_session_id"`

	// SpawnDepth is the nesting level (1 = top-level subagent, 2 = child of subagent, etc.).
	SpawnDepth int `json:"spawn_depth"`

	// ChildrenCount tracks how many children this subagent has spawned.
	ChildrenCount int `json:"children_count,omitempty"`

	// OriginChannel is the channel name (e.g. "telegram", "discord") where the
	// spawn was requested. When set, the completion announcement is delivered
	// directly to that channel/chat rather than only injecting into the agent loop.
	OriginChannel string `json:"origin_channel,omitempty"`

	// OriginTo is the chat ID / recipient address in the origin channel.
	// For Telegram this may include a topic suffix (e.g. "12345678:topic:42").
	OriginTo string `json:"origin_to,omitempty"`

	// OriginThreadID is an optional thread or topic ID for threaded delivery
	// within the origin channel (e.g. a Slack thread_ts or Telegram topic ID).
	// TODO: populate from IncomingMessage.Metadata once per-message thread context
	// is propagated through the tool execution context.
	OriginThreadID string `json:"origin_thread_id,omitempty"`

	// StartedAt is when the subagent started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the subagent finished (zero if still running).
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the wall-clock execution time.
	Duration time.Duration `json:"duration,omitempty"`

	// TokensUsed tracks approximate token usage.
	TokensUsed int `json:"tokens_used,omitempty"`

	// cancel is the context cancel function for this run.
	cancel context.CancelFunc `json:"-"`

	// done is closed when the run finishes.
	done chan struct{} `json:"-"`
}

// ─── Subagent Manager ───

// AnnounceCallback is called when a subagent completes, allowing push-style
// notification to the parent session.
// Receives the completed run so the caller can notify the user/agent.
type AnnounceCallback func(run *SubagentRun)

// SubagentManager orchestrates subagent lifecycle: spawning, tracking, and cleanup.
type SubagentManager struct {
	cfg    SubagentConfig
	logger *slog.Logger

	// runs tracks all subagent runs (active and in-memory completed).
	runs map[string]*SubagentRun

	// db is the central SQLite database for persisting completed runs.
	// When nil, runs are only kept in memory (lost on restart).
	db *sql.DB

	// semaphore limits concurrent subagents.
	semaphore chan struct{}

	// announceCallback is called when a subagent completes, pushing the result
	// instead of requiring the parent to poll with wait_subagent.
	announceCallback AnnounceCallback

	mu sync.RWMutex
}

// NewSubagentManager creates a new subagent manager.
func NewSubagentManager(cfg SubagentConfig, logger *slog.Logger) *SubagentManager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 4
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 15
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 300
	}

	return &SubagentManager{
		cfg:       cfg,
		logger:    logger.With("component", "subagent-mgr"),
		runs:      make(map[string]*SubagentRun),
		semaphore: make(chan struct{}, cfg.MaxConcurrent),
	}
}

// SetAnnounceCallback registers a callback that fires when any subagent completes.
// This enables push-style announce: the parent is notified immediately
// instead of having to poll via wait_subagent.
func (m *SubagentManager) SetAnnounceCallback(cb AnnounceCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.announceCallback = cb
}

// SetDB wires the central SQLite database for persisting subagent runs.
// When set, completed/failed runs survive process restarts.
func (m *SubagentManager) SetDB(db *sql.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
}

// ─── SQLite Persistence ───

// persistRun saves or updates a subagent run to SQLite.
func (m *SubagentManager) persistRun(run *SubagentRun) {
	if m.db == nil {
		return
	}

	completedAt := ""
	if !run.CompletedAt.IsZero() {
		completedAt = run.CompletedAt.Format(time.RFC3339)
	}

	_, err := m.db.Exec(`
		INSERT OR REPLACE INTO subagent_runs
			(id, label, task, status, result, error, model, parent_session_id, tokens_used, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.Label, run.Task, string(run.Status),
		run.Result, run.Error, run.Model,
		run.ParentSessionID, run.TokensUsed,
		run.StartedAt.Format(time.RFC3339), completedAt,
	)
	if err != nil {
		m.logger.Warn("failed to persist subagent run", "run_id", run.ID, "error", err)
	}
}

// loadRunFromDB loads a single subagent run from SQLite by ID.
// Returns nil if not found.
func (m *SubagentManager) loadRunFromDB(runID string) *SubagentRun {
	if m.db == nil {
		return nil
	}

	var run SubagentRun
	var status, startedAt, completedAt string

	err := m.db.QueryRow(`
		SELECT id, label, task, status, result, error, model, parent_session_id, tokens_used, started_at, completed_at
		FROM subagent_runs WHERE id = ?`, runID,
	).Scan(&run.ID, &run.Label, &run.Task, &status,
		&run.Result, &run.Error, &run.Model,
		&run.ParentSessionID, &run.TokensUsed,
		&startedAt, &completedAt,
	)
	if err != nil {
		return nil
	}

	run.Status = SubagentStatus(status)
	run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if completedAt != "" {
		run.CompletedAt, _ = time.Parse(time.RFC3339, completedAt)
		run.Duration = run.CompletedAt.Sub(run.StartedAt)
	}
	return &run
}

// loadRecentRunsFromDB loads completed runs from the last N days.
func (m *SubagentManager) loadRecentRunsFromDB(days int) []*SubagentRun {
	if m.db == nil {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)
	rows, err := m.db.Query(`
		SELECT id, label, task, status, result, error, model, parent_session_id, tokens_used, started_at, completed_at
		FROM subagent_runs
		WHERE started_at > ? AND status != 'running'
		ORDER BY started_at DESC
		LIMIT 50`, cutoff,
	)
	if err != nil {
		m.logger.Warn("failed to load recent subagent runs", "error", err)
		return nil
	}
	defer rows.Close()

	var runs []*SubagentRun
	for rows.Next() {
		var run SubagentRun
		var status, startedAt, completedAt string

		if err := rows.Scan(&run.ID, &run.Label, &run.Task, &status,
			&run.Result, &run.Error, &run.Model,
			&run.ParentSessionID, &run.TokensUsed,
			&startedAt, &completedAt,
		); err != nil {
			continue
		}

		run.Status = SubagentStatus(status)
		run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		if completedAt != "" {
			run.CompletedAt, _ = time.Parse(time.RFC3339, completedAt)
			run.Duration = run.CompletedAt.Sub(run.StartedAt)
		}
		runs = append(runs, &run)
	}
	return runs
}

// cleanupStaleRunning marks any "running" entries from previous crashes as "failed".
// Called on startup — if a subagent was still running when the process died, it can't
// be recovered, so we mark it failed so the user sees an honest status.
func (m *SubagentManager) cleanupStaleRunning() {
	if m.db == nil {
		return
	}

	result, err := m.db.Exec(`
		UPDATE subagent_runs
		SET status = 'failed', error = 'interrupted by process restart', completed_at = datetime('now')
		WHERE status = 'running'`,
	)
	if err != nil {
		m.logger.Warn("failed to cleanup stale running subagents", "error", err)
		return
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		m.logger.Info("cleaned up stale subagent runs", "count", affected)
	}
}

// PruneOldRuns removes persisted runs older than the given number of days.
func (m *SubagentManager) PruneOldRuns(days int) int {
	if m.db == nil {
		return 0
	}

	cutoff := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := m.db.Exec(`DELETE FROM subagent_runs WHERE started_at < ? AND status != 'running'`, cutoff)
	if err != nil {
		m.logger.Warn("failed to prune old subagent runs", "error", err)
		return 0
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		m.logger.Info("pruned old subagent runs", "deleted", affected, "cutoff_days", days)
	}
	return int(affected)
}

// SpawnParams holds parameters for spawning a subagent.
type SpawnParams struct {
	Task            string
	Label           string
	Model           string
	ParentSessionID string
	TimeoutSeconds  int

	// SpawnDepth is the nesting level (1 = top-level subagent).
	// If not set, defaults to 1.
	SpawnDepth int

	// OriginChannel, OriginTo, and OriginThreadID identify where to push the
	// completion announcement. When OriginChannel is set the announce callback
	// delivers the result directly to that channel/chat in addition to injecting
	// it into the parent agent loop.
	OriginChannel  string
	OriginTo       string
	OriginThreadID string
}

// Spawn creates and starts a new subagent. Returns the run ID immediately.
// The subagent executes in a background goroutine.
func (m *SubagentManager) Spawn(
	parentCtx context.Context,
	params SpawnParams,
	llmClient *LLMClient,
	parentExecutor *ToolExecutor,
	promptComposer *PromptComposer,
) (*SubagentRun, error) {
	if !m.cfg.Enabled {
		return nil, fmt.Errorf("subagent system is disabled")
	}

	// Check spawn depth limit (nested subagents).
	depth := params.SpawnDepth
	if depth <= 0 {
		depth = 1 // Default: first-level subagent
	}
	maxDepth := m.cfg.MaxSpawnDepth
	if maxDepth <= 0 {
		maxDepth = 1 // Default: no nesting
	}
	if depth > maxDepth {
		return nil, fmt.Errorf("max spawn depth exceeded (depth=%d, max=%d)", depth, maxDepth)
	}

	// Create the run.
	runID := uuid.New().String()[:8]
	timeout := time.Duration(m.cfg.TimeoutSeconds) * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)

	run := &SubagentRun{
		ID:              runID,
		Label:           params.Label,
		Task:            params.Task,
		Status:          SubagentStatusRunning,
		Model:           params.Model,
		ParentSessionID: params.ParentSessionID,
		SpawnDepth:      depth,
		OriginChannel:   params.OriginChannel,
		OriginTo:        params.OriginTo,
		OriginThreadID:  params.OriginThreadID,
		StartedAt:       time.Now(),
		cancel:          cancel,
		done:            make(chan struct{}),
	}

	if run.Label == "" {
		run.Label = fmt.Sprintf("subagent-%s", runID)
	}

	// Check concurrency limit and register atomically to prevent TOCTOU race.
	m.mu.Lock()
	activeCount := 0
	for _, r := range m.runs {
		if r.Status == SubagentStatusRunning {
			activeCount++
		}
	}
	if activeCount >= m.cfg.MaxConcurrent {
		m.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("max concurrent subagents reached (%d/%d)", activeCount, m.cfg.MaxConcurrent)
	}
	m.runs[runID] = run
	m.mu.Unlock()

	// Persist the "running" state so we can detect interrupted subagents on restart.
	m.persistRun(run)

	m.logger.Info("spawning subagent",
		"run_id", runID,
		"depth", depth,
		"max_depth", maxDepth,
		"label", run.Label,
		"task_preview", truncate(params.Task, 80),
		"timeout", timeout,
	)

	// Create a filtered tool executor for the subagent.
	// Pass depth to allow conditional spawn tool access for nested subagents.
	childExecutor := m.createChildExecutor(parentExecutor, depth)

	// Determine model (subagent override > spawn param > parent).
	model := llmClient.model
	if m.cfg.Model != "" {
		model = m.cfg.Model
	}
	if params.Model != "" {
		model = params.Model
	}

	// Create a child LLM client if model differs.
	childLLM := llmClient
	if model != llmClient.model {
		childLLM = &LLMClient{
			baseURL:    llmClient.baseURL,
			apiKey:     llmClient.apiKey,
			model:      model,
			httpClient: llmClient.httpClient,
			logger:     llmClient.logger,
		}
		run.Model = model
	}

	// Launch the subagent goroutine.
	go func() {
		defer close(run.done)
		defer cancel()

		// Acquire semaphore slot.
		select {
		case m.semaphore <- struct{}{}:
			defer func() { <-m.semaphore }()
		case <-ctx.Done():
			m.completeRun(run, "", fmt.Errorf("timeout waiting for semaphore slot"))
			return
		}

		m.logger.Info("subagent started",
			"run_id", runID,
			"model", model,
		)

		// Create an isolated session for the subagent.
		session := &Session{
			ID:           fmt.Sprintf("subagent:%s", runID),
			Channel:      "subagent",
			ChatID:       runID,
			config:       SessionConfig{},
			activeSkills: []string{},
			facts:        []string{},
			history:      []ConversationEntry{},
			maxHistory:   50,
			CreatedAt:    time.Now(),
			lastActiveAt: time.Now(),
		}

		// Build a minimal system prompt for the subagent.
		systemPrompt := m.buildSubagentPrompt(promptComposer, session, params.Task)

		// Create and run the agent.
		agent := NewAgentRun(childLLM, childExecutor, m.logger)
		if m.cfg.MaxTurns > 0 {
			agent.maxTurns = m.cfg.MaxTurns // 0 = unlimited
		}
		// Subagent run timeout is driven by the context timeout set above,
		// so set the agent's own run timeout generously (it won't exceed ctx).
		agent.runTimeout = timeout + 30*time.Second

		// Set spawn depth in context so child subagents know their depth.
		runCtx := ContextWithSpawnDepth(ctx, depth)

		result, err := agent.Run(runCtx, systemPrompt, nil, params.Task)

		if ctx.Err() == context.DeadlineExceeded {
			m.completeRun(run, result, fmt.Errorf("timeout after %v", timeout))
		} else {
			m.completeRun(run, result, err)
		}
	}()

	return run, nil
}

// completeRun finalizes a subagent run with its result or error.
// After updating state, fires the announce callback if registered.
func (m *SubagentManager) completeRun(run *SubagentRun, result string, err error) {
	m.mu.Lock()

	run.CompletedAt = time.Now()
	run.Duration = run.CompletedAt.Sub(run.StartedAt)

	if err != nil {
		run.Status = SubagentStatusFailed
		run.Error = err.Error()
		if run.Result == "" {
			run.Result = result // May have partial result.
		}
		m.logger.Error("subagent failed",
			"run_id", run.ID,
			"label", run.Label,
			"error", err,
			"duration", run.Duration,
		)
	} else {
		run.Status = SubagentStatusCompleted
		run.Result = result
		m.logger.Info("subagent completed",
			"run_id", run.ID,
			"label", run.Label,
			"duration", run.Duration,
			"result_len", len(result),
		)
	}

	cb := m.announceCallback
	m.mu.Unlock()

	// Persist the completed state to SQLite for restart recovery.
	m.persistRun(run)

	// ── Announce (push) ── Notify parent immediately
	// instead of requiring poll via wait_subagent.
	if cb != nil {
		go cb(run)
	}
}

// Wait blocks until the specified subagent run completes or the context is cancelled.
// Returns the final run state.
func (m *SubagentManager) Wait(ctx context.Context, runID string) (*SubagentRun, error) {
	m.mu.RLock()
	run, ok := m.runs[runID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("subagent run %q not found", runID)
	}

	select {
	case <-run.done:
		return run, nil
	case <-ctx.Done():
		return run, ctx.Err()
	}
}

// Get returns a subagent run by ID. Checks in-memory first, then SQLite.
func (m *SubagentManager) Get(runID string) (*SubagentRun, bool) {
	m.mu.RLock()
	run, ok := m.runs[runID]
	m.mu.RUnlock()

	if ok {
		return run, true
	}

	// Fall back to SQLite for completed runs that were evicted from memory.
	if dbRun := m.loadRunFromDB(runID); dbRun != nil {
		return dbRun, true
	}
	return nil, false
}

// List returns all subagent runs (active in-memory + recent from SQLite).
// Merges both sources, deduplicating by ID.
func (m *SubagentManager) List() []*SubagentRun {
	m.mu.RLock()
	seen := make(map[string]bool, len(m.runs))
	runs := make([]*SubagentRun, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, run)
		seen[run.ID] = true
	}
	m.mu.RUnlock()

	// Merge recent runs from SQLite (last 7 days).
	dbRuns := m.loadRecentRunsFromDB(7)
	for _, run := range dbRuns {
		if !seen[run.ID] {
			runs = append(runs, run)
			seen[run.ID] = true
		}
	}

	return runs
}

// ActiveCount returns the number of currently running subagents.
func (m *SubagentManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, run := range m.runs {
		if run.Status == SubagentStatusRunning {
			count++
		}
	}
	return count
}

// Stop cancels a running subagent.
func (m *SubagentManager) Stop(runID string) error {
	m.mu.RLock()
	run, ok := m.runs[runID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("subagent run %q not found", runID)
	}

	if run.Status != SubagentStatusRunning {
		return fmt.Errorf("subagent %q is not running (status: %s)", runID, run.Status)
	}

	run.cancel()
	m.logger.Info("subagent stop requested", "run_id", runID)
	return nil
}

// Cleanup removes completed/failed runs older than the given duration.
func (m *SubagentManager) Cleanup(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, run := range m.runs {
		if run.Status != SubagentStatusRunning && run.CompletedAt.Before(cutoff) {
			delete(m.runs, id)
			removed++
		}
	}

	return removed
}

// ─── Tool Executor Filtering ───

// defaultSubagentDeny lists tools that are always denied to subagents.
// These are administrative or side-effect-heavy tools that subagents should not access.
var defaultSubagentDeny = map[string]bool{
	"sessions":       true,
	"security_audit": true,
	"canvas_create":  true,
	"canvas_update":  true,
	"canvas_list":    true,
	"canvas_stop":    true,
}

// createChildExecutor creates a filtered ToolExecutor for the subagent,
// excluding denied tools to prevent recursion and unsafe operations.
// Supports group references (e.g. "group:memory") in the deny list.
// depth controls whether spawn tools are allowed (if depth < maxSpawnDepth).
func (m *SubagentManager) createChildExecutor(parent *ToolExecutor, depth int) *ToolExecutor {
	child := NewToolExecutor(m.logger)

	// Copy the guard from parent.
	if parent.guard != nil {
		child.SetGuard(parent.guard)
	}

	// Copy caller context from parent.
	child.callerLevel = parent.callerLevel
	child.callerJID = parent.callerJID

	// Build deny set — expand group references + default deny list.
	expanded := ExpandToolGroups(m.cfg.DeniedTools)
	denySet := make(map[string]bool, len(expanded)+len(defaultSubagentDeny)+4)
	for name := range defaultSubagentDeny {
		denySet[name] = true
	}
	for _, name := range expanded {
		denySet[name] = true
	}

	// Determine if spawn tools should be denied based on depth.
	maxDepth := m.cfg.MaxSpawnDepth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	canSpawn := depth < maxDepth

	// Only deny subagent tools if we've reached max depth.
	// If canSpawn is true, the subagent can spawn its own children.
	if !canSpawn {
		denySet["spawn_subagent"] = true
		denySet["list_subagents"] = true
		denySet["wait_subagent"] = true
		denySet["stop_subagent"] = true
	}

	// Copy allowed tools from parent.
	parent.mu.RLock()
	for name, rt := range parent.tools {
		if denySet[name] {
			continue
		}
		child.tools[name] = rt
	}
	parent.mu.RUnlock()

	m.logger.Debug("child executor created",
		"parent_tools", len(parent.tools),
		"child_tools", len(child.tools),
		"denied", len(denySet),
		"depth", depth,
		"can_spawn", canSpawn,
	)

	return child
}

// buildSubagentPrompt creates a focused, minimal system prompt for the subagent.
// Subagents get a lightweight bootstrap prompt, NOT the
// full Compose() — this saves tokens and keeps the subagent focused on its task.
func (m *SubagentManager) buildSubagentPrompt(composer *PromptComposer, session *Session, task string) string {
	// Use ComposeMinimal instead of full Compose — saves ~60% of system prompt tokens.
	base := composer.ComposeMinimal()

	subagentInstructions := fmt.Sprintf(`# Subagent Context

You are a **subagent** spawned by the main agent for a specific task.

## Your Role
- You were created to handle: %s
- Complete this task. That's your entire purpose.
- Be thorough and provide a clear, structured result.
- If you encounter an error, try at least 2-3 alternative approaches before giving up.

## Rules
- Focus ONLY on the assigned task.
- You cannot spawn other subagents (no recursion).
- Do NOT ask the user questions — you have all the context you need.
- When done, provide a concise summary of what you accomplished.
- For coding tasks: include relevant file paths, changes made, and any issues found.

## Persistence
- Tool errors are hints, not stop signs. Fix the input and retry.
- NEVER say "I cannot do X" unless you've exhausted every tool available.
- Try alternative approaches when one fails.
`, task)

	return base + "\n" + subagentInstructions
}

// ─── Tool Registration ───

// RegisterSubagentTools registers the spawn_subagent, list_subagents,
// wait_subagent, and stop_subagent tools in the tool executor. These allow
// the main agent to create and manage child agents.
func RegisterSubagentTools(
	executor *ToolExecutor,
	manager *SubagentManager,
	llmClient *LLMClient,
	promptComposer *PromptComposer,
	logger *slog.Logger,
) {
	if manager == nil || !manager.cfg.Enabled {
		return
	}

	// ── spawn_subagent ──
	executor.Register(
		MakeToolDefinition("spawn_subagent",
			"Spawn a subagent to handle a task concurrently. The subagent runs "+
				"independently with its own context and tools. Use this for parallelizable "+
				"tasks like: researching multiple topics, running commands while writing code, "+
				"or handling independent subtasks. Returns immediately with a run_id.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task": map[string]any{
						"type":        "string",
						"description": "The task description for the subagent. Be specific and provide all context needed.",
					},
					"label": map[string]any{
						"type":        "string",
						"description": "A short label to identify this subagent (e.g. 'research-api', 'write-tests').",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "Override the LLM model for this subagent. Empty = use default.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Max execution time in seconds. Default: 300 (5 minutes).",
					},
				},
				"required": []string{"task"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return nil, fmt.Errorf("task is required")
			}

			label, _ := args["label"].(string)
			model, _ := args["model"].(string)
			timeoutSec := 0
			if v, ok := args["timeout_seconds"].(float64); ok {
				timeoutSec = int(v)
			}

			// Get current spawn depth from context (default: 0 = main agent).
			currentDepth := SpawnDepthFromContext(ctx)
			childDepth := currentDepth + 1

			// Capture the origin channel/chat from the session context so the
			// completion announcement can be delivered directly to the requester.
			var originChannel, originTo string
			if sessionID := SessionIDFromContext(ctx); sessionID != "" {
				// sessionID has the form "channel:chatID" (may include Telegram
				// topic suffix, e.g. "telegram:12345678:topic:42").
				// Split on the first ":" only to get channel name.
				if idx := strings.IndexByte(sessionID, ':'); idx != -1 {
					originChannel = sessionID[:idx]
					originTo = sessionID[idx+1:]
				}
			}

			run, err := manager.Spawn(
				context.Background(),
				SpawnParams{
					Task:           task,
					Label:          label,
					Model:          model,
					TimeoutSeconds: timeoutSec,
					SpawnDepth:     childDepth,
					OriginChannel:  originChannel,
					OriginTo:       originTo,
				},
				llmClient,
				executor,
				promptComposer,
			)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf(
				"Subagent spawned successfully.\n"+
					"  run_id: %s\n"+
					"  label: %s\n"+
					"  status: running\n\n"+
					"The result will be sent back as a user message once completed. "+
					"You may also use wait_subagent to block until done, or list_subagents to check status.",
				run.ID, run.Label,
			), nil
		},
	)

	// ── list_subagents ──
	executor.Register(
		MakeToolDefinition("list_subagents",
			"List all subagent runs and their current status. Use to check progress of spawned subagents.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status_filter": map[string]any{
						"type":        "string",
						"description": "Filter by status: 'running', 'completed', 'failed', 'all'. Default: 'all'.",
						"enum":        []string{"running", "completed", "failed", "all"},
					},
				},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			filter, _ := args["status_filter"].(string)
			if filter == "" {
				filter = "all"
			}

			runs := manager.List()
			if len(runs) == 0 {
				return "No subagent runs found.", nil
			}

			var result string
			count := 0
			for _, run := range runs {
				if filter != "all" && string(run.Status) != filter {
					continue
				}

				duration := run.Duration
				if run.Status == SubagentStatusRunning {
					duration = time.Since(run.StartedAt)
				}

				result += fmt.Sprintf(
					"- [%s] %s (id: %s) — %s — %s",
					run.Status, run.Label, run.ID, truncate(run.Task, 60), duration.Round(time.Second),
				)

				if run.Status == SubagentStatusCompleted {
					result += fmt.Sprintf(" — result: %s", truncate(run.Result, 100))
				}
				if run.Status == SubagentStatusFailed {
					result += fmt.Sprintf(" — error: %s", run.Error)
				}
				result += "\n"
				count++
			}

			if count == 0 {
				return fmt.Sprintf("No subagent runs with status '%s'.", filter), nil
			}

			return fmt.Sprintf("Subagent runs (%d):\n%s\nActive: %d / Max: %d",
				count, result, manager.ActiveCount(), manager.cfg.MaxConcurrent), nil
		},
	)

	// ── wait_subagent ──
	executor.Register(
		MakeToolDefinition("wait_subagent",
			"Wait for a subagent to complete and return its result. "+
				"Blocks until the subagent finishes or the timeout is reached.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "The run_id of the subagent to wait for.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Max time to wait in seconds. Default: 120.",
					},
				},
				"required": []string{"run_id"},
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			runID, _ := args["run_id"].(string)
			if runID == "" {
				return nil, fmt.Errorf("run_id is required")
			}

			timeoutSec := 120
			if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
				timeoutSec = int(v)
			}

			waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
			defer cancel()

			run, err := manager.Wait(waitCtx, runID)
			if err != nil {
				if run != nil {
					return fmt.Sprintf(
						"Wait interrupted: %v\n\nPartial state:\n  status: %s\n  label: %s\n  running_for: %s",
						err, run.Status, run.Label, time.Since(run.StartedAt).Round(time.Second),
					), nil
				}
				return nil, err
			}

			switch run.Status {
			case SubagentStatusCompleted:
				return fmt.Sprintf(
					"Subagent '%s' completed successfully in %s.\n\n### Result:\n%s",
					run.Label, run.Duration.Round(time.Second), run.Result,
				), nil

			case SubagentStatusFailed:
				result := fmt.Sprintf(
					"Subagent '%s' failed after %s.\n\nError: %s",
					run.Label, run.Duration.Round(time.Second), run.Error,
				)
				if run.Result != "" {
					result += fmt.Sprintf("\n\nPartial result:\n%s", run.Result)
				}
				return result, nil

			case SubagentStatusTimeout:
				return fmt.Sprintf(
					"Subagent '%s' timed out after %s.\n\nPartial result:\n%s",
					run.Label, run.Duration.Round(time.Second), run.Result,
				), nil

			default:
				return fmt.Sprintf("Subagent '%s' is still %s.", run.Label, run.Status), nil
			}
		},
	)

	// ── stop_subagent ──
	executor.Register(
		MakeToolDefinition("stop_subagent",
			"Stop a running subagent by cancelling its execution.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "The run_id of the subagent to stop.",
					},
				},
				"required": []string{"run_id"},
			},
		),
		func(_ context.Context, args map[string]any) (any, error) {
			runID, _ := args["run_id"].(string)
			if runID == "" {
				return nil, fmt.Errorf("run_id is required")
			}

			if err := manager.Stop(runID); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Subagent %s stop requested.", runID), nil
		},
	)

	logger.Info("subagent tools registered",
		"tools", []string{"spawn_subagent", "list_subagents", "wait_subagent", "stop_subagent"},
		"max_concurrent", manager.cfg.MaxConcurrent,
	)
}
