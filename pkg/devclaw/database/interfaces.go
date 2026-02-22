// Package database provides a unified database abstraction layer (Database Hub)
// that supports multiple backends (SQLite, PostgreSQL, MySQL) with a common interface.
// SQLite is the default backend, requiring zero configuration.
package database

import (
	"context"
	"database/sql"
	"time"
)

// BackendType identifies the type of database backend.
type BackendType string

const (
	BackendSQLite     BackendType = "sqlite"
	BackendPostgreSQL BackendType = "postgresql"
	BackendMySQL      BackendType = "mysql"
)

// Backend represents a database backend connection with all its capabilities.
type Backend struct {
	// Name is the identifier for this backend (e.g., "primary", "analytics")
	Name string

	// Type indicates the database type
	Type BackendType

	// DB is the underlying database connection
	DB *sql.DB

	// Config holds the backend configuration
	Config Config

	// Migrator handles schema migrations
	Migrator Migrator

	// Vector provides vector search capabilities (nil if not supported)
	Vector VectorStore

	// Health monitors database health
	Health HealthChecker
}

// VectorStore interface for vector similarity search operations.
// Implementations: pgvector (PostgreSQL), InMemoryVectorStore (SQLite fallback).
type VectorStore interface {
	// Insert adds a vector with associated metadata to the collection.
	Insert(ctx context.Context, collection string, id string, vector []float32, metadata map[string]any) error

	// Search performs a similarity search and returns the top k results.
	Search(ctx context.Context, collection string, vector []float32, k int, filter map[string]any) ([]SearchResult, error)

	// Delete removes a vector from the collection.
	Delete(ctx context.Context, collection string, id string) error

	// SupportsVector returns true if the backend supports native vector operations.
	SupportsVector() bool
}

// SearchResult represents a single vector search result with score and metadata.
type SearchResult struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Text     string         `json:"text,omitempty"`
}

// Migrator interface for database schema migrations.
type Migrator interface {
	// CurrentVersion returns the current schema version.
	CurrentVersion(ctx context.Context) (int, error)

	// Migrate applies migrations up to the target version.
	// If target is 0, migrates to the latest version.
	Migrate(ctx context.Context, target int) error

	// NeedsMigration returns true if the schema is outdated.
	NeedsMigration(ctx context.Context) (bool, error)
}

// HealthChecker interface for monitoring database health.
type HealthChecker interface {
	// Ping checks basic database connectivity.
	Ping(ctx context.Context) error

	// Status returns detailed health status.
	Status(ctx context.Context) HealthStatus
}

// HealthStatus represents the health state of a database backend.
type HealthStatus struct {
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Version string        `json:"version"`
	Error   string        `json:"error,omitempty"`

	// Connection pool metrics
	OpenConnections  int           `json:"open_connections"`
	InUse            int           `json:"in_use"`
	Idle             int           `json:"idle"`
	WaitCount        int64         `json:"wait_count"`
	WaitDuration     time.Duration `json:"wait_duration"`
	MaxOpenConns     int           `json:"max_open_conns"`
	MaxIdleClosed    int64         `json:"max_idle_closed"`
	MaxLifetimeClosed int64        `json:"max_lifetime_closed"`
}

// SessionPersister interface for session storage operations.
// This mirrors the existing SessionPersister interface for compatibility.
type SessionPersister interface {
	SaveEntry(sessionID string, entry any) error
	LoadSession(sessionID string) (any, any, error)
	SaveFacts(sessionID string, facts []string) error
	SaveMeta(sessionID, channel, chatID string, config any, activeSkills []string) error
	DeleteSession(sessionID string) error
	Rotate(sessionID string, maxLines int) error
	LoadAll() (map[string]any, error)
	Close() error
}

// JobStorage interface for scheduler job persistence.
// This mirrors the existing JobStorage interface for compatibility.
type JobStorage interface {
	Save(job any) error
	Delete(id string) error
	LoadAll() ([]any, error)
}

// BackendFactory creates database backends based on configuration.
type BackendFactory interface {
	// Create creates a new backend with the given configuration.
	Create(config Config) (*Backend, error)

	// Supports returns true if this factory can create the given backend type.
	Supports(backendType BackendType) bool
}
