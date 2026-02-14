# GoClaw — Technical Architecture

Technical documentation of GoClaw's internal architecture, covering components, data flows, and design decisions.

## Overview

GoClaw is an AI assistant framework written in Go. Single binary, zero runtime dependencies. Supports interactive CLI and messaging channels (WhatsApp, with Discord/Telegram planned).

```
┌─────────────────────────────────────────────────────────┐
│                      cmd/copilot                        │
│              CLI (Cobra) — setup, serve, chat            │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                   pkg/goclaw/copilot                     │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │ Assistant │──│  Agent   │──│ LLMClient │              │
│  │ (message  │  │ (loop +  │  │ (OpenAI-  │              │
│  │  flow)    │  │  tools)  │  │  compat)  │              │
│  └────┬─────┘  └────┬─────┘  └───────────┘              │
│       │              │                                    │
│  ┌────▼─────┐  ┌────▼──────────────┐                     │
│  │ Session  │  │  ToolExecutor     │                     │
│  │ Manager  │  │  ├─ SystemTools   │                     │
│  │ (per-chat)│  │  ├─ SkillTools   │                     │
│  │          │  │  ├─ PluginTools   │                     │
│  └──────────┘  │  └─ ToolGuard    │                     │
│                └───────────────────┘                     │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │  Vault   │  │  Memory  │  │ Subagent  │              │
│  │ (AES-256)│  │ (SQLite+ │  │  Manager  │              │
│  │          │  │  FTS5)   │  │           │              │
│  └──────────┘  └──────────┘  └───────────┘              │
└──────────────────────────────────────────────────────────┘
         │                          │
┌────────▼──────────┐    ┌─────────▼──────────┐
│  pkg/goclaw/      │    │  pkg/goclaw/       │
│  channels/        │    │  gateway/           │
│  ├─ whatsapp/     │    │  (HTTP API,         │
│  ├─ discord/      │    │   OpenAI-compat)    │
│  └─ telegram/     │    └────────────────────┘
└───────────────────┘
         │
┌────────▼──────────┐    ┌────────────────────┐
│  pkg/goclaw/      │    │  pkg/goclaw/       │
│  sandbox/         │    │  scheduler/         │
│  (namespaces,     │    │  (cron + SQLite)    │
│   Docker)         │    └────────────────────┘
└───────────────────┘
```

## Core Components

### 1. Assistant (`assistant.go`)

Entry point for message processing. Responsible for:

- **Message routing**: receives messages from channels, resolves session, dispatches to the agent loop.
- **Media enrichment**: images are described via LLM vision; audio is transcribed via Whisper before reaching the agent.
- **Context compaction**: when the context exceeds the limit, applies one of three strategies (`summarize`, `truncate`, `sliding`).
- **Subagent dispatch**: creates child agents for parallel tasks.

### 2. Agent Loop (`agent.go`)

Agentic loop that orchestrates LLM calls with tool execution:

```
LLM Call → tool_calls? → Execute Tools → Append Results → LLM Call (repeat)
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_turns` | 25 | Maximum LLM round-trips per execution |
| `turn_timeout_seconds` | 300 | Timeout per LLM call |
| `max_continuations` | 2 | Auto-continuations when the budget is exhausted |
| `reflection_enabled` | true | Periodic budget nudges (every 8 turns) |
| `max_compaction_attempts` | 3 | Retries after context overflow |

**Auto-continue flow**: when the agent exhausts its turn budget while still calling tools, it automatically starts a continuation (up to `max_continuations` times).

**Context overflow**: if the LLM returns `context_length_exceeded`, the agent compacts messages (keeps system + recent history), truncates tool results to 2000 chars, and retries.

### 3. LLM Client (`llm.go`)

HTTP client for OpenAI-compatible APIs. Supports multiple providers:

| Provider | Base URL | Key |
|----------|----------|-----|
| OpenAI | `api.openai.com/v1` | `openai` |
| Z.AI (API) | `api.z.ai/api/paas/v4` | `zai` |
| Z.AI (Coding) | `api.z.ai/api/coding/paas/v4` | `zai-coding` |
| Z.AI (Anthropic) | `api.z.ai/api/anthropic` | `zai-anthropic` |
| Anthropic | `api.anthropic.com/v1` | `anthropic` |

**Features**:
- Streaming via SSE (`CompleteWithToolsStream`)
- Prompt caching for Anthropic (`cache_control: {"type": "ephemeral"}`)
- Fallback chain with exponential backoff
- Automatic provider detection from URL
- Per-model defaults (temperature, max tokens, tool support)

### 4. Prompt Composer (`prompt_layers.go`)

8-layer system prompt with priority-based token budget trimming:

| Layer | Priority | Content |
|-------|----------|---------|
| Core | 0 | Base identity, tooling guidance |
| Safety | 5 | Guardrails, boundaries |
| Identity | 10 | Custom instructions |
| Thinking | 12 | Extended thinking hints |
| Bootstrap | 15 | SOUL.md, AGENTS.md, etc. |
| Business | 20 | Workspace context |
| Skills | 40 | Active skill instructions |
| Memory | 50 | Long-term facts |
| Temporal | 60 | Date/time/timezone |
| Conversation | 70 | History (sliding window) |
| Runtime | 80 | System info |

**Trimming rules**:
- System prompt uses at most 40% of the total token budget.
- Layers with priority < 20 (Core, Safety, Identity, Thinking) are never trimmed.
- Layers with priority >= 50 can be dropped entirely if over budget.

### 5. Tool Executor (`tool_executor.go`)

Tool registry and dispatcher with parallel execution:

- **Dynamic registration**: system, skill, and plugin tools are registered in the same registry.
- **Name sanitization**: invalid characters are replaced with `_` via regex.
- **Parallel execution**: independent tools run concurrently (configurable semaphore, default 5).
- **Sequential tools**: `bash`, `write_file`, `edit_file`, `ssh`, `scp`, `exec`, `set_env` always run sequentially.
- **Timeout**: 30s per tool execution (configurable).
- **Session context**: session ID propagated via `context.Value` for goroutine-safe isolation.

### 6. Session Manager (`session.go`, `session_persistence.go`)

Per-chat/group isolation with disk persistence:

```
data/sessions/
├── whatsapp_5511999999999/
│   ├── history.jsonl     # Conversation entries (JSONL)
│   ├── facts.json        # Extracted facts
│   └── meta.json         # Session metadata
```

- **Thread-safety**: `sync.RWMutex` per session.
- **File locks**: file-level locks for persistence.
- **Preventive compaction**: triggers at 80% of the threshold (not 100%).

### 7. Subagent System (`subagent.go`)

Child agents for parallel work:

```
Main Agent ──spawn_subagent──▶ SubagentManager ──goroutine──▶ Child AgentRun
                                    │                              │
                                    ▼                              ▼
                             SubagentRegistry           (isolated session,
                             tracks runs + results       filtered tools,
                                                         separate prompt)
```

- **No recursion**: subagents cannot spawn subagents.
- **Semaphore**: maximum 4 concurrent subagents (default).
- **Timeout**: 300s per subagent.
- **Filtered tools**: deny list removes `spawn_subagent`, `list_subagent`, `wait_subagent`.

## Channels and Gateway

### Channels (`channels/`)

Abstract interface that each channel implements:

- **WhatsApp** (`channels/whatsapp/`): native Go implementation via whatsmeow. Supports text, images, audio, video, documents, stickers, locations, contacts, reactions, typing indicators, read receipts.
- **Discord** and **Telegram**: planned.

### HTTP Gateway (`gateway/`)

OpenAI-compatible REST API:

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/chat/completions` | Chat (supports SSE streaming) |
| GET | `/api/sessions` | List sessions |
| GET/DELETE | `/api/sessions/:id` | Specific session |
| GET | `/api/usage` | Usage statistics |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |
| GET | `/health` | Health check |

## Message Flow

```
1. Channel receives message (WhatsApp / Gateway / CLI)
2. Channel Manager routes to Assistant
3. Message Queue applies debounce (1s) and dedup (5s window)
4. Assistant resolves session (creates or reuses)
5. Media is enriched (vision/whisper if applicable)
6. Prompt Composer builds system prompt (8 layers + trimming)
7. Agent Loop starts:
   a. Calls LLM with context
   b. If tool_calls: ToolExecutor dispatches (parallel/sequential)
   c. ToolGuard validates permissions and blocks dangerous commands
   d. Results are appended to context
   e. Repeats until final response or max_turns
8. Response is formatted (WhatsApp markdown if needed)
9. Message Splitter divides into channel-compatible chunks
10. Block Streamer delivers progressively (if enabled)
11. Session is persisted to disk
```

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.22+ |
| CLI | Cobra + readline |
| Setup | charmbracelet/huh (TUI forms) |
| WhatsApp | whatsmeow (native Go) |
| Database | SQLite (go-sqlite3) with FTS5 |
| Encryption | AES-256-GCM + Argon2id (stdlib + x/crypto) |
| Scheduler | robfig/cron v3 |
| Config | YAML (gopkg.in/yaml.v3) |
| Keyring | go-keyring (GNOME/macOS/Windows) |
| QR Code | mdp/qrterminal |
| Sandbox | Linux namespaces (syscall) / Docker CLI |
