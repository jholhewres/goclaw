package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/database/backends"
)

// SQLiteFactory creates SQLite backends.
type SQLiteFactory struct{}

// Create creates a new SQLite backend with the given configuration.
func (f *SQLiteFactory) Create(config Config) (*Backend, error) {
	if config.Type != BackendSQLite {
		return nil, fmt.Errorf("sqlite factory cannot create %s backend", config.Type)
	}

	sqliteConfig := backends.SQLiteConfig{
		Path:        config.Path,
		JournalMode: config.JournalMode,
		BusyTimeout: config.BusyTimeout,
		ForeignKeys: true,
	}

	sqliteBackend, err := backends.OpenSQLite(sqliteConfig)
	if err != nil {
		return nil, err
	}

	return &Backend{
		Type:     BackendSQLite,
		DB:       sqliteBackend.DB,
		Config:   config,
		Migrator: &sqliteMigratorWrapper{sqliteBackend.Migrator},
		Vector:   &inMemoryVectorWrapper{sqliteBackend.Vector},
		Health:   &sqliteHealthWrapper{sqliteBackend.Health},
	}, nil
}

// Supports returns true for SQLite backend type.
func (f *SQLiteFactory) Supports(backendType BackendType) bool {
	return backendType == BackendSQLite
}

// Wrapper types to adapt backends package types to database interfaces

type sqliteMigratorWrapper struct {
	m *backends.SQLiteMigrator
}

func (w *sqliteMigratorWrapper) CurrentVersion(ctx context.Context) (int, error) {
	return w.m.CurrentVersion()
}

func (w *sqliteMigratorWrapper) Migrate(ctx context.Context, target int) error {
	return w.m.Migrate(target)
}

func (w *sqliteMigratorWrapper) NeedsMigration(ctx context.Context) (bool, error) {
	return w.m.NeedsMigration()
}

type inMemoryVectorWrapper struct {
	v *backends.InMemoryVectorStore
}

func (w *inMemoryVectorWrapper) Insert(ctx context.Context, collection string, id string, vector []float32, metadata map[string]any) error {
	text, _ := metadata["text"].(string)
	return w.v.Insert(id, vector, metadata, text)
}

func (w *inMemoryVectorWrapper) Search(ctx context.Context, collection string, vector []float32, k int, filter map[string]any) ([]SearchResult, error) {
	results, err := w.v.Search(vector, k)
	if err != nil {
		return nil, err
	}

	converted := make([]SearchResult, len(results))
	for i, r := range results {
		converted[i] = SearchResult{
			ID:       r.ID,
			Score:    r.Score,
			Metadata: r.Metadata,
			Text:     r.Text,
		}
	}
	return converted, nil
}

func (w *inMemoryVectorWrapper) Delete(ctx context.Context, collection string, id string) error {
	return w.v.Delete(id)
}

func (w *inMemoryVectorWrapper) SupportsVector() bool {
	return true
}

type sqliteHealthWrapper struct {
	h *backends.SQLiteHealthChecker
}

func (w *sqliteHealthWrapper) Ping(ctx context.Context) error {
	return w.h.Ping()
}

func (w *sqliteHealthWrapper) Status(ctx context.Context) HealthStatus {
	status, err := w.h.Status()
	if err != nil {
		return HealthStatus{Healthy: false, Error: err.Error()}
	}

	return HealthStatus{
		Healthy:            extractBool(status, "healthy"),
		Version:            extractString(status, "version"),
		Error:              extractString(status, "error"),
		OpenConnections:    extractInt(status, "open_conns"),
		InUse:              extractInt(status, "in_use"),
		Idle:               extractInt(status, "idle"),
		WaitCount:          extractInt64(status, "wait_count"),
		WaitDuration:       time.Duration(extractInt64(status, "wait_duration_ms")) * time.Millisecond,
		MaxOpenConns:       extractInt(status, "max_open_conns"),
		MaxIdleClosed:      extractInt64(status, "max_idle_closed"),
		MaxLifetimeClosed:  extractInt64(status, "max_lifetime_closed"),
	}
}

// PostgreSQLFactory creates PostgreSQL backends (including Supabase).
type PostgreSQLFactory struct {
	logger *slog.Logger
}

// NewPostgreSQLFactory creates a new PostgreSQL factory.
func NewPostgreSQLFactory(logger *slog.Logger) *PostgreSQLFactory {
	return &PostgreSQLFactory{logger: logger}
}

// Create creates a new PostgreSQL backend with the given configuration.
func (f *PostgreSQLFactory) Create(config Config) (*Backend, error) {
	if config.Type != BackendPostgreSQL {
		return nil, fmt.Errorf("postgresql factory cannot create %s backend", config.Type)
	}

	pgConfig := backends.PostgreSQLConfig{
		Host:            config.Host,
		Port:            config.Port,
		Database:        config.Database,
		User:            config.User,
		Password:        config.Password,
		SSLMode:         config.SSLMode,
		MaxOpenConns:    config.MaxOpenConns,
		MaxIdleConns:    config.MaxIdleConns,
		ConnMaxLifetime: config.ConnMaxLifetime,
		SupabaseURL:     config.SupabaseURL,
		SupabaseAnonKey: config.SupabaseAnonKey,
		Vector: backends.VectorConfig{
			Enabled:    config.Vector.Enabled,
			Dimensions: config.Vector.Dimensions,
			IndexType:  config.Vector.IndexType,
			IVFLists:   config.Vector.IVFLists,
			HNSWM:      config.Vector.HNSWM,
		},
	}

	logger := f.logger
	if logger == nil {
		logger = slog.Default()
	}

	pgBackend, err := backends.OpenPostgreSQL(pgConfig, logger)
	if err != nil {
		return nil, err
	}

	return &Backend{
		Type:     BackendPostgreSQL,
		DB:       pgBackend.DB,
		Config:   config,
		Migrator: &postgreSQLMigratorWrapper{pgBackend.Migrator},
		Vector:   &pgVectorWrapper{pgBackend.Vector},
		Health:   &postgreSQLHealthWrapper{pgBackend.Health},
	}, nil
}

// Supports returns true for PostgreSQL backend type.
func (f *PostgreSQLFactory) Supports(backendType BackendType) bool {
	return backendType == BackendPostgreSQL
}

// Wrapper types for PostgreSQL

type postgreSQLMigratorWrapper struct {
	m *backends.PostgreSQLMigrator
}

func (w *postgreSQLMigratorWrapper) CurrentVersion(ctx context.Context) (int, error) {
	return w.m.CurrentVersion()
}

func (w *postgreSQLMigratorWrapper) Migrate(ctx context.Context, target int) error {
	return w.m.Migrate(target)
}

func (w *postgreSQLMigratorWrapper) NeedsMigration(ctx context.Context) (bool, error) {
	return w.m.NeedsMigration()
}

type pgVectorWrapper struct {
	v *backends.PgVectorStore
}

func (w *pgVectorWrapper) Insert(ctx context.Context, collection string, id string, vector []float32, metadata map[string]any) error {
	text, _ := metadata["text"].(string)
	return w.v.Insert(collection, id, vector, metadata, text)
}

func (w *pgVectorWrapper) Search(ctx context.Context, collection string, vector []float32, k int, filter map[string]any) ([]SearchResult, error) {
	results, err := w.v.Search(collection, vector, k, filter)
	if err != nil {
		return nil, err
	}

	converted := make([]SearchResult, len(results))
	for i, r := range results {
		converted[i] = SearchResult{
			ID:       r.ID,
			Score:    r.Score,
			Metadata: r.Metadata,
			Text:     r.Text,
		}
	}
	return converted, nil
}

func (w *pgVectorWrapper) Delete(ctx context.Context, collection string, id string) error {
	return w.v.Delete(collection, id)
}

func (w *pgVectorWrapper) SupportsVector() bool {
	return w.v != nil && w.v.SupportsVector()
}

type postgreSQLHealthWrapper struct {
	h *backends.PostgreSQLHealthChecker
}

func (w *postgreSQLHealthWrapper) Ping(ctx context.Context) error {
	return w.h.Ping()
}

func (w *postgreSQLHealthWrapper) Status(ctx context.Context) HealthStatus {
	status, err := w.h.Status()
	if err != nil {
		return HealthStatus{Healthy: false, Error: err.Error()}
	}

	latencyStr := extractString(status, "latency")
	latency := parseDuration(latencyStr)
	waitDurationMs := extractInt64(status, "wait_duration_ms")

	return HealthStatus{
		Healthy:            extractBool(status, "healthy"),
		Version:            extractString(status, "version"),
		Error:              extractString(status, "error"),
		Latency:            latency,
		OpenConnections:    extractInt(status, "open_conns"),
		InUse:              extractInt(status, "in_use"),
		Idle:               extractInt(status, "idle"),
		WaitCount:          extractInt64(status, "wait_count"),
		WaitDuration:       time.Duration(waitDurationMs) * time.Millisecond,
		MaxOpenConns:       extractInt(status, "max_open_conns"),
		MaxIdleClosed:      extractInt64(status, "max_idle_closed"),
		MaxLifetimeClosed:  extractInt64(status, "max_lifetime_closed"),
	}
}

// Helper functions for extracting values from map[string]any

func extractBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func extractString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func extractInt(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func extractInt64(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

func parseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

// MySQLFactory creates MySQL backends.
// This is a placeholder that will be implemented in the mysql.go file.
type MySQLFactory struct{}

// Create creates a new MySQL backend with the given configuration.
func (f *MySQLFactory) Create(config Config) (*Backend, error) {
	return nil, fmt.Errorf("mysql backend not yet implemented - use sqlite for now")
}

// Supports returns true for MySQL backend type.
func (f *MySQLFactory) Supports(backendType BackendType) bool {
	return backendType == BackendMySQL
}
