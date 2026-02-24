---
name: skills
description: "Create, install, and manage agent skills to extend capabilities"
trigger: automatic
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

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
                           │
                           ▼
                   ┌───────────────┐
                   │   ClawHub     │
                   │   Registry    │
                   └───────────────┘
```

## Tools

| Tool | Action | Category |
|------|--------|----------|
| `init_skill` | Create new skill from template | Create |
| `edit_skill` | Edit SKILL.md instructions | Create |
| `add_script` | Add executable script | Create |
| `list_skills` | List installed skills | Manage |
| `test_skill` | Test skill with sample input | Manage |
| `install_skill` | Install from ClawHub/GitHub/URL | Discover |
| `search_skills` | Search ClawHub registry | Discover |
| `skill_defaults_list` | List bundled default skills | Discover |
| `skill_defaults_install` | Install bundled defaults | Discover |
| `remove_skill` | Remove installed skill | Manage |

## Skill Structure

```
~/.devclaw/skills/my-skill/
├── SKILL.md           # Instructions for the agent
│   ├── ---
│   │   name: my-skill
│   │   description: "What this skill does"
│   │   trigger: automatic
│   │   ---
│   └── # Markdown content with instructions
├── scripts/           # Executable scripts (optional)
│   ├── fetch_data.sh
│   └── process.py
└── references/        # Reference docs (optional)
    └── api-docs.md
```

## Creating Skills

### Initialize New Skill
```bash
init_skill(
  name="my-api-client",
  description="Interact with MyAPI service for data retrieval"
)
# Output:
# Skill 'my-api-client' created at ~/.devclaw/skills/my-api-client/
# Files: SKILL.md, metadata.json
```

### Edit Skill Instructions
```bash
edit_skill(
  name="my-api-client",
  content="# MyAPI Client

## Overview
This skill helps interact with MyAPI.

## Authentication
Use vault_get('MYAPI_KEY') to get the API key.

## Endpoints
- GET /users - List users
- POST /data - Submit data

## Examples
\`\`\`bash
curl -H \"Authorization: Bearer $API_KEY\" https://api.example.com/users
\`\`\`
"
)
# Output: Skill 'my-api-client' instructions updated
```

### Add Executable Script
```bash
add_script(
  skill_name="my-api-client",
  script_name="fetch_users",
  script_content='#!/bin/bash
API_KEY=$(cat ~/.devclaw/vault/MYAPI_KEY 2>/dev/null)
if [ -z "$API_KEY" ]; then
  echo "Error: MYAPI_KEY not found in vault"
  exit 1
fi
curl -s -H "Authorization: Bearer $API_KEY" "https://api.example.com/users"
'
)
# Output: Script 'fetch_users' added to skill 'my-api-client'
```

### Test the Skill
```bash
test_skill(
  name="my-api-client",
  input="fetch all users"
)
# Output: Testing skill 'my-api-client'...
# Result: [...list of users...]
```

## Installing Skills

### From ClawHub
```bash
# Search first
search_skills(query="github")
# Output:
# [1] github-integration - GitHub API integration
# [2] gh-issues - GitHub issue management

# Install
install_skill(source="github-integration")
# Output: Skill 'github-integration' installed successfully
```

### From GitHub
```bash
install_skill(source="https://github.com/user/skill-repo")
# Output: Skill installed from GitHub
```

### From URL
```bash
install_skill(source="https://example.com/skills/my-skill.zip")
# Output: Skill installed from URL
```

### Bundled Default Skills
```bash
# List available defaults
skill_defaults_list()
# Output:
# Default skills available:
# - github: GitHub integration
# - jira: Jira issue tracking
# - slack: Slack messaging
# - trello: Trello boards

# Install specific defaults
skill_defaults_install(names="github,jira")
# Output: Installed: github, jira

# Install all defaults
skill_defaults_install(names="all")
# Output: Installed all default skills
```

## Managing Skills

### List Installed Skills
```bash
list_skills()
# Output:
# Installed skills (5):
# [builtin] memory
# [builtin] teams
# [builtin] vault
# [user] my-api-client
# [installed] github-integration
# [default] jira
```

### Remove a Skill
```bash
remove_skill(name="old-unused-skill")
# Output: Skill 'old-unused-skill' removed
```

## Common Patterns

### Create Custom Integration
```bash
# 1. Initialize
init_skill(
  name="company-api",
  description="Internal CompanyAPI client for employee data"
)

# 2. Add instructions
edit_skill(
  name="company-api",
  content="# CompanyAPI Client

## Setup
Store API key: vault_save(name='COMPANY_API_KEY', value='your-key')

## Available Actions
- Get employee: GET /employees/{id}
- List teams: GET /teams
- Update record: PUT /employees/{id}

## Authentication
All requests need header: Authorization: Bearer <API_KEY>
"
)

# 3. Add helper script
add_script(
  skill_name="company-api",
  script_name="get_employee",
  script_content='#!/bin/bash
ID=$1
KEY=$(vault_get COMPANY_API_KEY)
curl -s -H "Authorization: Bearer $KEY" "https://api.company.com/employees/$ID"
'
)

# 4. Test
test_skill(name="company-api", input="get employee 123")
```

### Discover and Install
```bash
# 1. Search for functionality
search_skills(query="project management")
# Output:
# [1] trello - Trello board integration
# [2] jira - Jira issue tracking
# [3] asana - Asana task management

# 2. Install what you need
install_skill(source="trello")

# 3. Verify
list_skills()
```

### Update Existing Skill
```bash
# 1. View current content
list_skills()  # Find skill name

# 2. Edit instructions
edit_skill(
  name="my-skill",
  content="# Updated content..."
)

# 3. Add new script if needed
add_script(
  skill_name="my-skill",
  script_name="new_function",
  script_content="#!/bin/bash\necho 'new function'"
)

# 4. Test changes
test_skill(name="my-skill", input="test input")
```

## Troubleshooting

### "Skill not found"

**Cause:** Name typo or skill not installed.

**Debug:**
```bash
list_skills()  # See all available skills
```

### "Script execution failed"

**Cause:** Script error or missing dependencies.

**Debug:**
```bash
# Run script manually
bash(command="~/.devclaw/skills/my-skill/scripts/script.sh")

# Check permissions
bash(command="ls -la ~/.devclaw/skills/my-skill/scripts/")
```

### "Install failed"

**Cause:** Network issue or invalid source.

**Debug:**
```bash
# Test network
bash(command="curl -I https://clawhub.ai")

# Try alternative source
install_skill(source="https://github.com/user/repo")
```

### "Cannot remove builtin skill"

**Cause:** Builtin skills cannot be removed.

**Solution:**
- Only user-installed skills can be removed
- Builtin skills are part of the system

## Tips

- **Use descriptive names**: `company-api-client` not `api1`
- **Test after creation**: Verify skill works correctly
- **Document in SKILL.md**: Clear instructions for agent
- **Use vault for secrets**: Never hardcode credentials in scripts
- **Keep skills focused**: One purpose per skill
- **Clean up unused**: Remove skills you don't need

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Hardcoding API keys | Use `vault_get` in scripts |
| Vague skill description | Be specific about capabilities |
| Not testing | Always `test_skill` after creation |
| Installing too many | Only install what you need |
| No cleanup | Remove unused skills |

## SKILL.md Best Practices

```markdown
---
name: skill-name
description: "Clear description of what this skill does and when to use it"
trigger: automatic
---

# Skill Title

Brief overview of the skill.

## Setup
Any required setup (vault keys, config, etc.)

## Actions
- Action 1: Description
- Action 2: Description

## Examples
```bash
# Example command
command --arg value
```

## Notes
Important considerations.
```
