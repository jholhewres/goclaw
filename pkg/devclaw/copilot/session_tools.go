// Package copilot – session_tools.go implements the sessions dispatcher tool.
// Uses dispatcher pattern to consolidate 4 session tools into 1.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RegisterSessionsDispatcher registers a single "sessions" dispatcher tool that
// replaces sessions_list, sessions_delete, sessions_export, sessions_send.
func RegisterSessionsDispatcher(executor *ToolExecutor, wm *WorkspaceManager) {
	if wm == nil {
		return
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "delete", "export", "send"},
				"description": "Action: list (show sessions), delete (remove session), export (backup history), send (inter-agent message)",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Target session ID (for delete/export/send)",
			},
			"channel_filter": map[string]any{
				"type":        "string",
				"description": "Filter by channel name (for list, e.g. 'whatsapp', 'webui')",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message text (for send action)",
			},
			"sender_label": map[string]any{
				"type":        "string",
				"description": "Sender label (for send, e.g. 'research-agent')",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("sessions",
			"Manage conversation sessions. Actions: list, delete, export, send (inter-agent messaging).",
			schema),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			// Write actions (delete, send) require admin level.
			// Read actions (list, export) are safe for any user.
			if action == "delete" || action == "send" {
				level := CallerLevelFromContext(ctx)
				if level != AccessOwner && level != AccessAdmin {
					return nil, fmt.Errorf("action %q requires admin access (current: %s)", action, level)
				}
			}

			switch action {
			case "list":
				return handleSessionsList(wm, args)
			case "delete":
				return handleSessionsDelete(wm, args)
			case "export":
				return handleSessionsExport(wm, args)
			case "send":
				return handleSessionsSend(wm, args)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: list, delete, export, send)", action)
			}
		},
	)
}

func handleSessionsList(wm *WorkspaceManager, args map[string]any) (any, error) {
	channelFilter, _ := args["channel_filter"].(string)

	allSessions := wm.ListAllSessions()
	if len(allSessions) == 0 {
		return "No active sessions.", nil
	}

	var b strings.Builder
	count := 0
	for _, info := range allSessions {
		if channelFilter != "" && info.Channel != channelFilter {
			continue
		}
		ago := time.Since(info.LastActiveAt).Round(time.Second)
		fmt.Fprintf(&b, "- [%s] %s (id: %s, ws: %s) — %d msgs — last active: %s ago\n",
			info.Channel, info.ChatID, info.ID, info.WorkspaceID, info.MessageCount, ago)
		count++
	}

	if count == 0 {
		return fmt.Sprintf("No sessions found for channel '%s'.", channelFilter), nil
	}

	return fmt.Sprintf("Active sessions (%d):\n%s", count, b.String()), nil
}

func handleSessionsDelete(wm *WorkspaceManager, args map[string]any) (any, error) {
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for delete action")
	}
	if wm.DeleteSessionByID(sessionID) {
		return fmt.Sprintf("Session %s deleted.", sessionID), nil
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}

func handleSessionsExport(wm *WorkspaceManager, args map[string]any) (any, error) {
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for export action")
	}
	export := wm.ExportSession(sessionID)
	if export == nil {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	data, err := json.Marshal(export)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal export: %w", err)
	}
	result := string(data)
	if len(result) > 20000 {
		result = result[:20000] + "\n... (truncated, export too large)"
	}
	return result, nil
}

func handleSessionsSend(wm *WorkspaceManager, args map[string]any) (any, error) {
	sessionID, _ := args["session_id"].(string)
	message, _ := args["message"].(string)
	senderLabel, _ := args["sender_label"].(string)
	if sessionID == "" || message == "" {
		return nil, fmt.Errorf("session_id and message are required for send action")
	}
	if senderLabel == "" {
		senderLabel = "agent"
	}

	session := wm.FindSessionByID(sessionID)
	if session == nil {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}

	// Read channel under lock per CLAUDE.md concurrency rules.
	session.mu.RLock()
	channel := session.Channel
	session.mu.RUnlock()

	session.AddMessage(
		fmt.Sprintf("[Inter-agent message from %s]: %s", senderLabel, message),
		"",
	)

	return fmt.Sprintf("Message delivered to session %s (channel: %s).", sessionID, channel), nil
}
