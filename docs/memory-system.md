# Memory System

DevClaw's memory system provides long-term storage and retrieval of information across conversations. It uses a hybrid search approach combining vector embeddings and BM25 full-text search.

---

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Memory Tool (Dispatcher)                 │
│  memory(action="save|search|list|index", ...)               │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│  File Store   │  │ SQLite Store  │  │ Hybrid Search │
│  (Markdown)   │  │ (FTS5+Vec)    │  │ (Vec + BM25)  │
└───────────────┘  └───────────────┘  └───────────────┘
```

---

## Storage Backends

### 1. File Store (`memory/`)

Markdown files for human-readable storage:

```
data/memory/
├── fact/
│   ├── 2024-01-15-api-key.md
│   └── 2024-01-16-preferences.md
├── preference/
│   └── 2024-01-10-language.md
├── event/
│   └── 2024-01-20-deployment.md
└── summary/
    └── 2024-01-25-sprint-review.md
```

**Format:**
```markdown
---
category: fact
created_at: 2024-01-15T10:30:00Z
keywords: [api, authentication, stripe]
---

The Stripe API key is stored in the vault under `stripe_api_key`.
```

### 2. SQLite Store

Database for fast querying with FTS5 and vector search:

```sql
CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    keywords TEXT,  -- JSON array
    embedding BLOB, -- Float32 vector
    created_at DATETIME,
    metadata TEXT   -- JSON object
);

-- Full-text search
CREATE VIRTUAL TABLE memories_fts USING fts5(
    content,
    keywords,
    content='memories',
    content_rowid='rowid'
);
```

---

## Memory Tool (Dispatcher)

### Actions

| Action | Description | Parameters |
|--------|-------------|------------|
| `save` | Save new memory | content, category, keywords, metadata |
| `search` | Search memories | query, category, limit |
| `list` | List memories | category, limit, offset |
| `index` | Rebuild search index | - |

### Usage Examples

```bash
# Save a fact
memory(
  action="save",
  content="The API uses JWT tokens with 24h expiration",
  category="fact",
  keywords=["api", "jwt", "authentication"]
)

# Search memories
memory(
  action="search",
  query="authentication token",
  limit=10
)

# List by category
memory(
  action="list",
  category="preference",
  limit=20
)

# Rebuild index
memory(action="index")
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | `save`, `search`, `list`, or `index` |
| `content` | string | For save | Memory content to store |
| `category` | string | For save/list | `fact`, `preference`, `event`, `summary` |
| `keywords` | []string | No | Tags for categorization |
| `metadata` | object | No | Additional structured data |
| `query` | string | For search | Search query text |
| `limit` | int | No | Max results (default: 20, max: 100) |
| `offset` | int | For list | Pagination offset |

---

## Categories

| Category | Description | Example |
|----------|-------------|---------|
| `fact` | Objective information | "Database runs on port 5432" |
| `preference` | User preferences | "User prefers dark theme" |
| `event` | Timestamped events | "Deployed v2.1 on 2024-01-15" |
| `summary` | Summarized knowledge | "Sprint review conclusions" |

---

## Hybrid Search

Memory uses a hybrid search combining two approaches:

### 1. Vector Search (Semantic)

- **Embedding**: Text converted to vector via configured embedding model
- **Similarity**: Cosine similarity between query and memory vectors
- **Use case**: Finding conceptually similar content

```
Query: "how to authenticate"
Matches: "API uses JWT tokens" (semantic similarity)
```

### 2. BM25 Search (Lexical)

- **Engine**: SQLite FTS5
- **Algorithm**: Okapi BM25
- **Use case**: Exact keyword matching

```
Query: "JWT expiration"
Matches: "JWT tokens with 24h expiration" (keyword match)
```

### 3. Score Fusion

Results from both methods are combined using reciprocal rank fusion:

```
final_score = (k / (k + rank_vector)) + (k / (k + rank_bm25))
```

Where `k = 60` (standard RRF constant).

---

## Prompt Layer Integration

Memory is integrated into the prompt via **LayerMemory** (priority 50).

### Layer Content

```go
func (p *PromptComposer) buildMemoryLayer() string {
    // Hybrid search for relevant memories
    memories := p.memoryStore.HybridSearch(input, 20)

    // Format for prompt
    var sb strings.Builder
    sb.WriteString("## Relevant Memories\n\n")
    for _, m := range memories {
        sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Category, m.Content))
    }
    return sb.String()
}
```

### Token Budget

```go
LayerMemory: config.TokenBudget.Memory, // Default: 2000 tokens
```

If memories exceed the budget, they are truncated with an indicator:

```
## Relevant Memories

- [fact] The API uses JWT tokens with 24h expiration
- [preference] User prefers dark theme
- [event] Deployed v2.1 on 2024-01-15
...
[truncated - 8 more memories available]
```

---

## Caching

Memory layer uses session-level caching:

| Setting | Value |
|---------|-------|
| TTL | 30 seconds |
| Strategy | Lazy refresh |
| Storage | In-memory map |

### Cache Flow

1. Check cache for fresh result (< 30s old)
2. If fresh: return cached memories
3. If stale: return cached, trigger background refresh
4. Next prompt: use refreshed results

This prevents memory search from blocking agent startup.

---

## Memory Lifecycle

### Automatic Save (Pre-Compaction)

Before context compaction, DevClaw flushes durable memories:

```go
// In assistant.go
if shouldCompact {
    // Trigger pre-compaction memory flush
    p.flushMemoryTurn(ctx, session)
    // Then compact
    p.compactContext(session)
}
```

The agent is given a special turn to save important information before it's lost to compaction.

### Manual Save

Users or agents can explicitly save memories:

```bash
memory(action="save", content="...", category="fact")
```

### Expiration

Memories don't expire automatically, but can be pruned:

```bash
# List old memories
memory(action="list", category="event", limit=100)

# Delete specific memory (manual curation)
# (Future: memory action="delete")
```

---

## Configuration

### YAML Config

```yaml
memory:
  enabled: true
  storage_dir: "data/memory"
  database_path: "data/memory.db"
  embedding_model: "text-embedding-3-small"
  max_results: 20
  cache_ttl: 30s

token_budget:
  memory: 2000
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DEVCLAW_MEMORY_DIR` | `data/memory` | Memory storage directory |
| `DEVCLAW_MEMORY_DB` | `data/memory.db` | SQLite database path |

---

## Builtin Skill

Memory has a builtin skill embedded in the binary:

**Location**: `pkg/devclaw/copilot/builtin/skills/memory/SKILL.md`

**Trigger**: `automatic` (always included in prompt)

**Purpose**: Provides usage guidance for the memory tool

```yaml
---
name: memory
description: "Long-term memory management"
trigger: automatic
---
```

---

## Implementation Files

| File | Description |
|------|-------------|
| `memory_tools.go` | Dispatcher tool implementation |
| `memory/store.go` | File store implementation |
| `memory/sqlite.go` | SQLite store with FTS5 |
| `memory/hybrid.go` | Hybrid search implementation |
| `prompt_layers.go` | LayerMemory integration |
| `builtin/skills/memory/SKILL.md` | Usage documentation |

---

## Best Practices

### For Agents

1. **Save proactively** - Don't wait for user to ask
2. **Use keywords** - Add relevant tags for better retrieval
3. **Choose right category** - Facts vs preferences vs events
4. **Keep concise** - Memory budget is limited
5. **Search before save** - Avoid duplicates

### For Configuration

1. **Tune embedding model** - Balance quality vs cost
2. **Adjust cache TTL** - Higher for stable content, lower for dynamic
3. **Monitor token usage** - Increase budget if memories are truncated

### For Searches

1. **Use natural language** - Vector search handles synonyms
2. **Include keywords** - BM25 benefits from exact terms
3. **Filter by category** - Narrow search space when possible

---

## Troubleshooting

### Memories Not Found

1. Check if memory is enabled: `memory(action="list")`
2. Rebuild index: `memory(action="index")`
3. Check storage directory exists
4. Verify database path in config

### Slow Searches

1. Check database size: `SELECT COUNT(*) FROM memories`
2. Rebuild FTS index: `memory(action="index")`
3. Reduce `max_results` in config

### Token Budget Exceeded

1. Increase `token_budget.memory` in config
2. Use more specific category filters
3. Reduce `max_results` in search calls

---

## Summary

| Aspect | Details |
|--------|---------|
| Storage | Markdown files + SQLite with FTS5 |
| Search | Hybrid (vector + BM25) |
| Tool | `memory(action="...")` dispatcher |
| Categories | fact, preference, event, summary |
| Prompt Layer | LayerMemory (priority 50) |
| Token Budget | ~2000 tokens |
| Cache TTL | 30 seconds |
