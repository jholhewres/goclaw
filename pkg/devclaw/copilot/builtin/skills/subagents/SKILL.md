---
name: subagents
description: "Spawn isolated agents and communicate across sessions"
trigger: automatic
---

# Subagents & Sessions

Multi-agent system for spawning isolated workers and coordinating across sessions.

## Two Modes

| Mode | Tools | Purpose |
|------|-------|---------|
| **Spawning** | `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent` | Create new isolated agents |
| **Communication** | `sessions_list`, `sessions_send`, `sessions_export`, `sessions_delete` | Talk to existing agents |

---

## Spawning Subagents

Create isolated agents for parallel work, long tasks, or specialized operations.

### Tools

| Tool | Action |
|------|--------|
| `spawn_subagent` | Create a new isolated agent with a task |
| `list_subagents` | List spawned subagents and their status |
| `wait_subagent` | Wait for a subagent to complete |
| `stop_subagent` | Stop a running subagent |

### When to Use

| Tool | When |
|------|------|
| `spawn_subagent` | Long task, parallel work, specialized agent needed |
| `list_subagents` | Check what's running |
| `wait_subagent` | Need result before continuing |
| `stop_subagent` | Task no longer needed, or stuck |

### Spawn a Subagent

```bash
spawn_subagent(
  task="Research the best practices for GraphQL pagination and summarize findings",
  label="research-agent"
)
# Output: Subagent spawned with ID: sub_abc123
```

### List Running Subagents

```bash
list_subagents()
# Output:
# Subagents (2):
# - sub_abc123 [research-agent]: running, 2m elapsed
# - sub_def456 [test-runner]: completed, 45s
```

### Wait for Completion

```bash
wait_subagent(subagent_id="sub_abc123", timeout=300)
# Output: Subagent completed. Result: [summary of findings...]
```

### Stop a Subagent

```bash
stop_subagent(subagent_id="sub_abc123")
# Output: Subagent sub_abc123 stopped
```

### Spawning Patterns

#### Fire and Forget
```bash
# Spawn and continue working
spawn_subagent(task="Generate weekly report", label="reporter")
# Continue with other tasks...
# Check later with list_subagents()
```

#### Blocking Wait
```bash
# Spawn and wait for result
spawn_subagent(task="Analyze logs for errors", label="analyzer")
result = wait_subagent(subagent_id="sub_abc123", timeout=600)
# Use result...
```

#### Parallel Tasks
```bash
# Spawn multiple subagents
spawn_subagent(task="Research topic A", label="research-a")
spawn_subagent(task="Research topic B", label="research-b")

# Wait for both
list_subagents()  # Check status
wait_subagent(subagent_id="sub_abc123")
wait_subagent(subagent_id="sub_def456")
```

---

## Inter-Agent Communication

Communicate with existing agents across sessions.

### Tools

| Tool | Action |
|------|--------|
| `sessions_list` | List active sessions across all workspaces |
| `sessions_send` | Send message to another session |
| `sessions_export` | Export session history as JSON |
| `sessions_delete` | Delete a session (use carefully) |

### When to Use

| Tool | When |
|------|------|
| `sessions_list` | Discover other running agents |
| `sessions_send` | Send results, request collaboration, notify |
| `sessions_export` | Backup or analyze conversation history |
| `sessions_delete` | Clean up old/test sessions |

### Discover Agents

```bash
sessions_list()
# Output:
# Active sessions (3):
# - [whatsapp] 5511999999 (id: abc123, ws: main) — 15 msgs — last active: 2m ago
# - [webui] user-session (id: def456, ws: dev) — 8 msgs — last active: 5m ago
```

### Send Message

```bash
sessions_send(
  session_id="abc123",
  message="Task completed. Results saved to output.md",
  sender_label="research-agent"
)
# Output: Message delivered to session abc123 (channel: whatsapp).
```

### Export Session

```bash
sessions_export(session_id="abc123")
# Output: {"messages": [...], "metadata": {...}}
```

---

## Complete Workflow Examples

### Background Research
```bash
# 1. Spawn research agent
spawn_subagent(
  task="Research GraphQL vs REST performance comparison. Focus on mobile clients.",
  label="research-graphql"
)

# 2. Continue main work
# ... do other tasks ...

# 3. Check status
list_subagents()
# Output: sub_abc123 [research-graphql]: completed

# 4. Get results
wait_subagent(subagent_id="sub_abc123")
# Output: Research findings...
```

### Parallel Analysis
```bash
# 1. Spawn multiple analyzers
spawn_subagent(task="Analyze error logs from past 24h", label="log-analyzer")
spawn_subagent(task="Check database slow queries", label="db-analyzer")
spawn_subagent(task="Review API response times", label="api-analyzer")

# 2. Wait for all
result1 = wait_subagent(subagent_id="sub_001")
result2 = wait_subagent(subagent_id="sub_002")
result3 = wait_subagent(subagent_id="sub_003")

# 3. Combine results
# ... process combined findings ...
```

### Cross-Agent Collaboration
```bash
# 1. Find collaborator session
sessions_list()

# 2. Request help
sessions_send(
  session_id="backend-agent",
  message="Need API endpoint for user preferences. Can you create GET /api/user/preferences?"
)

# 3. Continue work while waiting...
# Backend agent responds via sessions_send to your session
```

---

## Subagent Isolation

| Aspect | Behavior |
|--------|----------|
| Context | Limited bootstrap (AGENTS.md + TOOLS.md only) |
| Session | Separate from spawner |
| Result | Announced back to requester chat |
| Lifetime | Until completed or stopped |

---

## Important Notes

| Note | Reason |
|------|--------|
| Subagents are isolated | Don't share full context with spawner |
| Use descriptive labels | Helps identify in list |
| Set reasonable timeouts | Don't wait forever |
| Stop stuck subagents | Free resources |
| Messages appear as system | Recipient sees `[Inter-agent message from X]: ...` |
| `sessions_delete` is destructive | Removes conversation history permanently |

---

## Architecture Overview

```
                    ┌─────────────────────────────────┐
                    │         Main Agent              │
                    │  (current session)              │
                    └────────┬────────────────────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
           ▼                 ▼                 ▼
    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
    │ Subagent A  │  │ Subagent B  │  │ Session C   │
    │ (spawned)   │  │ (spawned)   │  │ (existing)  │
    └─────────────┘  └─────────────┘  └─────────────┘
           │                 │                 │
           └─────────────────┴─────────────────┘
                             │
                    sessions_send / wait_subagent
```
