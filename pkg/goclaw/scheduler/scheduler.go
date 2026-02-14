// Package scheduler implements the task scheduling system for GoClaw.
// Uses robfig/cron for cron expression parsing and execution,
// with SQLite-based persistence for surviving restarts.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
}

// JobHandler is called when a job fires. Returns the agent response or error.
type JobHandler func(ctx context.Context, job *Job) (string, error)

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
func parseOneShotTime(timeStr string) (time.Time, error) {
	now := time.Now()

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

// executeJob runs a job's command through the handler with safety guards:
// - Per-job mutex prevents duplicate concurrent runs
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
	s.runningJobs[job.ID] = true
	s.mu.Unlock()

	defer func() {
		// Release the per-job lock.
		s.mu.Lock()
		delete(s.runningJobs, job.ID)
		s.mu.Unlock()

		// Recover from panics so one bad job doesn't crash all scheduling.
		if r := recover(); r != nil {
			job.LastError = fmt.Sprintf("panic: %v", r)
			s.logger.Error("scheduled job panicked",
				"id", job.ID, "panic", r)
			if s.storage != nil {
				s.storage.Save(job)
			}
		}
	}()

	s.logger.Info("executing scheduled job", "id", job.ID, "command", job.Command)

	now := time.Now()
	job.LastRunAt = &now
	job.RunCount++

	if s.handler == nil {
		job.LastError = "no handler configured"
		return
	}

	timeout := s.jobTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	result, err := s.handler(ctx, job)
	if err != nil {
		job.LastError = err.Error()
		s.logger.Error("scheduled job failed",
			"id", job.ID, "error", err)
	} else {
		job.LastError = ""
		s.logger.Info("scheduled job completed",
			"id", job.ID, "result_len", len(result))
	}

	// Persist updated state.
	if s.storage != nil {
		s.storage.Save(job)
	}
}
