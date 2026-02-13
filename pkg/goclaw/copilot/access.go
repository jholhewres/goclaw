// Package copilot – access.go implements the access control system for GoClaw.
//
// Like OpenClaw, the bot does NOT respond to everyone by default. Only
// explicitly authorized contacts (or groups) can interact with the assistant.
//
// Access levels:
//   - owner:   Full control, can manage admins, workspaces, and settings
//   - admin:   Can manage users, create workspaces, and use admin commands
//   - user:    Can interact with the assistant normally
//   - blocked: Explicitly blocked, never receives a response
//
// Default policy: "deny" — unknown contacts are silently ignored.
package copilot

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

// AccessLevel defines the permission level of a contact.
type AccessLevel string

const (
	AccessOwner   AccessLevel = "owner"
	AccessAdmin   AccessLevel = "admin"
	AccessUser    AccessLevel = "user"
	AccessBlocked AccessLevel = "blocked"
	AccessUnknown AccessLevel = ""
)

// AccessPolicy defines how unknown contacts are handled.
type AccessPolicy string

const (
	// PolicyDeny silently ignores unknown contacts (default, like OpenClaw).
	PolicyDeny AccessPolicy = "deny"

	// PolicyAllow responds to everyone not explicitly blocked.
	PolicyAllow AccessPolicy = "allow"

	// PolicyAsk sends a one-time "request access" message to unknown contacts.
	PolicyAsk AccessPolicy = "ask"
)

// AccessConfig holds the access control configuration.
type AccessConfig struct {
	// DefaultPolicy determines behavior for unknown contacts.
	// "deny" (default) = ignore unknown, "allow" = respond to all, "ask" = request access.
	DefaultPolicy AccessPolicy `yaml:"default_policy"`

	// Owners have full control (phone numbers or JIDs).
	Owners []string `yaml:"owners"`

	// Admins can manage users and workspaces.
	Admins []string `yaml:"admins"`

	// AllowedUsers can interact with the bot.
	AllowedUsers []string `yaml:"allowed_users"`

	// BlockedUsers are explicitly blocked.
	BlockedUsers []string `yaml:"blocked_users"`

	// AllowedGroups are group IDs where the bot responds.
	AllowedGroups []string `yaml:"allowed_groups"`

	// BlockedGroups are group IDs where the bot stays silent.
	BlockedGroups []string `yaml:"blocked_groups"`

	// AllowGroupAdmins grants "user" access to group admins automatically.
	AllowGroupAdmins bool `yaml:"allow_group_admins"`

	// PendingMessage is the message sent to unknown contacts when policy is "ask".
	PendingMessage string `yaml:"pending_message"`
}

// DefaultAccessConfig returns the default access control config.
func DefaultAccessConfig() AccessConfig {
	return AccessConfig{
		DefaultPolicy:  PolicyDeny,
		PendingMessage: "Access not authorized. Please contact an admin to request access.",
	}
}

// AccessEntry represents a contact in the access list.
type AccessEntry struct {
	// JID is the contact identifier (phone@server or group@server).
	JID string

	// Level is the access level.
	Level AccessLevel

	// AddedBy is the JID of the admin/owner who granted access.
	AddedBy string

	// AddedAt is when access was granted.
	AddedAt time.Time

	// Note is an optional admin note about this contact.
	Note string
}

// AccessManager handles access control for incoming messages.
type AccessManager struct {
	cfg    AccessConfig
	logger *slog.Logger

	// Runtime entries (can be modified via admin commands).
	users  map[string]*AccessEntry // JID → entry
	groups map[string]*AccessEntry // group JID → entry

	// Tracks contacts that already received the "pending" message
	// to avoid spamming them.
	askedOnce map[string]time.Time

	mu sync.RWMutex
}

// NewAccessManager creates a new access manager from config.
func NewAccessManager(cfg AccessConfig, logger *slog.Logger) *AccessManager {
	if logger == nil {
		logger = slog.Default()
	}

	am := &AccessManager{
		cfg:       cfg,
		logger:    logger.With("component", "access"),
		users:     make(map[string]*AccessEntry),
		groups:    make(map[string]*AccessEntry),
		askedOnce: make(map[string]time.Time),
	}

	// Seed from config.
	now := time.Now()
	for _, jid := range cfg.Owners {
		am.users[normalizeJID(jid)] = &AccessEntry{
			JID: normalizeJID(jid), Level: AccessOwner,
			AddedBy: "config", AddedAt: now,
		}
	}
	for _, jid := range cfg.Admins {
		am.users[normalizeJID(jid)] = &AccessEntry{
			JID: normalizeJID(jid), Level: AccessAdmin,
			AddedBy: "config", AddedAt: now,
		}
	}
	for _, jid := range cfg.AllowedUsers {
		am.users[normalizeJID(jid)] = &AccessEntry{
			JID: normalizeJID(jid), Level: AccessUser,
			AddedBy: "config", AddedAt: now,
		}
	}
	for _, jid := range cfg.BlockedUsers {
		am.users[normalizeJID(jid)] = &AccessEntry{
			JID: normalizeJID(jid), Level: AccessBlocked,
			AddedBy: "config", AddedAt: now,
		}
	}
	for _, gid := range cfg.AllowedGroups {
		am.groups[normalizeJID(gid)] = &AccessEntry{
			JID: normalizeJID(gid), Level: AccessUser,
			AddedBy: "config", AddedAt: now,
		}
	}
	for _, gid := range cfg.BlockedGroups {
		am.groups[normalizeJID(gid)] = &AccessEntry{
			JID: normalizeJID(gid), Level: AccessBlocked,
			AddedBy: "config", AddedAt: now,
		}
	}

	am.logger.Info("access manager initialized",
		"policy", cfg.DefaultPolicy,
		"owners", len(cfg.Owners),
		"admins", len(cfg.Admins),
		"users", len(cfg.AllowedUsers),
		"blocked", len(cfg.BlockedUsers),
		"groups", len(cfg.AllowedGroups),
	)

	return am
}

// CheckResult contains the result of an access check.
type CheckResult struct {
	// Allowed is true if the contact can interact.
	Allowed bool

	// Level is the resolved access level.
	Level AccessLevel

	// ShouldAsk is true if we should send a "request access" message.
	ShouldAsk bool

	// Reason explains why access was denied (for logging).
	Reason string
}

// Check evaluates whether an incoming message should be processed.
// This is the main entry point called before any message handling.
func (am *AccessManager) Check(msg *channels.IncomingMessage) CheckResult {
	am.mu.RLock()
	defer am.mu.RUnlock()

	from := normalizeJID(msg.From)
	chatID := normalizeJID(msg.ChatID)

	// 1. Check if sender is explicitly blocked.
	if entry, ok := am.users[from]; ok && entry.Level == AccessBlocked {
		return CheckResult{
			Allowed: false, Level: AccessBlocked,
			Reason: "user explicitly blocked",
		}
	}

	// 2. Check if group is explicitly blocked.
	if msg.IsGroup {
		if entry, ok := am.groups[chatID]; ok && entry.Level == AccessBlocked {
			return CheckResult{
				Allowed: false, Level: AccessBlocked,
				Reason: "group explicitly blocked",
			}
		}
	}

	// 3. Check if sender has explicit access.
	if entry, ok := am.users[from]; ok {
		if entry.Level == AccessOwner || entry.Level == AccessAdmin || entry.Level == AccessUser {
			return CheckResult{Allowed: true, Level: entry.Level}
		}
	}

	// 4. For group messages, check group access.
	if msg.IsGroup {
		if entry, ok := am.groups[chatID]; ok && entry.Level == AccessUser {
			// Group is allowed, grant user-level access.
			return CheckResult{Allowed: true, Level: AccessUser}
		}
	}

	// 5. Apply default policy for unknown contacts.
	switch am.cfg.DefaultPolicy {
	case PolicyAllow:
		return CheckResult{Allowed: true, Level: AccessUser}

	case PolicyAsk:
		// Check if we already asked this contact.
		if _, asked := am.askedOnce[from]; !asked {
			return CheckResult{
				Allowed:   false,
				Level:     AccessUnknown,
				ShouldAsk: true,
				Reason:    "unknown contact, sending access request",
			}
		}
		return CheckResult{
			Allowed: false, Level: AccessUnknown,
			Reason: "unknown contact, already asked",
		}

	default: // PolicyDeny
		return CheckResult{
			Allowed: false, Level: AccessUnknown,
			Reason: "unknown contact, default policy is deny",
		}
	}
}

// MarkAsked records that we sent the "pending" message to a contact.
func (am *AccessManager) MarkAsked(jid string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.askedOnce[normalizeJID(jid)] = time.Now()
}

// --- Admin operations (called via chat commands) ---

// Grant gives access to a contact at the specified level.
func (am *AccessManager) Grant(jid string, level AccessLevel, grantedBy string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	norm := normalizeJID(jid)

	// Cannot grant owner level via command.
	if level == AccessOwner {
		return fmt.Errorf("owner level can only be set in config")
	}

	am.users[norm] = &AccessEntry{
		JID:     norm,
		Level:   level,
		AddedBy: grantedBy,
		AddedAt: time.Now(),
	}

	// Remove from asked cache if they were pending.
	delete(am.askedOnce, norm)

	am.logger.Info("access granted",
		"jid", norm, "level", level, "by", grantedBy)
	return nil
}

// GrantGroup gives access to a group.
func (am *AccessManager) GrantGroup(groupJID string, level AccessLevel, grantedBy string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	norm := normalizeJID(groupJID)

	am.groups[norm] = &AccessEntry{
		JID:     norm,
		Level:   level,
		AddedBy: grantedBy,
		AddedAt: time.Now(),
	}

	am.logger.Info("group access granted",
		"group", norm, "level", level, "by", grantedBy)
	return nil
}

// Revoke removes access from a contact.
func (am *AccessManager) Revoke(jid string, revokedBy string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	norm := normalizeJID(jid)
	delete(am.users, norm)
	am.logger.Info("access revoked", "jid", norm, "by", revokedBy)
}

// Block explicitly blocks a contact.
func (am *AccessManager) Block(jid string, blockedBy string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	norm := normalizeJID(jid)
	am.users[norm] = &AccessEntry{
		JID:     norm,
		Level:   AccessBlocked,
		AddedBy: blockedBy,
		AddedAt: time.Now(),
	}

	am.logger.Info("user blocked", "jid", norm, "by", blockedBy)
}

// Unblock removes a block from a contact.
func (am *AccessManager) Unblock(jid string, unblockedBy string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	norm := normalizeJID(jid)
	if entry, ok := am.users[norm]; ok && entry.Level == AccessBlocked {
		delete(am.users, norm)
		am.logger.Info("user unblocked", "jid", norm, "by", unblockedBy)
	}
}

// GetLevel returns the access level for a JID.
func (am *AccessManager) GetLevel(jid string) AccessLevel {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if entry, ok := am.users[normalizeJID(jid)]; ok {
		return entry.Level
	}
	return AccessUnknown
}

// IsOwner returns true if the JID is an owner.
func (am *AccessManager) IsOwner(jid string) bool {
	return am.GetLevel(jid) == AccessOwner
}

// IsAdmin returns true if the JID is an admin or owner.
func (am *AccessManager) IsAdmin(jid string) bool {
	level := am.GetLevel(jid)
	return level == AccessAdmin || level == AccessOwner
}

// ListUsers returns all access entries.
func (am *AccessManager) ListUsers() []*AccessEntry {
	am.mu.RLock()
	defer am.mu.RUnlock()

	entries := make([]*AccessEntry, 0, len(am.users))
	for _, e := range am.users {
		entries = append(entries, e)
	}
	return entries
}

// ListGroups returns all group access entries.
func (am *AccessManager) ListGroups() []*AccessEntry {
	am.mu.RLock()
	defer am.mu.RUnlock()

	entries := make([]*AccessEntry, 0, len(am.groups))
	for _, e := range am.groups {
		entries = append(entries, e)
	}
	return entries
}

// PendingMessage returns the message to send to unknown contacts.
func (am *AccessManager) PendingMessage() string {
	if am.cfg.PendingMessage != "" {
		return am.cfg.PendingMessage
	}
	return "Access not authorized. Please contact an admin."
}

// --- Helpers ---

// normalizeJID strips whitespace and ensures consistent format.
func normalizeJID(jid string) string {
	jid = strings.TrimSpace(jid)

	// If it's just a phone number, add WhatsApp suffix.
	if jid != "" && !strings.Contains(jid, "@") {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, jid)
		if len(digits) >= 10 {
			return digits + "@s.whatsapp.net"
		}
	}

	return jid
}
