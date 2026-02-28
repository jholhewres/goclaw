// Package copilot – scheduler_tools.go implements the scheduler dispatcher tool.
// Uses dispatcher pattern to consolidate cron_add, cron_list, cron_remove,
// and reminder_search into a single tool.
package copilot

import (
	"context"
	"fmt"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/scheduler"
)

// RegisterSchedulerDispatcher registers a single "scheduler" dispatcher tool
// that replaces cron_add, cron_list, cron_remove, and reminder_search.
func RegisterSchedulerDispatcher(executor *ToolExecutor, sched *scheduler.Scheduler, skillDB *SkillDB) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"add", "list", "remove", "search"},
				"description": "Action: add (schedule task), list (show jobs), remove (delete job), search (find reminders by keyword)",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID (for add/remove)",
			},
			"schedule": map[string]any{
				"type":        "string",
				"description": "Natural language ('every 5 minutes', 'daily at 9am') or cron/interval format (for add)",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "'at' = fires ONCE (reminders), 'every' = repeats at interval, 'cron' = cron schedule (for add)",
				"enum":        []string{"cron", "every", "at"},
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Prompt/command to execute when job fires (for add)",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "Target channel for response (for add)",
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Target chat/group ID (for add)",
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Confirm removal (required for remove action)",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query for reminders (for search)",
			},
			"include_removed": map[string]any{
				"type":        "boolean",
				"description": "Include removed reminders in search results (for search)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results (for search, default: 20)",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("scheduler",
			"Schedule tasks and reminders. Actions: add (schedule), list (show jobs), remove (delete job), search (find reminders).",
			schema),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			switch action {
			case "add":
				return handleSchedulerAdd(ctx, sched, skillDB, args)
			case "list":
				return handleSchedulerList(sched)
			case "remove":
				return handleSchedulerRemove(sched, skillDB, args)
			case "search":
				return handleSchedulerSearch(skillDB, args)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: add, list, remove, search)", action)
			}
		},
	)
}

func handleSchedulerAdd(ctx context.Context, sched *scheduler.Scheduler, skillDB *SkillDB, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	schedule, _ := args["schedule"].(string)
	jobType, _ := args["type"].(string)
	command, _ := args["command"].(string)
	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)

	if id == "" || schedule == "" || command == "" {
		return nil, fmt.Errorf("id, schedule, and command are required for add action")
	}

	// Try natural language parsing first.
	if parsed, ok := scheduler.ParseNaturalLanguage(schedule); ok {
		schedule = parsed.Schedule
		if jobType == "" {
			jobType = parsed.Type
		}
	}

	if jobType == "" {
		jobType = "cron"
	}

	// Auto-fill channel/chatID from context.
	if channel == "" || chatID == "" {
		dt := DeliveryTargetFromContext(ctx)
		if dt.Channel != "" && channel == "" {
			channel = dt.Channel
		}
		if dt.ChatID != "" && chatID == "" {
			chatID = dt.ChatID
		}
	}

	job := &scheduler.Job{
		ID:       id,
		Schedule: schedule,
		Type:     jobType,
		Command:  command,
		Channel:  channel,
		ChatID:   chatID,
		Enabled:  true,
	}

	if err := sched.Add(job); err != nil {
		return nil, fmt.Errorf("scheduling job %q: %w", id, err)
	}

	// Save reminder to tracking table (best-effort).
	if skillDB != nil {
		_ = skillDB.SaveReminder(id, jobType, schedule, command, channel, chatID)
	}

	return fmt.Sprintf("Job '%s' scheduled: %s (%s) → %s:%s", id, schedule, jobType, channel, chatID), nil
}

func handleSchedulerList(sched *scheduler.Scheduler) (any, error) {
	jobs := sched.List()
	if len(jobs) == 0 {
		return "No scheduled jobs.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Scheduled jobs (%d):\n\n", len(jobs))
	for _, j := range jobs {
		status := "enabled"
		if !j.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(&sb, "- **%s** [%s] schedule=%s type=%s\n  Command: %s\n  Runs: %d",
			j.ID, status, j.Schedule, j.Type, j.Command, j.RunCount)
		if j.LastRunAt != nil {
			fmt.Fprintf(&sb, "  Last run: %s", j.LastRunAt.Format("2006-01-02 15:04"))
		}
		if j.LastError != "" {
			fmt.Fprintf(&sb, "  Last error: %s", j.LastError)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func handleSchedulerRemove(sched *scheduler.Scheduler, skillDB *SkillDB, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	confirm, _ := args["confirm"].(bool)
	if id == "" {
		return nil, fmt.Errorf("id is required for remove action")
	}
	if !confirm {
		return nil, fmt.Errorf("removal not confirmed. Set confirm=true to remove job '%s'", id)
	}
	if err := sched.Remove(id); err != nil {
		return nil, fmt.Errorf("removing job %q: %w", id, err)
	}

	// Mark reminder as removed in tracking table (best-effort).
	if skillDB != nil {
		_ = skillDB.MarkReminderRemoved(id)
	}

	return fmt.Sprintf("Job '%s' removed.", id), nil
}

func handleSchedulerSearch(skillDB *SkillDB, args map[string]any) (any, error) {
	if skillDB == nil {
		return nil, fmt.Errorf("reminder tracking not available")
	}

	query, _ := args["query"].(string)
	includeRemoved, _ := args["include_removed"].(bool)
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	reminders, err := skillDB.SearchReminders(query, includeRemoved, limit)
	if err != nil {
		return nil, fmt.Errorf("search reminders: %w", err)
	}

	if len(reminders) == 0 {
		if query != "" {
			return fmt.Sprintf("No reminders found matching '%s'.", query), nil
		}
		return "No reminders found.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Reminders found (%d):\n\n", len(reminders))
	for _, r := range reminders {
		status := r.Status
		if status == "removed" {
			status = "removed"
		}
		fmt.Fprintf(&sb, "- **%s** [%s] type=%s\n  Schedule: %s\n  Command: %s\n  Created: %s\n",
			r.JobID, status, r.JobType, r.Schedule, r.Command, r.CreatedAt)
		if r.RemovedAt != "" {
			fmt.Fprintf(&sb, "  Removed: %s\n", r.RemovedAt)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
