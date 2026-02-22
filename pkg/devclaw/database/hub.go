package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
)

// Hub is the central database management system that orchestrates
// multiple database backends and provides a unified API.
type Hub struct {
	// backends stores all registered database backends by name
	backends map[string]*Backend

	// primary is the name of the default backend
	primary string

	// logger for hub operations
	logger *slog.Logger

	// mu protects concurrent access to backends
	mu sync.RWMutex

	// factories stores registered backend factories by type
	factories map[BackendType]BackendFactory
}

// NewHub creates a new Database Hub with the given configuration.
func NewHub(config HubConfig, logger *slog.Logger) (*Hub, error) {
	if logger == nil {
		logger = slog.Default()
	}

	hub := &Hub{
		backends:  make(map[string]*Backend),
		factories: make(map[BackendType]BackendFactory),
		logger:    logger,
	}

	// Register default factories
	hub.RegisterFactory(BackendSQLite, &SQLiteFactory{})
	hub.RegisterFactory(BackendPostgreSQL, NewPostgreSQLFactory(logger))
	// MySQL factory will be registered when implemented

	// Create primary backend
	cfg := config.Effective()
	primaryConfig, err := hub.getPrimaryConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("get primary config: %w", err)
	}

	if err := hub.AddBackend(context.Background(), "primary", primaryConfig); err != nil {
		return nil, fmt.Errorf("create primary backend: %w", err)
	}

	hub.primary = "primary"

	return hub, nil
}

// getPrimaryConfig extracts the configuration for the primary backend.
func (h *Hub) getPrimaryConfig(cfg HubConfig) (Config, error) {
	switch cfg.Backend {
	case BackendSQLite:
		return cfg.SQLite.ToConfig(), nil
	case BackendPostgreSQL:
		return cfg.PostgreSQL.ToConfig(), nil
	case BackendMySQL:
		return cfg.MySQL.ToConfig(), nil
	default:
		return Config{}, fmt.Errorf("unsupported backend type: %s", cfg.Backend)
	}
}

// RegisterFactory registers a backend factory for a specific backend type.
func (h *Hub) RegisterFactory(backendType BackendType, factory BackendFactory) {
	h.factories[backendType] = factory
}

// AddBackend creates and registers a new database backend.
func (h *Hub) AddBackend(ctx context.Context, name string, config Config) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.backends[name]; exists {
		return fmt.Errorf("backend %q already exists", name)
	}

	factory, ok := h.factories[config.Type]
	if !ok {
		return fmt.Errorf("no factory registered for backend type: %s", config.Type)
	}

	backend, err := factory.Create(config)
	if err != nil {
		return fmt.Errorf("create backend %q: %w", name, err)
	}

	backend.Name = name
	h.backends[name] = backend

	h.logger.Info("database backend registered",
		"name", name,
		"type", config.Type,
		"vector_support", backend.Vector != nil && backend.Vector.SupportsVector(),
	)

	return nil
}

// GetBackend returns a backend by name, or the primary backend if name is empty.
func (h *Hub) GetBackend(name string) (*Backend, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if name == "" {
		name = h.primary
	}

	backend, ok := h.backends[name]
	if !ok {
		return nil, fmt.Errorf("backend %q not found", name)
	}

	return backend, nil
}

// Primary returns the primary database backend.
func (h *Hub) Primary() *Backend {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.backends[h.primary]
}

// DB returns the sql.DB of the primary backend for direct access.
func (h *Hub) DB() *sql.DB {
	if backend := h.Primary(); backend != nil {
		return backend.DB
	}
	return nil
}

// Status returns the health status of all backends.
func (h *Hub) Status(ctx context.Context) map[string]HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := make(map[string]HealthStatus)
	for name, backend := range h.backends {
		if backend.Health != nil {
			status[name] = backend.Health.Status(ctx)
		} else {
			status[name] = HealthStatus{
				Healthy: false,
				Error:   "health checker not available",
			}
		}
	}

	return status
}

// Migrate runs migrations on the specified backend (or primary if empty).
func (h *Hub) Migrate(ctx context.Context, backendName string, target int) error {
	backend, err := h.GetBackend(backendName)
	if err != nil {
		return err
	}

	if backend.Migrator == nil {
		return fmt.Errorf("migrator not available for backend %q", backend.Name)
	}

	return backend.Migrator.Migrate(ctx, target)
}

// VectorSearch performs a vector similarity search on the specified backend.
// The queryVector must be pre-computed (use an embedding provider to convert text to vector).
func (h *Hub) VectorSearch(ctx context.Context, backendName string, collection string, queryVector []float32, k int, filter map[string]any) ([]SearchResult, error) {
	backend, err := h.GetBackend(backendName)
	if err != nil {
		return nil, err
	}

	if backend.Vector == nil {
		return nil, fmt.Errorf("vector search not available for backend %q", backend.Name)
	}

	return backend.Vector.Search(ctx, collection, queryVector, k, filter)
}

// Query executes a query on the specified backend.
func (h *Hub) Query(ctx context.Context, backendName string, query string, args ...any) (*sql.Rows, error) {
	backend, err := h.GetBackend(backendName)
	if err != nil {
		return nil, err
	}

	return backend.DB.QueryContext(ctx, query, args...)
}

// Exec executes a statement on the specified backend.
func (h *Hub) Exec(ctx context.Context, backendName string, query string, args ...any) (sql.Result, error) {
	backend, err := h.GetBackend(backendName)
	if err != nil {
		return nil, err
	}

	return backend.DB.ExecContext(ctx, query, args...)
}

// Close closes all database connections.
func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var errs []error

	for name, backend := range h.backends {
		if err := backend.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close backend %q: %w", name, err))
		}
		h.logger.Debug("database backend closed", "name", name)
	}

	h.backends = make(map[string]*Backend)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing backends: %v", errs)
	}

	return nil
}

// ListBackends returns the names of all registered backends.
func (h *Hub) ListBackends() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	names := make([]string, 0, len(h.backends))
	for name := range h.backends {
		names = append(names, name)
	}
	return names
}

// RemoveBackend removes a backend by name (cannot remove primary).
func (h *Hub) RemoveBackend(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if name == h.primary {
		return fmt.Errorf("cannot remove primary backend")
	}

	backend, ok := h.backends[name]
	if !ok {
		return fmt.Errorf("backend %q not found", name)
	}

	if err := backend.DB.Close(); err != nil {
		return fmt.Errorf("close backend %q: %w", name, err)
	}

	delete(h.backends, name)
	h.logger.Info("database backend removed", "name", name)

	return nil
}
