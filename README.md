# DevClaw

> AI agent for tech teams. Single binary. Runs everywhere.

[![Release](https://img.shields.io/github/v/release/jholhewres/devclaw?style=for-the-badge)](https://github.com/jholhewres/devclaw/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![CI](https://img.shields.io/github/actions/workflow/status/jholhewres/devclaw/ci.yml?branch=master&style=for-the-badge&label=build)](https://github.com/jholhewres/devclaw/actions/workflows/ci.yml)

Open-source AI agent for tech teams — devs, DevOps, QA, PMs, designers, and everyone in between. Single Go binary with CLI, WebUI, MCP server, and messaging channels. Full system access, persistent memory, encrypted vault, and 70+ built-in tools.

**Not a chatbot. Not an IDE. Not a framework.** DevClaw is the AI backend that IDEs, terminals, and channels access — giving any tool persistent memory, infrastructure access, and integrations.

[**Docs**](docs/) | [**Getting Started**](#quick-start) | [**Skills**](https://github.com/jholhewres/devclaw-skills) | [**Releases**](https://github.com/jholhewres/devclaw/releases)

---

## Quick Start

### Docker (recommended)

No Go, Node, or build tools required — just Docker:

```bash
git clone https://github.com/jholhewres/devclaw.git && cd devclaw
docker compose up -d
```

Open **http://localhost:8090/setup** to configure your API key and start using DevClaw.

The container includes bash, python, node, git, and other tools the agent needs to execute scripts.

### From Source

Requires **Go 1.24+** and **Node 22+**:

```bash
git clone https://github.com/jholhewres/devclaw.git && cd devclaw
make build                  # builds frontend + Go binary
./bin/devclaw serve         # starts server
```

Open **http://localhost:8090/setup** for the setup wizard, or run `./bin/devclaw setup` for the CLI wizard.

### Go Install

Requires **Go 1.24+** (CGO enabled, SQLite dependency):

```bash
CGO_ENABLED=1 go install -tags 'sqlite_fts5' github.com/jholhewres/devclaw/cmd/devclaw@latest
devclaw serve
```

> **Note:** This builds without the WebUI. For full functionality (dashboard, setup wizard), use Docker or build from source.

### Install Script (macOS/Linux)

Tries binary download, then `go install`, then source build:

```bash
curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/scripts/install/install.sh | bash
```

### Setup Wizard

After starting the server, the setup wizard is available at:

```
http://localhost:8090/setup
```

It guides you through:
1. API provider and key configuration
2. Channel setup (WhatsApp, Discord, Telegram, Slack)
3. Security settings (vault password, access control)
4. Skills installation

All secrets are stored in the encrypted vault (`.devclaw.vault`), never in plain text.

---

## Highlights

- **Single binary** — one `go build`, zero runtime dependencies
- **9 LLM providers** — OpenAI, Anthropic, Ollama, Groq, Google AI, Z.AI, xAI, OpenRouter, and any OpenAI-compatible endpoint
- **N-provider fallback chain** — rate-limited model → fallback → local Ollama, with per-model cooldowns and budget tracking
- **70+ built-in tools** — Git, Docker, databases, testing, deploy, DORA metrics, and much more
- **MCP server** — any IDE (Cursor, VSCode, Claude Code, Windsurf) connects via Model Context Protocol
- **Pipe mode** — `git diff | devclaw diff` or `npm build 2>&1 | devclaw fix`
- **Quick commands** — `devclaw fix`, `devclaw explain .`, `devclaw commit`, `devclaw how "task"`
- **Extensible skills** — install from [devclaw-skills](https://github.com/jholhewres/devclaw-skills) or create your own
- **4 channels** — WhatsApp, Discord, Telegram, Slack
- **WebUI** — React dashboard with SSE streaming, session management, and setup wizard
- **Gateway API** — OpenAI-compatible HTTP API + WebSocket JSON-RPC
- **Encrypted vault** — AES-256-GCM + Argon2id for all secrets
- **Persistent memory** — SQLite FTS5 + vector embeddings for long-term context
- **Daemon manager** — start, monitor, and control background processes (dev servers, watchers)
- **Subagents** — spawn concurrent child agents for parallel tasks (research, code, deploy simultaneously)
- **Plugin system** — GitHub, Jira, Sentry integrations with webhook support
- **Shell hook** — auto-capture failed commands and suggest `devclaw fix`

---

## How It Works

```
┌──────────────────────────────────────────────────────┐
│                    Interfaces                        │
│  CLI   WebUI   WhatsApp   Discord   Telegram   Slack │
└──────────────────────┬───────────────────────────────┘
                       │
        ┌──────────────▼──────────────┐
        │         Assistant           │
        │       (agent loop)          │
        └──┬──────┬──────────┬────┬───┘
           │      │          │    │
    ┌──────▼──┐ ┌─▼──────┐ ┌▼──────────┐
    │  Tools  │ │  LLM   │ │  Memory   │
    │  (70+)  │ │ Client │ │ (SQLite)  │
    └─────────┘ └────────┘ └───────────┘
           │
    ┌──────▼────────────────────────────┐
    │          Subagent Manager         │
    │  (up to 8 concurrent child agents │
    │   with isolated sessions + tools) │
    └───────────────────────────────────┘
           │
    ┌──────▼────────────────────────────────┐
    │            MCP Server                 │
    │  (stdio + SSE — for IDE integration)  │
    └───────────────────────────────────────┘
           │
    ┌──────▼──────────────────────────────┐
    │  Cursor  VSCode  Claude Code  Aider │
    │  OpenCode  Windsurf  Zed  Neovim    │
    └─────────────────────────────────────┘
```

---

## Tools

70+ built-in tools across 20 categories:

| Category | Tools |
|----------|-------|
| **Files** | read, write, edit, list, search, glob |
| **Shell** | bash, environment variables |
| **Git** | status, diff, log, commit, branch, stash, blame (structured JSON) |
| **Docker** | ps, logs, exec, images, compose (up/down/ps/logs), stop, rm |
| **Database** | query, execute, schema, connections (PostgreSQL, MySQL, SQLite) |
| **Dev Utils** | json_format, jwt_decode, regex_test, base64, hash, uuid, url_parse, timestamp |
| **System** | env_info, port_scan, process_list |
| **Codebase** | index (file tree), code_search (ripgrep), symbols, cursor_rules_generate |
| **Testing** | test_run (auto-detect framework), api_test, test_coverage |
| **Ops** | server_health (HTTP/TCP/DNS), deploy_run, tunnel_manage, ssh_exec |
| **Product** | sprint_report, dora_metrics, project_summary |
| **Daemons** | start_daemon, daemon_logs, daemon_list, daemon_stop, daemon_restart |
| **Subagents** | spawn_subagent, list_subagents, wait_subagent, stop_subagent |
| **Plugins** | plugin_list, plugin_install, plugin_call (GitHub, Jira, Sentry) |
| **Team** | team_users (RBAC), shared_memory |
| **IDE** | ide_configure (VSCode, Cursor, JetBrains, Neovim) |
| **Remote** | SSH exec, SCP upload/download |
| **Web** | search, fetch, browser automation |
| **Memory** | save, search, list facts |
| **Scheduler** | create, list, delete cron jobs |
| **Vault** | save, get, list, delete secrets |

---

## MCP Server (IDE Integration)

DevClaw exposes all tools via the [Model Context Protocol](https://modelcontextprotocol.io/), making it a backend for any AI coding tool:

```bash
devclaw mcp serve    # starts MCP server on stdio
```

**Cursor / VSCode** (`.cursor/mcp.json` or `.vscode/mcp.json`):

```json
{
  "mcpServers": {
    "devclaw": {
      "command": "devclaw",
      "args": ["mcp", "serve"]
    }
  }
}
```

**Claude Code** (`.mcp.json`):

```json
{
  "mcpServers": {
    "devclaw": {
      "command": "devclaw",
      "args": ["mcp", "serve"]
    }
  }
}
```

Works with: Cursor, VSCode, Claude Code, OpenCode, Windsurf, Zed, Neovim, and any MCP-compatible client.

---

## CLI Reference

```
devclaw serve                  Start daemon with channels + WebUI
devclaw chat "message"         Single message or interactive REPL
devclaw setup                  Web-based setup wizard
devclaw mcp serve              Start MCP server for IDE integration

devclaw fix [file]             Analyze and fix errors
devclaw explain [path]         Explain code, files, or directories
devclaw diff [--staged]        AI review of git changes
devclaw commit [--dry-run]     Generate commit message and commit
devclaw how "task"             Generate shell commands without executing

devclaw config init            Create default config.yaml
devclaw config vault-init      Initialize encrypted vault
devclaw config vault-set       Store API key in vault
devclaw skill install <name>   Install a skill
devclaw skill list             List installed skills
devclaw schedule list          Show scheduled jobs
devclaw health                 Health check (Docker/monitoring)
devclaw shell-hook bash        Generate shell integration
devclaw completion bash        Generate shell completions
```

**Pipe mode:**

```bash
git diff | devclaw "review this"
npm run build 2>&1 | devclaw fix
cat error.log | devclaw "what went wrong?"
```

---

## Configuration

Minimal `config.yaml`:

```yaml
name: "DevClaw"
trigger: "@devclaw"
model: "gpt-4.1-mini"

api:
  base_url: "https://api.openai.com/v1"
  api_key: "${DEVCLAW_API_KEY}"

webui:
  enabled: true
  address: ":8090"
```

See [`configs/devclaw.example.yaml`](configs/devclaw.example.yaml) for the full reference.

---

## Skills

Extend DevClaw with installable skills:

```bash
devclaw skill install github
devclaw skill install docker
devclaw skill search kubernetes
devclaw skill list
```

Browse the catalog: [devclaw-skills](https://github.com/jholhewres/devclaw-skills)

---

## Subagents

DevClaw can spawn **concurrent child agents** to handle multiple tasks in parallel. The main agent delegates work to subagents, each running in its own goroutine with an isolated session and filtered tool set.

```
Main Agent
  ├── spawn_subagent("research API docs")     → runs concurrently
  ├── spawn_subagent("write unit tests")      → runs concurrently
  └── spawn_subagent("check deploy status")   → runs concurrently
         │
         ▼
  Results announced back to parent when done
```

**How it works:**

- `spawn_subagent` — creates a child agent with a specific task, returns a `run_id` immediately
- `list_subagents` — check status of all running/completed subagents
- `wait_subagent` — block until a subagent finishes and get its result
- `stop_subagent` — cancel a running subagent

**Key properties:**

- Up to **8 concurrent** subagents (configurable)
- Each subagent gets its own **isolated session** and **filtered tools** (no recursion, no memory writes)
- Configurable **timeout** (default: 10min) and optional **model override** per subagent
- Results are **persisted to SQLite** — survive process restarts
- **Push-style announce** — parent is notified immediately when a subagent completes

**Example use cases:**

- Research multiple topics simultaneously while the main agent continues working
- Run tests in background while writing code
- Deploy to staging while generating release notes
- Audit dependencies across multiple languages in parallel

```yaml
# config.yaml
subagents:
  enabled: true
  max_concurrent: 8
  timeout_seconds: 600
  denied_tools:
    - spawn_subagent    # no recursion
    - memory_save       # no memory pollution
    - cron_add          # no scheduling
```

---

## Channels

| Channel | Status | Protocol |
|---------|--------|----------|
| WhatsApp | Stable | whatsmeow (native Go) |
| Discord | Stable | discordgo |
| Telegram | Stable | telebot |
| Slack | Stable | slack-go |

See [docs/channels.md](docs/channels.md) for setup instructions.

---

## Security

- **Encrypted vault** — AES-256-GCM + Argon2id, all secrets encrypted at rest
- **Tool guard** — ACL-based permission system with dangerous command blocking
- **Sandbox** — skill scripts run in isolated environment
- **Audit logging** — all tool executions logged
- **SSRF protection** — web_fetch blocks internal network access
- **Budget tracking** — monthly cost limits with configurable alerts

See [docs/security.md](docs/security.md) for details.

---

## Deployment

**Docker Compose (recommended):**

```bash
docker compose up -d             # start
docker compose logs -f devclaw   # view logs
docker compose down              # stop
```

Data is persisted in a Docker volume (`devclaw-state`). Rebuilds (`docker compose build`) preserve all sessions, memory, and configuration.

To give the agent access to host directories, add bind mounts in `docker-compose.yml`:

```yaml
volumes:
  - ./skills:/home/devclaw/skills
  - ./workspace:/home/devclaw/workspace
  - /path/to/projects:/home/devclaw/projects  # custom mount
```

**systemd (bare metal):**

```bash
make build
sudo cp bin/devclaw /usr/local/bin/
sudo cp devclaw.service /etc/systemd/system/
sudo systemctl enable --now devclaw
```

**PM2:**

```bash
make build
pm2 start ./bin/devclaw --name devclaw -- serve
pm2 save
```

---

## Documentation

| Topic | Link |
|-------|------|
| Architecture | [docs/architecture.md](docs/architecture.md) |
| Features | [docs/features.md](docs/features.md) |
| Security | [docs/security.md](docs/security.md) |
| Performance | [docs/performance.md](docs/performance.md) |
| Skills Catalog | [docs/skills-catalog.md](docs/skills-catalog.md) |

---

## Author

**Jhol Hewres** — [@jholhewres](https://github.com/jholhewres)

## License

[MIT](LICENSE)
