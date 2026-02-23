// Package copilot – team_tools_dispatcher.go implements dispatcher tools for team/agent management.
// Uses a dispatcher pattern to reduce tool count from 34 to 5.
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

// RegisterTeamTools registers all team management tools using dispatcher pattern.
// Reduces tool count from 34 individual tools to 5 dispatcher tools.
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

	// Create handler context with shared dependencies
	hctx := &handlerContext{
		teamMgr: teamMgr,
		db:      db,
		sched:   sched,
		logger:  logger,
	}

	// Register 5 dispatcher tools
	registerTeamManageDispatcher(executor, hctx)
	registerTeamAgentDispatcher(executor, hctx)
	registerTeamTaskDispatcher(executor, hctx)
	registerTeamMemoryDispatcher(executor, hctx)
	registerTeamCommDispatcher(executor, hctx)

	logger.Info("team dispatcher tools registered (5 tools)")
}

// handlerContext holds shared dependencies for internal handlers
type handlerContext struct {
	teamMgr *TeamManager
	db      *sql.DB
	sched   *scheduler.Scheduler
	logger  *slog.Logger
}

// resolveTeamID resolves a team reference (ID, name, or empty for default)
// and returns the team ID. Returns error if team cannot be resolved.
func (h *handlerContext) resolveTeamID(teamRef string) (string, error) {
	team, err := h.teamMgr.ResolveTeam(teamRef)
	if err != nil {
		return "", err
	}
	return team.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM MANAGE DISPATCHER
// ═══════════════════════════════════════════════════════════════════════════════

func registerTeamManageDispatcher(executor *ToolExecutor, hctx *handlerContext) {
	executor.Register(
		MakeToolDefinition("team_manage",
			"Manage teams with actions: create, list, get, update, delete. Create teams to organize agents with shared memory and tasks.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"create", "list", "get", "update", "delete"},
						"description": "Action to perform",
					},
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (required for get/update/delete)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Team name (for create/update)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Team description (for create/update)",
					},
					"default_model": map[string]any{
						"type":        "string",
						"description": "Default LLM model for agents (optional)",
					},
				},
				"required": []string{"action"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "create":
				return hctx.handleTeamCreate(ctx, args)
			case "list":
				return hctx.handleTeamList(args)
			case "get":
				return hctx.handleTeamGet(args)
			case "update":
				return hctx.handleTeamUpdate(ctx, args)
			case "delete":
				return hctx.handleTeamDelete(args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func (h *handlerContext) handleTeamCreate(ctx context.Context, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	description, _ := args["description"].(string)
	defaultModel, _ := args["default_model"].(string)

	callerJID := CallerJIDFromContext(ctx)
	if callerJID == "" {
		callerJID = "system"
	}

	team, err := h.teamMgr.CreateTeam(name, description, callerJID, defaultModel)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Team created successfully!\n  ID: %s\n  Name: %s\n  Owner: %s",
		team.ID, team.Name, team.OwnerJID), nil
}

func (h *handlerContext) handleTeamList(args map[string]any) (any, error) {
	teams, err := h.teamMgr.ListTeams()
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return "No teams found. Create one with team_manage(action='create').", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d teams:\n", len(teams)))
	for _, t := range teams {
		status := "enabled"
		if !t.Enabled {
			status = "disabled"
		}
		sb.WriteString(fmt.Sprintf("  • %s (%s) - %s [%s]\n", t.Name, t.ID, t.Description, status))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleTeamGet(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)

	team, err := h.teamMgr.ResolveTeam(teamRef)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Team: %s\n  ID: %s\n  Description: %s\n  Owner: %s\n  Model: %s\n  Enabled: %v",
		team.Name, team.ID, team.Description, team.OwnerJID, team.DefaultModel, team.Enabled), nil
}

func (h *handlerContext) handleTeamUpdate(ctx context.Context, args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)

	// Get existing team
	team, err := h.teamMgr.ResolveTeam(teamRef)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if name, ok := args["name"].(string); ok && name != "" {
		team.Name = name
	}
	if description, ok := args["description"].(string); ok {
		team.Description = description
	}
	if defaultModel, ok := args["default_model"].(string); ok {
		team.DefaultModel = defaultModel
	}

	err = h.teamMgr.UpdateTeam(team)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Team %s updated successfully", team.ID), nil
}

func (h *handlerContext) handleTeamDelete(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)

	team, err := h.teamMgr.ResolveTeam(teamRef)
	if err != nil {
		return nil, err
	}

	err = h.teamMgr.DeleteTeam(team.ID)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Team %s deleted successfully", team.ID), nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM AGENT DISPATCHER
// ═══════════════════════════════════════════════════════════════════════════════

func registerTeamAgentDispatcher(executor *ToolExecutor, hctx *handlerContext) {
	executor.Register(
		MakeToolDefinition("team_agent",
			"Manage team agents with actions: create, list, get, update, start, stop, delete. Also manage working state: working_get, working_update, working_clear.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"create", "list", "get", "update", "start", "stop", "delete", "working_get", "working_update", "working_clear"},
						"description": "Action to perform",
					},
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (required for most actions)",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID (for get/update/start/stop/delete/working actions)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Agent name (for create/update)",
					},
					"role": map[string]any{
						"type":        "string",
						"description": "Agent role description (for create/update)",
					},
					"level": map[string]any{
						"type":        "string",
						"enum":        []string{"junior", "mid", "senior", "lead"},
						"description": "Agent level (for create/update)",
					},
					"personality": map[string]any{
						"type":        "string",
						"description": "Agent personality traits (optional)",
					},
					"instructions": map[string]any{
						"type":        "string",
						"description": "Specific instructions for the agent (optional)",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "LLM model override (optional)",
					},
					"skills": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of skills the agent has (optional)",
					},
					"heartbeat_schedule": map[string]any{
						"type":        "string",
						"description": "Cron expression for heartbeat (e.g., '*/15 * * * *')",
					},
					"current_task_id": map[string]any{
						"type":        "string",
						"description": "Current task ID (for working_update)",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Working status (for working_update)",
					},
					"next_steps": map[string]any{
						"type":        "string",
						"description": "Next steps description (for working_update)",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Working context (for working_update)",
					},
				},
				"required": []string{"action"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "create":
				return hctx.handleAgentCreate(args)
			case "list":
				return hctx.handleAgentList(args)
			case "get":
				return hctx.handleAgentGet(args)
			case "update":
				return hctx.handleAgentUpdate(args)
			case "start":
				return hctx.handleAgentStart(args)
			case "stop":
				return hctx.handleAgentStop(args)
			case "delete":
				return hctx.handleAgentDelete(args)
			case "working_get":
				return hctx.handleAgentWorkingGet(args)
			case "working_update":
				return hctx.handleAgentWorkingUpdate(args)
			case "working_clear":
				return hctx.handleAgentWorkingClear(args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func (h *handlerContext) handleAgentCreate(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	role, _ := args["role"].(string)
	if role == "" {
		role = "Team Member"
	}
	levelStr, _ := args["level"].(string)
	if levelStr == "" {
		levelStr = "mid"
	}
	level := AgentLevelSpecialist
	switch levelStr {
	case "junior", "intern":
		level = AgentLevelIntern
	case "senior":
		level = AgentLevelSpecialist
	case "lead":
		level = AgentLevelLead
	}
	personality, _ := args["personality"].(string)
	instructions, _ := args["instructions"].(string)
	model, _ := args["model"].(string)
	heartbeat, _ := args["heartbeat_schedule"].(string)
	if heartbeat == "" {
		heartbeat = "*/15 * * * *"
	}
	skills := getStringSlice(args, "skills")

	agent, err := h.teamMgr.CreateAgent(teamID, name, role, personality, instructions, model, skills, level, heartbeat)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Agent created successfully!\n  ID: %s\n  Name: %s\n  Role: %s\n  Level: %s\n  Heartbeat: %s",
		agent.ID, agent.Name, agent.Role, agent.Level, agent.HeartbeatSchedule), nil
}

func (h *handlerContext) handleAgentList(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	agents, err := h.teamMgr.ListAgents(teamID)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return "No agents in this team. Create one with team_agent(action='create').", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d agents in team %s:\n", len(agents), teamID))
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("  • %s (%s) - %s [%s]\n", a.Name, a.ID, a.Role, a.Status))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleAgentGet(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	agent, err := h.teamMgr.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent: %s\n", agent.Name))
	sb.WriteString(fmt.Sprintf("  ID: %s\n", agent.ID))
	sb.WriteString(fmt.Sprintf("  Role: %s\n", agent.Role))
	sb.WriteString(fmt.Sprintf("  Level: %s\n", agent.Level))
	sb.WriteString(fmt.Sprintf("  Status: %s\n", agent.Status))
	sb.WriteString(fmt.Sprintf("  Team: %s\n", agent.TeamID))
	sb.WriteString(fmt.Sprintf("  Model: %s\n", agent.Model))
	sb.WriteString(fmt.Sprintf("  Heartbeat: %s\n", agent.HeartbeatSchedule))
	sb.WriteString(fmt.Sprintf("  Skills: %s\n", strings.Join(agent.Skills, ", ")))
	if agent.Personality != "" {
		sb.WriteString(fmt.Sprintf("  Personality: %s\n", agent.Personality))
	}
	if agent.Instructions != "" {
		sb.WriteString(fmt.Sprintf("  Instructions:\n    %s\n", strings.ReplaceAll(agent.Instructions, "\n", "\n    ")))
	}

	return sb.String(), nil
}

func (h *handlerContext) handleAgentUpdate(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Get existing agent
	agent, err := h.teamMgr.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	// Update fields if provided
	if name, ok := args["name"].(string); ok && name != "" {
		agent.Name = name
	}
	if role, ok := args["role"].(string); ok && role != "" {
		agent.Role = role
	}
	if levelStr, ok := args["level"].(string); ok && levelStr != "" {
		switch levelStr {
		case "junior", "intern":
			agent.Level = AgentLevelIntern
		case "mid", "specialist":
			agent.Level = AgentLevelSpecialist
		case "senior":
			agent.Level = AgentLevelSpecialist
		case "lead":
			agent.Level = AgentLevelLead
		}
	}
	if personality, ok := args["personality"].(string); ok {
		agent.Personality = personality
	}
	if instructions, ok := args["instructions"].(string); ok {
		agent.Instructions = instructions
	}
	if model, ok := args["model"].(string); ok {
		agent.Model = model
	}
	if heartbeat, ok := args["heartbeat_schedule"].(string); ok && heartbeat != "" {
		agent.HeartbeatSchedule = heartbeat
	}

	err = h.teamMgr.UpdateAgent(agent)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Agent %s updated successfully", agentID), nil
}

func (h *handlerContext) handleAgentStart(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	err := h.teamMgr.StartAgent(agentID)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Agent %s started successfully", agentID), nil
}

func (h *handlerContext) handleAgentStop(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	err := h.teamMgr.StopAgent(agentID)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Agent %s stopped successfully", agentID), nil
}

func (h *handlerContext) handleAgentDelete(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	err := h.teamMgr.DeleteAgent(agentID)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Agent %s deleted successfully", agentID), nil
}

func (h *handlerContext) handleAgentWorkingGet(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	state, err := h.teamMgr.GetAgentWorkingState(agentID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return "No working state found. Agent is idle.", nil
	}

	return fmt.Sprintf("Working State for %s:\n  Task: %s\n  Status: %s\n  Next Steps: %s\n  Context: %s",
		agentID, state.CurrentTaskID, state.Status, state.NextSteps, state.Context), nil
}

func (h *handlerContext) handleAgentWorkingUpdate(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Get the agent to obtain team_id
	agent, err := h.teamMgr.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	currentTaskID, _ := args["current_task_id"].(string)
	status, _ := args["status"].(string)
	nextSteps, _ := args["next_steps"].(string)
	contextStr, _ := args["context"].(string)

	state := &AgentWorkingState{
		AgentID:       agentID,
		TeamID:        agent.TeamID,
		CurrentTaskID: currentTaskID,
		Status:        status,
		NextSteps:     nextSteps,
		Context:       contextStr,
		UpdatedAt:     time.Now(),
	}

	if err := h.teamMgr.SaveAgentWorkingState(state); err != nil {
		return nil, err
	}

	return fmt.Sprintf("Working state updated for agent %s", agentID), nil
}

func (h *handlerContext) handleAgentWorkingClear(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	if err := h.teamMgr.ClearAgentWorkingState(agentID); err != nil {
		return nil, err
	}

	return "Working state cleared. You are now idle.", nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM TASK DISPATCHER
// ═══════════════════════════════════════════════════════════════════════════════

func registerTeamTaskDispatcher(executor *ToolExecutor, hctx *handlerContext) {
	executor.Register(
		MakeToolDefinition("team_task",
			"Manage team tasks with actions: create, list, get, update, assign, delete. Tasks track work items for team agents.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"create", "list", "get", "update", "assign", "delete"},
						"description": "Action to perform",
					},
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (required)",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Task ID (for get/update/assign/delete)",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Task title (for create)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Task description (for create)",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"inbox", "assigned", "in_progress", "review", "done", "blocked", "cancelled"},
						"description": "Task status (for update)",
					},
					"assignees": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Agent IDs to assign (for create/assign)",
					},
					"status_filter": map[string]any{
						"type":        "string",
						"description": "Filter by status (for list)",
					},
					"assignee_filter": map[string]any{
						"type":        "string",
						"description": "Filter by assignee (for list)",
					},
					"comment": map[string]any{
						"type":        "string",
						"description": "Comment to add when updating status (for update)",
					},
				},
				"required": []string{"action", "team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "create":
				return hctx.handleTaskCreate(ctx, args)
			case "list":
				return hctx.handleTaskList(args)
			case "get":
				return hctx.handleTaskGet(args)
			case "update":
				return hctx.handleTaskUpdate(ctx, args)
			case "assign":
				return hctx.handleTaskAssign(ctx, args)
			case "delete":
				return hctx.handleTaskDelete(args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func (h *handlerContext) handleTaskCreate(ctx context.Context, args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	description, _ := args["description"].(string)
	assignees := getStringSlice(args, "assignees")

	callerID := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	task, err := teamMem.CreateTask(title, description, callerID, assignees)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return fmt.Sprintf("Task created successfully!\n  ID: %s\n  Title: %s\n  Status: %s",
		task.ID, task.Title, task.Status), nil
}

func (h *handlerContext) handleTaskList(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	statusFilter, _ := args["status_filter"].(string)
	assigneeFilter, _ := args["assignee_filter"].(string)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	tasks, err := teamMem.ListTasks(TaskStatus(statusFilter), assigneeFilter)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No tasks found. Create one with team_task(action='create').", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d tasks:\n", len(tasks)))
	for _, t := range tasks {
		assignees := strings.Join(t.Assignees, ", ")
		if assignees == "" {
			assignees = "unassigned"
		}
		sb.WriteString(fmt.Sprintf("  • [%s] %s - %s (assigned: %s)\n", t.Status, t.ID, t.Title, assignees))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleTaskGet(args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	taskID, _ := args["task_id"].(string)
	if teamID == "" || taskID == "" {
		return nil, fmt.Errorf("team_id and task_id are required")
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	task, err := teamMem.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("  ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("  Status: %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("  Priority: %d\n", task.Priority))
	sb.WriteString(fmt.Sprintf("  Assignees: %s\n", strings.Join(task.Assignees, ", ")))
	sb.WriteString(fmt.Sprintf("  Labels: %s\n", strings.Join(task.Labels, ", ")))
	sb.WriteString(fmt.Sprintf("  Description: %s\n", task.Description))
	if task.BlockedReason != "" {
		sb.WriteString(fmt.Sprintf("  Blocked Reason: %s\n", task.BlockedReason))
	}

	return sb.String(), nil
}

func (h *handlerContext) handleTaskUpdate(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	taskID, _ := args["task_id"].(string)
	if teamID == "" || taskID == "" {
		return nil, fmt.Errorf("team_id and task_id are required")
	}

	status, _ := args["status"].(string)
	if status == "" {
		return nil, fmt.Errorf("status is required for update")
	}
	comment, _ := args["comment"].(string)
	callerID := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	err := teamMem.UpdateTask(taskID, TaskStatus(status), comment, callerID)
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}

	return fmt.Sprintf("Task %s updated to status %s", taskID, status), nil
}

func (h *handlerContext) handleTaskAssign(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	taskID, _ := args["task_id"].(string)
	if teamID == "" || taskID == "" {
		return nil, fmt.Errorf("team_id and task_id are required")
	}

	assignees := getStringSlice(args, "assignees")
	callerID := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	err := teamMem.AssignTask(taskID, assignees, callerID)
	if err != nil {
		return nil, fmt.Errorf("assign task: %w", err)
	}

	return fmt.Sprintf("Task %s assigned to: %s", taskID, strings.Join(assignees, ", ")), nil
}

func (h *handlerContext) handleTaskDelete(args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	taskID, _ := args["task_id"].(string)
	if teamID == "" || taskID == "" {
		return nil, fmt.Errorf("team_id and task_id are required")
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	if err := teamMem.DeleteTask(taskID); err != nil {
		return nil, fmt.Errorf("delete task: %w", err)
	}

	return fmt.Sprintf("Task %s deleted successfully", taskID), nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM MEMORY DISPATCHER
// ═══════════════════════════════════════════════════════════════════════════════

func registerTeamMemoryDispatcher(executor *ToolExecutor, hctx *handlerContext) {
	executor.Register(
		MakeToolDefinition("team_memory",
			"Manage team memory with actions: fact_save, fact_list, fact_delete, doc_create, doc_list, doc_get, doc_update, doc_delete, standup. Shared memory for team knowledge.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"fact_save", "fact_list", "fact_delete", "doc_create", "doc_list", "doc_get", "doc_update", "doc_delete", "standup"},
						"description": "Action to perform",
					},
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (required)",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Fact key (for fact_save/fact_delete)",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Fact value (for fact_save)",
					},
					"doc_id": map[string]any{
						"type":        "string",
						"description": "Document ID (for doc_get/doc_update/doc_delete)",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Document title (for doc_create/doc_update)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Document content (for doc_create/doc_update)",
					},
					"doc_type": map[string]any{
						"type":        "string",
						"enum":        []string{"deliverable", "research", "protocol", "notes"},
						"description": "Document type (for doc_create)",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Related task ID (for doc_create/doc_list)",
					},
					"format": map[string]any{
						"type":        "string",
						"enum":        []string{"markdown", "text", "html"},
						"description": "Document format (default: markdown)",
					},
				},
				"required": []string{"action", "team_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "fact_save":
				return hctx.handleFactSave(ctx, args)
			case "fact_list":
				return hctx.handleFactList(args)
			case "fact_delete":
				return hctx.handleFactDelete(args)
			case "doc_create":
				return hctx.handleDocCreate(ctx, args)
			case "doc_list":
				return hctx.handleDocList(args)
			case "doc_get":
				return hctx.handleDocGet(args)
			case "doc_update":
				return hctx.handleDocUpdate(ctx, args)
			case "doc_delete":
				return hctx.handleDocDelete(args)
			case "standup":
				return hctx.handleStandup(args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func (h *handlerContext) handleFactSave(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)

	if teamID == "" || key == "" || value == "" {
		return nil, fmt.Errorf("team_id, key, and value are required")
	}

	author := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	if err := teamMem.SaveFact(key, value, author); err != nil {
		return nil, fmt.Errorf("save fact: %w", err)
	}

	return fmt.Sprintf("Fact saved: %s = %s", key, value), nil
}

func (h *handlerContext) handleFactList(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	facts, err := teamMem.GetFacts()
	if err != nil {
		return nil, fmt.Errorf("get facts: %w", err)
	}

	if len(facts) == 0 {
		return "No facts stored. Use team_memory(action='fact_save') to add one.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Team Facts (%d):\n", len(facts)))
	for _, f := range facts {
		sb.WriteString(fmt.Sprintf("  • %s: %s\n", f.Key, f.Value))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleFactDelete(args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	key, _ := args["key"].(string)

	if teamID == "" || key == "" {
		return nil, fmt.Errorf("team_id and key are required")
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	if err := teamMem.DeleteFact(key); err != nil {
		return nil, fmt.Errorf("delete fact: %w", err)
	}

	return fmt.Sprintf("Fact '%s' deleted", key), nil
}

func (h *handlerContext) handleDocCreate(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	title, _ := args["title"].(string)
	content, _ := args["content"].(string)

	if teamID == "" || title == "" || content == "" {
		return nil, fmt.Errorf("team_id, title, and content are required")
	}

	// Validate doc_type
	validDocTypes := map[string]bool{"deliverable": true, "research": true, "protocol": true, "notes": true}
	docType, _ := args["doc_type"].(string)
	if docType == "" {
		docType = "deliverable"
	} else if !validDocTypes[docType] {
		return nil, fmt.Errorf("invalid doc_type: %s (valid: deliverable, research, protocol, notes)", docType)
	}

	taskID, _ := args["task_id"].(string)

	// Validate format
	validFormats := map[string]bool{"markdown": true, "text": true, "html": true, "code": true, "json": true}
	format, _ := args["format"].(string)
	if format == "" {
		format = "markdown"
	} else if !validFormats[format] {
		return nil, fmt.Errorf("invalid format: %s (valid: markdown, text, html, code, json)", format)
	}
	author := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	doc, err := teamMem.CreateDocument(title, DocumentType(docType), content, format, taskID, author)
	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	return fmt.Sprintf("Document created!\n  ID: %s\n  Title: %s\n  Type: %s",
		doc.ID, doc.Title, doc.DocType), nil
}

func (h *handlerContext) handleDocList(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	docType, _ := args["doc_type"].(string)
	taskID, _ := args["task_id"].(string)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	docs, err := teamMem.ListDocuments(taskID, DocumentType(docType))
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}

	if len(docs) == 0 {
		return "No documents found. Use team_memory(action='doc_create') to add one.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Documents (%d):\n", len(docs)))
	for _, d := range docs {
		sb.WriteString(fmt.Sprintf("  • [%s] %s - %s\n", d.DocType, d.ID, d.Title))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleDocGet(args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	docID, _ := args["doc_id"].(string)

	if teamID == "" || docID == "" {
		return nil, fmt.Errorf("team_id and doc_id are required")
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	doc, err := teamMem.GetDocument(docID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", docID)
	}

	return fmt.Sprintf("Document: %s\nType: %s\nAuthor: %s\nVersion: %d\n\n%s",
		doc.Title, doc.DocType, doc.Author, doc.Version, doc.Content), nil
}

func (h *handlerContext) handleDocUpdate(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	docID, _ := args["doc_id"].(string)

	if teamID == "" || docID == "" {
		return nil, fmt.Errorf("team_id and doc_id are required")
	}

	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required for update")
	}
	author := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	if err := teamMem.UpdateDocument(docID, content, author); err != nil {
		return nil, fmt.Errorf("update document: %w", err)
	}

	return fmt.Sprintf("Document %s updated successfully", docID), nil
}

func (h *handlerContext) handleDocDelete(args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	docID, _ := args["doc_id"].(string)

	if teamID == "" || docID == "" {
		return nil, fmt.Errorf("team_id and doc_id are required")
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	if err := teamMem.DeleteDocument(docID); err != nil {
		return nil, fmt.Errorf("delete document: %w", err)
	}

	return fmt.Sprintf("Document %s deleted successfully", docID), nil
}

func (h *handlerContext) handleStandup(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	standup, err := teamMem.GenerateStandup()
	if err != nil {
		return nil, fmt.Errorf("generate standup: %w", err)
	}

	return standup, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM COMM DISPATCHER
// ═══════════════════════════════════════════════════════════════════════════════

func registerTeamCommDispatcher(executor *ToolExecutor, hctx *handlerContext) {
	executor.Register(
		MakeToolDefinition("team_comm",
			"Team communication with actions: comment, mention_check, send_message, notify, notify_list. Interact with other agents and receive notifications.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"comment", "mention_check", "send_message", "notify", "notify_list"},
						"description": "Action to perform",
					},
					"team_id": map[string]any{
						"type":        "string",
						"description": "Team ID (required)",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Task ID (for comment)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Message content (for comment/send_message)",
					},
					"to_agent": map[string]any{
						"type":        "string",
						"description": "Target agent ID (for send_message)",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID (for mention_check)",
					},
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"task_completed", "task_failed", "task_blocked", "task_progress", "agent_error"},
						"description": "Notification type (for notify)",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Notification message (for notify)",
					},
					"priority": map[string]any{
						"type":        "integer",
						"description": "Priority 1-5 (for notify, default 3)",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max notifications to return (for notify_list, default 20)",
					},
					"unread_only": map[string]any{
						"type":        "boolean",
						"description": "Only return unread notifications (for notify_list)",
					},
				},
				"required": []string{"action"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "comment":
				return hctx.handleComment(ctx, args)
			case "mention_check":
				return hctx.handleMentionCheck(args)
			case "send_message":
				return hctx.handleSendMessage(ctx, args)
			case "notify":
				return hctx.handleNotify(ctx, args)
			case "notify_list":
				return hctx.handleNotifyList(args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

func (h *handlerContext) handleComment(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	taskID, _ := args["task_id"].(string)
	content, _ := args["content"].(string)

	if teamID == "" || taskID == "" || content == "" {
		return nil, fmt.Errorf("team_id, task_id, and content are required")
	}

	callerID := getCallerID(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	_, err := teamMem.PostMessage(taskID, callerID, content, nil)
	if err != nil {
		return nil, fmt.Errorf("add comment: %w", err)
	}

	return fmt.Sprintf("Comment added to task %s", taskID), nil
}

func (h *handlerContext) handleMentionCheck(args map[string]any) (any, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Get pending messages for this agent
	rows, err := h.db.Query(`
		SELECT id, from_agent, from_user, content, thread_id, created_at
		FROM team_pending_messages
		WHERE to_agent = ? AND delivered = 0
		ORDER BY created_at ASC LIMIT 20`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query mentions: %w", err)
	}
	defer rows.Close()

	var messages []map[string]any
	var ids []string
	for rows.Next() {
		var id, fromAgent, fromUser, content, threadID, createdAt string
		if err := rows.Scan(&id, &fromAgent, &fromUser, &content, &threadID, &createdAt); err != nil {
			continue
		}
		messages = append(messages, map[string]any{
			"id":         id,
			"from_agent": fromAgent,
			"from_user":  fromUser,
			"content":    content,
			"thread_id":  threadID,
			"created_at": createdAt,
		})
		ids = append(ids, id)
	}

	if len(messages) == 0 {
		return "No pending mentions or messages.", nil
	}

	// Mark as delivered
	now := time.Now().Format(time.RFC3339)
	for _, id := range ids {
		h.db.Exec(`UPDATE team_pending_messages SET delivered = 1, delivered_at = ? WHERE id = ?`, now, id)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You have %d pending messages:\n", len(messages)))
	for _, m := range messages {
		from := m["from_agent"].(string)
		if from == "" {
			from = m["from_user"].(string)
		}
		sb.WriteString(fmt.Sprintf("  • From %s: %s\n", from, m["content"]))
	}
	return sb.String(), nil
}

func (h *handlerContext) handleSendMessage(ctx context.Context, args map[string]any) (any, error) {
	teamID, _ := args["team_id"].(string)
	toAgent, _ := args["to_agent"].(string)
	content, _ := args["content"].(string)

	if teamID == "" || toAgent == "" || content == "" {
		return nil, fmt.Errorf("team_id, to_agent, and content are required")
	}

	fromAgent := getAgentIDFromContext(ctx)

	teamMem := h.teamMgr.GetTeamMemory(teamID)

	// Post a direct message using the team memory
	_, err := teamMem.PostMessage(toAgent, fromAgent, content, nil)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	return fmt.Sprintf("Message sent to %s", toAgent), nil
}

func (h *handlerContext) handleNotify(ctx context.Context, args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	// Validate notification type
	validNotifTypes := map[string]bool{
		"task_completed": true, "task_failed": true, "task_blocked": true,
		"task_progress": true, "agent_error": true,
	}
	notifType, _ := args["type"].(string)
	if notifType == "" {
		return nil, fmt.Errorf("type is required")
	} else if !validNotifTypes[notifType] {
		return nil, fmt.Errorf("invalid type: %s (valid: task_completed, task_failed, task_blocked, task_progress, agent_error)", notifType)
	}

	message, _ := args["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	taskID, _ := args["task_id"].(string)

	// Validate priority (1-5 range)
	priority, _ := args["priority"].(float64)
	if priority == 0 {
		priority = 3
	} else if priority < 1 || priority > 5 {
		priority = 3 // Default to normal for invalid values
	}

	agentID := getAgentIDFromContext(ctx)
	agentName := agentID
	if agentName == "" {
		agentName = "unknown"
	}

	// Get task title if task_id provided
	var taskTitle string
	if taskID != "" && h.db != nil {
		_ = h.db.QueryRow(`
			SELECT title FROM team_tasks WHERE id = ? AND team_id = ?`,
			taskID, teamID,
		).Scan(&taskTitle)
	}

	// Get notification dispatcher from team manager
	if h.teamMgr.notifDisp == nil {
		return "Notification sent (no dispatcher configured - notification logged only)", nil
	}

	notif := &TeamNotification{
		ID:        fmt.Sprintf("n%d", time.Now().UnixNano()%1000000),
		TeamID:    teamID,
		Type:      NotificationType(notifType),
		AgentID:   agentID,
		AgentName: agentName,
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Action:    notifType,
		Message:   message,
		Timestamp: time.Now(),
		Priority:  int(priority),
	}

	// Set result based on type
	switch notifType {
	case "task_completed":
		notif.Result = ResultSuccess
	case "task_failed", "task_blocked", "agent_error":
		notif.Result = ResultFailure
	default:
		notif.Result = ResultInfo
	}

	if err := h.teamMgr.notifDisp.Dispatch(ctx, notif); err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	return fmt.Sprintf("Notification sent: [%s] %s", notifType, message), nil
}

func (h *handlerContext) handleNotifyList(args map[string]any) (any, error) {
	teamRef, _ := args["team_id"].(string)
	teamID, err := h.resolveTeamID(teamRef)
	if err != nil {
		return nil, err
	}

	// Validate and cap limit
	maxLimit := 100
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	unreadOnly, _ := args["unread_only"].(bool)

	if h.teamMgr.notifDisp == nil {
		return []map[string]any{}, nil
	}

	var notifications []*TeamNotification

	if unreadOnly {
		notifications, err = h.teamMgr.notifDisp.GetUnreadNotifications(context.Background(), teamID)
	} else {
		notifications, err = h.teamMgr.notifDisp.GetNotifications(context.Background(), teamID, int(limit))
	}

	if err != nil {
		return nil, fmt.Errorf("get notifications: %w", err)
	}

	result := make([]map[string]any, len(notifications))
	for i, n := range notifications {
		result[i] = map[string]any{
			"id":         n.ID,
			"type":       n.Type,
			"agent_name": n.AgentName,
			"task_title": n.TaskTitle,
			"action":     n.Action,
			"result":     n.Result,
			"message":    n.Message,
			"priority":   n.Priority,
			"timestamp":  n.Timestamp.Format(time.RFC3339),
			"read":       n.Read,
		}
	}

	return result, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════════

func getCallerID(ctx context.Context) string {
	caller := CallerJIDFromContext(ctx)
	if caller != "" {
		return caller
	}
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

	if strings.HasPrefix(sessionID, "agent:") {
		parts := strings.Split(sessionID, ":")
		if len(parts) >= 2 {
			return parts[1]
		}
	}

	if strings.HasPrefix(sessionID, "subagent:") {
		return ""
	}

	return ""
}

func getStringSlice(args map[string]any, key string) []string {
	val, ok := args[key]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				result[i] = s
			}
		}
		return result
	}
	return nil
}

