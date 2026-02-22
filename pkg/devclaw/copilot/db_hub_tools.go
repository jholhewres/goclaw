// Package copilot – db_hub_tools.go implements database hub management tools.
// These tools use the native Database Hub with Go drivers for better performance
// than the CLI-based db_tools.go tools.
package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/database"
)

// dbHubRateLimiter provides simple rate limiting for database hub tools.
// Prevents abuse of expensive operations like db_hub_raw.
type dbHubRateLimiter struct {
	mu         sync.Mutex
	lastCall   map[string]time.Time
	minInterval time.Duration
}

// globalRateLimiter is shared across all tool invocations (10 ops/sec per session)
var globalRateLimiter = &dbHubRateLimiter{
	lastCall:    make(map[string]time.Time),
	minInterval: 100 * time.Millisecond,
}

// Allow checks if the operation is allowed for the given session
func (r *dbHubRateLimiter) Allow(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if last, ok := r.lastCall[sessionID]; ok {
		if now.Sub(last) < r.minInterval {
			return false
		}
	}
	r.lastCall[sessionID] = now

	// Cleanup old entries periodically
	if len(r.lastCall) > 1000 {
		cutoff := now.Add(-5 * time.Minute)
		for k, v := range r.lastCall {
			if v.Before(cutoff) {
				delete(r.lastCall, k)
			}
		}
	}
	return true
}

// RegisterDBHubTools registers database hub management tools.
// These tools operate on the internal database hub, not external databases.
func RegisterDBHubTools(executor *ToolExecutor, hub *database.Hub) {
	if hub == nil {
		return
	}

	// db_hub_status
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_status",
			Description: "Check health status of all database backends in the hub. Shows connection info, latency, and health for each backend.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"backend": map[string]any{
						"type":        "string",
						"description": "Specific backend name (optional, defaults to all backends)",
					},
				},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		backendName, _ := args["backend"].(string)

		if backendName != "" {
			backend, err := hub.GetBackend(backendName)
			if err != nil {
				return nil, err
			}
			if backend.Health == nil {
				return map[string]any{
					"name":    backend.Name,
					"type":    backend.Type,
					"healthy": true,
					"message": "health checker not available",
				}, nil
			}
			status := backend.Health.Status(ctx)
			return map[string]any{
				"name":    backend.Name,
				"type":    backend.Type,
				"healthy": status.Healthy,
				"latency": status.Latency.String(),
				"version": status.Version,
				"error":   status.Error,
			}, nil
		}

		// Return all backends
		status := hub.Status(ctx)
		result := make([]map[string]any, 0, len(status))
		for name, s := range status {
			result = append(result, map[string]any{
				"name":    name,
				"healthy": s.Healthy,
				"latency": s.Latency.String(),
				"version": s.Version,
				"error":   s.Error,
			})
		}
		return result, nil
	})

	// db_hub_query
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_query",
			Description: "Execute a SQL SELECT query on the database hub using native Go drivers (faster than CLI-based db_query).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "SQL SELECT query to execute",
					},
					"backend": map[string]any{
						"type":        "string",
						"description": "Backend name (optional, defaults to primary)",
					},
					"max_rows": map[string]any{
						"type":        "integer",
						"description": "Maximum rows to return (default: 100)",
					},
				},
				"required": []string{"query"},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		query, _ := args["query"].(string)
		backendName, _ := args["backend"].(string)
		maxRows, _ := args["max_rows"].(int)
		if maxRows == 0 {
			maxRows = 100
		}

		// Validate it's a SELECT query
		normalizedQuery := strings.TrimSpace(strings.ToUpper(query))
		if !strings.HasPrefix(normalizedQuery, "SELECT") &&
			!strings.HasPrefix(normalizedQuery, "PRAGMA") &&
			!strings.HasPrefix(normalizedQuery, "SHOW") &&
			!strings.HasPrefix(normalizedQuery, "EXPLAIN") {
			return nil, fmt.Errorf("only SELECT queries are allowed with db_hub_query; use db_hub_execute for mutations")
		}

		rows, err := hub.Query(ctx, backendName, query)
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("get columns: %w", err)
		}

		var results []map[string]any
		count := 0

		for rows.Next() {
			if count >= maxRows {
				break
			}

			values := make([]any, len(columns))
			valuePtrs := make([]any, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("scan row: %w", err)
			}

			row := make(map[string]any)
			for i, col := range columns {
				val := values[i]
				// Convert []byte to string
				if b, ok := val.([]byte); ok {
					val = string(b)
				}
				row[col] = val
			}
			results = append(results, row)
			count++
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows error: %w", err)
		}

		return map[string]any{
			"columns": columns,
			"rows":    results,
			"count":   len(results),
			"truncated": count >= maxRows && rows.Next(),
		}, nil
	})

	// db_hub_execute
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_execute",
			Description: "Execute a mutating SQL statement (INSERT, UPDATE, DELETE, CREATE, etc.) on the database hub.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"statement": map[string]any{
						"type":        "string",
						"description": "SQL statement to execute",
					},
					"backend": map[string]any{
						"type":        "string",
						"description": "Backend name (optional, defaults to primary)",
					},
				},
				"required": []string{"statement"},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		statement, _ := args["statement"].(string)
		backendName, _ := args["backend"].(string)

		result, err := hub.Exec(ctx, backendName, statement)
		if err != nil {
			return nil, fmt.Errorf("execute failed: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		lastInsertID, _ := result.LastInsertId()

		return map[string]any{
			"rows_affected": rowsAffected,
			"last_insert_id": lastInsertID,
			"success":       true,
		}, nil
	})

	// db_hub_schema
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_schema",
			Description: "Get database schema: list tables, or describe a specific table's columns.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table": map[string]any{
						"type":        "string",
						"description": "Table name to describe (omit to list all tables)",
					},
					"backend": map[string]any{
						"type":        "string",
						"description": "Backend name (optional, defaults to primary)",
					},
				},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		table, _ := args["table"].(string)
		backendName, _ := args["backend"].(string)

		backend, err := hub.GetBackend(backendName)
		if err != nil {
			return nil, err
		}

		// Sanitize table name to prevent SQL injection
		if table != "" {
			// Only allow alphanumeric and underscore
			for _, r := range table {
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
					return nil, fmt.Errorf("invalid table name: %s (only alphanumeric and underscore allowed)", table)
				}
			}
		}

		var query string
		switch backend.Type {
		case database.BackendSQLite:
			if table == "" {
				query = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;"
			} else {
				query = fmt.Sprintf("PRAGMA table_info(%s);", table)
			}
		case database.BackendPostgreSQL:
			if table == "" {
				query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;"
			} else {
				query = fmt.Sprintf("SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '%s' ORDER BY ordinal_position;", table)
			}
		case database.BackendMySQL:
			if table == "" {
				query = "SHOW TABLES;"
			} else {
				query = fmt.Sprintf("DESCRIBE %s;", table)
			}
		default:
			return nil, fmt.Errorf("unsupported backend type: %s", backend.Type)
		}

		rows, err := backend.DB.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("schema query failed: %w", err)
		}
		defer rows.Close()

		columns, _ := rows.Columns()
		var results []map[string]any

		for rows.Next() {
			values := make([]any, len(columns))
			valuePtrs := make([]any, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}

			row := make(map[string]any)
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					val = string(b)
				}
				row[col] = val
			}
			results = append(results, row)
		}

		return map[string]any{
			"query":   query,
			"columns": columns,
			"rows":    results,
		}, nil
	})

	// db_hub_migrate
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_migrate",
			Description: "Run database schema migrations. Checks current version and applies pending migrations.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"backend": map[string]any{
						"type":        "string",
						"description": "Backend name (optional, defaults to primary)",
					},
					"target": map[string]any{
						"type":        "integer",
						"description": "Target version (0 = latest)",
					},
					"dry_run": map[string]any{
						"type":        "boolean",
						"description": "Only check what migrations would be applied (default: false)",
					},
				},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		backendName, _ := args["backend"].(string)
		target, _ := args["target"].(int)
		dryRun, _ := args["dry_run"].(bool)

		backend, err := hub.GetBackend(backendName)
		if err != nil {
			return nil, err
		}

		if backend.Migrator == nil {
			return nil, fmt.Errorf("migrator not available for backend %q", backend.Name)
		}

		currentVersion, err := backend.Migrator.CurrentVersion(ctx)
		if err != nil {
			return nil, fmt.Errorf("get current version: %w", err)
		}

		needsMigration, err := backend.Migrator.NeedsMigration(ctx)
		if err != nil {
			return nil, fmt.Errorf("check migration status: %w", err)
		}

		if dryRun {
			return map[string]any{
				"current_version":  currentVersion,
				"needs_migration":  needsMigration,
				"dry_run":          true,
				"message":          "Use dry_run=false to apply migrations",
			}, nil
		}

		if !needsMigration {
			return map[string]any{
				"current_version":  currentVersion,
				"needs_migration":  false,
				"message":          "Database is up to date",
			}, nil
		}

		if err := backend.Migrator.Migrate(ctx, target); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}

		newVersion, _ := backend.Migrator.CurrentVersion(ctx)

		return map[string]any{
			"previous_version": currentVersion,
			"current_version":  newVersion,
			"success":          true,
		}, nil
	})

	// db_hub_backup
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_backup",
			Description: "Create a backup of the database (SQLite only). Saves to the specified path.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"output": map[string]any{
						"type":        "string",
						"description": "Output path for backup file (default: ./data/backups/devclaw-TIMESTAMP.db)",
					},
				},
				"required": []string{},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		outputPath, _ := args["output"].(string)

		backend := hub.Primary()
		if backend == nil {
			return nil, fmt.Errorf("no primary backend available")
		}

		if backend.Type != database.BackendSQLite {
			return nil, fmt.Errorf("backup only supported for SQLite backend")
		}

		if outputPath == "" {
			timestamp := time.Now().Format("20060102-150405")
			outputPath = fmt.Sprintf("./data/backups/devclaw-%s.db", timestamp)
		}

		// Sanitize path to prevent path traversal and SQL injection
		outputPath = filepath.Clean(outputPath)
		if strings.Contains(outputPath, "..") {
			return nil, fmt.Errorf("invalid backup path: path traversal not allowed")
		}

		// Ensure backup directory exists
		backupDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return nil, fmt.Errorf("create backup directory: %w", err)
		}

		// For SQLite, we can use the backup API or VACUUM INTO
		// Quote the path to prevent SQL injection
		quotedPath := strings.ReplaceAll(outputPath, "'", "''")
		_, err := backend.DB.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s';", quotedPath))
		if err != nil {
			return nil, fmt.Errorf("backup failed: %w", err)
		}

		return map[string]any{
			"success":     true,
			"backup_path": outputPath,
			"backend":     backend.Name,
			"timestamp":   time.Now().Format(time.RFC3339),
		}, nil
	})

	// db_hub_backends
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_backends",
			Description: "List all registered database backends with their types and capabilities.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		backends := hub.ListBackends()
		result := make([]map[string]any, 0, len(backends))

		for _, name := range backends {
			backend, err := hub.GetBackend(name)
			if err != nil {
				continue
			}

			info := map[string]any{
				"name":           backend.Name,
				"type":           backend.Type,
				"has_migrator":   backend.Migrator != nil,
				"has_health":     backend.Health != nil,
				"has_vector":     backend.Vector != nil,
				"vector_support": backend.Vector != nil && backend.Vector.SupportsVector(),
			}

			if backend.Health != nil {
				status := backend.Health.Status(ctx)
				info["healthy"] = status.Healthy
				info["version"] = status.Version
			}

			result = append(result, info)
		}

		return result, nil
	})

	// db_hub_raw - direct SQL access for debugging (rate-limited)
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_hub_raw",
			Description: "Execute raw SQL and return results as-is. For debugging and advanced use cases. Rate-limited to 10 ops/sec.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "SQL query or statement",
					},
					"backend": map[string]any{
						"type":        "string",
						"description": "Backend name (optional, defaults to primary)",
					},
				},
				"required": []string{"query"},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		// Rate limiting - extract session ID from context or use default
		sessionID := extractSessionID(ctx)
		if !globalRateLimiter.Allow(sessionID) {
			return nil, fmt.Errorf("rate limit exceeded: too many database operations, please wait a moment")
		}

		query, _ := args["query"].(string)
		backendName, _ := args["backend"].(string)

		backend, err := hub.GetBackend(backendName)
		if err != nil {
			return nil, err
		}

		// Check if it's a query (returns rows) or exec (returns result)
		normalizedQuery := strings.TrimSpace(strings.ToUpper(query))
		isQuery := strings.HasPrefix(normalizedQuery, "SELECT") ||
			strings.HasPrefix(normalizedQuery, "PRAGMA") ||
			strings.HasPrefix(normalizedQuery, "SHOW") ||
			strings.HasPrefix(normalizedQuery, "EXPLAIN") ||
			strings.HasPrefix(normalizedQuery, "WITH")

		if isQuery {
			return executeRawQuery(ctx, backend.DB, query)
		}

		result, err := backend.DB.ExecContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("exec failed: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		lastInsertID, _ := result.LastInsertId()

		return map[string]any{
			"type":            "exec",
			"rows_affected":   rowsAffected,
			"last_insert_id":  lastInsertID,
		}, nil
	})
}

func executeRawQuery(ctx context.Context, db *sql.DB, query string) (any, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var results []map[string]any

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]any)
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return map[string]any{
		"type":    "query",
		"columns": columns,
		"rows":    results,
		"count":   len(results),
	}, nil
}

// sessionIDKey is the context key for session ID
type sessionIDKey struct{}

// extractSessionID extracts the session ID from context, or returns "default"
func extractSessionID(ctx context.Context) string {
	if v := ctx.Value(sessionIDKey{}); v != nil {
		if id, ok := v.(string); ok && id != "" {
			return id
		}
	}
	return "default"
}
