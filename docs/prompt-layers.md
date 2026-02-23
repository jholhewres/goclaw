# Prompt Layers System

DevClaw uses a layered prompt composition system to build the system prompt sent to the LLM. Each layer has a specific purpose and priority.

---

## Overview

The prompt is assembled from multiple layers, each contributing a specific type of context:

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 0: Core Identity                                       │
│   - Agent name, role, capabilities                           │
│   - Available tools summary                                  │
├─────────────────────────────────────────────────────────────┤
│ Layer 5: Safety Rules                                        │
│   - Security guidelines                                      │
│   - Prohibited actions                                       │
├─────────────────────────────────────────────────────────────┤
│ Layer 10: Custom Instructions                                │
│   - User-defined instructions                                │
│   - Agent-specific configuration                             │
├─────────────────────────────────────────────────────────────┤
│ Layer 12: Thinking Level                                     │
│   - Extended thinking hints (from /think)                    │
├─────────────────────────────────────────────────────────────┤
│ Layer 15: Bootstrap Files                                    │
│   - SOUL.md, AGENTS.md, TOOLS.md                             │
│   - Project-specific context                                 │
├─────────────────────────────────────────────────────────────┤
│ Layer 18: Built-in Skills                                    │
│   - Tool usage guides (memory, teams, etc.)                  │
├─────────────────────────────────────────────────────────────┤
│ Layer 20: Business Context                                   │
│   - Workspace context                                        │
│   - Project information                                      │
├─────────────────────────────────────────────────────────────┤
│ Layer 40: Active Skills                                      │
│   - User-loaded skill instructions                           │
├─────────────────────────────────────────────────────────────┤
│ Layer 50: Memory Context                                     │
│   - Relevant memories from search                            │
├─────────────────────────────────────────────────────────────┤
│ Layer 60: Temporal Context                                   │
│   - Current date/time                                        │
│   - Timezone information                                     │
├─────────────────────────────────────────────────────────────┤
│ Layer 70: Conversation History                               │
│   - Recent messages                                          │
│   - Summarized older content                                 │
├─────────────────────────────────────────────────────────────┤
│ Layer 80: Runtime Info                                       │
│   - Model, OS, hostname                                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Layer Definitions

```go
const (
    LayerCore          PromptLayer = 0  // Base identity and tooling
    LayerSafety        PromptLayer = 5  // Safety rules
    LayerIdentity      PromptLayer = 10 // Custom instructions
    LayerThinking      PromptLayer = 12 // Extended thinking hint
    LayerBootstrap     PromptLayer = 15 // SOUL.md, AGENTS.md, etc.
    LayerBuiltinSkills PromptLayer = 18 // Built-in tool guides
    LayerBusiness      PromptLayer = 20 // User/workspace context
    LayerSkills        PromptLayer = 40 // Active skill instructions
    LayerMemory        PromptLayer = 50 // Long-term memory facts
    LayerTemporal      PromptLayer = 60 // Date/time context
    LayerConversation  PromptLayer = 70 // Recent history summary
    LayerRuntime       PromptLayer = 80 // Runtime info (final line)
)
```

---

## Layer Details

### LayerCore (0)

Base identity that defines who the agent is and what it can do.

**Content:**
- Agent name and role
- Available tools summary
- Communication style guidelines

**Source:** `buildCoreLayer()` in `prompt_layers.go`

---

### LayerSafety (5)

Security rules and prohibited actions.

**Content:**
- SSRF prevention
- Dangerous command blocking
- Input/output guardrails

**Source:** `buildSafetyLayer()` in `prompt_layers.go`

---

### LayerIdentity (10)

User-defined custom instructions.

**Content:**
- `instructions` from config
- Agent-specific behavior rules

**Source:** `p.config.Instructions`

---

### LayerThinking (12)

Extended thinking level hints (activated by `/think`).

**Content:**
- Thinking depth level
- Reasoning mode indicators

**Source:** `buildThinkingLayer()` in `prompt_layers.go`

---

### LayerBootstrap (15)

Project-specific context loaded from files.

**Content:**
- `SOUL.md` - Agent personality/behavior
- `AGENTS.md` - Subagent definitions
- `TOOLS.md` - Custom tool documentation
- `COMMANDS.md` - Available commands

**Source:** `buildBootstrapLayer()` in `prompt_layers.go`

---

### LayerBuiltinSkills (18)

Built-in tool usage guides.

**Content:**
- Memory tool guide
- Teams tool guide
- Other core tool documentation

**Source:** `buildBuiltinSkillsLayer()` in `prompt_layers.go`

---

### LayerBusiness (20)

Workspace and project context.

**Content:**
- `business_context` from session config
- Project-specific information

**Source:** `session.GetConfig().BusinessContext`

---

### LayerSkills (40)

User-loaded skill instructions.

**Content:**
- Active skill system prompts
- Workflow-specific guidance

**Source:** `buildSkillsLayer()` in `prompt_layers.go`

---

### LayerMemory (50)

Relevant memories retrieved from long-term storage.

**Content:**
- Hybrid search results (vector + BM25)
- Recent memory entries
- Facts matching current context

**Source:** `buildMemoryLayer()` in `prompt_layers.go`

---

### LayerTemporal (60)

Date and time context.

**Content:**
- Current date/time
- Timezone information
- Day of week

**Source:** `buildTemporalLayer()` in `prompt_layers.go`

---

### LayerConversation (70)

Recent conversation history.

**Content:**
- Recent messages (with budget limit)
- Summarized older content
- Tool call history

**Source:** `buildConversationLayer()` in `prompt_layers.go`

---

### LayerRuntime (80)

Runtime information (final layer).

**Content:**
- Agent name
- Model in use
- OS/architecture
- Hostname
- Working directory

**Source:** `buildRuntimeLayer()` in `prompt_layers.go`

---

## Token Budgets

Each layer has a soft token budget:

```go
layerBudgets := map[PromptLayer]int{
    LayerCore:          config.TokenBudget.System,
    LayerSafety:        500,
    LayerIdentity:      1000,
    LayerThinking:      200,
    LayerBootstrap:     4000,
    LayerBuiltinSkills: 2000,
    LayerBusiness:      1000,
    LayerSkills:        config.TokenBudget.Skills,
    LayerMemory:        config.TokenBudget.Memory,
    LayerTemporal:      200,
    LayerConversation:  config.TokenBudget.History,
    LayerRuntime:       200,
}
```

### Budget Enforcement

When total prompt exceeds configured budget:

1. Layers sorted by priority (lower = higher priority)
2. Lower priority layers trimmed first
3. Truncation indicated in output

---

## Layer Caching

Heavy layers use session-level caching:

| Layer | Cache TTL | Notes |
|-------|-----------|-------|
| Memory | 30s | Refreshed in background |
| Skills | 30s | Refreshed in background |

### Cache Flow

1. Check cache for fresh result
2. If stale, use cached version for current prompt
3. Refresh in background for next prompt

---

## Concurrent Loading

Critical layers are loaded concurrently:

```go
var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); bootstrap = p.buildBootstrapLayer() }()
go func() { defer wg.Done(); history = p.buildConversationLayer(session) }()
wg.Wait()
```

---

## Minimal Prompt Mode

For scheduled jobs, a minimal prompt is used:

```go
func (p *PromptComposer) ComposeMinimal() string
```

Includes only:
- Core identity
- Safety rules
- Temporal context
- Custom instructions

Skips:
- Bootstrap files
- Memory search
- Skills
- Conversation history

---

## Configuration

### Token Budget

```yaml
token_budget:
  total: 128000      # Maximum total tokens
  system: 16000      # System prompt budget
  memory: 2000       # Memory layer budget
  skills: 4000       # Skills layer budget
  history: 10000     # History layer budget
```

### Cache TTL

```yaml
prompt_layer_cache_ttl: 30s
```

---

## Implementation

### File: prompt_layers.go

```go
type PromptComposer struct {
    config        *Config
    memoryStore   *memory.FileStore
    sqliteMemory  *memory.SQLiteStore
    skillGetter   func(name string) (interface{ SystemPrompt() string }, bool)
    builtinSkills *BuiltinSkills
    isSubagent    bool
}

type PromptLayer int

type layerEntry struct {
    layer   PromptLayer
    content string
}
```

### Key Methods

| Method | Description |
|--------|-------------|
| `Compose(session, input)` | Build full system prompt |
| `ComposeMinimal()` | Build lightweight prompt |
| `buildCoreLayer()` | Build core identity |
| `buildBootstrapLayer()` | Load bootstrap files |
| `buildMemoryLayer()` | Search and format memories |
| `assembleLayers()` | Combine and trim layers |

---

## Best Practices

### For Layer Authors

1. **Lower priority = more important** - Critical content in low layers
2. **Respect budgets** - Keep content concise
3. **Handle missing data gracefully** - Return empty string if unavailable
4. **Use concurrent loading** - Heavy I/O in goroutines

### For Configuration

1. **Set appropriate budgets** - Based on model context window
2. **Monitor cache hit rates** - Adjust TTL if needed
3. **Test with minimal mode** - Ensure scheduled jobs work
