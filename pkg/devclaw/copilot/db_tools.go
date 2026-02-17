// Package copilot – db_tools.go implements database tools for querying
// PostgreSQL, MySQL, and SQLite databases. Uses CLI clients (psql, mysql,
// sqlite3) to avoid heavy driver dependencies.
package copilot

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RegisterDBTools registers database query and management tools.
func RegisterDBTools(executor *ToolExecutor) {
	// db_query
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_query",
			Description: "Execute a SQL query against a database (PostgreSQL, MySQL, or SQLite). Returns results as text table.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"engine": map[string]any{"type": "string", "enum": []string{"postgres", "mysql", "sqlite"}, "description": "Database engine"},
					"query":  map[string]any{"type": "string", "description": "SQL query to execute"},
					"dsn":    map[string]any{"type": "string", "description": "Connection string. PostgreSQL: 'host=... dbname=... user=...', MySQL: 'user:pass@tcp(host)/db', SQLite: file path"},
				},
				"required": []string{"engine", "query", "dsn"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		engine, _ := args["engine"].(string)
		query, _ := args["query"].(string)
		dsn, _ := args["dsn"].(string)

		if isMutatingQuery(query) {
			return nil, fmt.Errorf("mutating queries (INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE) require db_execute tool")
		}

		return executeDBQuery(engine, query, dsn)
	})

	// db_execute
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_execute",
			Description: "Execute a mutating SQL statement (INSERT, UPDATE, DELETE, CREATE, etc.) against a database.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"engine":    map[string]any{"type": "string", "enum": []string{"postgres", "mysql", "sqlite"}, "description": "Database engine"},
					"statement": map[string]any{"type": "string", "description": "SQL statement to execute"},
					"dsn":       map[string]any{"type": "string", "description": "Connection string"},
				},
				"required": []string{"engine", "statement", "dsn"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		engine, _ := args["engine"].(string)
		statement, _ := args["statement"].(string)
		dsn, _ := args["dsn"].(string)

		return executeDBQuery(engine, statement, dsn)
	})

	// db_schema
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_schema",
			Description: "Get database schema: list tables, or describe a specific table's columns and types.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"engine": map[string]any{"type": "string", "enum": []string{"postgres", "mysql", "sqlite"}, "description": "Database engine"},
					"dsn":    map[string]any{"type": "string", "description": "Connection string"},
					"table":  map[string]any{"type": "string", "description": "Table name to describe (omit for table list)"},
				},
				"required": []string{"engine", "dsn"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		engine, _ := args["engine"].(string)
		dsn, _ := args["dsn"].(string)
		table, _ := args["table"].(string)

		var query string

		switch engine {
		case "postgres":
			if table == "" {
				query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;"
			} else {
				query = fmt.Sprintf("SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '%s' ORDER BY ordinal_position;", table)
			}

		case "mysql":
			if table == "" {
				query = "SHOW TABLES;"
			} else {
				query = fmt.Sprintf("DESCRIBE %s;", table)
			}

		case "sqlite":
			if table == "" {
				query = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;"
			} else {
				query = fmt.Sprintf("PRAGMA table_info(%s);", table)
			}

		default:
			return nil, fmt.Errorf("unsupported engine: %s", engine)
		}

		return executeDBQuery(engine, query, dsn)
	})

	// db_connections
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "db_connections",
			Description: "Check active database connections and connection info (PostgreSQL only).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dsn": map[string]any{"type": "string", "description": "PostgreSQL connection string"},
				},
				"required": []string{"dsn"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		dsn, _ := args["dsn"].(string)
		query := "SELECT pid, usename, application_name, client_addr, state, query_start, LEFT(query, 100) as query FROM pg_stat_activity WHERE datname = current_database() ORDER BY query_start DESC LIMIT 20;"
		return executeDBQuery("postgres", query, dsn)
	})
}

func executeDBQuery(engine, query, dsn string) (any, error) {
	var cmd *exec.Cmd

	switch engine {
	case "postgres":
		cmd = exec.Command("psql", dsn, "-c", query, "--no-align", "--tuples-only")
		// Add header for non-tuples output
		if !isMutatingQuery(query) {
			cmd = exec.Command("psql", dsn, "-c", query)
		}

	case "mysql":
		parts := parseMySQLDSN(dsn)
		args := []string{"-e", query}
		if parts["host"] != "" {
			args = append(args, "-h", parts["host"])
		}
		if parts["port"] != "" {
			args = append(args, "-P", parts["port"])
		}
		if parts["user"] != "" {
			args = append(args, "-u", parts["user"])
		}
		if parts["password"] != "" {
			args = append(args, fmt.Sprintf("-p%s", parts["password"]))
		}
		if parts["database"] != "" {
			args = append(args, parts["database"])
		}
		cmd = exec.Command("mysql", args...)

	case "sqlite":
		cmd = exec.Command("sqlite3", dsn, query)

	default:
		return nil, fmt.Errorf("unsupported engine: %s", engine)
	}

	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return nil, fmt.Errorf("%s error: %s", engine, result)
		}
		return nil, fmt.Errorf("%s error: %w", engine, err)
	}

	if result == "" {
		return "Query executed successfully. No output.", nil
	}

	// Truncate very long output
	const maxLen = 8000
	if len(result) > maxLen {
		result = result[:maxLen] + "\n\n... (truncated, use LIMIT clause)"
	}
	return result, nil
}

func isMutatingQuery(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	mutators := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE", "CREATE"}
	for _, m := range mutators {
		if strings.HasPrefix(upper, m) {
			return true
		}
	}
	return false
}

// parseMySQLDSN parses a simplified MySQL DSN format: user:pass@tcp(host:port)/dbname
func parseMySQLDSN(dsn string) map[string]string {
	result := map[string]string{}

	// user:pass@tcp(host:port)/database
	if at := strings.Index(dsn, "@"); at > 0 {
		userPass := dsn[:at]
		rest := dsn[at+1:]

		if colon := strings.Index(userPass, ":"); colon > 0 {
			result["user"] = userPass[:colon]
			result["password"] = userPass[colon+1:]
		} else {
			result["user"] = userPass
		}

		// tcp(host:port)/database
		rest = strings.TrimPrefix(rest, "tcp(")
		if paren := strings.Index(rest, ")"); paren > 0 {
			hostPort := rest[:paren]
			rest = rest[paren+1:]
			if colon := strings.Index(hostPort, ":"); colon > 0 {
				result["host"] = hostPort[:colon]
				result["port"] = hostPort[colon+1:]
			} else {
				result["host"] = hostPort
			}
		}

		result["database"] = strings.TrimPrefix(rest, "/")
	} else {
		result["database"] = dsn
	}

	return result
}
