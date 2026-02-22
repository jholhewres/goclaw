package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHub_New(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devclaw-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := HubConfig{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        filepath.Join(tmpDir, "test.db"),
			JournalMode: "WAL",
		},
	}

	hub, err := NewHub(config, nil)
	if err != nil {
		t.Fatalf("NewHub failed: %v", err)
	}
	defer hub.Close()

	if hub == nil {
		t.Fatal("hub is nil")
	}

	// Check primary backend exists
	primary := hub.Primary()
	if primary == nil {
		t.Fatal("primary backend is nil")
	}

	if primary.Type != BackendSQLite {
		t.Errorf("expected SQLite backend, got %s", primary.Type)
	}
}

func TestHub_GetBackend(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	// Get primary backend (empty name)
	backend, err := hub.GetBackend("")
	if err != nil {
		t.Fatalf("GetBackend failed: %v", err)
	}
	if backend == nil {
		t.Fatal("backend is nil")
	}

	// Get non-existent backend
	_, err = hub.GetBackend("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent backend")
	}
}

func TestHub_Status(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	ctx := context.Background()
	status := hub.Status(ctx)

	if len(status) == 0 {
		t.Fatal("expected at least one backend status")
	}

	if _, ok := status["primary"]; !ok {
		t.Fatal("expected 'primary' backend in status")
	}
}

func TestHub_ListBackends(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	backends := hub.ListBackends()
	if len(backends) != 1 {
		t.Errorf("expected 1 backend, got %d", len(backends))
	}

	if backends[0] != "primary" {
		t.Errorf("expected 'primary', got %s", backends[0])
	}
}

func TestHub_Query(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	ctx := context.Background()

	// Create a test table
	_, err := hub.Exec(ctx, "", "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	// Insert data
	_, err = hub.Exec(ctx, "", "INSERT INTO test (name) VALUES (?)", "hello")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// Query data
	rows, err := hub.Query(ctx, "", "SELECT name FROM test")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected one row")
	}

	var name string
	if err := rows.Scan(&name); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if name != "hello" {
		t.Errorf("expected 'hello', got %s", name)
	}
}

func TestHubConfig_Effective(t *testing.T) {
	tests := []struct {
		name     string
		config   HubConfig
		expected BackendType
	}{
		{
			name:     "empty config defaults to sqlite",
			config:   HubConfig{},
			expected: BackendSQLite,
		},
		{
			name: "explicit sqlite",
			config: HubConfig{
				Backend: BackendSQLite,
			},
			expected: BackendSQLite,
		},
		{
			name: "postgresql",
			config: HubConfig{
				Backend: BackendPostgreSQL,
			},
			expected: BackendPostgreSQL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effective := tt.config.Effective()
			if effective.Backend != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, effective.Backend)
			}
		})
	}
}

func TestHub_Close(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)

	// Close should work without error
	if err := hub.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Second close should also work
	if err := hub.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

func TestHub_VectorSearch(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	ctx := context.Background()

	// Insert a test vector
	backend, _ := hub.GetBackend("")
	vector := []float32{1.0, 0.0, 0.0}
	err := backend.Vector.Insert(ctx, "test-collection", "id1", vector, map[string]any{"text": "test"})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Search using the hub method
	queryVector := []float32{1.0, 0.0, 0.0}
	results, err := hub.VectorSearch(ctx, "", "test-collection", queryVector, 10, nil)
	if err != nil {
		t.Fatalf("VectorSearch failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].ID != "id1" {
		t.Errorf("expected id1, got %s", results[0].ID)
	}
}

func TestHub_RemoveBackend(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	// Cannot remove primary backend
	err := hub.RemoveBackend("primary")
	if err == nil {
		t.Fatal("expected error when removing primary backend")
	}

	// Remove non-existent backend
	err = hub.RemoveBackend("nonexistent")
	if err == nil {
		t.Fatal("expected error when removing non-existent backend")
	}
}

func TestHub_Migrate(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := DefaultHubConfig()
	config.SQLite.Path = filepath.Join(tmpDir, "test.db")

	hub, _ := NewHub(config, nil)
	defer hub.Close()

	ctx := context.Background()

	// Run migration
	err := hub.Migrate(ctx, "", 0)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify migration worked
	backend, _ := hub.GetBackend("")
	needs, err := backend.Migrator.NeedsMigration(ctx)
	if err != nil {
		t.Fatalf("NeedsMigration failed: %v", err)
	}
	if needs {
		t.Error("expected no migration needed after running migrations")
	}
}
