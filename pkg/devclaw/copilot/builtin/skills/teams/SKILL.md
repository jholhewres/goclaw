---
name: teams
description: "Manage persistent agents and team coordination"
trigger: automatic
---

# Agent Teams

Teams system for persistent agents with shared memory, tasks, and communication.

## Quick Reference

| Tool | Actions |
|------|---------|
| `team_manage` | create, list, get, update, delete |
| `team_agent` | create, list, get, update, start, stop, delete, working_get, working_update, working_clear |
| `team_task` | create, list, get, update, assign, delete |
| `team_memory` | fact_save, fact_list, fact_delete, doc_create, doc_list, doc_get, doc_update, doc_delete, standup |
| `team_comm` | comment, mention_check, send_message, notify, notify_list |

## Team Resolution

The `team_id` parameter supports flexible resolution:

```bash
# By ID
team_task(action="list", team_id="5916dabf")

# By name (case insensitive, normalized)
team_task(action="list", team_id="devclaw-os")
team_task(action="list", team_id="DevClaw OS")

# Empty - uses single team automatically, or asks if multiple exist
team_task(action="list")
```

## Agent Workflow

### On Heartbeat/Trigger

```
1. CHECK INCOMING → team_comm(action="mention_check", agent_id="siri")
2. CHECK WORKING  → team_agent(action="working_get", agent_id="siri")
3. CHECK TASKS    → team_task(action="list", assignee_filter="siri")
4. DO WORK        → team_agent(action="working_update", ...)
5. NOTIFY         → team_comm(action="notify", type="...", message="...")
```

### Update Working State

```bash
team_agent(
  action="working_update",
  agent_id="siri",
  current_task_id="abc12345",
  status="working",
  next_steps="1. Process request\n2. Send response",
  context="Handling user query about documentation"
)
```

## Notification Pattern

Always notify on important events:

| Type | When | Priority |
|------|------|----------|
| `task_completed` | Task finished successfully | 3 |
| `task_failed` | Task execution failed | 2 |
| `task_blocked` | Waiting for external input | 3 |
| `task_progress` | Progress update (long tasks) | 4 |
| `agent_error` | Internal agent error | 2 |

### Send Notification

```bash
team_comm(
  action="notify",
  type="task_completed",
  message="Report generated successfully",
  task_id="abc12345",
  priority=3
)
```

## Task Status Workflow

```
inbox → assigned → in_progress → review → done
                     │
                     └──→ blocked
                           │
                           └──→ cancelled
```

## Shared Memory

### Facts (Key-Value)

```bash
# Save
team_memory(action="fact_save", key="api_version", value="v2.1.0")

# List
team_memory(action="fact_list")

# Delete
team_memory(action="fact_delete", key="api_version")
```

### Documents

```bash
# Create
team_memory(
  action="doc_create",
  title="API Design",
  doc_type="deliverable",  # deliverable | research | protocol | notes
  content="# API Design\n...",
  task_id="abc12345"
)

# List
team_memory(action="doc_list", doc_type="deliverable")
```

## Communication

### Comment on Task

```bash
team_comm(
  action="comment",
  task_id="abc12345",
  content="@loki can you review this?"
)
```

### Send Direct Message

```bash
team_comm(
  action="send_message",
  to_agent="jarvis",
  content="Deployment complete"
)
```

### Check Mentions

```bash
team_comm(action="mention_check", agent_id="loki")
```

## Best Practices

1. **Always update working state** - Before/after tasks
2. **Notify on completion/errors** - Keep team informed
3. **Use facts for shared knowledge** - Avoid duplication
4. **Subscribe to threads** - Auto-subscribe when commenting
5. **Clear working state when done** - `working_clear`
