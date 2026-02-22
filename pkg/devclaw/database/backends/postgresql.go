// Package backends provides database backend implementations.
package backends

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// PostgreSQLBackend wraps the PostgreSQL database connection.
type PostgreSQLBackend struct {
	DB     *sql.DB
	Config PostgreSQLConfig

	// Migrator handles schema migrations
	Migrator *PostgreSQLMigrator

	// Health checker
	Health *PostgreSQLHealthChecker

	// Vector store (pgvector)
	Vector *PgVectorStore

	// logger
	logger *slog.Logger
}

// PostgreSQLConfig holds PostgreSQL-specific configuration.
type PostgreSQLConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	// Supabase-specific
	SupabaseURL    string
	SupabaseAnonKey string

	// Vector config
	Vector VectorConfig
}

// OpenPostgreSQL opens or creates a PostgreSQL database connection.
func OpenPostgreSQL(config PostgreSQLConfig, logger *slog.Logger) (*PostgreSQLBackend, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Set defaults
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.SSLMode == "" {
		config.SSLMode = "disable"
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 25
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 10
	}
	if config.ConnMaxLifetime == 0 {
		config.ConnMaxLifetime = 30 * time.Minute
	}
	if config.ConnMaxIdleTime == 0 {
		config.ConnMaxIdleTime = 5 * time.Minute
	}

	// Build DSN
	dsn := buildPostgreSQLDSN(config)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Verify connectivity with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	backend := &PostgreSQLBackend{
		DB:     db,
		Config: config,
		logger: logger,
	}

	// Initialize components
	backend.Migrator = NewPostgreSQLMigrator(db)
	backend.Health = NewPostgreSQLHealthChecker(db)

	// Initialize pgvector if enabled
	if config.Vector.Enabled {
		vectorStore, err := NewPgVectorStore(db, config.Vector, logger)
		if err != nil {
			logger.Warn("pgvector initialization failed, vector search disabled", "error", err)
		} else {
			backend.Vector = vectorStore
			logger.Info("pgvector enabled", "dimensions", config.Vector.Dimensions, "index", config.Vector.IndexType)
		}
	}

	return backend, nil
}

// buildPostgreSQLDSN builds the connection string.
func buildPostgreSQLDSN(config PostgreSQLConfig) string {
	// Handle Supabase URL
	if config.SupabaseURL != "" {
		// Parse Supabase URL: https://xxx.supabase.co
		// Direct connection: db.xxx.supabase.co
		u, err := url.Parse(config.SupabaseURL)
		if err == nil {
			host := u.Host
			if strings.HasPrefix(host, "https://") {
				host = strings.TrimPrefix(host, "https://")
			}
			// Convert to database host format
			parts := strings.Split(host, ".")
			if len(parts) >= 3 && parts[1] == "supabase" {
				dbHost := fmt.Sprintf("db.%s.%s.%s", parts[0], parts[1], parts[2])
				return fmt.Sprintf("host=%s port=5432 user=postgres password=%s dbname=postgres sslmode=require",
					dbHost, config.Password)
			}
		}
	}

	// Standard PostgreSQL DSN
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Database, config.SSLMode)
}

// Close closes the database connection.
func (b *PostgreSQLBackend) Close() error {
	return b.DB.Close()
}

// PostgreSQLMigrator handles schema migrations for PostgreSQL.
type PostgreSQLMigrator struct {
	db *sql.DB
}

// NewPostgreSQLMigrator creates a new PostgreSQL migrator.
func NewPostgreSQLMigrator(db *sql.DB) *PostgreSQLMigrator {
	return &PostgreSQLMigrator{db: db}
}

// CurrentVersion returns the current schema version.
func (m *PostgreSQLMigrator) CurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		// Table might not exist
		return 0, nil
	}
	return version, nil
}

// Migrate applies migrations up to the target version.
func (m *PostgreSQLMigrator) Migrate(target int) error {
	ctx := context.Background()

	// Create schema_version table if not exists
	_, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Run schema (idempotent via IF NOT EXISTS)
	schema := GetPostgreSQLSchema()
	if _, err := m.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	// Enable pgvector extension
	if _, err := m.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		// Non-fatal: pgvector might not be available
	}

	// Record migration
	current, _ := m.CurrentVersion()
	if current == 0 {
		_, err = m.db.ExecContext(ctx, "INSERT INTO schema_version (version) VALUES (1) ON CONFLICT DO NOTHING")
		if err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
	}

	return nil
}

// NeedsMigration returns true if schema is outdated.
func (m *PostgreSQLMigrator) NeedsMigration() (bool, error) {
	// Check if schema_version table exists
	var exists bool
	err := m.db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'schema_version'
		)
	`).Scan(&exists)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// PostgreSQLHealthChecker monitors PostgreSQL database health.
type PostgreSQLHealthChecker struct {
	db *sql.DB
}

// NewPostgreSQLHealthChecker creates a new health checker.
func NewPostgreSQLHealthChecker(db *sql.DB) *PostgreSQLHealthChecker {
	return &PostgreSQLHealthChecker{db: db}
}

// Ping checks database connectivity.
func (h *PostgreSQLHealthChecker) Ping() error {
	return h.db.Ping()
}

// Status returns detailed health status.
func (h *PostgreSQLHealthChecker) Status() (map[string]any, error) {
	start := time.Now()
	err := h.db.Ping()
	latency := time.Since(start)

	if err != nil {
		return map[string]any{
			"healthy": false,
			"error":   err.Error(),
			"latency": latency.String(),
		}, nil
	}

	var version string
	if err := h.db.QueryRow("SELECT version()").Scan(&version); err != nil {
		version = "unknown"
	}

	stats := h.db.Stats()

	return map[string]any{
		"healthy":           true,
		"version":           version,
		"latency":           latency.String(),
		"open_conns":        stats.OpenConnections,
		"in_use":            stats.InUse,
		"idle":              stats.Idle,
		"wait_count":        stats.WaitCount,
		"wait_duration_ms":  stats.WaitDuration.Milliseconds(),
		"max_open_conns":    stats.MaxOpenConnections,
		"max_idle_closed":   stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	}, nil
}

// PgVectorStore implements vector operations using pgvector extension.
type PgVectorStore struct {
	db         *sql.DB
	dimensions int
	indexType  string
	logger     *slog.Logger

	// cache for embeddings loaded into memory
	mu    sync.RWMutex
	cache []vectorCacheEntry
}

type vectorCacheEntry struct {
	id       string
	vector   []float32
	metadata map[string]any
	text     string
}

// NewPgVectorStore creates a new pgvector store.
func NewPgVectorStore(db *sql.DB, config VectorConfig, logger *slog.Logger) (*PgVectorStore, error) {
	if config.Dimensions == 0 {
		config.Dimensions = 1536
	}
	if config.IndexType == "" {
		config.IndexType = "hnsw"
	}

	store := &PgVectorStore{
		db:         db,
		dimensions: config.Dimensions,
		indexType:  config.IndexType,
		logger:     logger,
	}

	// Initialize vector tables
	if err := store.initTables(); err != nil {
		return nil, err
	}

	return store, nil
}

// initTables creates the necessary tables and indexes.
func (s *PgVectorStore) initTables() error {
	ctx := context.Background()

	// Create embeddings table
	createTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS embeddings (
			id TEXT PRIMARY KEY,
			collection TEXT NOT NULL DEFAULT 'default',
			embedding vector(%d),
			metadata JSONB DEFAULT '{}',
			text TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
	`, s.dimensions)

	if _, err := s.db.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("create embeddings table: %w", err)
	}

	// Create vector index
	var indexSQL string
	if s.indexType == "hnsw" {
		indexSQL = `
			CREATE INDEX IF NOT EXISTS embeddings_embedding_idx
			ON embeddings
			USING hnsw (embedding vector_cosine_ops)
			WITH (m = 16, ef_construction = 64);
		`
	} else {
		// IVFFlat
		indexSQL = `
			CREATE INDEX IF NOT EXISTS embeddings_embedding_idx
			ON embeddings
			USING ivfflat (embedding vector_cosine_ops)
			WITH (lists = 100);
		`
	}

	if _, err := s.db.ExecContext(ctx, indexSQL); err != nil {
		s.logger.Warn("vector index creation failed (pgvector may not be available)", "error", err)
	}

	// Create collection index
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS embeddings_collection_idx ON embeddings(collection);"); err != nil {
		return fmt.Errorf("create collection index: %w", err)
	}

	return nil
}

// Insert adds a vector with metadata.
func (s *PgVectorStore) Insert(collection string, id string, vector []float32, metadata map[string]any, text string) error {
	ctx := context.Background()

	// Convert vector to PostgreSQL array format
	vectorStr := vectorToPgArray(vector)

	// Convert metadata to JSON
	metadataJSON := "{}"
	if metadata != nil {
		if b, err := marshalJSON(metadata); err == nil {
			metadataJSON = string(b)
		}
	}

	query := `
		INSERT INTO embeddings (id, collection, embedding, metadata, text, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			metadata = EXCLUDED.metadata,
			text = EXCLUDED.text,
			updated_at = NOW();
	`

	_, err := s.db.ExecContext(ctx, query, id, collection, vectorStr, metadataJSON, text)
	return err
}

// Search performs a similarity search.
func (s *PgVectorStore) Search(collection string, vector []float32, k int, filter map[string]any) ([]SearchResult, error) {
	ctx := context.Background()

	vectorStr := vectorToPgArray(vector)

	query := fmt.Sprintf(`
		SELECT id, 1 - (embedding <=> $1) as score, metadata, text
		FROM embeddings
		WHERE collection = $2
		ORDER BY embedding <=> $1
		LIMIT $3;
	`)

	rows, err := s.db.QueryContext(ctx, query, vectorStr, collection, k)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var metadataJSON string
		if err := rows.Scan(&r.ID, &r.Score, &metadataJSON, &r.Text); err != nil {
			continue
		}
		// Parse metadata
		if metadataJSON != "" && metadataJSON != "{}" {
			_ = unmarshalJSON([]byte(metadataJSON), &r.Metadata)
		}
		results = append(results, r)
	}

	return results, nil
}

// Delete removes a vector.
func (s *PgVectorStore) Delete(collection string, id string) error {
	ctx := context.Background()
	_, err := s.db.ExecContext(ctx, "DELETE FROM embeddings WHERE collection = $1 AND id = $2", collection, id)
	return err
}

// DeleteCollection removes all vectors from a collection.
func (s *PgVectorStore) DeleteCollection(collection string) error {
	ctx := context.Background()
	_, err := s.db.ExecContext(ctx, "DELETE FROM embeddings WHERE collection = $1", collection)
	return err
}

// Count returns the number of vectors in a collection.
func (s *PgVectorStore) Count(collection string) (int, error) {
	ctx := context.Background()
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM embeddings WHERE collection = $1", collection).Scan(&count)
	return count, err
}

// SupportsVector returns true.
func (s *PgVectorStore) SupportsVector() bool {
	return true
}

// vectorToPgArray converts a float32 slice to PostgreSQL array format.
func vectorToPgArray(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", f))
	}
	sb.WriteString("]")
	return sb.String()
}

// marshalJSON marshals a value to JSON.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// unmarshalJSON unmarshals JSON data.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// GetPostgreSQLSchema returns the PostgreSQL schema DDL.
func GetPostgreSQLSchema() string {
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
    id                 SERIAL PRIMARY KEY,
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
    id         SERIAL PRIMARY KEY,
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
    id             SERIAL PRIMARY KEY,
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
