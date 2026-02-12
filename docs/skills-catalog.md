# GoClaw Skills Catalog

Complete catalog of available, planned, and compatible skills for GoClaw.

## Skill Formats

GoClaw supports two skill formats:

| Format | Files | Runtime | Sandbox |
|--------|-------|---------|---------|
| **Native Go** | `skill.yaml` + `skill.go` | Compiled Go, direct execution | Not needed |
| **ClawdHub (SKILL.md)** | `SKILL.md` + `scripts/` | Python, Node.js, Shell | Required |

Native Go skills are faster and have no runtime dependencies. ClawdHub skills give access to the 900+ community skills ecosystem.

---

## Available Skills (Phase 1)

### Builtin

Skills compiled into the default skill set. No external dependencies.

#### weather `v0.2.0`

Weather information and forecasts using free APIs — **no API key required**.

| Tool | Description |
|------|-------------|
| `get_weather` | Current conditions for any location (city, coordinates, airport code) |
| `get_forecast` | Multi-day forecast with hourly data (1-7 days) |
| `get_moon` | Current moon phase, illumination, rise/set times |

**Provider:** [wttr.in](https://wttr.in) (free, no registration)
**Config:** `default_city`, `units` (metric/imperial), `language` (en, pt, es, ...)
**Implementation:** Native Go HTTP client, JSON parsing

#### calculator `v0.1.0`

| Tool | Description |
|------|-------------|
| `calculate` | Evaluate mathematical expressions |
| `convert_units` | Convert between units (km→mi, kg→lb, °C→°F, ...) |

---

### Data

Skills for searching, fetching, and processing information from the web.

#### web-search `v0.1.0`

Web search via Brave Search API or self-hosted SearXNG.

| Tool | Description |
|------|-------------|
| `search` | General web search with freshness filter (day/week/month/year) |
| `search_news` | News-focused search, defaults to past week |

**Providers:**
- [Brave Search API](https://brave.com/search/api/) — requires `BRAVE_API_KEY`
- [SearXNG](https://docs.searxng.org/) — self-hosted, requires `SEARXNG_URL`

**Config:** `provider`, `brave_api_key`, `searxng_url`, `default_count`, `safe_search`
**Implementation:** Native Go HTTP client

#### web-fetch `v0.1.0`

Fetch URL content and extract readable text or markdown.

| Tool | Description |
|------|-------------|
| `fetch` | Fetch URL → text, markdown, or raw HTML. Optional CSS selector. |
| `fetch_headers` | HEAD request, returns HTTP headers |
| `fetch_json` | Fetch JSON API endpoint with jq-like path extraction |

**Config:** `timeout_seconds`, `max_body_bytes`, `user_agent`
**Implementation:** Native Go HTTP client + built-in HTML→text/markdown converter

#### summarize `v0.1.0`

Summarize content from URLs, YouTube videos, podcasts, and files.

| Tool | Description |
|------|-------------|
| `summarize_url` | Summarize articles, blogs, documentation |
| `transcribe_url` | Extract full transcript from YouTube/podcast |
| `summarize_file` | Summarize local files (text, PDF, markdown) |
| `summarize_text` | Summarize provided text directly |

**Requires:** [`summarize`](https://summarize.sh) CLI
**Config:** `default_model`, `default_language`
**Implementation:** CLI wrapper

---

### Development

#### github `v0.2.0`

Full GitHub integration via the `gh` CLI. 17 tools across 6 categories.

**Issues:**

| Tool | Description |
|------|-------------|
| `list_issues` | List with state/label/limit filters |
| `get_issue` | Full details (body, comments, labels, assignees) |
| `create_issue` | Create with title, body, labels, assignees |
| `close_issue` | Close by number |
| `comment_issue` | Add comment |

**Pull Requests:**

| Tool | Description |
|------|-------------|
| `list_prs` | List with state filter |
| `get_pr` | Full details (reviews, checks, stats) |
| `pr_diff` | View code diff |
| `pr_checks` | Check CI status |
| `pr_merge` | Merge (merge/squash/rebase) |

**CI/CD:**

| Tool | Description |
|------|-------------|
| `list_runs` | Recent workflow runs, filter by workflow |
| `view_run` | Run details + optional logs |
| `rerun_workflow` | Re-run failed workflows |

**Releases:**

| Tool | Description |
|------|-------------|
| `list_releases` | List recent releases |
| `create_release` | Create with tag, title, notes, draft/prerelease flags |

**API & Search:**

| Tool | Description |
|------|-------------|
| `api` | Raw `gh api` calls for anything not covered |
| `search_repos` | Search repositories by query |
| `search_code` | Search code across GitHub |

**Requires:** [`gh`](https://cli.github.com/) CLI (authenticated)
**Config:** `default_owner`
**Implementation:** CLI wrapper with JSON output parsing

---

### Productivity

#### gog `v0.1.0`

Google Workspace integration (Gmail, Calendar, Drive) via the `gog` CLI.

**Gmail:**

| Tool | Description |
|------|-------------|
| `gmail_list` | List emails with Gmail search query |
| `gmail_read` | Read specific email by ID |
| `gmail_send` | Send email (to, subject, body, cc) |
| `gmail_reply` | Reply to email thread |

**Calendar:**

| Tool | Description |
|------|-------------|
| `calendar_list` | Upcoming events for N days |
| `calendar_create` | Create event (title, time, duration, attendees) |
| `calendar_delete` | Delete event by ID |

**Drive:**

| Tool | Description |
|------|-------------|
| `drive_list` | List/search files |
| `drive_download` | Download file by ID |
| `drive_upload` | Upload local file |

**Requires:** [`gog`](https://github.com/steipete/gog) CLI
**Config:** `default_account`, `calendar_id`
**Implementation:** CLI wrapper with JSON output parsing

#### calendar `v0.1.0`

Google Calendar integration via credentials JSON.

| Tool | Description |
|------|-------------|
| `list_events` | List events for a date range |
| `create_event` | Create event with title, time, duration |
| `delete_event` | Delete event by ID |

**Config:** `credentials_path`, `calendar_id`
**Implementation:** Native Go (Google Calendar API)

---

## Planned Skills

### Phase 2 — Productivity & Security

| Skill | Category | Description | Requires |
|-------|----------|-------------|----------|
| **1password** | security | Read/inject secrets via 1Password CLI | `op` |
| **obsidian** | productivity | Read/write Obsidian vaults | `obsidian-cli` |
| **notion** | productivity | Notion API (pages, databases, blocks) | API key |
| **trello** | productivity | Trello boards and cards | API key |
| **openai-image-gen** | ai | Batch image generation via OpenAI | `OPENAI_API_KEY` |
| **memory-search** | ai | Semantic search over conversation memory | Embeddings model |
| **todoist** | productivity | Task management | API key |

### Phase 3 — Media & Smart Home

| Skill | Category | Description | Requires |
|-------|----------|-------------|----------|
| **tts** | media | Text-to-speech (Edge TTS / ElevenLabs) | — / API key |
| **openai-whisper-api** | media | Audio transcription via Whisper API | `OPENAI_API_KEY` |
| **video-frames** | media | Extract frames/clips from video | `ffmpeg` |
| **spotify-player** | media | Spotify playback control | `spogo` |
| **openhue** | smart-home | Philips Hue light control | `openhue` |
| **browser** | automation | Web automation via headless browser | `chromedp` / Docker |
| **nano-pdf** | data | PDF reading and editing | `nano-pdf` |
| **gifgrep** | media | GIF search | `gifgrep` |

### Phase 4 — Advanced

| Skill | Category | Description | Requires |
|-------|----------|-------------|----------|
| **blogwatcher** | automation | Blog/RSS monitoring | — |
| **healthcheck** | security | Security audit and hardening | — |
| **tmux** | automation | Remote tmux session control | `tmux` |
| **session-logs** | data | Search/analyze conversation logs | `jq` |
| **review-pr** | development | Automated PR review | `gh` |
| **food-order** | services | Food delivery orders | `ordercli` |
| **goplaces** | services | Google Places search | API key |
| **local-places** | services | Local places via Google proxy | API key |

---

## ClawdHub Compatible Skills

GoClaw can run any skill from [ClawdHub](https://github.com/openclaw/skills) (924 stars, 23K+ commits) via the SKILL.md compatibility layer. Scripts execute inside the [sandbox](../README.md#script-sandbox).

### How it works

```
ClawdHub Skill                GoClaw
─────────────                ────────
SKILL.md       ──parse──►   ClawdHubLoader
  frontmatter  ──validate►   Requirement check (bins, env, OS)
  body         ──inject──►   System prompt for the agent
scripts/       ──discover►   ScriptSkill (implements Skill interface)
  gen.py       ──sandbox─►   Runner → Executor (none/restricted/Docker)
```

### Categories available on ClawdHub

| Category | Examples | Count |
|----------|---------|-------|
| **Productivity** | 1password, apple-notes, obsidian, notion, trello, things-mac | 10+ |
| **Communication** | discord, slack, imessage, bluebubbles | 5+ |
| **Development** | github, coding-agent, skill-creator | 5+ |
| **Media** | summarize, whisper, video-frames, image-gen, gifgrep | 10+ |
| **Smart Home** | openhue, sonos, spotify, eightctl | 5+ |
| **Location** | weather, goplaces, local-places | 3+ |
| **Utilities** | canvas, session-logs, healthcheck, tmux, gemini | 8+ |

### Requirements checking

The loader automatically validates requirements before loading a skill:

```yaml
# From SKILL.md frontmatter
metadata:
  openclaw:
    requires:
      bins: [gh]          # All must be on PATH
      anyBins: [spogo, spotify_player]  # At least one
      env: [OPENAI_API_KEY]  # Must be set
    os: [darwin, linux]   # Supported platforms
```

Skills with unmet requirements are silently skipped. Use `copilot skill list --all` to see skipped skills and their missing dependencies.

---

## Creating a Skill

### Native Go skill

```bash
copilot skill create my-skill --category productivity
```

This scaffolds:

```
skills/skills/productivity/my-skill/
├── skill.yaml    # Metadata, tools, triggers, config
└── skill.go      # Go implementation
```

### Script skill (ClawdHub-compatible)

```
my-skill/
├── SKILL.md       # Frontmatter + instructions
├── scripts/       # Python, Node.js, Shell scripts
│   └── run.py
└── references/    # Optional docs for context
    └── api.md
```

The `SKILL.md` body becomes the agent's system prompt. Scripts referenced with `{baseDir}/scripts/...` are auto-discovered and exposed as tools.

---

## Skill Schema

### skill.yaml (Native Go)

```yaml
name: string          # Required. Pattern: ^[a-z][a-z0-9-]*$
version: string       # Required. Semver: 0.1.0
author: string        # Optional.
description: string   # Required.
category: string      # Required. Enum: builtin, data, development,
                      #   productivity, communication, automation,
                      #   ai, finance, monitoring, community
tags: [string]        # Optional.

requires:             # Optional. Auto-checked on load.
  bins: [string]      #   Binaries that must be on PATH
  any_env: [string]   #   At least one env var must be set

config:               # Optional. JSON Schema for user config.
  type: object
  properties: { ... }

tools:                # Optional. Functions the LLM can call.
  - name: string
    description: string
    parameters:
      param_name:
        type: string | integer | boolean | number
        description: string
        required: boolean
        default: any

system_prompt: string # Optional. Instructions for the agent.

triggers: [string]    # Optional. Activation phrases.
```

### SKILL.md (ClawdHub)

```yaml
---
name: string          # Required.
description: string   # Required.
homepage: string      # Optional.
metadata:             # Optional. Single-line JSON.
  openclaw:
    emoji: string
    always: boolean
    os: [darwin, linux, win32]
    requires:
      bins: [string]
      anyBins: [string]
      env: [string]
    install: [...]    # UI-only, not used by GoClaw
---
# Skill Title

Instructions for the agent...
Scripts referenced as: `{baseDir}/scripts/script.py`
```
