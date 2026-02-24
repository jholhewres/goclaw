---
name: subagents
description: "Spawn isolated agents and communicate across sessions"
trigger: automatic
---

# Subagents & Sessions

Multi-agent system for spawning isolated workers and coordinating across sessions.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Agent                              │
│                   (Current Session)                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
         ┌─────────────────┴─────────────────┐
         │                                   │
         ▼                                   ▼
┌─────────────────┐                 ┌─────────────────┐
│    SPAWNING     │                 │ COMMUNICATION   │
├─────────────────┤                 ├─────────────────┤
│ spawn_subagent  │                 │ sessions_list   │
│ list_subagents  │                 │ sessions_send   │
│ wait_subagent   │                 │ sessions_export │
│ stop_subagent   │                 │ sessions_delete │
└────────┬────────┘                 └────────┬────────┘
         │                                   │
         │                                   │
         ▼                                   ▼
┌─────────────────┐                 ┌─────────────────┐
│  Subagent Run   │                 │ Existing Session│
│  (isolated)     │                 │ (another agent) │
│  - New session  │                 │ - Separate chat │
│  - Limited ctx  │                 │ - Cross-agent   │
│  - Reports back │                 │   messaging     │
└─────────────────┘                 └─────────────────┘
```

## Two Modes

| Mode | Tools | Purpose |
|------|-------|---------|
| **Spawning** | `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent` | Create new isolated agents |
| **Communication** | `sessions_list`, `sessions_send`, `sessions_export`, `sessions_delete` | Talk to existing agents |

---

## Mode 1: Spawning Subagents

Create isolated agents for parallel work, long tasks, or specialized operations.

### Tools

| Tool | Action | Returns |
|------|--------|---------|
| `spawn_subagent` | Create isolated agent | subagent_id |
| `list_subagents` | List running subagents | status list |
| `wait_subagent` | Wait for completion | result |
| `stop_subagent` | Stop running subagent | confirmation |

### When to Use

| Scenario | Tool |
|----------|------|
| Long research task | `spawn_subagent` (non-blocking) |
| Parallel analysis | `spawn_subagent` multiple |
| Check running agents | `list_subagents` |
| Need result before continuing | `wait_subagent` |
| Task no longer needed | `stop_subagent` |

### Spawn a Subagent

```bash
spawn_subagent(
  task="Research GraphQL vs REST performance comparison. Focus on mobile clients.",
  label="research-api"
)
# Output: Subagent spawned with ID: sub_abc123
```

### List Running Subagents

```bash
list_subagents()
# Output:
# Subagents (3):
# - sub_abc123 [research-api]: running, 2m elapsed
# - sub_def456 [test-runner]: completed, 45s
# - sub_ghi789 [doc-writer]: running, 5m elapsed
```

### Wait for Completion

```bash
wait_subagent(subagent_id="sub_abc123", timeout=300)
# Output:
# Subagent completed.
# Result: GraphQL shows 30% better performance on mobile for complex queries...
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
spawn_subagent(
  task="Generate weekly report and save to reports/ folder",
  label="reporter"
)

# Continue with other tasks...
# Check later with list_subagents()
```

#### Blocking Wait
```bash
# Spawn
spawn_subagent(
  task="Analyze logs for errors in the past 24 hours",
  label="log-analyzer"
)

# Wait for result (blocks until complete)
result = wait_subagent(subagent_id="sub_abc123", timeout=600)
# Use result...
```

#### Parallel Tasks
```bash
# Spawn multiple subagents
spawn_subagent(task="Research topic A", label="research-a")
spawn_subagent(task="Research topic B", label="research-b")
spawn_subagent(task="Research topic C", label="research-c")

# Check all are running
list_subagents()

# Wait for each
result_a = wait_subagent(subagent_id="sub_001")
result_b = wait_subagent(subagent_id="sub_002")
result_c = wait_subagent(subagent_id="sub_003")

# Combine results
```

---

## Mode 2: Inter-Agent Communication

Communicate with existing agents across sessions.

### Tools

| Tool | Action | Use When |
|------|--------|----------|
| `sessions_list` | List active sessions | Discover other agents |
| `sessions_send` | Send message to session | Collaboration, notifications |
| `sessions_export` | Export session history | Backup, analysis |
| `sessions_delete` | Delete a session | Cleanup (careful!) |

### Discover Other Agents

```bash
sessions_list()
# Output:
# Active sessions (3):
# - [whatsapp] 5511999999 (id: abc123, ws: main) — 15 msgs — last active: 2m ago
# - [webui] user-session (id: def456, ws: dev) — 8 msgs — last active: 5m ago
# - [discord] bot-channel (id: ghi789, ws: prod) — 42 msgs — last active: 10m ago
```

### Send Message to Another Agent

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
# Output:
# {
#   "session_id": "abc123",
#   "messages": [...],
#   "metadata": {...}
# }
```

### Delete Session (Careful!)

```bash
sessions_delete(session_id="old-session-id")
# Output: Session old-session-id deleted permanently
```

---

## Complete Workflow Examples

### Background Research
```bash
# 1. Spawn research agent
spawn_subagent(
  task="Research best practices for GraphQL pagination. Include cursor-based and offset-based approaches.",
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
spawn_subagent(
  task="Analyze error logs from past 24h. Count errors by type.",
  label="log-analyzer"
)

spawn_subagent(
  task="Check database slow queries. List queries over 1 second.",
  label="db-analyzer"
)

spawn_subagent(
  task="Review API response times. Identify endpoints over 500ms.",
  label="api-analyzer"
)

# 2. Wait for all
result1 = wait_subagent(subagent_id="sub_001", timeout=120)
result2 = wait_subagent(subagent_id="sub_002", timeout=120)
result3 = wait_subagent(subagent_id="sub_003", timeout=120)

# 3. Combine findings
send_message("Analysis complete:\n- Logs: 47 errors\n- DB: 3 slow queries\n- API: 2 slow endpoints")
```

### Cross-Agent Collaboration
```bash
# 1. Find collaborator session
sessions_list()
# Output: ... backend-agent session id: xyz789 ...

# 2. Request help
sessions_send(
  session_id="xyz789",
  message="Need API endpoint for user preferences. Can you create GET /api/user/preferences?",
  sender_label="frontend-agent"
)

# 3. Continue work while waiting...
# Backend agent responds via sessions_send to your session
```

### Handoff Workflow
```bash
# 1. Export current context
context = sessions_export(session_id="current")

# 2. Send to next agent
sessions_send(
  session_id="next-agent-session",
  message="Handoff: Please continue from step 3. Context: " + context,
  sender_label="current-agent"
)

# 3. Next agent receives full context
```

---

## Subagent Isolation

| Aspect | Behavior |
|--------|----------|
| Context | Limited bootstrap (AGENTS.md + TOOLS.md only) |
| Session | Separate from spawner |
| Result | Announced back to requester chat |
| Lifetime | Until completed or stopped |
| Resources | Independent execution |

---

## Troubleshooting

### "Subagent timeout"

**Cause:** Task taking longer than timeout.

**Solution:**
```bash
# Increase timeout
wait_subagent(subagent_id="sub_abc", timeout=600)  # 10 minutes

# Or check status without waiting
list_subagents()
```

### "Subagent failed"

**Cause:** Error in subagent execution.

**Debug:**
```bash
# Check status
list_subagents()

# Spawn with simpler task
spawn_subagent(task="Simpler version of task", label="debug")
```

### "Session not found"

**Cause:** Session doesn't exist or wrong ID.

**Solution:**
```bash
# List all sessions
sessions_list()

# Use exact session ID from list
```

### "Message not delivered"

**Cause:** Session offline or invalid.

**Debug:**
```bash
# Verify session is active
sessions_list()

# Check session_id is correct
```

---

## Tips

- **Use descriptive labels**: "research-api" not "agent1"
- **Set reasonable timeouts**: Don't wait forever
- **Check status before wait**: Avoid blocking if already done
- **Stop stuck subagents**: Free resources
- **Fire and forget for non-urgent**: Don't wait if not needed
- **Messages appear as system**: Recipient sees `[Inter-agent message from X]`

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not checking list_subagents | Verify status before/after |
| Waiting without timeout | Always set reasonable timeout |
| Vague task description | Be specific about expected output |
| Not stopping stuck agents | Clean up with stop_subagent |
| Guessing session IDs | Get from sessions_list first |

## Comparison: Spawn vs Session

| Aspect | spawn_subagent | sessions_send |
|--------|----------------|---------------|
| Creates new agent | Yes | No |
| Target | New isolated run | Existing session |
| Use case | Parallel work | Communication |
| Context | Limited | Full session |
| Result | wait_subagent | Message in session |

## When to Use Which

| Scenario | Use |
|----------|-----|
| Need parallel execution | `spawn_subagent` |
| Long-running background task | `spawn_subagent` |
| Talk to existing agent | `sessions_send` |
| Collaborate with running agent | `sessions_send` |
| Isolated research/analysis | `spawn_subagent` |
| Notify another session | `sessions_send` |
