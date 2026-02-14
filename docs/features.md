# GoClaw — Features and Capabilities

Detailed documentation of all features available in GoClaw.

---

## Autonomous Agent Loop

The core of GoClaw is an agentic loop that allows the assistant to execute complex tasks autonomously, iterating between LLM calls and tool execution.

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

### Context Compaction

Three strategies to keep the context within limits:

| Strategy | Method | LLM Cost | Speed |
|----------|--------|----------|-------|
| `summarize` | Memory flush → LLM summarization → keep 25% recent | High | Slow |
| `truncate` | Drop oldest entries, keep 50% recent | Zero | Instant |
| `sliding` | Fixed window of most recent entries | Zero | Instant |

**Preventive compaction**: triggers automatically at 80% of the `max_messages` threshold, avoiding overflow during conversation.

---

## Tool System

### Built-in Tools

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
| `memory_save` | Save facts to long-term memory. Triggers re-index | user |
| `memory_search` | Hybrid semantic + keyword search (BM25 + cosine) | user |
| `memory_list` | List recent memory entries | user |
| `memory_index` | Manually re-index all memory files | admin |

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

```yaml
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
```

### Session Memory

When enabled, the `/new` command summarizes the conversation via LLM before clearing history. The summary is saved to `memory/YYYY-MM-DD-slug.md` and indexed for future recall.

---

## Skill System

Skills extend the agent's capabilities with custom tools and behaviors.

### Supported Formats

| Format | Structure | Execution |
|--------|-----------|-----------|
| **Native Go** | `skill.yaml` + `skill.go` | Compiled into binary |
| **ClawdHub** | `SKILL.md` + scripts (Python/Node/Shell) | Sandbox |

### Installation Sources

| Source | Example |
|--------|---------|
| ClawHub | `copilot skill install brave-search` |
| GitHub | `copilot skill install github.com/user/repo` |
| URL | `copilot skill install https://example.com/skill.tar.gz` |
| Local | `copilot skill install ./my-local-skill` |
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
- **Device name**: "GoClaw". LID resolution for phone number normalization.

### Media Processing

| Type | Processing | API |
|------|-----------|-----|
| Image | Automatic description via LLM vision | OpenAI Vision |
| Audio | Automatic transcription | Whisper |
| Video | Thumbnail extraction → vision | OpenAI Vision |

```yaml
media:
  vision_enabled: true
  vision_detail: auto       # auto | low | high
  transcription_enabled: true
  transcription_model: whisper-1
  max_image_size: 20971520  # 20 MB
  max_audio_size: 26214400  # 25 MB
```

### Message Splitting

Long responses are split into channel-compatible chunks:
- **WhatsApp**: 4096 characters per message.
- Splits preserve code blocks and prefer paragraph/sentence boundaries.
- Automatic markdown to WhatsApp format conversion (`**bold**` → `*bold*`).

### Block Streaming

Progressive delivery of long responses so the user sees activity in real-time:

```yaml
block_stream:
  enabled: false
  min_chars: 80        # Minimum before first block
  idle_ms: 1200        # Idle timeout before flush
  max_chars: 3000      # Force flush at this limit
```

---

## HTTP Gateway

OpenAI-compatible REST API for integration with external applications.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/v1/chat/completions` | Chat completions (supports SSE streaming) |
| GET | `/api/sessions` | List all sessions |
| GET | `/api/sessions/:id` | Session details |
| DELETE | `/api/sessions/:id` | Delete session |
| GET | `/api/usage` | Global token statistics |
| GET | `/api/usage/:session` | Per-session usage |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |

### Authentication

Optional Bearer token:

```yaml
gateway:
  enabled: true
  address: ":8080"
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

---

## Scheduler (Cron)

Task scheduling system with SQLite persistence:

```yaml
scheduler:
  enabled: true
  storage: ./data/scheduler.db
```

- **Cron expressions**: standard 5-field or predefined (`@hourly`, `@daily`).
- **Persistence**: tasks survive restarts.
- **Management**: via tools (`schedule_add/list/remove`) or CLI (`copilot schedule`).

---

## Heartbeat

Proactive agent behavior at configurable intervals:

```yaml
heartbeat:
  enabled: true
  interval: 30m
  active_start: 9       # Active hours start (hour)
  active_end: 22         # Active hours end (hour)
  channel: whatsapp
  chat_id: "5511999999999"
```

The agent reads `HEARTBEAT.md` for pending tasks and acts on them. Replies with `HEARTBEAT_OK` if nothing needs attention.

---

## Workspaces

Multi-tenant isolation with independent configurations per workspace:

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

Each workspace has: independent system prompt, skills, model, language, and conversation memory.

---

## Token Usage Tracking

Per-session and global tracking of consumed tokens:

| Model | Input/1M | Output/1M |
|-------|----------|-----------|
| `gpt-5-mini` | $0.15 | $0.60 |
| `gpt-5` | $2.00 | $8.00 |
| `gpt-4o` | $2.50 | $10.00 |
| `claude-opus-4.6` | $5.00 | $25.00 |
| `claude-sonnet-4.5` | $3.00 | $15.00 |
| `glm-5` | $1.00 | $3.20 |
| `glm-4.7-flash` | $0.10 | $0.40 |

Accessible via `/usage` command or `GET /api/usage`.

---

## Config Hot-Reload

`ConfigWatcher` monitors `config.yaml` for changes (mtime + SHA-256 hash).

**Hot-reloadable fields** (no restart required):
- Access control (allowed/blocked users)
- Custom instructions
- Tool guard rules
- Heartbeat config
- Token budgets

**Change detection**: periodic mtime polling, followed by SHA-256 verification to confirm actual change.

---

## CLI

### Main Commands

| Command | Description |
|---------|-------------|
| `copilot setup` | Interactive wizard (TUI) — model selection, vault, provider |
| `copilot serve` | Start daemon with messaging channels |
| `copilot chat [msg]` | Interactive REPL or single message |
| `copilot config init` | Create default config |
| `copilot config show` | Show current config |
| `copilot config validate` | Validate config |
| `copilot config vault-*` | Vault management |
| `copilot skill list/search/install` | Skills management |
| `copilot schedule list/add` | Cron management |
| `copilot health` | Health check |
| `copilot changelog` | Version changelog |

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
| `/new` | Clear history (with summarization if enabled) |
| `/reset` | Full session reset |
| `/stop` | Cancel active execution |
| `/approve`, `/deny` | Approve/reject tool execution |
| `/ws create/assign/list` | Workspace management |

### CLI REPL Features

Interactive REPL with readline support:
- Arrow key history (↑/↓)
- Reverse search (Ctrl+R)
- Tab completion
- Line editing (Ctrl+A/E/W/U)
- Persistent history (`~/.goclaw/chat_history`)

---

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
