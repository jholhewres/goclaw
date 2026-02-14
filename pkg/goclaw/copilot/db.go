// Package copilot – db.go provides the central SQLite database for GoClaw.
// A single goclaw.db file holds scheduler jobs, session history/meta/facts,
// and the audit log. The memory.db (FTS5/embeddings) and whatsapp.db
// (whatsmeow session) remain as separate databases.
package copilot

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver.
)

// schema is the DDL executed on every startup (idempotent via IF NOT EXISTS).
const schema = `
-- Scheduler jobs
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    schedule    TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'cron',
    command     TEXT NOT NULL,
    channel     TEXT DEFAULT '',
    chat_id     TEXT DEFAULT '',
    enabled     INTEGER DEFAULT 1,
    created_by  TEXT DEFAULT '',
    created_at  TEXT NOT NULL,
    last_run_at TEXT,
    last_error  TEXT DEFAULT '',
    run_count   INTEGER DEFAULT 0
);

-- Session conversation entries (append-only, one row per exchange).
CREATE TABLE IF NOT EXISTS session_entries (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id         TEXT NOT NULL,
    user_message       TEXT NOT NULL,
    assistant_response TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    meta               TEXT DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_session_entries_sid ON session_entries(session_id);

-- Session metadata (one row per session).
CREATE TABLE IF NOT EXISTS session_meta (
    session_id    TEXT PRIMARY KEY,
    channel       TEXT DEFAULT '',
    chat_id       TEXT DEFAULT '',
    config        TEXT DEFAULT '{}',
    active_skills TEXT DEFAULT '[]',
    updated_at    TEXT NOT NULL
);

-- Session long-term facts.
CREATE TABLE IF NOT EXISTS session_facts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    fact       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_facts_sid ON session_facts(session_id);

-- Tool execution audit log.
CREATE TABLE IF NOT EXISTS audit_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    tool           TEXT NOT NULL,
    caller         TEXT DEFAULT '',
    level          TEXT DEFAULT '',
    allowed        INTEGER NOT NULL,
    args_summary   TEXT DEFAULT '',
    result_summary TEXT DEFAULT '',
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
`

// OpenDatabase opens (or creates) the central goclaw.db at the given path.
// It enables WAL mode for concurrent read performance and creates all tables.
func OpenDatabase(path string) (*sql.DB, error) {
	if path == "" {
		path = "./data/goclaw.db"
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dir, err)
	}

	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Create schema (idempotent).
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}
