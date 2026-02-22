//go:build integration
// +build integration

package backends

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPostgreSQLBackend_Integration tests the PostgreSQL backend with a real database.
// To run these tests:
//   1. Start a PostgreSQL container: docker run -d --name devclaw-test-pg -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=devclaw_test -p 5432:5432 pgvector/pgvector:pg16
//   2. Run tests: go test -tags=integration ./pkg/devclaw/database/backends/...
//
// Environment variables:
//   PGHOST     - PostgreSQL host (default: localhost)
//   PGPORT     - PostgreSQL port (default: 5432)
//   PGUSER     - PostgreSQL user (default: test)
//   PGPASSWORD - PostgreSQL password (default: test)
//   PGDATABASE - PostgreSQL database (default: devclaw_test)

func getPostgreSQLTestConfig() PostgreSQLConfig {
	return PostgreSQLConfig{
		Host:     getEnv("PGHOST", "localhost"),
		Port:     getEnvInt("PGPORT", 5432),
		User:     getEnv("PGUSER", "test"),
		Password: getEnv("PGPASSWORD", "test"),
		Database: getEnv("PGDATABASE", "devclaw_test"),
		SSLMode:  "disable",
		Vector: VectorConfig{
			Enabled:    true,
			Dimensions: 1536,
			IndexType:  "hnsw",
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := os.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultVal
}

func TestPostgreSQLBackend_Open(t *testing.T) {
	config := getPostgreSQLTestConfig()

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
	defer backend.Close()

	if backend.DB == nil {
		t.Fatal("DB is nil")
	}

	if err := backend.DB.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestPostgreSQLBackend_Migration(t *testing.T) {
	config := getPostgreSQLTestConfig()

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
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

func TestPostgreSQLBackend_Health(t *testing.T) {
	config := getPostgreSQLTestConfig()

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
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

	// Check pool metrics
	if _, ok := status["open_conns"]; !ok {
		t.Error("expected open_conns in status")
	}
}

func TestPgVector_Integration(t *testing.T) {
	config := getPostgreSQLTestConfig()
	config.Vector.Enabled = true
	config.Vector.Dimensions = 3 // Use small dimension for testing

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
	defer backend.Close()

	// Run migrations first
	if err := backend.Migrator.Migrate(0); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if backend.Vector == nil {
		t.Fatal("Vector store is nil")
	}

	ctx := context.Background()

	// Test Insert
	vector1 := []float32{1.0, 0.0, 0.0}
	vector2 := []float32{0.0, 1.0, 0.0}
	vector3 := []float32{0.9, 0.1, 0.0} // Similar to vector1

	err = backend.Vector.Insert("test-collection", "id1", vector1, map[string]any{"source": "test"}, "text 1")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	err = backend.Vector.Insert("test-collection", "id2", vector2, map[string]any{"source": "test"}, "text 2")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	err = backend.Vector.Insert("test-collection", "id3", vector3, map[string]any{"source": "test"}, "text 3")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Test Count
	count, err := backend.Vector.Count("test-collection")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 vectors, got %d", count)
	}

	// Test Search
	queryVector := []float32{1.0, 0.0, 0.0} // Should match id1 best
	results, err := backend.Vector.Search("test-collection", queryVector, 2, nil)
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
	err = backend.Vector.Delete("test-collection", "id1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	count, _ = backend.Vector.Count("test-collection")
	if count != 2 {
		t.Errorf("expected 2 vectors after delete, got %d", count)
	}

	// Cleanup
	err = backend.Vector.DeleteCollection("test-collection")
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}
}

func TestPostgreSQLBackend_Query(t *testing.T) {
	config := getPostgreSQLTestConfig()

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
	defer backend.Close()

	// Run migrations
	if err := backend.Migrator.Migrate(0); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	ctx := context.Background()

	// Test query
	rows, err := backend.DB.QueryContext(ctx, "SELECT 1+1 as result")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected one row")
	}

	var result int
	if err := rows.Scan(&result); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if result != 2 {
		t.Errorf("expected 2, got %d", result)
	}
}

func TestPostgreSQLBackend_ConnectionPool(t *testing.T) {
	config := getPostgreSQLTestConfig()
	config.MaxOpenConns = 10
	config.MaxIdleConns = 5
	config.ConnMaxLifetime = 30 * time.Minute

	backend, err := OpenPostgreSQL(config, nil)
	if err != nil {
		t.Fatalf("OpenPostgreSQL failed: %v", err)
	}
	defer backend.Close()

	stats := backend.DB.Stats()

	if stats.MaxOpenConnections != 10 {
		t.Errorf("expected MaxOpenConnections 10, got %d", stats.MaxOpenConnections)
	}

	// Test multiple concurrent queries
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, err := backend.DB.Exec("SELECT pg_sleep(0.1)")
			done <- err
		}()
	}

	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent query failed: %v", err)
		}
	}
}
