// Package copilot – skill_db_tools.go registers tools that allow the agent
// to use the skill database for storing and retrieving structured data.
package copilot

import (
	"context"
	"fmt"
)

// RegisterSkillDBTools registers all skill database tools in the executor.
// These tools allow skills to create tables and perform CRUD operations
// on their data without requiring SQL knowledge.
func RegisterSkillDBTools(executor *ToolExecutor, skillDB *SkillDB) {
	if skillDB == nil {
		return
	}

	// skill_db_create_table - create a new table for a skill
	executor.Register(
		MakeToolDefinition("skill_db_create_table", "Create a new table for a skill with custom columns. Each skill can have multiple tables. The table is automatically prefixed with the skill name (e.g., 'crm_contacts' for skill 'crm' table 'contacts').", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns this table (lowercase, letters, numbers, underscores only)",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, letters, numbers, underscores only)",
				},
				"display_name": map[string]any{
					"type":        "string",
					"description": "Human-readable name for the table (optional)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Description of what this table stores (optional)",
				},
				"columns": map[string]any{
					"type":        "object",
					"description": "Column definitions. Keys are column names, values are SQL types like 'TEXT', 'TEXT NOT NULL', 'INTEGER', 'TEXT DEFAULT \"value\"'",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"skill_name", "table_name", "columns"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)
			displayName, _ := args["display_name"].(string)
			description, _ := args["description"].(string)

			columnsRaw, ok := args["columns"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("columns must be an object")
			}

			columns := make(map[string]string)
			for k, v := range columnsRaw {
				vs, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("column %q type must be a string", k)
				}
				columns[k] = vs
			}

			err := skillDB.CreateTable(skillName, tableName, displayName, description, columns)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Table '%s' created successfully for skill '%s'. Full table name: %s_%s",
				tableName, skillName, skillName, tableName), nil
		},
	)

	// skill_db_insert - insert a record into a table
	executor.Register(
		MakeToolDefinition("skill_db_insert", "Insert a new record into a skill's table. Returns the generated ID of the new record.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Record data as key-value pairs. Keys must match column names.",
					"additionalProperties": map[string]any{},
				},
			},
			"required": []string{"skill_name", "table_name", "data"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)

			data, ok := args["data"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("data must be an object")
			}

			id, err := skillDB.Insert(skillName, tableName, data)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"id":      id,
				"message": fmt.Sprintf("Record inserted with ID %s", id),
			}, nil
		},
	)

	// skill_db_query - query records from a table
	executor.Register(
		MakeToolDefinition("skill_db_query", "Query records from a skill's table. Use filters to narrow results. Supports pagination with offset and ordering.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table",
				},
				"where": map[string]any{
					"type":        "object",
					"description": "Filter conditions as key-value pairs (AND logic). Example: {\"status\": \"novo\"}",
					"additionalProperties": map[string]any{},
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of records to return (default: 100, max: 1000)",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Number of records to skip for pagination (default: 0)",
				},
				"order_by": map[string]any{
					"type":        "string",
					"description": "Column to sort results by (default: created_at)",
				},
				"order": map[string]any{
					"type":        "string",
					"description": "Sort direction: 'ASC' or 'DESC' (default: DESC)",
					"enum":        []string{"ASC", "DESC"},
				},
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)

			opts := QueryOptions{}

			if f, ok := args["where"].(map[string]any); ok {
				opts.Where = f
			}

			if l, ok := args["limit"].(float64); ok {
				opts.Limit = int(l)
			}

			if o, ok := args["offset"].(float64); ok {
				opts.Offset = int(o)
			}

			if ob, ok := args["order_by"].(string); ok {
				opts.OrderBy = ob
			}

			if od, ok := args["order"].(string); ok {
				opts.Order = od
			}

			results, err := skillDB.QueryWithOptions(skillName, tableName, opts)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"count":   len(results),
				"records": results,
			}, nil
		},
	)

	// skill_db_update - update records in a table
	executor.Register(
		MakeToolDefinition("skill_db_update", "Update a specific record in a skill's table by ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table",
				},
				"row_id": map[string]any{
					"type":        "string",
					"description": "ID of the record to update",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Fields to update as key-value pairs",
					"additionalProperties": map[string]any{},
				},
			},
			"required": []string{"skill_name", "table_name", "row_id", "data"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)
			rowID, _ := args["row_id"].(string)

			data, ok := args["data"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("data must be an object")
			}

			err := skillDB.Update(skillName, tableName, rowID, data)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"success": true,
				"message": fmt.Sprintf("Record %s updated successfully", rowID),
			}, nil
		},
	)

	// skill_db_delete - delete a record from a table
	executor.Register(
		MakeToolDefinition("skill_db_delete", "Delete a specific record from a skill's table by ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table",
				},
				"row_id": map[string]any{
					"type":        "string",
					"description": "ID of the record to delete",
				},
			},
			"required": []string{"skill_name", "table_name", "row_id"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)
			rowID, _ := args["row_id"].(string)

			err := skillDB.Delete(skillName, tableName, rowID)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"success": true,
				"message": fmt.Sprintf("Record %s deleted successfully", rowID),
			}, nil
		},
	)

	// skill_db_list_tables - list tables for a skill
	executor.Register(
		MakeToolDefinition("skill_db_list_tables", "List all tables for a specific skill, or all tables across all skills if skill_name is empty.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill to list tables for (optional - if empty, lists all tables)",
				},
			},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)

			tables, err := skillDB.ListTables(skillName)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"count":  len(tables),
				"tables": tables,
			}, nil
		},
	)

	// skill_db_describe - describe a table's structure
	executor.Register(
		MakeToolDefinition("skill_db_describe", "Get detailed information about a table's structure, including column definitions and row count.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table to describe",
				},
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)

			info, err := skillDB.DescribeTable(skillName, tableName)
			if err != nil {
				return nil, err
			}

			return info, nil
		},
	)

	// skill_db_drop_table - drop a table
	executor.Register(
		MakeToolDefinition("skill_db_drop_table", "Permanently delete a table and all its data. This cannot be undone.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill that owns the table",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table to drop",
				},
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			skillName, _ := args["skill_name"].(string)
			tableName, _ := args["table_name"].(string)

			err := skillDB.DropTable(skillName, tableName)
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"success": true,
				"message": fmt.Sprintf("Table %s_%s dropped successfully", skillName, tableName),
			}, nil
		},
	)
}
