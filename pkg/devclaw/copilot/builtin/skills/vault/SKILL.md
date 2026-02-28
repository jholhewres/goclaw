---
name: vault
description: "store and retrieve secrets securely with AES-256-GCM encryption"
trigger: automatic
---

# Vault

Secure credential storage for API keys, tokens, passwords, and other sensitive data.

## CRITICAL RULES

| Rule | Reason |
|------|--------|
| **NEVER** call `vault(action=delete)` before `vault(action=save)` | Causes data loss |
| **NEVER** store in `memory` | Use vault for secrets |
| `vault(action=save)` overwrites automatically | NO need to delete first |

## Architecture
```
                      Agent Context
                           |
        +------------------+------------------+
        |                                     |
        v                                     v
+---------------+                    +---------------+
| vault(save)   |                    | vault(get)    |
| (encrypt &    |                    | (decrypt &    |
|   store)      |                    |   retrieve)   |
+-------+-------+                    +-------+-------+
        |                                     |
        |     +-----------------------+       |
        +---->|   Encrypted Vault     |<------+
              |  (AES-256-GCM)        |
              +-----------------------+
```

## Actions
| Action | Description | Risk Level |
|--------|-------------|------------|
| `status` | Check vault state | None |
| `save` | Store a secret | Low (overwrites) |
| `get` | Retrieve a secret | Low |
| `list` | List secret names | None (names only) |
| `delete` | Remove a secret | **HIGH** (destructive) |

## When to Use
| Action | When |
|--------|------|
| `save` | User provides API key, token, password |
| `get` | Need credential for API call |
| `list` | Check what credentials are available |
| `delete` | User **explicitly** asks to remove |

## Saving Secrets
```bash
vault(action="save", name="OPENAI_API_KEY", value="sk-proj-xxxxx")
# Output: Secret 'OPENAI_API_KEY' saved.
```

## Retrieving Secrets
```bash
vault(action="get", name="OPENAI_API_KEY")
# Output: sk-proj-xxxxx
```

## Listing Secrets
```bash
vault(action="list")
# Output:
# Vault contains 3 secrets:
# - ANTHROPIC_API_KEY
# - DATABASE_URL
# - OPENAI_API_KEY
```

## Deleting Secrets
**ONLY use when user explicitly requests deletion**
```bash
vault(action="delete", name="OLD_API_KEY")
```

## Common Patterns

### Store New API Key
```bash
# User: "Here's my OpenAI key: sk-proj-xxxxx"
vault(action="save", name="OPENAI_API_KEY", value="sk-proj-xxxxx")
send_message("Your OpenAI API key has been stored securely.")
```

### Use Stored Credential
```bash
key = vault(action="get", name="OPENAI_API_KEY")
bash(command='curl -H "Authorization: Bearer ' + key + '" https://api.openai.com/v1/models')
```

### Update Existing Secret
```bash
# Just save - it overwrites automatically
vault(action="save", name="GITHUB_TOKEN", value="ghp_newToken")

# DO NOT delete first - unnecessary and risky
```

## Vault vs Memory
| Use Vault | Use Memory |
|-----------|------------|
| API keys | User preferences |
| Passwords | General facts |
| Database URLs | Project context |
| Access tokens | Conversation summaries |

**Rule:** If exposure could cause harm -> vault, not memory.

## Common Mistakes
| Mistake | Correct Approach |
|---------|-----------------|
| `vault(action=delete)` then `vault(action=save)` | Just `vault(action=save)` - it overwrites |
| Storing in `memory` | Use vault for secrets |
| Deleting "to clear space" | Never delete unless explicitly asked |
