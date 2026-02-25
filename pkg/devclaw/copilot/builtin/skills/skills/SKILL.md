---
name: skills
description: "Create, install, and manage agent skills to extend capabilities"
trigger: automatic
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

## IMPORTANT: Always Use Tools

**NEVER create skills with bash commands (mkdir, echo, etc.)**

ALWAYS use the `init_skill` tool to create skills. The tool handles:
- Directory creation in the correct location
- SKILL.md template generation
- Database table creation (if requested)

## Communication Guidelines

**When creating skills for users:**
1. NEVER show technical tool syntax in chat responses
2. ALWAYS ask: "Você quer que essa skill tenha um banco de dados para salvar dados estruturados (contatos, tarefas, etc.)?"
3. Explain the options:
   - **Com banco de dados**: dados persistentes, consultas estruturadas, ideal para CRM, tarefas, contatos
   - **Sem banco de dados**: usa memória/vault, ideal para integrações de API, automações

## Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│   CREATE      │  │   DISCOVER    │  │   MANAGE      │
├───────────────┤  ├───────────────┤  ├───────────────┤
│ init_skill    │  │ search_skills │  │ list_skills   │
│ edit_skill    │  │ skill_defaults│  │ test_skill    │
│ add_script    │  │    _list      │  │ remove_skill  │
└───────────────┘  │ install_skill │  └───────────────┘
                     └───────────────┘
```

## Skill Structure
```
./skills/my-skill/           # Skills are created in the project's skills/ directory
├── SKILL.md                 # Instructions for the agent
│   ├── ---
│   │   name: my-skill
│   │   description: "What this skill does"
│   │   trigger: automatic
│   │   ---
│   └── # Markdown content
├── scripts/                 # Executable scripts (optional)
└── references/              # Reference docs (optional)
```

## Tools
| Tool | Action | Category |
|------|--------|----------|
| `init_skill` | Create new skill | Create |
| `edit_skill` | Edit instructions | Create |
| `add_script` | Add executable | Create |
| `list_skills` | List installed | Manage |
| `test_skill` | Test skill | Manage |
| `install_skill` | Install from source | Discover |
| `search_skills` | Search ClawHub | Discover |
| `skill_defaults_list` | List bundled | Discover |
| `skill_defaults_install` | Install bundled | Discover |
| `remove_skill` | Remove skill | Manage |

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
Use the init_skill tool (NOT bash commands):
```
init_skill(name="my-api-client", description="Interact with MyAPI service")
# Output: Skill created at ./skills/my-api-client/
```

### Initialize with Database
```bash
init_skill(
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
edit_skill(name="my-api-client", content="# MyAPI Client\n\n## Authentication\nUse vault_get('MYAPI_KEY')")
```

### Add Script
```bash
add_script(skill_name="my-api-client", script_name="fetch_users", script_content='#!/bin/bash\n...')
```

### Test
```bash
test_skill(name="my-api-client", input="fetch all users")
```

## Installing Skills

### From ClawHub
```bash
search_skills(query="github")
install_skill(source="github-integration")
```

### From GitHub
```bash
install_skill(source="https://github.com/user/skill-repo")
```

### Bundled Defaults
```bash
skill_defaults_list()
skill_defaults_install(names="github,jira")
skill_defaults_install(names="all")
```

## Managing Skills

### List
```bash
list_skills()
# Output:
# Installed skills (5):
# [builtin] memory
# [user] my-api-client
# [installed] github
```

### Remove
```bash
remove_skill(name="old-skill")
```

## Common Patterns

### Create Custom Integration
```bash
# 1. Initialize
init_skill(name="company-api", description="CompanyAPI client")

# 2. Add instructions
edit_skill(name="company-api", content="# CompanyAPI\n\nUse vault_get('COMPANY_API_KEY')")

# 3. Add script
add_script(skill_name="company-api", script_name="get_employee", script_content='...')

# 4. Test
test_skill(name="company-api", input="get employee 123")
```

### Create Skill with Database (CRM Example)
```bash
# 1. Initialize with database
init_skill(
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
    "name": "João Silva",
    "email": "joao@empresa.com"
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
| Hardcoding API keys | Use vault_get in scripts |
| Vague description | Be specific about capabilities |
| Not testing | Always test_skill after creation |
