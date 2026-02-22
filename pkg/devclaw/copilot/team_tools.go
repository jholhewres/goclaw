// Package copilot – team_tools.go implements tools for team/agent management.
// These tools allow agents and users to manage teams, agents, tasks, and communication.
package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/scheduler"
)

// RegisterTeamTools registers all team management tools.
func RegisterTeamTools(
	executor *ToolExecutor,
	teamMgr *TeamManager,
	db *sql.DB,
	sched *scheduler.Scheduler,
	logger *slog.Logger,
) {
	if teamMgr == nil {
		return
	}

	// ── Team Management ──
	registerTeamManagementTools(executor, teamMgr, logger)

	// ── Agent Management ──
	registerAgentManagementTools(executor, teamMgr, logger)

	// ── Task Management ──
	registerTaskTools(executor, teamMgr, db, logger)

	// ── Communication Tools ──
	registerCommunicationTools(executor, teamMgr, db, logger)

	// ── Team Memory Tools ──
	registerTeamMemoryTools(executor, teamMgr, db, logger)

	// ── Document Tools ──
	registerDocumentTools(executor, teamMgr, db, logger)

	// ── Working State Tools ──
	registerWorkingStateTools(executor, teamMgr, logger)

	// ── Standup Tool ──
	registerStandupTool(executor, teamMgr, db, logger)

	logger.Info("team tools registered")
}

// ── Team Management Tools ──

func registerTeamManagementTools(executor *ToolExecutor, teamMgr *TeamManager, logger *slog.Logger) {
	// team_create - Create a new team
	executor.Register(
		MakeToolDefinition("team_create",
			"Create a new team for organizing agents. A team has shared memory, tasks, and agents.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Team name (e.g., 'Marketing', 'Development')",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "What this team does",
					},
					"default_model": map[string]any{
						"type":        "string",
						"description": "Default LLM model for agents (empty = use system default)",
					},
				},
				"required": []string{"name"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			name, _ := args["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("name is required")
			}
			description, _ := args["description"].(string)
			defaultModel, _ := args["default_model"].(string)

			// Get caller JID from context
			callerJID := CallerJIDFromContext(ctx)
			if callerJID == "" {
				callerJID = "system"
			}

			team, err := teamMgr.CreateTeam(name, description, callerJID, defaultModel)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Team created successfully!\n  ID: %s\n  Name: %s\n  Owner: %s",
				team.ID, team.Name, team.OwnerJID), nil
		},
	)

	// team_list - List all teams
	executor.Register(
		MakeToolDefinition("team_list",
			"List all teams with their agents.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teams, err := teamMgr.ListTeams()
			if err != nil {
				return nil, err
			}

			if len(teams) == 0 {
				return "No teams found. Use team_create to create one.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Teams (%d):\n\n", len(teams)))
			for _, t := range teams {
				agents, _ := teamMgr.ListAgents(t.ID)
				sb.WriteString(fmt.Sprintf("**%s** (%s)\n", t.Name, t.ID))
				sb.WriteString(fmt.Sprintf("  Description: %s\n", t.Description))
				sb.WriteString(fmt.Sprintf("  Agents: %d\n", len(agents)))
				for _, a := range agents {
					sb.WriteString(fmt.Sprintf("    - %s (%s) [%s]\n", a.Name, a.Role, a.Status))
				}
				sb.WriteString("\n")
			}

			return sb.String(), nil
		},
	)
}

// ── Agent Management Tools ──

func registerAgentManagementTools(executor *ToolExecutor, teamMgr *TeamManager, logger *slog.Logger) {
	// team_create_agent - Create a persistent agent
	executor.Register(
		MakeToolDefinition("team_create_agent",
			"Create a persistent team agent with a specific role. The agent will wake up periodically (heartbeat) to check for work and @mentions.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID to add agent to",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Agent name (e.g., 'Jarvis', 'Loki', 'Fury')",
					},
					"role": map[string]any{
						"type":        "string",
						"description": "Agent role (e.g., 'Writer', 'Researcher', 'Developer')",
					},
					"personality": map[string]any{
						"type":        "string",
						"description": "SOUL.md content - agent personality and style",
					},
					"instructions": map[string]any{
						"type":        "string",
						"description": "Additional operating instructions",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "LLM model override (empty = team default)",
					},
					"skills": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of skills this agent can use",
					},
					"level": map[string]any{
						"type":        "string",
						"enum":        []string{"intern", "specialist", "lead"},
						"description": "Agent autonomy level (default: specialist)",
					},
					"heartbeat": map[string]any{
						"type":        "string",
						"description": "Cron schedule for heartbeats (default: '*/15 * * * *')",
					},
				},
				"required": []string{"team_id", "name", "role"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			name, _ := args["name"].(string)
			role, _ := args["role"].(string)

			if teamID == "" || name == "" || role == "" {
				return nil, fmt.Errorf("team_id, name, and role are required")
			}

			personality, _ := args["personality"].(string)
			instructions, _ := args["instructions"].(string)
			model, _ := args["model"].(string)
			heartbeat, _ := args["heartbeat"].(string)
			level := AgentLevelSpecialist
			if l, ok := args["level"].(string); ok && l != "" {
				level = AgentLevel(l)
			}

			var skills []string
			if s, ok := args["skills"].([]interface{}); ok {
				for _, skill := range s {
					if str, ok := skill.(string); ok {
						skills = append(skills, str)
					}
				}
			}

			agent, err := teamMgr.CreateAgent(teamID, name, role, personality, instructions, model, skills, level, heartbeat)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Agent created successfully!\n  ID: %s\n  Name: %s\n  Role: %s\n  Level: %s\n  Heartbeat: %s\n\nUse @%s to mention this agent.",
				agent.ID, agent.Name, agent.Role, agent.Level, agent.HeartbeatSchedule, agent.ID), nil
		},
	)

	// team_list_agents - List agents in a team
	executor.Register(
		MakeToolDefinition("team_list_agents",
			"List all agents in a team with their current status.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (empty = all teams)",
					},
				},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			agents, err := teamMgr.ListAgents(teamID)
			if err != nil {
				return nil, err
			}

			if len(agents) == 0 {
				if teamID != "" {
					return fmt.Sprintf("No agents found in team %s.", teamID), nil
				}
				return "No agents found. Use team_create_agent to create one.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Agents (%d):\n\n", len(agents)))
			for _, a := range agents {
				sb.WriteString(fmt.Sprintf("**%s** (@%s)\n", a.Name, a.ID))
				sb.WriteString(fmt.Sprintf("  Role: %s | Level: %s | Status: %s\n", a.Role, a.Level, a.Status))
				if a.Model != "" {
					sb.WriteString(fmt.Sprintf("  Model: %s\n", a.Model))
				}
				if len(a.Skills) > 0 {
					sb.WriteString(fmt.Sprintf("  Skills: %s\n", strings.Join(a.Skills, ", ")))
				}
				if a.LastHeartbeatAt != nil {
					sb.WriteString(fmt.Sprintf("  Last heartbeat: %s\n", a.LastHeartbeatAt.Format("15:04")))
				}
				sb.WriteString("\n")
			}

			return sb.String(), nil
		},
	)

	// team_stop_agent - Stop a persistent agent
	executor.Register(
		MakeToolDefinition("team_stop_agent",
			"Stop a persistent agent (disables heartbeats but keeps the agent).",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID to stop",
					},
				},
				"required": []string{"agent_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			agentID, _ := args["agent_id"].(string)
			if agentID == "" {
				return nil, fmt.Errorf("agent_id is required")
			}

			if err := teamMgr.StopAgent(agentID); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Agent %s stopped. Use team_start_agent to restart.", agentID), nil
		},
	)

	// team_delete_agent - Delete an agent permanently
	executor.Register(
		MakeToolDefinition("team_delete_agent",
			"Permanently delete an agent from the team.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID to delete",
					},
				},
				"required": []string{"agent_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			agentID, _ := args["agent_id"].(string)
			if agentID == "" {
				return nil, fmt.Errorf("agent_id is required")
			}

			if err := teamMgr.DeleteAgent(agentID); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Agent %s deleted.", agentID), nil
		},
	)
}

// ── Task Management Tools ──

func registerTaskTools(executor *ToolExecutor, teamMgr *TeamManager, db *sql.DB, logger *slog.Logger) {
	// team_create_task - Create a new task
	executor.Register(
		MakeToolDefinition("team_create_task",
			"Create a new task for the team. Tasks can be assigned to specific agents.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Task title",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Detailed task description",
					},
					"assignees": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Agent IDs to assign (e.g., ['loki', 'fury'])",
					},
				},
				"required": []string{"team_id", "title"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			title, _ := args["title"].(string)
			description, _ := args["description"].(string)

			if teamID == "" || title == "" {
				return nil, fmt.Errorf("team_id and title are required")
			}

			// Get caller info
			caller := CallerJIDFromContext(ctx)
			sessionID := SessionIDFromContext(ctx)
			if caller == "" && sessionID != "" {
				// Try to extract agent ID from session
				if strings.HasPrefix(sessionID, "agent:") {
					parts := strings.Split(sessionID, ":")
					if len(parts) >= 2 {
						caller = parts[1]
					}
				}
			}
			if caller == "" {
				caller = "user"
			}

			var assignees []string
			if a, ok := args["assignees"].([]interface{}); ok {
				for _, id := range a {
					if str, ok := id.(string); ok {
						assignees = append(assignees, str)
					}
				}
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			task, err := teamMem.CreateTask(title, description, caller, assignees)
			if err != nil {
				return nil, err
			}

			result := fmt.Sprintf("Task created!\n  ID: %s\n  Title: %s\n  Status: %s",
				task.ID, task.Title, task.Status)
			if len(assignees) > 0 {
				result += fmt.Sprintf("\n  Assignees: %s", strings.Join(assignees, ", "))
			}
			return result, nil
		},
	)

	// team_list_tasks - List tasks
	executor.Register(
		MakeToolDefinition("team_list_tasks",
			"List tasks in a team. Can filter by status or assignee.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"inbox", "assigned", "in_progress", "review", "done", "blocked", ""},
						"description": "Filter by status (empty = all)",
					},
					"assignee": map[string]any{
						"type":        "string",
						"description": "Filter by assignee agent ID",
					},
				},
				"required": []string{"team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			status, _ := args["status"].(string)
			assignee, _ := args["assignee"].(string)

			if teamID == "" {
				return nil, fmt.Errorf("team_id is required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			tasks, err := teamMem.ListTasks(TaskStatus(status), assignee)
			if err != nil {
				return nil, err
			}

			if len(tasks) == 0 {
				return "No tasks found.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Tasks (%d):\n\n", len(tasks)))
			for _, t := range tasks {
				assignees := "unassigned"
				if len(t.Assignees) > 0 {
					assignees = strings.Join(t.Assignees, ", ")
				}
				sb.WriteString(fmt.Sprintf("**[%s]** %s (%s)\n", strings.ToUpper(string(t.Status)), t.Title, t.ID))
				sb.WriteString(fmt.Sprintf("  Assignees: %s | Created: %s\n", assignees, t.CreatedAt.Format("Jan 02")))
				if t.Description != "" {
					sb.WriteString(fmt.Sprintf("  Description: %s\n", truncateString(t.Description, 100)))
				}
				sb.WriteString("\n")
			}

			return sb.String(), nil
		},
	)

	// team_update_task - Update task status
	executor.Register(
		MakeToolDefinition("team_update_task",
			"Update a task's status and optionally add a comment.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Task ID",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"inbox", "assigned", "in_progress", "review", "done", "blocked", "cancelled"},
						"description": "New status",
					},
					"comment": map[string]any{
						"type":        "string",
						"description": "Optional comment to add to the task thread",
					},
				},
				"required": []string{"team_id", "task_id", "status"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			taskID, _ := args["task_id"].(string)
			status, _ := args["status"].(string)
			comment, _ := args["comment"].(string)

			if teamID == "" || taskID == "" || status == "" {
				return nil, fmt.Errorf("team_id, task_id, and status are required")
			}

			// Get caller
			caller := getCallerID(ctx)

			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.UpdateTask(taskID, TaskStatus(status), comment, caller); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Task %s updated to status: %s", taskID, status), nil
		},
	)

	// team_assign_task - Assign agents to a task
	executor.Register(
		MakeToolDefinition("team_assign_task",
			"Assign agents to a task.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Task ID",
					},
					"assignees": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Agent IDs to assign",
					},
				},
				"required": []string{"team_id", "task_id", "assignees"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			taskID, _ := args["task_id"].(string)

			if teamID == "" || taskID == "" {
				return nil, fmt.Errorf("team_id and task_id are required")
			}

			var assignees []string
			if a, ok := args["assignees"].([]interface{}); ok {
				for _, id := range a {
					if str, ok := id.(string); ok {
						assignees = append(assignees, str)
					}
				}
			}

			if len(assignees) == 0 {
				return nil, fmt.Errorf("at least one assignee is required")
			}

			caller := getCallerID(ctx)

			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.AssignTask(taskID, assignees, caller); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Task %s assigned to: %s", taskID, strings.Join(assignees, ", ")), nil
		},
	)
}

// ── Communication Tools ──

func registerCommunicationTools(executor *ToolExecutor, teamMgr *TeamManager, db *sql.DB, logger *slog.Logger) {
	// team_comment - Add a comment to a task thread
	executor.Register(
		MakeToolDefinition("team_comment",
			"Add a comment to a task thread. Use @agent_id to mention other agents.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Task ID (thread)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Comment content. Use @agent_id to mention agents.",
					},
				},
				"required": []string{"team_id", "task_id", "content"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			taskID, _ := args["task_id"].(string)
			content, _ := args["content"].(string)

			if teamID == "" || taskID == "" || content == "" {
				return nil, fmt.Errorf("team_id, task_id, and content are required")
			}

			caller := getCallerID(ctx)

			// Parse mentions from content
			mentions := teamMgr.ParseMentions(content)

			teamMem := NewTeamMemory(teamID, db, logger)
			msg, err := teamMem.PostMessage(taskID, caller, content, mentions)
			if err != nil {
				return nil, err
			}

			result := fmt.Sprintf("Comment added to task %s.", taskID)
			if len(mentions) > 0 {
				result += fmt.Sprintf(" Mentioned: %s", strings.Join(mentions, ", "))
			}
			result += fmt.Sprintf("\nMessage ID: %s", msg.ID)
			return result, nil
		},
	)

	// team_check_mentions - Check for @mentions to current agent
	executor.Register(
		MakeToolDefinition("team_check_mentions",
			"Check for pending @mentions to you. Returns messages that mentioned you.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"mark_delivered": map[string]any{
						"type":        "boolean",
						"description": "Mark messages as delivered (default: true)",
					},
				},
				"required": []string{"team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			markDelivered := true
			if v, ok := args["mark_delivered"].(bool); ok {
				markDelivered = v
			}

			if teamID == "" {
				return nil, fmt.Errorf("team_id is required")
			}

			agentID := getAgentIDFromContext(ctx)
			if agentID == "" {
				return "No agent context. This tool is for persistent agents only.", nil
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			messages, err := teamMem.GetPendingMessages(agentID, markDelivered)
			if err != nil {
				return nil, err
			}

			if len(messages) == 0 {
				return "No pending mentions.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Pending mentions (%d):\n\n", len(messages)))
			for _, m := range messages {
				from := m.FromAgent
				if from == "" {
					from = "user"
				}
				sb.WriteString(fmt.Sprintf("**From %s** (thread: %s)\n", from, m.ThreadID))
				sb.WriteString(fmt.Sprintf("%s\n\n", m.Content))
			}

			return sb.String(), nil
		},
	)

	// team_send_message - Send a message to another agent
	executor.Register(
		MakeToolDefinition("team_send_message",
			"Send a direct message to another agent.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to_agent": map[string]any{
						"type":        "string",
						"description": "Target agent ID",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Message content",
					},
				},
				"required": []string{"to_agent", "message"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			toAgent, _ := args["to_agent"].(string)
			message, _ := args["message"].(string)

			if toAgent == "" || message == "" {
				return nil, fmt.Errorf("to_agent and message are required")
			}

			fromAgent := getAgentIDFromContext(ctx)

			if err := teamMgr.SendToAgent(ctx, toAgent, fromAgent, message); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Message sent to @%s", toAgent), nil
		},
	)
}

// ── Memory Tools ──

func registerTeamMemoryTools(executor *ToolExecutor, teamMgr *TeamManager, db *sql.DB, logger *slog.Logger) {
	// team_save_fact - Save a fact to shared memory
	executor.Register(
		MakeToolDefinition("team_save_fact",
			"Save a fact to the team's shared memory. All agents can access this.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Fact key/label",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Fact value/content",
					},
				},
				"required": []string{"team_id", "key", "value"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			key, _ := args["key"].(string)
			value, _ := args["value"].(string)

			if teamID == "" || key == "" || value == "" {
				return nil, fmt.Errorf("team_id, key, and value are required")
			}

			author := getCallerID(ctx)

			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.SaveFact(key, value, author); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Fact saved: **%s** = %s", key, truncateString(value, 100)), nil
		},
	)

	// team_get_facts - Get all shared facts
	executor.Register(
		MakeToolDefinition("team_get_facts",
			"Get all facts from the team's shared memory.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"search": map[string]any{
						"type":        "string",
						"description": "Search query (optional)",
					},
				},
				"required": []string{"team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			search, _ := args["search"].(string)

			if teamID == "" {
				return nil, fmt.Errorf("team_id is required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)

			var facts []*TeamFact
			var err error

			if search != "" {
				facts, err = teamMem.SearchFacts(search)
			} else {
				facts, err = teamMem.GetFacts()
			}

			if err != nil {
				return nil, err
			}

			if len(facts) == 0 {
				return "No facts found.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Shared Facts (%d):\n\n", len(facts)))
			for _, f := range facts {
				sb.WriteString(fmt.Sprintf("**%s**: %s\n", f.Key, f.Value))
				sb.WriteString(fmt.Sprintf("  (by %s, %s)\n\n", f.Author, f.UpdatedAt.Format("Jan 02")))
			}

			return sb.String(), nil
		},
	)

	// team_delete_fact - Delete a fact
	executor.Register(
		MakeToolDefinition("team_delete_fact",
			"Delete a fact from shared memory.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Fact key to delete",
					},
				},
				"required": []string{"team_id", "key"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			key, _ := args["key"].(string)

			if teamID == "" || key == "" {
				return nil, fmt.Errorf("team_id and key are required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.DeleteFact(key); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Fact '%s' deleted.", key), nil
		},
	)
}

// ── Standup Tool ──

func registerStandupTool(executor *ToolExecutor, teamMgr *TeamManager, db *sql.DB, logger *slog.Logger) {
	// team_standup - Generate daily standup
	executor.Register(
		MakeToolDefinition("team_standup",
			"Generate a daily standup summary showing completed, in-progress, and blocked tasks.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
				},
				"required": []string{"team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)

			if teamID == "" {
				return nil, fmt.Errorf("team_id is required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			standup, err := teamMem.GenerateStandup()
			if err != nil {
				return nil, err
			}

			return standup, nil
		},
	)
}

// ── Document Tools ──

func registerDocumentTools(executor *ToolExecutor, teamMgr *TeamManager, db *sql.DB, logger *slog.Logger) {
	// team_create_document - Create a document
	executor.Register(
		MakeToolDefinition("team_create_document",
			"Create a document (deliverable, research, protocol, or notes) linked to a task.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Document title",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Document content (markdown)",
					},
					"doc_type": map[string]any{
						"type":        "string",
						"description": "Document type: deliverable, research, protocol, notes",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Optional task ID to link document to",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Content format: markdown (default), code, json",
					},
				},
				"required": []string{"team_id", "title", "content"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			title, _ := args["title"].(string)
			content, _ := args["content"].(string)
			docType, _ := args["doc_type"].(string)
			taskID, _ := args["task_id"].(string)
			format, _ := args["format"].(string)

			if teamID == "" || title == "" || content == "" {
				return nil, fmt.Errorf("team_id, title, and content are required")
			}

			if docType == "" {
				docType = "deliverable"
			}
			if format == "" {
				format = "markdown"
			}

			caller := getCallerID(ctx)
			teamMem := NewTeamMemory(teamID, db, logger)
			teamMem.SetTeamManager(teamMgr)

			doc, err := teamMem.CreateDocument(title, DocumentType(docType), content, format, taskID, caller)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Document '%s' created with ID: %s", title, doc.ID), nil
		},
	)

	// team_list_documents - List documents
	executor.Register(
		MakeToolDefinition("team_list_documents",
			"List documents with optional filters by task or type.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Optional filter by task ID",
					},
					"doc_type": map[string]any{
						"type":        "string",
						"description": "Optional filter by document type",
					},
				},
				"required": []string{"team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			taskID, _ := args["task_id"].(string)
			docType, _ := args["doc_type"].(string)

			if teamID == "" {
				return nil, fmt.Errorf("team_id is required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			docs, err := teamMem.ListDocuments(taskID, DocumentType(docType))
			if err != nil {
				return nil, err
			}

			if len(docs) == 0 {
				return "No documents found.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d documents:\n\n", len(docs)))
			for _, doc := range docs {
				sb.WriteString(fmt.Sprintf("- [%s] %s (v%d, %s)\n", doc.DocType, doc.Title, doc.Version, doc.ID))
			}

			return sb.String(), nil
		},
	)

	// team_get_document - Get full document content
	executor.Register(
		MakeToolDefinition("team_get_document",
			"Get the full content of a document by ID.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"doc_id": map[string]any{
						"type":        "string",
						"description": "Document ID",
					},
				},
				"required": []string{"team_id", "doc_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			docID, _ := args["doc_id"].(string)

			if teamID == "" || docID == "" {
				return nil, fmt.Errorf("team_id and doc_id are required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			doc, err := teamMem.GetDocument(docID)
			if err != nil {
				return nil, err
			}
			if doc == nil {
				return nil, fmt.Errorf("document %s not found", docID)
			}

			return fmt.Sprintf("# %s (v%d)\nType: %s | Author: %s | Updated: %s\n\n%s",
				doc.Title, doc.Version, doc.DocType, doc.Author, doc.UpdatedAt.Format("2006-01-02"), doc.Content), nil
		},
	)

	// team_update_document - Update document content
	executor.Register(
		MakeToolDefinition("team_update_document",
			"Update document content. Version is automatically incremented.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"doc_id": map[string]any{
						"type":        "string",
						"description": "Document ID",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "New document content",
					},
				},
				"required": []string{"team_id", "doc_id", "content"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			docID, _ := args["doc_id"].(string)
			content, _ := args["content"].(string)

			if teamID == "" || docID == "" || content == "" {
				return nil, fmt.Errorf("team_id, doc_id, and content are required")
			}

			caller := getCallerID(ctx)
			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.UpdateDocument(docID, content, caller); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Document %s updated.", docID), nil
		},
	)

	// team_delete_document - Delete a document
	executor.Register(
		MakeToolDefinition("team_delete_document",
			"Delete a document by ID.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID",
					},
					"doc_id": map[string]any{
						"type":        "string",
						"description": "Document ID",
					},
				},
				"required": []string{"team_id", "doc_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			teamID, _ := args["team_id"].(string)
			docID, _ := args["doc_id"].(string)

			if teamID == "" || docID == "" {
				return nil, fmt.Errorf("team_id and doc_id are required")
			}

			teamMem := NewTeamMemory(teamID, db, logger)
			if err := teamMem.DeleteDocument(docID); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Document %s deleted.", docID), nil
		},
	)
}

// ── Working State Tools ──

func registerWorkingStateTools(executor *ToolExecutor, teamMgr *TeamManager, logger *slog.Logger) {
	// team_get_working - Get agent's current working state
	executor.Register(
		MakeToolDefinition("team_get_working",
			"Get your current working state (WORKING.md). Shows current task, status, and next steps.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID (optional, defaults to calling agent)",
					},
				},
				"required": []string{},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			agentID, _ := args["agent_id"].(string)
			if agentID == "" {
				agentID = getAgentIDFromContext(ctx)
			}

			if agentID == "" {
				return "No working state found (no agent ID in context).", nil
			}

			state, err := teamMgr.GetAgentWorkingState(agentID)
			if err != nil {
				return nil, err
			}
			if state == nil {
				return "No active working state. You are currently idle.", nil
			}

			return fmt.Sprintf(`# WORKING.md

## Status: %s
## Current Task: %s
## Next Steps:
%s
## Context:
%s
## Last Updated: %s
`, state.Status, state.CurrentTaskID, state.NextSteps, state.Context, state.UpdatedAt.Format("2006-01-02 15:04")), nil
		},
	)

	// team_update_working - Update agent's working state
	executor.Register(
		MakeToolDefinition("team_update_working",
			"Update your working state (WORKING.md). Use this to save progress and next steps.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Work status: idle, working, blocked, waiting",
					},
					"current_task_id": map[string]any{
						"type":        "string",
						"description": "Current task ID",
					},
					"next_steps": map[string]any{
						"type":        "string",
						"description": "What you plan to do next (markdown)",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Additional context for resuming work",
					},
				},
				"required": []string{"status"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			agentID := getAgentIDFromContext(ctx)
			if agentID == "" {
				return nil, fmt.Errorf("no agent ID in context")
			}

			agent, err := teamMgr.GetAgent(agentID)
			if err != nil || agent == nil {
				return nil, fmt.Errorf("agent %s not found", agentID)
			}

			status, _ := args["status"].(string)
			currentTaskID, _ := args["current_task_id"].(string)
			nextSteps, _ := args["next_steps"].(string)
			context, _ := args["context"].(string)

			state := &AgentWorkingState{
				AgentID:       agentID,
				TeamID:        agent.TeamID,
				CurrentTaskID: currentTaskID,
				Status:        status,
				NextSteps:     nextSteps,
				Context:       context,
				UpdatedAt:     time.Now(),
			}

			if err := teamMgr.SaveAgentWorkingState(state); err != nil {
				return nil, err
			}

			return fmt.Sprintf("Working state updated. Status: %s", status), nil
		},
	)

	// team_clear_working - Clear working state (task done)
	executor.Register(
		MakeToolDefinition("team_clear_working",
			"Clear your working state. Use this when a task is completed.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID (optional, defaults to calling agent)",
					},
				},
				"required": []string{},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			agentID, _ := args["agent_id"].(string)
			if agentID == "" {
				agentID = getAgentIDFromContext(ctx)
			}

			if agentID == "" {
				return nil, fmt.Errorf("no agent ID in context")
			}

			if err := teamMgr.ClearAgentWorkingState(agentID); err != nil {
				return nil, err
			}

			return "Working state cleared. You are now idle.", nil
		},
	)
}

// ── Helpers ──

func getCallerID(ctx context.Context) string {
	// Try caller JID first
	caller := CallerJIDFromContext(ctx)
	if caller != "" {
		return caller
	}

	// Try agent ID from session
	agentID := getAgentIDFromContext(ctx)
	if agentID != "" {
		return agentID
	}

	return "system"
}

func getAgentIDFromContext(ctx context.Context) string {
	sessionID := SessionIDFromContext(ctx)
	if sessionID == "" {
		return ""
	}

	// Session ID format: "agent:<agent_id>:..."
	if strings.HasPrefix(sessionID, "agent:") {
		parts := strings.Split(sessionID, ":")
		if len(parts) >= 2 {
			return parts[1]
		}
	}

	// Also check for "subagent:" prefix
	if strings.HasPrefix(sessionID, "subagent:") {
		// Subagent runs don't have a persistent agent ID
		return ""
	}

	return ""
}
