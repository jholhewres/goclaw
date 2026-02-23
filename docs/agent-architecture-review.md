# DevClaw Agent Architecture Review

> Comparative analysis with OpenClaw and recommendations for autonomous agent improvements

## Executive Summary

DevClaw has a solid foundation with vault-based secrets management and hot-reload capabilities. However, compared to OpenClaw, it lacks several key features for autonomous operation:

| Feature | DevClaw | OpenClaw | Priority |
|---------|---------|----------|----------|
| Heartbeat/Wake System | Basic | Advanced (per-agent intervals) | HIGH |
| Cron Scheduling | Simple (every/at) | Full cron + backoff + auto-disable | HIGH |
| Hot Config Reload | Polling-based | Event-based with rules | MEDIUM |
| Auth Profiles | Single vault | Multi-profile with fallback | MEDIUM |
| Tool Policy Groups | Flat list | Hierarchical groups + profiles | MEDIUM |
| Skills Auto-Refresh | Manual restart | File watching | LOW |

---

## 1. VAULT OPERATIONS REVIEW

### Current Implementation ✅

```
Vault (AES-256-GCM + Argon2id)
    │
    ├─→ Create(password) - Initialize vault
    ├─→ Unlock(password) - Decrypt entries
    ├─→ Set(key, value) - Store secret
    ├─→ Get(key) - Retrieve secret
    ├─→ Lock() - Secure memory
    └─→ InjectVaultEnvVars() - Export to os.Env
```

### Resolution Chain (Working)
```
1. Vault (.devclaw.vault) ← Most Secure
2. OS Keyring
3. Environment Variables
4. .env file
5. config.yaml
```

### Issues Found ⚠️

1. **No automatic vault usage in agent tools**
   - The agent doesn't automatically know to use vault for secrets
   - Skills need to explicitly call vault tools
   - Recommendation: Add vault-aware tools that auto-resolve secrets

2. **No credential rotation tracking**
   - OpenClaw tracks failures per credential and auto-rotates
   - DevClaw has no failure tracking

3. **Single vault for all contexts**
   - OpenClaw has per-agent credential isolation
   - DevClaw shares vault across all workspaces

### Recommended Improvements

```go
// Add to assistant.go
type VaultOperation struct {
    Operation string // "get", "set", "delete", "list"
    Key       string
    Value     string // for set
    Result    string
    Error     string
}

// Auto-resolve secrets in config expansion
func (a *Assistant) ResolveSecret(key string) (string, error) {
    // 1. Check vault
    if a.vault != nil && a.vault.IsUnlocked() {
        if val, err := a.vault.Get(key); err == nil {
            return val, nil
        }
    }
    // 2. Check keyring
    if val := GetKeyring(key); val != "" {
        return val, nil
    }
    // 3. Check env
    return os.Getenv(key), nil
}
```

---

## 2. CONFIG HOT-RELOAD REVIEW

### Current Implementation ⚠️

```go
// config_watcher.go - Polling based
type ConfigWatcher struct {
    path       string
    interval   time.Duration  // Default: 5s
    lastHash   string
    onChange   func(*Config)
}
```

### Issues Found

1. **No reload rules**
   - OpenClaw has granular rules: `restart`, `hot`, `none`
   - DevClaw applies all changes uniformly

2. **No diff-based change detection**
   - Full config reload even for minor changes
   - Risk of disrupting active operations

3. **Skills require restart**
   - OpenClaw auto-refreshes skills via file watching
   - DevClaw needs manual restart

### Recommended Improvements

```go
type ReloadRule struct {
    Path     string // JSON path in config
    Strategy string // "hot", "restart", "none"
}

var reloadRules = []ReloadRule{
    {Path: "access", Strategy: "hot"},
    {Path: "security.tool_guard", Strategy: "hot"},
    {Path: "channels", Strategy: "restart"},
    {Path: "database.path", Strategy: "restart"},
}

func (cw *ConfigWatcher) detectChanges(old, new *Config) []ConfigChange {
    // Return specific changes with reload strategies
}
```

---

## 3. AGENT EXECUTION REVIEW

### Current Flow ✅

```
Message → Context Build → LLM Call → Tool Exec → Response
              │               │            │
              ▼               ▼            ▼
         Memory Search   Fallbacks    Guard Check
         Skills Load     Retry        Hooks
         History         Streaming    Audit
```

### Missing Autonomous Features

1. **No Heartbeat System**
   - OpenClaw: Periodic wake-ups for autonomous task checking
   - DevClaw: Only responds to incoming messages

2. **No Self-Initiated Actions**
   - Agent cannot start tasks independently
   - All actions triggered by user messages or scheduled jobs

3. **Limited Loop Detection**
   - Current: Pattern-based detection
   - OpenClaw: Semantic analysis + circuit breaker

### Recommended: Heartbeat System

```go
// New file: pkg/devclaw/copilot/heartbeat.go
type HeartbeatConfig struct {
    Enabled   bool          `yaml:"enabled"`
    Interval  time.Duration `yaml:"interval"`  // Default: 5m
    Tasks     []HeartbeatTask
}

type HeartbeatTask struct {
    Name     string
    Prompt   string
    Channel  string
    ChatID   string
    OnEvent  string // "startup", "interval", "idle"
}

type HeartbeatManager struct {
    config    HeartbeatConfig
    assistant *Assistant
    ticker    *time.Ticker
    lastMsg   time.Time
}

func (h *HeartbeatManager) Start(ctx context.Context) {
    // Wake agent periodically to check for tasks
}
```

---

## 4. JOB/SCHEDULER REVIEW

### Current Implementation ⚠️

```go
type Job struct {
    ID              string
    Schedule        string  // cron, @every, at
    Command         string
    Channel         string
    ChatID          string
    // ...
}
```

### Missing Features vs OpenClaw

| Feature | DevClaw | OpenClaw |
|---------|---------|----------|
| Full cron expressions | ✅ | ✅ |
| Timezone support | ❌ | ✅ |
| Error backoff | ❌ | ✅ |
| Auto-disable on failures | ❌ | ✅ |
| Staggered execution | ✅ | ✅ |
| Isolated session per run | ✅ | ✅ |
| Wake-at optimization | ❌ | ✅ |

### Recommended: Error Backoff

```go
// Add to Job struct
type Job struct {
    // ... existing fields
    ConsecutiveErrors int       `json:"consecutive_errors"`
    LastErrorAt       time.Time `json:"last_error_at"`
    AutoDisabled      bool      `json:"auto_disabled"`
    BackoffUntil      time.Time `json:"backoff_until"`
}

func (s *Scheduler) executeJobWithBackoff(job *Job) {
    // Exponential backoff: 1m, 5m, 15m, 1h, 6h
    if job.ConsecutiveErrors > 0 {
        backoff := calculateBackoff(job.ConsecutiveErrors)
        job.BackoffUntil = time.Now().Add(backoff)
    }

    // Auto-disable after 5 consecutive errors
    if job.ConsecutiveErrors >= 5 {
        job.AutoDisabled = true
        s.logger.Warn("job auto-disabled", "id", job.ID)
    }
}
```

---

## 5. TOOL POLICY REVIEW

### Current Implementation ⚠️

```go
type ToolGuardConfig struct {
    Enabled             bool
    AllowDestructive    bool
    AllowSudo           bool
    RequireConfirmation []string
    ToolPermissions     map[string]string // tool -> access level
}
```

### Missing Features vs OpenClaw

1. **No Tool Groups**
   - OpenClaw: `group:memory`, `group:fs`, `group:runtime`
   - DevClaw: Individual tool permissions only

2. **No Tool Profiles**
   - OpenClaw: `minimal`, `coding`, `messaging`, `full`
   - DevClaw: Manual configuration required

3. **No Owner-Only Enforcement**
   - OpenClaw: Dangerous tools blocked for non-owners
   - DevClaw: Relies on access level only

### Recommended: Tool Groups

```go
var ToolGroups = map[string][]string{
    "group:memory":    {"memory"},  // Dispatcher tool with actions
    "group:fs":        {"read_file", "write_file", "list_directory"},
    "group:runtime":   {"run_script", "bash"},
    "group:web":       {"web_search", "web_fetch"},
    "group:messaging": {"send_message", "send_reaction"},
}

type ToolProfile struct {
    Name        string
    AllowGroups []string
    DenyTools   []string
    ConfirmFor  []string
}

var DefaultProfiles = map[string]ToolProfile{
    "minimal": {
        AllowGroups: []string{"group:messaging"},
    },
    "coding": {
        AllowGroups: []string{"group:fs", "group:runtime", "group:memory"},
    },
    "full": {
        AllowGroups: []string{"group:*"},
    },
}
```

---

## 6. CRITICAL LAYERS FOR UNIT TESTS

### High Priority (Core Logic)

| Layer | File | Functions to Test | Coverage |
|-------|------|-------------------|----------|
| **Vault** | `vault.go` | Create, Unlock, Set, Get, Lock | ❌ Missing |
| **Config Watcher** | `config_watcher.go` | Detect changes, Hash comparison | ❌ Missing |
| **Access Control** | `access.go` | Check, Grant, Revoke, Levels | ⚠️ Partial |
| **Tool Guard** | `tool_guard.go` | Check, Path validation, Audit | ⚠️ Partial |
| **LLM Client** | `llm.go` | Error classification, Retry logic | ❌ Missing |
| **Agent Loop** | `agent.go` | Loop detection, Compaction | ⚠️ Partial |
| **Scheduler** | `scheduler.go` | Add, Remove, Execute, Stagger | ⚠️ Partial |

### Medium Priority (Integration)

| Layer | File | Functions to Test |
|-------|------|-------------------|
| **Session Store** | `session.go` | Create, History, Prune |
| **Memory Search** | `memory/sqlite_store.go` | Search, Index, Chunk |
| **Workspace Manager** | `workspace.go` | Resolve, Sessions, Config |
| **Hook Manager** | `hooks.go` | Register, Emit, Before/After |
| **Pairing Manager** | `pairing.go` | Token generation, Validation |
| **Group Policy** | `group_policy.go` | ShouldRespond, QuietHours |

### Low Priority (Utilities)

| Layer | File | Functions to Test |
|-------|------|-------------------|
| **Prompt Composer** | `prompt.go` | Layers, Skills, Memory |
| **Usage Tracker** | `usage.go` | Record, Aggregate |
| **Message Queue** | `queue.go` | Enqueue, Dequeue, Debounce |

---

## 7. RECOMMENDED TEST FILES

### Create These Test Files

```
pkg/devclaw/copilot/
├── vault_test.go          ← HIGH PRIORITY
├── config_watcher_test.go ← HIGH PRIORITY
├── llm_retry_test.go      ← HIGH PRIORITY
├── agent_loop_test.go     ← HIGH PRIORITY
├── access_test.go         ← MEDIUM
├── workspace_test.go      ← MEDIUM
├── memory_test.go         ← MEDIUM
├── hooks_test.go          ← MEDIUM
└── pairing_test.go        ← MEDIUM

pkg/devclaw/scheduler/
├── scheduler_test.go      ← Exists, needs expansion
└── job_backoff_test.go    ← NEW
```

---

## 8. ACTION PLAN

### Phase 1: Critical Tests (Week 1)

1. **Vault Tests** - Test encryption/decryption cycle
2. **Config Watcher Tests** - Test change detection
3. **LLM Retry Tests** - Test error classification and backoff
4. **Agent Loop Tests** - Test loop detection and compaction

### Phase 2: Autonomous Features (Week 2)

1. **Heartbeat System** - Add periodic wake capability
2. **Job Error Backoff** - Implement exponential backoff
3. **Tool Groups** - Add group-based permissions

### Phase 3: Integration Tests (Week 3)

1. **End-to-End Flows** - Message → Agent → Response
2. **Scheduler Integration** - Job → Agent → Channel
3. **Hot Reload Integration** - Config change → Component update

---

## APPENDIX: Test Examples

### Vault Test Example

```go
func TestVaultOperations(t *testing.T) {
    tmpFile := filepath.Join(t.TempDir(), "test.vault")
    vault := NewVault(tmpFile)

    t.Run("create and unlock", func(t *testing.T) {
        err := vault.Create("test-password")
        require.NoError(t, err)
        require.True(t, vault.Exists())

        err = vault.Unlock("test-password")
        require.NoError(t, err)
        require.True(t, vault.IsUnlocked())
    })

    t.Run("set and get", func(t *testing.T) {
        err := vault.Set("api_key", "secret123")
        require.NoError(t, err)

        val, err := vault.Get("api_key")
        require.NoError(t, err)
        require.Equal(t, "secret123", val)
    })

    t.Run("wrong password fails", func(t *testing.T) {
        vault.Lock()
        err := vault.Unlock("wrong-password")
        require.Error(t, err)
        require.False(t, vault.IsUnlocked())
    })

    t.Run("persistence across instances", func(t *testing.T) {
        vault.Lock()
        vault2 := NewVault(tmpFile)
        err := vault2.Unlock("test-password")
        require.NoError(t, err)

        val, err := vault2.Get("api_key")
        require.NoError(t, err)
        require.Equal(t, "secret123", val)
    })
}
```

### Config Watcher Test Example

```go
func TestConfigWatcherDetectsChanges(t *testing.T) {
    tmpFile := filepath.Join(t.TempDir(), "config.yaml")
    initialCfg := "name: test\nmodel: gpt-4"
    require.NoError(t, os.WriteFile(tmpFile, []byte(initialCfg), 0644))

    changeDetected := make(chan *Config, 1)
    watcher := NewConfigWatcher(tmpFile, 100*time.Millisecond, func(cfg *Config) {
        changeDetected <- cfg
    }, slog.Default())

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go watcher.Start(ctx)

    // Wait for initial load
    time.Sleep(200 * time.Millisecond)

    // Modify config
    newCfg := "name: test-modified\nmodel: gpt-4"
    require.NoError(t, os.WriteFile(tmpFile, []byte(newCfg), 0644))

    select {
    case cfg := <-changeDetected:
        require.Equal(t, "test-modified", cfg.Name)
    case <-time.After(2 * time.Second):
        t.Fatal("timeout waiting for config change")
    }
}
```

### LLM Retry Test Example

```go
func TestLLMErrorClassification(t *testing.T) {
    tests := []struct {
        statusCode int
        body       string
        expected   LLMErrorType
    }{
        {429, `{"error": "rate limit"}`, LLMErrorRateLimit},
        {500, `{"error": "internal"}`, LLMErrorRetryable},
        {502, ``, LLMErrorRetryable},
        {401, `{"error": "invalid key"}`, LLMErrorAuth},
        {400, `{"error": "bad request"}`, LLMErrorBadRequest},
    }

    for _, tt := range tests {
        t.Run(fmt.Sprintf("%d_%s", tt.statusCode, tt.expected), func(t *testing.T) {
            resp := &http.Response{StatusCode: tt.statusCode, Body: io.NopCloser(strings.NewReader(tt.body))}
            errType := classifyError(resp, errors.New("test"))
            require.Equal(t, tt.expected, errType)
        })
    }
}
```
