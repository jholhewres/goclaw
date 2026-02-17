# DevClaw

> AI agent for tech teams. Single binary. Runs everywhere.

[![Release](https://img.shields.io/github/v/release/jholhewres/devclaw?style=for-the-badge)](https://github.com/jholhewres/devclaw/releases)
[![License](https://img.shields.io/github/license/jholhewres/devclaw?style=for-the-badge)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![CI](https://img.shields.io/github/actions/workflow/status/jholhewres/devclaw/release.yml?style=for-the-badge)](https://github.com/jholhewres/devclaw/actions)

Open-source AI agent for tech teams — devs, DevOps, QA, PMs, designers, and everyone in between. Single Go binary with CLI, WebUI, and messaging channels. Full system access, persistent memory, encrypted vault, and 40+ built-in tools.

[**Docs**](docs/) | [**Getting Started**](#quick-start) | [**Skills**](https://github.com/jholhewres/devclaw-skills) | [**Releases**](https://github.com/jholhewres/devclaw/releases)

---

## Quick Start

**One-liner (macOS/Linux):**

```bash
curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/main/scripts/install/install.sh | bash
```

**Go install:**

```bash
go install github.com/jholhewres/devclaw/cmd/devclaw@latest
devclaw setup
devclaw serve
```

**From source:**

```bash
git clone https://github.com/jholhewres/devclaw.git && cd devclaw
make build
./bin/devclaw serve    # setup wizard starts at http://localhost:8090/setup
```

**Docker:**

```bash
docker run -d -p 8090:8090 -p 8080:8080 ghcr.io/jholhewres/devclaw serve
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/jholhewres/devclaw/main/scripts/install/install.ps1 | iex
```

---

## Highlights

- **Single binary** — one `go build`, zero runtime dependencies
- **9 LLM providers** — OpenAI, Anthropic, Ollama, Groq, Google AI, Z.AI, xAI, OpenRouter, and any OpenAI-compatible endpoint
- **Automatic fallback** — rate-limited model → fallback → local Ollama, with cooldown probing
- **40+ built-in tools** — file I/O, bash, SSH, web search, browser automation, memory, scheduler, vault
- **Extensible skills** — install from [devclaw-skills](https://github.com/jholhewres/devclaw-skills) or create your own
- **4 channels** — WhatsApp, Discord, Telegram, Slack
- **WebUI** — React dashboard with SSE streaming, session management, and setup wizard
- **Gateway API** — OpenAI-compatible HTTP API + WebSocket JSON-RPC
- **Encrypted vault** — AES-256-GCM + Argon2id for all secrets
- **Persistent memory** — SQLite FTS5 + vector embeddings for long-term context
- **Sandboxed execution** — skill scripts run in isolated sandbox
- **Tool guard** — ACL-based access control with audit logging

---

## How It Works

```
                    ┌─────────────┐
                    │   Channels  │  WhatsApp, Discord, Telegram, Slack
                    └──────┬──────┘
                           │
┌──────┐  ┌────────┐  ┌───▼───────────┐  ┌──────────┐
│  CLI │──│ WebUI  │──│   Assistant   │──│ Sessions │
└──────┘  └────────┘  │  (agent loop) │  │ (SQLite) │
                      └───┬───────────┘  └──────────┘
                          │
              ┌───────────┼───────────┐
              │           │           │
         ┌────▼────┐ ┌───▼───┐ ┌────▼────┐
         │  Tools  │ │  LLM  │ │ Memory  │
         │  (40+)  │ │Client │ │ (FTS5)  │
         └─────────┘ └───────┘ └─────────┘
```

---

## Configuration

Minimal `config.yaml`:

```yaml
name: "DevClaw"
trigger: "@devclaw"
model: "gpt-5-mini"

api:
  base_url: "https://api.openai.com/v1"
  api_key: "${DEVCLAW_API_KEY}"

webui:
  enabled: true
  address: ":8090"
```

See [`configs/devclaw.example.yaml`](configs/devclaw.example.yaml) for the full reference.

---

## Tools

40+ built-in tools across 12 categories:

| Category | Tools |
|----------|-------|
| **Files** | read, write, edit, list, search, glob |
| **Shell** | bash, environment variables |
| **Remote** | SSH exec, SCP upload/download |
| **Web** | search, fetch, browser automation |
| **Memory** | save, search, list facts |
| **Scheduler** | create, list, delete cron jobs |
| **Vault** | save, get, list, delete secrets |
| **Sessions** | list, inspect, manage conversations |
| **Canvas** | interactive HTML/JS canvas |
| **Subagents** | spawn specialized sub-agents |
| **Media** | image description, audio transcription |
| **Admin** | config reload, health check |

See [docs/tools.md](docs/tools.md) for the complete reference.

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

See [docs/security.md](docs/security.md) for details.

---

## CLI Reference

```
devclaw serve                  Start daemon with channels + WebUI
devclaw chat "message"         Single message or interactive REPL
devclaw setup                  Web-based setup wizard
devclaw config init            Create default config.yaml
devclaw config vault-init      Initialize encrypted vault
devclaw config vault-set       Store API key in vault
devclaw skill install <name>   Install a skill
devclaw skill list             List installed skills
devclaw schedule list          Show scheduled jobs
devclaw health                 Health check (Docker/monitoring)
devclaw completion bash        Generate shell completions
```

---

## Deployment

**Docker Compose:**

```bash
docker compose up -d
docker compose logs -f devclaw
```

**systemd:**

```bash
sudo cp devclaw.service /etc/systemd/system/
sudo systemctl enable --now devclaw
```

**From source:**

```bash
make build && ./bin/devclaw serve
```

---

## Documentation

| Topic | Link |
|-------|------|
| Architecture | [docs/architecture.md](docs/architecture.md) |
| Security | [docs/security.md](docs/security.md) |
| Features | [docs/features.md](docs/features.md) |
| Skills Catalog | [docs/skills-catalog.md](docs/skills-catalog.md) |
| Performance | [docs/performance.md](docs/performance.md) |

---

## Author

**Joel Holhewres** — [@jholhewres](https://github.com/jholhewres)

## License

[MIT](LICENSE)
