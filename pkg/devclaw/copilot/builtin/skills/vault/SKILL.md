---
name: vault
description: "Store and retrieve secrets securely with AES-256-GCM encryption"
trigger: automatic
---

# Vault

Secure credential storage for API keys, tokens, passwords, and other sensitive data.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│  vault_save   │                    │  vault_get    │
│ (encrypt &    │                    │ (decrypt &    │
│   store)      │                    │   retrieve)   │
└───────┬───────┘                    └───────┬───────┘
        │                                     │
        │     ┌───────────────────────┐       │
        └────▶│   Encrypted Vault     │◀──────┘
              │  ~/.devclaw/vault/    │
              │  (AES-256-GCM)        │
              └───────────┬───────────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
        ▼                 ▼                 ▼
┌───────────────┐ ┌───────────────┐ ┌───────────────┐
│  vault_list   │ │ vault_delete  │ │  Master Key   │
│ (names only)  │ │  (remove)     │ │ (derived from │
│               │ │               │ │   password)   │
└───────────────┘ └───────────────┘ └───────────────┘
```

## Tools

| Tool | Action | Risk Level |
|------|--------|------------|
| `vault_save` | Store a secret | Low (overwrites) |
| `vault_get` | Retrieve a secret | Low |
| `vault_list` | List secret names | None (names only) |
| `vault_delete` | Remove a secret | **HIGH** (destructive) |

## When to Use

| Tool | When |
|------|------|
| `vault_save` | User provides API key, token, password |
| `vault_get` | Need credential for API call |
| `vault_list` | Check what credentials are available |
| `vault_delete` | User **explicitly** asks to remove |

## IMPORTANT Rules

| Rule | Reason |
|------|--------|
| **NEVER** `vault_delete` before `vault_save` | `vault_save` overwrites automatically |
| **NEVER** store in `memory` | Use vault for secrets, memory for facts |
| **ALWAYS** use descriptive names | `GITHUB_TOKEN`, `DATABASE_URL` |
| **ALWAYS** confirm before delete | Deletion is permanent |

## Saving Secrets

```bash
vault_save(name="OPENAI_API_KEY", value="sk-proj-xxxxx")
# Output: Secret 'OPENAI_API_KEY' saved to encrypted vault.

vault_save(name="DATABASE_URL", value="postgres://user:pass@host:5432/db")
# Output: Secret 'DATABASE_URL' saved to encrypted vault.

vault_save(name="GITHUB_TOKEN", value="ghp_xxxxx")
# Output: Secret 'GITHUB_TOKEN' saved to encrypted vault.
```

## Retrieving Secrets

```bash
vault_get(name="OPENAI_API_KEY")
# Output: sk-proj-xxxxx

vault_get(name="DATABASE_URL")
# Output: postgres://user:pass@host:5432/db
```

## Listing Secrets

```bash
vault_list()
# Output:
# Vault contains 3 secrets:
# - ANTHROPIC_API_KEY
# - DATABASE_URL
# - GITHUB_TOKEN
# - OPENAI_API_KEY
```

## Deleting Secrets

**⚠️ ONLY use when user explicitly requests deletion**

```bash
vault_delete(name="OLD_API_KEY")
# Output: Secret 'OLD_API_KEY' removed from vault.
```

## Common Patterns

### Store New API Key
```bash
# User: "Here's my OpenAI key: sk-proj-xxxxx"

vault_save(name="OPENAI_API_KEY", value="sk-proj-xxxxx")
# Output: Secret 'OPENAI_API_KEY' saved.

send_message("Your OpenAI API key has been stored securely.")
```

### Use Stored Credential
```bash
# Need to make API call

# 1. Get the key
key = vault_get(name="OPENAI_API_KEY")

# 2. Use in API call
bash(command='curl -H "Authorization: Bearer ' + key + '" https://api.openai.com/v1/models')
```

### Update Existing Secret
```bash
# User: "My GitHub token changed to ghp_newToken"

# Just save - it overwrites automatically
vault_save(name="GITHUB_TOKEN", value="ghp_newToken")
# Output: Secret 'GITHUB_TOKEN' saved.

# DO NOT delete first - unnecessary and risky
```

### Check Available Credentials
```bash
# User: "What API keys do I have stored?"

vault_list()
# Output:
# Vault contains 2 secrets:
# - GITHUB_TOKEN
# - OPENAI_API_KEY

send_message("You have GitHub and OpenAI credentials stored.")
```

### Database Connection
```bash
# 1. Store connection string
vault_save(
  name="DATABASE_URL",
  value="postgres://admin:secret123@db.example.com:5432/production"
)

# 2. Retrieve and use
url = vault_get(name="DATABASE_URL")
bash(command="psql $DATABASE_URL -c 'SELECT 1'")
```

## Troubleshooting

### "Vault is locked"

**Cause:** Vault requires unlock after restart.

**Solution:**
- Vault auto-unlocks on first access with master password
- If prompted, provide the vault password

### "Secret not found"

**Cause:** Name doesn't exist or typo.

**Debug:**
```bash
# List all secrets to verify name
vault_list()

# Check exact spelling (case-sensitive)
```

### "Failed to save"

**Cause:** Vault directory issue or permission problem.

**Debug:**
```bash
# Check vault directory
bash(command="ls -la ~/.devclaw/vault/")

# Check permissions
bash(command="chmod 700 ~/.devclaw/vault/")
```

### Accidentally deleted a secret

**Cause:** Used `vault_delete` incorrectly.

**Prevention:**
- **NEVER** delete before save (it overwrites)
- Always confirm with user before any deletion
- There is no undo - secrets are permanently removed

## Vault vs Memory

| Use Vault | Use Memory |
|-----------|------------|
| API keys | User preferences |
| Passwords | General facts |
| Database URLs | Project context |
| Access tokens | Conversation summaries |
| Private keys | Non-sensitive info |

**Rule:** If exposure could cause harm → vault, not memory.

## Security Notes

- **Encryption**: AES-256-GCM at rest
- **Access**: Vault must be unlocked
- **Listing**: Shows names only, never values
- **Overwrite**: `vault_save` replaces automatically
- **No undo**: Deletion is permanent

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| `vault_delete` then `vault_save` | Just `vault_save` - it overwrites |
| Storing in `memory` | Use vault for all secrets |
| Deleting "to clear space" | Never delete unless explicitly asked |
| Vague names like `KEY1` | Use `GITHUB_TOKEN`, `DB_PASSWORD` |
| Not confirming before delete | Always verify with user |

## Complete Workflow Example

### Setting Up a New Project
```bash
# User provides credentials

# Store each credential
vault_save(name="OPENAI_API_KEY", value="sk-proj-xxxxx")
vault_save(name="DATABASE_URL", value="postgres://...")
vault_save(name="STRIPE_SECRET_KEY", value="sk_live_xxxxx")

# Verify storage
vault_list()
# Output:
# Vault contains 3 secrets:
# - DATABASE_URL
# - OPENAI_API_KEY
# - STRIPE_SECRET_KEY

send_message("All credentials stored securely. Ready to use them for API calls.")
```

### Using Credentials in Code
```bash
# Get credentials
api_key = vault_get(name="OPENAI_API_KEY")
db_url = vault_get(name="DATABASE_URL")

# Use in application
bash(command='''
export OPENAI_API_KEY="''' + api_key + '''"
export DATABASE_URL="''' + db_url + '''"
npm run start
''')
```
