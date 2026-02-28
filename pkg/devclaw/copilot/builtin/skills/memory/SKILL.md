---
name: memory
description: "store and retrieve long-term memories (facts, preferences, context)"
trigger: automatic
---

# Memory

Long-term memory for remembering information across conversations and sessions.

## ⚠️ CRITICAL RULES

| Rule | Reason |
|------|--------|
| **NEVER** store secrets in memory | Use **vault** for API keys, passwords |
| **NEVER** store database URLs with credentials | Use **vault** |
| Only store **non-sensitive** information | Facts, preferences, context |

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
        ▼                  ▼
┌───────────────────────────────────────────────────────┐
│                   MEMORY STORAGE                       │
│  ./data/memory/MEMORY.md                          │
│  + Semantic Search Index                              │
└───────────────────────────────────────────────────────┘
```

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

## When NOT to Use Memory
| Data Type | Use Instead |
|-----------|-------------|
| API keys, tokens, passwords | **vault(action=save)** |
| Database URLs with credentials | **vault(action=save)** |
| Private keys, secrets | **vault(action=save)** |

**Rule:** If exposure could cause harm → vault, not memory.

## Saving Memories
```bash
memory(
  action="save",
  content="User prefers 2-space indentation for all code",
  category="preference"
)
```

## Searching Memories
```bash
memory(
  action="search",
  query="user preferences about theme",
  limit=5
)
```

## Listing Memories
```bash
memory(action="list", limit=20)
```

## Common Patterns

### Learning and Using Preferences
```bash
# User: "I always use tabs, not spaces"
memory(action="save", content="User prefers tabs over spaces", category="preference")

# Later: User: "Format this file"
memory(action="search", query="indentation preference")
# Apply preference found
```

## Memory vs Vault
| Store in Memory | Store in Vault |
|-----------------|----------------|
| User preferences | API keys |
| Project details | Passwords |
| Team structure | Secret URLs |
| Past decisions | Credentials |

## Common Mistakes
| Mistake | Correct Approach |
|---------|-----------------|
| Storing API keys in memory | Use vault for secrets |
| Vague content | Be specific |
| Not searching before answering | Search for relevant context |
