// Package copilot – skill_db.go provides a database system for skills.
// It allows skills to store structured data in a dedicated SQLite database
// without requiring users to know SQL or create scripts.
//
// Each skill can have multiple tables. Table names are automatically prefixed
// with the skill name to ensure isolation (e.g., "crm_contacts" for skill "crm").
package copilot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SkillDB manages the skill database, allowing skills to create tables
// and perform CRUD operations on their data.
type SkillDB struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// TableInfo contains metadata about a skill's table.
type TableInfo struct {
	SkillName   string            `json:"skill_name"`
	TableName   string            `json:"table_name"`
	DisplayName string            `json:"display_name,omitempty"`
	Description string            `json:"description,omitempty"`
	Schema      map[string]string `json:"schema,omitempty"`
	RowCount    int               `json:"row_count"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// QueryOptions contains optional parameters for queries.
type QueryOptions struct {
	Where   map[string]any // Filter conditions
	Limit   int            // Max results (default 100, max 1000)
	Offset  int            // Skip results for pagination
	OrderBy string         // Column to order by (default: created_at)
	Order   string         // "ASC" or "DESC" (default: DESC)
}

// Constants for validation.
const (
	maxNameLength      = 64  // Max length for skill/table/column names
	maxFullNameLength  = 128 // Max length for combined skill_table name
	maxQueryLimit      = 1000
	defaultQueryLimit  = 100
	maxRowIDLength     = 64
)

// validNameRegex validates table and column names (lowercase letters, numbers, underscores).
var validNameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// validRowIDRegex validates row IDs (alphanumeric with hyphens).
var validRowIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// allowedColumnTypes is a whitelist of valid SQL column types.
var allowedColumnTypes = map[string]bool{
	"TEXT":               true,
	"TEXT NOT NULL":      true,
	"INTEGER":            true,
	"INTEGER NOT NULL":   true,
	"REAL":               true,
	"REAL NOT NULL":      true,
	"BLOB":               true,
	"NUMERIC":            true,
	"BOOLEAN":            true,
	"DATE":               true,
	"DATETIME":           true,
}

// allowedColumnTypesWithDefaults are type patterns that include DEFAULT values.
var allowedColumnDefaultPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^TEXT DEFAULT '.*'$`),
	regexp.MustCompile(`^TEXT NOT NULL DEFAULT '.*'$`),
	regexp.MustCompile(`^INTEGER DEFAULT -?\d+$`),
	regexp.MustCompile(`^INTEGER NOT NULL DEFAULT -?\d+$`),
	regexp.MustCompile(`^REAL DEFAULT -?\d+\.?\d*$`),
	regexp.MustCompile(`^REAL NOT NULL DEFAULT -?\d+\.?\d*$`),
	regexp.MustCompile(`^BOOLEAN DEFAULT (true|false|0|1)$`),
}

// registrySchema is the DDL for the skill tables registry.
const registrySchema = `
CREATE TABLE IF NOT EXISTS _skill_tables_registry (
    skill_name   TEXT NOT NULL,
    table_name   TEXT NOT NULL,
    display_name TEXT,
    description  TEXT,
    schema_json  TEXT NOT NULL DEFAULT '{}',
    row_count    INTEGER DEFAULT 0,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (skill_name, table_name)
);
CREATE INDEX IF NOT EXISTS idx_skill_tables_skill ON _skill_tables_registry(skill_name);
`

// OpenSkillDatabase opens (or creates) the skill database at dataDir/skill_database.db.
func OpenSkillDatabase(dataDir string) (*SkillDB, error) {
	if dataDir == "" {
		dataDir = "./data"
	}

	dbPath := filepath.Join(dataDir, "skill_database.db")

	// Ensure directory exists with secure permissions (0700 - owner only).
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Create registry schema.
	if _, err := db.Exec(registrySchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create registry schema: %w", err)
	}

	// Set database file permissions to owner-only (0600).
	if err := os.Chmod(dbPath, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("set database permissions: %w", err)
	}

	return &SkillDB{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// Close closes the database connection.
func (s *SkillDB) Close() error {
	return s.db.Close()
}

// Path returns the database file path.
func (s *SkillDB) Path() string {
	return s.dbPath
}

// validateName checks if a name is valid for tables/columns.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("name %q exceeds maximum length of %d characters", name, maxNameLength)
	}
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("invalid name %q: must start with lowercase letter and contain only lowercase letters, numbers, and underscores", name)
	}
	// Explicitly block sqlite_ prefix.
	if strings.HasPrefix(name, "sqlite_") {
		return fmt.Errorf("name %q cannot start with 'sqlite_' (reserved)", name)
	}
	return nil
}

// validateFullName checks if the combined skill_table name is valid.
func validateFullName(skillName, tableName string) error {
	fullName := fullTableName(skillName, tableName)
	if len(fullName) > maxFullNameLength {
		return fmt.Errorf("combined table name %q exceeds maximum length of %d characters", fullName, maxFullNameLength)
	}
	return nil
}

// validateRowID checks if a row ID is valid.
func validateRowID(rowID string) error {
	if rowID == "" {
		return fmt.Errorf("row ID cannot be empty")
	}
	if len(rowID) > maxRowIDLength {
		return fmt.Errorf("row ID exceeds maximum length of %d characters", maxRowIDLength)
	}
	if !validRowIDRegex.MatchString(rowID) {
		return fmt.Errorf("invalid row ID %q: must contain only alphanumeric characters, hyphens, and underscores", rowID)
	}
	return nil
}

// validateColumnType checks if a column type is in the allowed list.
func validateColumnType(colType string) error {
	// Check exact match first.
	if allowedColumnTypes[colType] {
		return nil
	}
	// Check patterns with DEFAULT values.
	for _, pattern := range allowedColumnDefaultPatterns {
		if pattern.MatchString(colType) {
			return nil
		}
	}
	return fmt.Errorf("invalid column type %q: allowed types are TEXT, INTEGER, REAL, BLOB, NUMERIC, BOOLEAN, DATE, DATETIME (with optional NOT NULL and DEFAULT)", colType)
}

// fullTableName returns the prefixed table name for a skill.
func fullTableName(skillName, tableName string) string {
	return skillName + "_" + tableName
}

// CreateTable creates a new table for a skill with the specified columns.
// Columns is a map of column name to SQL type definition (e.g., "TEXT NOT NULL").
// Automatic columns: id (TEXT PRIMARY KEY), created_at, updated_at.
func (s *SkillDB) CreateTable(skillName, tableName, displayName, description string, columns map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate names.
	if err := validateName(skillName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}
	if err := validateFullName(skillName, tableName); err != nil {
		return err
	}

	// Validate column names and types.
	for colName, colType := range columns {
		if err := validateName(colName); err != nil {
			return fmt.Errorf("invalid column name %q: %w", colName, err)
		}
		if err := validateColumnType(colType); err != nil {
			return fmt.Errorf("invalid column type for %q: %w", colName, err)
		}
	}

	fullName := fullTableName(skillName, tableName)

	// Check if table already exists in registry.
	var exists bool
	err := s.db.QueryRow(
		"SELECT 1 FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&exists)
	if err == nil {
		return fmt.Errorf("table %q already exists for skill %q", tableName, skillName)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check existing table: %w", err)
	}

	// Build CREATE TABLE statement.
	var colDefs []string
	colDefs = append(colDefs, "id TEXT PRIMARY KEY")
	for colName, colType := range columns {
		colDefs = append(colDefs, fmt.Sprintf("%s %s", colName, colType))
	}
	colDefs = append(colDefs, "created_at TEXT NOT NULL")
	colDefs = append(colDefs, "updated_at TEXT NOT NULL")

	createSQL := fmt.Sprintf("CREATE TABLE %s (\n    %s\n)", fullName, strings.Join(colDefs, ",\n    "))

	// Execute CREATE TABLE.
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// Create index on created_at.
	indexSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_created ON %s(created_at)", fullName, fullName)
	if _, err := s.db.Exec(indexSQL); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// Store schema as JSON for documentation.
	schemaJSON, err := json.Marshal(columns)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	// Insert into registry.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO _skill_tables_registry
		 (skill_name, table_name, display_name, description, schema_json, row_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		skillName, tableName, displayName, description, string(schemaJSON), now, now,
	)
	if err != nil {
		return fmt.Errorf("insert registry: %w", err)
	}

	return nil
}

// Insert inserts a new row into a skill's table and returns the generated ID.
func (s *SkillDB) Insert(skillName, tableName string, data map[string]any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate names.
	if err := validateName(skillName); err != nil {
		return "", fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return "", fmt.Errorf("invalid table name: %w", err)
	}

	fullName := fullTableName(skillName, tableName)

	// Start transaction for atomicity.
	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check table exists in registry.
	var schemaJSON string
	err = tx.QueryRow(
		"SELECT schema_json FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&schemaJSON)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return "", fmt.Errorf("check table: %w", err)
	}

	// Generate ID and timestamp.
	id := generateID()
	now := time.Now().UTC().Format(time.RFC3339)

	// Build INSERT statement.
	var columns []string
	var placeholders []string
	var values []any

	columns = append(columns, "id")
	placeholders = append(placeholders, "?")
	values = append(values, id)

	for colName, colValue := range data {
		if err := validateName(colName); err != nil {
			return "", fmt.Errorf("invalid column name %q: %w", colName, err)
		}
		columns = append(columns, colName)
		placeholders = append(placeholders, "?")
		values = append(values, colValue)
	}

	columns = append(columns, "created_at", "updated_at")
	placeholders = append(placeholders, "?", "?")
	values = append(values, now, now)

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		fullName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err = tx.Exec(insertSQL, values...)
	if err != nil {
		return "", fmt.Errorf("insert row: %w", err)
	}

	// Update row count.
	_, err = tx.Exec(
		"UPDATE _skill_tables_registry SET row_count = row_count + 1, updated_at = ? WHERE skill_name = ? AND table_name = ?",
		now, skillName, tableName,
	)
	if err != nil {
		return "", fmt.Errorf("update row count: %w", err)
	}

	// Commit transaction.
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transaction: %w", err)
	}

	return id, nil
}

// Query retrieves rows from a skill's table with optional filters and pagination.
func (s *SkillDB) Query(skillName, tableName string, filters map[string]any, limit int) ([]map[string]any, error) {
	opts := QueryOptions{
		Where: filters,
		Limit: limit,
	}
	return s.QueryWithOptions(skillName, tableName, opts)
}

// QueryWithOptions retrieves rows with full query options including pagination and ordering.
func (s *SkillDB) QueryWithOptions(skillName, tableName string, opts QueryOptions) ([]map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Validate names.
	if err := validateName(skillName); err != nil {
		return nil, fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}

	fullName := fullTableName(skillName, tableName)

	// Check table exists.
	var exists bool
	err := s.db.QueryRow(
		"SELECT 1 FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return nil, fmt.Errorf("check table: %w", err)
	}

	// Build SELECT statement.
	querySQL := fmt.Sprintf("SELECT * FROM %s", fullName)
	var whereClauses []string
	var values []any

	for colName, colValue := range opts.Where {
		if err := validateName(colName); err != nil {
			return nil, fmt.Errorf("invalid filter column %q: %w", colName, err)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("%s = ?", colName))
		values = append(values, colValue)
	}

	if len(whereClauses) > 0 {
		querySQL += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Apply ORDER BY.
	orderBy := "created_at"
	if opts.OrderBy != "" {
		if err := validateName(opts.OrderBy); err != nil {
			return nil, fmt.Errorf("invalid order_by column: %w", err)
		}
		orderBy = opts.OrderBy
	}
	order := "DESC"
	if strings.ToUpper(opts.Order) == "ASC" {
		order = "ASC"
	}
	querySQL += fmt.Sprintf(" ORDER BY %s %s", orderBy, order)

	// Apply limit with max.
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}
	querySQL += fmt.Sprintf(" LIMIT %d", limit)

	// Apply offset for pagination.
	if opts.Offset > 0 {
		querySQL += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	// Execute query.
	rows, err := s.db.Query(querySQL, values...)
	if err != nil {
		return nil, fmt.Errorf("query rows: %w", err)
	}
	defer rows.Close()

	// Get column names.
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	// Scan results with pre-allocated slice.
	var results []map[string]any
	for rows.Next() {
		rowValues := make([]any, len(columns))
		rowPointers := make([]any, len(columns))
		for i := range rowValues {
			rowPointers[i] = &rowValues[i]
		}

		if err := rows.Scan(rowPointers...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		rowMap := make(map[string]any)
		for i, col := range columns {
			val := rowValues[i]
			// Convert []byte to string.
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			rowMap[col] = val
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}

// GetByID retrieves a single row by ID.
func (s *SkillDB) GetByID(skillName, tableName, rowID string) (map[string]any, error) {
	// Validate row ID.
	if err := validateRowID(rowID); err != nil {
		return nil, err
	}
	results, err := s.Query(skillName, tableName, map[string]any{"id": rowID}, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("row with id %q not found", rowID)
	}
	return results[0], nil
}

// Update modifies a row in a skill's table.
func (s *SkillDB) Update(skillName, tableName, rowID string, data map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate row ID.
	if err := validateRowID(rowID); err != nil {
		return err
	}

	// Validate names.
	if err := validateName(skillName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	fullName := fullTableName(skillName, tableName)

	// Check table exists.
	var exists bool
	err := s.db.QueryRow(
		"SELECT 1 FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return fmt.Errorf("check table: %w", err)
	}

	// Build UPDATE statement.
	var setClauses []string
	var values []any

	for colName, colValue := range data {
		if err := validateName(colName); err != nil {
			return fmt.Errorf("invalid column name %q: %w", colName, err)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", colName))
		values = append(values, colValue)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no columns to update")
	}

	// Add updated_at.
	setClauses = append(setClauses, "updated_at = ?")
	values = append(values, time.Now().UTC().Format(time.RFC3339))

	// Add WHERE clause.
	values = append(values, rowID)

	updateSQL := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = ?",
		fullName,
		strings.Join(setClauses, ", "),
	)

	result, err := s.db.Exec(updateSQL, values...)
	if err != nil {
		return fmt.Errorf("update row: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("row with id %q not found", rowID)
	}

	// Update registry updated_at.
	_, err = s.db.Exec(
		"UPDATE _skill_tables_registry SET updated_at = ? WHERE skill_name = ? AND table_name = ?",
		time.Now().UTC().Format(time.RFC3339), skillName, tableName,
	)
	if err != nil {
		return fmt.Errorf("update registry: %w", err)
	}

	return nil
}

// Delete removes a row from a skill's table.
func (s *SkillDB) Delete(skillName, tableName, rowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate row ID.
	if err := validateRowID(rowID); err != nil {
		return err
	}

	// Validate names.
	if err := validateName(skillName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	fullName := fullTableName(skillName, tableName)

	// Start transaction for atomicity.
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check table exists.
	var exists bool
	err = tx.QueryRow(
		"SELECT 1 FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return fmt.Errorf("check table: %w", err)
	}

	// Execute DELETE.
	result, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", fullName), rowID)
	if err != nil {
		return fmt.Errorf("delete row: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("row with id %q not found", rowID)
	}

	// Update row count.
	_, err = tx.Exec(
		"UPDATE _skill_tables_registry SET row_count = row_count - 1, updated_at = ? WHERE skill_name = ? AND table_name = ?",
		time.Now().UTC().Format(time.RFC3339), skillName, tableName,
	)
	if err != nil {
		return fmt.Errorf("update row count: %w", err)
	}

	// Commit transaction.
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// DropTable removes a table and all its data.
func (s *SkillDB) DropTable(skillName, tableName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate names.
	if err := validateName(skillName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	fullName := fullTableName(skillName, tableName)

	// Check table exists in registry.
	var exists bool
	err := s.db.QueryRow(
		"SELECT 1 FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return fmt.Errorf("check table: %w", err)
	}

	// Drop the table.
	if _, err := s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", fullName)); err != nil {
		return fmt.Errorf("drop table: %w", err)
	}

	// Drop the index (SQLite drops automatically, but be explicit).
	if _, err := s.db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS idx_%s_created", fullName)); err != nil {
		// Ignore index drop errors.
	}

	// Remove from registry.
	_, err = s.db.Exec(
		"DELETE FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	)
	if err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}

	return nil
}

// ListTables returns all tables for a skill.
// If skillName is empty, returns all tables from all skills.
func (s *SkillDB) ListTables(skillName string) ([]TableInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rows *sql.Rows
	var err error

	if skillName != "" {
		if err := validateName(skillName); err != nil {
			return nil, fmt.Errorf("invalid skill name: %w", err)
		}
		rows, err = s.db.Query(
			"SELECT skill_name, table_name, display_name, description, schema_json, row_count, created_at, updated_at FROM _skill_tables_registry WHERE skill_name = ? ORDER BY table_name",
			skillName,
		)
	} else {
		rows, err = s.db.Query(
			"SELECT skill_name, table_name, display_name, description, schema_json, row_count, created_at, updated_at FROM _skill_tables_registry ORDER BY skill_name, table_name",
		)
	}

	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var schemaJSON string
		err := rows.Scan(&t.SkillName, &t.TableName, &t.DisplayName, &t.Description, &schemaJSON, &t.RowCount, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		if schemaJSON != "" && schemaJSON != "{}" {
			if err := json.Unmarshal([]byte(schemaJSON), &t.Schema); err != nil {
				// Log warning but don't fail - schema is for documentation only.
				t.Schema = make(map[string]string)
			}
		}
		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return tables, nil
}

// DescribeTable returns detailed information about a table.
func (s *SkillDB) DescribeTable(skillName, tableName string) (*TableInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Validate names.
	if err := validateName(skillName); err != nil {
		return nil, fmt.Errorf("invalid skill name: %w", err)
	}
	if err := validateName(tableName); err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}

	var t TableInfo
	var schemaJSON string
	err := s.db.QueryRow(
		"SELECT skill_name, table_name, display_name, description, schema_json, row_count, created_at, updated_at FROM _skill_tables_registry WHERE skill_name = ? AND table_name = ?",
		skillName, tableName,
	).Scan(&t.SkillName, &t.TableName, &t.DisplayName, &t.Description, &schemaJSON, &t.RowCount, &t.CreatedAt, &t.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("table %q not found for skill %q", tableName, skillName)
	}
	if err != nil {
		return nil, fmt.Errorf("query table: %w", err)
	}

	if schemaJSON != "" && schemaJSON != "{}" {
		if err := json.Unmarshal([]byte(schemaJSON), &t.Schema); err != nil {
			// Log warning but don't fail.
			t.Schema = make(map[string]string)
		}
	}

	return &t, nil
}

// generateID creates a short unique identifier.
func generateID() string {
	return uuid.New().String()[:8]
}
