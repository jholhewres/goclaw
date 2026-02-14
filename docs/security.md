# GoClaw — Security

Documentation of the security mechanisms implemented in GoClaw. The framework adopts a **deny-by-default** posture with multiple protection layers.

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
│            (Bearer token, CORS)                   │
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
| `user` | `read_file`, `search_files`, `glob_files`, `list_files`, `web_search`, `web_fetch`, `memory_save`, `memory_search`, `memory_list`, `describe_image`, `transcribe_audio`, `list_skills`, `search_skills`, `schedule_list` |
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

To enable destructive commands for the owner, set `allow_destructive: true`. Non-owners remain blocked regardless.

### Sensitive Path Protection

Protected paths cannot be read/written by non-owners:

```yaml
protected_paths:
  - ~/.ssh/id_*
  - ~/.ssh/authorized_keys
  - .goclaw.vault
  - /etc/shadow
  - /etc/passwd
  - credentials.json
  - .env
  - *.pem
  - *.key
```

Supports glob patterns. Protected paths are checked in `read_file`, `write_file`, `edit_file`, and `bash`.

### SSH Host Allowlist

```yaml
ssh_allowed_hosts: []   # Empty = any host allowed
```

When configured, only hosts in the list can be accessed via `ssh` and `scp`.

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

### Auto-Approve

Tools in the `auto_approve` list execute without any permission check:

```yaml
auto_approve: [web_search, memory_search]
```

Use with caution — completely bypasses the ToolGuard for those tools.

### Audit Log

**Every** tool execution (allowed or blocked) is logged:

```
2025-01-15T14:30:22Z | ALLOWED | owner:5511999999999 | bash | {"command": "ls -la"}
2025-01-15T14:30:45Z | BLOCKED | user:5511888888888 | bash | {"command": "rm -rf /"} | reason: destructive command
```

Fields: timestamp, result (ALLOWED/BLOCKED), role:user, tool, arguments, reason (if blocked).

```yaml
security:
  tool_guard:
    audit_log: ./data/audit.log
```

---

## 3. SSRF Protection (`security/ssrf.go`)

Protection against Server-Side Request Forgery in `web_fetch` and similar tools.

### Mechanism

1. **URL parsing**: validates format and extracts hostname.
2. **Scheme validation**: only `http` and `https` are allowed. `file://`, `ftp://`, etc. are blocked.
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

### DNS Rebinding Protection

The SSRF Guard resolves the hostname to an IP **before** making the request, and validates the resulting IP. This prevents attacks where a malicious DNS first returns a public IP (to pass the check) and then a private IP (for the actual request).

### Configuration

```yaml
security:
  ssrf:
    allow_private: false           # Block private IPs (default)
    allowed_hosts: []              # Whitelist (empty = no host restriction)
    blocked_hosts:                 # Additional blacklist
      - "internal.corp.com"
```

---

## 4. Encrypted Vault (`vault.go`, `keyring.go`)

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

These parameters follow OWASP recommendations for resistance against GPU/ASIC attacks.

### Vault Format

```json
{
  "version": 1,
  "salt": "base64-encoded-salt",
  "entries": {
    "openai_api_key": {
      "nonce": "base64-encoded-nonce",
      "ciphertext": "base64-encoded-encrypted-data"
    }
  }
}
```

- Each entry has its own nonce (never reused).
- Salt is global to the vault (key derivation).
- Master password is **never** stored — only the derived key is held in memory while the vault is unlocked.

### Secret Resolution Chain

```
1. Encrypted vault (.goclaw.vault)     → AES-256-GCM
2. OS keyring (GNOME/macOS/Windows)    → Native OS API
3. Environment variable                 → GOCLAW_API_KEY
4. config.yaml                          → ${GOCLAW_API_KEY}
```

First match wins. The priority ensures the encrypted vault is always preferred.

### Management

```bash
copilot config vault-init              # Create vault with master password
copilot config vault-set               # Store credential
copilot config vault-status            # Vault status
copilot config vault-change-password   # Change master password
```

Via agent tools:
- `vault_save <key> <value>` — store in vault
- `vault_get <key>` — retrieve from vault
- `vault_list` — list stored keys
- `vault_delete <key>` — remove from vault

### Additional Protections

- The `.goclaw.vault` file is in the protected paths list — cannot be read by non-owners.
- Secrets are never logged in audit log, daily notes, or MEMORY.md.
- `sync.RWMutex` protects concurrent vault operations.

---

## 5. Script Sandbox (`sandbox/`)

Execution isolation for community skill scripts.

### Isolation Levels

| Level | Method | Use Case | Risk |
|-------|--------|----------|------|
| `none` | Direct `exec.Command` | Builtin/trusted skills | High |
| `restricted` | Linux namespaces + seccomp + cgroups | Community skills | Medium |
| `container` | Docker with purpose-built image | Untrusted scripts | Low |

### Restricted Sandbox (Linux Namespaces)

Docker-free isolation via syscall:

- **PID namespace**: isolated processes (cannot see host processes).
- **Mount namespace**: separate filesystem, read-only workspace.
- **Network namespace**: isolated networking (no access to host network).
- **User namespace**: remapped UID/GID (no root on host).

### Resource Limits

```yaml
sandbox:
  default_isolation: restricted
  timeout: 60s              # Timeout per execution
  max_output_bytes: 1048576 # 1 MB stdout+stderr
  max_memory_mb: 256        # RAM limit
  max_cpu_percent: 50       # CPU limit
```

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

### Policy Engine

```go
// Binary allowlist — only permitted binaries can execute
AllowedBinaries: ["python3", "node", "sh", "bash"]

// Environment filtering — dangerous variables removed
BlockedEnvVars: ["LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES"]
```

### Supported Runtimes

| Runtime | Binary | Extensions |
|---------|--------|------------|
| Python | `python3` | `.py` |
| Node.js | `node` | `.js`, `.mjs` |
| Shell | `sh`, `bash` | `.sh` |
| Binary | (direct) | (executable) |

---

## 6. Gateway Authentication (`gateway/`)

Bearer token authentication for the HTTP API:

```yaml
gateway:
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

- Requests without a valid token receive `401 Unauthorized`.
- Configurable CORS to control allowed origins.
- Token is verified via the `Authorization: Bearer <token>` header.

---

## Security Matrix

Summary of protections by attack vector:

| Attack Vector | Protection | Layer |
|---------------|------------|-------|
| Unauthorized access | Role-based access control | Access Control |
| Destructive commands | Regex blocking + confirmation | Tool Guard |
| Secret reading | Protected paths + vault encryption | Tool Guard + Vault |
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
