# DevClaw — Features and Capabilities

Detailed documentation of all features available in DevClaw.

---

## Autonomous Agent Loop

The core of DevClaw is an agentic loop that allows the assistant to execute complex tasks autonomously, iterating between LLM calls and tool execution.

### Execution Cycle

```
User Message
       │
       ▼
  ┌─────────┐     ┌─────────────┐     ┌──────────┐
  │  LLM    │────▶│ Tool Calls? │────▶│ Execute  │
  │  Call   │     │  (parse)    │ Yes │ Tools    │
  └─────────┘     └──────┬──────┘     └────┬─────┘
       ▲                 │ No               │
       │                 ▼                  │
       │          Final Response            │
       └────────────────────────────────────┘
                  (append results)
```

### Auto-Continue

When the agent reaches the turn limit but is still executing tools, it automatically starts a new execution cycle. This enables long tasks without manual intervention.

- **Maximum continuations**: 2 (configurable)
- **Trigger**: last turn contains tool_calls (agent still working)
- **Context**: preserved between continuations

### Reflection (Self-Awareness)

Every 8 turns, the system injects a `[System: N turns used of M]` message, allowing the agent to consciously manage its budget — prioritizing remaining tasks or summarizing progress.

### Context Pruning

Old tool results are proactively trimmed based on turn age:

| Trim Type | Turn Age | Action |
|-----------|----------|--------|
| Soft trim | Medium | Truncate result to summary |
| Hard trim | Old | Remove result entirely |

This prevents context bloat without waiting for LLM overflow errors.

### Agent Steering

During tool execution, the agent monitors an interrupt channel for incoming messages. Users can redirect the agent mid-run, and the agent adjusts its behavior accordingly.

### Context Compaction

Three strategies to keep the context within limits:

| Strategy | Method | LLM Cost | Speed |
|----------|--------|----------|-------|
| `summarize` | Memory flush → LLM summarization → keep 25% recent | High | Slow |
| `truncate` | Drop oldest entries, keep 50% recent | Zero | Instant |
| `sliding` | Fixed window of most recent entries | Zero | Instant |

**Memory flush pre-compaction**: before compaction, the agent performs a dedicated turn to save durable memories to disk using an append-only strategy, ensuring important context survives.

**Preventive compaction**: triggers automatically at 80% of the `max_messages` threshold, avoiding overflow during conversation.

---

## Tool System

### Built-in Tools (70+)

#### Filesystem

| Tool | Description | Permission |
|------|-------------|------------|
| `read_file` | Read any file. Supports line ranges (offset + limit) | user |
| `write_file` | Create or overwrite files. Auto-creates parent directories | owner |
| `edit_file` | Precise line-based edits (search & replace) | owner |
| `list_files` | List directory contents with metadata (size, permissions, type) | user |
| `search_files` | Regex search across files (ripgrep-style). Supports include/exclude | user |
| `glob_files` | Find files by recursive glob pattern | user |

#### Shell and SSH

| Tool | Description | Permission |
|------|-------------|------------|
| `bash` | Execute shell commands. Persistent CWD and env across calls | owner |
| `set_env` | Set persistent environment variables for bash | owner |
| `ssh` | Execute commands on remote machines via SSH | owner |
| `scp` | Copy files to/from remote machines | admin |

#### Web

| Tool | Description | Permission |
|------|-------------|------------|
| `web_search` | DuckDuckGo search (HTML parsing). SSRF-protected | user |
| `web_fetch` | Fetch URL content with SSRF validation. Returns clean content | user |

#### Memory

| Tool | Description | Permission |
|------|-------------|------------|
| `memory` | Manage long-term memory with actions: save, search, list, index | user |

#### Scheduler

| Tool | Description | Permission |
|------|-------------|------------|
| `schedule_add` | Add cron task (cron expression + command/message) | admin |
| `schedule_list` | List scheduled tasks | user |
| `schedule_remove` | Remove a scheduled task | admin |

#### Media

| Tool | Description | Permission |
|------|-------------|------------|
| `describe_image` | Describe image content via LLM vision API | user |
| `transcribe_audio` | Audio transcription via Whisper API | user |

#### Subagents

| Tool | Description | Permission |
|------|-------------|------------|
| `spawn_subagent` | Create child agent for parallel work | admin |
| `list_subagents` | List active subagents and their status | admin |
| `wait_subagent` | Wait for subagent completion | admin |
| `stop_subagent` | Terminate a running subagent | admin |

#### Browser Automation

| Tool | Description | Permission |
|------|-------------|------------|
| `browser_navigate` | Navigate browser to URL via CDP | admin |
| `browser_screenshot` | Take screenshot of current page | admin |
| `browser_content` | Extract page text content | admin |
| `browser_click` | Click element by CSS selector | admin |
| `browser_fill` | Fill form input field | admin |

#### Interactive Canvas

| Tool | Description | Permission |
|------|-------------|------------|
| `canvas_create` | Create HTML/JS canvas with live-reload | admin |
| `canvas_update` | Update canvas content (triggers live-reload) | admin |
| `canvas_list` | List active canvases | user |
| `canvas_stop` | Stop a canvas server | admin |

#### Session Management

| Tool | Description | Permission |
|------|-------------|------------|
| `sessions_list` | List active sessions across all workspaces | admin |
| `sessions_send` | Send message to another session (inter-agent) | admin |
| `sessions_delete` | Delete a session by ID | admin |
| `sessions_export` | Export session history and metadata as JSON | admin |

#### Skills Management

| Tool | Description | Permission |
|------|-------------|------------|
| `init_skill` | Create new skill scaffold (SKILL.md + structure) | admin |
| `edit_skill` | Modify files of an existing skill | admin |
| `add_script` | Add script to a skill | admin |
| `list_skills` | List installed skills | user |
| `test_skill` | Test skill execution | admin |
| `install_skill` | Install from ClawHub, GitHub, URL, or local path | admin |
| `search_skills` | Search the ClawHub catalog | user |
| `remove_skill` | Uninstall a skill | admin |

#### Git (Native)

| Tool | Description | Permission |
|------|-------------|------------|
| `git_status` | Working tree status with file lists (staged, modified, untracked) | user |
| `git_diff` | View diffs (staged, unstaged, between refs) | user |
| `git_log` | Commit log with author, date, message | user |
| `git_commit` | Create commits (supports --all, --amend) | owner |
| `git_branch` | Create, delete, list, switch branches | owner |
| `git_stash` | Stash/pop/list/drop operations | owner |
| `git_blame` | Line-by-line attribution for files | user |

#### Docker (Native)

| Tool | Description | Permission |
|------|-------------|------------|
| `docker_ps` | List containers (running/all) | user |
| `docker_logs` | View container logs (tail, follow) | user |
| `docker_exec` | Execute commands inside containers | admin |
| `docker_images` | List local images | user |
| `docker_compose` | Docker Compose operations (up, down, ps, logs) | admin |
| `docker_stop` | Stop running containers | admin |
| `docker_rm` | Remove containers | admin |

#### Database

| Tool | Description | Permission |
|------|-------------|------------|
| `db_query` | Execute SELECT queries (PostgreSQL, MySQL, SQLite) | admin |
| `db_execute` | Execute mutating queries (INSERT, UPDATE, DELETE) | owner |
| `db_schema` | Inspect table schemas and column types | admin |
| `db_connections` | List active database connections | admin |

#### Developer Utilities

| Tool | Description | Permission |
|------|-------------|------------|
| `json_format` | Pretty-print or compact JSON | user |
| `jwt_decode` | Decode JWT tokens (header + payload) | user |
| `regex_test` | Test regex patterns against strings | user |
| `base64_encode` | Encode string to Base64 | user |
| `base64_decode` | Decode Base64 to string | user |
| `hash` | Generate MD5, SHA-1, or SHA-256 hashes | user |
| `uuid_generate` | Generate v4 UUIDs | user |
| `url_parse` | Parse URL into components | user |
| `timestamp_convert` | Convert between Unix timestamps and human-readable dates | user |

#### System & Environment

| Tool | Description | Permission |
|------|-------------|------------|
| `env_info` | OS, arch, Go version, hostname, memory, disk | user |
| `port_scan` | Check which ports are listening | user |
| `process_list` | List running processes with resource usage | user |

#### Codebase Analysis

| Tool | Description | Permission |
|------|-------------|------------|
| `codebase_index` | Build directory tree (respects .gitignore) | user |
| `code_search` | Regex search with ripgrep (grep fallback) | user |
| `code_symbols` | Extract functions, types, and classes from files | user |
| `cursor_rules_generate` | Generate IDE rules based on project structure | admin |

#### Testing

| Tool | Description | Permission |
|------|-------------|------------|
| `test_run` | Auto-detect framework and run tests (Go, Node, Python, Rust, etc.) | admin |
| `api_test` | HTTP endpoint validation (method, headers, body, assertions) | admin |
| `test_coverage` | Run tests with coverage reporting | admin |

#### Operations

| Tool | Description | Permission |
|------|-------------|------------|
| `server_health` | Check server health (HTTP, TCP, DNS) | user |
| `deploy_run` | Execute deploy pipeline with pre/post checks and dry-run | owner |
| `tunnel_manage` | Manage SSH tunnels (create, list, stop) | admin |
| `ssh_exec` | Execute commands on remote servers via SSH | admin |

#### Product Management

| Tool | Description | Permission |
|------|-------------|------------|
| `sprint_report` | Generate sprint report from git history | user |
| `dora_metrics` | Calculate DORA metrics (deploy freq, lead time, failure rate) | user |
| `project_summary` | Generate project overview from code and git data | user |

#### Daemon Management

| Tool | Description | Permission |
|------|-------------|------------|
| `start_daemon` | Start a background process (dev server, watcher, etc.) | admin |
| `daemon_logs` | View daemon output (ring buffer, last N lines) | user |
| `daemon_list` | List running daemons with health status | user |
| `daemon_stop` | Stop a running daemon | admin |
| `daemon_restart` | Restart a daemon | admin |

#### Plugins

| Tool | Description | Permission |
|------|-------------|------------|
| `plugin_list` | List available and active plugins | user |
| `plugin_install` | Install a plugin (GitHub, Jira, Sentry) | admin |
| `plugin_call` | Call a specific plugin tool | admin |

#### Team & Multi-user

| Tool | Description | Permission |
|------|-------------|------------|
| `team_users` | Manage team users and roles (owner, admin, editor, viewer) | admin |
| `shared_memory` | Read/write team-wide shared memory entries | user |

#### Teams (Persistent Agents)

| Tool | Description | Permission |
|------|-------------|------------|
| `team_create` | Create a new team | admin |
| `team_list` | List all teams | user |
| `team_create_agent` | Create a persistent agent with role and personality | admin |
| `team_list_agents` | List agents in a team | user |
| `team_stop_agent` | Stop an agent (disable heartbeats) | admin |
| `team_delete_agent` | Permanently delete an agent | admin |
| `team_create_task` | Create a task with assignees | user |
| `team_list_tasks` | List tasks (filterable by status/assignee) | user |
| `team_update_task` | Update task status with optional comment | user |
| `team_assign_task` | Assign agents to a task | admin |
| `team_comment` | Add comment to task thread with @mentions | user |
| `team_check_mentions` | Check pending @mentions for current agent | user |
| `team_send_message` | Send direct message to an agent | user |
| `team_save_fact` | Save a fact to shared team memory | user |
| `team_get_facts` | Get all shared facts | user |
| `team_delete_fact` | Delete a fact | admin |
| `team_standup` | Generate daily standup summary | user |

#### IDE Configuration

| Tool | Description | Permission |
|------|-------------|------------|
| `ide_configure` | Generate IDE configuration for MCP integration | user |

---

## MCP Server (IDE Integration)

DevClaw implements a [Model Context Protocol](https://modelcontextprotocol.io/) server:

- **Transports**: stdio (for IDEs) and SSE (for web clients)
- **Protocol**: JSON-RPC 2.0
- **Methods**: `initialize`, `tools/list`, `tools/call`, `resources/list`, `prompts/list`, `ping`
- **CLI**: `devclaw mcp serve`

Any MCP-compatible IDE can use DevClaw as a tool backend.

---

## Daemon Manager

Background process lifecycle management for development workflows:

- **Start/stop/restart** dev servers, watchers, databases, build tools
- **Ring buffer**: last 1000 lines of output per daemon
- **Health monitoring**: periodic checks with configurable intervals
- **Auto-restart**: restart on failure with backoff
- Exposed as tools: `start_daemon`, `daemon_logs`, `daemon_list`, `daemon_stop`, `daemon_restart`

---

## Plugin System

Extensible plugin architecture:

- **Built-in plugins**: GitHub (API), Jira (API), Sentry (API)
- **Plugin Manager**: YAML-based plugin definitions, load/enable/disable lifecycle
- **Webhook Dispatcher**: route events (tool_use, error, deploy) to external endpoints
- **Custom plugins**: define tools, config, and webhook subscriptions in YAML

---

## Lifecycle Hooks

16+ lifecycle events that external code can subscribe to for observing and modifying agent behavior.

### Events

| Event | Phase | Blocking |
|-------|-------|----------|
| `SessionStart` | Session creation | No |
| `SessionEnd` | Session destruction | No |
| `PreToolUse` | Before tool execution | Yes (can block) |
| `PostToolUse` | After tool execution | No |
| `AgentStart` | Agent loop begins | No |
| `AgentStop` | Agent loop ends | No |
| `PreCompact` | Before session compaction | No |
| `PostCompact` | After session compaction | No |
| `MessageReceived` | New message arrives | No |
| `MessageSent` | Response sent to channel | No |

### Features

- **Priority-ordered dispatch**: hooks execute in priority order.
- **Sync/async modes**: blocking hooks for pre-execution validation, non-blocking for logging/monitoring.
- **Panic recovery**: individual handler panics don't crash the dispatch or goroutine.
- **Deduplication**: prevents duplicate event subscriptions.

---

## Queue Modes

Configurable strategies for handling incoming messages when the agent is busy:

| Mode | Behavior |
|------|----------|
| `collect` | Queue messages, process as batch when agent is free |
| `steer` | Inject new message into running agent's context |
| `followup` | Queue as followup to current run |
| `interrupt` | Cancel current run, process new message |
| `steer-backlog` | Steer if possible, queue remainder |

```yaml
queue:
  default_mode: collect
  by_channel:
    whatsapp: collect
    webui: steer
  drop_policy: oldest
```

---

## Memory System

### File-Based Memory

- **MEMORY.md**: long-term facts curated by the agent.
- **Daily notes** (`memory/YYYY-MM-DD.md`): daily logs.
- **Session facts** (`facts.json`): per-session extracted facts.

### Advanced Memory (SQLite + Vectors)

SQLite store with FTS5 (keyword) and vector search (embeddings):

1. **Indexing**: `.md` files in the memory directory are chunked by heading/paragraph/sentence.
2. **Embeddings**: chunks embedded via `text-embedding-3-small` (1536 dims), cached in SQLite.
3. **Delta Sync**: only re-indexes/re-embeds chunks whose SHA-256 hash changed.
4. **Hybrid search**: combines BM25 (FTS5) and cosine similarity via Reciprocal Rank Fusion (RRF).

### Memory Security

Memory content injected into prompts is treated as untrusted data — HTML entities are escaped, dangerous tags stripped, content wrapped in `<relevant-memories>` tags, and injection patterns detected.

### Session Memory

When enabled, the `/new` command summarizes the conversation via LLM before clearing history. The summary is saved to `memory/YYYY-MM-DD-slug.md` and indexed for future recall.

---

## Skill System

Skills extend the agent's capabilities with custom tools and behaviors.

### Supported Formats

| Format | Structure | Execution |
|--------|-----------|-----------|
| **Native Go** | `skill.yaml` + `skill.go` | Compiled into binary |
| **ClawHub** | `SKILL.md` + `scripts/` | Python, Node.js, Shell — sandboxed |

### Installation Sources

| Source | Example |
|--------|---------|
| ClawHub | `devclaw skill install brave-search` |
| GitHub | `devclaw skill install github.com/user/repo` |
| URL | `devclaw skill install https://example.com/skill.tar.gz` |
| Local | `devclaw skill install ./my-local-skill` |
| Chat | Ask the agent to create one |

### Creation via Chat

The agent can create skills interactively:
1. `init_skill` creates the scaffold (SKILL.md + directory structure).
2. `edit_skill` and `add_script` allow modifying/adding code.
3. `test_skill` validates execution.

---

## Communication Channels

### WhatsApp

Native Go implementation via [whatsmeow](https://go.mau.fi/whatsmeow):

- **Messages**: text, images, audio, video, documents, stickers, voice notes, locations, contacts.
- **Interactions**: reactions, reply/quoting, typing indicators, read receipts.
- **Groups**: full group message support with access control.
- **Device name**: "DevClaw". LID resolution for phone number normalization.

### Group Chat (Enhanced)

| Feature | Description |
|---------|-------------|
| Activation modes | Always respond, mention-only, or keyword-based |
| Intro messages | Automatic welcome message for new groups |
| Context injection | Inject group context into agent prompts |
| Participant tracking | Track active participants |
| Quiet hours | Suppress responses during configured hours |
| Ignore patterns | Skip messages matching regex patterns |

### Media Processing

| Type | Processing | API |
|------|-----------|-----|
| Image | Automatic description via LLM vision | OpenAI Vision |
| Audio | Automatic transcription | Whisper |
| Video | Thumbnail extraction → vision | OpenAI Vision |

Media enrichment runs **asynchronously** — the agent starts responding immediately while vision/transcription happens in background.

### Block Streaming

Progressive delivery of long responses:

```yaml
block_stream:
  enabled: false
  min_chars: 20
  idle_ms: 200
  max_chars: 3000
```

---

## HTTP Gateway

OpenAI-compatible REST API with WebSocket support.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/v1/chat/completions` | Chat completions (SSE streaming) |
| GET | `/api/sessions` | List all sessions |
| GET | `/api/sessions/:id` | Session details |
| DELETE | `/api/sessions/:id` | Delete session |
| GET | `/api/usage` | Global token statistics |
| GET | `/api/usage/:session` | Per-session usage |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |
| POST | `/api/chat/{id}/stream` | Unified send+stream (SSE) |
| WS | `/ws` | WebSocket JSON-RPC (bidirectional) |

### WebSocket JSON-RPC

Bidirectional communication supporting:
- **Client requests**: `chat.send`, `chat.abort`, `session.list`
- **Server events**: `delta`, `tool_use`, `done`, `error`

### Authentication

```yaml
gateway:
  enabled: true
  address: ":8080"
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

---

## Scheduler (Advanced Cron)

Task scheduling system with advanced job features:

| Feature | Description |
|---------|-------------|
| Cron expressions | Standard 5-field or predefined (`@hourly`, `@daily`) |
| Isolated sessions | Each job runs in its own session |
| Announce | Broadcast results to target channels |
| Subagent spawn | Run job as a subagent |
| Per-job timeouts | Custom timeout per task |
| Labels | Categorize and filter jobs |
| Persistence | Jobs survive restarts |

---

## Remote Access (Tailscale)

Secure remote access via Tailscale Serve/Funnel:

```yaml
tailscale:
  enabled: true
  serve_port: 8080
  funnel: false     # expose to internet
```

No manual port forwarding or DNS configuration required.

---

## Heartbeat

Proactive agent behavior at configurable intervals:

```yaml
heartbeat:
  enabled: true
  interval: 30m
  active_start: 9
  active_end: 22
  channel: whatsapp
  chat_id: "5511999999999"
```

The agent reads `HEARTBEAT.md` for pending tasks and acts on them. Replies with `HEARTBEAT_OK` if nothing needs attention.

---

## Workspaces

Multi-tenant isolation with independent configurations per workspace. Each workspace has: independent system prompt, skills, model, language, and conversation memory.

---

## Session Management

### Structured Session Keys

Sessions are identified by `SessionKey` (Channel, ChatID, Branch), enabling multi-agent routing and session branching.

### CRUD Operations

| Operation | Description |
|-----------|-------------|
| Create | Automatic on first message |
| Get | By session ID or structured key |
| Delete | Via tool or API |
| Export | Full history + metadata as JSON |
| Rename | Change session display name |

### Bounded Followup Queue

When the agent is busy, incoming messages are queued with FIFO eviction at 20 items maximum.

---

## Token Usage Tracking

Per-session and global tracking of consumed tokens. Accessible via `/usage` command or `GET /api/usage`.

---

## Config Hot-Reload

`ConfigWatcher` monitors `config.yaml` for changes. Hot-reloadable: access control, instructions, tool guard, heartbeat, token budgets, queue modes. No restart required.

---

## CLI

### Main Commands

| Command | Description |
|---------|-------------|
| `devclaw setup` | Interactive wizard (TUI) |
| `devclaw serve` | Start daemon |
| `devclaw chat [msg]` | Interactive REPL or single message |
| `devclaw mcp serve` | Start MCP server over stdio (for IDE integration) |
| `devclaw fix [file]` | Analyze and fix errors (supports pipe: `npm build 2>&1 \| devclaw fix`) |
| `devclaw explain [path]` | Explain code, files, or entire directories |
| `devclaw diff [--staged]` | AI review of git changes |
| `devclaw commit [--dry-run]` | Generate conventional commit message and commit |
| `devclaw how "task"` | Generate shell commands without executing |
| `devclaw shell-hook bash\|zsh\|fish` | Generate shell hook for auto error capture |
| `devclaw config init/show/validate` | Config management |
| `devclaw config vault-*` | Vault management |
| `devclaw skill list/search/install` | Skills management |
| `devclaw schedule list/add` | Cron management |
| `devclaw health` | Health check |
| `devclaw changelog` | Version changelog |

### Pipe Mode

The `chat` command reads from stdin when piped, enabling powerful integrations:

```bash
git diff | devclaw "review this"
npm run build 2>&1 | devclaw fix
cat README.md | devclaw "translate to portuguese"
kubectl logs pod-name | devclaw "find the error"
```

### Chat Commands (via messaging or CLI REPL)

| Command | Description |
|---------|-------------|
| `/help` | List all commands |
| `/allow`, `/block`, `/admin` | Access management |
| `/users` | List authorized users |
| `/model [name]` | Show/change model |
| `/usage [global\|reset]` | Token statistics |
| `/compact` | Manually compact session |
| `/think [off\|low\|medium\|high]` | Extended thinking level |
| `/verbose [on\|off]` | Toggle verbose output |
| `/reasoning [level]` | Set reasoning format (alias for /think) |
| `/queue [mode]` | Show/change queue mode |
| `/activation [mode]` | Show/change group activation mode |
| `/new` | Clear history (with summarization if enabled) |
| `/reset` | Full session reset |
| `/stop` | Cancel active execution |
| `/approve`, `/deny` | Approve/reject tool execution |
| `/ws create/assign/list` | Workspace management |

---

## Deployment

### Docker

```bash
docker compose up -d
docker compose logs -f devclaw
```

### systemd

```bash
sudo cp devclaw.service /etc/systemd/system/
sudo systemctl enable --now devclaw
```

### Binary

```bash
make build && ./bin/devclaw serve
```
