---
name: skills
description: "Create, install, and manage agent skills to extend capabilities"
trigger: automatic
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

## IMPORTANT: Always Use the skill_manage Tool

**NEVER create skills with bash commands (mkdir, echo, etc.)**

ALWAYS use `skill_manage(action=init)` to create skills. The tool handles:
- Directory creation in the correct location
- SKILL.md template generation
- Database table creation (if requested)

## Communication Guidelines

**When creating skills for users:**
1. NEVER show technical tool syntax in chat responses
2. ALWAYS ask: "Do you want this skill to have a database for storing structured data (contacts, tasks, etc.)?"
3. Explain the options:
   - **With database**: persistent data, structured queries, ideal for CRM, tasks, contacts
   - **Without database**: uses memory/vault, ideal for API integrations, automations

## Architecture
```
                      Agent Context
                           |
        +------------------+------------------+
        |                  |                  |
        v                  v                  v
+---------------+  +---------------+  +---------------+
|   CREATE      |  |   DISCOVER    |  |   MANAGE      |
+---------------+  +---------------+  +---------------+
| action=init   |  | action=search |  | action=list   |
| action=edit   |  | action=       |  | action=test   |
| action=       |  |  defaults_list|  | action=remove |
|  add_script   |  | action=install|  |               |
+---------------+  +---------------+  +---------------+
```

## Skill Structure
```
./skills/my-skill/           # Skills are created in the project's skills/ directory
  SKILL.md                   # Instructions for the agent
    ---
    name: my-skill
    description: "What this skill does"
    trigger: automatic
    ---
    # Markdown content
  scripts/                   # Executable scripts (optional)
  references/                # Reference docs (optional)
```

## Actions
| Action | Description | Category |
|--------|-------------|----------|
| `init` | Create new skill | Create |
| `edit` | Edit instructions | Create |
| `add_script` | Add executable | Create |
| `list` | List installed | Manage |
| `test` | Test skill | Manage |
| `install` | Install from source | Discover |
| `search` | Search ClawHub | Discover |
| `defaults_list` | List bundled | Discover |
| `defaults_install` | Install bundled | Discover |
| `remove` | Remove skill | Manage |

### Database Tools
| Tool | Action | Description |
|------|--------|-------------|
| `skill_db_create_table` | Create | Create a table for storing data |
| `skill_db_insert` | Create | Insert a new record |
| `skill_db_query` | Read | Query records with filters |
| `skill_db_update` | Update | Update a record by ID |
| `skill_db_delete` | Delete | Delete a record by ID |
| `skill_db_list_tables` | Read | List tables for a skill |
| `skill_db_describe` | Read | View table structure |
| `skill_db_drop_table` | Delete | Permanently drop a table |

## Creating Skills

### Initialize
Use skill_manage (NOT bash commands):
```
skill_manage(action="init", name="my-api-client", description="Interact with MyAPI service")
# Output: Skill created at ./skills/my-api-client/
```

### Initialize with Database
```bash
skill_manage(
    action="init",
    name="crm",
    description="Contact and lead management",
    with_database=true,
    database_table="contacts",
    database_schema={
        "name": "TEXT NOT NULL",
        "email": "TEXT",
        "phone": "TEXT",
        "status": "TEXT DEFAULT 'novo'"
    }
)
# Creates: Skill + database table crm_contacts
```

When `with_database=true`, the skill automatically gets a database table for storing structured data. The SKILL.md includes usage instructions for skill_db_* tools.

### Edit Instructions
```bash
skill_manage(action="edit", name="my-api-client", content="# MyAPI Client\n\n## Authentication\nUse vault(action=get, name='MYAPI_KEY')")
```

### Add Script
```bash
skill_manage(action="add_script", skill_name="my-api-client", script_name="fetch_users", content='#!/bin/bash\n...')
```

### Test
```bash
skill_manage(action="test", name="my-api-client", input="fetch all users")
```

## Installing Skills

### From ClawHub
```bash
skill_manage(action="search", query="github")
skill_manage(action="install", source="github-integration")
```

### From GitHub
```bash
skill_manage(action="install", source="https://github.com/user/skill-repo")
```

### Bundled Defaults
```bash
skill_manage(action="defaults_list")
skill_manage(action="defaults_install", names="github,jira")
skill_manage(action="defaults_install", names="all")
```

## Managing Skills

### List
```bash
skill_manage(action="list")
# Output:
# Installed skills (5):
# [builtin] memory
# [user] my-api-client
# [installed] github
```

### Remove
```bash
skill_manage(action="remove", name="old-skill")
```

## Common Patterns

### Create Custom Integration
```bash
# 1. Initialize
skill_manage(action="init", name="company-api", description="CompanyAPI client")

# 2. Add instructions
skill_manage(action="edit", name="company-api", content="# CompanyAPI\n\nUse vault(action=get, name='COMPANY_API_KEY')")

# 3. Add script
skill_manage(action="add_script", skill_name="company-api", script_name="get_employee", content='...')

# 4. Test
skill_manage(action="test", name="company-api", input="get employee 123")
```

### Create Skill with Database (CRM Example)
```bash
# 1. Initialize with database
skill_manage(
    action="init",
    name="crm",
    description="Customer relationship management",
    with_database=true,
    database_table="contacts",
    database_schema={
        "name": "TEXT NOT NULL",
        "email": "TEXT",
        "company": "TEXT",
        "status": "TEXT DEFAULT 'lead'"
    }
)

# 2. Use the database
skill_db_insert(skill_name="crm", table_name="contacts", data={
    "name": "John Smith",
    "email": "john@company.com"
})

# 3. Query data
skill_db_query(skill_name="crm", table_name="contacts", where={"status": "lead"})
```

## Tips
- Use descriptive names
- Test after creation
- Use vault for secrets in scripts
- Keep skills focused
- Use `with_database=true` for skills that need to store data
- Query with filters to avoid loading all data

## Related Skills
- **skill-db** - Database system for storing structured data in skills

## Common Mistakes
| Mistake | Correct Approach |
|---------|------------------|
| Hardcoding API keys | Use vault(action=get) in scripts |
| Vague description | Be specific about capabilities |
| Not testing | Always skill_manage(action=test) after creation |
