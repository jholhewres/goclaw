// Package scheduler implements the task scheduling system for DevClaw.
// Uses robfig/cron for cron expression parsing and execution,
// with SQLite-based persistence for surviving restarts.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled tasks using cron expressions.
type Scheduler struct {
	// jobs stores registered jobs indexed by ID.
	jobs map[string]*Job

	// cron is the real cron scheduler from robfig/cron.
	cron *cron.Cron

	// cronIDs maps job IDs to their cron entry IDs for removal.
	cronIDs map[string]cron.EntryID

	// runningJobs tracks which jobs are currently executing to prevent
	// duplicate runs when a cron fires while the previous run is still active.
	runningJobs map[string]bool

	// storage persists jobs to disk/database.
	storage JobStorage

	// handler is called when a job triggers.
	handler JobHandler

	// jobTimeout is the maximum time a single job execution can take.
	// Defaults to 5 minutes. Jobs exceeding this are cancelled.
	jobTimeout time.Duration

	// announceHandler is called when a job with Announce=true completes,
	// sending the result back to the target channel/chat.
	announceHandler AnnounceHandler

	logger *slog.Logger
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// Job represents a scheduled task.
type Job struct {
	// ID is the unique job identifier.
	ID string `json:"id" yaml:"id"`

	// Schedule is the cron expression or shorthand.
	// Supports: standard 5-field cron, @daily, @hourly, @every 5m, etc.
	Schedule string `json:"schedule" yaml:"schedule"`

	// Type is the schedule type: "cron" (recurring), "at" (one-shot), "every" (interval).
	Type string `json:"type" yaml:"type"`

	// Command is the prompt/command executed by the agent.
	Command string `json:"command" yaml:"command"`

	// Channel is the target channel (e.g., "whatsapp").
	Channel string `json:"channel" yaml:"channel"`

	// ChatID is the target chat/group.
	ChatID string `json:"chat_id" yaml:"chat_id"`

	// Enabled indicates if the job is active.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CreatedBy is the user who created the job.
	CreatedBy string `json:"created_by,omitempty" yaml:"created_by,omitempty"`

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// LastRunAt is the last execution timestamp.
	LastRunAt *time.Time `json:"last_run_at,omitempty" yaml:"last_run_at,omitempty"`

	// LastError contains the error from the last run, if any.
	LastError string `json:"last_error,omitempty" yaml:"last_error,omitempty"`

	// RunCount tracks how many times the job has executed.
	RunCount int `json:"run_count" yaml:"run_count"`

	// ── Advanced cron features ──

	// IsolateSession runs each job execution in its own isolated session
	// instead of sharing the channel's main session. This prevents cron
	// output from polluting the main conversation history.
	IsolateSession bool `json:"isolate_session,omitempty" yaml:"isolate_session,omitempty"`

	// Announce sends the job result to the target channel/chat after completion.
	// When false, the job runs silently (result is only logged).
	Announce bool `json:"announce,omitempty" yaml:"announce,omitempty"`

	// AsSubagent runs the job as a subagent instead of in the main agent loop.
	// This provides better isolation and prevents cron jobs from blocking
	// user-initiated agent runs.
	AsSubagent bool `json:"as_subagent,omitempty" yaml:"as_subagent,omitempty"`

	// Model overrides the LLM model for this specific job (empty = default).
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// TimeoutSeconds overrides the global job timeout for this job.
	TimeoutSeconds int `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`

	// Labels are arbitrary tags for filtering and organization.
	Labels []string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// StaggerMs is a deterministic delay (in ms) applied before execution to
	// prevent thundering herd when multiple jobs share the same schedule.
	// Computed from job ID hash if zero and the schedule is top-of-hour.
	StaggerMs int `json:"stagger_ms,omitempty" yaml:"stagger_ms,omitempty"`

	// Exact disables stagger for this job (fire at exact schedule time).
	Exact bool `json:"exact,omitempty" yaml:"exact,omitempty"`

	// LastRunDuration is how long the last execution took.
	LastRunDuration time.Duration `json:"last_run_duration,omitempty" yaml:"last_run_duration,omitempty"`

	// LastUsage holds token usage from the last execution.
	LastUsage *JobUsage `json:"last_usage,omitempty" yaml:"last_usage,omitempty"`
}

// JobResult holds the outcome of a job execution, including usage telemetry.
type JobResult struct {
	Output string
	Usage  *JobUsage
}

// JobUsage tracks token usage from the LLM during a cron job execution.
type JobUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// JobHandler is called when a job fires. Returns the agent response or error.
// For backward compatibility, the handler may return just a string via the
// legacy signature, or a JobResult with usage telemetry.
type JobHandler func(ctx context.Context, job *Job) (string, error)

// AnnounceHandler is called when a job with Announce=true completes.
// Receives the channel, chatID, and formatted message to send.
type AnnounceHandler func(channel, chatID, message string) error

// JobStorage defines the persistence interface for jobs.
type JobStorage interface {
	Save(job *Job) error
	Delete(id string) error
	LoadAll() ([]*Job, error)
}

// New creates a new Scheduler with the given storage and handler.
func New(storage JobStorage, handler JobHandler, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		jobs:        make(map[string]*Job),
		cronIDs:     make(map[string]cron.EntryID),
		runningJobs: make(map[string]bool),
		storage:     storage,
		handler:     handler,
		jobTimeout:  5 * time.Minute,
		logger:      logger,
	}
}

// SetAnnounceHandler registers a callback for announce-enabled jobs.
func (s *Scheduler) SetAnnounceHandler(h AnnounceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.announceHandler = h
}

// Add registers a new job in the scheduler.
func (s *Scheduler) Add(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job %q already exists", job.ID)
	}
	if job.Schedule == "" {
		return fmt.Errorf("job schedule is required")
	}

	job.CreatedAt = time.Now()
	if job.Type == "" {
		job.Type = "cron"
	}

	// Register with cron if running and job is enabled.
	if s.cron != nil && job.Enabled {
		if err := s.scheduleCronJob(job); err != nil {
			return fmt.Errorf("invalid schedule %q: %w", job.Schedule, err)
		}
	}

	s.jobs[job.ID] = job

	if s.storage != nil {
		if err := s.storage.Save(job); err != nil {
			s.logger.Error("failed to persist job", "id", job.ID, "error", err)
		}
	}

	s.logger.Info("job added",
		"id", job.ID,
		"schedule", job.Schedule,
		"type", job.Type,
		"channel", job.Channel,
	)
	return nil
}

// Remove deletes a job by ID.
func (s *Scheduler) Remove(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[jobID]; !exists {
		return fmt.Errorf("job %q not found", jobID)
	}

	// Remove from cron if registered.
	if entryID, ok := s.cronIDs[jobID]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, jobID)
	}

	delete(s.jobs, jobID)

	if s.storage != nil {
		if err := s.storage.Delete(jobID); err != nil {
			s.logger.Error("failed to remove job from storage", "id", jobID, "error", err)
		}
	}

	s.logger.Info("job removed", "id", jobID)
	return nil
}

// List returns all registered jobs.
func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

// Get returns a job by ID.
func (s *Scheduler) Get(jobID string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[jobID]
	return j, ok
}

// Start initializes the cron scheduler and loads persisted jobs.
func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Create the cron scheduler with seconds support.
	s.cron = cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	// Load persisted jobs.
	if s.storage != nil {
		jobs, err := s.storage.LoadAll()
		if err != nil {
			s.logger.Error("failed to load jobs", "error", err)
		} else {
			s.mu.Lock()
			for _, job := range jobs {
				s.jobs[job.ID] = job
				if job.Enabled {
					if err := s.scheduleCronJob(job); err != nil {
						s.logger.Warn("skipping job with invalid schedule",
							"id", job.ID, "schedule", job.Schedule, "error", err)
					}
				}
			}
			s.mu.Unlock()
			s.logger.Info("jobs loaded from storage", "count", len(jobs))
		}
	}

	// Start cron.
	s.cron.Start()

	s.mu.RLock()
	jobCount := len(s.jobs)
	s.mu.RUnlock()

	s.logger.Info("scheduler started",
		"jobs", jobCount,
		"cron_entries", len(s.cron.Entries()),
	)
	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	if s.cron != nil {
		ctx := s.cron.Stop()
		// Wait for running jobs to finish (with timeout).
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
			s.logger.Warn("scheduler stop timed out")
		}
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.logger.Info("scheduler stopped")
}

// ToJSON serializes a job to JSON (for tool output).
func (j *Job) ToJSON() string {
	b, _ := json.MarshalIndent(j, "", "  ")
	return string(b)
}

// ---------- Internal ----------

// scheduleCronJob registers a job with the cron scheduler.
func (s *Scheduler) scheduleCronJob(job *Job) error {
	schedule := job.Schedule

	// Handle "at" type (one-shot): convert to nearest future time.
	if job.Type == "at" {
		// For one-shot jobs, we use a simple goroutine with a timer instead of cron.
		go s.runOneShotJob(job, schedule)
		return nil
	}

	// Handle "every" type: convert to cron's @every syntax.
	if job.Type == "every" && schedule[0] != '@' {
		schedule = "@every " + schedule
	}

	entryID, err := s.cron.AddFunc(schedule, func() {
		s.executeJob(job)
	})
	if err != nil {
		return err
	}

	s.cronIDs[job.ID] = entryID
	return nil
}

// runOneShotJob parses a time string and executes the job at that time.
// Supports: "15:04", "2006-01-02 15:04", ISO 8601, and Unix epoch seconds.
func (s *Scheduler) runOneShotJob(job *Job, timeStr string) {
	target, err := parseOneShotTime(timeStr)
	if err != nil {
		s.logger.Warn("invalid one-shot time", "id", job.ID, "time", timeStr, "error", err)
		return
	}

	delay := time.Until(target)
	if delay <= 0 {
		s.logger.Warn("one-shot time is in the past, executing immediately", "id", job.ID)
		if _, ok := s.Get(job.ID); ok {
			s.executeJob(job)
			s.Remove(job.ID)
		}
		return
	}

	s.logger.Info("one-shot job scheduled", "id", job.ID, "fires_at", target.Format(time.RFC3339), "fires_in", delay.String())

	select {
	case <-time.After(delay):
		// Verify job still exists (may have been removed while waiting).
		if _, ok := s.Get(job.ID); !ok {
			s.logger.Info("one-shot job was removed before firing", "id", job.ID)
			return
		}
		s.executeJob(job)
		s.Remove(job.ID)
	case <-s.ctx.Done():
		return
	}
}

// parseOneShotTime parses various time formats for one-shot scheduling.
// Supports: relative duration ("5m", "1h30m"), Unix epoch, ISO 8601,
// "2006-01-02 15:04", and "15:04" (today or tomorrow).
func parseOneShotTime(timeStr string) (time.Time, error) {
	now := time.Now()

	// Try relative duration first (e.g. "5m", "1h30m", "2h", "30s").
	// This allows "at" type to support "fire X time from now".
	if d, err := time.ParseDuration(timeStr); err == nil && d > 0 {
		return now.Add(d), nil
	}

	// Try Unix epoch (seconds).
	if len(timeStr) >= 10 {
		var allDigits bool = true
		for _, c := range timeStr {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			var epoch int64
			if _, err := fmt.Sscanf(timeStr, "%d", &epoch); err == nil {
				return time.Unix(epoch, 0), nil
			}
		}
	}

	// Try ISO 8601 (RFC3339).
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}

	// Try "2006-01-02T15:04:05".
	if t, err := time.Parse("2006-01-02T15:04:05", timeStr); err == nil {
		return t, nil
	}

	// Try "2006-01-02 15:04".
	if t, err := time.Parse("2006-01-02 15:04", timeStr); err == nil {
		return t, nil
	}

	// Try "15:04" (today or tomorrow).
	if t, err := time.Parse("15:04", timeStr); err == nil {
		target := time.Date(now.Year(), now.Month(), now.Day(),
			t.Hour(), t.Minute(), 0, 0, now.Location())
		if target.Before(now) {
			target = target.Add(24 * time.Hour)
		}
		return target, nil
	}

	return time.Time{}, fmt.Errorf("unrecognized time format: %s", timeStr)
}

// minJobInterval is the minimum time between consecutive executions of the
// same job. Prevents "spin loop" where cron fires multiple times within the
// same second (e.g., when cron.Next(now) == now).
const minJobInterval = 2 * time.Second

// executeJob runs a job's command through the handler with safety guards:
// - Per-job mutex prevents duplicate concurrent runs
// - Spin loop guard: skips execution if the job ran less than minJobInterval ago
// - Panic recovery isolates errors so one bad job doesn't crash others
// - Configurable timeout prevents stalls
func (s *Scheduler) executeJob(job *Job) {
	// Check if this job is already running (skip duplicate fires).
	s.mu.Lock()
	if s.runningJobs[job.ID] {
		s.mu.Unlock()
		s.logger.Warn("skipping job (already running)", "id", job.ID)
		return
	}

	// Spin loop guard: if the job ran less than minJobInterval ago, skip.
	// This prevents rapid re-execution when cron schedules fire at the exact
	// same second boundary.
	if job.LastRunAt != nil && time.Since(*job.LastRunAt) < minJobInterval {
		s.mu.Unlock()
		s.logger.Debug("skipping job (spin loop guard, ran too recently)",
			"id", job.ID,
			"last_run_at", job.LastRunAt.Format(time.RFC3339),
			"elapsed", time.Since(*job.LastRunAt).String(),
		)
		return
	}

	s.runningJobs[job.ID] = true
	s.mu.Unlock()

	defer func() {
		// Release the per-job lock.
		s.mu.Lock()
		delete(s.runningJobs, job.ID)
		s.mu.Unlock()

		// Recover from panics so one bad job doesn't crash all scheduling.
		if r := recover(); r != nil {
			s.mu.Lock()
			job.LastError = fmt.Sprintf("panic: %v", r)
			_, stillExists := s.jobs[job.ID]
			s.mu.Unlock()
			s.logger.Error("scheduled job panicked",
				"id", job.ID, "panic", r)
			if s.storage != nil && stillExists {
				s.storage.Save(job)
			}
		}
	}()

	// Apply stagger delay to prevent thundering herd.
	if stagger := resolveStagger(job); stagger > 0 {
		s.logger.Debug("applying stagger delay", "id", job.ID, "stagger", stagger)
		select {
		case <-time.After(stagger):
		case <-s.ctx.Done():
			s.mu.Lock()
			delete(s.runningJobs, job.ID)
			s.mu.Unlock()
			return
		}
	}

	s.logger.Info("executing scheduled job", "id", job.ID, "command", job.Command)

	s.mu.Lock()
	now := time.Now()
	job.LastRunAt = &now
	job.RunCount++
	s.mu.Unlock()

	// Persist LastRunAt immediately so that if the process crashes during
	// execution, the job won't fire again immediately on restart (avoiding
	// the 48h skip bug seen in cron implementations).
	if s.storage != nil {
		s.storage.Save(job)
	}

	if s.handler == nil {
		job.LastError = "no handler configured"
		return
	}

	timeout := s.jobTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if job.TimeoutSeconds > 0 {
		timeout = time.Duration(job.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	runStart := time.Now()
	result, err := s.handler(ctx, job)
	runDuration := time.Since(runStart)

	s.mu.Lock()
	job.LastRunDuration = runDuration
	if err != nil {
		job.LastError = err.Error()
	} else {
		job.LastError = ""
	}
	_, stillExists := s.jobs[job.ID]
	s.mu.Unlock()

	if err != nil {
		s.logger.Error("scheduled job failed",
			"id", job.ID, "error", err, "duration", runDuration)
	} else {
		s.logger.Info("scheduled job completed",
			"id", job.ID, "result_len", len(result), "duration", runDuration)
	}

	// Announce result to target channel if configured.
	if job.Announce && job.Channel != "" && job.ChatID != "" {
		s.mu.RLock()
		announcer := s.announceHandler
		s.mu.RUnlock()

		if announcer != nil {
			announceMsg := result
			if err != nil {
				announceMsg = fmt.Sprintf("[Cron job %q failed]: %s", job.ID, err)
				if result != "" {
					announceMsg += "\n\nPartial output:\n" + result
				}
			}
			if announceMsg != "" {
				if aErr := announcer(job.Channel, job.ChatID, announceMsg); aErr != nil {
					s.logger.Error("failed to announce cron result",
						"job_id", job.ID, "channel", job.Channel, "error", aErr)
				}
			}
		}
	}

	// Persist updated state (only if job wasn't removed while running).
	if s.storage != nil && stillExists {
		s.storage.Save(job)
	}
}

// resolveStagger computes the stagger delay for a job. If StaggerMs is
// explicitly set, uses that. Otherwise, for top-of-hour recurring schedules,
// derives a deterministic offset from the job ID hash (up to 5 minutes).
func resolveStagger(job *Job) time.Duration {
	if job.Exact {
		return 0
	}
	if job.StaggerMs > 0 {
		return time.Duration(job.StaggerMs) * time.Millisecond
	}
	if job.Type == "at" {
		return 0
	}
	if !isTopOfHourSchedule(job.Schedule) {
		return 0
	}
	return resolveStableCronOffset(job.ID, 5*time.Minute)
}

// resolveStableCronOffset returns a deterministic offset derived from the
// job ID hash, bounded by maxStagger. The same job ID always gets the same
// offset, distributing execution across the stagger window.
func resolveStableCronOffset(jobID string, maxStagger time.Duration) time.Duration {
	h := sha256.Sum256([]byte(jobID))
	n := binary.BigEndian.Uint32(h[:4])
	ms := int64(n) % maxStagger.Milliseconds()
	return time.Duration(ms) * time.Millisecond
}

// isTopOfHourSchedule detects schedules that fire at the top of the hour
// (e.g. "0 * * * *", "@hourly", "0 0 * * *", "@daily").
func isTopOfHourSchedule(schedule string) bool {
	s := strings.TrimSpace(strings.ToLower(schedule))
	if s == "@hourly" || s == "@daily" || s == "@weekly" || s == "@monthly" || s == "@yearly" || s == "@annually" {
		return true
	}
	fields := strings.Fields(s)
	if len(fields) >= 5 && fields[0] == "0" {
		return true
	}
	return false
}
