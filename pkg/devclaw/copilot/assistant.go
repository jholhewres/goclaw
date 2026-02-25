// Package copilot implements the main orchestrator for DevClaw.
// Coordinates channels, skills, scheduler, access control, workspaces,
// and security to process user messages and generate LLM responses.
package copilot

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/security"
	"github.com/jholhewres/devclaw/pkg/devclaw/database"
	"github.com/jholhewres/devclaw/pkg/devclaw/media"
	"github.com/jholhewres/devclaw/pkg/devclaw/sandbox"
	"github.com/jholhewres/devclaw/pkg/devclaw/scheduler"
	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
	"github.com/jholhewres/devclaw/pkg/devclaw/tts"
)

// Assistant is the main orchestrator for DevClaw.
// Message flow: receive → access check → command check → trigger check →
// workspace resolve → input validation → context build → agent → output validation → send.
type Assistant struct {
	config *Config

	// channelMgr manages communication channels.
	channelMgr *channels.Manager

	// accessMgr manages access control (who can use the bot).
	accessMgr *AccessManager

	// workspaceMgr manages isolated workspaces/profiles.
	workspaceMgr *WorkspaceManager

	// llmClient communicates with the LLM provider API.
	llmClient *LLMClient

	// toolExecutor manages tool registration and dispatches tool calls from the LLM.
	toolExecutor *ToolExecutor

	// approvalMgr manages pending tool approvals for RequireConfirmation tools.
	approvalMgr *ApprovalManager

	// skillRegistry manages available skills.
	skillRegistry *skills.Registry

	// skillDB provides database storage for skills to persist structured data.
	skillDB *SkillDB

	// scheduler manages scheduled tasks.
	scheduler *scheduler.Scheduler

	// sessionStore manages sessions for the default workspace (backward compat).
	sessionStore *SessionStore

	// promptComposer builds layered prompts.
	promptComposer *PromptComposer

	// inputGuard validates inputs before processing.
	inputGuard *security.InputGuardrail

	// outputGuard validates outputs before sending.
	outputGuard *security.OutputGuardrail

	// memoryStore provides persistent long-term memory (file-based, always available).
	memoryStore *memory.FileStore

	// sqliteMemory provides advanced memory with FTS5 + vector search.
	sqliteMemory *memory.SQLiteStore

	// subagentMgr orchestrates subagent spawning and lifecycle.
	subagentMgr *SubagentManager

	// hookMgr manages lifecycle hooks (16+ events).
	hookMgr *HookManager

	// heartbeat runs periodic proactive checks (stored for config hot-reload).
	heartbeat *Heartbeat

	// messageQueue handles message bursts with debouncing per session.
	messageQueue *MessageQueue

	// activeRuns tracks cancel functions for in-flight agent runs (key: workspaceID:sessionID).
	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	// interruptInboxes maps sessionID (channel:chatID) → channel for injecting
	// follow-up messages into active agent runs. When a user sends a message
	// while the agent is processing, the enriched content is pushed here so the
	// agent loop picks it up on its next turn (Claude Code-style).
	interruptInboxes   map[string]chan string
	interruptInboxesMu sync.Mutex

	// followupQueues holds messages received while a session is busy.
	// Unlike interrupt injection (which waits for the current tool to finish),
	// followup messages are processed as NEW agent runs after the current run
	// completes.
	followupQueues   map[string][]*channels.IncomingMessage
	followupQueuesMu sync.Mutex

	// usageTracker records token usage and estimated costs per session.
	usageTracker *UsageTracker

	// vault provides encrypted secret storage (nil if unavailable/locked).
	vault *Vault

	// projectMgr manages registered development projects.
	projectMgr *ProjectManager

	// devclawDB is the central SQLite database (devclaw.db) shared by the
	// scheduler, session persistence, and audit logger.
	devclawDB *sql.DB

	// dbHub is the Database Hub providing unified database access with
	// multi-backend support (SQLite, PostgreSQL, MySQL). When using SQLite,
	// dbHub.DB() returns the same connection as devclawDB.
	dbHub *database.Hub

	// ttsProvider handles text-to-speech synthesis (nil if TTS is disabled).
	ttsProvider tts.Provider

	// loopDetectorConfig holds tool loop detection config for creating per-run detectors.
	loopDetectorConfig ToolLoopConfig

	// daemonMgr manages background processes (dev servers, watchers, etc.).
	daemonMgr *DaemonManager

	// pluginMgr manages installed plugins (GitHub, Jira, Sentry, etc.).
	pluginMgr *PluginManager

	// userMgr handles multi-user operations when team mode is enabled.
	userMgr *UserManager

	// teamMgr manages persistent agents and team memory.
	teamMgr *TeamManager

	// maintenanceMgr manages maintenance mode state.
	maintenanceMgr *MaintenanceManager

	// systemCommands handles system administration commands.
	systemCommands *SystemCommands

	// pairingMgr manages DM pairing tokens and requests.
	pairingMgr *PairingManager

	// agentRouter routes messages to specialized agent profiles.
	agentRouter *AgentRouter

	// groupPolicyMgr manages group-specific policies and activation modes.
	groupPolicyMgr *GroupPolicyManager

	// webhookMgr manages external webhook delivery.
	webhookMgr *WebhookManager

	// metricsCollector collects and reports system metrics.
	metricsCollector *MetricsCollector

	// memoryIndexer performs background memory indexing.
	memoryIndexer *MemoryIndexer

	// mediaSvc provides native media handling (upload, enrich, send).
	mediaSvc *media.MediaService

	// configMu protects hot-reloadable config fields.
	configMu sync.RWMutex

	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Assistant with all dependencies.
func New(cfg *Config, logger *slog.Logger) *Assistant {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	te := NewToolExecutor(logger)
	te.Configure(cfg.Security.ToolExecutor)

	// Initialize the tool security guard.
	toolGuard := NewToolGuard(cfg.Security.ToolGuard, logger)
	te.SetGuard(toolGuard)

	// Initialize approval manager for RequireConfirmation tools.
	approvalMgr := NewApprovalManager(logger)

	// Initialize project manager for coding skills.
	dataDir := filepath.Dir(cfg.Memory.Path)
	if dataDir == "" || dataDir == "." {
		dataDir = "./data"
	}
	projectMgr := NewProjectManager(dataDir)

	// Create assistant first (needed for onDrain closure).
	a := &Assistant{
		config:         cfg,
		channelMgr:     channels.NewManager(logger.With("component", "channels")),
		accessMgr:      NewAccessManager(cfg.Access, logger),
		workspaceMgr:   NewWorkspaceManager(cfg, cfg.Workspaces, logger),
		llmClient:      NewLLMClient(cfg, logger),
		toolExecutor:   te,
		approvalMgr:    approvalMgr,
		skillRegistry:  skills.NewRegistry(logger.With("component", "skills")),
		sessionStore:   NewSessionStore(logger.With("component", "sessions")),
		promptComposer: NewPromptComposer(cfg),
		inputGuard:     security.NewInputGuardrail(cfg.Security.MaxInputLength, cfg.Security.RateLimit),
		outputGuard:    security.NewOutputGuardrail(),
		subagentMgr:    NewSubagentManager(cfg.Subagents, logger),
		hookMgr:        NewHookManager(logger),
		projectMgr:      projectMgr,
		activeRuns:       make(map[string]context.CancelFunc),
		interruptInboxes: make(map[string]chan string),
		followupQueues:   make(map[string][]*channels.IncomingMessage),
		usageTracker:     NewUsageTracker(logger.With("component", "usage")),
		logger:           logger,
	}

	// Initialize tool loop detection config (detectors are created per-run to avoid races).
	// Use defaults, then apply user overrides. NewToolLoopDetector normalizes zero-values.
	a.loopDetectorConfig = cfg.Agent.ToolLoop
	if !a.loopDetectorConfig.Enabled && a.loopDetectorConfig.HistorySize == 0 {
		// No explicit config provided (all zero-values) → use defaults (enabled by default).
		a.loopDetectorConfig = DefaultToolLoopConfig()
	}

	// Wire message queue with onDrain callback (requires assistant reference).
	debounceMs := cfg.Queue.DebounceMs
	if debounceMs <= 0 {
		debounceMs = 1000
	}
	maxPending := cfg.Queue.MaxPending
	if maxPending <= 0 {
		maxPending = 20
	}
	a.messageQueue = NewMessageQueue(debounceMs, maxPending, a.handleDrainedMessages, logger)

	// Wire confirmation requester for tools in RequireConfirmation list.
	te.SetConfirmationRequester(func(sessionID, callerJID, toolName string, args map[string]any) (bool, error) {
		sendMsg := func(msg string) {
			channel, chatID, ok := strings.Cut(sessionID, ":")
			if !ok {
				return
			}
			_ = a.channelMgr.Send(a.ctx, channel, chatID, &channels.OutgoingMessage{Content: msg})
		}
		return approvalMgr.Request(sessionID, callerJID, toolName, args, sendMsg)
	})

	// Wire subagent announce callback: when a subagent completes, inject the
	// result back into the parent session so the main agent can process and
	// reformulate it (matching approach). This allows the agent to
	// synthesize multiple subagent results and maintain conversation context.
	a.subagentMgr.SetAnnounceCallback(func(run *SubagentRun) {
		// Build session ID from origin coordinates.
		channel := run.OriginChannel
		chatID := run.OriginTo
		if channel == "" || chatID == "" {
			// Fallback: derive from ParentSessionID ("channel:chatID").
			var ok bool
			channel, chatID, ok = strings.Cut(run.ParentSessionID, ":")
			if !ok {
				return
			}
		}
		sessionID := MakeSessionID(channel, chatID)

		// Build the system message for the main agent.
		var msg string
		switch run.Status {
		case SubagentStatusCompleted:
			result := run.Result
			if len(result) > 4000 {
				result = result[:4000] + "\n... (truncated)"
			}
			msg = fmt.Sprintf("[System Message] A subagent task %q just completed successfully.\n\nResult:\n%s\n\nConvert this result into your normal assistant voice and send that user-facing update now. Keep this internal context private (don't mention system/log/stats details), and do not copy the system message verbatim. Reply ONLY: NO_REPLY if this exact result was already delivered to the user in this same turn.",
				run.Label, result)
		case SubagentStatusFailed:
			msg = fmt.Sprintf("[System Message] A subagent task %q failed after %s: %s\n\nLet the user know about this failure briefly and offer to retry or investigate.",
				run.Label, run.Duration.Round(time.Second), run.Error)
		case SubagentStatusTimeout:
			msg = fmt.Sprintf("[System Message] A subagent task %q timed out after %s.\n\nLet the user know about this timeout briefly and offer to retry.",
				run.Label, run.Duration.Round(time.Second))
		default:
			return
		}

		// Inject as a follow-up message into the parent session.
		// This triggers a new agent run to process the subagent result.
		a.enqueueFollowupMessage(sessionID, msg, channel, chatID)
	})

	return a
}

// Start initializes and starts all subsystems.
func (a *Assistant) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.logger.Info("starting DevClaw Copilot",
		"name", a.config.Name,
		"model", a.config.Model,
		"access_policy", a.config.Access.DefaultPolicy,
		"workspaces", a.workspaceMgr.Count(),
	)

	// 0pre. Inject vault secrets as environment variables so skills and scripts
	// can access them via os.Getenv / process.env without needing .env files.
	// This runs once at startup with zero runtime cost.
	if a.vault != nil && a.vault.IsUnlocked() {
		a.InjectVaultEnvVars()
	}

	// 0pre-b. Auto-resolve media transcription provider from main API config.
	a.config.Media.ResolveForProvider(a.config.API.Provider, a.config.API.BaseURL)

	// 0. Initialize memory stores.
	memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
	memStore, err := memory.NewFileStore(memDir)
	if err != nil {
		a.logger.Warn("memory store not available", "error", err)
	} else {
		a.memoryStore = memStore
	}

	// 0a. Initialize SQLite memory with FTS5 + vector search (if configured).
	if a.config.Memory.Type == "sqlite" {
		embedCfg := a.config.Memory.Embedding
		// Use main API key if embedding key not set.
		if embedCfg.APIKey == "" {
			embedCfg.APIKey = a.config.API.APIKey
		}
		embedder := memory.NewEmbeddingProvider(embedCfg)

		dbPath := a.config.Memory.Path
		if dbPath == "" {
			dbPath = "./data/memory.db"
		}

		sqlStore, err := memory.NewSQLiteStore(dbPath, embedder, a.logger.With("component", "memory-index"))
		if err != nil {
			a.logger.Warn("SQLite memory store not available, falling back to file-based",
				"error", err)
		} else {
			a.sqliteMemory = sqlStore
			a.logger.Info("SQLite memory store initialized",
				"embedding_provider", embedder.Name(),
				"db", dbPath,
			)

			// Index memory files in background (fire-and-forget).
			if a.config.Memory.Index.Auto {
				go func() {
					chunkCfg := memory.ChunkConfig{
						MaxTokens: a.config.Memory.Index.ChunkMaxTokens,
						Overlap:   100,
					}
					if chunkCfg.MaxTokens <= 0 {
						chunkCfg.MaxTokens = 500
					}
					if err := sqlStore.IndexMemoryDir(a.ctx, memDir, chunkCfg); err != nil {
						a.logger.Warn("initial memory indexing failed", "error", err)
					}
				}()
			}
		}
	}

	// 0b. Connect memory store and skill getter to prompt composer.
	if a.memoryStore != nil {
		a.promptComposer.SetMemoryStore(a.memoryStore)
	}
	if a.sqliteMemory != nil {
		a.promptComposer.SetSQLiteMemory(a.sqliteMemory)
	}
	a.promptComposer.SetSkillGetter(func(name string) (interface{ SystemPrompt() string }, bool) {
		skill, ok := a.skillRegistry.Get(name)
		if !ok {
			return nil, false
		}
		return skill, true
	})

	// Load built-in skills (embedded in binary).
	builtinSkills := LoadBuiltinSkills(a.logger.With("component", "builtin-skills"))
	a.promptComposer.SetBuiltinSkills(builtinSkills)

	// Wire tool executor to prompt composer for dynamic tool list generation.
	a.promptComposer.SetToolExecutor(a.toolExecutor)

	// 0c. Open the central devclaw.db and wire all SQLite-backed storage.
	// Uses the Database Hub for unified access (supports SQLite, PostgreSQL, MySQL).
	hubConfig := a.config.Database.Effective()
	dbHub, hubErr := database.NewHub(hubConfig, a.logger.With("component", "database-hub"))
	if hubErr != nil {
		a.logger.Error("failed to initialize database hub, falling back to file-based storage",
			"backend", hubConfig.Backend, "error", hubErr)
	} else {
		a.dbHub = dbHub

		// Run database migrations (creates all tables if needed)
		if err := dbHub.Migrate(context.Background(), "primary", 0); err != nil {
			a.logger.Error("failed to run database migrations", "error", err)
		}

		// Get the underlying DB connection for backward compatibility
		if dbHub.DB() != nil {
			a.devclawDB = dbHub.DB()
			a.logger.Info("database hub initialized",
				"backend", hubConfig.Backend,
				"path", hubConfig.SQLite.Path)

			// Migrate legacy JSON/JSONL data to SQLite (one-time, idempotent).
			dataDir := filepath.Dir(hubConfig.SQLite.Path)
			MigrateToSQLite(a.devclawDB, dataDir, a.logger.With("component", "migrate"))
		}
	}

	// 0c-1. Session persistence: prefer SQLite, fall back to JSONL.
	var sessPersister SessionPersister
	if a.devclawDB != nil {
		sessPersister = NewSQLiteSessionPersistence(a.devclawDB, a.logger.With("component", "session-persist"))
		a.sessionStore.SetPersistence(sessPersister)
		a.logger.Info("session persistence enabled (SQLite)")
	} else {
		sessDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "sessions")
		if sessDir == "" {
			sessDir = "./data/sessions"
		}
		sp, err := NewSessionPersistence(sessDir, a.logger.With("component", "session-persist"))
		if err != nil {
			a.logger.Warn("session persistence not available", "error", err)
		} else {
			sessPersister = sp
			a.sessionStore.SetPersistence(sessPersister)
			a.logger.Info("session persistence enabled (JSONL)", "dir", sessDir)
		}
	}

	// Propagate persistence to workspace session stores so channel conversations
	// (WhatsApp, Telegram, etc.) survive container restarts.
	if sessPersister != nil && a.workspaceMgr != nil {
		a.workspaceMgr.SetPersistence(sessPersister)
	}

	// 0c-2. Audit logger: prefer SQLite, fall back to file-based.
	if a.devclawDB != nil {
		if guard := a.toolExecutor.Guard(); guard != nil {
			auditLogger := NewSQLiteAuditLogger(a.devclawDB, a.logger.With("component", "audit"))
			guard.SetSQLiteAudit(auditLogger)
			a.logger.Info("audit logging enabled (SQLite)")
		}
	}

	// 0c-3. Subagent persistence: wire SQLite for run history across restarts.
	if a.devclawDB != nil {
		a.subagentMgr.SetDB(a.devclawDB)
		// Auto-prune runs older than 7 days on startup.
		a.subagentMgr.PruneOldRuns(7)
		// Clean up stale "running" entries from previous crashes.
		a.subagentMgr.cleanupStaleRunning()
		a.logger.Info("subagent persistence enabled (SQLite)")
	}

	// 0c-4. Maintenance manager for maintenance mode state.
	a.maintenanceMgr = NewMaintenanceManager(a.devclawDB, a.logger.With("component", "maintenance"))
	if err := a.maintenanceMgr.Load(); err != nil {
		a.logger.Warn("failed to load maintenance state", "error", err)
	}

	// 0c-5. System commands handler.
	a.systemCommands = NewSystemCommands(a, a.config.Database.Path, a.maintenanceMgr)

	// 0c-6. Pairing manager for DM access tokens.
	a.pairingMgr = NewPairingManager(a.devclawDB, a.accessMgr, a.workspaceMgr, a.logger)
	if err := a.pairingMgr.Load(); err != nil {
		a.logger.Warn("failed to load pairing tokens", "error", err)
	}

	// 0d. Agent router for specialized profiles.
	if len(a.config.Agents.Profiles) > 0 {
		a.agentRouter = NewAgentRouter(a.config.Agents, a.logger)
	}

	// 0e. Group policy manager for group-specific behavior.
	if len(a.config.Groups.Groups) > 0 || len(a.config.Groups.Blocked) > 0 {
		a.groupPolicyMgr = NewGroupPolicyManager(a.config.Groups, a.logger)
	}

	// 0f. Webhook manager for external webhook delivery.
	if a.config.Hooks.Enabled && len(a.config.Hooks.Webhooks) > 0 {
		a.webhookMgr = NewWebhookManager(WebhooksConfig{
			Enabled:  a.config.Hooks.Enabled,
			Webhooks: a.config.Hooks.Webhooks,
		}, a.hookMgr, a.logger)
	}

	// 1. Register skill loaders and load all skills.
	a.registerSkillLoaders()
	if err := a.skillRegistry.LoadAll(a.ctx); err != nil {
		a.logger.Error("failed to load skills", "error", err)
	}

	// 1b. Initialize skills with sandbox runner.
	a.initializeSkills()

	// 1c. Register skill tools + system tools in the executor.
	a.registerSkillTools()

	// 1d. Create and start scheduler if enabled.
	if a.config.Scheduler.Enabled {
		a.initScheduler()
	}

	// 1d-2. Initialize TeamManager for persistent agents.
	if a.devclawDB != nil && a.scheduler != nil {
		a.teamMgr = NewTeamManager(a.devclawDB, a.scheduler, a.logger.With("component", "team-manager"))
		// Wire spawn callback for active notification push.
		a.teamMgr.SetSpawnAgentCallback(func(ctx context.Context, agent *PersistentAgent, task string) error {
			sessionID := fmt.Sprintf("agent:%s:run", agent.ID)
			a.enqueueFollowupMessage(sessionID, task, "team", agent.TeamID)
			a.logger.Info("persistent agent triggered via spawn callback", "agent_id", agent.ID)
			return nil
		})

		// Initialize notification dispatcher for team agents.
		notifConfig := &NotificationConfig{
			Enabled: true,
			Defaults: NotificationDefaults{
				ActivityFeed: true,
			},
		}
		notifDisp := NewNotificationDispatcher(
			a.devclawDB,
			a.teamMgr,
			a.channelMgr,
			a.hookMgr,
			notifConfig,
			a.logger.With("component", "notifications"),
		)
		a.teamMgr.SetNotificationDispatcher(notifDisp)
		a.logger.Info("team manager initialized with spawn callback and notification dispatcher")
	}

	// 1e. Register system tools (needs scheduler to be created first).
	a.registerSystemTools()

	// 2. Start channel manager (non-fatal: webui/gateway can work without channels).
	if err := a.channelMgr.Start(a.ctx); err != nil {
		a.logger.Warn("channels not connected yet (will retry in background)", "error", err)
	}

	// 3. Start session pruners for all workspaces.
	a.workspaceMgr.StartPruners(a.ctx)

	// 4. Start scheduler if created.
	if a.scheduler != nil {
		if err := a.scheduler.Start(a.ctx); err != nil {
			a.logger.Error("failed to start scheduler", "error", err)
		}
	}

	// 5. Start heartbeat if enabled.
	if a.config.Heartbeat.Enabled {
		a.heartbeat = NewHeartbeat(a.config.Heartbeat, a, a.logger)
		a.heartbeat.Start(a.ctx)
	}

	// 5b. Start metrics collector if enabled.
	if a.config.Routines.Metrics.Enabled {
		a.metricsCollector = NewMetricsCollector(a.config.Routines.Metrics, a.logger)
		// Wire callbacks for session and subagent counts
		a.metricsCollector.SetSessionsCountFunc(func() int64 {
			return int64(a.sessionStore.Count())
		})
		if a.subagentMgr != nil {
			a.metricsCollector.SetSubagentsCountFunc(func() int64 {
				return int64(a.subagentMgr.ActiveCount())
			})
		}
		if a.devclawDB != nil && a.config.Database.Path != "" {
			a.metricsCollector.SetDBSizeFunc(func() int64 {
				// Get DB file size
				info, err := os.Stat(a.config.Database.Path)
				if err != nil {
					return 0
				}
				return info.Size() / 1024 / 1024 // MB
			})
		}
		go a.metricsCollector.Start(a.ctx)
	}

	// 5c. Start memory indexer if enabled.
	if a.config.Routines.MemoryIndexer.Enabled && a.sqliteMemory != nil {
		a.memoryIndexer = NewMemoryIndexer(a.config.Routines.MemoryIndexer, a.logger)
		memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
		a.memoryIndexer.SetMemoryDir(memDir)
		// Wire callbacks for memory indexing
		a.memoryIndexer.SetIndexChunkFunc(func(chunks []MemoryChunk) error {
			// Convert to memory.Chunk format and index
			for _, c := range chunks {
				ctx := context.Background()
				mChunks := []memory.Chunk{{FileID: c.Filepath, Text: c.Content, Hash: c.Hash}}
				if err := a.sqliteMemory.IndexChunks(ctx, c.Filepath, mChunks, c.Hash); err != nil {
					return err
				}
			}
			return nil
		})
		go a.memoryIndexer.Start(a.ctx)
	}

	// 5d. Initialize native media service if enabled.
	if a.config.NativeMedia.Enabled {
		// Create media store
		storeCfg := media.StoreConfig{
			BaseDir:     a.config.NativeMedia.Store.BaseDir,
			TempDir:     a.config.NativeMedia.Store.TempDir,
			MaxFileSize: a.config.NativeMedia.Store.MaxFileSize,
		}
		mediaStore := media.NewFileSystemStore(storeCfg, a.logger)

		// Create service config
		svcCfg := media.ServiceConfig{
			Enabled:         true,
			MaxImageSize:    a.config.NativeMedia.Service.MaxImageSize,
			MaxAudioSize:    a.config.NativeMedia.Service.MaxAudioSize,
			MaxDocSize:      a.config.NativeMedia.Service.MaxDocSize,
			TempTTL:         a.config.NativeMedia.Service.TempTTL,
			CleanupEnabled:  a.config.NativeMedia.Service.CleanupEnabled,
			CleanupInterval: a.config.NativeMedia.Service.CleanupInterval,
		}

		// Get effective media config to check model capabilities
		mCfg := a.config.Media.Effective()

		// Create enrichment config - sync with model capabilities
		enrichCfg := media.EnrichmentConfig{
			// Only auto-enrich images if vision is enabled AND config says so
			AutoEnrichImages:    mCfg.VisionEnabled && a.config.NativeMedia.Enrichment.AutoEnrichImages,
			// Only auto-enrich audio if transcription is enabled AND config says so
			AutoEnrichAudio:     mCfg.TranscriptionEnabled && a.config.NativeMedia.Enrichment.AutoEnrichAudio,
			// Documents don't depend on external APIs
			AutoEnrichDocuments: a.config.NativeMedia.Enrichment.AutoEnrichDocuments,
		}

		// Build options list for media service
		opts := []media.MediaServiceOption{
			media.WithEnrichmentConfig(enrichCfg),
			media.WithDocumentExtraction(func(ctx context.Context, data []byte, mimeType string) (string, error) {
				return extractDocumentText(data, mimeType, "document", a.logger), nil
			}),
		}

		// Add vision callback only if supported
		if mCfg.VisionEnabled && a.llmClient != nil {
			opts = append(opts, media.WithVision(func(ctx context.Context, imageData []byte, mimeType string) (string, error) {
				encoded := base64.StdEncoding.EncodeToString(imageData)
				prompt := "Describe this image in detail. Include any visible text, objects, and context."
				// Pass vision model if configured, otherwise falls back to main model
				return a.llmClient.CompleteWithVision(ctx, "", encoded, mimeType, prompt, mCfg.VisionDetail, mCfg.VisionModel)
			}))
		}

		// Add transcription callback only if supported
		if mCfg.TranscriptionEnabled && a.llmClient != nil {
			opts = append(opts, media.WithTranscription(func(ctx context.Context, audioData []byte, filename string) (string, error) {
				return a.llmClient.TranscribeAudio(ctx, audioData, filename, mCfg.TranscriptionModel, mCfg)
			}))
		}

		// Create media service
		a.mediaSvc = media.NewMediaService(mediaStore, a.channelMgr, svcCfg, a.logger, opts...)

		// Start cleanup goroutine if enabled
		if a.config.NativeMedia.Service.CleanupEnabled {
			go a.mediaSvc.StartCleanup(a.ctx)
		}

		a.logger.Info("native media service initialized",
			"base_dir", storeCfg.BaseDir,
			"max_file_size", storeCfg.MaxFileSize,
			"vision_enabled", mCfg.VisionEnabled,
			"vision_model", mCfg.VisionModel,
			"transcription_enabled", mCfg.TranscriptionEnabled,
			"transcription_model", mCfg.TranscriptionModel,
		)
	}

	// 6. Start main message processing loop.
	go a.messageLoop()

	// 6b. Start session watchdog to recover stuck sessions.
	go a.sessionWatchdog()

	// 7. Run BOOT.md if present (gateway startup).
	// Executes after all channels are connected, with a short delay for stabilization.
	go a.runBootOnce()

	// 7b. Resume interrupted runs from previous process lifecycle.
	// Any agent runs that were active when the process last exited are
	// re-submitted so the user doesn't lose work in progress.
	go a.resumeInterruptedRuns()

	// 8. Initialize TTS provider if enabled.
	if a.config.TTS.Enabled {
		a.ttsProvider = a.buildTTSProvider()
		if a.ttsProvider != nil {
			a.logger.Info("TTS enabled", "provider", a.config.TTS.Provider, "voice", a.config.TTS.Voice, "mode", a.config.TTS.AutoMode)
		}
	}

	a.logger.Info("DevClaw Copilot started successfully")
	return nil
}

// runBootOnce executes BOOT.md instructions once after startup.
// If BOOT.md exists in the workspace, its content is fed to the agent as a
// startup command. This enables proactive behaviors like "check emails" or
// "review today's calendar" on boot.
func (a *Assistant) runBootOnce() {
	// Short delay to let channels stabilize.
	time.Sleep(500 * time.Millisecond)

	// Search for BOOT.md in the workspace directories.
	searchDirs := []string{"."}
	if a.config.Heartbeat.WorkspaceDir != "" && a.config.Heartbeat.WorkspaceDir != "." {
		searchDirs = append([]string{a.config.Heartbeat.WorkspaceDir}, searchDirs...)
	}
	searchDirs = append(searchDirs, "configs")

	var bootContent string
	for _, dir := range searchDirs {
		data, err := os.ReadFile(filepath.Join(dir, "BOOT.md"))
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			bootContent = strings.TrimSpace(string(data))
			break
		}
	}

	if bootContent == "" {
		return
	}

	a.logger.Info("executing BOOT.md startup instructions")

	// Create a dedicated session for boot.
	session := a.sessionStore.GetOrCreate("system", "boot")
	prompt := a.promptComposer.Compose(session, bootContent)

	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	result, err := agent.Run(a.ctx, prompt, nil, bootContent)
	if err != nil {
		a.logger.Error("BOOT.md execution failed", "error", err)
		return
	}

	session.AddMessage(bootContent, result)
	a.logger.Info("BOOT.md execution completed",
		"result_preview", truncate(result, 200),
	)
}

// GetMediaService returns the media service for WebUI adapter wiring.
// Returns nil if native media is not enabled.
func (a *Assistant) GetMediaService() *media.MediaService {
	return a.mediaSvc
}

// Stop gracefully shuts down all subsystems.
func (a *Assistant) Stop() {
	a.logger.Info("stopping DevClaw Copilot...")

	if a.cancel != nil {
		a.cancel()
	}

	// Shut down in reverse initialization order.
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	a.channelMgr.Stop()
	a.skillRegistry.ShutdownAll()

	// Close SQLite memory store.
	if a.sqliteMemory != nil {
		if err := a.sqliteMemory.Close(); err != nil {
			a.logger.Warn("error closing SQLite memory", "error", err)
		}
	}

	// Close skill database.
	if a.skillDB != nil {
		if err := a.skillDB.Close(); err != nil {
			a.logger.Warn("error closing skill database", "error", err)
		}
	}

	// Close central devclaw.db.
	if a.devclawDB != nil {
		if err := a.devclawDB.Close(); err != nil {
			a.logger.Warn("error closing devclaw.db", "error", err)
		}
	}

	a.logger.Info("DevClaw Copilot stopped")
}

// ApplyConfigUpdate applies hot-reloadable config changes. Updates: access control,
// instructions, tool guard, heartbeat, token budget. Does NOT update: API, channels,
// model, plugins (require restart).
func (a *Assistant) ApplyConfigUpdate(newCfg *Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.config.Instructions = newCfg.Instructions
	a.config.Access = newCfg.Access
	a.config.Security.ToolGuard = newCfg.Security.ToolGuard
	a.config.Security.ToolExecutor = newCfg.Security.ToolExecutor
	a.config.Heartbeat = newCfg.Heartbeat
	a.config.TokenBudget = newCfg.TokenBudget

	a.accessMgr.ApplyConfig(newCfg.Access)
	a.toolExecutor.UpdateGuardConfig(newCfg.Security.ToolGuard)
	a.toolExecutor.Configure(newCfg.Security.ToolExecutor)
	if a.heartbeat != nil {
		a.heartbeat.UpdateConfig(newCfg.Heartbeat)
	}

	a.logger.Info("config hot-reload applied",
		"updated", []string{"access", "instructions", "tool_guard", "heartbeat", "token_budget"},
	)
}

// UpdateMediaConfig safely updates the media configuration under lock.
func (a *Assistant) UpdateMediaConfig(media MediaConfig) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.config.Media = media
}

// MediaConfig returns the current effective media config under read lock.
func (a *Assistant) MediaConfig() MediaConfig {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config.Media.Effective()
}

// ChannelManager returns the channel manager for external registration.
func (a *Assistant) ChannelManager() *channels.Manager {
	return a.channelMgr
}

// SetVault sets the unlocked vault for the assistant (enables vault tools).
func (a *Assistant) SetVault(v *Vault) {
	a.vault = v
}

// Vault returns the vault instance (may be nil if unavailable).
func (a *Assistant) Vault() *Vault {
	return a.vault
}

// InjectVaultEnvVars loads all vault secrets as environment variables.
// Key names are uppercased and prefixed if not already (e.g. "brave_api_key" → "BRAVE_API_KEY").
// Existing env vars are NOT overwritten — vault only fills gaps.
// This allows skills/scripts to use process.env.BRAVE_API_KEY without .env files.
func (a *Assistant) InjectVaultEnvVars() {
	keys := a.vault.List()
	if len(keys) == 0 {
		return
	}

	injected := 0
	for _, key := range keys {
		envName := strings.ToUpper(key)

		// Don't overwrite existing env vars.
		if os.Getenv(envName) != "" {
			continue
		}

		val, err := a.vault.Get(key)
		if err != nil || val == "" {
			continue
		}

		if err := os.Setenv(envName, val); err != nil {
			a.logger.Warn("failed to set env from vault", "key", envName, "error", err)
			continue
		}
		injected++
	}

	if injected > 0 {
		a.logger.Info("vault secrets injected as env vars", "count", injected, "total_keys", len(keys))
	}
}

// AccessManager returns the access manager.
func (a *Assistant) AccessManager() *AccessManager {
	return a.accessMgr
}

// WorkspaceManager returns the workspace manager.
func (a *Assistant) WorkspaceManager() *WorkspaceManager {
	return a.workspaceMgr
}

// SkillRegistry returns the skills registry.
func (a *Assistant) SkillRegistry() *skills.Registry {
	return a.skillRegistry
}

// ProjectManager returns the project manager.
func (a *Assistant) ProjectManager() *ProjectManager {
	return a.projectMgr
}

// SetScheduler configures the assistant's scheduler.
func (a *Assistant) SetScheduler(s *scheduler.Scheduler) {
	a.scheduler = s
}

// handleDrainedMessages processes messages drained from the queue after debounce.
// Called by MessageQueue when the debounce timer fires.
func (a *Assistant) handleDrainedMessages(sessionID string, msgs []*channels.IncomingMessage) {
	if len(msgs) == 0 {
		return
	}
	combined := a.messageQueue.CombineMessages(msgs)
	// Use first message as base for metadata; replace content with combined.
	synthetic := *msgs[0]
	synthetic.Content = combined
	synthetic.ID = msgs[0].ID + "-combined"
	a.handleMessage(&synthetic)
}

// handleBusySession processes a new message when the session is already running
// an agent. Behavior depends on the configured queue mode for the channel.
func (a *Assistant) handleBusySession(msg *channels.IncomingMessage, sessionID string, logger *slog.Logger) {
	a.configMu.RLock()
	mode := EffectiveQueueMode(a.config.Queue, msg.Channel)
	a.configMu.RUnlock()

	logger.Info("session busy, applying queue mode",
		"session", sessionID,
		"mode", mode,
		"content_preview", truncate(msg.Content, 50),
	)

	switch mode {
	case QueueModeInterrupt:
		// Abort the current run and let this message be processed fresh.
		a.activeRunsMu.Lock()
		for key, cancel := range a.activeRuns {
			if strings.HasSuffix(key, ":"+sessionID) || strings.HasSuffix(key, sessionID) {
				cancel()
				delete(a.activeRuns, key)
				break
			}
		}
		a.activeRunsMu.Unlock()

		// Clear the followup queue (new message supersedes pending ones).
		a.followupQueuesMu.Lock()
		delete(a.followupQueues, sessionID)
		a.followupQueuesMu.Unlock()

		// Wait briefly for the cancelled run to release the processing lock.
		time.Sleep(200 * time.Millisecond)

		// Re-enqueue for immediate processing.
		go a.handleMessage(msg)
		return

	case QueueModeSteer, QueueModeSteerBacklog:
		// Inject into the active run's interrupt inbox so the agent sees it
		// between turns and can adapt its behavior. The agent should respond
		// to it within the current run — do NOT also enqueue as followup to
		// avoid double processing.
		a.interruptInboxesMu.Lock()
		inbox, hasInbox := a.interruptInboxes[sessionID]
		a.interruptInboxesMu.Unlock()

		if hasInbox {
			enriched := a.enrichMessageContent(a.ctx, msg, logger)
			select {
			case inbox <- enriched:
				logger.Debug("message injected into active run (steer)", "session", sessionID)
				a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "👀")
				return
			default:
				logger.Warn("interrupt inbox full, falling back to followup", "session", sessionID)
			}
		}

		// Fallback: inbox was full or didn't exist — enqueue as followup.
		a.enqueueFollowup(msg, sessionID, logger)
		a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "👀")
		return

	case QueueModeCollect:
		// Just enqueue; all queued messages will be combined into a single
		// prompt when the current run completes.
		a.enqueueFollowup(msg, sessionID, logger)
		a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "👀")
		return

	default: // QueueModeFollowup (default)
		// Enqueue as individual followup — will be processed as a separate
		// agent run after the current one completes. No injection into the
		// active run to avoid the same message being processed twice.
		a.enqueueFollowup(msg, sessionID, logger)
		a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "👀")
		return
	}
}

// enqueueFollowup adds a message to the followup queue with bounds checking.
func (a *Assistant) enqueueFollowup(msg *channels.IncomingMessage, sessionID string, logger *slog.Logger) {
	const maxFollowupQueue = 20
	a.followupQueuesMu.Lock()
	if len(a.followupQueues[sessionID]) >= maxFollowupQueue {
		a.followupQueues[sessionID] = a.followupQueues[sessionID][1:]
		logger.Warn("followup queue full, dropped oldest", "session", sessionID)
	}
	a.followupQueues[sessionID] = append(a.followupQueues[sessionID], msg)
	qLen := len(a.followupQueues[sessionID])
	a.followupQueuesMu.Unlock()

	logger.Info("message enqueued as followup",
		"session", sessionID,
		"queue_length", qLen,
		"content_preview", truncate(msg.Content, 50),
	)
}

// enqueueFollowupMessage creates a synthetic message and enqueues it for processing.
// This is used by subagent announce to inject results back into the parent session.
func (a *Assistant) enqueueFollowupMessage(sessionID, content, channel, chatID string) {
	msg := &channels.IncomingMessage{
		Content: content,
		Channel: channel,
		ChatID:  chatID,
		From:    "system",
		ID:      fmt.Sprintf("subagent-announce-%d", time.Now().UnixNano()),
		IsGroup: strings.Contains(chatID, "@g.us"),
	}

	a.followupQueuesMu.Lock()
	const maxFollowupQueue = 20
	if len(a.followupQueues[sessionID]) >= maxFollowupQueue {
		a.followupQueues[sessionID] = a.followupQueues[sessionID][1:]
		a.logger.Warn("followup queue full, dropped oldest", "session", sessionID)
	}
	a.followupQueues[sessionID] = append(a.followupQueues[sessionID], msg)
	qLen := len(a.followupQueues[sessionID])
	a.followupQueuesMu.Unlock()

	a.logger.Info("subagent result enqueued as followup",
		"session", sessionID,
		"queue_length", qLen,
	)

	// If the session is not currently processing, trigger immediate processing.
	if !a.messageQueue.IsProcessing(sessionID) {
		go a.drainFollowupQueue(sessionID)
	}
}

// drainFollowupQueue processes messages that were enqueued while a session was
// busy. Each followup message is processed as a new, independent agent run.
// When there are multiple queued messages, they are combined into a single run.
func (a *Assistant) drainFollowupQueue(sessionID string) {
	a.followupQueuesMu.Lock()
	msgs := a.followupQueues[sessionID]
	delete(a.followupQueues, sessionID)
	a.followupQueuesMu.Unlock()

	if len(msgs) == 0 {
		return
	}

	a.logger.Info("draining followup queue",
		"session", sessionID,
		"count", len(msgs),
	)

	// Collect mode: combine multiple queued messages into one prompt,
	// then process as a single agent run.
	if len(msgs) > 1 {
		combined := a.messageQueue.CombineMessages(msgs)
		synthetic := *msgs[0]
		synthetic.Content = combined
		synthetic.ID = msgs[0].ID + "-followup-collected"
		a.handleMessage(&synthetic)
		return
	}

	// Single followup: process directly.
	a.handleMessage(msgs[0])
}

// messageLoop is the main loop that processes messages from all channels.
func (a *Assistant) messageLoop() {
	for {
		select {
		case msg, ok := <-a.channelMgr.Messages():
			if !ok {
				return
			}
			go a.handleMessage(msg)

		case <-a.ctx.Done():
			return
		}
	}
}

// handleMessage processes an individual message following the full flow:
// access check → command → trigger → workspace → validate → build → execute → validate → send.
func (a *Assistant) handleMessage(msg *channels.IncomingMessage) {
	start := time.Now()
	logger := a.logger.With(
		"channel", msg.Channel,
		"chat_id", msg.ChatID,
		"from", msg.From,
		"msg_id", msg.ID,
	)

	logger.Info("incoming message",
		"content_preview", truncate(msg.Content, 50),
		"type", msg.Type,
		"is_group", msg.IsGroup,
	)

	// ── Step 0: Access control ──
	// Check if the sender is authorized BEFORE anything else.
	// Unknown contacts are silently ignored (deny-by-default policy).
	accessResult := a.accessMgr.Check(msg)

	if !accessResult.Allowed {
		// Check if this is a DM with a potential pairing token.
		if !msg.IsGroup && a.pairingMgr != nil {
			token := ExtractTokenFromMessage(msg.Content)
			if token != "" {
				approved, response, err := a.pairingMgr.ProcessTokenRedemption(
					token, msg.From, msg.FromName)
				if err != nil {
					logger.Warn("pairing token error", "error", err)
				}
				a.sendReply(msg, response)
				if approved {
					// Re-check access now that user is approved.
					accessResult = a.accessMgr.Check(msg)
					logger.Info("access granted via pairing token",
						"from", msg.From)
				}
				return
			}
		}

		// If policy is "ask", send a one-time message.
		if accessResult.ShouldAsk {
			a.sendReply(msg, a.accessMgr.PendingMessage())
			a.accessMgr.MarkAsked(msg.From)
			logger.Info("access pending, sent request message",
				"from", msg.From)
		} else {
			logger.Info("message ignored (access denied)",
				"reason", accessResult.Reason,
				"from_raw", msg.From)
		}
		return
	}

	logger.Info("access granted", "level", accessResult.Level)

	// ── Step 0b: Maintenance mode check ──
	// Allow commands through, block regular messages.
	if a.maintenanceMgr != nil && a.maintenanceMgr.IsEnabled() {
		if !IsCommand(msg.Content) {
			maint := a.maintenanceMgr.Get()
			response := "System is under maintenance."
			if maint != nil && maint.Message != "" {
				response = maint.Message
			}
			a.sendReply(msg, response)
			logger.Info("message blocked (maintenance mode)")
			return
		}
	}

	// ── Step 1: Admin commands ──
	// Check for /commands BEFORE trigger check (commands always work).
	if IsCommand(msg.Content) {
		result := a.HandleCommand(msg)
		if result.Handled {
			if result.Response != "" {
				a.sendReply(msg, result.Response)
			}
			logger.Info("admin command processed",
				"duration_ms", time.Since(start).Milliseconds())
			return
		}
	}

	// ── Step 1a: Natural language approval ──
	// If there are pending approvals for this session and the user sends
	// a short affirmative/negative message, treat it as an approval/denial.
	sessionID := MakeSessionID(msg.Channel, msg.ChatID)
	if a.approvalMgr.PendingCountForSession(sessionID) > 0 {
		action := matchNaturalApproval(msg.Content)
		if action != "" {
			latestID := a.approvalMgr.LatestPendingForSession(sessionID)
			if latestID != "" {
				approved := action == "approve"
				if a.approvalMgr.Resolve(latestID, sessionID, msg.From, approved, "") {
					if approved {
						a.sendReply(msg, "✅ Approved.")
					} else {
						a.sendReply(msg, "❌ Denied.")
					}
					logger.Info("natural language approval",
						"action", action,
						"duration_ms", time.Since(start).Milliseconds())
					return
				}
			}
		}
	}

	// ── Step 1b: Atomic processing lock + followup queue ──
	// TrySetProcessing atomically checks and sets, eliminating the race window
	// where two goroutines could both pass IsProcessing and start parallel runs.
	if !a.messageQueue.TrySetProcessing(sessionID) {
		a.handleBusySession(msg, sessionID, logger)
		return
	}
	defer func() {
		a.messageQueue.SetProcessing(sessionID, false)
		// Drain followup queue: process messages received during this run.
		// Each followup is handled as a new, independent agent run.
		a.drainFollowupQueue(sessionID)
	}()

	// ── Step 2: Resolve workspace ──
	// Determine which workspace this message belongs to.
	resolved := a.workspaceMgr.Resolve(
		msg.Channel, msg.ChatID, msg.From, msg.IsGroup)

	workspace := resolved.Workspace
	session := resolved.Session

	logger = logger.With("workspace", workspace.ID)

	// ── Step 3: Check trigger ──
	// Use workspace trigger if set, otherwise global.
	trigger := a.config.Trigger
	if workspace.Trigger != "" {
		trigger = workspace.Trigger
	}
	triggered := a.matchesTrigger(msg.Content, trigger, msg.IsGroup)

	// ── Step 3a: Group policy check ──
	// For group messages, check if we should respond based on group policy.
	if msg.IsGroup && a.groupPolicyMgr != nil {
		isReplyToBot := false // TODO: detect if message is a reply to bot
		matchedTrigger := ""
		if triggered {
			matchedTrigger = trigger
		}
		if !a.groupPolicyMgr.ShouldRespond(msg.ChatID, msg.From, msg.Content, isReplyToBot, matchedTrigger) {
			logger.Debug("group policy: not responding")
			return
		}
		// Override workspace if group has a specific workspace configured.
		if wsID := a.groupPolicyMgr.GetWorkspace(msg.ChatID); wsID != "" {
			if altWS, ok := a.workspaceMgr.Get(wsID); ok {
				workspace = altWS
				logger = logger.With("workspace_override", wsID)
			}
		}
	} else if !triggered {
		// Non-group messages still need trigger match.
		return
	}

	logger.Info("message received, processing...",
		"access_level", accessResult.Level)

	// ── Step 3b: React, send typing indicator, and mark as read ──
	// React with ⏳ to acknowledge processing.
	a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "⏳")
	a.channelMgr.SendTyping(a.ctx, msg.Channel, msg.ChatID)
	a.channelMgr.MarkRead(a.ctx, msg.Channel, msg.ChatID, []string{msg.ID})

	// ── Step 4: Enrich content with media (images → description, audio → transcript) ──
	// Phase 1 (fast): extract text immediately, schedule media for async processing.
	// Phase 2 (async): media results are injected via interruptCh when ready.
	userContent, hasMediaPending := a.enrichMessageContentFast(msg, logger)

	// ── Step 5: Validate input ──
	if err := a.inputGuard.Validate(msg.From, userContent); err != nil {
		logger.Warn("input rejected", "error", err)
		a.sendReply(msg, fmt.Sprintf("Sorry, I can't process that: %v", err))
		return
	}

	// ── Step 6: Caller context is now passed via context.Context (see Step 8).
	// The old global SetCallerContext/SetSessionContext is kept for backward
	// compatibility (CLI, scheduler) but the agent run uses per-request context.

	// ── Step 7: Build prompt with workspace context ──
	promptStart := time.Now()
	prompt := a.composeWorkspacePrompt(workspace, session, userContent)

	// ── Step 7b: Agent routing (model/instructions override) ──
	// If agent profiles are configured, route based on channel/user/group.
	var agentProfile *AgentProfileConfig
	var modelOverride string
	if a.agentRouter != nil {
		// For group messages, use ChatID as group JID.
		groupJID := ""
		if msg.IsGroup {
			groupJID = msg.ChatID
		}
		agentProfile = a.agentRouter.Route(msg.Channel, msg.From, groupJID)
		if agentProfile != nil {
			logger.Info("agent routed",
				"profile", agentProfile.ID,
				"channel", msg.Channel,
				"user", msg.From,
				"group", groupJID,
			)

			// Override model if specified in profile.
			if agentProfile.Model != "" {
				modelOverride = agentProfile.Model
			}

			// Override prompt if profile has custom instructions.
			if agentProfile.Instructions != "" {
				// Replace the base instructions with profile instructions.
				// Keep workspace context but use profile's system prompt.
				prompt = a.composePromptWithAgent(agentProfile, workspace, session, userContent)
			}
		}
	}

	// Apply model override from session config if not set by agent profile.
	if modelOverride == "" {
		modelOverride = session.GetConfig().Model
	}

	logger.Info("prompt composed",
		"duration_ms", time.Since(promptStart).Milliseconds(),
		"prompt_chars", len(prompt),
		"model_override", modelOverride,
		"agent_profile", func() string {
			if agentProfile != nil {
				return agentProfile.ID
			}
			return ""
		}(),
	)

	// ── Step 8: Execute agent (with optional block streaming) ──
	// Propagate caller, session, and delivery target through context so
	// tools get per-request security context without shared mutable state.
	agentCtx := ContextWithSession(a.ctx, sessionID)
	agentCtx = ContextWithDelivery(agentCtx, msg.Channel, msg.ChatID)
	agentCtx = ContextWithCaller(agentCtx, accessResult.Level, msg.From)

	// Resolve tool profile for this workspace (workspace can override global).
	if profile := a.resolveToolProfile(workspace); profile != nil {
		agentCtx = ContextWithToolProfile(agentCtx, profile)
	}

	// Inject ProgressSender with per-channel cooldown.
	// WhatsApp doesn't support editing messages, so we rate-limit progress
	// to avoid flooding the chat with dozens of "still working..." messages.
	var lastProgressMu sync.Mutex
	var lastProgressAt time.Time
	progressCooldown := 60 * time.Second // default: max 1 progress msg/min
	if msg.Channel == "webui" {
		progressCooldown = 10 * time.Second
	}
	agentCtx = ContextWithProgressSender(agentCtx, func(_ context.Context, progressMsg string) {
		lastProgressMu.Lock()
		if time.Since(lastProgressAt) < progressCooldown {
			lastProgressMu.Unlock()
			return
		}
		lastProgressAt = time.Now()
		lastProgressMu.Unlock()
		formatted := FormatForChannel(progressMsg, msg.Channel)
		if formatted == "" {
			return
		}
		outMsg := &channels.OutgoingMessage{Content: formatted}
		_ = a.channelMgr.Send(a.ctx, msg.Channel, msg.ChatID, outMsg)
	})

	bsCfg := a.config.BlockStream.Effective()
	var blockStreamer *BlockStreamer
	if bsCfg.Enabled {
		blockStreamer = NewBlockStreamer(bsCfg, a.channelMgr, msg.Channel, msg.ChatID, msg.ID)
	}

	// Start a typing heartbeat goroutine that re-sends typing indicators
	// every 8 seconds while the agent is processing. WhatsApp's typing
	// indicator expires after ~10s, so refreshing keeps it alive.
	typingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ticker.C:
				a.channelMgr.SendTyping(a.ctx, msg.Channel, msg.ChatID)
			}
		}
	}()

	// ── Step 8b: Schedule async media processing if pending ──
	// Media enrichment runs in parallel with the agent. When results arrive,
	// they are injected via the interrupt channel so the agent incorporates
	// them into its next turn without blocking the initial response.
	if hasMediaPending {
		go a.enrichMediaAsync(a.ctx, msg, sessionID, logger)
	}

	agentStart := time.Now()
	response := a.executeAgentWithStream(agentCtx, workspace.ID, session, sessionID, prompt, userContent, blockStreamer, modelOverride)
	logger.Info("agent execution complete",
		"agent_duration_ms", time.Since(agentStart).Milliseconds(),
		"response_len", len(response),
	)

	// Stop the typing heartbeat.
	close(typingDone)

	// Finalize the block streamer (flush remaining text).
	if blockStreamer != nil {
		blockStreamer.Finish()
	}

	// ── Step 9: Validate output ──
	if err := a.outputGuard.Validate(response); err != nil {
		logger.Warn("output rejected, applying fallback", "error", err)
		response = "Sorry, I encountered an issue generating the response. Could you rephrase?"
	}

	// ── Step 10: Update session ──
	session.AddMessage(userContent, response)

	// ── Step 10b: Auto-capture memories from this conversation turn ──
	// Asynchronously extract important facts, preferences, and decisions from
	// the user+assistant exchange so they're available for future recall.
	if a.memoryStore != nil {
		go a.autoCaptureFacts(userContent, response, sessionID)
	}

	// ── Step 10c: Check if session needs compaction (background) ──
	// Compaction may trigger an LLM call (summarize strategy), so run it in
	// the background to avoid blocking the user's response delivery.
	go a.maybeCompactSession(session)

	// ── Step 11: Send reply (skip if block streamer already sent everything) ──
	if blockStreamer == nil || !blockStreamer.HasSentBlocks() {
		a.sendReply(msg, response)
	}

	// ── Step 11b: TTS — synthesize and send audio if enabled ──
	a.maybeSendTTS(msg, response)

	// React with ✅ to signal processing is complete.
	a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "✅")

	logger.Info("message processed",
		"duration_ms", time.Since(start).Milliseconds(),
		"workspace", workspace.ID,
	)
}

// matchesTrigger checks if a message matches the activation keyword.
// In DMs, the trigger is optional (always responds).
// In groups, the trigger is required unless the group has its own trigger.
// matchNaturalApproval checks if a short message matches common approval/denial
// patterns in Portuguese and English. Returns "approve", "deny", or "".
func matchNaturalApproval(content string) string {
	text := strings.ToLower(strings.TrimSpace(content))

	// Only match short messages (< 40 chars) to avoid false positives.
	if len(text) > 40 {
		return ""
	}

	approvePatterns := []string{
		"aprovo", "aprovado", "aprova", "pode executar", "pode rodar",
		"executa", "execute", "roda", "rode", "vai", "manda",
		"sim", "yes", "ok", "pode", "go", "run", "approve",
		"confirmo", "confirmado", "autorizo", "autorizado",
		"libera", "liberado", "faz", "faça", "bora",
	}

	denyPatterns := []string{
		"não", "nao", "no", "deny", "denied", "nega", "negado",
		"cancela", "cancelado", "cancel", "para", "stop",
		"bloqueia", "bloqueado", "não pode", "nao pode",
	}

	for _, p := range denyPatterns {
		if text == p || strings.HasPrefix(text, p+" ") {
			return "deny"
		}
	}

	for _, p := range approvePatterns {
		if text == p || strings.HasPrefix(text, p+" ") {
			return "approve"
		}
	}

	return ""
}

func (a *Assistant) matchesTrigger(content, trigger string, isGroup bool) bool {
	// No trigger configured = always respond.
	if trigger == "" {
		return true
	}

	// In DMs, respond even without trigger.
	if !isGroup {
		return true
	}

	// In groups, require the trigger.
	content = strings.TrimSpace(content)
	return len(content) >= len(trigger) &&
		strings.EqualFold(content[:len(trigger)], trigger)
}

// resolveToolProfile returns the effective tool profile for a workspace.
// Workspace profile takes precedence over global profile.
// Returns nil if no profile is configured.
func (a *Assistant) resolveToolProfile(ws *Workspace) *ToolProfile {
	// Workspace profile takes precedence.
	if ws.ToolProfile != "" {
		if profile := GetProfile(ws.ToolProfile, a.config.Security.ToolGuard.CustomProfiles); profile != nil {
			return profile
		}
	}

	// Fall back to global profile.
	if a.config.Security.ToolGuard.Profile != "" {
		return GetProfile(a.config.Security.ToolGuard.Profile, a.config.Security.ToolGuard.CustomProfiles)
	}

	return nil
}

// composeWorkspacePrompt builds the prompt using workspace overrides.
func (a *Assistant) composeWorkspacePrompt(ws *Workspace, session *Session, input string) string {
	// If workspace has custom instructions, inject them as business context.
	if ws.Instructions != "" {
		cfg := session.GetConfig()
		if cfg.BusinessContext != ws.Instructions {
			cfg.BusinessContext = ws.Instructions
			session.SetConfig(cfg)
		}
	}

	return a.promptComposer.Compose(session, input)
}

// composePromptWithAgent builds a prompt using agent profile instructions.
// The agent profile's instructions replace the base instructions while
// preserving workspace context.
func (a *Assistant) composePromptWithAgent(profile *AgentProfileConfig, ws *Workspace, session *Session, input string) string {
	// Store original instructions to restore later.
	originalInstructions := a.config.Instructions

	// Temporarily set agent's instructions.
	a.config.Instructions = profile.Instructions

	// Also add workspace instructions as business context if available.
	if ws.Instructions != "" {
		cfg := session.GetConfig()
		cfg.BusinessContext = ws.Instructions
		session.SetConfig(cfg)
	}

	// Compose with agent instructions.
	prompt := a.promptComposer.Compose(session, input)

	// Restore original instructions.
	a.config.Instructions = originalInstructions

	return prompt
}

// executeAgentWithStream runs the agentic loop, optionally streaming text
// progressively to the channel via a BlockStreamer.
// sessionID is the channel:chatID key used for interrupt inbox routing.
// modelOverride specifies the model to use (empty = use default).
func (a *Assistant) executeAgentWithStream(ctx context.Context, workspaceID string, session *Session, sessionID string, systemPrompt string, userMessage string, streamer *BlockStreamer, modelOverride string) string {
	runKey := workspaceID + ":" + session.ID

	// Create interrupt inbox so follow-up messages can be injected mid-run.
	interruptInbox := make(chan string, 10)
	a.interruptInboxesMu.Lock()
	a.interruptInboxes[sessionID] = interruptInbox
	a.interruptInboxesMu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)

	// ── Persist active run for restart recovery ──
	channel, chatID, _ := strings.Cut(sessionID, ":")
	a.markRunActive(sessionID, channel, chatID, userMessage)

	defer func() {
		// Remove interrupt inbox before releasing the processing lock.
		a.interruptInboxesMu.Lock()
		delete(a.interruptInboxes, sessionID)
		a.interruptInboxesMu.Unlock()

		a.activeRunsMu.Lock()
		delete(a.activeRuns, runKey)
		a.activeRunsMu.Unlock()

		// Clear the active run marker — run completed normally.
		a.clearRunActive(sessionID)

		cancel()
	}()

	a.activeRunsMu.Lock()
	a.activeRuns[runKey] = cancel
	a.activeRunsMu.Unlock()

	// 10 recent entries ≈ 2-3K tokens: enough context without bloating the
	// prompt. Older history is summarized by session memory if enabled.
	history := session.RecentHistory(10)

	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	agent.SetModelOverride(modelOverride)

	// Wire interrupt channel for live message injection.
	agent.SetInterruptChannel(interruptInbox)

	// Wire block streaming if provided.
	if streamer != nil {
		agent.SetStreamCallback(streamer.StreamCallback())
		// Flush buffered text before tools start so the user sees intermediate
		// reasoning/thoughts immediately instead of waiting for the full response.
		agent.SetOnBeforeToolExec(streamer.FlushNow)
	}

	// Wire auto-send media hook for tools that produce files (e.g. generate_image).
	dt := DeliveryTargetFromContext(ctx)
	if dt.Channel != "" {
		agent.SetOnToolResult(a.makeToolResultHook(dt.Channel, dt.ChatID))
	}

	// Wire tool loop detector (new instance per-run to avoid cross-session races).
	if a.loopDetectorConfig.Enabled {
		detector := NewToolLoopDetector(a.loopDetectorConfig, a.logger.With("component", "loop-detect"))
		agent.SetLoopDetector(detector)
	}

	if a.usageTracker != nil {
		agent.SetUsageRecorder(func(model string, usage LLMUsage) {
			a.usageTracker.Record(session.ID, model, usage)
		})
	}

	response, usage, err := agent.RunWithUsage(runCtx, systemPrompt, history, userMessage)
	if err != nil {
		if runCtx.Err() != nil {
			return "Agent stopped."
		}
		a.logger.Error("agent failed", "error", err)
		return fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return response
}

// executeAgent runs the agentic loop with tool use support.
// Uses a cancelable context so /stop can abort the run.
func (a *Assistant) executeAgent(ctx context.Context, workspaceID string, session *Session, systemPrompt string, userMessage string) string {
	runKey := workspaceID + ":" + session.ID

	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		a.activeRunsMu.Lock()
		delete(a.activeRuns, runKey)
		a.activeRunsMu.Unlock()
		cancel()
	}()

	a.activeRunsMu.Lock()
	a.activeRuns[runKey] = cancel
	a.activeRunsMu.Unlock()

	history := session.RecentHistory(10)

	modelOverride := session.GetConfig().Model
	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	agent.SetModelOverride(modelOverride)

	// Wire tool loop detector (new instance per-run to avoid cross-session races).
	if a.loopDetectorConfig.Enabled {
		detector := NewToolLoopDetector(a.loopDetectorConfig, a.logger.With("component", "loop-detect"))
		agent.SetLoopDetector(detector)
	}

	if a.usageTracker != nil {
		agent.SetUsageRecorder(func(model string, usage LLMUsage) {
			a.usageTracker.Record(session.ID, model, usage)
		})
	}

	response, usage, err := agent.RunWithUsage(runCtx, systemPrompt, history, userMessage)
	if err != nil {
		if runCtx.Err() != nil {
			return "Agent stopped."
		}
		a.logger.Error("agent failed", "error", err)
		return fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return response
}

// ToolExecutor returns the tool executor for external tool registration.
func (a *Assistant) ToolExecutor() *ToolExecutor {
	return a.toolExecutor
}

// UsageTracker returns the usage tracker for token/cost stats.
func (a *Assistant) UsageTracker() *UsageTracker {
	return a.usageTracker
}

// HookManager returns the lifecycle hook manager for registering plugin hooks.
func (a *Assistant) HookManager() *HookManager {
	return a.hookMgr
}

// Config returns the assistant configuration.
func (a *Assistant) Config() *Config {
	return a.config
}

// LLMClient returns the LLM client (for gateway chat completions).
func (a *Assistant) LLMClient() *LLMClient {
	return a.llmClient
}

// ForceCompactSession runs compaction immediately, returns old and new history length.
func (a *Assistant) ForceCompactSession(session *Session) (oldLen, newLen int) {
	return a.forceCompactSession(session)
}

// SchedulerEnabled returns true if the scheduler is running.
func (a *Assistant) SchedulerEnabled() bool {
	return a.scheduler != nil
}

// MemoryEnabled returns true if the memory store is available.
func (a *Assistant) MemoryEnabled() bool {
	return a.memoryStore != nil
}

// SQLiteMemory returns the SQLite memory store (for advanced search), or nil.
func (a *Assistant) SQLiteMemory() *memory.SQLiteStore {
	return a.sqliteMemory
}

// SessionStore returns the session store (used by CLI chat).
func (a *Assistant) SessionStore() *SessionStore {
	return a.sessionStore
}

// Scheduler returns the task scheduler (may be nil if not initialized).
func (a *Assistant) Scheduler() *scheduler.Scheduler {
	return a.scheduler
}

// ComposePrompt builds a system prompt for the given session and input.
// Convenience method for CLI and external callers.
func (a *Assistant) ComposePrompt(session *Session, input string) string {
	return a.promptComposer.Compose(session, input)
}

// ExecuteAgent runs the agent loop with tools and returns the response text.
// Public wrapper for CLI and external callers. Uses "default" as workspace ID.
func (a *Assistant) ExecuteAgent(ctx context.Context, systemPrompt string, session *Session, userMessage string) string {
	return a.executeAgent(ctx, "default", session, systemPrompt, userMessage)
}

// StopActiveRun cancels the active agent run for the given workspace and session.
// It also signals the tool executor to abort all running tools and forces the
// session out of "processing" state so new messages are handled immediately.
// Returns true if a run was stopped, false if none was active.
func (a *Assistant) StopActiveRun(workspaceID, sessionID string) bool {
	runKey := workspaceID + ":" + sessionID
	a.activeRunsMu.Lock()
	cancel, ok := a.activeRuns[runKey]
	if ok {
		delete(a.activeRuns, runKey)
	}
	a.activeRunsMu.Unlock()

	if ok && cancel != nil {
		// Signal tool executor to abort all running tools immediately.
		a.toolExecutor.Abort()
		// Cancel the run context (kills LLM calls and tool contexts).
		cancel()
		// Force-clear the processing flag so the session is unblocked.
		a.messageQueue.SetProcessing(sessionID, false)
		// Reset abort channel for the next run.
		a.toolExecutor.ResetAbort()
		a.logger.Info("active run force-stopped", "workspace", workspaceID, "session", sessionID)
		return true
	}

	// Even if no active run was found, clear a potentially stuck processing flag.
	if a.messageQueue.IsProcessing(sessionID) {
		a.messageQueue.SetProcessing(sessionID, false)
		a.logger.Warn("cleared stuck processing flag (no active run found)", "session", sessionID)
		return true
	}

	return false
}

// sessionWatchdog periodically checks for sessions stuck in "processing" state
// and force-recovers them. This prevents sessions from being permanently blocked
// when a tool hangs beyond all timeout layers (e.g. orphaned child processes).
func (a *Assistant) sessionWatchdog() {
	const checkInterval = 60 * time.Second
	// Max time a session can be "processing" before the watchdog intervenes.
	// Set above the agent run timeout (default 20min) to avoid false positives.
	maxBusy := time.Duration(a.config.Agent.RunTimeoutSeconds)*time.Second + 5*time.Minute
	if maxBusy < 10*time.Minute {
		maxBusy = 25 * time.Minute
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			stuck := a.messageQueue.StuckSessions(maxBusy)
			for _, sessionID := range stuck {
				a.logger.Warn("watchdog: session stuck in processing, force-recovering",
					"session", sessionID, "max_busy", maxBusy)

				// Try to cancel the active run.
				a.activeRunsMu.Lock()
				for key, cancel := range a.activeRuns {
					if strings.HasSuffix(key, ":"+sessionID) || key == sessionID {
						cancel()
						delete(a.activeRuns, key)
						break
					}
				}
				a.activeRunsMu.Unlock()

				// Abort running tools and reset.
				a.toolExecutor.Abort()
				a.toolExecutor.ResetAbort()

				// Force-clear the processing flag.
				a.messageQueue.SetProcessing(sessionID, false)
			}
		}
	}
}

// initScheduler creates and configures the scheduler.
// Uses SQLite storage from devclawDB when available, falls back to JSON file.
func (a *Assistant) initScheduler() {
	var storage scheduler.JobStorage

	if a.devclawDB != nil {
		storage = scheduler.NewSQLiteJobStorage(a.devclawDB)
		a.logger.Info("scheduler storage: SQLite (devclaw.db)")
	} else {
		storagePath := a.config.Scheduler.Storage
		if storagePath == "" {
			storagePath = "./data/scheduler.json"
		}
		fileStorage, err := scheduler.NewFileJobStorage(storagePath)
		if err != nil {
			a.logger.Error("failed to create scheduler storage", "error", err)
			return
		}
		storage = fileStorage
		a.logger.Info("scheduler storage: JSON file", "path", storagePath)
	}

	// Job handler: runs the command as an agent turn.
	// Scheduled jobs run with full trust (no approval prompts) because they
	// were explicitly created by the user and execute autonomously.
	handler := func(ctx context.Context, job *scheduler.Job) (string, error) {
		a.logger.Info("scheduler executing job", "id", job.ID, "command", job.Command,
			"channel", job.Channel, "chat_id", job.ChatID)

		// Get or create a session for this scheduled job.
		session := a.sessionStore.GetOrCreate("scheduler", job.ID)

		// Grant session trust for all tools that normally require confirmation
		// so the scheduled agent can run without user interaction.
		schedulerSessionID := "scheduler:" + job.ID
		for _, toolName := range a.config.Security.ToolGuard.RequireConfirmation {
			a.approvalMgr.GrantTrust(schedulerSessionID, toolName)
		}
		// Propagate caller, session, and delivery target via context (goroutine-safe).
		// This replaces the old global SetCallerContext/SetSessionContext pattern.
		jobCtx := ContextWithCaller(ctx, AccessOwner, "scheduler")
		jobCtx = ContextWithSession(jobCtx, schedulerSessionID)
		if job.Channel != "" && job.ChatID != "" {
			jobCtx = ContextWithDelivery(jobCtx, job.Channel, job.ChatID)
		}

		// Build a delivery-focused prompt. Use ComposeMinimal() to skip
		// bootstrap files, memory search, and conversation history — this
		// cuts the prompt from ~7600 tokens to ~500, dramatically reducing
		// latency and preventing the LLM from wasting turns reading files.
		deliveryPrompt := fmt.Sprintf(
			"[SCHEDULED REMINDER — deliver this to the user]\n"+
				"You are delivering a previously scheduled reminder/task. "+
				"The user set this themselves. Deliver the message below concisely.\n"+
				"Do NOT use any tools. Do NOT ask follow-up questions.\n"+
				"Do NOT treat this as a conversation. Just deliver the reminder clearly.\n\n"+
				"Reminder: %s", job.Command)

		prompt := a.promptComposer.ComposeMinimal()

		// Use a minimal agent config: 1 turn, short timeout, no continuations.
		// The agent just needs to generate a single delivery response.
		jobAgentCfg := AgentConfig{
			MaxTurns:              1,
			RunTimeoutSeconds:     60,
			LLMCallTimeoutSeconds: 30,
			MaxContinuations:      0,
			ReflectionEnabled:     false,
		}

		agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, jobAgentCfg, a.logger)
		result, err := agent.Run(jobCtx, prompt, nil, deliveryPrompt)
		if err != nil {
			return "", err
		}

		// Save to session history.
		session.AddMessage(job.Command, result)

		// If job has a target channel/chat, send the result.
		if job.Channel != "" && job.ChatID != "" {
			// Strip internal tags before sending to user
			cleanResult := StripInternalTags(result)
			outMsg := &channels.OutgoingMessage{Content: cleanResult}
			if sendErr := a.channelMgr.Send(ctx, job.Channel, job.ChatID, outMsg); sendErr != nil {
				a.logger.Error("failed to deliver scheduled message",
					"job_id", job.ID, "error", sendErr,
					"channel", job.Channel, "chat_id", job.ChatID)
			}
		}

		return result, nil
	}

	a.scheduler = scheduler.New(storage, handler, a.logger)
	a.logger.Info("scheduler initialized")
}

// registerSkillLoaders registers the builtin and clawdhub skill loaders
// in the skill registry based on configuration.
func (a *Assistant) registerSkillLoaders() {
	// Builtin skills loader.
	if len(a.config.Skills.Builtin) > 0 {
		builtinLoader := skills.NewBuiltinLoader(a.config.Skills.Builtin, a.logger)

		// Inject project provider for coding skills (claude-code, project-manager).
		if a.projectMgr != nil {
			builtinLoader.SetProjectProvider(NewProjectProviderAdapter(a.projectMgr))
		}

		// Inject API credentials so skills like claude-code can authenticate
		// through the same LLM provider (e.g. Z.AI) as DevClaw itself.
		builtinLoader.SetAPIConfig(a.config.API.APIKey, a.config.API.BaseURL, a.config.Model)

		a.skillRegistry.AddLoader(builtinLoader)
	}

	// ClawdHub skills loader (loads from configured skill directories).
	// Always include ./skills/ as the default user skills directory, even if
	// not explicitly listed in config. This ensures user-installed skills are
	// always discovered.
	dirs := a.config.Skills.ClawdHubDirs
	defaultDir := "./skills"
	hasDefault := false
	for _, d := range dirs {
		if d == defaultDir || d == "skills" || d == "skills/" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		dirs = append(dirs, defaultDir)
	}
	clawdHubLoader := skills.NewClawdHubLoader(dirs, a.logger)
	a.skillRegistry.AddLoader(clawdHubLoader)
}

// initializeSkills initializes all loaded skills, passing the sandbox runner
// and other configuration via the config map.
func (a *Assistant) initializeSkills() {
	// Create sandbox runner if configured.
	var sandboxRunner *sandbox.Runner
	runner, err := sandbox.NewRunner(a.config.Sandbox, a.logger)
	if err != nil {
		a.logger.Warn("sandbox runner not available", "error", err)
	} else {
		sandboxRunner = runner
	}

	initConfig := map[string]any{}
	if sandboxRunner != nil {
		initConfig["_sandbox_runner"] = sandboxRunner
	}

	allSkills := a.skillRegistry.List()
	for _, meta := range allSkills {
		skill, ok := a.skillRegistry.Get(meta.Name)
		if !ok {
			continue
		}
		if err := skill.Init(a.ctx, initConfig); err != nil {
			a.logger.Warn("skill init failed", "name", meta.Name, "error", err)
		}
	}
}

// registerSkillTools iterates all loaded skills and registers their tools
// in the tool executor so the agent loop can use them.
func (a *Assistant) registerSkillTools() {
	allSkills := a.skillRegistry.List()
	registered := 0

	for _, meta := range allSkills {
		skill, ok := a.skillRegistry.Get(meta.Name)
		if !ok {
			continue
		}

		tools := skill.Tools()
		if len(tools) == 0 {
			continue
		}

		a.toolExecutor.RegisterSkillTools(skill)
		registered += len(tools)
	}

	a.logger.Info("skill tools registered",
		"skills", len(allSkills),
		"tools", registered,
	)
}

// registerSystemTools registers core system tools (web_fetch, exec, file I/O)
// that are always available to the agent regardless of skills configuration.
func (a *Assistant) registerSystemTools() {
	// Create sandbox runner for the exec tool.
	var sandboxRunner *sandbox.Runner
	runner, err := sandbox.NewRunner(a.config.Sandbox, a.logger)
	if err != nil {
		a.logger.Warn("sandbox runner not available for system tools", "error", err)
	} else {
		sandboxRunner = runner
	}

	dataDir := a.config.Memory.Path
	if dataDir == "" {
		dataDir = "./data"
	}
	// Use the parent dir of the memory path as the data directory.
	dataDir = filepath.Dir(dataDir)

	// Initialize skill database for skills to store structured data.
	// Must be done before RegisterSystemTools so cron tools can track reminders.
	skillDB, err := OpenSkillDatabase(dataDir)
	if err != nil {
		a.logger.Warn("skill database not available", "error", err)
	} else {
		a.skillDB = skillDB
		// Initialize reminders tracking table
		if err := a.skillDB.InitRemindersTable(); err != nil {
			a.logger.Warn("failed to initialize reminders table", "error", err)
		}
	}

	ssrfGuard := security.NewSSRFGuard(a.config.Security.SSRF, a.logger)
	RegisterSystemTools(a.toolExecutor, sandboxRunner, a.memoryStore, a.sqliteMemory, a.config.Memory, a.scheduler, dataDir, ssrfGuard, a.vault, a.config.WebSearch, a.skillDB)

	// Register skill database tools if available.
	if a.skillDB != nil {
		RegisterSkillDBTools(a.toolExecutor, a.skillDB)
	}

	// Register skill creator tools (including install_skill, search_skills, remove_skill).
	skillsDir := "./skills"
	if len(a.config.Skills.ClawdHubDirs) > 0 {
		skillsDir = a.config.Skills.ClawdHubDirs[0]
	}
	RegisterSkillCreatorTools(a.toolExecutor, a.skillRegistry, skillsDir, a.skillDB, a.logger)

	// Register subagent tools (spawn, list, wait, stop).
	RegisterSubagentTools(a.toolExecutor, a.subagentMgr, a.llmClient, a.promptComposer, a.logger)

	// Register session management tools (sessions_list, sessions_send) for multi-agent routing.
	RegisterSessionTools(a.toolExecutor, a.workspaceMgr)

	// Register team tools for persistent agents and team memory.
	if a.teamMgr != nil {
		RegisterTeamTools(a.toolExecutor, a.teamMgr, a.devclawDB, a.scheduler, a.logger)
	}

	// Register media tools (describe_image, transcribe_audio).
	RegisterMediaTools(a.toolExecutor, a.llmClient, a.config, a.logger)

	// Register native media tools (send_image, send_audio, send_document).
	if a.mediaSvc != nil {
		RegisterNativeMediaTools(a.toolExecutor, a.mediaSvc, a.channelMgr, a.logger)
	}

	// Register native developer tools (git, docker, db, env, utils, codebase, testing, ops, product, IDE).
	RegisterGitTools(a.toolExecutor)
	RegisterDockerTools(a.toolExecutor)
	RegisterDBTools(a.toolExecutor)
	RegisterDBHubTools(a.toolExecutor, a.dbHub) // Database hub management tools
	RegisterEnvTools(a.toolExecutor)
	RegisterDevUtilTools(a.toolExecutor)
	RegisterCodebaseTools(a.toolExecutor)
	RegisterTestingTools(a.toolExecutor)
	RegisterOpsTools(a.toolExecutor)
	RegisterProductTools(a.toolExecutor)
	RegisterIDETools(a.toolExecutor)

	// Register daemon manager for background process control.
	if a.daemonMgr == nil {
		a.daemonMgr = NewDaemonManager()
	}
	RegisterDaemonTools(a.toolExecutor, a.daemonMgr)

	// Register plugin system.
	if a.pluginMgr == nil {
		a.pluginMgr = NewPluginManager()
	}
	RegisterPluginTools(a.toolExecutor, a.pluginMgr)

	// Register multi-user tools (when enabled).
	if a.config.Team.Enabled {
		if a.userMgr == nil {
			a.userMgr = NewUserManager(a.config.Team)
		}
		RegisterMultiUserTools(a.toolExecutor, a.userMgr)
	}

	a.logger.Info("system tools registered",
		"tools", a.toolExecutor.ToolNames(),
	)
}

// maybeCompactSession checks if the session history is too large and compacts it.
func (a *Assistant) maybeCompactSession(session *Session) {
	threshold := a.config.Memory.MaxMessages
	if threshold <= 0 {
		threshold = 100
	}

	histLen := session.HistoryLen()

	// Preventive compaction: start at 80% of threshold to avoid hitting
	// the hard limit during active conversation.
	preventiveThreshold := threshold * 80 / 100
	if preventiveThreshold < 10 {
		preventiveThreshold = 10
	}

	if histLen < preventiveThreshold {
		return
	}

	a.logger.Info("preventive compaction triggered",
		"session", session.ID,
		"history_len", histLen,
		"threshold", threshold,
		"preventive_at", preventiveThreshold,
	)

	a.doCompactSession(session)
}

// forceCompactSession runs compaction immediately (used by /compact command).
// Skips threshold check; returns old and new history length.
func (a *Assistant) forceCompactSession(session *Session) (oldLen, newLen int) {
	oldLen = session.HistoryLen()
	if oldLen < 5 {
		return oldLen, oldLen
	}
	a.doCompactSession(session)
	return oldLen, session.HistoryLen()
}

// doCompactSession performs compaction using the configured CompressionStrategy.
//
// Strategies:
//   - "summarize" (default): LLM summarizes old history → single summary entry + recent.
//   - "truncate": simply drops the oldest entries, keeping the most recent.
//   - "sliding": keeps a fixed window of the N most recent entries (no summary).
func (a *Assistant) doCompactSession(session *Session) {
	strategy := a.config.Memory.CompressionStrategy
	if strategy == "" {
		strategy = "summarize"
	}

	a.logger.Info("session compaction",
		"session", session.ID,
		"strategy", strategy,
		"history_len", session.HistoryLen(),
	)

	threshold := a.config.Memory.MaxMessages
	if threshold <= 0 {
		threshold = 100
	}

	switch strategy {
	case "truncate":
		a.compactTruncate(session, threshold)
	case "sliding":
		a.compactSliding(session, threshold)
	default: // "summarize"
		a.compactSummarize(session, threshold)
	}
}

// compactSummarize uses the LLM to generate a summary of older conversation
// and replaces old entries with the summary, keeping recent entries.
// autoCaptureFacts performs lightweight fact extraction from a conversation turn.
// Runs asynchronously — should not block message delivery.
// Scans for memory triggers (preferences, decisions, entities, facts) and
// saves them via memory tool.
func (a *Assistant) autoCaptureFacts(userMessage, assistantResponse, sessionID string) {
	// Only capture from substantive exchanges (skip greetings, short replies).
	if len(userMessage) < 30 && len(assistantResponse) < 100 {
		return
	}

	// Check for memory triggers.
	combined := strings.ToLower(userMessage + " " + assistantResponse)
	triggers := []string{
		"remember", "lembre", "lembra", "prefer", "prefiro", "prefere",
		"always", "sempre", "never", "nunca", "my name", "meu nome",
		"i live", "eu moro", "i work", "eu trabalho", "important",
		"importante", "note that", "anota", "salva", "save",
		"decision", "decisão", "decided", "decidimos", "escolhi",
	}

	hasTrigger := false
	for _, t := range triggers {
		if strings.Contains(combined, t) {
			hasTrigger = true
			break
		}
	}

	if !hasTrigger {
		return
	}

	// Use a lightweight LLM call to extract facts worth remembering.
	extractPrompt := fmt.Sprintf(
		"Analyze this conversation exchange and extract ONLY genuinely important facts "+
			"worth remembering long-term (preferences, personal info, decisions, key facts). "+
			"If nothing important, reply with exactly: NOTHING\n\n"+
			"User: %s\n\nAssistant: %s\n\n"+
			"Reply with a JSON array of strings, each being one fact to save. Example: "+
			`["User prefers dark mode", "User's name is João"]`+
			"\nOr reply: NOTHING",
		truncateForCapture(userMessage, 500),
		truncateForCapture(assistantResponse, 500),
	)

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	result, err := a.llmClient.Complete(ctx, "", nil, extractPrompt)
	if err != nil || strings.TrimSpace(result) == "NOTHING" || strings.TrimSpace(result) == "" {
		return
	}

	// Parse the JSON array of facts.
	var facts []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &facts); err != nil {
		// Try single-line: maybe model returned plain text.
		lines := strings.Split(result, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && line != "NOTHING" && len(line) > 10 {
				facts = append(facts, line)
			}
		}
	}

	if len(facts) == 0 {
		return
	}

	// Save each fact to memory.
	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" || len(fact) < 5 {
			continue
		}
		_ = a.memoryStore.Save(memory.Entry{
			Content:   fact,
			Source:    "auto-capture",
			Category:  "fact",
			Timestamp: time.Now(),
		})
		a.logger.Debug("auto-captured memory fact",
			"fact_preview", truncateForCapture(fact, 60),
			"session", sessionID,
		)
	}

	a.logger.Info("memory auto-capture completed",
		"facts_saved", len(facts),
		"session", sessionID,
	)
}

// truncateForCapture limits text length for memory extraction prompts.
func truncateForCapture(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (a *Assistant) compactSummarize(session *Session, threshold int) {
	// Step 1: Memory flush — extract important facts before discarding.
	// The agent saves durable memories to disk BEFORE the session history is compacted.
	// IMPORTANT: Use append-only to avoid overwriting existing entries.
	if a.memoryStore != nil {
		flushPrompt := "Pre-compaction memory flush turn. The session is near auto-compaction; " +
			"capture durable memories to disk.\n" +
			"IMPORTANT: If the file already exists, APPEND new content only and do not overwrite existing entries.\n\n" +
			"Extract the most important facts, preferences, decisions, and information from this conversation " +
			"that should be remembered long-term. Save them using the memory(action=\"save\", ...) tool. " +
			"If nothing important, reply with NO_REPLY."

		agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
		systemPrompt := a.promptComposer.Compose(session, flushPrompt)

		flushCtx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
		_, err := agent.Run(flushCtx, systemPrompt, session.RecentHistory(20), flushPrompt)
		cancel()

		if err != nil {
			a.logger.Warn("memory flush failed", "error", err)
		} else {
			a.logger.Info("memory flush completed before compaction")
		}
	}

	// Step 2: LLM summarizes the conversation with retry and exponential backoff.
	// Transient errors (rate-limits, timeouts) are retried up to 3 times with
	// backoff: 2s → 4s → 8s. On permanent failure, a static fallback is used.
	summaryPrompt := "Summarize the key points of this conversation in 2-3 sentences. Focus on decisions made, tasks completed, and important context."
	var summary string
	var summaryErr error

	backoff := 2 * time.Second
	const maxSummaryRetries = 3

	for attempt := 1; attempt <= maxSummaryRetries; attempt++ {
		summary, summaryErr = a.llmClient.Complete(a.ctx, "", session.RecentHistory(20), summaryPrompt)
		if summaryErr == nil {
			break
		}

		a.logger.Warn("compaction summary attempt failed",
			"attempt", attempt,
			"max_retries", maxSummaryRetries,
			"error", summaryErr,
			"next_backoff", backoff.String(),
		)

		// Stop retrying if the context is already cancelled.
		if a.ctx.Err() != nil {
			break
		}

		if attempt < maxSummaryRetries {
			select {
			case <-time.After(backoff):
				backoff *= 2
			case <-a.ctx.Done():
				// Context cancelled during backoff wait — stop retrying.
			}
		}
	}
	if summaryErr != nil {
		a.logger.Error("compaction summary failed after all retries, using fallback",
			"retries", maxSummaryRetries, "error", summaryErr)
		summary = "Previous conversation context was compacted."
	}

	// Step 3: Keep 25% of threshold as recent history.
	keepRecent := threshold / 4
	if keepRecent < 5 {
		keepRecent = 5
	}

	oldEntries := session.CompactHistory(summary, keepRecent)

	// Step 4: Save the old entries to daily log.
	if a.memoryStore != nil && len(oldEntries) > 0 {
		var logContent strings.Builder
		logContent.WriteString(fmt.Sprintf("### Compacted session: %s\n\n", session.ID))
		logContent.WriteString(fmt.Sprintf("Summary: %s\n\n", summary))
		logContent.WriteString(fmt.Sprintf("Entries compacted: %d\n", len(oldEntries)))

		_ = a.memoryStore.SaveDailyLog(time.Now(), logContent.String())
	}

	a.logger.Info("session compacted (summarize)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// compactTruncate simply drops the oldest entries, keeping the N most recent.
// No LLM call needed — fast and cost-free.
func (a *Assistant) compactTruncate(session *Session, threshold int) {
	keepRecent := threshold / 2
	if keepRecent < 10 {
		keepRecent = 10
	}

	oldEntries := session.CompactHistory("", keepRecent)

	a.logger.Info("session compacted (truncate)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// compactSliding keeps a fixed sliding window of the most recent entries.
// Drops everything outside the window — no summary, no LLM call.
func (a *Assistant) compactSliding(session *Session, threshold int) {
	windowSize := threshold / 2
	if windowSize < 10 {
		windowSize = 10
	}

	oldEntries := session.CompactHistory("", windowSize)

	a.logger.Info("session compacted (sliding)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// enrichMessageContentFast returns the text content immediately, indicating whether
// async media processing is needed. This avoids blocking the agent start on media
// downloads, Vision API calls, or Whisper transcription.
// Returns (userContent, hasMediaPending).
func (a *Assistant) enrichMessageContentFast(msg *channels.IncomingMessage, logger *slog.Logger) (string, bool) {
	if msg.Media == nil {
		return msg.Content, false
	}

	// Check if the channel supports media and if we have relevant config.
	media := a.MediaConfig()
	_, ok := a.channelMgr.Channel(msg.Channel)
	if !ok {
		return msg.Content, false
	}

	switch msg.Media.Type {
	case channels.MessageImage:
		if !media.VisionEnabled {
			return msg.Content, false
		}
		// Run vision inline so the agent sees the description before responding.
		enriched := a.enrichMessageContent(a.ctx, msg, logger)
		if enriched != msg.Content {
			return enriched, false
		}
		return msg.Content, false

	case channels.MessageAudio:
		if !media.TranscriptionEnabled {
			return msg.Content, false
		}
		// Audio transcription is fast enough to do inline (< 5s for typical
		// voice notes). Running it synchronously avoids the race where the
		// agent responds to a placeholder before the transcript arrives.
		enriched := a.enrichMessageContent(a.ctx, msg, logger)
		if enriched != msg.Content {
			return enriched, false
		}
		return msg.Content, false

	case channels.MessageDocument:
		enriched := a.enrichMessageContent(a.ctx, msg, logger)
		if enriched != msg.Content {
			return enriched, false
		}
		return msg.Content, false

	case channels.MessageVideo:
		if !media.VisionEnabled {
			return msg.Content, false
		}
		enriched := a.enrichMessageContent(a.ctx, msg, logger)
		if enriched != msg.Content {
			return enriched, false
		}
		return msg.Content, false
	}

	return msg.Content, false
}

// enrichMediaAsync runs media enrichment in a background goroutine and injects
// the result into the agent's interrupt channel. This allows the agent to start
// processing the user's text immediately while media is being downloaded and
// analyzed in parallel.
func (a *Assistant) enrichMediaAsync(ctx context.Context, msg *channels.IncomingMessage, sessionID string, logger *slog.Logger) {
	enriched := a.enrichMessageContent(ctx, msg, logger)
	if enriched == msg.Content {
		return // Nothing enriched.
	}

	// Build the enrichment result message.
	var result string
	switch msg.Media.Type {
	case channels.MessageImage:
		result = fmt.Sprintf("[Media enrichment complete]\n%s", enriched)
	case channels.MessageAudio:
		result = fmt.Sprintf("[Audio transcription complete]\n%s", enriched)
	case channels.MessageDocument:
		result = fmt.Sprintf("[Document content extracted]\n%s", enriched)
	case channels.MessageVideo:
		result = fmt.Sprintf("[Video analysis complete]\n%s", enriched)
	default:
		result = enriched
	}

	// Inject into the interrupt inbox so the active agent run picks it up.
	a.interruptInboxesMu.Lock()
	inbox, hasInbox := a.interruptInboxes[sessionID]
	a.interruptInboxesMu.Unlock()

	if hasInbox {
		select {
		case inbox <- result:
			logger.Info("media enrichment injected into agent", "type", msg.Media.Type)
		default:
			logger.Warn("interrupt inbox full, media enrichment dropped")
		}
	}
}

// enrichMessageContent downloads media when present, describes images via vision API,
// transcribes audio via Whisper, and returns the enriched content for the agent.
// If no media or enrichment fails, returns the original msg.Content.
func (a *Assistant) enrichMessageContent(ctx context.Context, msg *channels.IncomingMessage, logger *slog.Logger) string {
	if msg.Media == nil {
		return msg.Content
	}

	media := a.MediaConfig()
	ch, ok := a.channelMgr.Channel(msg.Channel)
	if !ok {
		return msg.Content
	}
	mc, ok := ch.(channels.MediaChannel)
	if !ok {
		return msg.Content
	}

	data, mimeType, err := mc.DownloadMedia(ctx, msg)
	if err != nil {
		logger.Warn("failed to download media", "error", err)
		return msg.Content
	}

	switch msg.Media.Type {
	case channels.MessageImage:
		if !media.VisionEnabled {
			return msg.Content
		}
		if int64(len(data)) > media.MaxImageSize {
			logger.Warn("image too large to process", "size", len(data), "max", media.MaxImageSize)
			return msg.Content
		}
		imgBase64 := base64.StdEncoding.EncodeToString(data)
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
		desc, err := a.llmClient.CompleteWithVision(ctx, "", imgBase64, mimeType, "Describe this image in detail. Include any text visible.", media.VisionDetail, media.VisionModel)
		if err != nil {
			logger.Warn("vision description failed", "error", err)
			return msg.Content
		}
		logger.Info("image described via vision API", "desc_len", len(desc))
		if msg.Content != "" {
			return fmt.Sprintf("[Image: %s]\n\n%s", desc, msg.Content)
		}
		return fmt.Sprintf("[Image: %s]", desc)

	case channels.MessageAudio:
		if !media.TranscriptionEnabled {
			return msg.Content
		}
		if int64(len(data)) > media.MaxAudioSize {
			logger.Warn("audio too large to process", "size", len(data), "max", media.MaxAudioSize)
			return msg.Content
		}
		filename := msg.Media.Filename
		if filename == "" {
			filename = "audio.ogg"
		}
		transcript, err := a.llmClient.TranscribeAudio(ctx, data, filename, media.TranscriptionModel, media)
		if err != nil {
			logger.Warn("audio transcription failed", "error", err)
			return msg.Content
		}
		logger.Info("audio transcribed via Whisper", "transcript_len", len(transcript))
		content := msg.Content
		content = strings.ReplaceAll(content, "[audio]", transcript)
		content = strings.ReplaceAll(content, "[voice note]", transcript)
		return content

	case channels.MessageDocument:
		text := extractDocumentText(data, msg.Media.MimeType, msg.Media.Filename, logger)
		if text == "" {
			logger.Warn("no text extracted from document", "filename", msg.Media.Filename)
			return msg.Content
		}
		// Truncate very large documents to avoid context overflow.
		const maxDocChars = 30000
		if len(text) > maxDocChars {
			text = text[:maxDocChars] + "\n... [truncated — document too large]"
		}
		logger.Info("document text extracted", "chars", len(text), "filename", msg.Media.Filename)
		if msg.Content != "" {
			return fmt.Sprintf("[Document: %s]\n%s\n\n%s", msg.Media.Filename, text, msg.Content)
		}
		return fmt.Sprintf("[Document: %s]\n%s", msg.Media.Filename, text)

	case channels.MessageVideo:
		if !media.VisionEnabled {
			return msg.Content
		}
		desc := extractVideoFrame(ctx, data, mimeType, a.llmClient, media, logger)
		if desc == "" {
			return msg.Content
		}
		logger.Info("video frame described via vision API", "desc_len", len(desc))
		if msg.Content != "" {
			return fmt.Sprintf("[Video: %s]\n\n%s", desc, msg.Content)
		}
		return fmt.Sprintf("[Video: %s]", desc)
	}

	return msg.Content
}

// truncate returns the first n characters of s, adding "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// summarizeAndSaveSessionFromHistory uses the LLM to summarize a pre-captured
// history snapshot and saves it to memory/YYYY-MM-DD-slug.md. The history must
// be captured before session.ClearHistory()
// to avoid race conditions.
func (a *Assistant) summarizeAndSaveSessionFromHistory(history []ConversationEntry) {
	if len(history) < 2 {
		return // Too short to summarize.
	}

	// Build a conversation transcript for the LLM.
	var transcript strings.Builder
	for _, entry := range history {
		transcript.WriteString(fmt.Sprintf("User: %s\nAssistant: %s\n\n",
			truncate(entry.UserMessage, 500),
			truncate(entry.AssistantResponse, 1000),
		))
	}

	prompt := `Summarize this conversation in 2-5 bullet points. Focus on key decisions, facts learned, and tasks completed. Be concise. Output only the bullet points, no preamble.

Conversation:
` + transcript.String()

	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()

	agent := NewAgentRun(a.llmClient, a.toolExecutor, a.logger)
	summary, err := agent.Run(ctx, "You are a conversation summarizer. Output only concise bullet points.", nil, prompt)
	if err != nil {
		a.logger.Warn("session summary generation failed", "error", err)
		return
	}

	// Generate a slug from the first few words of the summary.
	slug := generateSlug(summary, 5)
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slug)

	// Write to memory directory.
	memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
	_ = os.MkdirAll(memDir, 0o755)

	content := fmt.Sprintf("# Session Summary — %s\n\n%s\n",
		now.Format("2006-01-02 15:04"), summary)

	filePath := filepath.Join(memDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		a.logger.Warn("failed to save session summary", "path", filePath, "error", err)
		return
	}

	a.logger.Info("session summary saved", "path", filePath)

	// Re-index if SQLite memory is available.
	if a.sqliteMemory != nil && a.config.Memory.Index.Auto {
		chunkCfg := memory.ChunkConfig{MaxTokens: a.config.Memory.Index.ChunkMaxTokens, Overlap: 100}
		if chunkCfg.MaxTokens <= 0 {
			chunkCfg.MaxTokens = 500
		}
		_ = a.sqliteMemory.IndexMemoryDir(a.ctx, memDir, chunkCfg)
	}
}

// generateSlug creates a URL-safe slug from the first n words of text.
func generateSlug(text string, maxWords int) string {
	words := strings.Fields(text)
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	slug := strings.Join(words, "-")
	slug = strings.ToLower(slug)

	// Keep only alphanumeric and hyphens.
	var clean strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}

	result := clean.String()
	if len(result) > 40 {
		result = result[:40]
	}
	if result == "" {
		result = "session"
	}
	return strings.TrimRight(result, "-")
}

// sendReply sends a response to the original message's channel.
// Long messages are split into chunks respecting the channel limit (default 4000 chars).
// buildTTSProvider creates the appropriate TTS provider based on config.
func (a *Assistant) buildTTSProvider() tts.Provider {
	switch a.config.TTS.Provider {
	case "openai":
		apiKey := a.config.API.APIKey
		baseURL := a.config.API.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return tts.NewOpenAIProvider(apiKey, baseURL, a.config.TTS.Model)

	case "edge":
		return tts.NewEdgeProvider(a.logger)

	case "auto":
		// Try OpenAI first, fall back to Edge TTS.
		apiKey := a.config.API.APIKey
		baseURL := a.config.API.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		primary := tts.NewOpenAIProvider(apiKey, baseURL, a.config.TTS.Model)
		secondary := tts.NewEdgeProvider(a.logger)
		edgeVoice := a.config.TTS.EdgeVoice
		if edgeVoice == "" {
			edgeVoice = "pt-BR-FranciscaNeural"
		}
		return tts.NewFallbackProvider(primary, secondary, a.config.TTS.Voice, edgeVoice, a.logger)

	default:
		a.logger.Warn("unknown TTS provider, using edge as fallback", "provider", a.config.TTS.Provider)
		return tts.NewEdgeProvider(a.logger)
	}
}

// maybeSendTTS synthesizes audio from the response text and sends it as a
// voice message, depending on the TTS auto-mode configuration.
// Skips synthesis for silent tokens (NO_REPLY, HEARTBEAT_OK) and empty
// responses to prevent sending audio of internal control tokens.
func (a *Assistant) maybeSendTTS(msg *channels.IncomingMessage, response string) {
	if a.ttsProvider == nil || response == "" {
		return
	}

	// Skip TTS for silent tokens — the agent used a tool to deliver its
	// response and the text output is just a control token, not user content.
	trimmed := strings.TrimSpace(response)
	if strings.EqualFold(trimmed, TokenNoReply) || strings.EqualFold(trimmed, TokenHeartbeatOK) {
		return
	}

	// Read TTS config under lock to avoid data race with /tts command.
	a.configMu.RLock()
	mode := a.config.TTS.AutoMode
	voice := a.config.TTS.Voice
	a.configMu.RUnlock()

	switch mode {
	case "always":
		// Always send audio.
	case "inbound":
		// Only send audio if the user sent a voice note.
		if msg.Type != channels.MessageAudio {
			return
		}
	default:
		// "off" or unknown: skip.
		return
	}

	// Truncate for TTS (avoid synthesizing huge responses).
	text := response
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	audio, mimeType, err := a.ttsProvider.Synthesize(ctx, text, voice)
	if err != nil {
		a.logger.Warn("TTS synthesis failed", "error", err)
		return
	}

	media := &channels.MediaMessage{
		Type:     channels.MessageAudio,
		Data:     audio,
		MimeType: mimeType,
		Filename: "response.ogg",
		ReplyTo:  msg.ID,
	}
	if err := a.channelMgr.SendMedia(a.ctx, msg.Channel, msg.ChatID, media); err != nil {
		a.logger.Warn("failed to send TTS audio", "error", err)
	}
}

// makeToolResultHook returns a callback that auto-sends media files produced by
// tools (e.g. generate_image) to the channel. This avoids the LLM having to
// describe "image saved to /tmp/..." — the user sees the actual image.
func (a *Assistant) makeToolResultHook(channel, chatID string) func(string, ToolResult) {
	return func(toolName string, result ToolResult) {
		if toolName != "generate_image" && toolName != "image-gen_generate_image" {
			return
		}
		// Parse the JSON result to find image_path.
		var parsed map[string]any
		if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
			// Try extracting from the stringified map format.
			return
		}
		imgPath, _ := parsed["image_path"].(string)
		if imgPath == "" {
			return
		}
		data, err := os.ReadFile(imgPath)
		if err != nil {
			a.logger.Warn("failed to read generated image", "path", imgPath, "error", err)
			return
		}
		caption, _ := parsed["revised_prompt"].(string)
		media := &channels.MediaMessage{
			Type:     channels.MessageImage,
			Data:     data,
			MimeType: "image/png",
			Filename: filepath.Base(imgPath),
			Caption:  caption,
		}
		if err := a.channelMgr.SendMedia(a.ctx, channel, chatID, media); err != nil {
			a.logger.Warn("failed to send generated image", "error", err)
		} else {
			a.logger.Info("auto-sent generated image to channel", "path", imgPath)
			// Clean up temp file.
			os.Remove(imgPath)
		}
	}
}

func (a *Assistant) sendReply(original *channels.IncomingMessage, content string) {
	content = FormatForChannel(content, original.Channel)
	if content == "" {
		return // Nothing to send (e.g. NO_REPLY, HEARTBEAT_OK, or only tags).
	}

	maxLen := MaxMessageDefault
	// Could be per-channel configurable later (e.g. WhatsApp: MaxMessageWhatsApp)

	chunks := SplitMessage(content, maxLen)
	if chunks == nil {
		chunks = []string{content}
	}
	for _, chunk := range chunks {
		outMsg := &channels.OutgoingMessage{
			Content: chunk,
			ReplyTo: original.ID,
		}
		if err := a.channelMgr.Send(a.ctx, original.Channel, original.ChatID, outMsg); err != nil {
			a.logger.Error("failed to send reply chunk",
				"channel", original.Channel,
				"chat_id", original.ChatID,
				"error", err,
			)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Active run persistence — restart recovery
// ─────────────────────────────────────────────────────────────────────────────

// markRunActive persists an active run entry in the DB so that if the process
// restarts, we know which sessions had work in progress.
func (a *Assistant) markRunActive(sessionID, channel, chatID, userMessage string) {
	if a.devclawDB == nil {
		return
	}
	_, err := a.devclawDB.Exec(`
		INSERT OR REPLACE INTO active_runs (session_id, channel, chat_id, user_message, started_at)
		VALUES (?, ?, ?, ?, datetime('now'))
	`, sessionID, channel, chatID, userMessage)
	if err != nil {
		a.logger.Warn("failed to mark run active", "session", sessionID, "error", err)
	}
}

// clearRunActive removes the active run entry after normal completion.
func (a *Assistant) clearRunActive(sessionID string) {
	if a.devclawDB == nil {
		return
	}
	_, err := a.devclawDB.Exec(`DELETE FROM active_runs WHERE session_id = ?`, sessionID)
	if err != nil {
		a.logger.Warn("failed to clear active run", "session", sessionID, "error", err)
	}
}

// interruptedRun holds information about a run that was active when the process
// was last terminated.
type interruptedRun struct {
	SessionID   string
	Channel     string
	ChatID      string
	UserMessage string
	StartedAt   string
}

// loadInterruptedRuns reads all active_runs rows from the DB.
// These represent runs that were in progress when the process last exited.
func (a *Assistant) loadInterruptedRuns() []interruptedRun {
	if a.devclawDB == nil {
		return nil
	}
	rows, err := a.devclawDB.Query(`SELECT session_id, channel, chat_id, user_message, started_at FROM active_runs`)
	if err != nil {
		a.logger.Warn("failed to query interrupted runs", "error", err)
		return nil
	}
	defer rows.Close()

	var runs []interruptedRun
	for rows.Next() {
		var r interruptedRun
		if err := rows.Scan(&r.SessionID, &r.Channel, &r.ChatID, &r.UserMessage, &r.StartedAt); err != nil {
			a.logger.Warn("failed to scan interrupted run", "error", err)
			continue
		}
		runs = append(runs, r)
	}
	return runs
}

// resumeInterruptedRuns checks for runs that were active when the process
// last exited and re-submits them to the message pipeline so the user
// doesn't lose work-in-progress tasks after a restart.
func (a *Assistant) resumeInterruptedRuns() {
	runs := a.loadInterruptedRuns()
	if len(runs) == 0 {
		return
	}

	a.logger.Info("found interrupted runs from previous session", "count", len(runs))

	for _, r := range runs {
		// Clear the stale entry first — the new run will create its own.
		a.clearRunActive(r.SessionID)

		// Truncate the original message for display.
		preview := r.UserMessage
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}

		// Notify the user that we're resuming.
		resumeNotice := fmt.Sprintf(
			"🔄 *Retomando tarefa interrompida*\n\nEu fui reiniciado enquanto processava sua solicitação:\n> %s\n\nContinuando de onde parei...",
			preview,
		)
		outMsg := &channels.OutgoingMessage{
			Content: FormatForChannel(resumeNotice, r.Channel),
		}
		if err := a.channelMgr.Send(a.ctx, r.Channel, r.ChatID, outMsg); err != nil {
			a.logger.Error("failed to notify about resumed run",
				"channel", r.Channel, "chat_id", r.ChatID, "error", err)
			continue
		}

		a.logger.Info("re-submitting interrupted task",
			"channel", r.Channel,
			"chat_id", r.ChatID,
			"message_preview", preview,
		)

		// Resume the task directly via the agent, bypassing access checks
		// (the user already had access when the original run started).
		go func(run interruptedRun) {
			// Small delay to let channels fully stabilize.
			time.Sleep(2 * time.Second)

			// Resolve workspace (uses empty senderJID, non-group).
			resolved := a.workspaceMgr.Resolve(run.Channel, run.ChatID, "", false)
			if resolved == nil {
				a.logger.Error("could not resolve workspace for interrupted run",
					"channel", run.Channel, "chat_id", run.ChatID)
				return
			}

			session := resolved.Session
			sessionID := MakeSessionID(run.Channel, run.ChatID)

			// Propagate caller/session via context (goroutine-safe).
			resumeCtx := ContextWithCaller(a.ctx, AccessOwner, "system:resume")
			resumeCtx = ContextWithSession(resumeCtx, sessionID)
			resumeCtx = ContextWithDelivery(resumeCtx, run.Channel, run.ChatID)

			prompt := a.composeWorkspacePrompt(resolved.Workspace, session, run.UserMessage)

			// Get model override from session config.
			modelOverride := session.GetConfig().Model

			// Build block streamer for progressive output.
			blockStreamer := NewBlockStreamer(
				DefaultBlockStreamConfig(),
				a.channelMgr,
				run.Channel, run.ChatID, "",
			)
			defer blockStreamer.Finish()

			response := a.executeAgentWithStream(
				resumeCtx, resolved.Workspace.ID, session, sessionID,
				prompt, run.UserMessage, blockStreamer, modelOverride,
			)

			// Flush any remaining streamed text.
			blockStreamer.Finish()

			// Send final response if there's leftover and streamer didn't send it.
			if response != "" && !blockStreamer.HasSentBlocks() {
				formatted := FormatForChannel(response, run.Channel)
				outMsg := &channels.OutgoingMessage{Content: formatted}
				_ = a.channelMgr.Send(a.ctx, run.Channel, run.ChatID, outMsg)
			}

			// Save to session history.
			session.AddMessage(run.UserMessage, response)
		}(r)
	}
}

