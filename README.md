# GoClaw

**Open-source personal AI assistant in Go.** CLI + messaging channels (WhatsApp, Discord, Telegram). Built on the [AgentGo](https://github.com/jholhewres/agent-go) SDK.

Single binary. Zero runtime dependencies. Cross-compilable.

## Why GoClaw

[OpenClaw](https://github.com/openclaw/openclaw) is an impressive project — 18+ channels, 45+ skills, 188K stars. But it's 52+ modules on Node.js, ~500MB in memory, and ~5s to start.

[NanoClaw](https://github.com/qwibitai/nanoclaw) nailed the philosophy — small, understandable, secure by isolation. But it's still Node.js, still Baileys for WhatsApp, still tied to Claude Code.

GoClaw gives you the same core functionality compiled into a single binary you can `scp` to any server.

| | NanoClaw | OpenClaw | **GoClaw** |
|---|----------|----------|------------|
| Single binary | ❌ Node.js | ❌ Node.js | ✅ |
| Memory footprint | ~200MB | ~500MB | **~30MB** |
| Startup time | ~2s | ~5s | **~50ms** |
| Runtime deps | Node.js 20+ | Node.js 22+ | **None** |
| Cross-compile | ❌ | ❌ | ✅ Linux/Mac/Win/ARM |
| WhatsApp | Baileys (JS) | Baileys (JS) | **Whatsmeow (native Go)** |
| LLM providers | Claude only | 15+ | **15+ (via AgentGo SDK)** |

## Quick Start

### Install

```bash
go install github.com/jholhewres/goclaw/cmd/copilot@latest
```

Or build from source:

```bash
git clone https://github.com/jholhewres/goclaw.git
cd goclaw
make build
./bin/copilot --version
```

### Chat (CLI)

```bash
export OPENAI_API_KEY=sk-...

# Single message
copilot chat "What's on my calendar today?"

# Interactive REPL
copilot chat
```

### Serve (daemon with channels)

```bash
copilot config init
# Edit config.yaml, then:
copilot serve --channel whatsapp
```

A QR code will be displayed on first run to link your WhatsApp.

## Architecture

GoClaw has three layers of extensibility: **Core**, **Plugins**, and **Skills**. Each serves a different purpose.

```
┌──────────────────────────────────────────────────────────────┐
│                           GoClaw                              │
├──────────────────────────────────────────────────────────────┤
│  CLI (cmd/copilot/)                                           │
│  chat · serve · schedule · skill · config · remember · health │
├───────────────┬──────────────────┬───────────────────────────┤
│  Core         │  Plugins (.so)   │  Skills (separate repo)   │
│  Compiled in  │  Loaded at       │  Installed via CLI         │
│  the binary   │  runtime         │  from goclaw-skills        │
│               │                  │                           │
│  ▸ WhatsApp   │  ▸ Extra         │  ▸ Prompt / Soul          │
│  ▸ Hooks      │    channels      │  ▸ Tools for the agent    │
│  ▸ Guardrails │  ▸ Webhooks      │  ▸ Triggers               │
│  ▸ Sessions   │  ▸ Custom        │  ▸ Config schema          │
│  ▸ Scheduler  │    integrations  │  ▸ System prompt inject   │
│  ▸ Prompt     │                  │                           │
│    Composer   │                  │                           │
├───────────────┴──────────────────┴───────────────────────────┤
│  AgentGo SDK (github.com/jholhewres/agent-go)                │
│  Agent · Models · Tools · Memory · Hooks · Guardrails         │
└──────────────────────────────────────────────────────────────┘
```

### Core vs Plugins vs Skills

| | **Core** | **Plugins** | **Skills** |
|---|----------|-------------|-----------|
| **What** | Essential runtime | Optional extensions | Agent capabilities |
| **Where** | Compiled in binary | `.so` loaded at runtime | `goclaw-skills` repo |
| **Examples** | WhatsApp channel, hooks, guardrails, scheduler, prompt composer | Discord channel, Telegram channel, webhooks, custom integrations | Calendar, Gmail, GitHub, weather, calculator |
| **Install** | Always available | Drop `.so` in plugins dir | `copilot skill install` |
| **Contains** | Go code | Go code (plugin interface) | Prompt/soul + tools + triggers + config |
| **Lifecycle** | Starts with the binary | Loaded on startup | Registered in skill registry |

**Why this separation?**

- **Core** = what GoClaw needs to run. WhatsApp is core because it's the primary channel. Hooks and guardrails are core because security is not optional.
- **Plugins** = runtime extensions via Go's native plugin system (`.so`). Add channels, webhooks, or custom integrations without recompiling. Not everyone needs Discord or Telegram — load them only if enabled.
- **Skills** = what the *agent* can do. A skill teaches the LLM new capabilities by injecting prompt instructions, exposing tools, and defining triggers. Skills live in a separate repository and are managed via CLI.

## Connecting to AgentGo SDK

GoClaw **does not reimplement** the AI core — it uses the [AgentGo SDK](https://github.com/jholhewres/agent-go) directly for agent execution, LLM models, tools, and memory.

### Model + Agent

Any provider supported by the AgentGo SDK works out of the box:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/jholhewres/agent-go/pkg/agentgo/agent"
    "github.com/jholhewres/agent-go/pkg/agentgo/models/openai"
    "github.com/jholhewres/agent-go/pkg/agentgo/tools/calculator"
    "github.com/jholhewres/agent-go/pkg/agentgo/tools/toolkit"
)

func main() {
    // 1. Create a model (any AgentGo provider)
    model, _ := openai.New("gpt-4o-mini", openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })

    // 2. Create an agent with tools
    ag, _ := agent.New(agent.Config{
        Name:         "Copilot",
        Model:        model,
        Toolkits:     []toolkit.Toolkit{calculator.New()},
        Instructions: "You are a helpful assistant. Be concise.",
    })

    // 3. Run
    output, _ := ag.Run(context.Background(), "What is 42 * 58?")
    fmt.Println(output.Content)
}
```

### Supported Providers

| Provider | Import | Example model |
|----------|--------|---------------|
| OpenAI | `models/openai` | `gpt-4o-mini`, `gpt-4o` |
| Anthropic | `models/anthropic` | `claude-3-5-sonnet-20241022` |
| Google Gemini | `models/gemini` | `gemini-1.5-pro` |
| Ollama (local) | `models/ollama` | `llama2`, `mistral` |
| DeepSeek | `models/deepseek` | `deepseek-chat` |
| Groq | `models/groq` | `llama-3.1-70b-versatile` |
| Together | `models/together` | `meta-llama/Llama-3-70b` |
| OpenRouter | `models/openrouter` | Any model |
| LM Studio | `models/lmstudio` | Local models |

```go
// Anthropic Claude
model, _ := anthropic.New("claude-3-5-sonnet-20241022", anthropic.Config{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
})

// Ollama (local, no API key)
model, _ := ollama.New("llama2", ollama.Config{
    BaseURL: "http://localhost:11434",
})
```

### Tools

GoClaw inherits all 20+ built-in tools from AgentGo. Custom tools can be created inline or packaged as skills:

```go
import (
    "github.com/jholhewres/agent-go/pkg/agentgo/tools/calculator"
    "github.com/jholhewres/agent-go/pkg/agentgo/tools/websearch"
    "github.com/jholhewres/agent-go/pkg/agentgo/tools/toolkit"
)

// Built-in AgentGo tools
toolkits := []toolkit.Toolkit{
    calculator.New(),
    websearch.New(websearch.Config{APIKey: "..."}),
}

// Custom inline tool
myTool := toolkit.NewBaseToolkit("my_tool")
myTool.RegisterFunction(&toolkit.Function{
    Name:        "get_price",
    Description: "Get the current price of a product",
    Parameters: map[string]toolkit.Parameter{
        "product": {Type: "string", Description: "Product name", Required: true},
    },
    Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
        product := args["product"].(string)
        return fmt.Sprintf("The price of %s is $99.90", product), nil
    },
})
```

### Memory

```go
import "github.com/jholhewres/agent-go/pkg/agentgo/memory"

// Simple in-memory (default)
mem := memory.NewInMemory(100)

// Hybrid memory with vector DB for RAG
hybridMem := memory.NewHybridMemory(memory.HybridConfig{
    ShortTermSize: 50,
    VectorDB:      chromaDB,
    Embeddings:    embeddingFunc,
})
```

### Hooks & Guardrails

GoClaw has its own hook system on top of the AgentGo SDK hooks. Core hooks run at the GoClaw level (message routing, session management, security). AgentGo hooks run at the agent execution level (pre/post LLM call, tool calls).

```go
import (
    "github.com/jholhewres/agent-go/pkg/agentgo/hooks"
    "github.com/jholhewres/agent-go/pkg/agentgo/guardrails"
)

// Pre-execution logging hook (AgentGo level)
preHook := hooks.HookFunc(func(ctx context.Context, input *hooks.HookInput) error {
    log.Printf("Input: %s", input.Input)
    return nil
})

// Prompt injection guardrail (AgentGo level)
injectionGuard := guardrails.NewPromptInjectionGuardrail()

ag, _ := agent.New(agent.Config{
    Model:     model,
    PreHooks:  []hooks.Hook{injectionGuard, preHook},
    PostHooks: []hooks.Hook{urlValidator},
})
```

GoClaw-level hooks (applied before the message reaches the agent):

| Hook | Stage | Purpose |
|------|-------|---------|
| Input guardrail | Before agent | Rate limit, injection detection, max length |
| Session hook | Before agent | Load/create session, apply per-chat config |
| Prompt composer | Before agent | Build 8-layer prompt with token budget |
| Output guardrail | After agent | Leak detection, PII check, empty fallback |
| Memory hook | After agent | Store conversation, extract facts |

## Channels

### WhatsApp (core)

Uses [whatsmeow](https://go.mau.fi/whatsmeow) — native Go, no Node.js or Baileys. Compiled into the binary.

```yaml
# config.yaml
channels:
  whatsapp:
    enabled: true
    session_dir: "./sessions/whatsapp"
    trigger: "@copilot"
```

```
User: @copilot how many meetings do I have today?
Copilot: You have 3 meetings today:
         • 9am - Team standup
         • 2pm - Client presentation
         • 4pm - Code review

User: @copilot remind me to call John at 3pm
Copilot: ✅ Reminder set for 3pm: "call John"
```

### Discord (plugin)

Uses [discordgo](https://github.com/bwmarrin/discordgo). Loaded as a Go plugin at runtime.

```yaml
channels:
  discord:
    enabled: true
    token: "${DISCORD_TOKEN}"
    trigger: "!copilot"
```

### Telegram (plugin)

Uses [telego](https://github.com/mymmrac/telego). Loaded as a Go plugin at runtime.

```yaml
channels:
  telegram:
    enabled: true
    token: "${TELEGRAM_TOKEN}"
    trigger: "/copilot"
```

### Adding a channel via plugin

Plugins implement the `Channel` interface and are compiled as `.so` files:

```go
// my_channel_plugin.go
package main

import "github.com/jholhewres/goclaw/pkg/goclaw/channels"

type MyChannel struct { /* ... */ }

func (c *MyChannel) Name() string                                           { return "mychannel" }
func (c *MyChannel) Connect(ctx context.Context) error                      { /* ... */ }
func (c *MyChannel) Disconnect() error                                      { /* ... */ }
func (c *MyChannel) Send(ctx context.Context, to string, msg *channels.OutgoingMessage) error { /* ... */ }
func (c *MyChannel) Receive() <-chan *channels.IncomingMessage              { /* ... */ }
func (c *MyChannel) IsConnected() bool                                      { /* ... */ }
func (c *MyChannel) Health() channels.HealthStatus                          { /* ... */ }

// Exported symbol for plugin loading
var Channel channels.Channel = &MyChannel{}
```

```bash
# Build as plugin
go build -buildmode=plugin -o plugins/mychannel.so my_channel_plugin.go
```

## Skills

Skills teach the agent new capabilities. Unlike plugins (which extend the runtime), skills extend the *agent's knowledge and tools*. They live in a separate repository ([goclaw-skills](https://github.com/jholhewres/goclaw-skills)) and are managed via CLI.

GoClaw supports **two skill formats**:

| Format | Origin | Files | Runtime |
|--------|--------|-------|---------|
| **Native Go** | GoClaw | `skill.yaml` + `skill.go` | Compiled Go |
| **SKILL.md** | [ClawdHub](https://github.com/openclaw/skills) | `SKILL.md` + `scripts/` | Python, Node.js, Shell (sandboxed) |

### Skills Catalog

#### Builtin

| Skill | Tools | Description | API Key |
|-------|-------|-------------|---------|
| **weather** | `get_weather`, `get_forecast`, `get_moon` | Weather via [wttr.in](https://wttr.in) + Open-Meteo | No |
| **calculator** | `calculate`, `convert_units` | Math expressions and unit conversions | No |

#### Data

| Skill | Tools | Description | Requires |
|-------|-------|-------------|----------|
| **web-search** | `search`, `search_news` | Web search via Brave Search API or SearXNG | `BRAVE_API_KEY` or `SEARXNG_URL` |
| **web-fetch** | `fetch`, `fetch_headers`, `fetch_json` | Fetch URLs, extract text/markdown, parse JSON | — |
| **summarize** | `summarize_url`, `transcribe_url`, `summarize_file`, `summarize_text` | Summarize articles, YouTube, podcasts, files | `summarize` CLI |

#### Development

| Skill | Tools | Description | Requires |
|-------|-------|-------------|----------|
| **github** | 17 tools: issues (CRUD), PRs (list/view/diff/checks/merge), CI/CD (runs/rerun), releases, search, raw API | Full GitHub integration | `gh` CLI |

#### Productivity

| Skill | Tools | Description | Requires |
|-------|-------|-------------|----------|
| **gog** | Gmail (list/read/send/reply), Calendar (list/create/delete), Drive (list/download/upload) | Google Workspace | `gog` CLI |
| **calendar** | `list_events`, `create_event`, `delete_event` | Google Calendar | Credentials JSON |

#### ClawdHub Community (900+ skills)

GoClaw can run any skill from the [ClawdHub repository](https://github.com/openclaw/skills) — the community skills archive with 924 stars and 23K+ commits. These skills use Python, Node.js, and Shell scripts, executed inside GoClaw's [script sandbox](#script-sandbox).

```bash
# Install a ClawdHub skill
copilot skill install --from clawdhub 1password

# List all available ClawdHub skills
copilot skill search --source clawdhub
```

See the full [Skills Catalog](docs/skills-catalog.md) for the complete list and roadmap.

### What a skill contains

| Component | Purpose | Required |
|-----------|---------|----------|
| **Prompt / Soul** | Instructions injected into the system prompt | Yes |
| **Tools** | Functions the LLM can call | Optional |
| **Triggers** | Natural language patterns that activate the skill | Optional |
| **Config schema** | User-configurable parameters | Optional |
| **Metadata** | Name, version, author, category, tags | Yes |
| **Requirements** | Binaries, env vars, OS (auto-checked) | Optional |

### CLI Management

```bash
# Search available skills
copilot skill search calendar

# Install a skill (native Go)
copilot skill install calendar

# Install from ClawdHub (Python/JS/Shell, sandboxed)
copilot skill install --from clawdhub openai-image-gen

# List installed
copilot skill list

# Update all
copilot skill update --all

# Create new skill (scaffold)
copilot skill create my-skill
```

### Native Go skill (skill.yaml + skill.go)

```yaml
# skill.yaml
name: calendar
version: 1.0.0
author: goclaw
description: Google Calendar integration
category: productivity
tags: [calendar, google, scheduling]

config:
  type: object
  properties:
    credentials_path:
      type: string
    calendar_id:
      type: string
      default: primary

tools:
  - name: list_events
    description: List calendar events for a date range
    parameters:
      start_date: { type: string, required: true }
      end_date:   { type: string, required: true }

  - name: create_event
    description: Create a new calendar event
    parameters:
      title:    { type: string, required: true }
      start_time: { type: string, required: true }
      duration_minutes: { type: integer, default: 60 }

system_prompt: |
  You have access to the user's Google Calendar.
  When asked about schedule, use list_events.
  When asked to schedule something, use create_event.
  Always confirm before deleting events.

triggers:
  - "what's on my calendar"
  - "schedule a meeting"
  - "check my schedule"
```

```go
// skill.go
package calendar

import (
    "context"
    "github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

type CalendarSkill struct {
    calendarID string
}

func (s *CalendarSkill) Metadata() skills.Metadata {
    return skills.Metadata{
        Name: "calendar", Version: "1.0.0",
        Description: "Google Calendar integration",
        Category: "productivity",
    }
}

func (s *CalendarSkill) Tools() []skills.Tool {
    return []skills.Tool{{
        Name:        "list_events",
        Description: "List calendar events for a date range",
        Parameters: []skills.ToolParameter{
            {Name: "start_date", Type: "string", Required: true},
            {Name: "end_date", Type: "string", Required: true},
        },
        Handler: s.listEvents,
    }}
}

func (s *CalendarSkill) listEvents(ctx context.Context, args map[string]any) (any, error) {
    // Google Calendar API call here
    return map[string]any{"events": []string{}}, nil
}
```

### ClawdHub skill (SKILL.md + scripts/)

GoClaw reads the same `SKILL.md` format used by OpenClaw/ClawdHub. Scripts run in the [sandbox](#script-sandbox).

```markdown
---
name: openai-image-gen
description: Batch-generate images via OpenAI Images API.
metadata: { "openclaw": { "emoji": "...", "requires": { "bins": ["python3"], "env": ["OPENAI_API_KEY"] } } }
---
# OpenAI Image Gen
Run: `python3 {baseDir}/scripts/gen.py --prompt "a sunset" --count 4`
```

The `{baseDir}` token is automatically replaced with the skill's directory path at runtime.

## Script Sandbox

GoClaw includes a multi-level sandbox for secure execution of Python, Node.js, and Shell scripts from community skills. This is what enables running [ClawdHub](https://github.com/openclaw/skills) skills safely.

### Isolation Levels

| Level | How | Use Case |
|-------|-----|----------|
| **none** | Direct `exec.Command` | Trusted/builtin skills |
| **restricted** | Linux namespaces (PID, mount, network, user) | Community skills |
| **container** | Docker with purpose-built image | Untrusted scripts |

### Security Layers

```
Script → Policy Check → Env Filter → Scanner → Executor
           │                │            │
           ├─ Allowlist      ├─ Blocks    ├─ Detects eval(),
           │  validation     │  injection │  reverse shells,
           └─ Requirement    │  vectors   │  crypto mining,
              check          │  (LD_*,    │  obfuscated code
                             │  NODE_*,   └─ data exfiltration
                             │  PYTHON*)
                             └─ Strips dangerous env vars
```

**Environment filtering** blocks injection vectors: `NODE_OPTIONS`, `PYTHONPATH`, `LD_PRELOAD`, `BASH_ENV`, `DYLD_INSERT_LIBRARIES`, and 10+ more.

**Script scanning** detects dangerous patterns before execution:

| Rule | Severity | Detects |
|------|----------|---------|
| `python-exec` | Critical | `exec()`, `eval()` in Python |
| `subprocess-shell` | Critical | `subprocess.run(shell=True)` |
| `node-eval` | Critical | `eval()`, `new Function()` |
| `reverse-shell` | Critical | `/dev/tcp/`, `nc -e`, socket.connect |
| `crypto-mining` | Critical | stratum+tcp, coinhive, xmrig |
| `exfiltration` | Warn | Access to `/etc/passwd`, `.ssh/` |
| `obfuscation` | Warn | Hex-encoded strings, base64+exec |

### Sandbox Configuration

```yaml
# config.yaml
sandbox:
  default_isolation: restricted   # none, restricted, container
  timeout: 60s
  max_memory_mb: 256
  max_cpu_percent: 50
  max_output_bytes: 1048576       # 1MB
  allow_network: false
  docker:
    image: goclaw-sandbox:latest
    build_on_start: true
    network: none
```

### Docker Sandbox Image

The container executor auto-builds a Debian image with Python 3, Node.js, and common tools:

```bash
# Build manually
docker build -t goclaw-sandbox -f Dockerfile.sandbox .

# Or let GoClaw build it on first use (docker.build_on_start: true)
```

The container runs as non-root, with `--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges`, and `--network none` by default.

## Scheduler

Schedule recurring tasks with cron expressions:

```bash
# Daily briefing at 9am on weekdays via WhatsApp
copilot schedule add "0 9 * * 1-5" "Send me a daily briefing" --channel whatsapp

# Weekly summary every Friday at 5pm
copilot schedule add "0 17 * * 5" "Generate a weekly summary" --channel whatsapp

# List schedules
copilot schedule list

# Remove
copilot schedule remove <id>
```

## Security

GoClaw applies guardrails at every stage of the message flow:

| Stage | Protection |
|-------|-----------|
| **Input** | Rate limiting (sliding window per user), prompt injection detection, max input length |
| **Session** | Isolated per chat/group, auto-pruning of inactive sessions |
| **Prompt** | 8-layer system with token budget, no unbounded context |
| **Tools** | Whitelist per skill, confirmation for destructive actions |
| **Scripts** | Multi-level sandbox (namespaces/Docker), env filtering, content scanning |
| **Output** | System prompt leak detection, empty response fallback |
| **Deploy** | systemd hardening (ProtectSystem, PrivateTmp, MemoryMax) |

## Configuration

```yaml
# config.yaml
assistant:
  name: "Copilot"
  trigger: "@copilot"
  model: "gpt-4o-mini"          # Any AgentGo model
  timezone: "America/Sao_Paulo"
  language: "en"
  instructions: |
    You are a helpful personal assistant.
    Be concise and practical.

channels:
  whatsapp:
    enabled: true
    session_dir: "./sessions/whatsapp"
  discord:
    enabled: false
    token: "${DISCORD_TOKEN}"
  telegram:
    enabled: false
    token: "${TELEGRAM_TOKEN}"

memory:
  type: "sqlite"                # sqlite, postgres, memory
  path: "./data/memory.db"
  max_messages: 100
  compression_strategy: "summarize"  # summarize, truncate, semantic

security:
  max_input_length: 4096
  rate_limit: 30                # messages/min/user
  enable_pii_detection: false
  enable_url_validation: true

scheduler:
  enabled: true
  storage: "./data/scheduler.db"

plugins:
  dir: "./plugins"              # Directory for .so plugin files

skills:
  builtin:
    - weather
    - calculator
    - web-search
    - web-fetch
  installed:
    - github
    - gog
    - summarize
  clawdhub_dirs:                # Directories with SKILL.md format skills
    - "./skills/clawdhub"

sandbox:
  default_isolation: restricted # none, restricted, container
  timeout: 60s
  max_memory_mb: 256
  allow_network: false
  docker:
    image: goclaw-sandbox:latest
    build_on_start: true
```

## Deploy

### Docker

```bash
docker compose up -d
docker compose logs -f copilot
```

### systemd

```bash
sudo cp copilot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now copilot
```

### Direct binary

```bash
make build
./bin/copilot serve --config config.yaml
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `copilot chat [msg]` | Interactive chat or single message |
| `copilot serve` | Start daemon with messaging channels |
| `copilot schedule list` | List scheduled tasks |
| `copilot schedule add <cron> <cmd>` | Add a scheduled task |
| `copilot schedule remove <id>` | Remove a scheduled task |
| `copilot skill list` | List installed skills |
| `copilot skill search <query>` | Search available skills |
| `copilot skill install <name>` | Install a skill |
| `copilot skill update [name\|--all]` | Update skills |
| `copilot config init` | Create default config |
| `copilot config show` | Show current config |
| `copilot config set <key> <value>` | Set a config value |
| `copilot remember <fact>` | Save a fact to long-term memory |
| `copilot health` | Check service health |

## Project Structure

```
goclaw/
├── cmd/copilot/                # CLI application
│   ├── main.go
│   └── commands/               # Cobra commands (chat, serve, skill, ...)
├── pkg/goclaw/
│   ├── channels/               # Channel interface + Manager (core)
│   ├── copilot/                # Assistant + Prompt + Session (core)
│   │   └── security/           # I/O guardrails (core)
│   ├── sandbox/                # Script sandbox (multi-level isolation)
│   │   ├── sandbox.go          #   Interface, config, types
│   │   ├── runner.go           #   Main runner (dispatch + policy)
│   │   ├── policy.go           #   Security policy (allowlist, scanner)
│   │   ├── exec_direct.go      #   IsolationNone executor
│   │   ├── exec_restricted.go  #   Linux namespace executor
│   │   └── exec_docker.go      #   Docker container executor
│   ├── skills/                 # Skill system (core)
│   │   ├── skill.go            #   Skill interface + types
│   │   ├── registry.go         #   Registry + index + loaders
│   │   ├── clawdhub_loader.go  #   ClawdHub SKILL.md parser
│   │   └── script_skill.go     #   Script→Skill adapter
│   ├── plugins/                # Plugin loader (core)
│   └── scheduler/              # Cron-based job scheduling (core)
├── plugins/                    # Plugin .so files (loaded at runtime)
├── skills/                     # Submodule → goclaw-skills
│   ├── skills/
│   │   ├── builtin/            #   weather, calculator
│   │   ├── data/               #   web-search, web-fetch, summarize
│   │   ├── development/        #   github
│   │   ├── productivity/       #   gog, calendar
│   │   └── communication/      #   gmail
│   ├── schemas/                #   skill.schema.json
│   ├── templates/              #   Scaffolding templates
│   └── index.yaml              #   Skills catalog
├── configs/                    # Example configs
├── docs/                       # Plans, specs, skills catalog
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── go.mod
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| [agent-go](https://github.com/jholhewres/agent-go) | Agent SDK (models, tools, memory, hooks) |
| [whatsmeow](https://go.mau.fi/whatsmeow) | WhatsApp channel (native Go, core) |
| [discordgo](https://github.com/bwmarrin/discordgo) | Discord channel (plugin) |
| [telego](https://github.com/mymmrac/telego) | Telegram channel (plugin) |
| [cobra](https://github.com/spf13/cobra) | CLI framework |
| [cron](https://github.com/robfig/cron) | Task scheduler |

No external dependencies for the sandbox — it uses Go's `os/exec`, `syscall` (Linux namespaces), and Docker CLI.

## Roadmap

- [x] Core scaffolding: channels, skills, scheduler, assistant, security
- [x] CLI: chat, serve, schedule, skill, config, remember, health
- [x] Prompt composer with 8 layers and token budget
- [x] Security guardrails (input + output + tool policy)
- [x] Session isolation with auto-pruning
- [x] Docker + systemd + Makefile
- [x] Skills repository as submodule
- [x] 10+ skills: weather, calculator, github, web-search, web-fetch, summarize, gog, calendar
- [x] Script sandbox (none / Linux namespaces / Docker)
- [x] ClawdHub SKILL.md compatibility layer
- [x] Script security: env filtering, content scanning, allowlisting
- [ ] WhatsApp channel implementation (whatsmeow, core)
- [ ] Plugin loader system (Go native plugins)
- [ ] Discord channel as plugin
- [ ] Telegram channel as plugin
- [ ] Webhook plugin
- [ ] Full AgentGo SDK integration (agent.Run in message loop)
- [ ] Memory persistence (SQLite)
- [ ] RAG with embeddings
- [ ] Filesystem skill loader (auto-discover skills from disk)
- [ ] `copilot skill install --from clawdhub` implementation
- [ ] Phase 2 skills: 1password, obsidian, notion, openai-image-gen, memory-search
- [ ] Phase 3 skills: tts, whisper, video-frames, spotify, openhue, browser
- [ ] Web dashboard
- [ ] Multi-agent teams

## License

MIT
