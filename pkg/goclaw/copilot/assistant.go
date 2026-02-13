// Package copilot implements the main orchestrator for GoClaw.
// Coordinates channels, skills, scheduler, access control, workspaces,
// and security to process user messages and generate LLM responses.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
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

	// 1. Load skills.
	if err := a.skillRegistry.LoadAll(a.ctx); err != nil {
		a.logger.Error("failed to load skills", "error", err)
	}

	// 2. Start channel manager (allows 0 channels for CLI mode).
	if err := a.channelMgr.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start channels: %w", err)
	}

	// 3. Start session pruners for all workspaces.
	a.workspaceMgr.StartPruners(a.ctx)

	// 4. Start scheduler if enabled.
	if a.scheduler != nil {
		if err := a.scheduler.Start(a.ctx); err != nil {
			a.logger.Error("failed to start scheduler", "error", err)
		}
	}

	// 5. Start main message processing loop.
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
			logger.Debug("message ignored (access denied)",
				"reason", accessResult.Reason)
		}
		return
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

	// ── Step 4: Validate input ──
	if err := a.inputGuard.Validate(msg.From, msg.Content); err != nil {
		logger.Warn("input rejected", "error", err)
		a.sendReply(msg, fmt.Sprintf("Sorry, I can't process that: %v", err))
		return
	}

	// ── Step 5: Build prompt with workspace context ──
	prompt := a.composeWorkspacePrompt(workspace, session, msg.Content)

	// ── Step 6: Execute agent ──
	// TODO: Integrate with the real agent executor.
	response := a.executeAgent(a.ctx, prompt, session)

	// ── Step 7: Validate output ──
	if err := a.outputGuard.Validate(response); err != nil {
		logger.Warn("output rejected, applying fallback", "error", err)
		response = "Sorry, I encountered an issue generating the response. Could you rephrase?"
	}

	// ── Step 8: Update session ──
	session.AddMessage(msg.Content, response)

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

// executeAgent runs the LLM agent with the composed prompt.
func (a *Assistant) executeAgent(_ context.Context, prompt string, _ *Session) string {
	// TODO: Integrate with AgentGo SDK (agent.Run, tools, etc).
	_ = prompt
	return "GoClaw Copilot is under development. I'll be fully functional soon!"
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

