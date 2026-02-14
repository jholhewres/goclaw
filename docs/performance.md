# GoClaw — Performance and Optimizations

Documentation of the performance strategies implemented in GoClaw, including concurrency, caching, memory management, and tuning.

---

## Overview

GoClaw is designed for high throughput with low latency, leveraging Go's concurrency model (goroutines + channels) to the fullest. The optimizations cover:

1. **Parallel tool execution** — semaphore for controlled concurrency.
2. **Concurrent subagents** — isolated goroutines for parallel work.
3. **Message queue with debounce** — batching of burst messages.
4. **Progressive streaming** — partial delivery without waiting for full response.
5. **Prompt caching** — cost and latency reduction on compatible providers.
6. **Incremental memory indexing** — delta sync for efficient re-indexing.
7. **Config hot-reload** — zero downtime for configuration changes.

---

## 1. Parallel Tool Execution (`tool_executor.go`)

### Mechanism

When the LLM returns multiple tool calls, the ToolExecutor identifies which can run in parallel and which must be sequential.

```
LLM Tool Calls: [read_file, search_files, bash, web_search, glob_files]
                       │                       │
                       ▼                       ▼
            ┌─ Parallel ─────────┐    ┌─ Sequential ──┐
            │ read_file           │    │ bash           │
            │ search_files        │    └────────────────┘
            │ web_search          │
            │ glob_files          │
            └─────────────────────┘
```

### Sequential Tools (state-sharing)

These tools **always** run one at a time, as they share mutable state:

| Tool | Reason for Sequentiality |
|------|--------------------------|
| `bash` | Shared CWD and environment |
| `write_file` | May conflict with simultaneous writes |
| `edit_file` | Same file editing |
| `ssh` | Shared SSH session |
| `scp` | File transfer |
| `exec` | Process execution |
| `set_env` | Modifies global environment |

### Semaphore Configuration

```yaml
security:
  tool_executor:
    parallel: true       # Enable parallel execution
    max_parallel: 5      # Maximum concurrent tools
```

### Implementation

- **Semaphore**: buffered channel of size `max_parallel`.
- **Wait group**: `sync.WaitGroup` waits for all tools to complete.
- **Timeout**: each tool has an individual timeout (default 30s).
- **Error isolation**: failure in one tool does not affect the others.

### Expected Benchmarks

| Scenario | Sequential | Parallel (5) | Speedup |
|----------|-----------|--------------|---------|
| 5x `read_file` (local) | ~5ms | ~1ms | 5x |
| 3x `web_search` + 2x `read_file` | ~3.5s | ~1.2s | ~3x |
| 2x `bash` (forced sequential) | ~2s | ~2s | 1x |

---

## 2. Concurrent Subagents (`subagent.go`)

### Architecture

Each subagent runs in its own goroutine with an isolated context:

```
Main Agent
    ├── spawn_subagent("search docs")    ──▶ goroutine #1 (AgentRun)
    ├── spawn_subagent("analyze code")   ──▶ goroutine #2 (AgentRun)
    └── spawn_subagent("create tests")   ──▶ goroutine #3 (AgentRun)
         │
         ▼
    wait_subagent / poll status
```

### Concurrency Control

```yaml
subagents:
  max_concurrent: 4       # Global semaphore
  max_turns: 15            # Turns per subagent (vs 25 for main)
  timeout_seconds: 300     # 5 min per subagent
```

- **Semaphore**: limits concurrent subagents globally.
- **Cancellable context**: each subagent has `context.WithTimeout`.
- **No recursion**: subagents cannot spawn other subagents.
- **Tool filtering**: deny list removes spawning tools.

### Model Optimization

```yaml
subagents:
  model: "gpt-4o-mini"   # Faster/cheaper model for subagents
```

Subagents can use a different model than the main agent, enabling cost/speed trade-offs per task.

---

## 3. Message Queue (`message_queue.go`)

### Problem

In messaging channels, users often send multiple messages in quick succession ("message burst"). Processing each individually creates unnecessary overhead.

### Solution

```
Message 1 ──▶ ┌──────────┐
Message 2 ──▶ │  Queue   │──debounce 1s──▶ Batch Processing
Message 3 ──▶ │  (dedup) │                 (combined messages)
               └──────────┘
```

### Configuration

```yaml
queue:
  debounce_ms: 1000     # Wait 1s of silence before processing
  max_pending: 20       # Maximum messages in queue
```

### Mechanisms

| Feature | Description | Impact |
|---------|-------------|--------|
| **Debounce** | Waits for 1s of silence between messages | Reduces LLM calls |
| **Deduplication** | Identical messages within 5s window are discarded | Eliminates duplicates |
| **Batching** | Accumulated messages are combined into one | 1 LLM call vs N |
| **Max pending** | Discards messages beyond the limit (20) | Prevents overload |
| **Queuing during processing** | Messages arriving while the agent processes are queued | No loss |

### Throughput Impact

For a burst of 5 messages in 2s:
- **Without queue**: 5 LLM calls (~5x cost, ~15s total)
- **With queue**: 1 combined LLM call (~1x cost, ~3s total)

---

## 4. Block Streaming (`block_streamer.go`)

### Motivation

Long LLM responses can take 10-30s to complete. Without streaming, the user waits without feedback. The Block Streamer delivers progressive blocks.

### How It Works

```
LLM Response (tokens arriving)
    │
    ├──80 chars──▶ Block 1 sent to channel
    ├──idle 1.2s──▶ Block 2 sent
    ├──3000 chars──▶ Block 3 (force flush)
    └──end──▶ Final block (or skip if already delivered)
```

### Configuration

```yaml
block_stream:
  enabled: false        # Disabled by default
  min_chars: 80         # Minimum chars before first block
  idle_ms: 1200         # Idle timeout before flush
  max_chars: 3000       # Force flush at this limit
```

### Smart Splitting

Blocks are split at natural boundaries (in order of preference):
1. Paragraphs (double line break)
2. Sentence endings (`.`, `!`, `?`)
3. List items (`- `, `* `, `1. `)
4. Character limit (force flush)

### Deduplication

The final block is only sent if no partial blocks were already delivered, avoiding duplicate messages.

---

## 5. Prompt Caching

### Anthropic Prompt Caching

For Anthropic and Z.AI Anthropic proxy providers, GoClaw automatically adds `cache_control` to the system message and the second-to-last user message:

```json
{
  "cache_control": {"type": "ephemeral"}
}
```

### Impact

| Metric | Without Cache | With Cache | Savings |
|--------|--------------|------------|---------|
| Prompt token cost | 100% | ~10% | **90%** |
| Latency (TTFT) | ~2s | ~500ms | **75%** |

Caching is particularly effective for:
- Long conversations with the same system prompt.
- Skills with extensive instructions.
- Bootstrap files (SOUL.md, AGENTS.md) that change rarely.

### Model Fallback

On API failure, the client implements fallback with exponential backoff:

```yaml
fallback:
  models: [gpt-4o, glm-4.7-flash, claude-sonnet-4.5]
  max_retries: 2
  initial_backoff_ms: 1000
  max_backoff_ms: 30000
  retry_on_status_codes: [429, 500, 502, 503, 529]
```

Fallback minimizes downtime — if the primary provider fails, secondaries take over automatically.

---

## 6. Incremental Memory Indexing (`memory/sqlite_store.go`)

### Delta Sync

Instead of re-indexing all memory on every startup, the system performs delta sync:

1. Computes SHA-256 of each chunk of each `.md` file.
2. Compares with hashes stored in SQLite.
3. Only re-embeds chunks whose hash changed.
4. Removes chunks from deleted files.

### Impact

| Scenario | Full Index | Delta Sync | Savings |
|----------|-----------|------------|---------|
| 100 files, 2 changed | ~50s + API calls | ~1s + 2 API calls | **98%** |
| 500 chunks, 10 new | ~25 API calls | ~1 API call (batch) | **96%** |

### Hybrid Search

Search combines two strategies in parallel:

```
Query ──▶ ┌─── BM25 (FTS5) ──▶ keyword scores
          │
          └─── Cosine Similarity ──▶ vector scores
                        │
                        ▼
              Reciprocal Rank Fusion (RRF)
                        │
                        ▼
                 Results ranked
```

```yaml
memory:
  search:
    hybrid_weight_vector: 0.7    # Vector search weight
    hybrid_weight_bm25: 0.3      # Keyword search weight
    max_results: 6
    min_score: 0.1
```

### Embedding Cache

Embeddings are cached in SQLite by chunk hash. If the text hasn't changed, the existing embedding is reused — zero unnecessary API calls.

### SQLite Tuning

```go
// WAL mode for concurrent reads
PRAGMA journal_mode=WAL;

// Busy timeout to avoid SQLITE_BUSY
PRAGMA busy_timeout=5000;
```

---

## 7. Config Hot-Reload (`config_watcher.go`)

### Mechanism

```
config.yaml changed
       │
       ▼
  mtime changed? ──No──▶ (skip)
       │
      Yes
       │
       ▼
  SHA-256 changed? ──No──▶ (skip, mtime glitch)
       │
      Yes
       │
       ▼
  Reload hot-reloadable fields
  (no process restart)
```

### Hot-Reloadable Fields

| Field | Restart Required |
|-------|-----------------|
| Access control (users, groups) | No |
| Instructions | No |
| Tool guard rules | No |
| Heartbeat config | No |
| Token budgets | No |
| LLM provider/model | **Yes** |
| Channel config | **Yes** |
| Gateway config | **Yes** |

### Impact

Zero downtime for common operational changes (access, rules, instructions).

---

## 8. Context Compaction

### Preventive Compaction

Triggers automatically at **80%** of the `max_messages` threshold, not 100%. This prevents overflow during message processing.

```
max_messages = 100
trigger = 80 messages (80%)

[70 msgs] ─ normal
[80 msgs] ─ compaction triggered automatically
[100 msgs] ─ overflow (prevented by preventive compaction)
```

### Strategies and Trade-offs

| Strategy | Latency | LLM Cost | Context Quality |
|----------|---------|----------|----------------|
| `summarize` | ~5-10s | High (1 extra call) | Excellent — semantic context preserved |
| `truncate` | <1ms | Zero | Good — loses old context |
| `sliding` | <1ms | Zero | Basic — fixed window |

### Context Overflow Recovery

If the LLM returns `context_length_exceeded`:

1. Compacts messages (keeps system + recent history).
2. Truncates tool results to 2000 chars.
3. Retries up to `max_compaction_attempts` (default 3).

---

## 9. Session Management

### Thread Safety

Each session has a `sync.RWMutex`:
- **Read lock**: read operations (history, facts) can be concurrent.
- **Write lock**: write operations (append, compaction) are exclusive.

### File Locks

Disk persistence uses file locks to prevent corruption in crash/restart scenarios.

### JSONL Persistence

JSONL format (one entry per line) for:
- **Efficient appending**: new messages are appended without rewriting the entire file.
- **Partial recovery**: in case of corruption, valid lines are preserved.
- **Predictable size**: each entry is independent.

---

## 10. SSE Streaming (`llm.go`)

### Implementation

The LLM Client supports Server-Sent Events for response streaming:

```
Client ──POST /v1/chat/completions──▶ Server
         (stream: true)

Server ──data: {"choices":[...]}──▶ Client
         data: {"choices":[...]}──▶ Client
         data: {"choices":[...]}──▶ Client
         data: [DONE]──▶ Client
```

### Fallback

If the provider doesn't support streaming, the client automatically falls back to a synchronous request.

### Performance Benefits

| Metric | Without Streaming | With Streaming |
|--------|------------------|----------------|
| Time to First Token (TTFT) | 3-15s | 0.5-2s |
| Perceived latency | High | Low |
| User experience | Wait → full response | See tokens arriving |

---

## Tuning Guide

### High Throughput (Server)

```yaml
security:
  tool_executor:
    parallel: true
    max_parallel: 8        # More concurrent tools

subagents:
  max_concurrent: 6        # More subagents

queue:
  debounce_ms: 500         # Shorter debounce for faster response
  max_pending: 50          # Larger queue

block_stream:
  enabled: true
  idle_ms: 800             # Faster flush

memory:
  embedding:
    batch_size: 50         # Larger embedding batches
```

### Low Cost (Personal Use)

```yaml
security:
  tool_executor:
    parallel: true
    max_parallel: 3

subagents:
  max_concurrent: 2
  model: "glm-4.7-flash"  # Cheap model for subagents

memory:
  compression_strategy: truncate   # No summarization cost
  session_memory:
    enabled: false

block_stream:
  enabled: false
```

### Low Latency (Real-time)

```yaml
agent:
  max_turns: 10            # Limit turns for fast response
  turn_timeout_seconds: 60

security:
  tool_executor:
    parallel: true
    max_parallel: 10

queue:
  debounce_ms: 300

block_stream:
  enabled: true
  min_chars: 40
  idle_ms: 600
```
