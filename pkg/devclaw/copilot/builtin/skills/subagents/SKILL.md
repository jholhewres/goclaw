---
name: subagents
description: "Spawn isolated agents and communicate across sessions"
trigger: automatic
---

# Subagents & Sessions

Multi-agent system for spawning isolated workers and coordinating across sessions.

## CRITICAL RULES

| Rule | Reason |
|-------|--------|
| Use `spawn_subagent` for complex/parallel tasks | Don't overload main context |
| **DO NOT** use `scheduler` when asked for subagent | Different tools |
| Wait for result with `wait_subagent` | If you need the result |
| Use descriptive labels | Easier identification |

### Wrong
```
User: "create a subagent to create a skill"
Agent: Uses scheduler(action=add)
Agent: Creates schedule
```

### Correct
```
User: "create a subagent to create a skill"
Agent: spawn_subagent(task="Create skill...", label="skill-creator")
```

## Architecture

```
                      Main Agent
                   (Current Session)
                           |
         +-----------------+-----------------+
         |                                   |
         v                                   v
+-----------------+                 +-----------------+
|    SPAWNING     |                 | COMMUNICATION   |
+-----------------+                 +-----------------+
| spawn_subagent  |                 | sessions        |
| list_subagents  |                 |  (action=list)  |
| wait_subagent   |                 |  (action=send)  |
| stop_subagent   |                 |  (action=export)|
+---------+-------+                 |  (action=delete)|
         |                          +--------+--------+
         v                                   v
+-----------------+                 +-----------------+
|  Subagent Run   |                 | Existing Session|
|  (isolated)     |                 | (another agent) |
|  - New session  |                 | - Separate chat |
|  - Limited ctx  |                 | - Cross-agent   |
|  - Reports back |                 |   messaging     |
+-----------------+                 +-----------------+
```

## Two Modes

| Mode | Tools | Purpose |
|------|-------|---------|
| **Spawning** | `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent` | Create new isolated agents |
| **Communication** | `sessions(action=list/send/export/delete)` | Talk to existing agents |

---

## Mode 1: Spawning Subagents

### When to Use

| Scenario | Use spawn_subagent |
|---------|-------------------|
| Long research task | Yes |
| Multiple parallel tasks | Yes |
| Create skills/plugins | Yes |
| Complex analysis | Yes |
| Background processing | Yes |

### When NOT to Use

| Scenario | Alternative |
|---------|-------------|
| Schedule task for specific time | `scheduler(action=add)` |
| Reminder | `scheduler(action=add, type=at)` |
| Communicate with existing agent | `sessions(action=send)` |

### Spawn a Subagent

```bash
spawn_subagent(
  task="Create a skill for ViaCEP API. When user provides a CEP, fetch and return address information.",
  label="viacep-skill-creator"
)
# Output: Subagent spawned with ID: sub_abc123
```

### List Running Subagents

```bash
list_subagents()
# Output:
# Subagents (2):
# - sub_abc123 [viacep-skill-creator]: running, 2m elapsed
# - sub_def456 [research-api]: completed, 45s
```

### Wait for Completion

```bash
wait_subagent(subagent_id="sub_abc123", timeout=300)
# Output:
# Subagent completed.
# Result: Skill 'viacep' created successfully...
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
# Use the result...
```

#### Parallel Tasks
```bash
# Spawn multiple subagents
spawn_subagent(task="Research topic A", label="research-a")
spawn_subagent(task="Research topic B", label="research-b")
spawn_subagent(task="Research topic C", label="research-c")

# Check status
list_subagents()

# Wait for each
result_a = wait_subagent(subagent_id="sub_001")
result_b = wait_subagent(subagent_id="sub_002")
result_c = wait_subagent(subagent_id="sub_003")
```

---

## Mode 2: Inter-Agent Communication

### Discover Other Agents

```bash
sessions(action="list")
# Output:
# Active sessions (3):
# - [whatsapp] 5511999999 (id: abc123, ws: main) -- 15 msgs -- last active: 2m ago
# - [webui] user-session (id: def456, ws: dev) -- 8 msgs -- last active: 5m ago
```

### Send Message to Another Agent

```bash
sessions(
  action="send",
  session_id="abc123",
  message="Task completed. Results saved to output.md",
  sender_label="research-agent"
)
# Output: Message delivered to session abc123 (channel: whatsapp).
```

### Export Session

```bash
sessions(action="export", session_id="abc123")
# Output:
# {
#   "session_id": "abc123",
#   "messages": [...],
#   "metadata": {...}
# }
```

---

## Complete Workflow Examples

### Create Skill via Subagent
```bash
# User: "create a subagent to create a skill for https://viacep.com.br/"

# 1. Spawn subagent (NOT scheduler!)
spawn_subagent(
  task="Create skill for ViaCEP API (https://viacep.com.br/).
        The skill should:
        1. Accept CEP from user
        2. Fetch information from API
        3. Return formatted address
        Include script to make the request.",
  label="viacep-skill"
)

# 2. Check status
list_subagents()

# 3. Wait for result
result = wait_subagent(subagent_id="sub_abc", timeout=300)

# 4. Inform user
send_message("ViaCEP skill created! " + result)
```

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

# 4. Get results when ready
wait_subagent(subagent_id="sub_abc123")
```

### Cross-Agent Collaboration
```bash
# 1. Find collaborator session
sessions(action="list")

# 2. Request help
sessions(
  action="send",
  session_id="backend-agent-session",
  message="Need API endpoint for user preferences. Can you create GET /api/user/preferences?",
  sender_label="frontend-agent"
)

# Backend agent responds via sessions(action=send) to your session
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

### Used wrong tool

**Cause:** Confusion between spawn and scheduler.

**Correction:**
- Subagent = immediate parallel execution
- Scheduler = schedule for specific time

### "Session not found"

**Cause:** Session doesn't exist or wrong ID.

**Solution:**
```bash
sessions(action="list")  # List all first
```

---

## Tips

- **Descriptive labels**: "viacep-skill" not "agent1"
- **Reasonable timeouts**: Don't wait forever
- **Check status before waiting**: Avoid blocking if already done
- **Stop stuck subagents**: Free resources
- **Fire and forget for non-urgent**: Don't wait if not needed

## Common Mistakes

| Mistake | Correction |
|---------|-----------|
| Using `scheduler` when asked for subagent | Use `spawn_subagent` |
| Not checking list_subagents | Verify status |
| Waiting without timeout | Always set timeout |
| Vague task in spawn | Be specific about expected output |
| Not stopping stuck agents | Clean up with `stop_subagent` |
