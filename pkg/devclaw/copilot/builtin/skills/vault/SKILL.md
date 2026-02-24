---
name: vault
description: "Store and retrieve secrets securely with AES-256-GCM encryption"
trigger: automatic
---

# Vault

Secure credential storage for API keys, tokens, passwords, and other sensitive data.

## Tools

| Tool | Action |
|------|--------|
| `vault_save` | Store a secret (overwrites if exists) |
| `vault_get` | Retrieve a secret by name |
| `vault_list` | List all secret names |
| `vault_delete` | Delete a secret (use carefully) |

## When to Use

| Tool | When |
|------|------|
| `vault_save` | User provides API key, token, password, database URL |
| `vault_get` | Need a credential for an API call or operation |
| `vault_list` | Check what credentials are available |
| `vault_delete` | User explicitly asks to remove a credential |

## Workflow

```
User provides API key:
1. vault_save("OPENAI_API_KEY", "sk-xxx")
2. Done - key is encrypted and stored

Need to use stored credential:
1. key := vault_get("OPENAI_API_KEY")
2. Use key in API call
```

## Important Rules

| Rule | Reason |
|------|--------|
| **NEVER** call vault_delete before vault_save | Causes data loss - vault_save overwrites automatically |
| **NEVER** store in memory | Use vault for secrets, memory for facts |
| **ALWAYS** use descriptive names | `GITHUB_TOKEN`, `DATABASE_URL`, `STRIPE_KEY` |

## Examples

### Store a new API key
```bash
vault_save(name="ANTHROPIC_API_KEY", value="sk-ant-xxx")
# Output: Secret 'ANTHROPIC_API_KEY' saved to encrypted vault.
```

### Retrieve when needed
```bash
vault_get(name="ANTHROPIC_API_KEY")
# Output: sk-ant-xxx
```

### List available secrets
```bash
vault_list()
# Output:
# Vault contains 3 secrets:
# - ANTHROPIC_API_KEY
# - GITHUB_TOKEN
# - DATABASE_URL
```

### Delete (only when user asks)
```bash
vault_delete(name="OLD_KEY")
# Output: Secret 'OLD_KEY' removed from vault.
```

## Security Notes

- Secrets are encrypted at rest with AES-256-GCM
- Vault must be unlocked before operations
- `vault_list` shows names only, never values
- `vault_save` overwrites automatically - no need to delete first

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| `vault_delete` then `vault_save` | Just `vault_save` - it overwrites |
| Storing in memory | Use vault for secrets |
| Deleting "to clear space" | Never delete unless user explicitly requests |
