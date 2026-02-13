// Package copilot implements the main orchestrator for GoClaw.
// Coordinates channels, skills, scheduler, access control, workspaces,
// and security to process user messages and generate LLM responses.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// Assistant is the main orchestrator for GoClaw.
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

	// skillRegistry manages available skills.
	skillRegistry *skills.Registry

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

	// memoryStore provides persistent long-term memory.
	memoryStore *memory.FileStore

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

	return &Assistant{
		config:         cfg,
		channelMgr:     channels.NewManager(logger.With("component", "channels")),
		accessMgr:      NewAccessManager(cfg.Access, logger),
		workspaceMgr:   NewWorkspaceManager(cfg, cfg.Workspaces, logger),
		llmClient:      NewLLMClient(cfg, logger),
		toolExecutor:   NewToolExecutor(logger),
		skillRegistry:  skills.NewRegistry(logger.With("component", "skills")),
		sessionStore:   NewSessionStore(logger.With("component", "sessions")),
		promptComposer: NewPromptComposer(cfg),
		inputGuard:     security.NewInputGuardrail(cfg.Security.MaxInputLength, cfg.Security.RateLimit),
		outputGuard:    security.NewOutputGuardrail(),
		logger:         logger,
	}
}

// Start initializes and starts all subsystems.
func (a *Assistant) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.logger.Info("starting GoClaw Copilot",
		"name", a.config.Name,
		"model", a.config.Model,
		"access_policy", a.config.Access.DefaultPolicy,
		"workspaces", a.workspaceMgr.Count(),
	)

	// 0. Initialize memory store.
	memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
	memStore, err := memory.NewFileStore(memDir)
	if err != nil {
		a.logger.Warn("memory store not available", "error", err)
	} else {
		a.memoryStore = memStore
	}

	// 0b. Connect memory store and skill getter to prompt composer.
	if a.memoryStore != nil {
		a.promptComposer.SetMemoryStore(a.memoryStore)
	}
	a.promptComposer.SetSkillGetter(func(name string) (interface{ SystemPrompt() string }, bool) {
		skill, ok := a.skillRegistry.Get(name)
		if !ok {
			return nil, false
		}
		return skill, true
	})

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

	// 1e. Register system tools (needs scheduler to be created first).
	a.registerSystemTools()

	// 2. Start channel manager (allows 0 channels for CLI mode).
	if err := a.channelMgr.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start channels: %w", err)
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
		hb := NewHeartbeat(a.config.Heartbeat, a, a.logger)
		hb.Start(a.ctx)
	}

	// 6. Start main message processing loop.
	go a.messageLoop()

	a.logger.Info("GoClaw Copilot started successfully")
	return nil
}

// Stop gracefully shuts down all subsystems.
func (a *Assistant) Stop() {
	a.logger.Info("stopping GoClaw Copilot...")

	if a.cancel != nil {
		a.cancel()
	}

	// Shut down in reverse initialization order.
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	a.channelMgr.Stop()
	a.skillRegistry.ShutdownAll()

	a.logger.Info("GoClaw Copilot stopped")
}

// ChannelManager returns the channel manager for external registration.
func (a *Assistant) ChannelManager() *channels.Manager {
	return a.channelMgr
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

// SetScheduler configures the assistant's scheduler.
func (a *Assistant) SetScheduler(s *scheduler.Scheduler) {
	a.scheduler = s
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
	// This is the OpenClaw-style behavior: unknown contacts are silently ignored.
	accessResult := a.accessMgr.Check(msg)

	if !accessResult.Allowed {
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
	if !a.matchesTrigger(msg.Content, trigger, msg.IsGroup) {
		return
	}

	logger.Info("message received, processing...",
		"access_level", accessResult.Level)

	// ── Step 3b: Send typing indicator and mark as read ──
	a.channelMgr.SendTyping(a.ctx, msg.Channel, msg.ChatID)
	a.channelMgr.MarkRead(a.ctx, msg.Channel, msg.ChatID, []string{msg.ID})

	// ── Step 4: Validate input ──
	if err := a.inputGuard.Validate(msg.From, msg.Content); err != nil {
		logger.Warn("input rejected", "error", err)
		a.sendReply(msg, fmt.Sprintf("Sorry, I can't process that: %v", err))
		return
	}

	// ── Step 5: Build prompt with workspace context ──
	prompt := a.composeWorkspacePrompt(workspace, session, msg.Content)

	// ── Step 6: Execute agent ──
	response := a.executeAgent(a.ctx, prompt, session, msg.Content)

	// ── Step 7: Validate output ──
	if err := a.outputGuard.Validate(response); err != nil {
		logger.Warn("output rejected, applying fallback", "error", err)
		response = "Sorry, I encountered an issue generating the response. Could you rephrase?"
	}

	// ── Step 8: Update session ──
	session.AddMessage(msg.Content, response)

	// ── Step 8b: Check if session needs compaction ──
	a.maybeCompactSession(session)

	// ── Step 9: Send reply ──
	a.sendReply(msg, response)

	logger.Info("message processed",
		"duration_ms", time.Since(start).Milliseconds(),
		"workspace", workspace.ID,
	)
}

// matchesTrigger checks if a message matches the activation keyword.
// In DMs, the trigger is optional (always responds).
// In groups, the trigger is required unless the group has its own trigger.
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

// executeAgent runs the agentic loop with tool use support.
func (a *Assistant) executeAgent(ctx context.Context, systemPrompt string, session *Session, userMessage string) string {
	history := session.RecentHistory(20)

	agent := NewAgentRun(a.llmClient, a.toolExecutor, a.logger)
	response, err := agent.Run(ctx, systemPrompt, history, userMessage)
	if err != nil {
		a.logger.Error("agent failed", "error", err)
		return fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	return response
}

// ToolExecutor returns the tool executor for external tool registration.
func (a *Assistant) ToolExecutor() *ToolExecutor {
	return a.toolExecutor
}

// SessionStore returns the session store (used by CLI chat).
func (a *Assistant) SessionStore() *SessionStore {
	return a.sessionStore
}

// ComposePrompt builds a system prompt for the given session and input.
// Convenience method for CLI and external callers.
func (a *Assistant) ComposePrompt(session *Session, input string) string {
	return a.promptComposer.Compose(session, input)
}

// ExecuteAgent runs the agent loop with tools and returns the response text.
// Public wrapper for CLI and external callers.
func (a *Assistant) ExecuteAgent(ctx context.Context, systemPrompt string, session *Session, userMessage string) string {
	return a.executeAgent(ctx, systemPrompt, session, userMessage)
}

// initScheduler creates and configures the scheduler with file-based storage.
func (a *Assistant) initScheduler() {
	storagePath := a.config.Scheduler.Storage
	if storagePath == "" {
		storagePath = "./data/scheduler.json"
	}

	storage, err := scheduler.NewFileJobStorage(storagePath)
	if err != nil {
		a.logger.Error("failed to create scheduler storage", "error", err)
		return
	}

	// Job handler: runs the command as an agent turn.
	handler := func(ctx context.Context, job *scheduler.Job) (string, error) {
		a.logger.Info("scheduler executing job", "id", job.ID, "command", job.Command)

		// Get or create a session for this scheduled job.
		session := a.sessionStore.GetOrCreate("scheduler", job.ID)

		prompt := a.promptComposer.Compose(session, job.Command)

		agent := NewAgentRun(a.llmClient, a.toolExecutor, a.logger)
		result, err := agent.Run(ctx, prompt, session.RecentHistory(10), job.Command)
		if err != nil {
			return "", err
		}

		// Save to session history.
		session.AddMessage(job.Command, result)

		// If job has a target channel/chat, send the result.
		if job.Channel != "" && job.ChatID != "" {
			outMsg := &channels.OutgoingMessage{Content: result}
			if sendErr := a.channelMgr.Send(ctx, job.Channel, job.ChatID, outMsg); sendErr != nil {
				a.logger.Error("failed to deliver scheduled message",
					"job_id", job.ID, "error", sendErr)
			}
		}

		return result, nil
	}

	a.scheduler = scheduler.New(storage, handler, a.logger)
	a.logger.Info("scheduler initialized", "storage", storagePath)
}

// registerSkillLoaders registers the builtin and clawdhub skill loaders
// in the skill registry based on configuration.
func (a *Assistant) registerSkillLoaders() {
	// Builtin skills loader.
	if len(a.config.Skills.Builtin) > 0 {
		builtinLoader := skills.NewBuiltinLoader(a.config.Skills.Builtin, a.logger)
		a.skillRegistry.AddLoader(builtinLoader)
	}

	// ClawdHub (OpenClaw-compatible) skills loader.
	if len(a.config.Skills.ClawdHubDirs) > 0 {
		clawdHubLoader := skills.NewClawdHubLoader(a.config.Skills.ClawdHubDirs, a.logger)
		a.skillRegistry.AddLoader(clawdHubLoader)
	}
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

	RegisterSystemTools(a.toolExecutor, sandboxRunner, a.memoryStore, a.scheduler, dataDir)

	// Register skill creator tools.
	RegisterSkillCreatorTools(a.toolExecutor, a.skillRegistry, "./skills")

	a.logger.Info("system tools registered",
		"tools", a.toolExecutor.ToolNames(),
	)
}

// maybeCompactSession checks if the session history is too large and compacts it.
// Before compaction, performs a memory flush: a silent agent turn that extracts
// important facts from the conversation into long-term memory.
func (a *Assistant) maybeCompactSession(session *Session) {
	threshold := a.config.Memory.MaxMessages
	if threshold <= 0 {
		threshold = 100
	}

	if session.HistoryLen() < threshold {
		return
	}

	a.logger.Info("session compaction triggered",
		"session", session.ID,
		"history_len", session.HistoryLen(),
		"threshold", threshold,
	)

	// Step 1: Memory flush — ask the agent to extract important facts.
	if a.memoryStore != nil {
		flushPrompt := "Extract the most important facts, preferences, and information from this conversation that should be remembered long-term. Save them using the memory_save tool. If nothing important, reply with NO_REPLY."

		agent := NewAgentRun(a.llmClient, a.toolExecutor, a.logger)
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

	// Step 2: Ask the LLM to summarize the old history.
	summaryPrompt := "Summarize the key points of this conversation in 2-3 sentences. Focus on decisions made, tasks completed, and important context."
	summary, err := a.llmClient.Complete(a.ctx, "", session.RecentHistory(20), summaryPrompt)
	if err != nil {
		summary = "Previous conversation context was compacted."
	}

	// Step 3: Compact the session.
	keepRecent := threshold / 4 // Keep 25% of the threshold as recent history.
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

	a.logger.Info("session compacted",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// truncate returns the first n characters of s, adding "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// sendReply sends a response to the original message's channel.
func (a *Assistant) sendReply(original *channels.IncomingMessage, content string) {
	outMsg := &channels.OutgoingMessage{
		Content: content,
		ReplyTo: original.ID,
	}

	if err := a.channelMgr.Send(a.ctx, original.Channel, original.ChatID, outMsg); err != nil {
		a.logger.Error("failed to send reply",
			"channel", original.Channel,
			"chat_id", original.ChatID,
			"error", err,
		)
	}
}

