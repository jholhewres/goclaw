// Package copilot – exec_approval.go implements interactive approval for tools
// that require confirmation before execution (e.g. bash, ssh, write_file).
package copilot

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// sanitizeForMarkdown prevents backtick injection in Discord/Slack markdown
// by inserting a zero-width space after each backtick.
func sanitizeForMarkdown(s string) string {
	return strings.ReplaceAll(s, "`", "`\u200b")
}

const (
	// ApprovalTimeout is how long to wait for user approval before giving up.
	// 120s gives ample time for users to read and respond via chat.
	ApprovalTimeout = 120 * time.Second
)

// ApprovalResult holds the outcome of an approval request.
type ApprovalResult struct {
	Approved bool
	Reason   string
}

// PendingApproval represents a tool call waiting for user approval.
type PendingApproval struct {
	ID          string
	ToolName    string
	Args        map[string]any
	Description string
	SessionID   string
	CallerJID   string
	CreatedAt   time.Time
	Result      chan ApprovalResult
}

// ApprovalManager manages pending tool approvals and their resolution.
// It also tracks session-scoped trust: once a user approves a tool in a session,
// subsequent uses of the same tool are auto-approved (no re-prompting).
type ApprovalManager struct {
	pending map[string]*PendingApproval

	// sessionTrust tracks which tools have been approved per session.
	// key: "sessionID:toolName" → true means auto-approved for this session.
	sessionTrust map[string]bool

	mu     sync.Mutex
	logger *slog.Logger
}

// NewApprovalManager creates a new approval manager.
func NewApprovalManager(logger *slog.Logger) *ApprovalManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ApprovalManager{
		pending:      make(map[string]*PendingApproval),
		sessionTrust: make(map[string]bool),
		logger:       logger.With("component", "approval_manager"),
	}
}

// Create creates a pending approval and returns the ID and message for the user.
// The caller should send the message to the chat, then call Wait to block for the result.
func (m *ApprovalManager) Create(sessionID, callerJID, toolName string, args map[string]any) (id string, message string) {
	desc := formatApprovalDescription(toolName, args)
	id = uuid.New().String()

	pa := &PendingApproval{
		ID:          id,
		ToolName:    toolName,
		Args:        args,
		Description: desc,
		SessionID:   sessionID,
		CallerJID:   callerJID,
		CreatedAt:   time.Now(),
		Result:      make(chan ApprovalResult, 1),
	}

	m.mu.Lock()
	m.pending[id] = pa
	m.mu.Unlock()

	message = fmt.Sprintf("⚠️ Approval required: %s\n\nReply /approve %s or /deny %s", desc, id, id)

	m.logger.Info("approval created",
		"id", id,
		"tool", toolName,
		"session", sessionID,
	)

	return id, message
}

// Wait blocks until the approval is resolved or times out.
// Must be called after Create. Removes the pending approval when done.
func (m *ApprovalManager) Wait(id string) (approved bool, err error) {
	m.mu.Lock()
	pa, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return false, fmt.Errorf("approval not found: %s", id)
	}

	defer func() {
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
	}()

	select {
	case res := <-pa.Result:
		if res.Approved {
			m.logger.Info("approval granted", "id", id, "tool", pa.ToolName)
			return true, nil
		}
		m.logger.Info("approval denied", "id", id, "reason", res.Reason)
		return false, nil

	case <-time.After(ApprovalTimeout):
		m.logger.Warn("approval timed out", "id", id, "tool", pa.ToolName)
		return false, fmt.Errorf("approval timed out")
	}
}

// Request creates a pending approval, invokes sendMsg with the approval message,
// then blocks until the user approves, denies, or timeout.
// sendMsg is called so the user sees the approval request (e.g. send to channel).
//
// If the tool has already been approved in this session (session trust), the
// request is auto-approved without prompting the user.
func (m *ApprovalManager) Request(sessionID, callerJID, toolName string, args map[string]any, sendMsg func(msg string)) (bool, error) {
	// Check session trust — if already approved in this session, auto-approve.
	if m.IsTrusted(sessionID, toolName) {
		m.logger.Debug("tool auto-approved (session trust)",
			"tool", toolName,
			"session", sessionID,
		)
		return true, nil
	}

	id, message := m.Create(sessionID, callerJID, toolName, args)
	if sendMsg != nil {
		sendMsg(message)
	}
	approved, err := m.Wait(id)

	// If approved, grant session trust so future calls skip the prompt.
	if approved && err == nil {
		m.GrantTrust(sessionID, toolName)
	}

	return approved, err
}

// Resolve resolves a pending approval by ID. Returns true if the approval was found and resolved.
// resolverJID is the user resolving (must match CallerJID for "own requests only").
func (m *ApprovalManager) Resolve(id, sessionID, resolverJID string, approved bool, reason string) bool {
	m.mu.Lock()
	pa, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return false
	}

	// Per-session: only the session that created the approval can resolve it.
	if pa.SessionID != sessionID {
		m.logger.Warn("approval resolve rejected: session mismatch",
			"id", id, "requested_session", sessionID, "actual_session", pa.SessionID)
		return false
	}

	// Users can only approve their own requests (same JID that triggered the agent).
	if resolverJID != "" && pa.CallerJID != "" && pa.CallerJID != resolverJID {
		m.logger.Warn("approval resolve rejected: caller mismatch",
			"id", id, "resolver", resolverJID, "caller", pa.CallerJID)
		return false
	}

	select {
	case pa.Result <- ApprovalResult{Approved: approved, Reason: reason}:
		return true
	default:
		// Already resolved (e.g. timeout)
		return false
	}
}

// LatestPendingForSession returns the ID of the most recent pending approval
// for the given session, or empty string if none. This allows "/approve" without
// specifying the UUID — it resolves the latest pending request.
func (m *ApprovalManager) LatestPendingForSession(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var latest *PendingApproval
	for _, pa := range m.pending {
		if pa.SessionID == sessionID {
			if latest == nil || pa.CreatedAt.After(latest.CreatedAt) {
				latest = pa
			}
		}
	}
	if latest != nil {
		return latest.ID
	}
	return ""
}

// PendingCountForSession returns the number of pending approvals for a session.
func (m *ApprovalManager) PendingCountForSession(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, pa := range m.pending {
		if pa.SessionID == sessionID {
			count++
		}
	}
	return count
}

// ── Session Trust ──

// IsTrusted returns true if the tool has been previously approved in this session.
func (m *ApprovalManager) IsTrusted(sessionID, toolName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionTrust[sessionID+":"+toolName]
}

// GrantTrust marks a tool as trusted for the given session.
// Future calls to Request for this tool+session will be auto-approved.
func (m *ApprovalManager) GrantTrust(sessionID, toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionTrust[sessionID+":"+toolName] = true
	m.logger.Info("session trust granted",
		"tool", toolName,
		"session", sessionID,
	)
}

// ClearSessionTrust removes all trusted tools for a session (e.g. on /new or /reset).
func (m *ApprovalManager) ClearSessionTrust(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := sessionID + ":"
	for key := range m.sessionTrust {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(m.sessionTrust, key)
		}
	}
}

// formatApprovalDescription builds a human-readable description of the tool action.
func formatApprovalDescription(toolName string, args map[string]any) string {
	switch toolName {
	case "bash", "exec":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			if len(cmd) > 80 {
				return fmt.Sprintf("run: %s...", sanitizeForMarkdown(cmd[:80]))
			}
			return "run: " + sanitizeForMarkdown(cmd)
		}
		return fmt.Sprintf("%s (command)", toolName)

	case "write_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return "write to " + path
		}
		return "write_file"

	case "edit_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return "edit " + path
		}
		return "edit_file"

	case "ssh":
		if host, ok := args["host"].(string); ok && host != "" {
			cmd, _ := args["command"].(string)
			if cmd != "" {
				return fmt.Sprintf("ssh %s: %s", sanitizeForMarkdown(host), sanitizeForMarkdown(truncateForApproval(cmd, 40)))
			}
			return "ssh " + sanitizeForMarkdown(host)
		}
		return "ssh"

	case "scp":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		if src != "" && dst != "" {
			return fmt.Sprintf("scp %s → %s", src, dst)
		}
		return "scp"

	default:
		return toolName
	}
}

func truncateForApproval(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
