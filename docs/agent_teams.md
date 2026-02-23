# DevClaw Teams — Persistent Agents & Shared Memory

Documentation for the Teams system that adds persistent agent management and team coordination capabilities to DevClaw.

---

## Quick Start

### 1. Criar um Time

```bash
team_manage(action="create", name="DevClaw OS", description="Main development team")
# Retorna: { "id": "5916dabf", "name": "DevClaw OS", ... }
```

### 2. Criar um Agente

```bash
team_agent(
  action="create",
  team_id="devclaw-os",  # Pode usar ID ou nome do time!
  name="Siri",
  role="Personal Assistant",
  personality="Helpful, proactive, organized",
  instructions="Help with daily tasks, manage calendar, answer questions",
  level="mid",
  heartbeat_schedule="*/15 * * * *"
)
```

### 3. O Agente em Ação

No heartbeat ou quando mencionado, o agente:

```bash
# 1. Verifica menções
team_comm(action="mention_check", agent_id="siri")

# 2. Lista tarefas atribuídas
team_task(action="list", team_id="devclaw-os", assignee_filter="siri")

# 3. Atualiza estado de trabalho
team_agent(
  action="working_update",
  agent_id="siri",
  current_task_id="abc12345",
  status="working",
  next_steps="1. Process request\n2. Send response",
  context="Handling user query about documentation"
)

# 4. Ao terminar, notifica
team_comm(
  action="notify",
  type="task_completed",
  message="Documentation updated successfully",
  task_id="abc12345",
  priority=3
)
```

---

## Overview

DevClaw Teams extends the existing subagent architecture with:

- **Persistent Agents**: Long-lived agents with specific roles, personalities, and instructions
- **Team Memory**: Shared state accessible by all team members (tasks, messages, facts, documents)
- **Agent Communication**: Inter-agent messaging via @mentions and direct messages
- **Heartbeat Integration**: Periodic wake-ups for proactive behavior
- **Thread Subscriptions**: Auto-subscribe to threads for continuous notifications
- **Working State**: Persist work-in-progress across heartbeats (WORKING.md pattern)
- **Active Notifications**: Trigger agents immediately on @mentions

```
┌─────────────────────────────────────────────────────────────┐
│                      TeamManager                             │
│  - CreateTeam / CreateAgent                                 │
│  - Heartbeat scheduling (via Scheduler)                     │
│  - Agent lifecycle (start, stop, delete)                    │
│  - @mention parsing and routing                             │
│  - Working state persistence (WORKING.md)                   │
│  - Active notification push (spawn callback)                │
│  - Team resolution by ID or name                            │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                      TeamMemory                              │
│  - Tasks (CRUD, status workflow)                            │
│  - Messages (@mentions, mailbox)                            │
│  - Facts (shared key-value store)                           │
│  - Activities (audit trail)                                 │
│  - Documents (deliverables, research, protocols)            │
│  - Thread Subscriptions (auto, mentioned, assigned)         │
│  - Standup generation                                       │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│               Existing Architecture                          │
│  - SQLite DB (new tables added)                             │
│  - Scheduler (for heartbeats)                               │
│  - SubagentManager (unchanged, works in parallel)           │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### Teams

A team is a collection of persistent agents working together:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `name` | string | Team name |
| `description` | string | Team purpose |
| `owner_jid` | string | Owner's JID (user identifier) |
| `default_model` | string | Default LLM model for agents |
| `workspace_path` | string | Optional workspace directory |
| `enabled` | bool | Team active status |

### Team Resolution (ID or Name)

The `team_id` parameter in all tools supports **flexible resolution**:

```bash
# Por ID (exato)
team_task(action="list", team_id="5916dabf")

# Por nome (case insensitive, normalizado)
team_task(action="list", team_id="DevClaw OS")    # → encontra "devclaw-os"
team_task(action="list", team_id="devclaw-os")    # → encontra "devclaw-os"
team_task(action="list", team_id="DEVCLAW_OS")    # → encontra "devclaw-os"

# Sem team_id (usa time único automaticamente)
team_task(action="list")  # Se só houver 1 time, usa ele
                          # Se houver múltiplos, retorna erro pedindo para especificar
```

**Normalização**: Hífens, underscores e espaços são tratados como equivalentes.

### Persistent Agents

Unlike subagents (ephemeral), persistent agents have:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Derived from name (lowercase, alphanumeric) |
| `name` | string | Display name (e.g., "Jarvis", "Loki") |
| `role` | string | Role description (e.g., "Squad Lead") |
| `team_id` | string | Parent team ID |
| `level` | enum | `junior`, `mid`, `senior`, `lead` |
| `status` | enum | `idle`, `working`, `paused`, `stopped` |
| `personality` | string | Custom personality traits |
| `instructions` | string | Specific instructions for this agent |
| `model` | string | LLM model override |
| `skills` | []string | List of skill names |
| `heartbeat_schedule` | string | Cron expression for wake-ups |

### Agent Levels

| Level | Description |
|-------|-------------|
| `junior` | Entry-level agent, simple tasks |
| `mid` | Standard agent, moderate complexity |
| `senior` | Advanced agent, complex tasks |
| `lead` | Leadership role, coordination |

### Agent Status

| Status | Description |
|--------|-------------|
| `idle` | Available for work |
| `working` | Currently executing a task |
| `paused` | Temporarily inactive |
| `stopped` | Disabled (no heartbeats) |

---

## Agent Workflow

### Ciclo de Vida do Agente

```
┌─────────────────────────────────────────────────────────────┐
│                    HEARTBEAT / MENTION                       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. CHECK INCOMING                                            │
│    team_comm(action="mention_check", agent_id="siri")       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. CHECK WORKING STATE                                       │
│    team_agent(action="working_get", agent_id="siri")        │
│    → Resume in-progress work if exists                       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. CHECK TASKS                                               │
│    team_task(action="list", assignee_filter="siri")        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. DO WORK & UPDATE STATE                                    │
│    team_agent(action="working_update", ...)                 │
└──────────────────────────┬──────────────────────────────────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
            ▼              ▼              ▼
      ┌──────────┐   ┌──────────┐   ┌──────────┐
      │ SUCCESS  │   │  ERROR   │   │ BLOCKED  │
      └────┬─────┘   └────┬─────┘   └────┬─────┘
           │              │              │
           ▼              ▼              ▼
   task_completed    task_failed    task_blocked
   task_progress     agent_error
           │              │              │
           └──────────────┼──────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. NOTIFY                                                    │
│    team_comm(action="notify", type="...", message="...")    │
└─────────────────────────────────────────────────────────────┘
```

### Padrão de Notificação

O agente **DEVE** notificar em eventos importantes:

```bash
# Iniciando trabalho
team_agent(
  action="working_update",
  agent_id="siri",
  status="working",
  current_task_id="abc12345",
  next_steps="1. Analyze data\n2. Generate report"
)

# Progresso (opcional, para tarefas longas)
team_comm(
  action="notify",
  type="task_progress",
  message="50% complete - data analysis done",
  task_id="abc12345",
  priority=4
)

# Sucesso
team_comm(
  action="notify",
  type="task_completed",
  message="Report generated and saved to /reports/monthly.md",
  task_id="abc12345",
  priority=3
)

# Erro
team_comm(
  action="notify",
  type="task_failed",
  message="Failed to connect to database: connection refused",
  task_id="abc12345",
  priority=2
)

# Bloqueado
team_comm(
  action="notify",
  type="task_blocked",
  message="Waiting for API credentials from admin",
  task_id="abc12345",
  priority=3
)

# Ao finalizar, limpar working state
team_agent(action="working_clear", agent_id="siri")
```

---

## Team Memory

### Tasks

Tasks represent work items tracked in shared memory:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `title` | string | Task title |
| `description` | string | Detailed description |
| `status` | enum | Current status |
| `assignees` | []string | Assigned agent IDs |
| `priority` | int | 1-5 priority level |
| `labels` | []string | Tags for categorization |
| `created_by` | string | Creator identifier |
| `completed_at` | time | Completion timestamp |

#### Task Status Workflow

```
inbox → assigned → in_progress → review → done
                     │
                     └──→ blocked
                           │
                           └──→ cancelled
```

| Status | Description |
|--------|-------------|
| `inbox` | New task, not yet assigned |
| `assigned` | Assigned to agent(s), not started |
| `in_progress` | Work in progress |
| `review` | Needs review before completion |
| `done` | Completed successfully |
| `blocked` | Blocked by external dependency |
| `cancelled` | Cancelled, will not complete |

### Messages & @Mentions

Agents communicate via messages with @mentions:

```
PostMessage(thread_id, from_agent, content, mentions)
    │
    ├── Stores message in team_messages
    ├── Creates pending messages for mentioned agents
    └── Logs activity
```

When an agent is mentioned (`@jarvis check this`), a pending message is added to their mailbox. The agent can retrieve these on their next heartbeat or when triggered.

### Facts (Shared Memory)

Key-value store for team-wide knowledge:

```yaml
# Example facts
project_name: "DevClaw Teams"
api_endpoint: "https://api.example.com"
sprint_goal: "Complete authentication flow"
```

- Facts are unique by key within a team
- Updates overwrite existing values
- Authors are tracked for audit

### Activities

Audit trail of team actions:

| Type | Description |
|------|-------------|
| `task_created` | New task created |
| `task_updated` | Task status changed |
| `task_assigned` | Task assigned to agent |
| `message_sent` | Message posted |
| `mention` | Agent mentioned |
| `fact_created` | Fact saved |
| `document_created` | Document created |
| `document_updated` | Document updated |
| `subscribed` | Agent subscribed to thread |

### Documents

Store deliverables and research linked to tasks:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `title` | string | Document title |
| `doc_type` | enum | `deliverable`, `research`, `protocol`, `notes` |
| `content` | string | Document content |
| `format` | string | `markdown`, `code`, `json`, `image` |
| `task_id` | string | Linked task ID (optional) |
| `version` | int | Version number (auto-incremented) |
| `author` | string | Creator agent ID |

### Thread Subscriptions

Agents automatically subscribe to threads for continuous notifications:

| Reason | Trigger |
|--------|---------|
| `auto` | Posted a message in thread |
| `mentioned` | Was @mentioned in thread |
| `assigned` | Assigned to linked task |

When a new message is posted, ALL subscribers receive a pending message notification—not just explicitly @mentioned agents.

### Working State (WORKING.md)

Each agent can persist their current work state:

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | string | Agent identifier |
| `current_task_id` | string | Task being worked on |
| `status` | string | `idle`, `working`, `blocked`, `waiting` |
| `next_steps` | string | Markdown checklist of next steps |
| `context` | string | Context for resuming work |

This allows agents to resume work across heartbeats without losing context.

---

## Heartbeat System

Persistent agents can be woken up periodically via the scheduler:

```yaml
# Agent configuration
heartbeat_schedule: "*/15 * * * *"  # Every 15 minutes
```

### Heartbeat Checklist

When triggered, the agent:

1. Checks `WORKING.md` for ongoing tasks (via `working_get`)
2. Resumes any in-progress work (using saved context and next steps)
3. Checks TeamMemory for @mentions and subscribed thread notifications
4. Reviews assigned tasks
5. Scans activity feed for relevant updates

### Working State Persistence

The agent's working state is automatically included in the heartbeat prompt:

```
## Current Work State (WORKING.md)
- Status: working
- Current Task: xyz78901
- Next Steps:
1. Complete authentication
2. Add tests
3. Review with team
```

This allows agents to seamlessly resume work across heartbeats.

### Response Protocol

- **Has work**: Execute the work, update working state with `working_update`
- **No work**: Respond with exactly `HEARTBEAT_OK`

### Active Notification Push

When an agent is @mentioned or a subscribed thread has activity, the agent is triggered immediately—without waiting for the scheduled heartbeat. This enables real-time collaboration between agents.

---

## Tools Reference

Team tools use a **dispatcher pattern** to reduce tool count while maintaining full functionality. Each dispatcher tool accepts an `action` parameter that determines the operation.

### team_manage - Team CRUD

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `create` | Create a new team | `name` |
| `list` | List all teams | - |
| `get` | Get a specific team | `team_id` (ID or name) |
| `update` | Update team properties | `team_id` |
| `delete` | Delete a team and all data | `team_id` |

### team_agent - Agent Management

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `create` | Create a persistent agent | `team_id`, `name` |
| `list` | List agents in a team | `team_id` (ID or name) |
| `get` | Get agent details | `agent_id` |
| `update` | Update agent properties | `agent_id` |
| `start` | Start an agent | `agent_id` |
| `stop` | Stop an agent | `agent_id` |
| `delete` | Delete an agent | `agent_id` |
| `working_get` | Get agent's working state | `agent_id` |
| `working_update` | Update working state | `agent_id`, `status`, `next_steps`, `context` |
| `working_clear` | Clear working state | `agent_id` |

### team_task - Task Management

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `create` | Create a task | `team_id` (ID or name), `title` |
| `list` | List tasks (filterable) | `team_id` (ID or name) |
| `get` | Get task details | `team_id`, `task_id` |
| `update` | Update task status | `team_id`, `task_id`, `status` |
| `assign` | Assign agents to task | `team_id`, `task_id`, `assignees` |
| `delete` | Delete a task | `team_id`, `task_id` |

### team_memory - Shared Memory

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `fact_save` | Save a fact | `team_id` (ID or name), `key`, `value` |
| `fact_list` | Get all facts | `team_id` |
| `fact_delete` | Delete a fact | `team_id`, `key` |
| `doc_create` | Create a document | `team_id`, `title`, `content` |
| `doc_list` | List documents | `team_id` |
| `doc_get` | Get document by ID | `team_id`, `doc_id` |
| `doc_update` | Update document | `team_id`, `doc_id`, `content` |
| `doc_delete` | Delete a document | `team_id`, `doc_id` |
| `standup` | Generate daily standup | `team_id` |

### team_comm - Communication

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `comment` | Add comment to task thread | `team_id`, `task_id`, `content` |
| `mention_check` | Check pending @mentions | `agent_id` |
| `send_message` | Send direct message | `team_id`, `to_agent`, `content` |
| `notify` | Send a notification | `team_id` (ID or name), `type`, `message` |
| `notify_list` | Get team notifications | `team_id` |

---

## Notification System

Agents can send notifications about their work to configured destinations. This enables real-time alerts and activity tracking.

### Notification Types

| Type | Description | When to Use |
|------|-------------|-------------|
| `task_completed` | Task finished successfully | When agent finishes a task |
| `task_failed` | Task execution failed | When agent encounters an error |
| `task_blocked` | Task is blocked by dependency | When agent can't proceed |
| `task_progress` | Progress update on task | During long-running tasks |
| `agent_error` | Agent encountered an error | Internal agent errors |

### Notification Priority

| Priority | Description | Use Case |
|----------|-------------|----------|
| 1 | Urgent - always delivered | Critical failures, security issues |
| 2 | High - important | Task failures, blockers |
| 3 | Normal | Standard completions, progress |
| 4 | Low | Informational updates |
| 5 | Minimal | Background updates |

### Sending Notifications

```bash
# Task completed
team_comm(
  action="notify",
  type="task_completed",
  message="Authentication feature completed and tested",
  task_id="xyz78901",
  priority=3
)

# Task failed
team_comm(
  action="notify",
  type="task_failed",
  message="Database connection failed after 3 retries",
  task_id="xyz78901",
  priority=2
)

# Task blocked
team_comm(
  action="notify",
  type="task_blocked",
  message="Waiting for API key from infrastructure team",
  task_id="xyz78901",
  priority=3
)

# Progress update
team_comm(
  action="notify",
  type="task_progress",
  message="75% complete - running final tests",
  task_id="xyz78901",
  priority=4
)
```

### Listing Notifications

```bash
team_comm(
  action="notify_list",
  team_id="devclaw-os",
  limit=20,
  unread_only=true
)
```

### Notification Destinations

| Type | Description |
|------|-------------|
| `channel` | Send to connected channel (WhatsApp, Discord, etc.) |
| `inbox` | Add to agent's pending messages |
| `webhook` | HTTP POST to external URL |
| `owner` | Direct message to team owner |
| `activity` | Add to team activity feed |

### Configuration

Configure notification rules in `config.yaml`:

```yaml
notifications:
  enabled: true
  defaults:
    activity_feed: true
    owner: false
  quiet_hours:
    enabled: true
    start: "22:00"
    end: "08:00"
    timezone: "America/Sao_Paulo"
  rate_limit_per_hour: 20
  rules:
    - name: "Critical Alerts"
      enabled: true
      events: [task_failed, agent_error]
      destinations:
        - type: channel
          channel: "whatsapp"
          chat_id: "120363XXXXXX@g.us"
      priority: 1
```

### Notification Rules

Rules define when and how notifications are sent:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique rule identifier |
| `team_id` | string | Team ID (empty = global) |
| `name` | string | Human-readable rule name |
| `enabled` | bool | Rule active status |
| `events` | []string | Notification types that trigger |
| `conditions` | object | Additional filters |
| `destinations` | []object | Where to send notifications |
| `template` | string | Go template for message (optional) |
| `priority` | int | Minimum priority to trigger (1-5) |
| `rate_limit` | int | Max notifications per hour (0 = unlimited) |
| `quiet_hours` | object | When to suppress notifications |

### Quiet Hours

Suppress non-urgent notifications during specific hours:

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Quiet hours active |
| `start` | string | Start time (HH:MM) |
| `end` | string | End time (HH:MM) |
| `timezone` | string | Timezone for times |
| `days` | []int | Days of week (0=Sunday, 6=Saturday) |

---

## Usage Examples

### Creating a Team

```bash
# Via conversation
"Create a team called Engineering with description 'Main development team'"

# Via tool call
team_manage(
  action="create",
  name="Engineering",
  description="Main development team"
)
```

### Managing Teams

```bash
# List all teams
team_manage(action="list")

# Get team details (by ID or name)
team_manage(action="get", team_id="5916dabf")
team_manage(action="get", team_id="engineering")

# Update team properties
team_manage(
  action="update",
  team_id="engineering",
  name="Engineering Team",
  description="Core engineering team",
  default_model="gpt-4.1-mini"
)

# Delete a team (WARNING: deletes all agents, tasks, and memory)
team_manage(action="delete", team_id="engineering")
```

### Creating Agents

```bash
# Create a squad lead
team_agent(
  action="create",
  team_id="engineering",
  name="Jarvis",
  role="Squad Lead",
  personality="Professional, proactive, detail-oriented",
  instructions="Coordinate team activities, assign tasks, generate reports",
  model="claude-sonnet",
  skills=["planning", "coordination"],
  level="lead",
  heartbeat_schedule="*/15 * * * *"
)

# Create a writer
team_agent(
  action="create",
  team_id="engineering",
  name="Loki",
  role="Technical Writer",
  personality="Creative, thorough, articulate",
  instructions="Write documentation, blog posts, and technical guides",
  level="mid",
  heartbeat_schedule="*/30 * * * *"
)
```

### Managing Agents

```bash
# List agents
team_agent(action="list", team_id="engineering")

# Get agent details (includes instructions, personality, skills)
team_agent(action="get", agent_id="jarvis")

# Update agent properties
team_agent(
  action="update",
  agent_id="jarvis",
  role="Senior Squad Lead",
  personality="More decisive and autonomous",
  heartbeat_schedule="*/10 * * * *"
)

# Stop an agent (disables heartbeats)
team_agent(action="stop", agent_id="loki")

# Start a stopped agent
team_agent(action="start", agent_id="loki")

# Delete an agent permanently
team_agent(action="delete", agent_id="loki")
```

### Task Workflow

```bash
# Create a task
team_task(
  action="create",
  team_id="engineering",
  title="Implement authentication",
  description="Add OAuth2 support with Google and GitHub providers",
  assignees=["jarvis", "loki"]
)

# List tasks
team_task(action="list", team_id="engineering")

# Filter tasks by status
team_task(action="list", team_id="engineering", status="in_progress")

# Filter tasks by assignee
team_task(action="list", team_id="engineering", assignee_filter="jarvis")

# Get task details
team_task(action="get", team_id="engineering", task_id="xyz78901")

# Update status
team_task(
  action="update",
  team_id="engineering",
  task_id="xyz78901",
  status="in_progress"
)

# Complete task
team_task(
  action="update",
  team_id="engineering",
  task_id="xyz78901",
  status="done"
)

# Delete a task
team_task(action="delete", team_id="engineering", task_id="xyz78901")
```

### Agent Communication

```bash
# Comment on task with mention
team_comm(
  action="comment",
  team_id="engineering",
  task_id="xyz78901",
  content="@loki can you write the docs for this endpoint?"
)

# Check mentions (agent calls this on heartbeat)
team_comm(action="mention_check", agent_id="loki")

# Send direct message
team_comm(
  action="send_message",
  team_id="engineering",
  to_agent="jarvis",
  content="The deployment is complete"
)
```

### Shared Facts

```bash
# Save facts
team_memory(
  action="fact_save",
  team_id="engineering",
  key="api_version",
  value="v2.1.0"
)

# List facts
team_memory(action="fact_list", team_id="engineering")

# Delete fact
team_memory(action="fact_delete", team_id="engineering", key="api_version")

# Generate standup
team_memory(action="standup", team_id="engineering")
```

### Documents

```bash
# Create a deliverable document
team_memory(
  action="doc_create",
  team_id="engineering",
  title="API Design Document",
  doc_type="deliverable",
  content="# API Design\n\n## Endpoints\n...",
  task_id="xyz78901"
)

# List documents
team_memory(action="doc_list", team_id="engineering")

# Filter by type
team_memory(action="doc_list", team_id="engineering", doc_type="research")

# Get document
team_memory(action="doc_get", team_id="engineering", doc_id="doc123")

# Update document
team_memory(
  action="doc_update",
  team_id="engineering",
  doc_id="doc123",
  content="# API Design v2\n..."
)

# Delete document
team_memory(action="doc_delete", team_id="engineering", doc_id="doc123")
```

### Working State (WORKING.md)

```bash
# Save work in progress
team_agent(
  action="working_update",
  agent_id="jarvis",
  current_task_id="xyz78901",
  status="working",
  next_steps="1. Complete authentication\n2. Add tests\n3. Review with team",
  context="Implementing OAuth2 with Google provider"
)

# Check current working state
team_agent(action="working_get", agent_id="jarvis")

# Clear when task is done
team_agent(action="working_clear", agent_id="jarvis")
```

---

## Database Schema

### New Tables

```sql
-- Teams
CREATE TABLE teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    owner_jid TEXT NOT NULL,
    default_model TEXT DEFAULT '',
    workspace_path TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    enabled INTEGER DEFAULT 1
);

-- Persistent Agents
CREATE TABLE persistent_agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    team_id TEXT NOT NULL,
    level TEXT DEFAULT 'mid',
    status TEXT DEFAULT 'idle',
    personality TEXT DEFAULT '',
    instructions TEXT DEFAULT '',
    model TEXT DEFAULT '',
    skills TEXT DEFAULT '[]',
    heartbeat_schedule TEXT DEFAULT '*/15 * * * *',
    current_task_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    last_active_at TEXT DEFAULT '',
    last_heartbeat_at TEXT DEFAULT ''
);

-- Team Tasks
CREATE TABLE team_tasks (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT DEFAULT 'inbox',
    assignees TEXT DEFAULT '[]',
    priority INTEGER DEFAULT 3,
    labels TEXT DEFAULT '[]',
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT DEFAULT '',
    blocked_reason TEXT DEFAULT ''
);

-- Team Messages
CREATE TABLE team_messages (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    thread_id TEXT DEFAULT '',
    from_agent TEXT DEFAULT '',
    from_user TEXT DEFAULT '',
    content TEXT NOT NULL,
    mentions TEXT DEFAULT '[]',
    created_at TEXT NOT NULL,
    delivered INTEGER DEFAULT 0
);

-- Pending Messages (Mailbox)
CREATE TABLE team_pending_messages (
    id TEXT PRIMARY KEY,
    to_agent TEXT NOT NULL,
    from_agent TEXT DEFAULT '',
    from_user TEXT DEFAULT '',
    content TEXT NOT NULL,
    thread_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    delivered INTEGER DEFAULT 0,
    delivered_at TEXT DEFAULT ''
);

-- Team Facts
CREATE TABLE team_facts (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    author TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(team_id, key)
);

-- Team Activities
CREATE TABLE team_activities (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    type TEXT NOT NULL,
    agent_id TEXT DEFAULT '',
    message TEXT NOT NULL,
    related_id TEXT DEFAULT '',
    created_at TEXT NOT NULL
);

-- Team Documents
CREATE TABLE team_documents (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    task_id TEXT DEFAULT '',
    title TEXT NOT NULL,
    doc_type TEXT DEFAULT 'deliverable',
    content TEXT NOT NULL,
    format TEXT DEFAULT 'markdown',
    file_path TEXT DEFAULT '',
    version INTEGER DEFAULT 1,
    author TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Thread Subscriptions
CREATE TABLE team_thread_subscriptions (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    subscribed_at TEXT NOT NULL,
    reason TEXT DEFAULT 'auto',
    UNIQUE(team_id, thread_id, agent_id)
);

-- Agent Working State
CREATE TABLE agent_working_state (
    agent_id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    current_task_id TEXT DEFAULT '',
    status TEXT DEFAULT 'idle',
    next_steps TEXT DEFAULT '',
    context TEXT DEFAULT '',
    updated_at TEXT NOT NULL
);
```

---

## Integration with Existing Systems

### SubagentManager

Teams work alongside the existing subagent system:

- **Subagents**: Ephemeral, spawned for parallel work, discarded after completion
- **Persistent Agents**: Long-lived, maintain state, managed via TeamManager

Both can be used together:
```
Main Agent → spawn_subagent → Subagent (parallel task)
         ↘ TeamManager → PersistentAgent (long-running role)
```

### Scheduler

Persistent agent heartbeats use the existing scheduler:

```go
// TeamManager creates heartbeat jobs automatically
func (tm *TeamManager) createHeartbeatJob(agent *PersistentAgent) {
    job := &scheduler.Job{
        ID:       fmt.Sprintf("heartbeat-%s", agent.ID),
        Schedule: agent.HeartbeatSchedule,
        Type:     "cron",
        Command:  tm.buildHeartbeatPrompt(agent),
        Enabled:  true,
    }
    tm.scheduler.Add(job)
}
```

### Session Isolation

Each agent run uses session isolation:
- Separate session ID per agent
- Independent conversation history
- Isolated tool permissions

---

## Standup Generation

The `team_memory(action="standup")` tool generates a daily standup summary:

```
📊 DAILY STANDUP — Feb 21, 2026

✅ COMPLETED TODAY
• Implement authentication (by jarvis)
• Write API docs (by loki)

🔄 IN PROGRESS
• Add unit tests (jarvis)

🚫 BLOCKED
• Deploy to staging — Waiting for CI credentials

👀 NEEDS REVIEW
• OAuth callback handler
```

---

## Best Practices

### Agent Design

1. **Clear Roles**: Each agent should have a well-defined role
2. **Specific Instructions**: Provide detailed instructions for expected behavior
3. **Appropriate Heartbeat**: Set heartbeat interval based on task frequency
4. **Skill Alignment**: Assign skills that match the agent's role

### Task Management

1. **Descriptive Titles**: Use clear, actionable task titles
2. **Proper Assignment**: Assign tasks to appropriate agents
3. **Status Updates**: Keep task status current
4. **Use Comments**: Add context via thread comments

### Communication

1. **Use @Mentions**: Direct messages to specific agents
2. **Thread Context**: Keep related messages in the same thread
3. **Facts for Knowledge**: Store persistent knowledge as facts
4. **Activity Awareness**: Monitor activity feed for team updates

### Notifications

1. **Always Notify on Completion**: Use `task_completed` when done
2. **Notify on Errors**: Use `task_failed` or `agent_error` immediately
3. **Notify on Blocks**: Use `task_blocked` when waiting for external input
4. **Use Appropriate Priority**: Priority 2 for failures, 3 for normal, 4 for progress

---

## Troubleshooting

### Agent Not Responding to Heartbeats

1. Check agent status (`team_agent(action="get", agent_id="...")`)
2. Verify scheduler job exists
3. Check heartbeat schedule expression
4. Review agent logs

### Missing @Mentions

1. Verify agent ID matches the mention
2. Check pending messages (`team_comm(action="mention_check", agent_id="...")`)
3. Ensure agent is in the same team

### Task Not Showing

1. Verify team ID/name matches
2. Check task status filter
3. Ensure proper assignment

### Team Resolution Fails

1. Check team exists with `team_manage(action="list")`
2. Try using exact team ID instead of name
3. Check for duplicate team names

---

## Files Reference

| File | Purpose |
|------|---------|
| `team_types.go` | Data structures (Team, PersistentAgent, TeamTask, etc.) |
| `team_memory.go` | TeamMemory implementation (tasks, messages, facts) |
| `team_manager.go` | TeamManager implementation (lifecycle, heartbeats, team resolution) |
| `team_tools.go` | Dispatcher tools (5 tools with action parameter) |
| `team_tools_dispatcher_test.go` | Unit tests for dispatcher tools |
| `notification_dispatcher.go` | Notification routing and delivery |
| `notification_dispatcher_test.go` | Unit tests for NotificationDispatcher |
| `team_memory_test.go` | Unit tests for TeamMemory |
| `team_manager_test.go` | Unit tests for TeamManager |
