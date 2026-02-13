# GoClaw

Open-source AI assistant framework in Go. Single binary, zero runtime dependencies. Supports CLI and messaging channels (WhatsApp, Discord, Telegram). Full filesystem access, tool use, subagents, encrypted vault, and sandboxed skill execution.

## Quick Start

```bash
git clone https://github.com/jholhewres/goclaw.git && cd goclaw
make build
./bin/copilot setup   # interactive wizard (arrow keys, model selection, vault)
make run              # build + serve
```

Or install directly:

```bash
go install github.com/jholhewres/goclaw/cmd/copilot@latest
copilot setup && copilot serve
```

## Architecture

```
goclaw/
├── cmd/copilot/commands/     CLI (cobra): setup, serve, chat, config, skill, schedule, health
├── pkg/goclaw/
│   ├── copilot/              Core assistant orchestrator
│   │   ├── assistant.go      Message flow, agent dispatch, compaction, media enrichment
│   │   ├── agent.go          Agent loop (multi-turn tool calling, auto-continue, reflection)
│   │   ├── llm.go            LLM client (OpenAI-compatible, streaming, fallback, prompt caching)
│   │   ├── prompt_layers.go  8-layer prompt composer with token budget trimming
│   │   ├── session.go        Per-chat session isolation with persistence
│   │   ├── session_persist.go JSONL session persistence to disk
│   │   ├── tool_executor.go  Tool registry, dispatch, parallel execution
│   │   ├── tool_guard.go     Access control, dangerous command blocking, audit log
│   │   ├── exec_approval.go  Interactive tool approval via chat
│   │   ├── system_tools.go   Built-in tools (file I/O, bash, ssh, web, memory, cron)
│   │   ├── media_tools.go    Vision (image description) and audio transcription (Whisper)
│   │   ├── subagent.go       Subagent spawning and orchestration
│   │   ├── skill_creator.go  Create/edit skills via chat
│   │   ├── vault.go          Encrypted vault (AES-256-GCM + Argon2id)
│   │   ├── keyring.go        OS keyring + secret resolution chain
│   │   ├── heartbeat.go      Proactive scheduled agent behavior
│   │   ├── config_watcher.go Config hot-reload (mtime + SHA256)
│   │   ├── message_queue.go  Per-session debounce, dedup, burst handling
│   │   ├── message_split.go  Channel-aware message splitting (preserves code blocks)
│   │   ├── markdown.go       WhatsApp-specific markdown formatting
│   │   ├── usage_tracker.go  Token/cost tracking per session and global
│   │   ├── access.go         Per-user/group allowlist, blocklist, roles
│   │   ├── workspace.go      Multi-tenant workspace isolation
│   │   ├── block_streamer.go Progressive message delivery to channels
│   │   ├── memory/           Persistent memory (SQLite FTS5+vector, embeddings, MEMORY.md)
│   │   └── security/         SSRF guard for web_fetch
│   ├── channels/             Channel interface + manager
│   │   └── whatsapp/         Native Go WhatsApp (whatsmeow)
│   ├── gateway/              HTTP API gateway (OpenAI-compatible)
│   ├── plugins/              Go native plugin loader (.so)
│   ├── sandbox/              Script sandbox (namespaces / Docker)
│   ├── scheduler/            Cron scheduler with file persistence
│   └── skills/               Skill system, ClawHub client, builtin adapter
├── skills/                   User-managed skills directory (git-ignored)
├── configs/
│   ├── bootstrap/            Template bootstrap files (SOUL.md, AGENTS.md, etc.)
│   └── copilot.example.yaml  Full config reference
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── copilot.service           systemd unit
```

## LLM Client

OpenAI-compatible HTTP client with provider-specific optimizations.

### Supported Providers

| Provider | Base URL | Provider Key |
|----------|----------|--------------|
| OpenAI | `https://api.openai.com/v1` | `openai` |
| Z.AI (API) | `https://api.z.ai/api/paas/v4` | `zai` |
| Z.AI (Coding) | `https://api.z.ai/api/coding/paas/v4` | `zai-coding` |
| Z.AI (Anthropic proxy) | `https://api.z.ai/api/anthropic` | `zai-anthropic` |
| Anthropic | `https://api.anthropic.com/v1` | `anthropic` |
| Any OpenAI-compatible | Custom URL | auto-detected |

Provider is auto-detected from the base URL or set explicitly in config.

### Model Defaults

Each model gets provider-specific defaults applied automatically:

| Model | Temperature | Max Output Tokens | Tools |
|-------|-------------|-------------------|-------|
| `gpt-5`, `gpt-5-mini` | 0.7 | 16384 | Yes |
| `gpt-4o`, `gpt-4o-mini` | 0.7 | 16384 | Yes |
| `gpt-4.5-preview` | 0.7 | 16384 | Yes |
| `claude-opus-4.6`, `claude-opus-4.5` | 1.0 | 16384 | Yes |
| `claude-sonnet-4.5` | 1.0 | 16384 | Yes |
| `glm-5` | 0.7 | 8192 | Yes |
| `glm-4.7`, `glm-4.7-flash` | 0.7 | 4096 | Yes |

### Model Fallback

On API failure, the client retries with exponential backoff and falls back to alternative models:

```yaml
fallback:
  models: [gpt-4o, glm-4.7-flash, claude-sonnet-4.5]
  max_retries: 2
  initial_backoff_ms: 1000
  max_backoff_ms: 30000
  retry_on_status_codes: [429, 500, 502, 503, 529]
```

Errors are classified as `Retryable`, `Auth`, `Context`, `BadRequest`, or `Fatal`. Only retryable errors trigger the fallback chain. `Retry-After` headers are respected.

### Streaming

Server-Sent Events (SSE) streaming via `CompleteWithToolsStream`. Parses `data: [DONE]` terminator. Falls back to non-streaming if the provider doesn't support it.

### Prompt Caching

For Anthropic and Z.AI Anthropic proxy, `cache_control: {"type": "ephemeral"}` is automatically added to the system message and the second-to-last user message. This enables prompt caching, reducing costs by up to 90% for conversations with the same system prompt.

## Agent Loop

The agent iterates: call LLM → if tool_calls → execute tools → append results → call LLM again, until a final text response or the turn limit.

| Parameter | Default | Config Key |
|-----------|---------|------------|
| Max turns per request | 25 | `agent.max_turns` |
| Turn timeout | 300s | `agent.turn_timeout_seconds` |
| Auto-continuations | 2 | `agent.max_continuations` |
| Reflection interval | every 8 turns | — |
| Max compaction attempts | 3 | `agent.max_compaction_attempts` |

**Auto-continue**: when the agent exhausts its turn budget while still calling tools, it automatically starts a continuation run (up to `max_continuations` times).

**Reflection**: every 8 turns, a `[System: N turns used of M]` nudge is injected so the agent can self-manage its budget.

**Context overflow handling**: if the LLM returns `context_length_exceeded`, the agent automatically compacts messages (keeping system + recent history), truncates tool results to 2000 chars, and retries up to `max_compaction_attempts` times.

## Prompt Composer

8-layer system prompt with priority-based token budget trimming.

| Layer | Priority | Content | Budget Source |
|-------|----------|---------|---------------|
| Core | 0 | Base identity, tooling guidance | `token_budget.system` |
| Safety | 5 | Guardrails, boundaries | 500 tokens |
| Identity | 10 | Custom instructions from config | 1000 tokens |
| Thinking | 12 | Extended thinking hints (`/think`) | 200 tokens |
| Bootstrap | 15 | SOUL.md, AGENTS.md, IDENTITY.md, USER.md, TOOLS.md | 4000 tokens |
| Business | 20 | Workspace context | 1000 tokens |
| Skills | 40 | Active skill instructions | `token_budget.skills` |
| Memory | 50 | Long-term facts, session facts | `token_budget.memory` |
| Temporal | 60 | Current date/time/timezone | 200 tokens |
| Conversation | 70 | Recent history (sliding window) | `token_budget.history` |
| Runtime | 80 | System info (OS, host, model, cwd) | 200 tokens |

**Token budget trimming**: the system prompt uses at most 40% of the total token budget. When exceeded, layers are trimmed from lowest priority first. Layers with priority < 20 (Core, Safety, Identity, Thinking) are never trimmed. Layers at priority >= 50 can be dropped entirely if over budget.

**Conversation sliding window**: history is built backwards from most recent, stopping when the history token budget is reached. Individual messages are truncated (user: 2000 chars, assistant: 4000 chars). Older messages are omitted with a count indicator.

### Bootstrap Files

Template files in `configs/bootstrap/` are loaded from the workspace root at runtime. Copy to project root and customize:

| File | Purpose |
|------|---------|
| `SOUL.md` | Agent persona, tone, boundaries |
| `AGENTS.md` | Operating rules, memory protocol, session behavior |
| `IDENTITY.md` | Self-assigned identity (filled by agent) |
| `USER.md` | User profile (learned over time) |
| `TOOLS.md` | Environment-specific notes (SSH hosts, API endpoints) |
| `HEARTBEAT.md` | Periodic tasks for the heartbeat system |
| `BOOT.md` | Instructions executed once after startup (proactive init) |

## Block Streaming (Progressive Message Delivery)

Long LLM responses are sent progressively to channels as partial messages, so the user sees activity in real-time instead of waiting for the complete response.

```yaml
block_stream:
  enabled: false        # enable progressive delivery
  min_chars: 80         # minimum chars before first block
  idle_ms: 1200         # idle timeout before flushing partial block
  max_chars: 3000       # force flush at this limit
```

Blocks are split at natural boundaries (paragraphs, sentence endings, list items). The final message is only sent if no blocks were already delivered, avoiding duplicates.

## Advanced Memory (SQLite + Vector Search)

In addition to file-based memory (`MEMORY.md`, daily logs), GoClaw supports a SQLite-backed memory store with FTS5 keyword search and in-process vector similarity search.

### How It Works

1. **Indexing**: `.md` files in the memory directory are chunked (by heading, paragraph, sentence) with configurable overlap
2. **Embeddings**: chunks are embedded via OpenAI `text-embedding-3-small` (1536 dims) and cached in SQLite to minimize API calls
3. **Delta Sync**: only re-indexes/re-embeds chunks whose SHA-256 hash changed
4. **Hybrid Search**: combines BM25 (FTS5) and cosine similarity scores using Reciprocal Rank Fusion (RRF)

### Configuration

```yaml
memory:
  embedding:
    provider: openai        # openai or none
    model: text-embedding-3-small
    dimensions: 1536
    batch_size: 20
  search:
    hybrid_weight_vector: 0.7
    hybrid_weight_bm25: 0.3
    max_results: 6
    min_score: 0.1
  index:
    auto: true              # auto-index on startup
    chunk_max_tokens: 500
  session_memory:
    enabled: false          # summarize sessions on /new
    messages: 15            # last N messages to summarize
```

### Memory Tools

| Tool | Description |
|------|-------------|
| `memory_save` | Save facts (triggers re-index if SQLite enabled) |
| `memory_search` | Hybrid semantic + keyword search (falls back to substring) |
| `memory_list` | List recent memory entries |
| `memory_index` | Manually trigger re-indexing of all memory files |

### Session Memory

When `session_memory.enabled` is true, the `/new` command summarizes the conversation via LLM before clearing history. The summary is saved to `memory/YYYY-MM-DD-slug.md` and indexed for future retrieval, giving the agent long-term recall of past interactions.

## Tools

### Built-in Tools

| Tool | Description |
|------|-------------|
| `read_file` | Read any file on the filesystem |
| `write_file` | Write/create files anywhere |
| `edit_file` | Precise line-based edits (search & replace) |
| `list_files` | List directory contents with metadata |
| `search_files` | Regex search across files (ripgrep-style) |
| `glob_files` | Find files by glob pattern |
| `bash` | Execute shell commands with persistent cwd and env |
| `set_env` | Set persistent environment variables for bash |
| `ssh` | Execute commands on remote machines |
| `scp` | Copy files to/from remote machines |
| `web_search` | DuckDuckGo search (HTML parsing, SSRF-protected) |
| `web_fetch` | Fetch URL content (SSRF-protected) |
| `memory_save` | Save facts to long-term memory |
| `memory_search` | Hybrid semantic + keyword search |
| `memory_list` | List recent memory entries |
| `memory_index` | Re-index memory files (SQLite store) |
| `schedule_add` | Add cron task |
| `schedule_list` | List scheduled tasks |
| `schedule_remove` | Remove a scheduled task |
| `describe_image` | Vision: describe image content via LLM |
| `transcribe_audio` | Audio transcription via Whisper API |
| `init_skill` | Create a new skill scaffold |
| `edit_skill` | Modify skill files |
| `add_script` | Add script to a skill |
| `list_skills` | List installed skills |
| `test_skill` | Test a skill |
| `install_skill` | Install from ClawHub, GitHub, URL, or local path |
| `search_skills` | Search ClawHub catalog |
| `remove_skill` | Uninstall a skill |
| `spawn_subagent` | Create a child agent for parallel work |
| `list_subagents` | List active subagents |
| `wait_subagent` | Wait for subagent completion |
| `stop_subagent` | Terminate a subagent |

### Tool Guard

Fine-grained security layer for all tool executions.

```yaml
security:
  tool_guard:
    enabled: true
    audit_log: ./data/audit.log
    allow_destructive: false    # rm -rf, mkfs, dd — blocked by default
    allow_sudo: false           # sudo blocked for non-owners
    allow_reboot: false         # shutdown/reboot blocked
    require_confirmation: [bash, ssh, scp, write_file]
    auto_approve: []
    dangerous_commands:          # additional regex patterns
      - "curl.*\\|.*sh"
    protected_paths:
      - /etc/shadow
      - ~/.ssh/id_*
    ssh_allowed_hosts: []       # empty = all allowed
    tool_permissions:
      bash: owner
      ssh: owner
      scp: admin
```

**Audit logging**: every tool execution (allowed or blocked) is logged to the audit log with timestamp, user, tool name, arguments, and result.

**Interactive approval**: tools in `require_confirmation` trigger a chat message asking the user to `/approve <id>` or `/deny <id>` before execution.

### Parallel Tool Execution

Independent tools run concurrently with a configurable semaphore:

```yaml
security:
  tool_executor:
    parallel: true
    max_parallel: 5
```

Stateful tools (`bash`) always run sequentially.

## Session Management

### Isolation

Each chat/group gets an independent session with its own history, facts, active skills, and config overrides. Sessions are thread-safe (`sync.RWMutex`).

### Persistence

Sessions are persisted to disk as JSONL (one entry per line). Facts and metadata are stored as separate JSON files. Sessions survive restarts.

```
data/sessions/
├── whatsapp_5511999999999/
│   ├── history.jsonl     # conversation entries
│   ├── facts.json        # extracted facts
│   └── meta.json         # session metadata
```

### Compaction

Three compression strategies, configurable via `memory.compression_strategy`:

| Strategy | Method | LLM Cost | Speed |
|----------|--------|----------|-------|
| `summarize` (default) | Memory flush → LLM summarization → keep 25% recent | High | Slow |
| `truncate` | Drop oldest entries, keep 50% recent | Zero | Instant |
| `sliding` | Fixed window of most recent entries | Zero | Instant |

**Preventive compaction**: triggers at 80% of `memory.max_messages` threshold (not 100%), avoiding mid-conversation overflow.

## Subagent System

The main agent can spawn child agents for parallel work.

```yaml
subagents:
  enabled: true
  max_concurrent: 3
  max_turns: 15
  timeout_seconds: 300
  denied_tools: [spawn_subagent, list_subagents, wait_subagent]
```

Subagents get a filtered tool set (no recursive spawning). Each runs in its own goroutine with an isolated context. The parent can wait for results or stop subagents.

## HTTP API Gateway

OpenAI-compatible REST API with optional Bearer token authentication.

```yaml
gateway:
  enabled: true
  address: ":8080"
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/v1/chat/completions` | OpenAI-compatible chat (supports SSE streaming) |
| GET | `/api/sessions` | List all sessions |
| GET | `/api/sessions/:id` | Get session details |
| DELETE | `/api/sessions/:id` | Delete a session |
| GET | `/api/usage` | Global token usage stats |
| GET | `/api/usage/:session` | Per-session usage |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |

## Security

### Encrypted Vault

Secrets stored in `.goclaw.vault` — AES-256-GCM encryption with Argon2id key derivation (64 MB memory, 3 iterations, 4 threads). Master password never stored.

```
Resolution order (first match wins):
  1. Encrypted vault (.goclaw.vault)
  2. OS keyring (GNOME/macOS/Windows)
  3. Environment variable (GOCLAW_API_KEY)
  4. config.yaml (${GOCLAW_API_KEY} reference)
```

### SSRF Protection

`web_fetch` validates URLs before fetching: blocks private IPs (10.x, 172.16-31.x, 192.168.x), loopback, link-local, cloud metadata endpoints (169.254.169.254), and non-HTTP/S schemes. Hostnames are resolved to IPs before checking.

### Access Control

```yaml
access:
  default_policy: deny    # deny | allow | ask
  owners: ["5511999999999"]
  admins: []
  allowed_users: []
  blocked_users: []
  allowed_groups: []
  blocked_groups: []
```

Roles: **owner** > **admin** > **user** > **blocked**. Managed via chat commands (`/allow`, `/block`, `/admin`, `/users`).

### Skills

Skills extend the agent's capabilities. The `skills/` directory at the project root is **user-managed** and git-ignored — your custom skills are never overwritten during updates.

```bash
copilot skill install brave-search    # Install from ClawHub
copilot skill install github.com/user/my-skill  # Install from GitHub
copilot skill list                    # List installed skills
```

Each skill is a folder inside `skills/` containing a `SKILL.md` (instructions for the agent) and optional scripts. You can also create skills via chat:

> "Crie uma skill de clima que usa a API do OpenWeatherMap"

The built-in skill engine (`pkg/goclaw/skills/`) handles loading, validation, and ClawHub integration. The `skills/` directory is exclusively for user-installed content.

| Source | Example |
|--------|---------|
| ClawHub | `copilot skill install brave-search` |
| GitHub | `copilot skill install github.com/user/repo` |
| URL | `copilot skill install https://example.com/skill.tar.gz` |
| Local | `copilot skill install ./my-local-skill` |
| Chat | Ask the agent to create one |

### Script Sandbox

Community scripts (Python, Node.js, Shell) execute in isolated environments:

| Level | Isolation | Use Case |
|-------|-----------|----------|
| `none` | Direct exec | Trusted/builtin only |
| `restricted` | Linux namespaces (PID, mount, net, user) | Community skills |
| `container` | Docker with purpose-built image | Untrusted scripts |

Pre-execution content scanning detects: `eval()`, reverse shells, crypto mining, shell injection, data exfiltration, and obfuscated code.

## Channels

### WhatsApp (core)

Native Go via [whatsmeow](https://go.mau.fi/whatsmeow). Supports: text, images, audio, video, documents, stickers, voice notes, locations, contacts, reactions, reply/quoting, typing indicators, read receipts, group messages.

Device name: "GoClaw". LID (Linked Identity) resolution for phone number normalization.

### Media Processing

Incoming images are automatically described via the LLM vision API. Incoming audio is transcribed via Whisper. Results are prepended to the user message before agent processing.

```yaml
media:
  vision_enabled: true
  vision_detail: auto       # auto | low | high
  transcription_enabled: true
  transcription_model: whisper-1
  max_image_size: 20971520  # 20MB
  max_audio_size: 26214400  # 25MB
```

### Message Splitting

Long responses are split into channel-compatible chunks (WhatsApp: 4096 chars). Splits preserve code blocks and prefer paragraph/sentence boundaries.

### WhatsApp Markdown

Standard markdown is converted to WhatsApp format: `**bold**` → `*bold*`, `_italic_`, `~~strike~~`, `` `code` ``, code blocks.

## Message Queue

Per-session debounce and deduplication for message bursts:

```yaml
queue:
  debounce_ms: 1000
  max_pending: 20
```

Messages arriving while the agent is processing are queued. Duplicate messages (same content within 5s) are dropped. When the agent finishes, queued messages are combined and processed as one.

## Token Usage Tracking

Per-session and global tracking of prompt tokens, completion tokens, request count, and estimated cost.

| Model | Input/1M | Output/1M |
|-------|----------|-----------|
| `gpt-5-mini` | $0.15 | $0.60 |
| `gpt-5` | $2.00 | $8.00 |
| `gpt-4o` | $2.50 | $10.00 |
| `claude-opus-4.6` | $5.00 | $25.00 |
| `claude-sonnet-4.5` | $3.00 | $15.00 |
| `glm-5` | $1.00 | $3.20 |
| `glm-4.7-flash` | $0.10 | $0.40 |

View via `/usage` command or `GET /api/usage`.

## Config Hot-Reload

`ConfigWatcher` monitors `config.yaml` for changes (mtime + SHA256 hash). Hot-reloadable fields: access control, instructions, tool guard, heartbeat, token budget. No restart required.

## Scheduler

Cron-based task scheduler with file persistence:

```yaml
scheduler:
  enabled: true
  storage: ./data/scheduler.db
```

Tasks are managed via tools (`schedule_add`, `schedule_list`, `schedule_remove`) or CLI (`copilot schedule`).

## Heartbeat

Proactive agent behavior on a configurable interval:

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

## Workspaces

Multi-tenant isolation with independent system prompts, skills, models, languages, and conversation memory per workspace.

```yaml
workspaces:
  default_workspace: default
  workspaces:
    - id: default
      name: Default
      active: true
    - id: work
      name: Dev Team
      model: gpt-4o-mini
      language: en
      skills: [github, web-search]
      members: ["5511888888888"]
      groups: ["120363000000000000@g.us"]
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `copilot setup` | Interactive setup wizard (arrow keys, model selection, vault) |
| `copilot serve` | Start daemon with messaging channels |
| `copilot chat [msg]` | Interactive REPL or single message |
| `copilot config init` | Create default config |
| `copilot config show` | Show current config |
| `copilot config validate` | Validate config |
| `copilot config vault-init` | Create encrypted vault |
| `copilot config vault-set` | Store API key in vault |
| `copilot config vault-status` | Show vault status |
| `copilot config vault-change-password` | Change vault master password |
| `copilot config set-key` | Store API key in OS keyring |
| `copilot config key-status` | Show API key resolution source |
| `copilot skill list` | List installed skills |
| `copilot skill search <query>` | Search available skills |
| `copilot skill install <name>` | Install a skill |
| `copilot schedule list` | List scheduled tasks |
| `copilot schedule add <cron> <cmd>` | Add a scheduled task |
| `copilot health` | Check service health |
| `copilot changelog` | Show changelog for current version |
| `copilot changelog --all` | Show full changelog |

### Chat Commands (via messaging or CLI REPL)

| Command | Description |
|---------|-------------|
| `/help` | List all commands |
| `/allow <phone>` | Grant access |
| `/block <phone>` | Block a contact |
| `/admin <phone>` | Promote to admin |
| `/users` | List authorized users |
| `/group allow` | Allow current group |
| `/status` | Bot status |
| `/model [name]` | Show or change model |
| `/usage [global\|reset]` | Token usage stats |
| `/compact` | Trigger session compaction |
| `/think [off\|low\|medium\|high]` | Set thinking level |
| `/new` | Clear session history |
| `/reset` | Full session reset |
| `/stop` | Cancel active agent run |
| `/approve <id>` | Approve pending tool execution |
| `/deny <id>` | Deny pending tool execution |
| `/ws create <id> <name>` | Create workspace |
| `/ws assign <phone> <ws>` | Assign user to workspace |
| `/ws list` | List workspaces |

### CLI Chat Features

Interactive REPL with `readline` support: arrow key history (↑/↓), reverse search (Ctrl+R), tab completion, line editing (Ctrl+A/E/W/U), persistent history (`~/.goclaw/chat_history`).

## Deployment

### Docker

```bash
docker compose up -d
docker compose logs -f copilot
```

### systemd

```bash
sudo cp copilot.service /etc/systemd/system/
sudo systemctl enable --now copilot
```

### Binary

```bash
make build && ./bin/copilot serve
```

## Dependencies

| Package | Purpose |
|---------|---------|
| [whatsmeow](https://go.mau.fi/whatsmeow) | WhatsApp channel (native Go) |
| [cobra](https://github.com/spf13/cobra) | CLI framework |
| [huh](https://github.com/charmbracelet/huh) | Interactive terminal forms (setup wizard) |
| [readline](https://github.com/chzyer/readline) | CLI chat interactivity |
| [cron](https://github.com/robfig/cron) | Task scheduler |
| [yaml.v3](https://gopkg.in/yaml.v3) | Configuration |
| [go-keyring](https://github.com/zalando/go-keyring) | OS keyring integration |
| [x/crypto](https://pkg.go.dev/golang.org/x/crypto) | Argon2id (vault key derivation) |
| [qrterminal](https://github.com/mdp/qrterminal) | QR code rendering |
| [go-sqlite3](https://github.com/mattn/go-sqlite3) | SQLite driver with FTS5 (memory index) |

Encryption: Go stdlib (`crypto/aes`, `crypto/cipher`). Sandbox: `os/exec`, `syscall` (Linux namespaces), Docker CLI.

## Author

**Jhol Hewres** — [jhol.code@gmail.com](mailto:jhol.code@gmail.com)

## License

MIT
