# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build          # Build frontend + Go binary
make build-go       # Build only Go binary (skip frontend)
make build-linux    # Cross-compile for Linux AMD64 (deploy target)

make run            # Start server (uses existing binary)
make build-run      # Build and start server
make dev            # Start Vite dev server + Go server in parallel

make test           # Run unit tests
make test-v         # Run unit tests (verbose)
make lint           # Run golangci-lint
```

Frontend (in `web/`):
```bash
npm run dev         # Vite dev server
npm run build       # Production build
npm run lint        # ESLint
npm run test        # Vitest
```

## Architecture

DevClaw is an AI agent platform in Go with a React frontend. Single binary, zero runtime dependencies.

```
Interfaces: CLI / WebUI / WhatsApp / Discord / Telegram / Slack
     │
     ▼
Assistant (pkg/devclaw/copilot/assistant.go) — message routing, media enrichment
     │
     ▼
Agent Loop (agent.go) — LLM → tool execution cycle (max 25 turns)
     │
     ├─► Tools (90+) — pkg/devclaw/copilot/system_tools.go
     ├─► LLM Client — 9 providers with fallback
     ├─► Memory — SQLite FTS5 + vector embeddings
     └─► Subagents — up to 8 concurrent child agents
```

### Key Packages

- `cmd/devclaw/commands/` — CLI entry points (Cobra)
- `pkg/devclaw/copilot/` — Core agent logic (assistant, agent, tools, sessions)
- `pkg/devclaw/channels/` — Messaging channel integrations
- `pkg/devclaw/gateway/` — HTTP API + WebSocket
- `pkg/devclaw/webui/` — Web interface backend
- `pkg/devclaw/skills/` — Skill system
- `web/src/` — React frontend

## Conventions

### Go

- **Concurrency**: `sync.RWMutex` for read-heavy state, `sync.Mutex` for write-heavy. Always `defer mu.Unlock()` after `mu.Lock()`.
- **Session access**: All session fields must be accessed under `session.mu` lock.
- **Errors**: Wrap with context: `fmt.Errorf("operation: %w", err)`. Don't panic.
- **Tools**: Register in `system_tools.go` via `RegisterSystemTools`. Names in snake_case.
- **Vault**: All secrets in encrypted vault (`.devclaw.vault` in project root. Never put API keys in `.env` or `config.yaml`.

### Frontend

- React 18+ TypeScript, Vite, Tailwind CSS
- SSE for streaming: `createPOSTSSEConnection` (POST) or `createSSEConnection` (GET)
- API calls through `web/src/lib/api.ts`

### Git Commits

- Conventional Commits: `type(scope): description`
- Types: `feat`, `fix`, `refactor`, `perf`, `docs`, `chore`, `ci`, `test`, `build`
- Always in English

## Adding a New Tool

1. Define in `system_tools.go` → `RegisterSystemTools`
2. Add parameter schema (types, descriptions)
3. Implement handler returning `(string, error)`
4. Set permission level (owner/admin/user)

## Output Sanitization

All outgoing text passes through `FormatForChannel` → `StripInternalTags`:
- Strips `[[reply_to_*]]`, `<final>`, `<thinking>`, `<reasoning>` XML tags
- Removes silent tokens: `NO_REPLY`, `HEARTBEAT_OK`

## File Permissions

- Session files, vault, config: `0600`
- Sessions directory: `0700`
