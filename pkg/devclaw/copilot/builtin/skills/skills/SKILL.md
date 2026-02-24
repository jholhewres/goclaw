---
name: skills
description: "Create, install, and manage agent skills"
trigger: automatic
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

## Tools

| Tool | Action |
|------|--------|
| `init_skill` | Create a new skill from template |
| `edit_skill` | Edit skill instructions (SKILL.md) |
| `add_script` | Add executable script to skill |
| `list_skills` | List all installed skills |
| `test_skill` | Test a skill with sample input |
| `install_skill` | Install skill from ClawHub, GitHub, URL, or local |
| `search_skills` | Search ClawHub for available skills |
| `skill_defaults_list` | List bundled default skills |
| `skill_defaults_install` | Install bundled default skills |
| `remove_skill` | Remove an installed skill |

## When to Use

| Tool | When |
|------|------|
| `init_skill` | Create new custom skill |
| `edit_skill` | Modify existing skill instructions |
| `add_script` | Add executable to skill |
| `list_skills` | See what's installed |
| `test_skill` | Verify skill works correctly |
| `install_skill` | Get skill from external source |
| `search_skills` | Find skills on ClawHub |
| `remove_skill` | Clean up unused skill |

## Creating Skills

### Initialize New Skill
```bash
init_skill(
  name="my-api-client",
  description="Interact with MyAPI service"
)
# Output: Skill created at ~/.devclaw/skills/my-api-client/
```

### Edit Skill Instructions
```bash
edit_skill(
  name="my-api-client",
  content="# MyAPI Client\n\nUse this skill to interact with MyAPI...\n\n## Endpoints\n- GET /users\n- POST /data"
)
# Output: Skill instructions updated
```

### Add Executable Script
```bash
add_script(
  skill_name="my-api-client",
  script_name="fetch_users",
  script_content='#!/bin/bash\ncurl -s "https://api.example.com/users"'
)
# Output: Script added to skill
```

### Test the Skill
```bash
test_skill(
  name="my-api-client",
  input="fetch all users"
)
# Output: Skill execution result...
```

## Installing Skills

### From ClawHub
```bash
install_skill(source="steipete/trello")
# Output: Skill installed from ClawHub
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

### Default Bundled Skills
```bash
# List available defaults
skill_defaults_list()
# Output:
# - github: GitHub integration
# - jira: Jira integration
# - slack: Slack messaging

# Install specific default
skill_defaults_install(names="github,jira")

# Install all defaults
skill_defaults_install(names="all")
```

## Discovering Skills

### Search ClawHub
```bash
search_skills(query="project management")
# Output:
# [1] trello - Trello board integration
# [2] jira-client - Jira issue tracking
# [3] asana - Asana task management
```

### List Installed Skills
```bash
list_skills()
# Output:
# Installed skills (5):
# - memory (builtin)
# - teams (builtin)
# - my-api-client (user)
# - trello (installed)
# - github (default)
```

## Managing Skills

### Remove a Skill
```bash
remove_skill(name="old-unused-skill")
# Output: Skill 'old-unused-skill' removed
```

## Workflow Examples

### Create Custom Integration
```bash
# 1. Initialize
init_skill(name="company-api", description="Internal company API client")

# 2. Add instructions
edit_skill(
  name="company-api",
  content="# Company API\n\n## Authentication\nUse vault_get('COMPANY_API_KEY')\n\n## Endpoints\n- GET /users\n- POST /reports"
)

# 3. Add helper script
add_script(
  skill_name="company-api",
  script_name="get_users",
  script_content='#!/bin/bash\nKEY=$(cat ~/.devclaw/vault/COMPANY_API_KEY)\ncurl -H "Authorization: Bearer $KEY" https://api.company.com/users'
)

# 4. Test
test_skill(name="company-api", input="list all users")
```

### Install and Use External Skill
```bash
# 1. Search for skill
search_skills(query="slack")

# 2. Install it
install_skill(source="slack-integration")

# 3. List to confirm
list_skills()

# 4. Use the skill (now available to agent)
```

## Skill Structure

```
~/.devclaw/skills/my-skill/
├── SKILL.md          # Instructions for the agent
├── scripts/          # Executable scripts
│   ├── fetch_data.sh
│   └── process.py
└── metadata.json     # Skill metadata
```

## Best Practices

| Practice | Reason |
|----------|--------|
| Use descriptive names | Easy to identify purpose |
| Test after creating | Verify skill works |
| Document in SKILL.md | Clear instructions for agent |
| Use vault for secrets | Never hardcode credentials |
| Clean up unused skills | Avoid clutter |

## Important Notes

| Note | Reason |
|------|--------|
| Builtin skills can't be removed | Only user skills are removable |
| Scripts must be executable | Agent runs them directly |
| SKILL.md is markdown | Use formatting for clarity |
| Changes take effect immediately | No restart needed |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Hardcoding API keys | Use vault_get in scripts |
| Vague skill description | Be specific about capabilities |
| Not testing | Always test_skill after creation |
| Installing too many skills | Only install what you need |
