# Changelog

All notable changes to DevClaw are documented in this file.

## [1.8.1] - 2025-02-22

### Fixed

- **Provider-specific API keys**: Changed from generic `DEVCLAW_API_KEY` to standard names (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, etc.)
- **Tool schema validation**: Fixed `team_list` tool missing `properties` field causing OpenAI API 400 errors
- **Setup wizard**: Now saves API keys with correct provider-specific names in vault
- **Vault injection**: All secrets are now injected as environment variables with their original names

### Changed

- **LLM client**: `resolveAPIKey()` now looks for provider-specific env vars first, then generic fallback
- **Key resolution priority**: Config key → Provider env var → Generic `API_KEY`
- `DEVCLAW_API_KEY` is now reserved for DevClaw gateway authentication (not LLM providers)

### Security

- Removed hardcoded API keys from example configs
- Vault now injects all secrets as env vars at runtime (no plaintext in config files)

## [Unreleased]

### WebUI Configuration Pages — Complete Settings UI

Full configuration management via WebUI with 7 new pages covering 100% of YAML settings:

- **API Configuration Page** (`/config`): LLM provider selection, API key management, base URL config, connection testing
- **Access Control Page** (`/access`): Owner/admin/user management, allowlist/blocklist, default policy settings
- **Budget Page** (`/budget`): Monthly spending limits, warning thresholds, usage visualization
- **Memory Page** (`/memory`): Embedding settings, search configuration, compression strategies
- **Database Hub Page** (`/database`): Backend selection (SQLite/PostgreSQL), connection status, pool metrics, vector search config
- **Groups Page** (`/groups`): Activation modes, intro messages, quiet hours, participant limits
- **MCP Servers Page** (`/mcp`): Server CRUD, start/stop controls, status monitoring

**New REST Endpoints:**
- `GET/PUT /api/config` - General configuration
- `GET/PUT /api/access` - Access control settings
- `GET/PUT /api/config/budget` - Budget configuration
- `GET/PUT /api/config/memory` - Memory configuration
- `GET/PUT /api/config/groups` - Groups configuration
- `GET /api/database/status` - Database health and pool metrics
- `GET/POST /api/mcp/servers` - List/create MCP servers
- `GET/PUT/DELETE /api/mcp/servers/{name}` - Manage specific server
- `POST /api/mcp/servers/{name}/start` - Start MCP server
- `POST /api/mcp/servers/{name}/stop` - Stop MCP server

**Reusable UI Components:**
- `ConfigPage`, `ConfigSection`, `ConfigField` - Layout components
- `ConfigInput`, `ConfigSelect`, `ConfigToggle`, `ConfigTextarea` - Form controls
- `ConfigCard`, `ConfigActions`, `ConfigTagList` - Interactive elements
- `LoadingSpinner`, `ErrorState`, `ConfigEmptyState`, `ConfigInfoBox` - State components

**Testing:**
- 49 frontend unit tests (Vitest + React Testing Library)
- All configuration pages tested for rendering, interactions, and API calls

**Internationalization:**
- Full i18n support with English and Portuguese translations
- All new pages support language switching

### Database Hub — Multi-Backend Support

Complete database abstraction layer with SQLite (default) and PostgreSQL/Supabase support:

- **Multi-Backend Architecture**: Unified interface for SQLite, PostgreSQL, and future backends (MySQL)
- **Connection Pool Management**: Full pool metrics (open/in-use/idle connections, wait count/duration)
- **Vector Search**: In-memory for SQLite, pgvector for PostgreSQL with HNSW/IVFFlat indexes
- **Schema Migrations**: Automatic schema creation and version tracking for all backends
- **Health Monitoring**: Detailed health status with pool metrics and latency tracking

**Security Improvements:**
- SQL injection prevention in `db_hub_schema` (table name validation)
- Path traversal prevention in `db_hub_backup` (path sanitization)
- Rate limiting in `db_hub_raw` (10 ops/sec per session)

**New Agent Tools:**
- `db_hub_status`: Health status with pool metrics
- `db_hub_query`: Execute SELECT queries (validated)
- `db_hub_execute`: Execute INSERT/UPDATE/DELETE
- `db_hub_schema`: View schema/tables (SQL injection protected)
- `db_hub_migrate`: Run schema migrations
- `db_hub_backup`: Create SQLite backups (path traversal protected)
- `db_hub_backends`: List available backends
- `db_hub_raw`: Execute raw SQL (rate-limited)

**Configuration:**
```yaml
database:
  hub:
    backend: "sqlite"  # sqlite | postgresql
    sqlite:
      path: "./data/devclaw.db"
      journal_mode: "WAL"
    postgresql:
      host: "localhost"
      port: 5432
      vector:
        enabled: true
        dimensions: 1536
        index_type: "hnsw"
```

**Testing:**
- Unit tests for SQLite backend (62% coverage)
- Integration tests for PostgreSQL (run with `-tags=integration`)
- Rate limiter tests with concurrent access

### Native Media Handling

Complete native media system for receiving, processing, and sending images, audio, and documents across all channels:

- **MediaService**: Channel-agnostic media handling with upload, enrichment, and send capabilities
- **MediaStore**: File-based storage with automatic cleanup of temporary files
- **Validator**: MIME type detection using magic bytes + extension heuristics
- **WebUI API**: REST endpoints for media upload/download at `/api/media`

**New Media Tools:**
- `send_image`: Send images to users via media_id, file_path, or URL
- `send_audio`: Send audio files to users
- `send_document`: Send documents (PDF, DOCX, TXT) to users

**Enrichment Features:**
- **Images**: Auto-description via Vision API (configurable model with fallback)
- **Audio**: Auto-transcription via Whisper API (configurable model with fallback)
- **Documents**: Text extraction from PDF (pdftotext), DOCX (unzip), and plain text

**Configuration:**
```yaml
native_media:
  enabled: true
  store:
    base_dir: "./data/media"
    temp_dir: "./data/media/temp"
  service:
    max_image_size: 20MB
    max_audio_size: 25MB
    max_doc_size: 50MB
  enrichment:
    auto_enrich_images: true     # requires media.vision_enabled
    auto_enrich_audio: true      # requires media.transcription_enabled
    auto_enrich_documents: true  # works independently
```

**Model Selection:**
- Vision uses `media.vision_model` (falls back to main model)
- Transcription uses `media.transcription_model`
- Set specific models for better quality or cost optimization

### Teams System — Persistent Agents & Shared Memory

Complete team coordination system with persistent agents, shared memory, and real-time collaboration:

- **Persistent Agents**: Long-lived agents with specific roles, personalities, and instructions
- **Team Memory**: Shared state accessible by all team members (tasks, messages, facts, documents)
- **Thread Subscriptions**: Auto-subscribe to threads for continuous notifications
- **Working State (WORKING.md)**: Persist work-in-progress across heartbeats
- **Active Notification Push**: Trigger agents immediately on @mentions
- **Documents Storage**: Store deliverables linked to tasks with versioning

**New Team Tools:**
- Team management: `team_create`, `team_list`, `team_create_agent`, `team_list_agents`, `team_stop_agent`, `team_delete_agent`
- Tasks: `team_create_task`, `team_list_tasks`, `team_update_task`, `team_assign_task`
- Communication: `team_comment`, `team_check_mentions`, `team_send_message`, `team_mention`
- Memory: `team_save_fact`, `team_get_facts`, `team_delete_fact`, `team_standup`
- Documents: `team_create_document`, `team_list_documents`, `team_get_document`, `team_update_document`, `team_delete_document`
- Working State: `team_get_working`, `team_update_working`, `team_clear_working`

### LLM Improvements

- **Gemini model ID normalization**: Automatic conversion of Gemini model IDs (e.g., `gemini-2.0-flash` → `gemini-2.0-flash-001`) for compatibility with Google AI API
- **Enhanced subagent spawning controls**: Better control over which models can spawn subagents

### Agent Improvements

- **Tool filtering by profile**: Filter available tools based on agent profile configuration
- **Tool count limits**: Configurable limits on number of tools exposed to agent

### Background Routines

- **Metrics Collector**: Periodic system metrics collection with webhook support
  - Message and token counts with per-minute rates
  - Agent runs (total, active, success, failed, timeout)
  - Tool calls and subagent statistics
  - System metrics (goroutines, memory, uptime)
  - Latency tracking with P50/P99 percentiles
  - Subscriber pattern for real-time metrics delivery

- **Memory Indexer**: Incremental indexing of memory files for enhanced search
  - SHA-256 hash-based change detection
  - Automatic detection of deleted files
  - Configurable interval and memory directory

- **Internal Routines Documentation**: Complete documentation of all 26+ background routines in `docs/internal-routines.md`

---

## [1.7.0] — 2026-02-18

WhatsApp UX overhaul, setup improvements, tool profiles, native media, and comprehensive documentation.

### WhatsApp & Session Management

- **Enhanced session management**: Improved WhatsApp connection handling and session persistence
- **Better connection management**: Robust event handling for WhatsApp connections
- **Session persistence**: Workspace session stores now survive container restarts

### Setup & Configuration

- **New setup components**: Streamlined setup wizard with improved UX
- **Tool profiles**: Simplified permission management with predefined profiles (admin, developer, readonly, etc.)
- **System administration commands**: New `/maintenance` and system commands for admin control
- **Maintenance mode**: Enable maintenance mode to block non-admin access during updates

### Media & Vision

- **Native vision and transcription**: Vision and transcription are now native tools with auto-detection
- **Vision model selector**: UI for selecting vision model in settings
- **Synchronous media enrichment**: All media enrichment runs synchronously for better reliability
- **Audio transcription**: End-to-end audio transcription working

### Documentation

- **Agent architecture review**: Comprehensive documentation of agent routing, DM pairing, environment variables
- **Hooks system documentation**: Full documentation of lifecycle hooks
- **Group policy documentation**: Group chat policy configuration

### UI Improvements

- **Enhanced dark mode**: Better dark mode styling across all components
- **Chat message UI**: Improved chat message display and localization
- **Language switcher**: Multilingual support with language selection

### Docker & Deployment

- **Restructured Docker setup**: Simplified Dockerfile and docker-compose for easier deployment
- **Runtime tools in Docker**: All necessary tools included in Docker image
- **Workspace bind mount**: Workspace directory bind-mounted into container

---

## [1.6.1] — 2026-02-17

Starter pack system and UI fixes.

### Skills

- **Starter pack system**: Skills can now teach the LLM how to use native tools via embedded instructions

### UI Fixes

- Fixed vault nomenclature in security page
- Fixed toggle bug using canonical Tailwind classes
- Fixed PT-BR accents, gateway port 8085
- More concise vault texts in setup wizard

### Docker

- **Named volume for state**: State now persists between container restarts via named volume
- **Simplified Dockerfile**: Cleaner Dockerfile and compose for VM deployment
- Removed .env from tracking, conditional phone, improved Channels

---

## [1.6.0] — 2026-02-17

Major release with 20+ native tools, MCP server, plugin system, and redesigned UI.

### Native Tools (20+ New Tools)

- **Git tools**: status, diff, log, commit, branch, stash, blame with structured JSON output
- **Docker tools**: ps, logs, exec, images, compose (up/down/ps/logs), stop, rm
- **Database tools**: query, execute, schema, connections (PostgreSQL, MySQL, SQLite)
- **Dev utility tools**: json_format, jwt_decode, regex_test, base64, hash, uuid, url_parse, timestamp
- **System tools**: env_info, port_scan, process_list
- **Codebase tools**: index (file tree), code_search (ripgrep), symbols, cursor_rules_generate
- **Testing tools**: test_run (auto-detect framework), api_test, test_coverage
- **Ops tools**: server_health (HTTP/TCP/DNS), deploy_run, tunnel_manage, ssh_exec
- **Product tools**: sprint_report, dora_metrics, project_summary
- **Daemon manager**: start_daemon, daemon_logs, daemon_list, daemon_stop, daemon_restart

### MCP Server

- **Model Context Protocol**: Full MCP server implementation for IDE integration
- **stdio and SSE transports**: Support for both transport modes
- Works with Cursor, VSCode, Claude Code, OpenCode, Windsurf, Zed, Neovim

### Plugin System

- **Extensible plugins**: GitHub, Jira, Sentry integrations
- **Webhook dispatcher**: Event-driven webhook support

### CLI Enhancements

- **Pipe mode**: `git diff | devclaw diff` or `npm build 2>&1 | devclaw fix`
- **Quick commands**: `devclaw fix`, `devclaw explain .`, `devclaw commit`, `devclaw how "task"`
- **Shell hook**: Auto-capture failed commands with `devclaw shell-hook bash`

### Multi-User System

- **RBAC**: Role-based access control with owner/admin/user roles
- **IDE extension configuration**: Configure IDE extensions per user

### N-Provider Fallback

- **Budget tracking**: Configurable cost limits and alerts
- **Fallback chain**: Automatic model fallback with per-model cooldowns

### UI Redesign

- **v1.6.0 redesign**: Clean, professional look
- **Hooks management**: Lifecycle hooks via UI
- **Webhooks management**: Webhook configuration via UI
- **Domain/network settings**: Configuration page for domain and network

### Project Rename

- Renamed from GoClaw to DevClaw across entire project

---

## [1.5.1] — 2026-02-16

Agent loop safety improvements ported from OpenClaw: tool loop detection, skills token budget guard, heartbeat transcript pruning, compaction retry with backoff, and cron spin loop fix.

### Agent Loop Safety

- **Tool loop detection**: New `ToolLoopDetector` module tracks tool call history with a ring buffer and detects two patterns — **repeat** (same tool+args N times) and **ping-pong** (A→B→A→B). Three severity levels: warning (8x, injects hint), critical (15x, strong nudge), circuit breaker (25x, terminates run). Fully configurable via `config.yaml` under `agent.tool_loop`
- **Per-run detector isolation**: Each agent run creates its own `ToolLoopDetector` instance to avoid cross-session race conditions when multiple users interact concurrently
- **Valid message ordering**: Loop warnings are injected AFTER tool results (assistant→tool→user sequence) to avoid API rejections from providers that validate message order

### Prompt & Memory Optimization

- **Skills prompt bloat guard**: Skills layer now enforces a ~4000 token budget. When total skills text exceeds the budget, largest skills are truncated first (minimum 200 chars preserved). Prevents verbose skill prompts from consuming the entire context window
- **Heartbeat transcript pruning**: No-op heartbeat turns (HEARTBEAT_OK, NO_REPLY, empty) are no longer saved to session history. Only actionable heartbeat responses are persisted, preventing transcript bloat over time

### Reliability

- **Compaction retry with exponential backoff**: The `compactSummarize` LLM call now retries up to 3 times with backoff (2s→4s→8s) on transient errors (rate-limits, timeouts). Properly exits retry loop on context cancellation. Falls back to static summary only after all retries are exhausted
- **Cron spin loop fix**: New `minJobInterval` (2s) guard in scheduler's `executeJob` prevents rapid re-execution when cron fires at the exact same second boundary. Skips silently with debug log when a job ran too recently

### Testing

- **Tool loop detection tests**: 12 test cases covering warning/critical/breaker thresholds, ping-pong detection, reset behavior, disabled mode, ring buffer limits, threshold normalization, and hash determinism
- **Scheduler spin loop tests**: 3 test cases covering spin loop guard, duplicate execution guard, and minJobInterval value assertion
- All tests pass with `-race` detector

---

## [1.5.0] — 2026-02-16

Media processing, document enrichment, WhatsApp UX overhaul, comprehensive security hardening, agent intelligence improvements, and full unit test coverage.

### Media & Document Processing

- **Document parsing**: Automatic text extraction from PDF (`pdftotext`), DOCX (`unzip` + XML strip), and 30+ plain text formats (code files, CSV, JSON, YAML, Markdown, etc.)
- **Video enrichment**: First-frame extraction via `ffmpeg` + Vision API description — agent sees `[Video: description]` in context
- **Auto-send generated images**: New `onToolResult` hook on `AgentRun` detects `generate_image` tool output and sends the image file directly to the user's channel as media — no more "image saved to /tmp/..." text responses
- **Async media pipeline**: Document and video enrichment wired into the existing background media processing with placeholder → enriched content flow
- **System prompt documentation**: Agent is informed of media capabilities and how to install system dependencies (`poppler-utils`, `ffmpeg`, `unzip`)

### WhatsApp UX

- **Message duplication fix**: Steer mode now injects OR enqueues (never both) — each message processed exactly once; eliminates duplicate and triple responses to `/stop`
- **Message fragmentation fix**: BlockStreamer params tuned — MinChars 20→200, MaxChars 600→1500, IdleMs 200→1500 — produces coherent paragraphs instead of 4-word fragments
- **Progress flood eliminated**: `ProgressSender` now has per-channel cooldown (60s WhatsApp, 10s WebUI); removed duplicate heartbeats from tool_executor and claude-code skill
- **Queue modes operational**: All 5 modes (collect, steer, followup, interrupt, steer-backlog) fully implemented; removed hardcoded "Recebi sua mensagem..." canned response; default changed from collect→steer

### Agent Intelligence

- **Subagent delegation rewrite**: Complete rewrite of subagent section in system prompt with detailed workflow, concrete examples, and clear rules for when to delegate
- **Parallel task handling**: New "Handling New Messages During Work" prompt section — agent uses `spawn_subagent` for parallel tasks and recognizes follow-ups vs new requests
- **Media awareness**: System prompt now documents all media types the agent can receive and process (images, audio, documents, video)

### Security Hardening

- **Output sanitization**: Strip `[[reply_to_*]]`, `<final>`, `<thinking>`, `NO_REPLY`, `HEARTBEAT_OK` from all outgoing messages via `StripInternalTags` in `FormatForChannel`
- **Empty message guards**: Prevent sending blank messages after tag stripping in `sendReply`, `BlockStreamer`, and `ProgressSender`
- **File permissions hardened**: Session transcripts/facts/meta files 0644→0600 (owner-only); sessions directory 0755→0700
- **Config save safety**: YAML validation before writing + `.bak` backup creation
- **TTS duplicate prevention**: Skip TTS synthesis for `NO_REPLY`/`HEARTBEAT_OK` silent tokens
- **Centralized constants**: `TokenNoReply` and `TokenHeartbeatOK` exported as constants

### LLM Reliability

- **Rate-limit auto-recovery**: Per-model cooldown tracking on 429 responses (duration from `Retry-After` header, min 60s, max 10min)
- **Smart fallback**: Rate-limited models skipped in fallback loop; periodic probe near cooldown expiry (within 10s, throttled at 30s between probes) for automatic recovery without restart
- **Cron reliability**: `LastRunAt` persisted before job execution (not after) to prevent duplicate fires on crash recovery

### Setup & Configuration

- **Vault in setup wizard**: Dedicated vault password field in StepSecurity with toggle for separate vault vs WebUI password
- **Owner phone in setup**: Setup wizard collects owner phone number, formats as WhatsApp JID — owner gets full tool access (bash, exec, write_file, etc.)
- **Skill installation from WebUI**: New endpoints `GET /api/skills/available`, `POST /api/skills/install`, `POST /api/skills/{name}/toggle`; modal with search and categories in the Skills page

### Testing

- **259 test cases across 13 files** covering 3 phases:
  - *Pure logic*: markdown formatting, message splitting, queue modes, config loading, memory hardening, session keys
  - *Security*: SSRF protection, input/output guardrails, rate limiting, vault encryption, keyring injection
  - *Stateful components*: tool guard permissions, access control, skill registry
- **Makefile integration**: `make test` and `make test-v` targets with `-race` detector and `sqlite_fts5` tag
- All tests run with `t.Parallel()` where safe, table-driven patterns, `t.TempDir()` for isolation

### Dependencies

- System: `poppler-utils` (PDF), `ffmpeg` (video), `unzip` (DOCX) — optional, graceful fallback when missing

---

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
