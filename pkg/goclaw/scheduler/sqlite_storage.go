// Package scheduler – sqlite_storage.go implements JobStorage backed by the
// central goclaw.db SQLite database. It is a drop-in replacement for
// FileJobStorage: same interface, no changes needed in scheduler.go.
package scheduler

import (
	"database/sql"
	"fmt"
	"time"
)

// SQLiteJobStorage persists jobs in the central goclaw.db "jobs" table.
type SQLiteJobStorage struct {
	db *sql.DB
}

// NewSQLiteJobStorage creates a SQLite-backed job storage using the shared DB.
// The "jobs" table must already exist (created by copilot.OpenDatabase).
func NewSQLiteJobStorage(db *sql.DB) *SQLiteJobStorage {
	return &SQLiteJobStorage{db: db}
}

// Save persists a job (insert or update).
func (s *SQLiteJobStorage) Save(job *Job) error {
	var lastRunAt sql.NullString
	if job.LastRunAt != nil {
		lastRunAt = sql.NullString{String: job.LastRunAt.UTC().Format(time.RFC3339), Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO jobs
			(id, schedule, type, command, channel, chat_id, enabled,
			 created_by, created_at, last_run_at, last_error, run_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID,
		job.Schedule,
		job.Type,
		job.Command,
		job.Channel,
		job.ChatID,
		boolToInt(job.Enabled),
		job.CreatedBy,
		job.CreatedAt.UTC().Format(time.RFC3339),
		lastRunAt,
		job.LastError,
		job.RunCount,
	)
	if err != nil {
		return fmt.Errorf("save job %q: %w", job.ID, err)
	}
	return nil
}

// Delete removes a job by ID.
func (s *SQLiteJobStorage) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM jobs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete job %q: %w", id, err)
	}
	return nil
}

// LoadAll reads all persisted jobs.
func (s *SQLiteJobStorage) LoadAll() ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, schedule, type, command, channel, chat_id, enabled,
		       created_by, created_at, last_run_at, last_error, run_count
		FROM jobs`)
	if err != nil {
		return nil, fmt.Errorf("load jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var (
			j          Job
			enabled    int
			createdAt  string
			lastRunAt  sql.NullString
		)
		if err := rows.Scan(
			&j.ID, &j.Schedule, &j.Type, &j.Command,
			&j.Channel, &j.ChatID, &enabled,
			&j.CreatedBy, &createdAt, &lastRunAt,
			&j.LastError, &j.RunCount,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}

		j.Enabled = enabled != 0
		j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if lastRunAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastRunAt.String)
			j.LastRunAt = &t
		}
		jobs = append(jobs, &j)
	}

	return jobs, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
