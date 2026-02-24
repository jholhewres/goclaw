---
name: teams
description: "Manage persistent agents and team coordination with shared memory"
trigger: automatic
---

# Agent Teams

Teams system for persistent agents with shared memory, tasks, and communication.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Team                                     │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    Shared Memory                         │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │    │
│  │  │   Facts     │  │  Documents  │  │   Standup   │      │    │
│  │  │ (key-value) │  │ (deliverables)│ │  (reports) │      │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘      │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Agent A   │  │   Agent B   │  │   Agent C   │              │
│  │  (siri)     │  │  (jarvis)   │  │  (loki)     │              │
│  │  ┌───────┐  │  │  ┌───────┐  │  │  ┌───────┐  │              │
│  │  │Task 1 │  │  │  │Task 2 │  │  │  │Task 3 │  │              │
│  │  └───────┘  │  │  └───────┘  │  │  └───────┘  │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│         │                │                │                      │
│         └────────────────┼────────────────┘                      │
│                          │                                       │
│                   ┌──────┴──────┐                               │
│                   │ Communication│                               │
│                   │ @mentions    │                               │
│                   │ notifications│                               │
│                   └─────────────┘                               │
└─────────────────────────────────────────────────────────────────┘
```

## Tools Overview

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

## Task Status Workflow

```
inbox → assigned → in_progress → review → done
                     │
                     └──→ blocked
                           │
                           └──→ cancelled
```

## Agent Heartbeat Pattern

On each heartbeat/trigger, agents should follow this pattern:

```
┌─────────────────────────────────────────────────────┐
│                   AGENT HEARTBEAT                    │
├─────────────────────────────────────────────────────┤
│ 1. CHECK INCOMING                                   │
│    team_comm(action="mention_check", agent_id="X")  │
│                                                     │
│ 2. CHECK WORKING STATE                              │
│    team_agent(action="working_get", agent_id="X")   │
│                                                     │
│ 3. CHECK TASKS                                      │
│    team_task(action="list", assignee_filter="X")    │
│                                                     │
│ 4. DO WORK                                          │
│    team_agent(action="working_update", ...)         │
│                                                     │
│ 5. NOTIFY ON COMPLETION                             │
│    team_comm(action="notify", type="...", ...)      │
│                                                     │
│ 6. CLEAR STATE WHEN DONE                            │
│    team_agent(action="working_clear", ...)          │
└─────────────────────────────────────────────────────┘
```

## Working State Management

### Get Current State
```bash
team_agent(action="working_get", agent_id="siri")
# Output:
# status: idle
# current_task: none
# next_steps: []
# context: ""
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
# Output: Working state updated for agent 'siri'
```

### Clear Working State
```bash
team_agent(action="working_clear", agent_id="siri")
# Output: Working state cleared for agent 'siri'
```

## Notification Types

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
# Output: Notification sent
```

### List Notifications
```bash
team_comm(action="notify_list")
# Output:
# Notifications (3):
# [14:30] task_completed: Report generated (siri)
# [14:25] task_blocked: Waiting for API key (jarvis)
# [14:20] agent_error: Connection timeout (loki)
```

## Shared Memory

### Facts (Key-Value)
```bash
# Save fact
team_memory(action="fact_save", key="api_version", value="v2.1.0")
# Output: Fact 'api_version' saved

# List facts
team_memory(action="fact_list")
# Output:
# Facts (3):
# - api_version: v2.1.0
# - database: postgres
# - region: us-east-1

# Delete fact
team_memory(action="fact_delete", key="old_config")
# Output: Fact 'old_config' deleted
```

### Documents
```bash
# Create document
team_memory(
  action="doc_create",
  title="API Design Document",
  doc_type="deliverable",
  content="# API Design\n\n## Endpoints\n- GET /users\n- POST /data",
  task_id="abc12345"
)
# Output: Document created with ID: doc-xyz

# List documents
team_memory(action="doc_list", doc_type="deliverable")
# Output:
# Documents (2):
# - API Design Document (doc-xyz)
# - Database Schema (doc-abc)

# Get document
team_memory(action="doc_get", doc_id="doc-xyz")
# Output: Full document content...
```

## Communication

### Comment on Task
```bash
team_comm(
  action="comment",
  task_id="abc12345",
  content="@loki can you review this implementation?"
)
# Output: Comment added to task abc12345
```

### Check Mentions
```bash
team_comm(action="mention_check", agent_id="loki")
# Output:
# Mentions (1):
# Task abc12345: "@loki can you review this implementation?"
```

### Send Direct Message
```bash
team_comm(
  action="send_message",
  to_agent="jarvis",
  content="Deployment complete. New version is live."
)
# Output: Message sent to jarvis
```

## Complete Workflow Examples

### Heartbeat Cycle (Full)
```bash
# Step 1: Check for mentions
team_comm(action="mention_check", agent_id="siri")
# Output: 1 mention in task abc12345: "@siri can you check the API?"

# Step 2: Check current working state
team_agent(action="working_get", agent_id="siri")
# Output: status=idle, no current task

# Step 3: Check assigned tasks
team_task(action="list", assignee_filter="siri")
# Output: 2 tasks: abc12345 (in_progress), def67890 (inbox)

# Step 4: Update working state
team_agent(
  action="working_update",
  agent_id="siri",
  current_task_id="abc12345",
  status="working",
  next_steps="1. Read API docs\n2. Test endpoint\n3. Report findings",
  context="Investigating API issue reported by user"
)
# Output: Working state updated

# Step 5: Do the work...
# ... perform task ...

# Step 6: Complete and notify
team_task(action="update", task_id="abc12345", status="done")
team_comm(
  action="notify",
  type="task_completed",
  message="API issue resolved. Root cause was rate limiting.",
  task_id="abc12345",
  priority=3
)
# Output: Notification sent

# Step 7: Clear working state
team_agent(action="working_clear", agent_id="siri")
# Output: Working state cleared
```

### Task Resolution Flow
```bash
# User requests a feature via any agent
team_task(
  action="create",
  title="Add dark mode support",
  description="Implement dark mode toggle in settings",
  assignee="frontend-agent",
  priority=2
)
# Output: Created task xyz78901

# Assigned agent picks up task
team_task(action="update", task_id="xyz78901", status="in_progress")
team_agent(
  action="working_update",
  agent_id="frontend-agent",
  current_task_id="xyz78901",
  status="working"
)

# Agent completes and documents
team_memory(
  action="doc_create",
  title="Dark Mode Implementation",
  doc_type="deliverable",
  content="## Changes\n- Added theme toggle in settings\n- CSS variables for colors\n- LocalStorage persistence",
  task_id="xyz78901"
)

# Notify completion
team_comm(
  action="notify",
  type="task_completed",
  message="Dark mode implemented and ready for review",
  task_id="xyz78901"
)

# Close task
team_task(action="update", task_id="xyz78901", status="review")
```

### Cross-Agent Collaboration
```bash
# Agent A needs help from Agent B
team_comm(
  action="send_message",
  to_agent="backend-agent",
  content="Need API endpoint for user preferences. Can you create GET /api/user/preferences?"
)
# Output: Message sent

# Agent B checks messages
team_comm(action="mention_check", agent_id="backend-agent")
# Output: 1 message from frontend-agent

# Agent B creates task and works on it
team_task(
  action="create",
  title="User preferences API endpoint",
  assignee="backend-agent",
  priority=2
)

# Agent B responds when done
team_comm(
  action="send_message",
  to_agent="frontend-agent",
  content="Done! Endpoint is live at /api/user/preferences. Returns JSON with theme, language, etc."
)
```

### Standup Report
```bash
team_memory(action="standup", agent_id="siri")
# Output:
# Standup for siri:
# Yesterday: Completed 3 tasks, 1 blocked
# Today: Working on API integration
# Blockers: Waiting for credentials from ops team
```

## Troubleshooting

### "Team not found"

**Cause:** Invalid team_id or team doesn't exist.

**Solution:**
```bash
team_manage(action="list")
# Shows all available teams
```

### "Agent not found"

**Cause:** Agent doesn't exist in team.

**Solution:**
```bash
team_agent(action="list")
# Shows all agents in team
```

### "Task not found"

**Cause:** Invalid task_id.

**Solution:**
```bash
team_task(action="list")
# Shows all tasks
```

### Mentions not triggering

**Cause:** Agent not checking mentions.

**Solution:**
- Ensure agent runs `mention_check` on heartbeat
- Verify agent_id is correct

## Tips

- **Always update working state**: Before/after tasks
- **Notify on completion/errors**: Keep team informed
- **Use facts for shared knowledge**: Avoid duplication
- **Subscribe to threads**: Auto-subscribe when commenting
- **Clear working state when done**: `working_clear`
- **Use descriptive task IDs**: Easier to reference

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not updating working state | Update before/after each task |
| Forgetting to notify | Always notify on task completion |
| Hardcoding team_id | Use name resolution or empty for auto |
| Not checking mentions | Include in heartbeat cycle |
| Leaving working state stale | Clear when task complete |
