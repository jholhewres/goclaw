---
name: skill-db
description: "Database system for skills to store and retrieve structured data like contacts, tasks, and notes"
trigger: automatic
---

# Skill Database

A built-in database system that allows skills to store structured data without requiring SQL knowledge or custom scripts.

## Architecture
```
./data/skill_database.db
├── _skill_tables_registry    # Metadata about tables
└── {skill}_{table}           # Dynamic tables per skill
    ├── id (TEXT PRIMARY KEY)
    ├── {custom_columns}
    ├── created_at (TEXT)
    └── updated_at (TEXT)
```

## Tools

| Tool | Action | Description |
|------|--------|-------------|
| `skill_db_create_table` | Create | Create a new table with custom columns |
| `skill_db_insert` | Create | Insert a new record, returns ID |
| `skill_db_query` | Read | Query records with optional filters |
| `skill_db_update` | Update | Update a record by ID |
| `skill_db_delete` | Delete | Delete a record by ID |
| `skill_db_list_tables` | Read | List tables for a skill |
| `skill_db_describe` | Read | View table structure |
| `skill_db_drop_table` | Delete | Permanently drop a table |

## Creating Tables

```bash
skill_db_create_table(
    skill_name="crm",
    table_name="contacts",
    display_name="Contatos",
    description="Lista de contatos do CRM",
    columns={
        "name": "TEXT NOT NULL",
        "email": "TEXT",
        "phone": "TEXT",
        "status": "TEXT DEFAULT 'novo'",
        "tags": "TEXT"
    }
)
# Creates table: crm_contacts
```

### Column Types
- `TEXT` - String values
- `INTEGER` - Numeric values
- `REAL` - Floating point
- `TEXT NOT NULL` - Required field
- `TEXT DEFAULT 'value'` - With default value

## Inserting Records

```bash
skill_db_insert(
    skill_name="crm",
    table_name="contacts",
    data={
        "name": "João Silva",
        "email": "joao@empresa.com",
        "status": "novo"
    }
)
# Returns: {"id": "a1b2c3d4", "message": "Record inserted with ID a1b2c3d4"}
```

## Querying Records

### Query All
```bash
skill_db_query(skill_name="crm", table_name="contacts")
# Returns: {"count": 5, "records": [...]}
```

### Query with Filters
```bash
skill_db_query(
    skill_name="crm",
    table_name="contacts",
    where={"status": "novo"},
    limit=10
)
```

## Updating Records

```bash
skill_db_update(
    skill_name="crm",
    table_name="contacts",
    row_id="a1b2c3d4",
    data={"status": "contatado"}
)
```

## Deleting Records

```bash
skill_db_delete(
    skill_name="crm",
    table_name="contacts",
    row_id="a1b2c3d4"
)
```

## Managing Tables

### List Tables
```bash
# List tables for a specific skill
skill_db_list_tables(skill_name="crm")

# List all tables from all skills
skill_db_list_tables(skill_name="")
```

### Describe Table
```bash
skill_db_describe(skill_name="crm", table_name="contacts")
# Returns: columns, row_count, created_at, updated_at
```

### Drop Table
```bash
skill_db_drop_table(skill_name="crm", table_name="contacts")
# WARNING: Permanently deletes all data!
```

## Common Patterns

### CRM (Contact Management)
```bash
# Create table
skill_db_create_table(skill_name="crm", table_name="contacts", columns={
    "name": "TEXT NOT NULL",
    "email": "TEXT",
    "phone": "TEXT",
    "company": "TEXT",
    "status": "TEXT DEFAULT 'lead'",
    "notes": "TEXT"
})

# Add contact
skill_db_insert(skill_name="crm", table_name="contacts", data={
    "name": "Maria Santos",
    "email": "maria@company.com",
    "status": "lead"
})

# Find leads
skill_db_query(skill_name="crm", table_name="contacts", where={"status": "lead"})

# Update status
skill_db_update(skill_name="crm", table_name="contacts", row_id="abc123",
    data={"status": "cliente"})
```

### Task Manager
```bash
skill_db_create_table(skill_name="tasks", table_name="items", columns={
    "title": "TEXT NOT NULL",
    "description": "TEXT",
    "status": "TEXT DEFAULT 'pending'",
    "priority": "INTEGER DEFAULT 3",
    "due_date": "TEXT"
})

skill_db_insert(skill_name="tasks", table_name="items", data={
    "title": "Revisar proposta",
    "priority": 1,
    "due_date": "2026-02-26"
})
```

### Notes/Journal
```bash
skill_db_create_table(skill_name="notes", table_name="entries", columns={
    "title": "TEXT",
    "content": "TEXT",
    "category": "TEXT DEFAULT 'general'",
    "tags": "TEXT"
})

skill_db_insert(skill_name="notes", table_name="entries", data={
    "title": "Reunião com cliente",
    "content": "Discutimos o projeto X...",
    "category": "reuniao"
})
```

## Tips
- Use meaningful skill and table names (lowercase, underscores)
- Always include a status or category column for filtering
- Use DEFAULT values for common states
- Query with filters to avoid loading all data
- Use tags as TEXT with JSON array for multiple tags

## Limitations
- No relations between tables (each skill is isolated)
- No ORDER BY or OFFSET (use filters and limit)
- Maximum table name: 64 characters combined (skill_table)
- Only TEXT, INTEGER, REAL types supported

## Security
- Each skill can only access its own tables
- Table names are validated (lowercase, alphanumeric, underscore)
- SQL injection is prevented via prepared statements
