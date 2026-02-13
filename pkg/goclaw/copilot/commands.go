// Package copilot – commands.go implements admin commands that can be
// executed via chat messages (WhatsApp, Discord, etc.).
//
// Commands are prefixed with "/" and only available to admins/owners:
//
//	/allow <phone>           - Grant user access
//	/block <phone>           - Block a user
//	/unblock <phone>         - Unblock a user
//	/revoke <phone>          - Revoke user access
//	/admin <phone>           - Promote user to admin
//	/users                   - List all authorized users
//	/ws create <id> <name>   - Create a workspace
//	/ws delete <id>          - Delete a workspace
//	/ws assign <phone> <id>  - Assign user to workspace
//	/ws list                 - List all workspaces
//	/ws info [id]            - Show workspace details
//	/ws set <key> <value>    - Update current workspace setting
//	/group allow             - Allow current group
//	/group block             - Block current group
//	/group assign <ws_id>    - Assign current group to workspace
//	/status                  - Show bot status
//	/help                    - Show available commands
package copilot

import (
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

// CommandResult contains the result of a command execution.
type CommandResult struct {
	// Response is the text to send back.
	Response string

	// Handled is true if the message was a valid command.
	Handled bool
}

// IsCommand returns true if the message starts with "/".
func IsCommand(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "/")
}

// HandleCommand processes an admin command from a chat message.
// Returns handled=true if it was a valid command (even if permission denied).
func (a *Assistant) HandleCommand(msg *channels.IncomingMessage) CommandResult {
	content := strings.TrimSpace(msg.Content)
	if !IsCommand(content) {
		return CommandResult{Handled: false}
	}

	// Parse command and args.
	parts := strings.Fields(content)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	// Check permissions.
	senderLevel := a.accessMgr.GetLevel(msg.From)
	isAdmin := senderLevel == AccessOwner || senderLevel == AccessAdmin

	switch cmd {
	case "/help":
		return CommandResult{
			Response: a.helpCommand(isAdmin),
			Handled:  true,
		}

	case "/status":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.statusCommand(), Handled: true}

	case "/allow":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.allowCommand(args, msg.From), Handled: true}

	case "/block":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.blockCommand(args, msg.From), Handled: true}

	case "/unblock":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.unblockCommand(args, msg.From), Handled: true}

	case "/revoke":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.revokeCommand(args, msg.From), Handled: true}

	case "/admin":
		if senderLevel != AccessOwner {
			return CommandResult{Response: "Only owners can promote admins.", Handled: true}
		}
		return CommandResult{Response: a.adminCommand(args, msg.From), Handled: true}

	case "/users":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.usersCommand(), Handled: true}

	case "/ws", "/workspace":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.workspaceCommand(args, msg), Handled: true}

	case "/group":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.groupCommand(args, msg), Handled: true}

	default:
		return CommandResult{Handled: false}
	}
}

// --- Command implementations ---

func (a *Assistant) helpCommand(isAdmin bool) string {
	var b strings.Builder
	b.WriteString("*GoClaw Commands*\n\n")

	if isAdmin {
		b.WriteString("*Access Control:*\n")
		b.WriteString("/allow <phone> - Grant user access\n")
		b.WriteString("/block <phone> - Block a user\n")
		b.WriteString("/unblock <phone> - Unblock a user\n")
		b.WriteString("/revoke <phone> - Revoke access\n")
		b.WriteString("/admin <phone> - Promote to admin\n")
		b.WriteString("/users - List authorized users\n\n")

		b.WriteString("*Workspaces:*\n")
		b.WriteString("/ws create <id> <name> - Create workspace\n")
		b.WriteString("/ws delete <id> - Delete workspace\n")
		b.WriteString("/ws assign <phone> <id> - Assign user\n")
		b.WriteString("/ws list - List workspaces\n")
		b.WriteString("/ws info [id] - Workspace details\n\n")

		b.WriteString("*Groups:*\n")
		b.WriteString("/group allow - Allow this group\n")
		b.WriteString("/group block - Block this group\n")
		b.WriteString("/group assign <ws_id> - Assign to workspace\n\n")

		b.WriteString("/status - Bot status\n")
	}

	b.WriteString("/help - Show this message")
	return b.String()
}

func (a *Assistant) statusCommand() string {
	health := a.channelMgr.HealthAll()
	workspaces := a.workspaceMgr.Count()
	users := a.accessMgr.ListUsers()

	var b strings.Builder
	b.WriteString("*GoClaw Status*\n\n")
	b.WriteString(fmt.Sprintf("Workspaces: %d\n", workspaces))
	b.WriteString(fmt.Sprintf("Users: %d\n", len(users)))

	for name, h := range health {
		status := "disconnected"
		if h.Connected {
			status = "connected"
		}
		b.WriteString(fmt.Sprintf("Channel %s: %s (errors: %d)\n", name, status, h.ErrorCount))
	}

	return b.String()
}

func (a *Assistant) allowCommand(args []string, grantedBy string) string {
	if len(args) < 1 {
		return "Usage: /allow <phone_number>"
	}
	jid := args[0]
	if err := a.accessMgr.Grant(jid, AccessUser, grantedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return fmt.Sprintf("User %s has been granted access.", jid)
}

func (a *Assistant) blockCommand(args []string, blockedBy string) string {
	if len(args) < 1 {
		return "Usage: /block <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Block(jid, blockedBy)
	return fmt.Sprintf("User %s has been blocked.", jid)
}

func (a *Assistant) unblockCommand(args []string, unblockedBy string) string {
	if len(args) < 1 {
		return "Usage: /unblock <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Unblock(jid, unblockedBy)
	return fmt.Sprintf("User %s has been unblocked.", jid)
}

func (a *Assistant) revokeCommand(args []string, revokedBy string) string {
	if len(args) < 1 {
		return "Usage: /revoke <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Revoke(jid, revokedBy)
	return fmt.Sprintf("Access revoked for %s.", jid)
}

func (a *Assistant) adminCommand(args []string, grantedBy string) string {
	if len(args) < 1 {
		return "Usage: /admin <phone_number>"
	}
	jid := args[0]
	if err := a.accessMgr.Grant(jid, AccessAdmin, grantedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return fmt.Sprintf("User %s promoted to admin.", jid)
}

func (a *Assistant) usersCommand() string {
	entries := a.accessMgr.ListUsers()
	if len(entries) == 0 {
		return "No users configured."
	}

	var b strings.Builder
	b.WriteString("*Authorized Users:*\n\n")

	for _, e := range entries {
		b.WriteString(fmt.Sprintf("• %s [%s]", e.JID, e.Level))
		if e.Note != "" {
			b.WriteString(fmt.Sprintf(" - %s", e.Note))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (a *Assistant) workspaceCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		return "Usage: /ws <create|delete|assign|list|info> [args...]"
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "create":
		if len(subArgs) < 2 {
			return "Usage: /ws create <id> <name...>"
		}
		id := subArgs[0]
		name := strings.Join(subArgs[1:], " ")
		ws := Workspace{
			ID:   id,
			Name: name,
		}
		if err := a.workspaceMgr.Create(ws, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Workspace '%s' (%s) created.", name, id)

	case "delete":
		if len(subArgs) < 1 {
			return "Usage: /ws delete <id>"
		}
		if err := a.workspaceMgr.Delete(subArgs[0], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Workspace '%s' deleted.", subArgs[0])

	case "assign":
		if len(subArgs) < 2 {
			return "Usage: /ws assign <phone> <workspace_id>"
		}
		if err := a.workspaceMgr.AssignUser(subArgs[0], subArgs[1], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("User %s assigned to workspace '%s'.", subArgs[0], subArgs[1])

	case "list":
		workspaces := a.workspaceMgr.List()
		if len(workspaces) == 0 {
			return "No workspaces configured."
		}

		var b strings.Builder
		b.WriteString("*Workspaces:*\n\n")
		for _, ws := range workspaces {
			status := "active"
			if !ws.Active {
				status = "inactive"
			}
			b.WriteString(fmt.Sprintf("• *%s* (%s) - %s\n", ws.Name, ws.ID, status))
			b.WriteString(fmt.Sprintf("  Members: %d | Groups: %d\n", len(ws.Members), len(ws.Groups)))
			if ws.Model != "" {
				b.WriteString(fmt.Sprintf("  Model: %s\n", ws.Model))
			}
		}
		return b.String()

	case "info":
		wsID := ""
		if len(subArgs) > 0 {
			wsID = subArgs[0]
		} else {
			// Show workspace for the current sender.
			if ws, ok := a.workspaceMgr.GetForUser(msg.From); ok {
				wsID = ws.ID
			}
		}
		if wsID == "" {
			return "Usage: /ws info <id>"
		}

		ws, ok := a.workspaceMgr.Get(wsID)
		if !ok {
			return fmt.Sprintf("Workspace '%s' not found.", wsID)
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("*Workspace: %s*\n", ws.Name))
		b.WriteString(fmt.Sprintf("ID: %s\n", ws.ID))
		b.WriteString(fmt.Sprintf("Active: %v\n", ws.Active))
		if ws.Description != "" {
			b.WriteString(fmt.Sprintf("Description: %s\n", ws.Description))
		}
		if ws.Model != "" {
			b.WriteString(fmt.Sprintf("Model: %s\n", ws.Model))
		}
		if ws.Language != "" {
			b.WriteString(fmt.Sprintf("Language: %s\n", ws.Language))
		}
		if ws.Instructions != "" {
			instr := ws.Instructions
			if len(instr) > 100 {
				instr = instr[:100] + "..."
			}
			b.WriteString(fmt.Sprintf("Instructions: %s\n", instr))
		}
		if len(ws.Skills) > 0 {
			b.WriteString(fmt.Sprintf("Skills: %s\n", strings.Join(ws.Skills, ", ")))
		}
		b.WriteString(fmt.Sprintf("Members (%d): %s\n", len(ws.Members), strings.Join(ws.Members, ", ")))
		b.WriteString(fmt.Sprintf("Groups (%d): %s\n", len(ws.Groups), strings.Join(ws.Groups, ", ")))
		if !ws.CreatedAt.IsZero() {
			b.WriteString(fmt.Sprintf("Created: %s", ws.CreatedAt.Format(time.RFC3339)))
			if ws.CreatedBy != "" {
				b.WriteString(fmt.Sprintf(" by %s", ws.CreatedBy))
			}
			b.WriteString("\n")
		}

		return b.String()

	default:
		return "Unknown workspace command. Use: create, delete, assign, list, info"
	}
}

func (a *Assistant) groupCommand(args []string, msg *channels.IncomingMessage) string {
	if !msg.IsGroup {
		return "This command can only be used in groups."
	}

	if len(args) == 0 {
		return "Usage: /group <allow|block|assign> [args...]"
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "allow":
		if err := a.accessMgr.GrantGroup(msg.ChatID, AccessUser, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return "Group allowed."

	case "block":
		if err := a.accessMgr.GrantGroup(msg.ChatID, AccessBlocked, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return "Group blocked."

	case "assign":
		if len(subArgs) < 1 {
			return "Usage: /group assign <workspace_id>"
		}
		if err := a.workspaceMgr.AssignGroup(msg.ChatID, subArgs[0], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Group assigned to workspace '%s'.", subArgs[0])

	default:
		return "Unknown group command. Use: allow, block, assign"
	}
}
