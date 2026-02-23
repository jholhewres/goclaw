// Package copilot – team_manager.go manages persistent agents and teams.
// Provides lifecycle management, heartbeats, and integration with the scheduler.
package copilot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jholhewres/devclaw/pkg/devclaw/scheduler"
)

// TeamManager manages teams and persistent agents.
type TeamManager struct {
	db        *sql.DB
	scheduler *scheduler.Scheduler
	logger    *slog.Logger
	notifDisp *NotificationDispatcher

	// In-memory cache of active persistent agents
	agents map[string]*PersistentAgent
	mu     sync.RWMutex

	// Callbacks for spawning agents
	spawnAgent func(ctx context.Context, agent *PersistentAgent, task string) error
}

// NewTeamManager creates a new team manager.
func NewTeamManager(db *sql.DB, sched *scheduler.Scheduler, logger *slog.Logger) *TeamManager {
	if logger == nil {
		logger = slog.Default()
	}
	tm := &TeamManager{
		db:        db,
		scheduler: sched,
		logger:    logger.With("component", "team-manager"),
		agents:    make(map[string]*PersistentAgent),
	}
	tm.loadAgentsFromDB()
	return tm
}

// SetSpawnAgentCallback sets the callback for spawning agent runs.
func (tm *TeamManager) SetSpawnAgentCallback(fn func(ctx context.Context, agent *PersistentAgent, task string) error) {
	tm.spawnAgent = fn
}

// SetNotificationDispatcher sets the notification dispatcher for team notifications.
func (tm *TeamManager) SetNotificationDispatcher(nd *NotificationDispatcher) {
	tm.notifDisp = nd
}

// NotifyTaskCompletion sends a notification about task completion.
func (tm *TeamManager) NotifyTaskCompletion(ctx context.Context, teamID, agentID, agentName, taskID, taskTitle, message string, success bool) {
	if tm.notifDisp == nil {
		return
	}
	if err := tm.notifDisp.DispatchTaskCompletion(ctx, teamID, agentID, agentName, taskID, taskTitle, message, success); err != nil {
		tm.logger.Warn("failed to dispatch task completion notification", "error", err)
	}
}

// ─── Teams ───

// CreateTeam creates a new team.
func (tm *TeamManager) CreateTeam(name, description, ownerJID, defaultModel string) (*Team, error) {
	team := &Team{
		ID:           uuid.New().String()[:8],
		Name:         name,
		Description:  description,
		OwnerJID:     ownerJID,
		DefaultModel: defaultModel,
		CreatedAt:    time.Now(),
		Enabled:      true,
	}

	_, err := tm.db.Exec(`
		INSERT INTO teams (id, name, description, owner_jid, default_model, created_at, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		team.ID, team.Name, team.Description, team.OwnerJID, team.DefaultModel,
		team.CreatedAt.Format(time.RFC3339), team.Enabled,
	)
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}

	tm.logger.Info("team created", "team_id", team.ID, "name", name, "owner", ownerJID)
	return team, nil
}

// GetTeam retrieves a team by ID.
func (tm *TeamManager) GetTeam(teamID string) (*Team, error) {
	var team Team
	var defaultModel, workspacePath sql.NullString
	var createdAtStr string

	err := tm.db.QueryRow(`
		SELECT id, name, description, owner_jid, default_model, workspace_path, created_at, enabled
		FROM teams WHERE id = ?`,
		teamID,
	).Scan(
		&team.ID, &team.Name, &team.Description, &team.OwnerJID,
		&defaultModel, &workspacePath, &createdAtStr, &team.Enabled,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}

	team.DefaultModel = defaultModel.String
	team.WorkspacePath = workspacePath.String
	team.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &team, nil
}

// ResolveTeam resolves a team by ID or name.
// If teamRef is empty and there's only one team, returns that team.
// If teamRef is empty and there are multiple teams, returns an error asking to specify.
// Normalizes names: case insensitive, spaces/hyphens/underscores are equivalent.
func (tm *TeamManager) ResolveTeam(teamRef string) (*Team, error) {
	// If teamRef provided, try to find by ID or name
	if teamRef != "" {
		// Try by ID first
		team, err := tm.GetTeam(teamRef)
		if err != nil {
			return nil, err
		}
		if team != nil {
			return team, nil
		}

		// Try by name (case insensitive, normalized)
		normalizedRef := normalizeTeamName(teamRef)
		teams, err := tm.ListTeams()
		if err != nil {
			return nil, err
		}

		var matches []*Team
		for _, t := range teams {
			if normalizeTeamName(t.Name) == normalizedRef {
				matches = append(matches, t)
			}
		}

		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			var names []string
			for _, t := range matches {
				names = append(names, fmt.Sprintf("%s (ID: %s)", t.Name, t.ID))
			}
			return nil, fmt.Errorf("multiple teams match '%s': %s. Please specify the team ID", teamRef, strings.Join(names, ", "))
		}

		return nil, fmt.Errorf("team not found: %s. Use team_manage(action='list') to see available teams", teamRef)
	}

	// No teamRef provided - check if there's a single default team
	teams, err := tm.ListTeams()
	if err != nil {
		return nil, err
	}

	if len(teams) == 0 {
		return nil, fmt.Errorf("no teams exist. Create one with team_manage(action='create')")
	}

	if len(teams) == 1 {
		tm.logger.Debug("using single team as default", "team_id", teams[0].ID, "name", teams[0].Name)
		return teams[0], nil
	}

	// Multiple teams - ask user to specify
	var teamNames []string
	for _, t := range teams {
		teamNames = append(teamNames, fmt.Sprintf("- %s (ID: %s)", t.Name, t.ID))
	}
	return nil, fmt.Errorf("multiple teams exist, please specify which team to use:\n%s\n\nExample: team_agent(action='list', team_id='%s')",
		strings.Join(teamNames, "\n"), teams[0].ID)
}

// normalizeTeamName normalizes a team name for comparison.
// Case insensitive, replaces spaces/hyphens/underscores with single space, trims.
func normalizeTeamName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Replace hyphens and underscores with spaces
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	// Collapse multiple spaces
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	return name
}

// ListTeams lists all teams.
func (tm *TeamManager) ListTeams() ([]*Team, error) {
	rows, err := tm.db.Query(`
		SELECT id, name, description, owner_jid, default_model, workspace_path, created_at, enabled
		FROM teams WHERE enabled = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var team Team
		var defaultModel, workspacePath sql.NullString
		var createdAtStr string
		if err := rows.Scan(
			&team.ID, &team.Name, &team.Description, &team.OwnerJID,
			&defaultModel, &workspacePath, &createdAtStr, &team.Enabled,
		); err != nil {
			continue
		}
		team.DefaultModel = defaultModel.String
		team.WorkspacePath = workspacePath.String
		team.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		teams = append(teams, &team)
	}
	return teams, nil
}

// UpdateTeam updates a team's properties.
func (tm *TeamManager) UpdateTeam(team *Team) error {
	_, err := tm.db.Exec(`
		UPDATE teams SET name = ?, description = ?, default_model = ?, workspace_path = ?
		WHERE id = ?`,
		team.Name, team.Description, team.DefaultModel, team.WorkspacePath, team.ID,
	)
	if err != nil {
		return fmt.Errorf("update team: %w", err)
	}

	tm.logger.Info("team updated", "team_id", team.ID, "name", team.Name)
	return nil
}

// DeleteTeam permanently deletes a team and all its agents.
func (tm *TeamManager) DeleteTeam(teamID string) error {
	// First, stop and delete all agents
	agents, err := tm.ListAgents(teamID)
	if err != nil {
		return fmt.Errorf("list agents for deletion: %w", err)
	}
	for _, agent := range agents {
		if err := tm.DeleteAgent(agent.ID); err != nil {
			tm.logger.Warn("failed to delete agent during team deletion", "agent_id", agent.ID, "error", err)
		}
	}

	// Delete team memory data
	teamMem := NewTeamMemory(teamID, tm.db, tm.logger)
	if err := teamMem.DeleteAllData(); err != nil {
		tm.logger.Warn("failed to delete team memory data", "team_id", teamID, "error", err)
	}

	// Delete team from database
	_, err = tm.db.Exec(`DELETE FROM teams WHERE id = ?`, teamID)
	if err != nil {
		return fmt.Errorf("delete team: %w", err)
	}

	tm.logger.Info("team deleted", "team_id", teamID)
	return nil
}

// ─── Persistent Agents ───

// CreateAgent creates a new persistent agent.
func (tm *TeamManager) CreateAgent(
	teamID, name, role, personality, instructions, model string,
	skills []string, level AgentLevel, heartbeatSchedule string,
) (*PersistentAgent, error) {
	// Validate team exists
	team, err := tm.GetTeam(teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, fmt.Errorf("team %s not found", teamID)
	}

	// Generate agent ID from name (lowercase, alphanumeric)
	agentID := strings.ToLower(name)
	agentID = strings.ReplaceAll(agentID, " ", "-")
	agentID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, agentID)

	if heartbeatSchedule == "" {
		heartbeatSchedule = "*/15 * * * *" // Default: every 15 minutes
	}

	agent := &PersistentAgent{
		ID:                agentID,
		Name:              name,
		Role:              role,
		TeamID:            teamID,
		Level:             level,
		Status:            AgentStatusIdle,
		Personality:       personality,
		Instructions:      instructions,
		Model:             model,
		Skills:            skills,
		HeartbeatSchedule: heartbeatSchedule,
		CreatedAt:         time.Now(),
	}

	skillsJSON, _ := json.Marshal(agent.Skills)

	_, err = tm.db.Exec(`
		INSERT INTO persistent_agents (id, name, role, team_id, level, status, personality, instructions, model, skills, heartbeat_schedule, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID, agent.Name, agent.Role, agent.TeamID, string(agent.Level), string(agent.Status),
		agent.Personality, agent.Instructions, agent.Model, string(skillsJSON),
		agent.HeartbeatSchedule, agent.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	// Create heartbeat job if scheduler is available
	if tm.scheduler != nil {
		tm.createHeartbeatJob(agent)
	}

	// Add to in-memory cache
	tm.mu.Lock()
	tm.agents[agent.ID] = agent
	tm.mu.Unlock()

	tm.logger.Info("agent created", "agent_id", agent.ID, "name", name, "role", role, "team_id", teamID)
	return agent, nil
}

// GetAgent retrieves a persistent agent by ID.
func (tm *TeamManager) GetAgent(agentID string) (*PersistentAgent, error) {
	// Check cache first
	tm.mu.RLock()
	if agent, ok := tm.agents[agentID]; ok {
		tm.mu.RUnlock()
		return agent, nil
	}
	tm.mu.RUnlock()

	// Load from DB
	var agent PersistentAgent
	var skillsJSON string
	var lastHeartbeat, lastActive sql.NullString
	var personality, instructions, model sql.NullString
	var createdAtStr string

	err := tm.db.QueryRow(`
		SELECT id, name, role, team_id, level, status, personality, instructions, model, skills, heartbeat_schedule, created_at, last_active_at, last_heartbeat_at
		FROM persistent_agents WHERE id = ?`,
		agentID,
	).Scan(
		&agent.ID, &agent.Name, &agent.Role, &agent.TeamID, &agent.Level, &agent.Status,
		&personality, &instructions, &model, &skillsJSON, &agent.HeartbeatSchedule,
		&createdAtStr, &lastActive, &lastHeartbeat,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	agent.Personality = personality.String
	agent.Instructions = instructions.String
	agent.Model = model.String
	agent.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if lastActive.Valid {
		agent.LastActiveAt, _ = time.Parse(time.RFC3339, lastActive.String)
	}
	json.Unmarshal([]byte(skillsJSON), &agent.Skills)

	if lastHeartbeat.Valid {
		t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
		agent.LastHeartbeatAt = &t
	}

	return &agent, nil
}

// ListAgents lists all agents in a team.
func (tm *TeamManager) ListAgents(teamID string) ([]*PersistentAgent, error) {
	query := `SELECT id, name, role, team_id, level, status, personality, instructions, model, skills, heartbeat_schedule, created_at, last_active_at, last_heartbeat_at
              FROM persistent_agents`
	args := []interface{}{}

	if teamID != "" {
		query += " WHERE team_id = ?"
		args = append(args, teamID)
	}
	query += " ORDER BY created_at ASC"

	rows, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*PersistentAgent
	for rows.Next() {
		var agent PersistentAgent
		var skillsJSON string
		var lastHeartbeat, lastActive sql.NullString
		var personality, instructions, model sql.NullString
		var createdAtStr string

		if err := rows.Scan(
			&agent.ID, &agent.Name, &agent.Role, &agent.TeamID, &agent.Level, &agent.Status,
			&personality, &instructions, &model, &skillsJSON, &agent.HeartbeatSchedule,
			&createdAtStr, &lastActive, &lastHeartbeat,
		); err != nil {
			continue
		}

		agent.Personality = personality.String
		agent.Instructions = instructions.String
		agent.Model = model.String
		agent.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if lastActive.Valid {
			agent.LastActiveAt, _ = time.Parse(time.RFC3339, lastActive.String)
		}
		json.Unmarshal([]byte(skillsJSON), &agent.Skills)

		if lastHeartbeat.Valid {
			t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
			agent.LastHeartbeatAt = &t
		}

		agents = append(agents, &agent)
	}
	return agents, nil
}

// UpdateAgentStatus updates an agent's status.
func (tm *TeamManager) UpdateAgentStatus(agentID string, status AgentStatus) error {
	now := time.Now().Format(time.RFC3339)

	_, err := tm.db.Exec(`
		UPDATE persistent_agents SET status = ?, last_active_at = ? WHERE id = ?`,
		string(status), now, agentID,
	)
	if err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}

	// Update cache
	tm.mu.Lock()
	if agent, ok := tm.agents[agentID]; ok {
		agent.Status = status
		agent.LastActiveAt = time.Now()
	}
	tm.mu.Unlock()

	tm.logger.Debug("agent status updated", "agent_id", agentID, "status", status)
	return nil
}

// SetAgentCurrentTask sets the current task for an agent.
func (tm *TeamManager) SetAgentCurrentTask(agentID, taskID string) error {
	_, err := tm.db.Exec(`
		UPDATE persistent_agents SET current_task_id = ?, last_active_at = ? WHERE id = ?`,
		taskID, time.Now().Format(time.RFC3339), agentID,
	)
	if err != nil {
		return fmt.Errorf("set agent task: %w", err)
	}

	tm.mu.Lock()
	if agent, ok := tm.agents[agentID]; ok {
		agent.CurrentTaskID = taskID
	}
	tm.mu.Unlock()

	return nil
}

// StopAgent stops a persistent agent (disables heartbeats).
func (tm *TeamManager) StopAgent(agentID string) error {
	// Update status
	if err := tm.UpdateAgentStatus(agentID, AgentStatusStopped); err != nil {
		return err
	}

	// Remove heartbeat job
	if tm.scheduler != nil {
		jobID := fmt.Sprintf("heartbeat-%s", agentID)
		tm.scheduler.Remove(jobID)
	}

	tm.logger.Info("agent stopped", "agent_id", agentID)
	return nil
}

// StartAgent restarts a stopped persistent agent.
func (tm *TeamManager) StartAgent(agentID string) error {
	agent, err := tm.GetAgent(agentID)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Update status
	if err := tm.UpdateAgentStatus(agentID, AgentStatusIdle); err != nil {
		return err
	}

	// Recreate heartbeat job if scheduler is available
	if tm.scheduler != nil {
		tm.createHeartbeatJob(agent)
	}

	// Add to in-memory cache
	tm.mu.Lock()
	tm.agents[agentID] = agent
	tm.mu.Unlock()

	tm.logger.Info("agent started", "agent_id", agentID)
	return nil
}

// UpdateAgent updates an agent's properties.
func (tm *TeamManager) UpdateAgent(agent *PersistentAgent) error {
	skillsJSON, _ := json.Marshal(agent.Skills)

	_, err := tm.db.Exec(`
		UPDATE persistent_agents SET
			name = ?, role = ?, personality = ?, instructions = ?,
			model = ?, skills = ?, level = ?, heartbeat_schedule = ?
		WHERE id = ?`,
		agent.Name, agent.Role, agent.Personality, agent.Instructions,
		agent.Model, string(skillsJSON), string(agent.Level), agent.HeartbeatSchedule,
		agent.ID,
	)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}

	// Update cache
	tm.mu.Lock()
	if cached, ok := tm.agents[agent.ID]; ok {
		cached.Name = agent.Name
		cached.Role = agent.Role
		cached.Personality = agent.Personality
		cached.Instructions = agent.Instructions
		cached.Model = agent.Model
		cached.Skills = agent.Skills
		cached.Level = agent.Level
		cached.HeartbeatSchedule = agent.HeartbeatSchedule
	}
	tm.mu.Unlock()

	tm.logger.Info("agent updated", "agent_id", agent.ID, "name", agent.Name)
	return nil
}

// DeleteAgent permanently removes an agent.
func (tm *TeamManager) DeleteAgent(agentID string) error {
	// Stop first
	tm.StopAgent(agentID)

	// Delete from DB
	_, err := tm.db.Exec(`DELETE FROM persistent_agents WHERE id = ?`, agentID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}

	// Remove from cache
	tm.mu.Lock()
	delete(tm.agents, agentID)
	tm.mu.Unlock()

	tm.logger.Info("agent deleted", "agent_id", agentID)
	return nil
}

// ─── Heartbeats ───

// createHeartbeatJob creates a scheduler job for agent heartbeats.
func (tm *TeamManager) createHeartbeatJob(agent *PersistentAgent) {
	jobID := fmt.Sprintf("heartbeat-%s", agent.ID)

	job := &scheduler.Job{
		ID:        jobID,
		Schedule:  agent.HeartbeatSchedule,
		Type:      "cron",
		Command:   tm.buildHeartbeatPrompt(agent),
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	if err := tm.scheduler.Add(job); err != nil {
		tm.logger.Warn("failed to create heartbeat job", "agent_id", agent.ID, "error", err)
	} else {
		tm.logger.Info("heartbeat job created", "agent_id", agent.ID, "schedule", agent.HeartbeatSchedule)
	}
}

// buildHeartbeatPrompt builds the heartbeat prompt for an agent.
func (tm *TeamManager) buildHeartbeatPrompt(agent *PersistentAgent) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`[HEARTBEAT for %s]

You are %s, %s of the team.
`, agent.Name, agent.Name, agent.Role))

	// Load and include working state if exists
	state, _ := tm.GetAgentWorkingState(agent.ID)
	if state != nil && state.Status != "idle" {
		sb.WriteString("\n## Current Work State (WORKING.md)\n")
		sb.WriteString(fmt.Sprintf("- Status: %s\n", state.Status))
		if state.CurrentTaskID != "" {
			sb.WriteString(fmt.Sprintf("- Current Task: %s\n", state.CurrentTaskID))
		}
		if state.NextSteps != "" {
			sb.WriteString("- Next Steps:\n")
			sb.WriteString(state.NextSteps)
			sb.WriteString("\n")
		}
		if state.Context != "" {
			sb.WriteString(fmt.Sprintf("- Context: %s\n", state.Context))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf(`HEARTBEAT CHECKLIST:
1. Check your WORKING.md file for ongoing tasks
2. If a task is in progress, resume it
3. Check TeamMemory for @%s mentions
4. Check tasks assigned to you
5. Scan the activity feed for relevant discussions

ACTIONS:
- If there is work to do: DO IT
- If nothing to do: respond with exactly "HEARTBEAT_OK"
- Use team_update_working to save your progress

Your agent_id: %s
Your role: %s
`, agent.ID, agent.ID, agent.Role))

	return sb.String()
}

// RecordHeartbeat records that a heartbeat was executed.
func (tm *TeamManager) RecordHeartbeat(agentID string) error {
	now := time.Now().Format(time.RFC3339)

	_, err := tm.db.Exec(`
		UPDATE persistent_agents SET last_heartbeat_at = ? WHERE id = ?`,
		now, agentID,
	)
	if err != nil {
		return fmt.Errorf("record heartbeat: %w", err)
	}

	tm.mu.Lock()
	if agent, ok := tm.agents[agentID]; ok {
		now := time.Now()
		agent.LastHeartbeatAt = &now
	}
	tm.mu.Unlock()

	tm.logger.Debug("heartbeat recorded", "agent_id", agentID)
	return nil
}

// ─── Agent Communication ───

// SendToAgent sends a message to an agent, triggering an immediate run.
func (tm *TeamManager) SendToAgent(ctx context.Context, toAgentID, fromAgent, message string) error {
	agent, err := tm.GetAgent(toAgentID)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent %s not found", toAgentID)
	}

	// Add to pending messages
	teamMem := NewTeamMemory(agent.TeamID, tm.db, tm.logger)
	teamMem.addPendingMessage(toAgentID, fromAgent, message, "")

	// Trigger immediate run if spawn callback is set
	if tm.spawnAgent != nil {
		fullMessage := fmt.Sprintf("Message from %s:\n\n%s", fromAgent, message)
		if err := tm.spawnAgent(ctx, agent, fullMessage); err != nil {
			tm.logger.Warn("failed to trigger agent run", "agent_id", toAgentID, "error", err)
		}
	}

	tm.logger.Info("message sent to agent", "to", toAgentID, "from", fromAgent)
	return nil
}

// ─── Helpers ───

// loadAgentsFromDB loads all agents into memory on startup.
func (tm *TeamManager) loadAgentsFromDB() {
	agents, err := tm.ListAgents("")
	if err != nil {
		tm.logger.Warn("failed to load agents from DB", "error", err)
		return
	}

	tm.mu.Lock()
	for _, agent := range agents {
		// Skip stopped agents
		if agent.Status == AgentStatusStopped {
			continue
		}
		tm.agents[agent.ID] = agent
	}
	tm.mu.Unlock()

	tm.logger.Info("agents loaded from DB", "count", len(tm.agents))
}

// GetTeamMemory creates a TeamMemory instance for a team.
func (tm *TeamManager) GetTeamMemory(teamID string) *TeamMemory {
	return NewTeamMemory(teamID, tm.db, tm.logger)
}

// FindAgentByName finds an agent by name (case-insensitive).
func (tm *TeamManager) FindAgentByName(name string) (*PersistentAgent, error) {
	name = strings.ToLower(name)

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	for _, agent := range tm.agents {
		if strings.ToLower(agent.Name) == name || strings.ToLower(agent.ID) == name {
			return agent, nil
		}
	}

	return nil, nil
}

// ParseMentions extracts @mentions from text and returns agent IDs.
func (tm *TeamManager) ParseMentions(text string) []string {
	var mentions []string
	words := strings.Fields(text)

	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			agentName := strings.TrimPrefix(word, "@")
			agentName = strings.ToLower(agentName)
			agentName = strings.TrimRight(agentName, ".,!?:;")

			if agent, _ := tm.FindAgentByName(agentName); agent != nil {
				mentions = append(mentions, agent.ID)
			}
		}
	}

	return mentions
}

// BuildAgentSystemPrompt builds a system prompt for a persistent agent.
func (tm *TeamManager) BuildAgentSystemPrompt(agent *PersistentAgent, teamMemory *TeamMemory) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# You are %s\n\n", agent.Name))
	sb.WriteString(fmt.Sprintf("**Role:** %s\n", agent.Role))
	sb.WriteString(fmt.Sprintf("**Agent ID:** %s\n\n", agent.ID))

	if agent.Personality != "" {
		sb.WriteString("## Personality\n")
		sb.WriteString(agent.Personality)
		sb.WriteString("\n\n")
	}

	if agent.Instructions != "" {
		sb.WriteString("## Instructions\n")
		sb.WriteString(agent.Instructions)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Team Tools\n")
	sb.WriteString("You have access to team tools for coordination:\n")
	sb.WriteString("- `team_check_mentions` - Check for @mentions to you\n")
	sb.WriteString("- `team_list_tasks` - List tasks (optionally filtered by status or assignee)\n")
	sb.WriteString("- `team_update_task` - Update a task's status\n")
	sb.WriteString("- `team_comment` - Add a comment to a task thread\n")
	sb.WriteString("- `team_mention` - Mention another agent (@agent_id)\n")
	sb.WriteString("- `team_save_fact` - Save a fact to shared memory\n")
	sb.WriteString("- `team_get_facts` - Get all shared facts\n")
	sb.WriteString("\n")

	// Add team context if available
	if teamMemory != nil {
		context, err := teamMemory.BuildTeamContext(agent.ID)
		if err == nil && context != "" {
			sb.WriteString("## Current Team State\n\n")
			sb.WriteString(context)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// ─── Agent Working State (WORKING.md) ───

// GetAgentWorkingState retrieves an agent's working state.
func (tm *TeamManager) GetAgentWorkingState(agentID string) (*AgentWorkingState, error) {
	var state AgentWorkingState
	var updatedAtStr string
	var currentTaskID, nextSteps, context sql.NullString

	err := tm.db.QueryRow(`
		SELECT agent_id, team_id, current_task_id, status, next_steps, context, updated_at
		FROM agent_working_state WHERE agent_id = ?`,
		agentID,
	).Scan(
		&state.AgentID, &state.TeamID, &currentTaskID, &state.Status,
		&nextSteps, &context, &updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent working state: %w", err)
	}

	state.CurrentTaskID = currentTaskID.String
	state.NextSteps = nextSteps.String
	state.Context = context.String
	state.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	return &state, nil
}

// SaveAgentWorkingState saves an agent's working state.
func (tm *TeamManager) SaveAgentWorkingState(state *AgentWorkingState) error {
	now := time.Now().Format(time.RFC3339)

	// Check if state exists
	var existing string
	err := tm.db.QueryRow(`SELECT agent_id FROM agent_working_state WHERE agent_id = ?`, state.AgentID).Scan(&existing)

	if err == sql.ErrNoRows {
		// Insert new state
		_, err = tm.db.Exec(`
			INSERT INTO agent_working_state (agent_id, team_id, current_task_id, status, next_steps, context, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			state.AgentID, state.TeamID, state.CurrentTaskID, state.Status,
			state.NextSteps, state.Context, now,
		)
	} else if err == nil {
		// Update existing state
		_, err = tm.db.Exec(`
			UPDATE agent_working_state SET current_task_id = ?, status = ?, next_steps = ?, context = ?, updated_at = ?
			WHERE agent_id = ?`,
			state.CurrentTaskID, state.Status, state.NextSteps, state.Context, now,
			state.AgentID,
		)
	}

	if err != nil {
		return fmt.Errorf("save agent working state: %w", err)
	}

	tm.logger.Debug("agent working state saved", "agent_id", state.AgentID, "status", state.Status)
	return nil
}

// ClearAgentWorkingState clears an agent's working state (e.g., on task completion).
func (tm *TeamManager) ClearAgentWorkingState(agentID string) error {
	_, err := tm.db.Exec(`
		UPDATE agent_working_state SET current_task_id = '', status = 'idle', next_steps = '', context = '', updated_at = ?
		WHERE agent_id = ?`,
		time.Now().Format(time.RFC3339), agentID,
	)
	if err != nil {
		return fmt.Errorf("clear agent working state: %w", err)
	}

	tm.logger.Debug("agent working state cleared", "agent_id", agentID)
	return nil
}
