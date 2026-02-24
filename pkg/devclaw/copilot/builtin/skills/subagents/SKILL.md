---
name: subagents
description: "Communicate with other agents and manage multi-agent sessions"
trigger: automatic
---

# Subagents & Sessions

Multi-agent communication and session management for coordinating work across multiple agent instances.

## Tools

| Tool | Action |
|------|--------|
| `sessions_list` | List active sessions across all workspaces |
| `sessions_send` | Send message to another session (inter-agent) |
| `sessions_export` | Export session history as JSON |
| `sessions_delete` | Delete a session (use carefully) |

## When to Use

| Tool | When |
|------|------|
| `sessions_list` | Discover other running agents |
| `sessions_send` | Send results, request collaboration, notify |
| `sessions_export` | Backup or analyze conversation history |
| `sessions_delete` | Clean up old/test sessions |

## Inter-Agent Communication

### Discover other agents
```bash
sessions_list()
# Output:
# Active sessions (3):
# - [whatsapp] 5511999999 (id: abc123, ws: main) — 15 msgs — last active: 2m ago
# - [webui] user-session (id: def456, ws: dev) — 8 msgs — last active: 5m ago
```

### Send message to another agent
```bash
sessions_send(
  session_id="abc123",
  message="Task completed. Results saved to output.md",
  sender_label="research-agent"
)
# Output: Message delivered to session abc123 (channel: whatsapp).
```

### Export session for analysis
```bash
sessions_export(session_id="abc123")
# Output: {"messages": [...], "metadata": {...}}
```

## Common Patterns

### Notify main agent of completion
```
1. Complete background task
2. sessions_send(session_id="main-session", message="Research complete. Found 5 relevant articles.")
3. Results delivered to main conversation
```

### Request collaboration
```
1. sessions_list() — find collaborator session
2. sessions_send(session_id="collaborator", message="Need help with API integration. Can you assist?")
3. Wait for response in your session
```

### Handoff workflow
```
1. sessions_export(session_id="current") — get context
2. sessions_send(session_id="next-agent", message="Handoff: Please continue from step 3. Context: ...")
3. Other agent receives full context
```

## Session ID Resolution

The `session_id` parameter can be:
- Full session ID from `sessions_list`: `"abc123def456"`
- Short match if unique: `"abc123"`

## Important Notes

| Note | Reason |
|------|--------|
| Messages appear as system messages | Recipient sees `[Inter-agent message from X]: ...` |
| `sender_label` helps identify source | Use descriptive names: "research-agent", "dev-assistant" |
| `sessions_delete` is destructive | Removes conversation history permanently |

## Multi-Agent Architecture

```
┌─────────────┐     sessions_send     ┌─────────────┐
│  Agent A    │ ───────────────────▶ │  Agent B    │
│  (main)     │ ◀─────────────────── │  (worker)   │
└─────────────┘     sessions_send     └─────────────┘
```

- Each agent has its own session and context
- Messages are injected as system entries
- Use `sessions_list` to discover available agents
- Use `sessions_send` for all cross-agent communication
