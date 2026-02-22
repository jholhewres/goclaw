// Package backends provides database backend implementations.
package backends

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteBackend wraps the SQLite database connection with additional functionality.
type SQLiteBackend struct {
	DB     *sql.DB
	Config SQLiteConfig

	// Migrator handles schema migrations
	Migrator *SQLiteMigrator

	// Health checker
	Health *SQLiteHealthChecker

	// Vector store (in-memory)
	Vector *InMemoryVectorStore
}

// SQLiteConfig holds SQLite-specific configuration.
type SQLiteConfig struct {
	Path        string
	JournalMode string
	BusyTimeout int
	ForeignKeys bool
}

// OpenSQLite opens or creates a SQLite database with the given configuration.
func OpenSQLite(config SQLiteConfig) (*SQLiteBackend, error) {
	if config.Path == "" {
		config.Path = "./data/devclaw.db"
	}
	if config.JournalMode == "" {
		config.JournalMode = "WAL"
	}
	if config.BusyTimeout == 0 {
		config.BusyTimeout = 5000
	}

	// Ensure parent directory exists
	dir := filepath.Dir(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dir, err)
	}

	// Build DSN with options
	dsn := fmt.Sprintf("%s?_journal_mode=%s&_busy_timeout=%d", config.Path, config.JournalMode, config.BusyTimeout)
	if config.ForeignKeys {
		dsn += "&_foreign_keys=ON"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", config.Path, err)
	}

	// Verify connectivity
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	backend := &SQLiteBackend{
		DB:     db,
		Config: config,
	}

	// Initialize components
	backend.Migrator = NewSQLiteMigrator(db)
	backend.Health = NewSQLiteHealthChecker(db)
	backend.Vector = NewInMemoryVectorStore()

	return backend, nil
}

// Close closes the database connection.
func (b *SQLiteBackend) Close() error {
	return b.DB.Close()
}

// SQLiteMigrator handles schema migrations for SQLite.
type SQLiteMigrator struct {
	db *sql.DB
}

// NewSQLiteMigrator creates a new SQLite migrator.
func NewSQLiteMigrator(db *sql.DB) *SQLiteMigrator {
	return &SQLiteMigrator{db: db}
}

// CurrentVersion returns the current schema version.
func (m *SQLiteMigrator) CurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		// Table might not exist yet
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

// Migrate applies migrations up to the target version.
func (m *SQLiteMigrator) Migrate(target int) error {
	// Create schema_version table if not exists
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	current, err := m.CurrentVersion()
	if err != nil {
		return err
	}

	// Run schema (idempotent via IF NOT EXISTS)
	schema := GetSQLiteSchema()
	if _, err := m.db.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	// Record migration
	if current == 0 {
		_, err = m.db.Exec("INSERT INTO schema_version (version) VALUES (1)")
		if err != nil {
			// Ignore duplicate key error
			if !isDuplicateKeyError(err) {
				return fmt.Errorf("record migration: %w", err)
			}
		}
	}

	return nil
}

// NeedsMigration returns true if schema is outdated.
func (m *SQLiteMigrator) NeedsMigration() (bool, error) {
	current, err := m.CurrentVersion()
	if err != nil {
		return false, err
	}
	return current < 1, nil // Version 1 is the current schema
}

// SQLiteHealthChecker monitors SQLite database health.
type SQLiteHealthChecker struct {
	db *sql.DB
}

// NewSQLiteHealthChecker creates a new health checker.
func NewSQLiteHealthChecker(db *sql.DB) *SQLiteHealthChecker {
	return &SQLiteHealthChecker{db: db}
}

// Ping checks database connectivity.
func (h *SQLiteHealthChecker) Ping() error {
	return h.db.Ping()
}

// Status returns detailed health status.
func (h *SQLiteHealthChecker) Status() (map[string]any, error) {
	stats := h.db.Stats()

	var version string
	err := h.db.QueryRow("SELECT sqlite_version()").Scan(&version)
	if err != nil {
		version = "unknown"
	}

	return map[string]any{
		"healthy":            true,
		"version":            version,
		"open_conns":         stats.OpenConnections,
		"in_use":             stats.InUse,
		"idle":               stats.Idle,
		"wait_count":         stats.WaitCount,
		"wait_duration_ms":   stats.WaitDuration.Milliseconds(),
		"max_open_conns":     stats.MaxOpenConnections,
		"max_idle_closed":    stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	}, nil
}

// InMemoryVectorStore provides in-memory vector search for SQLite.
type InMemoryVectorStore struct {
	mu      sync.RWMutex
	entries []vectorEntry
}

type vectorEntry struct {
	id       string
	vector   []float32
	metadata map[string]any
	text     string
}

// NewInMemoryVectorStore creates a new in-memory vector store.
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{
		entries: make([]vectorEntry, 0),
	}
}

// Insert adds a vector to the store.
func (s *InMemoryVectorStore) Insert(id string, vector []float32, metadata map[string]any, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for existing entry
	for i, e := range s.entries {
		if e.id == id {
			s.entries[i] = vectorEntry{id, vector, metadata, text}
			return nil
		}
	}

	s.entries = append(s.entries, vectorEntry{id, vector, metadata, text})
	return nil
}

// Search performs a similarity search.
func (s *InMemoryVectorStore) Search(vector []float32, k int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		entry vectorEntry
		score float64
	}

	var candidates []scored
	for _, e := range s.entries {
		sim := cosineSimilarity(vector, e.vector)
		if sim > 0 {
			candidates = append(candidates, scored{e, sim})
		}
	}

	// Sort by score descending
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if len(candidates) > k {
		candidates = candidates[:k]
	}

	results := make([]SearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = SearchResult{
			ID:       c.entry.id,
			Score:    c.score,
			Metadata: c.entry.metadata,
			Text:     c.entry.text,
		}
	}

	return results, nil
}

// Delete removes a vector from the store.
func (s *InMemoryVectorStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, e := range s.entries {
		if e.id == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return nil
		}
	}

	return nil
}

// Count returns the number of vectors in the store.
func (s *InMemoryVectorStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := sqrt(normA) * sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method for square root
	z := x
	for i := 0; i < 100; i++ {
		z = z - (z*z-x)/(2*z)
		if z*z-x < 1e-10 && z*z-x > -1e-10 {
			break
		}
	}
	return z
}

func isDuplicateKeyError(err error) bool {
	return err != nil && (err.Error() == "UNIQUE constraint failed: schema_version.version" ||
		err.Error() == "constraint failed")
}

// GetSQLiteSchema returns the SQLite schema DDL.
func GetSQLiteSchema() string {
	return `
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

-- Session conversation entries
CREATE TABLE IF NOT EXISTS session_entries (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id         TEXT NOT NULL,
    user_message       TEXT NOT NULL,
    assistant_response TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    meta               TEXT DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_session_entries_sid ON session_entries(session_id);

-- Session metadata
CREATE TABLE IF NOT EXISTS session_meta (
    session_id    TEXT PRIMARY KEY,
    channel       TEXT DEFAULT '',
    chat_id       TEXT DEFAULT '',
    config        TEXT DEFAULT '{}',
    active_skills TEXT DEFAULT '[]',
    updated_at    TEXT NOT NULL
);

-- Session long-term facts
CREATE TABLE IF NOT EXISTS session_facts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    fact       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_facts_sid ON session_facts(session_id);

-- Active agent runs
CREATE TABLE IF NOT EXISTS active_runs (
    session_id   TEXT PRIMARY KEY,
    channel      TEXT NOT NULL,
    chat_id      TEXT NOT NULL,
    user_message TEXT NOT NULL,
    started_at   TEXT NOT NULL
);

-- Tool execution audit log
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

-- Subagent runs
CREATE TABLE IF NOT EXISTS subagent_runs (
    id                TEXT PRIMARY KEY,
    label             TEXT NOT NULL,
    task              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'running',
    result            TEXT DEFAULT '',
    error             TEXT DEFAULT '',
    model             TEXT DEFAULT '',
    parent_session_id TEXT DEFAULT '',
    tokens_used       INTEGER DEFAULT 0,
    started_at        TEXT NOT NULL,
    completed_at      TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_parent ON subagent_runs(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_status ON subagent_runs(status);

-- System state
CREATE TABLE IF NOT EXISTS system_state (
    key       TEXT PRIMARY KEY,
    value     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Pairing tokens
CREATE TABLE IF NOT EXISTS pairing_tokens (
    id           TEXT PRIMARY KEY,
    token        TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL DEFAULT 'user',
    max_uses     INTEGER DEFAULT 0,
    use_count    INTEGER DEFAULT 0,
    auto_approve INTEGER DEFAULT 0,
    workspace_id TEXT DEFAULT '',
    note         TEXT DEFAULT '',
    created_by   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    expires_at   TEXT DEFAULT '',
    revoked      INTEGER DEFAULT 0,
    revoked_at   TEXT DEFAULT '',
    revoked_by   TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pairing_tokens_token ON pairing_tokens(token);
CREATE INDEX IF NOT EXISTS idx_pairing_tokens_created ON pairing_tokens(created_at);

-- Pairing requests
CREATE TABLE IF NOT EXISTS pairing_requests (
    id           TEXT PRIMARY KEY,
    token_id     TEXT NOT NULL,
    user_jid     TEXT NOT NULL,
    user_name    TEXT DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    reviewed_by  TEXT DEFAULT '',
    reviewed_at  TEXT DEFAULT '',
    created_at   TEXT NOT NULL,
    FOREIGN KEY (token_id) REFERENCES pairing_tokens(id)
);
CREATE INDEX IF NOT EXISTS idx_pairing_requests_status ON pairing_requests(status);
CREATE INDEX IF NOT EXISTS idx_pairing_requests_user ON pairing_requests(user_jid);

-- Teams
CREATE TABLE IF NOT EXISTS teams (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    description   TEXT DEFAULT '',
    owner_jid     TEXT NOT NULL,
    default_model TEXT DEFAULT '',
    workspace_path TEXT DEFAULT '',
    created_at    TEXT NOT NULL,
    enabled       INTEGER DEFAULT 1
);

-- Persistent agents
CREATE TABLE IF NOT EXISTS persistent_agents (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    role               TEXT NOT NULL,
    team_id            TEXT NOT NULL,
    level              TEXT DEFAULT 'specialist',
    status             TEXT DEFAULT 'idle',
    personality        TEXT DEFAULT '',
    instructions       TEXT DEFAULT '',
    model              TEXT DEFAULT '',
    skills             TEXT DEFAULT '[]',
    session_id         TEXT DEFAULT '',
    current_task_id    TEXT DEFAULT '',
    heartbeat_schedule TEXT DEFAULT '*/15 * * * *',
    created_at         TEXT NOT NULL,
    last_active_at     TEXT DEFAULT '',
    last_heartbeat_at  TEXT DEFAULT '',
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_agents_team ON persistent_agents(team_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON persistent_agents(status);

-- Team tasks
CREATE TABLE IF NOT EXISTS team_tasks (
    id             TEXT PRIMARY KEY,
    team_id        TEXT NOT NULL,
    title          TEXT NOT NULL,
    description    TEXT DEFAULT '',
    status         TEXT DEFAULT 'inbox',
    assignees      TEXT DEFAULT '[]',
    priority       INTEGER DEFAULT 3,
    labels         TEXT DEFAULT '[]',
    created_by     TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    completed_at   TEXT DEFAULT '',
    blocked_reason TEXT DEFAULT '',
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_team ON team_tasks(team_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON team_tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assignees ON team_tasks(assignees);

-- Team messages
CREATE TABLE IF NOT EXISTS team_messages (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    thread_id  TEXT DEFAULT '',
    from_agent TEXT DEFAULT '',
    from_user  TEXT DEFAULT '',
    content    TEXT NOT NULL,
    mentions   TEXT DEFAULT '[]',
    created_at TEXT NOT NULL,
    delivered  INTEGER DEFAULT 0,
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_messages_team ON team_messages(team_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON team_messages(thread_id);

-- Team pending messages
CREATE TABLE IF NOT EXISTS team_pending_messages (
    id           TEXT PRIMARY KEY,
    to_agent     TEXT NOT NULL,
    from_agent   TEXT DEFAULT '',
    from_user    TEXT DEFAULT '',
    content      TEXT NOT NULL,
    thread_id    TEXT DEFAULT '',
    created_at   TEXT NOT NULL,
    delivered    INTEGER DEFAULT 0,
    delivered_at TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pending_to ON team_pending_messages(to_agent);
CREATE INDEX IF NOT EXISTS idx_pending_delivered ON team_pending_messages(delivered);

-- Team facts
CREATE TABLE IF NOT EXISTS team_facts (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES teams(id),
    UNIQUE(team_id, key)
);
CREATE INDEX IF NOT EXISTS idx_facts_team ON team_facts(team_id);

-- Team activities
CREATE TABLE IF NOT EXISTS team_activities (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    type       TEXT NOT NULL,
    agent_id   TEXT DEFAULT '',
    message    TEXT NOT NULL,
    related_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_activities_team ON team_activities(team_id);
CREATE INDEX IF NOT EXISTS idx_activities_created ON team_activities(created_at);
`
}
