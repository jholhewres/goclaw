# DevClaw — Security

Documentation of the security mechanisms implemented in DevClaw. The framework adopts a **deny-by-default** posture with multiple protection layers.

---

## Layer Overview

```
┌──────────────────────────────────────────────────┐
│                  Access Control                   │
│         (roles: owner > admin > user > blocked)   │
├──────────────────────────────────────────────────┤
│                   Tool Guard                      │
│      (permissions, command blocking, auditing)    │
├──────────────────────────────────────────────────┤
│            Workspace Containment                  │
│   (sandbox path, symlink escape protection)       │
├──────────────────────────────────────────────────┤
│            Memory Injection Hardening             │
│   (untrusted content sanitization)                │
├──────────────────────────────────────────────────┤
│                  SSRF Protection                  │
│       (URL validation, private IP blocking)       │
├──────────────────────────────────────────────────┤
│                 Encrypted Vault                   │
│       (AES-256-GCM + Argon2id, at-rest)          │
├──────────────────────────────────────────────────┤
│                 Script Sandbox                    │
│      (namespaces, Docker, content scanning)       │
├──────────────────────────────────────────────────┤
│              Gateway Authentication               │
│     (Bearer token, CORS, body limit, WebSocket)   │
└──────────────────────────────────────────────────┘
```

---

## 1. Access Control (`access.go`)

Per-user and per-group access control with a role hierarchy.

### Role Hierarchy

```
owner  →  Full access. Only role that can execute bash, ssh, set_env.
  │
admin  →  Can use scp, exec, manage skills and scheduler.
  │
user   →  Read-only tools, memory, web search.
  │
blocked →  No access. Messages silently ignored.
```

### Default Policy

```yaml
access:
  default_policy: deny    # deny | allow | ask
  owners: ["5511999999999"]
  admins: []
  allowed_users: []
  blocked_users: []
  allowed_groups: []
  blocked_groups: []
```

- **deny** (default): unknown contacts are ignored.
- **allow**: any contact can interact (personal use).
- **ask**: asks the owner for confirmation on new contacts.

### Chat Management

| Command | Action |
|---------|--------|
| `/allow <phone>` | Authorize contact |
| `/block <phone>` | Block contact |
| `/admin <phone>` | Promote to admin |
| `/users` | List users and roles |
| `/group allow` | Authorize current group |

---

## 2. Tool Guard (`tool_guard.go`)

Security layer that controls which tools can be used, by whom, and under what conditions.

### Per-Tool Permissions

Each tool has a minimum access level:

| Permission | Tools |
|------------|-------|
| `owner` | `bash`, `ssh`, `set_env` |
| `admin` | `scp`, `exec`, `schedule_add`, `schedule_remove`, `install_skill`, `remove_skill`, `spawn_subagent` |
| `user` | `read_file`, `search_files`, `glob_files`, `list_files`, `web_search`, `web_fetch`, `memory`, `describe_image`, `transcribe_audio`, `list_skills`, `search_skills`, `schedule_list` |
| `public` | None by default (configurable) |

### Destructive Command Blocking

The ToolGuard maintains a list of regex patterns for dangerous commands. These are **blocked for everyone** by default (even owners):

| Category | Pattern Examples |
|----------|-----------------|
| **Destructive** | `rm -rf /`, `mkfs`, `dd if=`, `format` |
| **Permission** | `chmod 777 /`, `chmod -R 777` |
| **Fork bomb** | `:(){ :|:& };:` |
| **Pipe to shell** | `curl.*\|.*sh`, `wget.*\|.*bash` |
| **Shutdown** | `shutdown`, `reboot`, `halt`, `poweroff` |
| **Sudo** | `sudo` (for non-owners) |

```yaml
security:
  tool_guard:
    allow_destructive: false   # Even owner is blocked by default
    allow_sudo: false           # sudo blocked for non-owners
    allow_reboot: false         # shutdown/reboot blocked
    dangerous_commands:         # Additional custom patterns
      - "curl.*\\|.*sh"
```

### Sensitive Path Protection

Protected paths cannot be read/written by non-owners:

```yaml
protected_paths:
  - ~/.ssh/id_*
  - ~/.ssh/authorized_keys
  - .devclaw.vault
  - /etc/shadow
  - /etc/passwd
  - credentials.json
  - *.pem
  - *.key
```

Supports glob patterns. Protected paths are checked in `read_file`, `write_file`, `edit_file`, and `bash`.

### Interactive Approval

Tools in the `require_confirmation` list require explicit user approval before executing:

```yaml
require_confirmation: [bash, ssh, scp, write_file]
```

Flow:
1. Agent calls the tool.
2. ToolGuard intercepts and sends a chat message: `"Confirm: bash — rm old-logs/ ?"`.
3. User responds with `/approve <id>` or `/deny <id>`.
4. If approved, executes. If denied or timeout, cancels.

### Audit Log

**Every** tool execution (allowed or blocked) is logged:

```
2025-01-15T14:30:22Z | ALLOWED | owner:5511999999999 | bash | {"command": "ls -la"}
2025-01-15T14:30:45Z | BLOCKED | user:5511888888888 | bash | {"command": "rm -rf /"} | reason: destructive command
```

```yaml
security:
  tool_guard:
    audit_log: ./data/audit.log
```

---

## 3. Workspace Containment (`workspace_containment.go`)

All file operations are validated against the configured workspace root.

### Protections

| Attack | Protection |
|--------|------------|
| Path traversal (`../../etc/passwd`) | Resolved path must be under workspace root |
| Symlink escape | `os.Lstat` + `filepath.EvalSymlinks` check |
| Absolute path bypass | Verified against workspace root |

### Implementation

```
File operation request (path: "../../../etc/shadow")
       │
       ▼
  Resolve to absolute path
       │
       ▼
  Check symlink target (if symlink)
       │
       ▼
  Verify resolved path is under workspace root
       │
       ├── Under root ──▶ ALLOW
       └── Outside root ──▶ BLOCK
```

---

## 4. Memory Injection Hardening (`memory_hardening.go`)

Memory content injected into LLM prompts is treated as **untrusted historical data**.

### Protections

| Threat | Mitigation |
|--------|------------|
| Prompt injection via saved memories | Content wrapped in `<relevant-memories>` tags |
| HTML/script injection | HTML entities escaped |
| Dangerous tags | Stripped before injection |
| Common injection patterns | Detected and neutralized |

### How It Works

```
Memory recalled from SQLite
       │
       ▼
  Escape HTML entities (< > & " ')
       │
       ▼
  Strip dangerous tags (<script>, <iframe>, etc.)
       │
       ▼
  Detect injection patterns (system prompts, role overrides)
       │
       ▼
  Wrap in <relevant-memories> tags
       │
       ▼
  Inject into prompt
```

---

## 5. SSRF Protection (`security/ssrf.go`)

Protection against Server-Side Request Forgery in `web_fetch` and similar tools.

### Mechanism

1. **URL parsing**: validates format and extracts hostname.
2. **Scheme validation**: only `http` and `https` are allowed.
3. **DNS resolution**: hostname is resolved to IPs **before** validation (defense against DNS rebinding).
4. **IP validation**: resolved IPs are checked against blocked ranges.
5. **Allowlist/Blocklist**: if configured, only whitelisted hosts are allowed.

### Blocked IP Ranges

| Range | Reason |
|-------|--------|
| `127.0.0.0/8` | Loopback |
| `10.0.0.0/8` | Private network (Class A) |
| `172.16.0.0/12` | Private network (Class B) |
| `192.168.0.0/16` | Private network (Class C) |
| `169.254.0.0/16` | Link-local |
| `169.254.169.254` | Cloud metadata (AWS/GCP/Azure) |
| `0.0.0.0` | Any local interface |
| `::1`, `fe80::/10` | IPv6 loopback and link-local |

---

## 6. Encrypted Vault (`vault.go`, `keyring.go`)

Encrypted credential storage using military-grade cryptography.

### Algorithms

| Component | Algorithm | Specification |
|-----------|-----------|---------------|
| **Encryption** | AES-256-GCM | 256-bit key, authenticated encryption |
| **Key Derivation** | Argon2id | OWASP recommended |
| **Salt** | Random | 16 bytes (crypto/rand) |

### Argon2id Parameters

```go
argonTime    = 3           // 3 iterations
argonMemory  = 64 * 1024   // 64 MB RAM
argonThreads = 4           // 4 parallel threads
argonKeyLen  = 32          // AES-256 (32 bytes)
```

### Secret Resolution Chain

```
1. Encrypted vault (.devclaw.vault)     → AES-256-GCM
2. OS keyring (GNOME/macOS/Windows)    → Native OS API
3. Environment variable                 → DEVCLAW_API_KEY
4. config.yaml                          → ${DEVCLAW_API_KEY}
```

First match wins. The priority ensures the encrypted vault is always preferred.

---

## 7. Script Sandbox (`sandbox/`)

Execution isolation for community skill scripts.

### Isolation Levels

| Level | Method | Use Case | Risk |
|-------|--------|----------|------|
| `none` | Direct `exec.Command` | Builtin/trusted skills | High |
| `restricted` | Linux namespaces + seccomp + cgroups | Community skills | Medium |
| `container` | Docker with purpose-built image | Untrusted scripts | Low |

### Pre-Execution Content Scanning (`policy.go`)

Before execution, scripts are scanned for malicious patterns:

| Category | Detected Patterns | Severity |
|----------|-------------------|----------|
| **Code injection** | `exec()`, `eval()`, `os.system()`, `subprocess` | Warning |
| **Shell injection** | `` `cmd` ``, `$(cmd)`, pipe chains | Warning |
| **Crypto mining** | `xmrig`, `minerd`, `cryptonight`, `stratum+tcp` | Critical |
| **Reverse shell** | `/dev/tcp`, `nc -e`, `bash -i`, `python -c "import socket"` | Critical |
| **Data exfiltration** | `curl.*POST`, `wget --post`, `nc.*<` | Critical |
| **Obfuscation** | `base64 -d.*\|.*sh`, `python -c "exec(..."` | Critical |

- **Warning**: execution allowed with logging.
- **Critical**: execution **blocked**.

---

## 8. Gateway Authentication (`gateway/`)

### HTTP Authentication

Bearer token authentication for the HTTP API:

```yaml
gateway:
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

### Request Body Limiter

API gateway enforces a 2MB request body limit on `/v1/chat/completions` to prevent oversized payloads from causing memory exhaustion (OOM).

### WebSocket Security

WebSocket connections at `/ws` share the same Bearer token authentication as HTTP endpoints.

---

## Security Matrix

Summary of protections by attack vector:

| Attack Vector | Protection | Layer |
|---------------|------------|-------|
| Unauthorized access | Role-based access control | Access Control |
| Destructive commands | Regex blocking + confirmation | Tool Guard |
| Secret reading | Protected paths + vault encryption | Tool Guard + Vault |
| Path traversal | Workspace containment | Containment |
| Symlink escape | Target resolution + root check | Containment |
| Prompt injection via memory | Sanitization + wrapping | Memory Hardening |
| SSRF (request forgery) | DNS resolve + IP validation | SSRF Guard |
| DNS rebinding | Pre-resolve hostname to IP | SSRF Guard |
| Cloud metadata theft | Block 169.254.169.254 | SSRF Guard |
| Credential theft | AES-256-GCM at-rest encryption | Vault |
| Vault brute force | Argon2id (64MB, 3 iter) | Vault |
| Malicious scripts | Content scanning + sandbox | Sandbox |
| Reverse shells | Pattern detection (critical) | Sandbox |
| Crypto mining | Pattern detection (critical) | Sandbox |
| Container escape | Docker isolation | Sandbox |
| Privilege escalation | Namespace isolation + sudo block | Sandbox + Tool Guard |
| Audit evasion | Mandatory logging of all tool calls | Tool Guard |
| Unauthorized API access | Bearer token + CORS | Gateway |
| OOM via oversized payload | Request body limiter (2MB) | Gateway |

---

## Configuration Best Practices

### Production (recommended)

```yaml
access:
  default_policy: deny
  owners: ["your-number"]

security:
  tool_guard:
    enabled: true
    allow_destructive: false
    allow_sudo: false
    allow_reboot: false
    require_confirmation: [bash, ssh, scp, write_file, edit_file]
    audit_log: ./data/audit.log
  ssrf:
    allow_private: false

sandbox:
  default_isolation: restricted

gateway:
  auth_token: "strong-generated-token"
```

### Development (relaxed)

```yaml
access:
  default_policy: allow

security:
  tool_guard:
    enabled: true
    allow_sudo: true
    require_confirmation: [bash]
    auto_approve: [read_file, write_file, edit_file]

sandbox:
  default_isolation: none
```
