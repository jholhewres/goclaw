---
name: memory
description: "Store and retrieve long-term memories (facts, preferences, context)"
trigger: automatic
---

# Memory

Long-term memory for remembering information across conversations and sessions.

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
│ memory(save)  │  │memory(search) │  │ memory(list)  │
│ (store fact)  │  │ (find related)│  │ (browse all)  │
└───────┬───────┘  └───────┬───────┘  └───────────────┘
        │                  │
        │                  │
        ▼                  ▼
┌───────────────────────────────────────────────────────┐
│                   MEMORY STORAGE                       │
│  ~/.devclaw/memory/MEMORY.md                          │
│  + Semantic Search Index (vector embeddings)          │
└───────────────────────────────────────────────────────┘
```

## Tools

| Tool | Action | Use When |
|------|--------|----------|
| `memory(save)` | Store new memory | Learn something important |
| `memory(search)` | Find related memories | Need past context |
| `memory(list)` | Browse recent memories | Review what's stored |
| `memory(index)` | Rebuild search index | After manual edits |

## Categories

| Category | Use For | Example |
|----------|---------|---------|
| `fact` | Objective information | "User works at Acme Corp" |
| `preference` | User preferences | "Prefers dark mode" |
| `event` | Important events | "Started new project on Jan 15" |
| `summary` | Conversation summaries | "Discussed migration plan" |

## When to Use

| Action | Trigger |
|--------|---------|
| `save` | User shares personal info, preferences, important context |
| `search` | Before answering questions that might relate to past context |
| `list` | Reviewing stored information |
| `index` | After manually editing MEMORY.md |

## When NOT to Use Memory

| Data Type | Use Instead |
|-----------|-------------|
| API keys, tokens, passwords | **vault_save** (encrypted) |
| Database URLs with credentials | **vault_save** |
| Private keys, secrets | **vault_save** |
| Temporary session data | Context only (don't persist) |

**Rule:** If exposure could cause harm → vault, not memory.

## Saving Memories

```bash
memory(
  action="save",
  content="User prefers 2-space indentation for all code",
  category="preference"
)
# Output: Memory saved with timestamp: 2026-02-24 14:30
```

```bash
memory(
  action="save",
  content="Project devclaw is a CLI tool in Go for AI agent orchestration",
  category="fact"
)
# Output: Memory saved with timestamp: 2026-02-24 14:31
```

## Searching Memories

```bash
memory(
  action="search",
  query="user preferences about theme",
  limit=5
)
# Output:
# [1] User prefers dark mode in all applications (preference, 2d ago)
# [2] User prefers 2-space indentation for all code (preference, 1w ago)
```

```bash
memory(
  action="search",
  query="project devclaw",
  limit=10
)
# Output:
# [1] Project devclaw is a CLI tool in Go... (fact, 1w ago)
# [2] DevClaw uses prompt layers for context injection (fact, 3d ago)
```

## Listing Memories

```bash
memory(action="list", limit=20)
# Output:
# Recent memories (5):
# [2026-02-24] User prefers dark mode (preference)
# [2026-02-23] Project devclaw is a CLI tool (fact)
# [2026-02-22] Discussed migration plan (summary)
```

## Rebuilding Index

```bash
memory(action="index")
# Output: Search index rebuilt with 47 memories
```

## Common Patterns

### Learning Preferences
```bash
# User: "I always use tabs, not spaces"

# Save the preference
memory(
  action="save",
  content="User prefers tabs over spaces for indentation",
  category="preference"
)

send_message("Got it! I'll use tabs for indentation in your code.")
```

### Using Stored Preferences
```bash
# User: "Format this file"

# Search for formatting preferences first
memory(action="search", query="indentation preference")
# Output: User prefers tabs over spaces...

# Apply the preference
edit_file(...using tabs...)
```

### Project Context
```bash
# User: "We're building a real-time chat system with WebSockets"

# Save context
memory(
  action="save",
  content="Building real-time chat system using WebSockets. Stack: Node.js, Socket.io, Redis for pub/sub",
  category="fact"
)

# Later, this context is available when working on the project
```

### Session Summary
```bash
# At end of complex discussion

memory(
  action="save",
  content="Migration discussion: Decided on gradual migration from REST to GraphQL. Starting with user endpoints. Using Apollo Server.",
  category="summary"
)
```

### Before Answering Questions
```bash
# User: "What's my favorite editor?"

# Search memory first
memory(action="search", query="favorite editor")
# Output: User prefers VS Code with Vim keybindings

send_message("You prefer VS Code with Vim keybindings, if I recall correctly!")
```

## Troubleshooting

### "No memories found"

**Cause:** Nothing stored yet or search query too specific.

**Solution:**
```bash
# List all to see what's stored
memory(action="list", limit=50)

# Try broader search
memory(action="search", query="project")  # Instead of "project architecture details"
```

### Search not finding relevant memories

**Cause:** Query doesn't match semantic meaning.

**Solution:**
```bash
# Try different query terms
memory(action="search", query="editor preference")
memory(action="search", query="VS Code")
memory(action="search", query="development tools")
```

### "Memory not saved"

**Cause:** Content too long or invalid category.

**Solution:**
```bash
# Keep content concise
memory(
  action="save",
  content="Brief, focused memory content",
  category="fact"  # Use valid category: fact, preference, event, summary
)
```

### Index out of sync

**Cause:** Manual edits to MEMORY.md file.

**Solution:**
```bash
memory(action="index")
# Rebuilds search index from MEMORY.md
```

## Workflow Examples

### New User Onboarding
```bash
# User: "Hi, I'm starting a new project"

# Ask about preferences
# User: "I use Python, prefer black formatting, and work in the morning"

# Save preferences
memory(action="save", content="User uses Python as primary language", category="fact")
memory(action="save", content="User prefers black formatter for Python", category="preference")
memory(action="save", content="User is most productive in the morning", category="preference")
```

### Project Setup
```bash
# User: "Starting a new React project with TypeScript"

# Save context
memory(
  action="save",
  content="New project: React + TypeScript. Using Vite for build. Testing with Vitest.",
  category="fact"
)

# Later sessions know the stack
```

### Continuous Learning
```bash
# Throughout conversation, save important context

# User mentions team structure
memory(action="save", content="Team: 3 backend devs, 2 frontend, 1 DevOps", category="fact")

# User mentions deadline
memory(action="save", content="Project deadline: March 15, 2026", category="event")

# User mentions constraint
memory(action="save", content="Budget constraint: must use open-source tools only", category="fact")
```

## Tips

- **Save proactively**: When you learn something important, save immediately
- **Search first**: Before answering questions, search for relevant context
- **Be specific**: Clear, descriptive content is easier to find later
- **Use categories**: Helps organize and filter memories
- **Don't duplicate**: Check if information already exists
- **Keep it concise**: Shorter memories are easier to search

## Memory vs Vault vs Context

| Store in Memory | Store in Vault | Keep in Context |
|-----------------|----------------|-----------------|
| User preferences | API keys | Current task |
| Project details | Passwords | Active files |
| Team structure | Tokens | Recent messages |
| Past decisions | Secret URLs | Temporary state |
| General facts | Credentials | Working memory |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Storing API keys in memory | Use vault for secrets |
| Vague content: "likes stuff" | Be specific: "prefers dark themes" |
| Not searching before answering | Always search for relevant context |
| Duplicating memories | Check if already stored |
| Long paragraphs | Keep memories focused and concise |
