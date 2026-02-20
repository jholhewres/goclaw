# Tool Profiles

Tool Profiles provide a simplified way to configure tool permissions in DevClaw. Instead of configuring each tool individually in the `tool_guard` section, you can use predefined profiles or create custom ones.

## Overview

Tool profiles define sets of allowed and denied tools using:
- Direct tool names (e.g., `bash`, `read_file`)
- Tool groups (e.g., `group:web`, `group:memory`)
- Wildcards (e.g., `git_*`, `docker_*`)

## Built-in Profiles

| Profile | Description | Use Case |
|---------|-------------|----------|
| `minimal` | Basic queries only | Read-only access for simple interactions |
| `coding` | Software development | File access, git, docker, tests |
| `messaging` | Chat channel usage | Web search and memory only |
| `full` | Full access | All tools available |

### Minimal Profile

Best for: Simple queries, read-only access

```yaml
Allow:
  - group:web        # web_search, web_fetch
  - group:memory     # memory_save, memory_search, memory_list
  - read_file
  - list_files
  - search_files
  - glob_files

Deny:
  - group:runtime    # bash, exec, ssh, scp
  - write_file
  - edit_file
  - group:skills
  - group:scheduler
  - group:vault
  - group:subagents
```

### Coding Profile

Best for: Software development, DevOps

```yaml
Allow:
  - group:fs         # read_file, write_file, edit_file, etc.
  - group:web
  - group:memory
  - bash
  - exec
  - git_*            # All git tools
  - docker_*         # All docker tools
  - test_*
  - cron_list

Deny:
  - ssh
  - scp
```

### Messaging Profile

Best for: Chat channels (WhatsApp, Discord, etc.)

```yaml
Allow:
  - group:web
  - group:memory
  - list_skills
  - search_skills

Deny:
  - group:runtime
  - group:fs
  - group:skills
  - group:scheduler
  - group:vault
  - group:subagents
```

### Full Profile

Best for: Owner access, unrestricted development

```yaml
Allow:
  - "*"              # All tools

Deny:
  (none)
```

## Configuration

### Global Profile

Set a default profile for all workspaces in `config.yaml`:

```yaml
security:
  tool_guard:
    enabled: true
    profile: coding        # Global default profile

    # Optional: Custom profiles
    custom_profiles:
      support:
        name: support
        description: Customer support team
        allow:
          - group:web
          - group:memory
          - read_file
          - list_files
        deny:
          - bash
          - ssh
          - group:vault
```

### Workspace Profile

Override the global profile per-workspace:

```yaml
workspaces:
  workspaces:
    - id: developers
      name: Developer Team
      tool_profile: coding       # Uses coding profile

    - id: support
      name: Support Team
      tool_profile: support       # Uses custom support profile

    - id: public
      name: Public Channels
      tool_profile: messaging     # Limited to messaging tools
```

## Custom Profiles

Define custom profiles in the configuration:

```yaml
security:
  tool_guard:
    custom_profiles:
      readonly:
        name: readonly
        description: Read-only access
        allow:
          - group:fs
          - group:web
          - group:memory
        deny:
          - write_file
          - edit_file
          - bash
          - exec

      dataops:
        name: dataops
        description: Data operations
        allow:
          - group:fs
          - bash
          - docker_*
        deny:
          - ssh
          - group:vault
```

## Tool Groups

Available tool groups for use in profiles:

| Group | Tools |
|-------|-------|
| `group:memory` | memory_save, memory_search, memory_list, memory_index |
| `group:web` | web_search, web_fetch |
| `group:fs` | read_file, write_file, edit_file, list_files, search_files, glob_files |
| `group:runtime` | bash, exec, ssh, scp, set_env |
| `group:subagents` | spawn_subagent, list_subagents, wait_subagent, stop_subagent |
| `group:skills` | install_skill, remove_skill, search_skills, list_skills, etc. |
| `group:scheduler` | cron_add, cron_list, cron_remove |
| `group:vault` | vault_save, vault_get, vault_list, vault_delete |
| `group:media` | describe_image, transcribe_audio, image-gen_generate_image |

## Chat Commands

### View Current Profile

```
/profile
```

Shows the current tool profile and its allow/deny lists.

### List Available Profiles

```
/profile list
```

Lists all available profiles (built-in and custom).

### Set Workspace Profile (Admin Only)

```
/profile set coding
```

Changes the tool profile for the current workspace.

## Resolution Order

1. **Workspace profile** - If the workspace has a `tool_profile` set, it takes precedence
2. **Global profile** - Falls back to `security.tool_guard.profile`
3. **Permission levels** - If no profile is set, uses traditional permission levels

## How It Works

When a tool is called:

1. Check if tool is in the profile's **deny** list → Block
2. Check if tool is in the profile's **allow** list → Continue
3. If allow list is empty → All tools allowed
4. Continue with normal permission checks (owner/admin/user levels)

Deny always takes precedence over allow.

## Examples

### Development Team Setup

```yaml
security:
  tool_guard:
    profile: coding

workspaces:
  workspaces:
    - id: devs
      name: Developers
      tool_profile: coding
      members:
        - dev1@example.com
        - dev2@example.com

    - id: interns
      name: Interns
      tool_profile: minimal    # More restrictive
      members:
        - intern1@example.com
```

### Multi-Environment Setup

```yaml
security:
  tool_guard:
    profile: minimal    # Safe default

    custom_profiles:
      production:
        name: production
        description: Production access
        allow:
          - read_file
          - list_files
          - group:web
          - group:memory
        deny:
          - bash
          - write_file
          - edit_file
          - ssh

      staging:
        name: staging
        description: Staging environment
        allow:
          - group:fs
          - group:web
          - group:memory
          - bash
          - exec
        deny:
          - ssh
          - group:vault

workspaces:
  workspaces:
    - id: prod
      name: Production
      tool_profile: production

    - id: stage
      name: Staging
      tool_profile: staging
```

## Migration from Permission Levels

Before (permission levels only):

```yaml
security:
  tool_guard:
    tool_permissions:
      bash: owner
      ssh: owner
      write_file: admin
      read_file: user
      # ... configure each tool
```

After (using profiles):

```yaml
security:
  tool_guard:
    profile: coding    # Simple preset
```

You can still use `tool_permissions` alongside profiles for fine-grained control:

```yaml
security:
  tool_guard:
    profile: coding
    tool_permissions:
      ssh: owner        # Additional restriction
```

## Troubleshooting

### Tool Blocked Unexpectedly

1. Check the current profile: `/profile`
2. Verify the tool is in the allow list: `/profile list`
3. Check if it's in the deny list (deny takes precedence)

### Profile Not Applied

1. Ensure the profile name is correct
2. Check if workspace has a different profile set
3. Verify `tool_guard.enabled` is `true`

### Custom Profile Not Found

1. Check YAML syntax (proper indentation)
2. Verify the profile is in `custom_profiles` section
3. Restart the service after config changes
