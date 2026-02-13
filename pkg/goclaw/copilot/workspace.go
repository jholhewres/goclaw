// Package copilot – workspace.go implements the multi-tenant workspace system.
//
// Workspaces (also called "profiles") allow multiple people to use the same
// WhatsApp number with completely isolated contexts:
//
//   - Each workspace has its own system prompt, skills, model, and language
//   - Each workspace has its own session store (isolated conversation memory)
//   - A workspace can be assigned to specific users (JIDs) or groups
//   - One user can belong to one workspace at a time
//   - There is always a "default" workspace for unassigned users
//
// Example use cases:
//   - Personal workspace: your own assistant with custom instructions
//   - Team workspace: shared context for a project team
//   - Client workspace: different personality/language per client
//   - Testing workspace: experimental settings without affecting production
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Workspace represents an isolated assistant profile.
// Each workspace has its own instructions, skills, model, and memory.
type Workspace struct {
	// ID is the unique workspace identifier (slug, e.g. "personal", "work").
	ID string `yaml:"id"`

	// Name is the human-readable workspace name.
	Name string `yaml:"name"`

	// Description explains the workspace purpose.
	Description string `yaml:"description"`

	// Instructions are the custom system prompt for this workspace.
	// Overrides the global instructions when set.
	Instructions string `yaml:"instructions"`

	// Model overrides the default LLM model.
	// Empty = use global default.
	Model string `yaml:"model"`

	// Language overrides the default language.
	// Empty = use global default.
	Language string `yaml:"language"`

	// Timezone overrides the default timezone.
	// Empty = use global default.
	Timezone string `yaml:"timezone"`

	// Trigger overrides the activation keyword for this workspace.
	// Empty = use global default.
	Trigger string `yaml:"trigger"`

	// Skills lists the skills available in this workspace.
	// Empty = use all globally enabled skills.
	Skills []string `yaml:"skills"`

	// TokenBudget overrides token limits for this workspace.
	// Nil = use global defaults.
	TokenBudget *TokenBudgetConfig `yaml:"token_budget,omitempty"`

	// MaxMessages overrides the session history limit.
	// 0 = use global default.
	MaxMessages int `yaml:"max_messages"`

	// Members lists the user JIDs assigned to this workspace.
	Members []string `yaml:"members"`

	// Groups lists the group JIDs assigned to this workspace.
	Groups []string `yaml:"groups"`

	// CreatedBy is the JID of whoever created this workspace.
	CreatedBy string `yaml:"created_by"`

	// CreatedAt is when the workspace was created.
	CreatedAt time.Time `yaml:"created_at"`

	// Active indicates if the workspace is enabled.
	Active bool `yaml:"active"`
}

// WorkspaceConfig holds the workspaces configuration.
type WorkspaceConfig struct {
	// DefaultWorkspace is the ID of the workspace used for unassigned contacts.
	DefaultWorkspace string `yaml:"default_workspace"`

	// Workspaces is the list of defined workspaces.
	Workspaces []Workspace `yaml:"workspaces"`
}

// DefaultWorkspaceConfig returns a minimal workspace configuration.
func DefaultWorkspaceConfig() WorkspaceConfig {
	return WorkspaceConfig{
		DefaultWorkspace: "default",
		Workspaces: []Workspace{
			{
				ID:          "default",
				Name:        "Default",
				Description: "Default workspace for all users",
				Active:      true,
				CreatedAt:   time.Now(),
			},
		},
	}
}

// WorkspaceManager manages workspaces and their member assignments.
// It provides workspace resolution for incoming messages and maintains
// isolated session stores per workspace.
type WorkspaceManager struct {
	globalCfg *Config
	logger    *slog.Logger

	// workspaces stores all workspaces by ID.
	workspaces map[string]*Workspace

	// userMap maps user JID → workspace ID.
	userMap map[string]string

	// groupMap maps group JID → workspace ID.
	groupMap map[string]string

	// sessions stores isolated SessionStores per workspace.
	sessions map[string]*SessionStore

	// defaultWSID is the fallback workspace ID.
	defaultWSID string

	mu sync.RWMutex
}

// NewWorkspaceManager creates a new workspace manager.
func NewWorkspaceManager(globalCfg *Config, wsCfg WorkspaceConfig, logger *slog.Logger) *WorkspaceManager {
	if logger == nil {
		logger = slog.Default()
	}

	wm := &WorkspaceManager{
		globalCfg:   globalCfg,
		logger:      logger.With("component", "workspaces"),
		workspaces:  make(map[string]*Workspace),
		userMap:     make(map[string]string),
		groupMap:    make(map[string]string),
		sessions:    make(map[string]*SessionStore),
		defaultWSID: wsCfg.DefaultWorkspace,
	}

	// Load workspaces from config.
	for i := range wsCfg.Workspaces {
		ws := &wsCfg.Workspaces[i]
		wm.workspaces[ws.ID] = ws

		// Create isolated session store for this workspace.
		wm.sessions[ws.ID] = NewSessionStore(
			logger.With("workspace", ws.ID),
		)

		// Map members to workspace.
		for _, jid := range ws.Members {
			wm.userMap[normalizeJID(jid)] = ws.ID
		}
		for _, gid := range ws.Groups {
			wm.groupMap[normalizeJID(gid)] = ws.ID
		}
	}

	// Ensure default workspace exists.
	if _, ok := wm.workspaces[wm.defaultWSID]; !ok && wm.defaultWSID != "" {
		wm.workspaces[wm.defaultWSID] = &Workspace{
			ID:     wm.defaultWSID,
			Name:   "Default",
			Active: true,
		}
		wm.sessions[wm.defaultWSID] = NewSessionStore(
			logger.With("workspace", wm.defaultWSID),
		)
	}

	wm.logger.Info("workspace manager initialized",
		"workspaces", len(wm.workspaces),
		"default", wm.defaultWSID,
	)

	return wm
}

// ResolvedWorkspace contains the resolved workspace and session for a message.
type ResolvedWorkspace struct {
	// Workspace is the resolved workspace.
	Workspace *Workspace

	// Session is the workspace-isolated session for this chat.
	Session *Session

	// SessionStore is the workspace session store (for pruning, etc.).
	SessionStore *SessionStore
}

// Resolve determines which workspace a message belongs to and returns
// the workspace along with its isolated session.
func (wm *WorkspaceManager) Resolve(channel, chatID, senderJID string, isGroup bool) *ResolvedWorkspace {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	wsID := wm.resolveWorkspaceID(chatID, senderJID, isGroup)

	ws, ok := wm.workspaces[wsID]
	if !ok || !ws.Active {
		// Fallback to default.
		wsID = wm.defaultWSID
		ws = wm.workspaces[wsID]
	}

	store := wm.sessions[wsID]
	if store == nil {
		// Should never happen, but be safe.
		store = NewSessionStore(wm.logger)
		wm.sessions[wsID] = store
	}

	session := store.GetOrCreate(channel, chatID)

	// Apply workspace overrides to session config.
	wm.applyWorkspaceConfig(ws, session)

	return &ResolvedWorkspace{
		Workspace:    ws,
		Session:      session,
		SessionStore: store,
	}
}

// resolveWorkspaceID finds the workspace for a JID/group.
func (wm *WorkspaceManager) resolveWorkspaceID(chatID, senderJID string, isGroup bool) string {
	normSender := normalizeJID(senderJID)
	normChat := normalizeJID(chatID)

	// 1. Check group assignment first (for group messages).
	if isGroup {
		if wsID, ok := wm.groupMap[normChat]; ok {
			return wsID
		}
	}

	// 2. Check user assignment.
	if wsID, ok := wm.userMap[normSender]; ok {
		return wsID
	}

	// 3. Fallback to default.
	return wm.defaultWSID
}

// applyWorkspaceConfig applies workspace overrides to a session.
func (wm *WorkspaceManager) applyWorkspaceConfig(ws *Workspace, session *Session) {
	cfg := session.GetConfig()
	changed := false

	if ws.Model != "" && cfg.Model != ws.Model {
		cfg.Model = ws.Model
		changed = true
	}
	if ws.Language != "" && cfg.Language != ws.Language {
		cfg.Language = ws.Language
		changed = true
	}
	if ws.Trigger != "" && cfg.Trigger != ws.Trigger {
		cfg.Trigger = ws.Trigger
		changed = true
	}
	if ws.Instructions != "" && cfg.BusinessContext != ws.Instructions {
		cfg.BusinessContext = ws.Instructions
		changed = true
	}

	if changed {
		session.SetConfig(cfg)
	}

	// Apply skills if workspace defines specific ones.
	if len(ws.Skills) > 0 {
		session.SetActiveSkills(ws.Skills)
	}
}

// --- Admin operations ---

// Create creates a new workspace.
func (wm *WorkspaceManager) Create(ws Workspace, createdBy string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if ws.ID == "" {
		return fmt.Errorf("workspace ID cannot be empty")
	}
	if _, exists := wm.workspaces[ws.ID]; exists {
		return fmt.Errorf("workspace %q already exists", ws.ID)
	}

	ws.CreatedBy = createdBy
	ws.CreatedAt = time.Now()
	ws.Active = true

	wm.workspaces[ws.ID] = &ws
	wm.sessions[ws.ID] = NewSessionStore(
		wm.logger.With("workspace", ws.ID),
	)

	// Map members.
	for _, jid := range ws.Members {
		wm.userMap[normalizeJID(jid)] = ws.ID
	}
	for _, gid := range ws.Groups {
		wm.groupMap[normalizeJID(gid)] = ws.ID
	}

	wm.logger.Info("workspace created",
		"id", ws.ID, "name", ws.Name, "by", createdBy)
	return nil
}

// Delete removes a workspace (members go back to default).
func (wm *WorkspaceManager) Delete(wsID, deletedBy string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wsID == wm.defaultWSID {
		return fmt.Errorf("cannot delete the default workspace")
	}

	ws, exists := wm.workspaces[wsID]
	if !exists {
		return fmt.Errorf("workspace %q not found", wsID)
	}

	// Unmap all members.
	for _, jid := range ws.Members {
		delete(wm.userMap, normalizeJID(jid))
	}
	for _, gid := range ws.Groups {
		delete(wm.groupMap, normalizeJID(gid))
	}

	delete(wm.workspaces, wsID)
	delete(wm.sessions, wsID)

	wm.logger.Info("workspace deleted",
		"id", wsID, "by", deletedBy)
	return nil
}

// AssignUser assigns a user to a workspace.
func (wm *WorkspaceManager) AssignUser(jid, wsID, assignedBy string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workspaces[wsID]; !exists {
		return fmt.Errorf("workspace %q not found", wsID)
	}

	norm := normalizeJID(jid)

	// Remove from old workspace member list.
	if oldWS, ok := wm.userMap[norm]; ok {
		if ws, exists := wm.workspaces[oldWS]; exists {
			ws.Members = removeFromSlice(ws.Members, jid)
		}
	}

	wm.userMap[norm] = wsID
	wm.workspaces[wsID].Members = append(wm.workspaces[wsID].Members, jid)

	wm.logger.Info("user assigned to workspace",
		"jid", norm, "workspace", wsID, "by", assignedBy)
	return nil
}

// AssignGroup assigns a group to a workspace.
func (wm *WorkspaceManager) AssignGroup(groupJID, wsID, assignedBy string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workspaces[wsID]; !exists {
		return fmt.Errorf("workspace %q not found", wsID)
	}

	norm := normalizeJID(groupJID)

	// Remove from old workspace.
	if oldWS, ok := wm.groupMap[norm]; ok {
		if ws, exists := wm.workspaces[oldWS]; exists {
			ws.Groups = removeFromSlice(ws.Groups, groupJID)
		}
	}

	wm.groupMap[norm] = wsID
	wm.workspaces[wsID].Groups = append(wm.workspaces[wsID].Groups, groupJID)

	wm.logger.Info("group assigned to workspace",
		"group", norm, "workspace", wsID, "by", assignedBy)
	return nil
}

// UnassignUser removes a user from their workspace (goes to default).
func (wm *WorkspaceManager) UnassignUser(jid string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	norm := normalizeJID(jid)
	if wsID, ok := wm.userMap[norm]; ok {
		if ws, exists := wm.workspaces[wsID]; exists {
			ws.Members = removeFromSlice(ws.Members, jid)
		}
		delete(wm.userMap, norm)
	}
}

// Get returns a workspace by ID.
func (wm *WorkspaceManager) Get(wsID string) (*Workspace, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	ws, ok := wm.workspaces[wsID]
	return ws, ok
}

// GetForUser returns the workspace assigned to a user JID.
func (wm *WorkspaceManager) GetForUser(jid string) (*Workspace, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	wsID, ok := wm.userMap[normalizeJID(jid)]
	if !ok {
		return wm.workspaces[wm.defaultWSID], true
	}
	ws, exists := wm.workspaces[wsID]
	return ws, exists
}

// List returns all workspaces.
func (wm *WorkspaceManager) List() []*Workspace {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]*Workspace, 0, len(wm.workspaces))
	for _, ws := range wm.workspaces {
		result = append(result, ws)
	}
	return result
}

// Count returns the number of workspaces.
func (wm *WorkspaceManager) Count() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.workspaces)
}

// Update modifies a workspace's settings.
func (wm *WorkspaceManager) Update(wsID string, fn func(ws *Workspace)) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	ws, exists := wm.workspaces[wsID]
	if !exists {
		return fmt.Errorf("workspace %q not found", wsID)
	}

	fn(ws)
	return nil
}

// StartPruners starts session pruning for all workspace session stores.
func (wm *WorkspaceManager) StartPruners(ctx context.Context) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	for _, store := range wm.sessions {
		store.StartPruner(ctx)
	}
}

// --- Helpers ---

func removeFromSlice(slice []string, item string) []string {
	for i, s := range slice {
		if s == item || normalizeJID(s) == normalizeJID(item) {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
