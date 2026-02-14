# GoClaw — Arquitetura Técnica

Documentação técnica da arquitetura interna do GoClaw, cobrindo componentes, fluxos de dados e decisões de design.

## Visão Geral

GoClaw é um framework de assistente AI escrito em Go. Binário único, sem dependências de runtime. Suporta CLI interativo e canais de mensageria (WhatsApp, com Discord/Telegram planejados).

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

## Componentes Principais

### 1. Assistant (`assistant.go`)

Ponto de entrada para processamento de mensagens. Responsável por:

- **Roteamento de mensagens**: recebe mensagens dos canais, resolve sessão, despacha para o agent loop.
- **Enriquecimento de mídia**: imagens são descritas via LLM vision; áudios transcritos via Whisper antes de chegar ao agente.
- **Compactação de contexto**: quando o contexto excede o limite, aplica uma das três estratégias (`summarize`, `truncate`, `sliding`).
- **Despacho de subagents**: cria agentes filhos para tarefas paralelas.

### 2. Agent Loop (`agent.go`)

Loop agentic que orquestra chamadas LLM com execução de tools:

```
LLM Call → tool_calls? → Execute Tools → Append Results → LLM Call (repeat)
```

| Parâmetro | Default | Descrição |
|-----------|---------|-----------|
| `max_turns` | 25 | Máximo de round-trips LLM por execução |
| `turn_timeout_seconds` | 300 | Timeout por chamada LLM |
| `max_continuations` | 2 | Auto-continuações quando o budget se esgota |
| `reflection_enabled` | true | Nudges periódicos de budget (a cada 8 turnos) |
| `max_compaction_attempts` | 3 | Retentativas após overflow de contexto |

**Fluxo de auto-continue**: quando o agente esgota o budget de turnos enquanto ainda chama tools, automaticamente inicia uma continuação (até `max_continuations` vezes).

**Context overflow**: se o LLM retorna `context_length_exceeded`, o agent compacta mensagens (mantém system + histórico recente), trunca resultados de tools para 2000 chars, e retenta.

### 3. LLM Client (`llm.go`)

Cliente HTTP para APIs compatíveis com OpenAI. Suporta múltiplos providers:

| Provider | Base URL | Chave |
|----------|----------|-------|
| OpenAI | `api.openai.com/v1` | `openai` |
| Z.AI (API) | `api.z.ai/api/paas/v4` | `zai` |
| Z.AI (Coding) | `api.z.ai/api/coding/paas/v4` | `zai-coding` |
| Z.AI (Anthropic) | `api.z.ai/api/anthropic` | `zai-anthropic` |
| Anthropic | `api.anthropic.com/v1` | `anthropic` |

**Features**:
- Streaming via SSE (`CompleteWithToolsStream`)
- Prompt caching para Anthropic (`cache_control: {"type": "ephemeral"}`)
- Fallback chain com backoff exponencial
- Detecção automática de provider pela URL
- Defaults por modelo (temperatura, max tokens, suporte a tools)

### 4. Prompt Composer (`prompt_layers.go`)

Sistema de prompt de 8 camadas com trimming baseado em prioridade e budget de tokens:

| Camada | Prioridade | Conteúdo |
|--------|-----------|----------|
| Core | 0 | Identidade base, guia de tooling |
| Safety | 5 | Guardrails, limites |
| Identity | 10 | Instruções customizadas |
| Thinking | 12 | Hints de extended thinking |
| Bootstrap | 15 | SOUL.md, AGENTS.md, etc. |
| Business | 20 | Contexto de workspace |
| Skills | 40 | Instruções de skills ativas |
| Memory | 50 | Fatos de longo prazo |
| Temporal | 60 | Data/hora/timezone |
| Conversation | 70 | Histórico (sliding window) |
| Runtime | 80 | Info do sistema |

**Regras de trimming**:
- System prompt usa no máximo 40% do budget total de tokens.
- Camadas com prioridade < 20 (Core, Safety, Identity, Thinking) nunca são cortadas.
- Camadas com prioridade >= 50 podem ser descartadas completamente se over budget.

### 5. Tool Executor (`tool_executor.go`)

Registry e dispatcher de tools com execução paralela:

- **Registro dinâmico**: tools de system, skills e plugins são registrados no mesmo registry.
- **Sanitização de nomes**: caracteres inválidos são substituídos por `_` via regex.
- **Execução paralela**: tools independentes rodam concorrentemente (semáforo configurável, default 5).
- **Tools sequenciais**: `bash`, `write_file`, `edit_file`, `ssh`, `scp`, `exec`, `set_env` sempre rodam sequencialmente.
- **Timeout**: 30s por execução de tool (configurável).
- **Contexto de sessão**: session ID propagado via `context.Value` para isolamento goroutine-safe.

### 6. Session Manager (`session.go`, `session_persistence.go`)

Isolamento por chat/grupo com persistência em disco:

```
data/sessions/
├── whatsapp_5511999999999/
│   ├── history.jsonl     # Entradas de conversação (JSONL)
│   ├── facts.json        # Fatos extraídos
│   └── meta.json         # Metadados da sessão
```

- **Thread-safety**: `sync.RWMutex` por sessão.
- **File locks**: locks de arquivo para persistência.
- **Compactação preventiva**: dispara a 80% do threshold (não 100%).

### 7. Subagent System (`subagent.go`)

Agentes filhos para trabalho paralelo:

```
Main Agent ──spawn_subagent──▶ SubagentManager ──goroutine──▶ Child AgentRun
                                    │                              │
                                    ▼                              ▼
                             SubagentRegistry           (sessão isolada,
                             tracks runs + results       tools filtradas,
                                                         prompt separado)
```

- **Sem recursão**: subagents não podem spawnar subagents.
- **Semáforo**: máximo 4 subagents concorrentes (default).
- **Timeout**: 300s por subagent.
- **Tools filtradas**: deny list remove `spawn_subagent`, `list_subagent`, `wait_subagent`.

## Canais e Gateway

### Channels (`channels/`)

Interface abstrata que cada canal implementa:

- **WhatsApp** (`channels/whatsapp/`): implementação nativa em Go via whatsmeow. Suporta texto, imagens, áudio, vídeo, documentos, stickers, localizações, contatos, reações, typing indicators, read receipts.
- **Discord** e **Telegram**: planejados.

### HTTP Gateway (`gateway/`)

API REST compatível com OpenAI:

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| POST | `/v1/chat/completions` | Chat (suporta streaming SSE) |
| GET | `/api/sessions` | Listar sessões |
| GET/DELETE | `/api/sessions/:id` | Sessão específica |
| GET | `/api/usage` | Estatísticas de uso |
| GET | `/api/status` | Status do sistema |
| POST | `/api/webhooks` | Registrar webhook |
| GET | `/health` | Health check |

## Fluxo de uma Mensagem

```
1. Canal recebe mensagem (WhatsApp / Gateway / CLI)
2. Channel Manager roteia para Assistant
3. Message Queue aplica debounce (1s) e dedup (5s window)
4. Assistant resolve sessão (cria ou reutiliza)
5. Mídia é enriquecida (vision/whisper se aplicável)
6. Prompt Composer monta system prompt (8 camadas + trimming)
7. Agent Loop inicia:
   a. Chama LLM com contexto
   b. Se tool_calls: ToolExecutor despacha (paralelo/sequencial)
   c. ToolGuard valida permissões e bloqueia comandos perigosos
   d. Resultados são appendados ao contexto
   e. Repete até resposta final ou max_turns
8. Resposta é formatada (markdown WhatsApp se necessário)
9. Message Splitter divide em chunks compatíveis com o canal
10. Block Streamer entrega progressivamente (se habilitado)
11. Sessão é persistida em disco
```

## Stack Tecnológica

| Componente | Tecnologia |
|-----------|-----------|
| Linguagem | Go 1.22+ |
| CLI | Cobra + readline |
| Setup | charmbracelet/huh (TUI forms) |
| WhatsApp | whatsmeow (nativo Go) |
| Database | SQLite (go-sqlite3) com FTS5 |
| Criptografia | AES-256-GCM + Argon2id (stdlib + x/crypto) |
| Scheduler | robfig/cron v3 |
| Config | YAML (gopkg.in/yaml.v3) |
| Keyring | go-keyring (GNOME/macOS/Windows) |
| QR Code | mdp/qrterminal |
| Sandbox | Linux namespaces (syscall) / Docker CLI |
