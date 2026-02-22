package backends

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSQLite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devclaw-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := SQLiteConfig{
		Path:        filepath.Join(tmpDir, "test.db"),
		JournalMode: "WAL",
		BusyTimeout: 5000,
		ForeignKeys: true,
	}

	backend, err := OpenSQLite(config)
	if err != nil {
		t.Fatalf("OpenSQLite failed: %v", err)
	}
	defer backend.Close()

	if backend.DB == nil {
		t.Fatal("DB is nil")
	}

	// Verify database is working
	if err := backend.DB.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestSQLiteBackend_Migration(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := SQLiteConfig{
		Path:        filepath.Join(tmpDir, "test.db"),
		JournalMode: "WAL",
	}

	backend, _ := OpenSQLite(config)
	defer backend.Close()

	// Run migration
	if err := backend.Migrator.Migrate(0); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Check version
	version, err := backend.Migrator.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}

	if version < 1 {
		t.Errorf("expected version >= 1, got %d", version)
	}

	// Check needs migration (should be false after migration)
	needs, err := backend.Migrator.NeedsMigration()
	if err != nil {
		t.Fatalf("NeedsMigration failed: %v", err)
	}

	if needs {
		t.Error("expected no migration needed after running migrations")
	}
}

func TestSQLiteBackend_Health(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	config := SQLiteConfig{
		Path: filepath.Join(tmpDir, "test.db"),
	}

	backend, _ := OpenSQLite(config)
	defer backend.Close()

	// Test Ping
	if err := backend.Health.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// Test Status
	status, err := backend.Health.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	healthy, ok := status["healthy"].(bool)
	if !ok || !healthy {
		t.Error("expected healthy status")
	}

	version, ok := status["version"].(string)
	if !ok || version == "" {
		t.Error("expected version in status")
	}
}

func TestInMemoryVectorStore(t *testing.T) {
	store := NewInMemoryVectorStore()

	// Test Insert
	vector1 := []float32{1.0, 0.0, 0.0}
	vector2 := []float32{0.0, 1.0, 0.0}
	vector3 := []float32{0.9, 0.1, 0.0} // Similar to vector1

	store.Insert("id1", vector1, nil, "text 1")
	store.Insert("id2", vector2, nil, "text 2")
	store.Insert("id3", vector3, nil, "text 3")

	if store.Count() != 3 {
		t.Errorf("expected 3 entries, got %d", store.Count())
	}

	// Test Search
	queryVector := []float32{1.0, 0.0, 0.0} // Should match id1 best
	results, err := store.Search(queryVector, 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// First result should be id1 (exact match)
	if results[0].ID != "id1" {
		t.Errorf("expected first result to be id1, got %s", results[0].ID)
	}

	// Test Delete
	store.Delete("id1")
	if store.Count() != 2 {
		t.Errorf("expected 2 entries after delete, got %d", store.Count())
	}
}

func TestGetSQLiteSchema(t *testing.T) {
	schema := GetSQLiteSchema()

	if schema == "" {
		t.Fatal("schema is empty")
	}

	// Check for expected tables
	expectedTables := []string{
		"jobs",
		"session_entries",
		"session_meta",
		"audit_log",
		"subagent_runs",
		"teams",
		"persistent_agents",
		"team_tasks",
	}

	for _, table := range expectedTables {
		if !contains(schema, table) {
			t.Errorf("expected table %s in schema", table)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
