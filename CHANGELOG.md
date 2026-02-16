# Changelog

All notable changes to GoClaw are documented in this file.

## [1.4.0] — 2026-02-16

Performance improvements, new concurrency architecture, advanced agent capabilities, comprehensive security hardening, and new tools.

### Performance

- **Adaptive debounce**: reduced default from 1000ms to 200ms; followup debounce at 500ms; idle sessions drain immediately
- **Unified send+stream endpoint**: new `POST /api/chat/{sessionId}/stream` combines send and stream in a single HTTP request, eliminating a round-trip
- **Asynchronous media enrichment**: vision/transcription runs in background while the agent starts responding; results are injected via interrupt channel
- **Background compaction**: `maybeCompactSession` now runs in a goroutine, no longer blocking the response path
- **Faster block streaming**: `MinChars` reduced from 50→20, `IdleMs` from 400→200ms; eliminated double idle check for immediate flushing
- **Lazy prompt composition**: memory and skills layers cached (60s TTL) with background refresh; critical layers (bootstrap, history) still loaded synchronously

### Architecture

- **Lane-based concurrency** (`lanes.go`): configurable lanes (session, cron, subagent) with per-lane queue and concurrency limits — prevents work-type contention
- **Event bus** (`events.go`): in-memory pub/sub for agent lifecycle events (delta, tool_use, done, error) — supports multi-consumer streams
- **WebSocket JSON-RPC** (`gateway/websocket.go`): bidirectional real-time communication as alternative to HTTP/SSE; supports client requests (chat.abort) and server events
- **Fast abort** (`tool_executor.go`): abort channel with `ResetAbort`/`Abort`/`IsAborted` — tools detect and respond to external abort signals during execution

### Agent Capabilities

- **Queue modes** (`queue_modes.go`): configurable strategies for incoming messages when session is busy — collect, steer, followup, interrupt, steer-backlog
- **Model failover** (`model_failover.go`): automatic LLM failover with reason classification (billing, rate limit, auth, timeout, format) and per-model cooldowns
- **Proactive prompts** (`prompt_layers.go`): reply tags, silent reply tokens, heartbeat guidance, reasoning format, memory recall, subagent orchestration, and messaging directives
- **Agent steering**: interrupt during tool execution; tools check `interruptCh` for incoming messages to steer behavior mid-run
- **Context pruning** (`agent.go`): proactive soft/hard trim of old tool results based on turn age — prevents context bloat without waiting for LLM overflow
- **Memory flush pre-compaction**: explicit "pre-compaction memory flush turn" saves durable memories to disk before compaction using append-only strategy
- **Expanded directives**: new `/verbose`, `/reasoning` (alias for `/think`), `/queue`, `/usage`, `/activation` commands with thread-safe config access

### Security

- **Workspace containment** (`workspace_containment.go`): sandbox path validation and symlink escape protection for all file operations
- **Memory injection hardening** (`memory_hardening.go`): treats memories as untrusted data; escapes HTML entities, strips dangerous tags, wraps in `<relevant-memories>` tags, detects injection patterns
- **Request body limiter**: 2MB limit on `POST /v1/chat/completions` to prevent OOM from oversized payloads
- **Partial output on abort**: `handleChatAbort` now includes a `partial` flag indicating preserved partial output before cancellation

### New Tools & Features

- **Browser automation** (`browser_tool.go`): Chrome DevTools Protocol (CDP) integration with `browser_navigate`, `browser_screenshot`, `browser_content`, `browser_click`, `browser_fill`
- **Interactive canvas** (`canvas_host.go`): HTML/JS canvas with live-reload via temporary HTTP server; `canvas_create`, `canvas_update`, `canvas_list`, `canvas_stop`
- **Session management tools**: `sessions_list` (all workspaces), `sessions_send` (inter-agent messaging), `sessions_delete`, `sessions_export` (full history + metadata as JSON)
- **Lifecycle hooks** (`hooks.go`): 16+ event types (SessionStart, PreToolUse, AgentStop, etc.) with priority-ordered dispatch, sync blocking, async non-blocking, and panic recovery
- **Group chat** (`group_chat.go`): activation modes, intro messages, context injection, participant tracking, quiet hours, ignore patterns
- **Tailscale integration** (`tailscale.go`): Tailscale Serve/Funnel for secure remote access without manual port forwarding or DNS configuration
- **Advanced cron** (`scheduler.go`): isolated sessions per job, announce handler for broadcasting results, subagent spawn option, per-job timeouts, job labels

### Session Management

- **Structured session keys**: `SessionKey` struct (Channel, ChatID, Branch) enables multi-agent routing
- **Session CRUD**: `DeleteByID`, `Export`, `RenameSession` on SessionStore
- **Bounded followup queue**: FIFO eviction at 20 items maximum

### Bug Fixes

- Fixed data race in `ListSessions` when reading `lastActiveAt`/`CreatedAt` without lock
- Fixed `splitMax` panic on empty separator
- Fixed restored sessions not persisting (missing `persistence` field)
- Fixed duplicate hook event registration in `HookManager.Register`
- Fixed panic in hook dispatch — individual handler panics no longer crash dispatch goroutine
- Fixed race condition in canvas `Update` — hold mutex during channel send to prevent panic from concurrent `Stop`
- Fixed infinite loop in `findFreePort` when `MaxCanvases` exceeds available port range
- Fixed `tailscale.go` hostname parsing — replaced manual JSON parsing with `json.Unmarshal`
- Fixed data race on `Job.LastRunAt`/`RunCount`/`LastError` in scheduler `executeJob`
- Fixed scheduler persisting removed jobs — added existence check before `Save`
- Fixed `commands.go` reading config without mutex — added `configMu.RLock()` to `/queue` and `/activation`

### Frontend (WebUI)

- New `createPOSTSSEConnection` for POST-based SSE in unified send+stream flow
- Updated `useChat` hook to eliminate separate `api.chat.send` + `createSSEConnection` calls
- Added `run_start` event handler for unified stream protocol

### Dependencies

- Added `github.com/gorilla/websocket` (WebSocket support)
- Added `github.com/robfig/cron/v3` (advanced cron scheduling)

---

## [1.1.0] — 2026-02-12

### Performance

- Increased default agent turn timeout from 90s to 300s to handle slow model cold starts (e.g. GLM-5 ~30-60s first turn)
- Added transient error retry (1x, 2.5s delay) for streaming LLM calls before falling back to non-streaming
- Bootstrap file loading now uses in-memory SHA-256 cache to avoid redundant disk reads

### Progressive Message Delivery (Block Streaming)

- New `BlockStreamer` module: accumulates LLM output tokens and sends them progressively to channels (WhatsApp) as partial messages
- Configurable: `block_stream.enabled`, `block_stream.min_chars` (default: 80), `block_stream.idle_ms` (default: 1200), `block_stream.max_chars` (default: 3000)
- Natural break detection: splits at paragraph boundaries, sentence endings, or list items
- Avoids duplicate final messages when blocks have already been sent

### Advanced Memory System

- **SQLite-backed memory store** with FTS5 (BM25 ranking) and in-process vector search (cosine similarity)
- **Embedding provider**: OpenAI `text-embedding-3-small` (1536 dims) with SQLite-backed embedding cache to reduce API calls
- **Markdown chunker**: intelligent splitting by headings, paragraphs, and sentences with configurable overlap and max tokens
- **Delta sync**: hash-based change detection — only re-embeds chunks that actually changed
- **Hybrid search** (RRF): combines vector similarity and BM25 keyword scores with configurable weights (default: 0.7 vector / 0.3 BM25)
- **Session memory hook**: on `/new` command, summarizes the conversation via LLM and saves to `memory/YYYY-MM-DD-slug.md`
- New `memory_index` tool for manual re-indexing of memory files
- `memory_search` upgraded to use hybrid search when SQLite store is available (falls back to substring search)
- Configurable via `memory.embedding`, `memory.search`, `memory.index`, `memory.session_memory` in config.yaml

### Bootstrap Improvements

- `BOOT.md` support: agent executes instructions from `BOOT.md` after startup (proactive initialization)
- `BootstrapMaxChars` limit prevents oversized bootstrap from consuming the token budget
- Subagent-specific filtering: subagents only load `AGENTS.md` and `TOOLS.md` (not the full bootstrap set)

### Session Persistence

- `SessionPersistence` properly wired to session store — conversations now survive restarts
- JSONL history, facts, and metadata are loaded on startup and saved on changes

### Bug Fixes

- Fixed race condition in `/new` command where session history could be cleared before the summary goroutine reads it (now captures a snapshot first)
- Fixed hybrid search merge key collision: different chunks from the same file that share a prefix are no longer collapsed
- Fixed FTS5 query injection: user input is now sanitized and wrapped in double quotes for phrase matching
- Fixed potential SQL row leak in `IndexChunks` error path

### New Config Options

```yaml
block_stream:
  enabled: false
  min_chars: 80
  idle_ms: 1200
  max_chars: 3000

memory:
  embedding:
    provider: openai
    model: text-embedding-3-small
    dimensions: 1536
    batch_size: 20
  search:
    hybrid_weight_vector: 0.7
    hybrid_weight_bm25: 0.3
    max_results: 6
    min_score: 0.1
  index:
    auto: true
    chunk_max_tokens: 500
  session_memory:
    enabled: false
    messages: 15
```

### Dependencies

- Added `github.com/mattn/go-sqlite3` (SQLite driver with FTS5 support)

## [1.0.0] — 2026-02-12

First stable release.

### Core

- Agent loop with multi-turn tool calling, auto-continue (up to 2 continuations), and reflection nudges every 8 turns
- 8-layer prompt composer (Core, Safety, Identity, Thinking, Bootstrap, Business, Skills, Memory, Temporal, Conversation, Runtime) with priority-based token budget trimming
- Session isolation per chat/group with JSONL persistence and auto-pruning
- Three compression strategies for session compaction: `summarize` (LLM), `truncate`, `sliding` — preventive trigger at 80% capacity
- Token-aware sliding window for conversation history (backwards construction, per-message truncation)
- Subagent system: spawn, track, wait, stop child agents with filtered tool sets
- Message queue with per-session debounce (configurable ms), deduplication (5s window), and burst handling
- Config hot-reload via file watcher (mtime + SHA256 hash)
- Token/cost tracking per session and global with model-specific pricing

### LLM Client

- OpenAI-compatible HTTP client with provider auto-detection
- Providers: OpenAI, Z.AI (API, Coding, Anthropic proxy), Anthropic, any OpenAI-compatible
- Model-specific defaults for temperature and max_tokens (GPT-5, Claude Opus 4.6, GLM-5, etc.)
- Model fallback chain with exponential backoff, `Retry-After` header support, and error classification
- SSE streaming with `[DONE]` terminator handling
- Prompt caching (`cache_control: ephemeral`) for Anthropic and Z.AI Anthropic proxy
- Context overflow handling: auto-compaction, tool result truncation, retry

### Security

- Encrypted vault (AES-256-GCM + Argon2id key derivation: 64 MB, 3 iterations, 4 threads)
- Secret resolution chain: vault → OS keyring → env vars → .env → config.yaml
- Tool Guard: per-tool role permissions (owner/admin/user), dangerous command regex blocking, protected paths, SSH host allowlist, audit logging
- Interactive execution approval via chat (`/approve`, `/deny`)
- SSRF protection for `web_fetch`: blocks private IPs, loopback, link-local, cloud metadata
- Script sandbox: none / Linux namespaces (PID, mount, net, user) / Docker container
- Pre-execution content scanning: eval, reverse shells, crypto mining, shell injection, obfuscation
- Input guardrails: rate limiting (sliding window), prompt injection detection, max input length
- Output guardrails: system prompt leak detection, empty response fallback

### Tools (35+)

- File I/O: `read_file`, `write_file`, `edit_file`, `list_files`, `search_files`, `glob_files` — full filesystem access
- Shell: `bash` (persistent cwd/env), `set_env`, `ssh`, `scp`
- Web: `web_search` (DuckDuckGo HTML parsing), `web_fetch` (SSRF-protected)
- Memory: `memory_save`, `memory_search`, `memory_list`
- Scheduler: `schedule_add`, `schedule_list`, `schedule_remove`
- Media: `describe_image` (LLM vision), `transcribe_audio` (Whisper API)
- Skills: `init_skill`, `edit_skill`, `add_script`, `list_skills`, `test_skill`, `install_skill`, `search_skills`, `remove_skill`
- Subagents: `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent`
- Parallel tool execution with configurable semaphore (max 5 concurrent)

### Channels

- WhatsApp (native Go via whatsmeow): text, images, audio, video, documents, stickers, voice notes, locations, contacts, reactions, reply/quoting, typing indicators, read receipts, group messages
- Automatic media enrichment: vision (image description) and audio transcription (Whisper) on incoming media
- WhatsApp markdown formatting (bold, italic, strikethrough, code, code blocks)
- Message splitting for long responses (preserves code blocks, prefers paragraph/sentence boundaries)
- Plugin loader for additional channels (Go native `.so`)

### Access Control & Workspaces

- Per-user/group allowlist and blocklist with deny-by-default policy
- Roles: owner > admin > user > blocked
- Chat commands: `/allow`, `/block`, `/admin`, `/users`, `/group allow`
- Multi-tenant workspaces with isolated system prompts, skills, models, languages, and memory
- Workspace management via chat: `/ws create`, `/ws assign`, `/ws list`

### Skills

- Native Go skills (compiled, direct execution) and SKILL.md format (ClawHub compatible)
- Skill installation from ClawHub, GitHub, HTTP URLs, and local paths
- Built-in skills: weather, calculator, web-search, web-fetch, summarize, github, gog, calendar
- Skill creation via chat (agent can author its own skills)
- Hot-reload on install/remove

### HTTP API Gateway

- OpenAI-compatible `POST /v1/chat/completions` with SSE streaming
- Session management: `GET/DELETE /api/sessions`
- Usage tracking: `GET /api/usage`
- System status: `GET /api/status`
- Webhook registration: `POST /api/webhooks`
- Bearer token authentication and CORS

### CLI

- Interactive setup wizard with arrow-key navigation (`charmbracelet/huh`), model auto-detection, vault creation
- CLI chat REPL with `readline` support: arrow-key history, reverse search (Ctrl+R), tab completion, persistent history
- Commands: `setup`, `serve`, `chat`, `config` (init, show, validate, vault-*, set-key, key-status), `skill` (list, search, install, create), `schedule` (list, add), `health`
- Chat commands: `/help`, `/model`, `/usage`, `/compact`, `/think`, `/new`, `/reset`, `/stop`, `/approve`, `/deny`

### Bootstrap System

- Template files in `configs/bootstrap/`: SOUL.md, AGENTS.md, IDENTITY.md, USER.md, TOOLS.md, HEARTBEAT.md
- Loaded at runtime into the prompt layer system
- Agent can read and update its own bootstrap files

### Scheduler & Heartbeat

- Cron-based task scheduler with file persistence
- Heartbeat: proactive agent behavior on configurable interval with active hours
- Agent reads HEARTBEAT.md for pending tasks

### Deployment

- Single binary, zero runtime dependencies
- Docker and Docker Compose support
- systemd service unit with hardening (ProtectSystem, PrivateTmp, MemoryMax)
- Makefile: build, run, setup, chat, test, lint, clean, docker-build, docker-up
