package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteFactory_Create(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "devclaw-test-*")
	defer os.RemoveAll(tmpDir)

	factory := &SQLiteFactory{}

	config := Config{
		Type:        BackendSQLite,
		Path:        filepath.Join(tmpDir, "test.db"),
		JournalMode: "WAL",
		BusyTimeout: 5000,
	}

	backend, err := factory.Create(config)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer backend.DB.Close()

	if backend.Type != BackendSQLite {
		t.Errorf("expected SQLite, got %s", backend.Type)
	}

	if backend.DB == nil {
		t.Fatal("DB is nil")
	}

	if backend.Migrator == nil {
		t.Fatal("Migrator is nil")
	}

	if backend.Vector == nil {
		t.Fatal("Vector is nil")
	}

	if backend.Health == nil {
		t.Fatal("Health is nil")
	}
}

func TestSQLiteFactory_Supports(t *testing.T) {
	factory := &SQLiteFactory{}

	if !factory.Supports(BackendSQLite) {
		t.Error("expected to support SQLite")
	}

	if factory.Supports(BackendPostgreSQL) {
		t.Error("expected NOT to support PostgreSQL")
	}

	if factory.Supports(BackendMySQL) {
		t.Error("expected NOT to support MySQL")
	}
}

func TestSQLiteFactory_WrongType(t *testing.T) {
	factory := &SQLiteFactory{}

	config := Config{
		Type: BackendPostgreSQL,
	}

	_, err := factory.Create(config)
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestPostgreSQLFactory_Supports(t *testing.T) {
	factory := &PostgreSQLFactory{}

	if !factory.Supports(BackendPostgreSQL) {
		t.Error("expected to support PostgreSQL")
	}

	if factory.Supports(BackendSQLite) {
		t.Error("expected NOT to support SQLite")
	}
}

func TestMySQLFactory_Supports(t *testing.T) {
	factory := &MySQLFactory{}

	if !factory.Supports(BackendMySQL) {
		t.Error("expected to support MySQL")
	}

	if factory.Supports(BackendSQLite) {
		t.Error("expected NOT to support SQLite")
	}
}

func TestFactory_BackendType(t *testing.T) {
	tests := []struct {
		backendType BackendType
		string      string
	}{
		{BackendSQLite, "sqlite"},
		{BackendPostgreSQL, "postgresql"},
		{BackendMySQL, "mysql"},
	}

	for _, tt := range tests {
		t.Run(string(tt.backendType), func(t *testing.T) {
			if string(tt.backendType) != tt.string {
				t.Errorf("expected %s, got %s", tt.string, tt.backendType)
			}
		})
	}
}
