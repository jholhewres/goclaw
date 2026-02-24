---
name: memory
description: "Manage long-term memory to remember facts, preferences, and context"
trigger: automatic
---

# Memory

Long-term memory allows you to remember information across conversations and sessions. Use for **non-sensitive** facts, preferences, and context.

## When to Use

| Action | When |
|--------|------|
| `save` | Learn something new about the user |
| `search` | Find relevant past information before responding |
| `list` | Review recent memories |
| `index` | Rebuild search index after manual edits |

## When NOT to Use Memory

| Data Type | Use Instead |
|-----------|-------------|
| API keys, tokens, passwords | **vault_save** (encrypted storage) |
| Database URLs with credentials | **vault_save** |
| Private keys, secrets | **vault_save** |
| Temporary session data | Context only (don't persist) |

**Rule:** If it's a secret that could cause harm if exposed → use vault, not memory.

## Tool: `memory`

```bash
memory(action="...", ...)
```

### Actions

#### Save a Memory

```bash
memory(
  action="save",
  content="User prefers dark mode in all applications",
  category="preference"
)
```

**Categories:**
- `fact` - Objective information (default)
- `preference` - User preferences and tastes
- `event` - Important events or milestones
- `summary` - Summaries of conversations or work

#### Search Memories

```bash
memory(
  action="search",
  query="user preferences about theme",
  limit=10
)
```

Uses **semantic search** (vector + keyword hybrid) when available.

#### List Recent Memories

```bash
memory(action="list", limit=20)
```

#### Rebuild Search Index

```bash
memory(action="index")
```

## Best Practices

1. **Save proactively** - When you learn something important, save it immediately
2. **Search first** - Before answering questions, search for relevant context
3. **Be specific** - Use clear, descriptive content for better recall
4. **Use categories** - Helps organize and filter memories
5. **Don't duplicate** - Check if information already exists before saving

## Workflow Example

```
User: "What's my favorite editor?"

1. Search: memory(action="search", query="favorite editor")
2. If found → Answer with the memory
3. If not found → Ask user, then save the answer
```

## Storage

Memories are stored in `MEMORY.md` files and indexed for semantic search. The index enables finding conceptually similar content even without exact keyword matches.

## Category Examples

| Category | Example Content |
|----------|-----------------|
| `fact` | "User works at Company X as a backend developer" |
| `preference` | "User prefers TypeScript over JavaScript for new projects" |
| `event` | "User started new project 'clawd' on 2024-01-15" |
| `summary` | "Discussed migration from REST to GraphQL, decided to use Apollo" |

## Complete Workflow Examples

### Learning and Using Preferences
```
User: "I always use 2-space indentation"

1. memory(action="save", content="User prefers 2-space indentation for all code", category="preference")
   → Saved with timestamp

[Later session]

User: "Format this file"

1. memory(action="search", query="indentation preference")
   → Found: "User prefers 2-space indentation"
2. Apply 2-space formatting
```

### Project Context Flow
```
User: "We're building a CLI tool in Go called devclaw"

1. memory(action="save", content="Project devclaw: CLI tool in Go for AI agent orchestration", category="fact")

[Next session]

User: "Add a new command"

1. memory(action="search", query="project devclaw")
   → Found: "Project devclaw: CLI tool in Go..."
2. Create Go command following existing patterns
```
