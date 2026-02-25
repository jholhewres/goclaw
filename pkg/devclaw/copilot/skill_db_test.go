// Package copilot – skill_db_test.go contains unit tests for the skill database.
package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestOpenSkillDatabase tests opening/creating the skill database.
func TestOpenSkillDatabase(t *testing.T) {
	// Create temp directory.
	tmpDir, err := os.MkdirTemp("", "skilldb-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Open database.
	db, err := OpenSkillDatabase(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Check file was created.
	dbPath := filepath.Join(tmpDir, "skill_database.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Check path.
	if db.Path() != dbPath {
		t.Errorf("Expected path %s, got %s", dbPath, db.Path())
	}
}

// TestCreateTable tests creating a table with custom fields.
func TestCreateTable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	columns := map[string]string{
		"name":  "TEXT NOT NULL",
		"email": "TEXT",
		"phone": "TEXT",
	}

	err := db.CreateTable("crm", "contacts", "Contatos", "Lista de contatos", columns)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Verify table was registered.
	tables, err := db.ListTables("crm")
	if err != nil {
		t.Fatalf("Failed to list tables: %v", err)
	}

	if len(tables) != 1 {
		t.Fatalf("Expected 1 table, got %d", len(tables))
	}

	if tables[0].TableName != "contacts" {
		t.Errorf("Expected table name 'contacts', got %s", tables[0].TableName)
	}

	if tables[0].DisplayName != "Contatos" {
		t.Errorf("Expected display name 'Contatos', got %s", tables[0].DisplayName)
	}
}

// TestCreateTableDuplicate tests error when creating duplicate table.
func TestCreateTableDuplicate(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	columns := map[string]string{"name": "TEXT"}

	// Create first table.
	err := db.CreateTable("crm", "contacts", "", "", columns)
	if err != nil {
		t.Fatalf("Failed to create first table: %v", err)
	}

	// Try to create duplicate.
	err = db.CreateTable("crm", "contacts", "", "", columns)
	if err == nil {
		t.Error("Expected error when creating duplicate table")
	}
}

// TestInsert tests inserting a record.
func TestInsert(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create table.
	columns := map[string]string{
		"name":  "TEXT NOT NULL",
		"email": "TEXT",
	}
	err := db.CreateTable("crm", "contacts", "", "", columns)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert record.
	data := map[string]any{
		"name":  "João Silva",
		"email": "joao@example.com",
	}
	id, err := db.Insert("crm", "contacts", data)
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	if id == "" {
		t.Error("Expected non-empty ID")
	}

	// Verify row count.
	tables, _ := db.ListTables("crm")
	if tables[0].RowCount != 1 {
		t.Errorf("Expected row count 1, got %d", tables[0].RowCount)
	}
}

// TestInsertAutoID tests that ID is automatically generated.
func TestInsertAutoID(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create table and insert.
	db.CreateTable("test", "items", "", "", map[string]string{"name": "TEXT"})
	id1, _ := db.Insert("test", "items", map[string]any{"name": "item1"})
	id2, _ := db.Insert("test", "items", map[string]any{"name": "item2"})

	if id1 == id2 {
		t.Error("Expected different IDs for different inserts")
	}

	if len(id1) != 8 {
		t.Errorf("Expected ID length 8, got %d", len(id1))
	}
}

// TestQuery tests querying with filters.
func TestQuery(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{
		"name":   "TEXT",
		"status": "TEXT",
	})
	db.Insert("crm", "contacts", map[string]any{"name": "João", "status": "novo"})
	db.Insert("crm", "contacts", map[string]any{"name": "Maria", "status": "contatado"})
	db.Insert("crm", "contacts", map[string]any{"name": "Pedro", "status": "novo"})

	// Query with filter.
	results, err := db.Query("crm", "contacts", map[string]any{"status": "novo"}, 0)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestQueryAll tests querying all records.
func TestQueryAll(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	db.Insert("crm", "contacts", map[string]any{"name": "João"})
	db.Insert("crm", "contacts", map[string]any{"name": "Maria"})

	// Query all (nil filter).
	results, err := db.Query("crm", "contacts", nil, 0)
	if err != nil {
		t.Fatalf("Failed to query all: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestQueryLimit tests limiting query results.
func TestQueryLimit(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	db.Insert("crm", "contacts", map[string]any{"name": "João"})
	db.Insert("crm", "contacts", map[string]any{"name": "Maria"})
	db.Insert("crm", "contacts", map[string]any{"name": "Pedro"})

	// Query with limit.
	results, err := db.Query("crm", "contacts", nil, 2)
	if err != nil {
		t.Fatalf("Failed to query with limit: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestUpdate tests updating a record.
func TestUpdate(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{
		"name":   "TEXT",
		"status": "TEXT",
	})
	id, _ := db.Insert("crm", "contacts", map[string]any{"name": "João", "status": "novo"})

	// Update.
	err := db.Update("crm", "contacts", id, map[string]any{"status": "contatado"})
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Verify.
	row, _ := db.GetByID("crm", "contacts", id)
	if row["status"] != "contatado" {
		t.Errorf("Expected status 'contatado', got %v", row["status"])
	}
}

// TestUpdateNotFound tests error when updating non-existent record.
func TestUpdateNotFound(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})

	// Try to update non-existent.
	err := db.Update("crm", "contacts", "notexist", map[string]any{"name": "Test"})
	if err == nil {
		t.Error("Expected error when updating non-existent record")
	}
}

// TestDelete tests deleting a record.
func TestDelete(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	id, _ := db.Insert("crm", "contacts", map[string]any{"name": "João"})

	// Delete.
	err := db.Delete("crm", "contacts", id)
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify.
	results, _ := db.Query("crm", "contacts", nil, 0)
	if len(results) != 0 {
		t.Errorf("Expected 0 results after delete, got %d", len(results))
	}

	// Verify row count.
	tables, _ := db.ListTables("crm")
	if tables[0].RowCount != 0 {
		t.Errorf("Expected row count 0, got %d", tables[0].RowCount)
	}
}

// TestDropTable tests dropping a table.
func TestDropTable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	db.Insert("crm", "contacts", map[string]any{"name": "João"})

	// Drop.
	err := db.DropTable("crm", "contacts")
	if err != nil {
		t.Fatalf("Failed to drop table: %v", err)
	}

	// Verify.
	tables, _ := db.ListTables("crm")
	if len(tables) != 0 {
		t.Errorf("Expected 0 tables after drop, got %d", len(tables))
	}
}

// TestTableNameSanitization tests name validation.
func TestTableNameSanitization(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	invalidNames := []string{
		"",               // empty
		"123abc",         // starts with number
		"ABC",            // uppercase
		"my-table",       // hyphen
		"my table",       // space
		"sqlite_test",    // sqlite_ prefix (now blocked)
		"_test",          // starts with underscore
		strings.Repeat("a", 65), // too long
	}

	for _, name := range invalidNames {
		err := db.CreateTable(name, "test", "", "", map[string]string{"col": "TEXT"})
		if err == nil {
			t.Errorf("Expected error for invalid skill name %q", name)
		}
	}
}

// TestSQLInjectionPrevention tests that SQL injection is prevented.
func TestSQLInjectionPrevention(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Valid table first.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})

	// Try injection in column name (should fail validation).
	injectionAttempts := []string{
		"name; DROP TABLE crm_contacts--",
		"name) TEXT; DROP TABLE crm_contacts--",
		"name TEXT); DROP TABLE crm_contacts; --",
	}

	for _, attempt := range injectionAttempts {
		err := db.CreateTable("test", attempt, "", "", map[string]string{"col": "TEXT"})
		if err == nil {
			t.Errorf("Expected error for injection attempt %q", attempt)
		}
	}

	// Try injection in data (should be safe via prepared statements).
	id, _ := db.Insert("crm", "contacts", map[string]any{
		"name": "'); DROP TABLE crm_contacts; --",
	})

	// Verify table still exists.
	row, err := db.GetByID("crm", "contacts", id)
	if err != nil {
		t.Errorf("Table should still exist after injection attempt in data")
	}
	if row["name"] != "'); DROP TABLE crm_contacts; --" {
		t.Errorf("Data should be stored as-is, got %v", row["name"])
	}
}

// TestConcurrentAccess tests concurrent read/write operations.
func TestConcurrentAccess(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Concurrent inserts.
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := db.Insert("crm", "contacts", map[string]any{"name": fmt.Sprintf("item%d", i)})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Concurrent reads.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.Query("crm", "contacts", nil, 0)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify all inserts.
	results, _ := db.Query("crm", "contacts", nil, 0)
	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}

// TestListTablesAllSkills tests listing tables from all skills.
func TestListTablesAllSkills(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create tables for different skills.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	db.CreateTable("crm", "deals", "", "", map[string]string{"title": "TEXT"})
	db.CreateTable("tasks", "items", "", "", map[string]string{"title": "TEXT"})

	// List all (empty skill name).
	tables, err := db.ListTables("")
	if err != nil {
		t.Fatalf("Failed to list all tables: %v", err)
	}

	if len(tables) != 3 {
		t.Errorf("Expected 3 tables, got %d", len(tables))
	}
}

// TestDescribeTable tests describing a table.
func TestDescribeTable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create table with schema.
	schema := map[string]string{
		"name":   "TEXT NOT NULL",
		"email":  "TEXT",
		"status": "TEXT DEFAULT 'novo'",
	}
	db.CreateTable("crm", "contacts", "Contatos", "Lista de contatos", schema)

	// Describe.
	info, err := db.DescribeTable("crm", "contacts")
	if err != nil {
		t.Fatalf("Failed to describe table: %v", err)
	}

	if info.DisplayName != "Contatos" {
		t.Errorf("Expected display name 'Contatos', got %s", info.DisplayName)
	}

	if info.Description != "Lista de contatos" {
		t.Errorf("Expected description 'Lista de contatos', got %s", info.Description)
	}

	if len(info.Schema) != 3 {
		t.Errorf("Expected 3 schema columns, got %d", len(info.Schema))
	}
}

// TestGetByID tests getting a single record by ID.
func TestGetByID(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})
	id, _ := db.Insert("crm", "contacts", map[string]any{"name": "João"})

	// Get by ID.
	row, err := db.GetByID("crm", "contacts", id)
	if err != nil {
		t.Fatalf("Failed to get by ID: %v", err)
	}

	if row["name"] != "João" {
		t.Errorf("Expected name 'João', got %v", row["name"])
	}

	if row["id"] != id {
		t.Errorf("Expected id %s, got %v", id, row["id"])
	}
}

// TestGetByIDNotFound tests error when record not found.
func TestGetByIDNotFound(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("crm", "contacts", "", "", map[string]string{"name": "TEXT"})

	// Get non-existent.
	_, err := db.GetByID("crm", "contacts", "notexist")
	if err == nil {
		t.Error("Expected error when getting non-existent record")
	}
}

// TestTableNotFound tests error when table doesn't exist.
func TestTableNotFound(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Try to insert into non-existent table.
	_, err := db.Insert("crm", "contacts", map[string]any{"name": "Test"})
	if err == nil {
		t.Error("Expected error when inserting into non-existent table")
	}
}

// TestIsolatedSkills tests that skills are isolated from each other.
func TestIsolatedSkills(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create tables for two skills.
	db.CreateTable("skill1", "data", "", "", map[string]string{"value": "TEXT"})
	db.CreateTable("skill2", "data", "", "", map[string]string{"value": "TEXT"})

	// Insert data in each.
	id1, _ := db.Insert("skill1", "data", map[string]any{"value": "skill1 data"})
	id2, _ := db.Insert("skill2", "data", map[string]any{"value": "skill2 data"})

	// Verify isolation.
	row1, _ := db.GetByID("skill1", "data", id1)
	row2, _ := db.GetByID("skill2", "data", id2)

	if row1["value"] == row2["value"] {
		t.Error("Skills should have isolated data")
	}

	// Verify tables list is isolated per skill.
	tables1, _ := db.ListTables("skill1")
	tables2, _ := db.ListTables("skill2")

	if len(tables1) != 1 || tables1[0].TableName != "data" {
		t.Error("Skill1 should only see its own table")
	}
	if len(tables2) != 1 || tables2[0].TableName != "data" {
		t.Error("Skill2 should only see its own table")
	}
}

// Helper functions

func newTestDB(t *testing.T) *SkillDB {
	tmpDir, err := os.MkdirTemp("", "skilldb-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	db, err := OpenSkillDatabase(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// TestQueryWithOptions tests query with options.
func TestQueryWithOptions(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Setup.
	db.CreateTable("test", "items", "", "", map[string]string{
		"name":   "TEXT",
		"status": "TEXT",
		"value":  "INTEGER",
	})
	db.Insert("test", "items", map[string]any{"name": "item1", "status": "active", "value": 1})
	db.Insert("test", "items", map[string]any{"name": "item2", "status": "active", "value": 2})
	db.Insert("test", "items", map[string]any{"name": "item3", "status": "inactive", "value": 3})

	// Test with OrderBy.
	opts := QueryOptions{
		Where:   map[string]any{"status": "active"},
		OrderBy: "value",
		Order:   "ASC",
		Limit:   10,
	}
	results, err := db.QueryWithOptions("test", "items", opts)
	if err != nil {
		t.Fatalf("QueryWithOptions failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Test with Offset.
	opts = QueryOptions{
		Limit:   1,
		Offset:  1,
		OrderBy: "value",
		Order:   "ASC",
	}
	results, err = db.QueryWithOptions("test", "items", opts)
	if err != nil {
		t.Fatalf("QueryWithOptions with offset failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result with offset, got %d", len(results))
	}

	if results[0]["name"] != "item2" {
		t.Errorf("Expected second item with offset, got %v", results[0]["name"])
	}
}

// TestValidateColumnType tests column type validation.
func TestValidateColumnType(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	validTypes := []string{
		"TEXT",
		"TEXT NOT NULL",
		"INTEGER",
		"INTEGER NOT NULL",
		"REAL",
		"TEXT DEFAULT 'value'",
		"INTEGER DEFAULT 0",
		"BOOLEAN DEFAULT true",
	}

	for _, colType := range validTypes {
		tableName := strings.ToLower(strings.ReplaceAll(colType, " ", "_"))
		tableName = strings.ReplaceAll(tableName, "'", "")
		err := db.CreateTable("test", "temp_"+tableName, "", "", map[string]string{"col": colType})
		if err != nil {
			t.Errorf("Valid type %q should be accepted: %v", colType, err)
		}
	}

	invalidTypes := []string{
		"VARCHAR(255)",
		"TEXT; DROP TABLE users",
		"BLOB; malicious",
		"CUSTOM_TYPE",
	}

	for _, colType := range invalidTypes {
		err := db.CreateTable("test", "invalid_"+strings.ToLower(strings.ReplaceAll(colType, " ", "_")), "", "", map[string]string{"col": colType})
		if err == nil {
			t.Errorf("Invalid type %q should be rejected", colType)
		}
	}
}

// TestValidateRowID tests row ID validation.
func TestValidateRowID(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.CreateTable("test", "items", "", "", map[string]string{"name": "TEXT"})

	// Invalid row IDs should be rejected.
	invalidIDs := []string{
		"",
		strings.Repeat("a", 65), // too long
		"id with spaces",
		"id;DROP TABLE test_items",
	}

	for _, id := range invalidIDs {
		_, err := db.GetByID("test", "items", id)
		if err == nil {
			t.Errorf("Invalid row ID %q should be rejected", id)
		}
	}

	// Valid row IDs should be accepted.
	validID, _ := db.Insert("test", "items", map[string]any{"name": "test"})
	_, err := db.GetByID("test", "items", validID)
	if err != nil {
		t.Errorf("Valid row ID should be accepted: %v", err)
	}
}

// TestMaxQueryLimit tests that query limit is enforced.
func TestMaxQueryLimit(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.CreateTable("test", "items", "", "", map[string]string{"name": "TEXT"})

	// Request more than max limit.
	opts := QueryOptions{Limit: 5000}
	_, err := db.QueryWithOptions("test", "items", opts)
	if err != nil {
		t.Errorf("Query with excessive limit should be capped, not fail: %v", err)
	}
}

// TestPermissions tests database file permissions.
func TestPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skilldb-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := OpenSkillDatabase(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Check directory permissions (0700).
	dirInfo, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	dirMode := dirInfo.Mode().Perm()
	if dirMode != 0o700 {
		t.Errorf("Expected directory permissions 0700, got %o", dirMode)
	}

	// Check file permissions (0600).
	dbPath := filepath.Join(tmpDir, "skill_database.db")
	fileInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Failed to stat database file: %v", err)
	}
	fileMode := fileInfo.Mode().Perm()
	if fileMode != 0o600 {
		t.Errorf("Expected file permissions 0600, got %o", fileMode)
	}
}

// TestSqlitePrefixBlocked tests that sqlite_ prefix is blocked.
func TestSqlitePrefixBlocked(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	err := db.CreateTable("sqlite_test", "table1", "", "", map[string]string{"col": "TEXT"})
	if err == nil {
		t.Error("sqlite_ prefix should be blocked for skill names")
	}

	err = db.CreateTable("test", "sqlite_table", "", "", map[string]string{"col": "TEXT"})
	if err == nil {
		t.Error("sqlite_ prefix should be blocked for table names")
	}
}

// TestFullNameValidation tests combined name length validation.
func TestFullNameValidation(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Create a name that's valid individually but too long combined.
	longSkill := strings.Repeat("a", 64)
	longTable := strings.Repeat("b", 64)

	err := db.CreateTable(longSkill, longTable, "", "", map[string]string{"col": "TEXT"})
	if err == nil {
		t.Error("Combined name exceeding max length should be rejected")
	}
}

// TestConcurrentAccessWaitGroup tests concurrent access with proper WaitGroup usage.
func TestConcurrentAccessWaitGroup(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.CreateTable("test", "items", "", "", map[string]string{"value": "TEXT"})

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Concurrent inserts.
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := db.Insert("test", "items", map[string]any{"value": fmt.Sprintf("item%d", i)})
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Errorf("Concurrent inserts failed: %v", errors)
	}

	results, _ := db.Query("test", "items", nil, 0)
	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}
