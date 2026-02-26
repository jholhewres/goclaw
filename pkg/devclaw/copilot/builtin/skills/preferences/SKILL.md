---
name: preferences
description: "update user preferences in USER.md (name, language, style, context)"
trigger: automatic
---

# User Preferences

Update the user's personal preferences in `configs/bootstrap/USER.md`.

## When to Use

| Trigger | Example |
|---------|---------|
| User shares their name | "Meu nome é João" |
| User specifies language | "Responde em português sempre" |
| User defines style preference | "Quero respostas curtas" |
| User adds context about themselves | "Trabalho com Go e React" |

## How to Update

Use `edit_file` to modify `configs/bootstrap/USER.md`:

```bash
edit_file(
  path="configs/bootstrap/USER.md",
  old="- **Name:**",
  new="- **Name:** João"
)
```

## USER.md Structure

```markdown
# USER.md — About Your Human

- **Name:** [user's name]
- **What to call them:** [how to address them]
- **Timezone:** [e.g., America/Sao_Paulo]
- **Language:** [e.g., Portuguese (pt-BR)]

## Context

[Projects they work on, what they care about, etc.]

## Preferences

[How they like responses, formatting, etc.]
```

## Common Updates

### Name
```bash
edit_file(path="configs/bootstrap/USER.md",
  old="- **Name:**",
  new="- **Name:** João Silva")
```

### Language
```bash
edit_file(path="configs/bootstrap/USER.md",
  old="- **Language:**",
  new="- **Language:** Portuguese (pt-BR) — always respond in Portuguese")
```

### Add Context
```bash
edit_file(path="configs/bootstrap/USER.md",
  old="## Context",
  new="## Context\n\n- Building a Go project called DevClaw\n- Values concise responses")
```

### Add Preference
```bash
edit_file(path="configs/bootstrap/USER.md",
  old="## Preferences",
  new="## Preferences\n\n- **Style:** Direct and concise\n- **Commits:** In English")
```

## After Updating

Confirm to the user what was saved. Example:
> "Got it! I've updated your preferences. I'll always respond in Portuguese from now on."

## Rules

| Rule | Reason |
|------|--------|
| NEVER store secrets in USER.md | Use vault for API keys, passwords |
| Keep it concise | This file is loaded in every prompt |
| Confirm changes | Let user know what was saved |
