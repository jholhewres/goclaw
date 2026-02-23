# Built-in Skills System

Built-in skills are embedded documentation files that provide tool usage guides, best practices, and workflow examples for core DevClaw features.

---

## Overview

Unlike user skills (stored in `skills/` directory), built-in skills are:

- **Embedded in the binary** via `embed.FS`
- **Always available** - no installation required
- **Loaded automatically** into the system prompt
- **Updated via code changes** - requires rebuild

```
pkg/devclaw/copilot/
└── builtin/
    └── skills/
        ├── memory/
        │   └── SKILL.md
        └── teams/
            └── SKILL.md
```

---

## How It Works

### 1. Embedding

Skills are embedded at compile time using Go's `embed.FS`:

```go
//go:embed builtin/skills/*/SKILL.md
var builtinSkillsFS embed.FS
```

### 2. Loading

Skills are loaded once at startup using singleton pattern:

```go
func LoadBuiltinSkills(logger *slog.Logger) *BuiltinSkills
```

### 3. Integration

Skills are included in the system prompt via `PromptComposer`:

```go
// In assistant.go
builtinSkills := LoadBuiltinSkills(logger)
promptComposer.SetBuiltinSkills(builtinSkills)

// In prompt_layers.go
if builtinSkills := p.buildBuiltinSkillsLayer(); builtinSkills != "" {
    layers = append(layers, layerEntry{layer: LayerBuiltinSkills, content: builtinSkills})
}
```

---

## SKILL.md Format

### Frontmatter (YAML)

```yaml
---
name: skill-name
description: "Short description of the skill"
trigger: automatic | manual
---
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Skill identifier (lowercase, alphanumeric) |
| `description` | Yes | Brief description shown in prompts |
| `trigger` | No | `automatic` (default) = always included, `manual` = on-demand |

### Content (Markdown)

```markdown
# Title

Description of what this skill covers.

## Section

Content with examples:

\`\`\`bash
tool_name(action="example", param="value")
\`\`\`

## Best Practices

1. Practice one
2. Practice two
```

---

## Trigger Types

### Automatic

Skills with `trigger: automatic` are always included in the system prompt:

```yaml
---
trigger: automatic
---
```

These provide guidance for core tools that agents use frequently.

### Manual

Skills with `trigger: manual` are loaded on-demand (future feature):

```yaml
---
trigger: manual
---
```

Use for specialized workflows that aren't needed every conversation.

---

## Adding a New Built-in Skill

### Step 1: Create Directory

```bash
mkdir -p pkg/devclaw/copilot/builtin/skills/vault
```

### Step 2: Create SKILL.md

```markdown
---
name: vault
description: "Manage encrypted secrets and credentials"
trigger: automatic
---

# Vault

Secure storage for API keys, tokens, and other secrets.

## Tool: vault

\`\`\`bash
vault(action="save", key="api_key", value="secret123")
vault(action="get", key="api_key")
vault(action="list")
vault(action="delete", key="api_key")
\`\`\`

## Best Practices

1. Never log retrieved secrets
2. Use descriptive key names
3. Rotate secrets regularly
```

### Step 3: Rebuild

```bash
go build ./...
```

The skill will be automatically loaded on next startup.

---

## Current Built-in Skills

| Skill | Description | Trigger |
|-------|-------------|---------|
| `memory` | Long-term memory management (save, search, list, index) | automatic |
| `teams` | Persistent agents, tasks, communication | automatic |

---

## Prompt Integration

### Layer Priority

Built-in skills use `LayerBuiltinSkills` (priority 18):

```go
const (
    LayerCore          PromptLayer = 0
    LayerSafety        PromptLayer = 5
    LayerIdentity      PromptLayer = 10
    LayerThinking      PromptLayer = 12
    LayerBootstrap     PromptLayer = 15
    LayerBuiltinSkills PromptLayer = 18  // Built-in tool guides
    LayerBusiness      PromptLayer = 20
    // ...
)
```

### Token Budget

Built-in skills have a budget of ~2000 tokens:

```go
layerBudgets := map[PromptLayer]int{
    // ...
    LayerBuiltinSkills: 2000,
    // ...
}
```

If content exceeds budget, it may be truncated during prompt assembly.

---

## Implementation Details

### File: builtin_skills.go

```go
type BuiltinSkill struct {
    Name        string
    Description string
    Content     string
    Trigger     string
}

type BuiltinSkills struct {
    skills map[string]*BuiltinSkill
    mu     sync.RWMutex
    logger *slog.Logger
}
```

### Key Methods

| Method | Description |
|--------|-------------|
| `LoadBuiltinSkills(logger)` | Load all skills (singleton) |
| `Get(name)` | Get skill by name |
| `All()` | Get all skills |
| `Names()` | Get skill names |
| `FormatForPrompt()` | Format all automatic skills for prompt |
| `FormatSkillForPrompt(name)` | Format specific skill |

---

## Best Practices

### For Skill Authors

1. **Keep it concise** - Token budget is limited
2. **Use examples** - Show actual tool signatures
3. **Include workflows** - Step-by-step guides help agents
4. **Document edge cases** - Mention common pitfalls
5. **Update with code changes** - Keep docs in sync with implementation

### For Tool Developers

1. Create a skill when adding complex tools
2. Document the dispatcher pattern if using it
3. Include validation rules and defaults
4. Reference the skill in error messages

---

## Comparison: Built-in vs User Skills

| Aspect | Built-in Skills | User Skills |
|--------|-----------------|-------------|
| Location | `pkg/devclaw/copilot/builtin/skills/` | `skills/` |
| Loading | Embedded in binary | Filesystem |
| Availability | Always | Per-project |
| Update method | Rebuild binary | Edit file |
| Trigger | Automatic/manual | Via `/skill` command |
| Purpose | Core tool guides | Custom workflows |
