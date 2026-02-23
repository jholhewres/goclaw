// Package copilot – team_memory.go implements shared memory for team agents.
// Provides tasks, messages, facts, and activity tracking accessible by all team members.
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
)

// TeamMemory provides shared memory and communication for a team.
type TeamMemory struct {
	teamID      string
	db          *sql.DB
	logger      *slog.Logger
	mu          sync.RWMutex
	teamManager *TeamManager // Optional: for notification push
}

// NewTeamMemory creates a new team memory instance.
func NewTeamMemory(teamID string, db *sql.DB, logger *slog.Logger) *TeamMemory {
	if logger == nil {
		logger = slog.Default()
	}
	return &TeamMemory{
		teamID: teamID,
		db:     db,
		logger: logger.With("component", "team-memory", "team_id", teamID),
	}
}

// SetTeamManager sets the team manager for notification push.
func (tm *TeamMemory) SetTeamManager(mgr *TeamManager) {
	tm.teamManager = mgr
}

// ─── Tasks ───

// CreateTask creates a new task in the team.
func (tm *TeamMemory) CreateTask(title, description, createdBy string, assignees []string) (*TeamTask, error) {
	task := &TeamTask{
		ID:          uuid.New().String()[:8],
		TeamID:      tm.teamID,
		Title:       title,
		Description: description,
		Status:      TaskStatusInbox,
		Assignees:   assignees,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	assigneesJSON, _ := json.Marshal(task.Assignees)

	_, err := tm.db.Exec(`
		INSERT INTO team_tasks (id, team_id, title, description, status, assignees, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.TeamID, task.Title, task.Description, string(task.Status),
		string(assigneesJSON), task.CreatedBy, task.CreatedAt.Format(time.RFC3339), task.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	// Log activity
	tm.logActivity(ActivityTaskCreated, createdBy, fmt.Sprintf("Task created: %s", title), task.ID)

	// If assigned, log assignment
	if len(assignees) > 0 {
		tm.logActivity(ActivityTaskAssigned, createdBy,
			fmt.Sprintf("Task '%s' assigned to %s", title, strings.Join(assignees, ", ")), task.ID)
	}

	tm.logger.Info("task created", "task_id", task.ID, "title", title, "assignees", assignees)
	return task, nil
}

// GetTask retrieves a task by ID.
func (tm *TeamMemory) GetTask(taskID string) (*TeamTask, error) {
	var task TeamTask
	var assigneesJSON, labelsJSON string
	var completedAt sql.NullString
	var createdAtStr, updatedAtStr string

	err := tm.db.QueryRow(`
		SELECT id, team_id, title, description, status, assignees, priority, labels,
		       created_by, created_at, updated_at, completed_at, blocked_reason
		FROM team_tasks WHERE id = ? AND team_id = ?`,
		taskID, tm.teamID,
	).Scan(
		&task.ID, &task.TeamID, &task.Title, &task.Description, &task.Status, &assigneesJSON,
		&task.Priority, &labelsJSON, &task.CreatedBy, &createdAtStr, &updatedAtStr,
		&completedAt, &task.BlockedReason,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	json.Unmarshal([]byte(assigneesJSON), &task.Assignees)
	json.Unmarshal([]byte(labelsJSON), &task.Labels)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		task.CompletedAt = &t
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	return &task, nil
}

// UpdateTask updates a task's status and optionally adds a comment.
func (tm *TeamMemory) UpdateTask(taskID string, status TaskStatus, comment, updatedBy string) error {
	// Get current task
	task, err := tm.GetTask(taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	now := time.Now()
	var completedAt *string
	if status == TaskStatusDone {
		ts := now.Format(time.RFC3339)
		completedAt = &ts
	}

	var completedAtSQL interface{}
	if completedAt != nil {
		completedAtSQL = *completedAt
	}

	_, err = tm.db.Exec(`
		UPDATE team_tasks SET status = ?, updated_at = ?, completed_at = ? WHERE id = ?`,
		string(status), now.Format(time.RFC3339), completedAtSQL, taskID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	// Add comment if provided
	if comment != "" {
		_, err = tm.PostMessage(taskID, updatedBy, comment, nil)
		if err != nil {
			tm.logger.Warn("failed to add task comment", "task_id", taskID, "error", err)
		}
	}

	// Log activity
	tm.logActivity(ActivityTaskUpdated, updatedBy,
		fmt.Sprintf("Task '%s' status changed to %s", task.Title, status), taskID)

	tm.logger.Info("task updated", "task_id", taskID, "status", status, "by", updatedBy)
	return nil
}

// AssignTask assigns agents to a task.
func (tm *TeamMemory) AssignTask(taskID string, assignees []string, assignedBy string) error {
	assigneesJSON, _ := json.Marshal(assignees)

	_, err := tm.db.Exec(`
		UPDATE team_tasks SET assignees = ?, status = ?, updated_at = ? WHERE id = ? AND team_id = ?`,
		string(assigneesJSON), string(TaskStatusAssigned), time.Now().Format(time.RFC3339),
		taskID, tm.teamID,
	)
	if err != nil {
		return fmt.Errorf("assign task: %w", err)
	}

	task, _ := tm.GetTask(taskID)
	taskTitle := taskID
	if task != nil {
		taskTitle = task.Title
	}

	// Subscribe assignees to the task thread
	for _, agentID := range assignees {
		tm.SubscribeToThread(taskID, agentID, SubscriptionAssigned)
	}

	tm.logActivity(ActivityTaskAssigned, assignedBy,
		fmt.Sprintf("Task '%s' assigned to %s", taskTitle, strings.Join(assignees, ", ")), taskID)

	tm.logger.Info("task assigned", "task_id", taskID, "assignees", assignees)
	return nil
}

// ListTasks lists tasks with optional filters.
func (tm *TeamMemory) ListTasks(statusFilter TaskStatus, assigneeFilter string) ([]*TeamTask, error) {
	query := `SELECT id, team_id, title, description, status, assignees, priority, labels,
              created_by, created_at, updated_at, completed_at, blocked_reason
              FROM team_tasks WHERE team_id = ?`
	args := []interface{}{tm.teamID}

	if statusFilter != "" {
		query += " AND status = ?"
		args = append(args, string(statusFilter))
	}

	if assigneeFilter != "" {
		query += " AND assignees LIKE ?"
		args = append(args, "%"+assigneeFilter+"%")
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*TeamTask
	for rows.Next() {
		var task TeamTask
		var assigneesJSON, labelsJSON string
		var completedAt sql.NullString
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(
			&task.ID, &task.TeamID, &task.Title, &task.Description, &task.Status, &assigneesJSON,
			&task.Priority, &labelsJSON, &task.CreatedBy, &createdAtStr, &updatedAtStr,
			&completedAt, &task.BlockedReason,
		); err != nil {
			continue
		}

		json.Unmarshal([]byte(assigneesJSON), &task.Assignees)
		json.Unmarshal([]byte(labelsJSON), &task.Labels)
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			task.CompletedAt = &t
		}
		task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetTasksForAgent returns tasks assigned to a specific agent.
func (tm *TeamMemory) GetTasksForAgent(agentID string) ([]*TeamTask, error) {
	return tm.ListTasks("", agentID)
}

// DeleteTask permanently deletes a task and all its messages.
func (tm *TeamMemory) DeleteTask(taskID string) error {
	// Delete messages first
	_, err := tm.db.Exec(`DELETE FROM team_messages WHERE team_id = ? AND thread_id = ?`, tm.teamID, taskID)
	if err != nil {
		tm.logger.Warn("failed to delete task messages", "task_id", taskID, "error", err)
	}

	// Delete subscriptions
	_, err = tm.db.Exec(`DELETE FROM team_thread_subscriptions WHERE team_id = ? AND thread_id = ?`, tm.teamID, taskID)
	if err != nil {
		tm.logger.Warn("failed to delete task subscriptions", "task_id", taskID, "error", err)
	}

	// Delete the task
	_, err = tm.db.Exec(`DELETE FROM team_tasks WHERE id = ? AND team_id = ?`, taskID, tm.teamID)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}

	tm.logger.Info("task deleted", "task_id", taskID)
	return nil
}

// ─── Messages ───

// PostMessage posts a message to a thread/task.
func (tm *TeamMemory) PostMessage(threadID, fromAgent, content string, mentions []string) (*TeamMessage, error) {
	msg := &TeamMessage{
		ID:        uuid.New().String()[:8],
		TeamID:    tm.teamID,
		ThreadID:  threadID,
		FromAgent: fromAgent,
		Content:   content,
		Mentions:  mentions,
		CreatedAt: time.Now(),
		Delivered: false,
	}

	mentionsJSON, _ := json.Marshal(msg.Mentions)

	_, err := tm.db.Exec(`
		INSERT INTO team_messages (id, team_id, thread_id, from_agent, content, mentions, created_at, delivered)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.TeamID, msg.ThreadID, msg.FromAgent, msg.Content,
		string(mentionsJSON), msg.CreatedAt.Format(time.RFC3339), msg.Delivered,
	)
	if err != nil {
		return nil, fmt.Errorf("post message: %w", err)
	}

	// Auto-subscribe sender to thread
	tm.SubscribeToThread(threadID, fromAgent, SubscriptionAuto)

	// Subscribe mentioned agents to thread
	for _, agentID := range mentions {
		if agentID == "" || agentID == fromAgent {
			continue
		}
		tm.SubscribeToThread(threadID, agentID, SubscriptionMentioned)
		tm.addPendingMessage(agentID, fromAgent, content, threadID)
		tm.logActivity(ActivityMention, fromAgent,
			fmt.Sprintf("@%s mentioned in thread", agentID), msg.ID)
	}

	// Notify all thread subscribers (push notification)
	// Note: Run synchronously to avoid race conditions with test DB cleanup
	tm.NotifySubscribers(threadID, fromAgent, content, append(mentions, fromAgent))

	tm.logActivity(ActivityMessageSent, fromAgent,
		fmt.Sprintf("Message posted to thread %s", threadID), msg.ID)

	return msg, nil
}

// GetThreadMessages returns all messages in a thread.
func (tm *TeamMemory) GetThreadMessages(threadID string, limit int) ([]*TeamMessage, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := tm.db.Query(`
		SELECT id, team_id, thread_id, from_agent, from_user, content, mentions, created_at, delivered
		FROM team_messages WHERE team_id = ? AND thread_id = ?
		ORDER BY created_at ASC LIMIT ?`,
		tm.teamID, threadID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get thread messages: %w", err)
	}
	defer rows.Close()

	var messages []*TeamMessage
	for rows.Next() {
		var msg TeamMessage
		var mentionsJSON string
		var fromUser sql.NullString
		var createdAtStr string

		if err := rows.Scan(
			&msg.ID, &msg.TeamID, &msg.ThreadID, &msg.FromAgent, &fromUser,
			&msg.Content, &mentionsJSON, &createdAtStr, &msg.Delivered,
		); err != nil {
			continue
		}

		msg.FromUser = fromUser.String
		msg.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		json.Unmarshal([]byte(mentionsJSON), &msg.Mentions)
		messages = append(messages, &msg)
	}

	return messages, nil
}

// ─── Pending Messages (Mailbox) ───

// addPendingMessage adds a message to an agent's mailbox.
func (tm *TeamMemory) addPendingMessage(toAgent, fromAgent, content, threadID string) error {
	msgID := uuid.New().String()[:8]

	_, err := tm.db.Exec(`
		INSERT INTO team_pending_messages (id, to_agent, from_agent, content, thread_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		msgID, toAgent, fromAgent, content, threadID, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		tm.logger.Warn("failed to add pending message", "to", toAgent, "error", err)
		return err
	}

	tm.logger.Debug("pending message added", "id", msgID, "to", toAgent, "from", fromAgent)
	return nil
}

// GetPendingMessages retrieves undelivered messages for an agent.
func (tm *TeamMemory) GetPendingMessages(agentID string, markDelivered bool) ([]*PendingMessage, error) {
	rows, err := tm.db.Query(`
		SELECT id, to_agent, from_agent, content, thread_id, created_at, delivered
		FROM team_pending_messages WHERE to_agent = ? AND delivered = 0
		ORDER BY created_at ASC LIMIT 50`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending messages: %w", err)
	}
	defer rows.Close()

	var messages []*PendingMessage
	var ids []string

	for rows.Next() {
		var msg PendingMessage
		var createdAt string
		var threadID sql.NullString

		if err := rows.Scan(
			&msg.ID, &msg.ToAgent, &msg.FromAgent, &msg.Content, &threadID, &createdAt, &msg.Delivered,
		); err != nil {
			continue
		}

		msg.ThreadID = threadID.String
		msg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		messages = append(messages, &msg)
		ids = append(ids, msg.ID)
	}

	// Mark as delivered if requested
	if markDelivered && len(ids) > 0 {
		now := time.Now().Format(time.RFC3339)
		for _, id := range ids {
			tm.db.Exec(`
				UPDATE team_pending_messages SET delivered = 1, delivered_at = ? WHERE id = ?`,
				now, id,
			)
		}
	}

	return messages, nil
}

// ─── Facts (Shared Memory) ───

// SaveFact saves a fact to shared team memory.
func (tm *TeamMemory) SaveFact(key, value, author string) error {
	now := time.Now().Format(time.RFC3339)

	// Check if fact exists
	var existingID string
	err := tm.db.QueryRow(`
		SELECT id FROM team_facts WHERE team_id = ? AND key = ?`,
		tm.teamID, key,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Insert new fact
		factID := uuid.New().String()[:8]
		_, err = tm.db.Exec(`
			INSERT INTO team_facts (id, team_id, key, value, author, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			factID, tm.teamID, key, value, author, now, now,
		)
	} else if err == nil {
		// Update existing fact
		_, err = tm.db.Exec(`
			UPDATE team_facts SET value = ?, author = ?, updated_at = ? WHERE id = ?`,
			value, author, now, existingID,
		)
	}

	if err != nil {
		return fmt.Errorf("save fact: %w", err)
	}

	tm.logActivity(ActivityFactCreated, author, fmt.Sprintf("Fact saved: %s", key), "")
	tm.logger.Info("fact saved", "key", key, "author", author)
	return nil
}

// GetFacts retrieves all facts for the team.
func (tm *TeamMemory) GetFacts() ([]*TeamFact, error) {
	rows, err := tm.db.Query(`
		SELECT id, team_id, key, value, author, created_at, updated_at
		FROM team_facts WHERE team_id = ?
		ORDER BY key ASC`,
		tm.teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("get facts: %w", err)
	}
	defer rows.Close()

	var facts []*TeamFact
	for rows.Next() {
		var fact TeamFact
		var createdAt, updatedAt string
		if err := rows.Scan(
			&fact.ID, &fact.TeamID, &fact.Key, &fact.Value,
			&fact.Author, &createdAt, &updatedAt,
		); err != nil {
			continue
		}
		fact.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		fact.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		facts = append(facts, &fact)
	}

	return facts, nil
}

// SearchFacts searches facts by key or value.
func (tm *TeamMemory) SearchFacts(query string) ([]*TeamFact, error) {
	rows, err := tm.db.Query(`
		SELECT id, team_id, key, value, author, created_at, updated_at
		FROM team_facts WHERE team_id = ? AND (key LIKE ? OR value LIKE ?)
		ORDER BY updated_at DESC LIMIT 20`,
		tm.teamID, "%"+query+"%", "%"+query+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("search facts: %w", err)
	}
	defer rows.Close()

	var facts []*TeamFact
	for rows.Next() {
		var fact TeamFact
		var createdAt, updatedAt string
		if err := rows.Scan(
			&fact.ID, &fact.TeamID, &fact.Key, &fact.Value,
			&fact.Author, &createdAt, &updatedAt,
		); err != nil {
			continue
		}
		fact.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		fact.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		facts = append(facts, &fact)
	}

	return facts, nil
}

// DeleteFact removes a fact from team memory.
func (tm *TeamMemory) DeleteFact(key string) error {
	_, err := tm.db.Exec(`DELETE FROM team_facts WHERE team_id = ? AND key = ?`, tm.teamID, key)
	if err != nil {
		return fmt.Errorf("delete fact: %w", err)
	}
	tm.logger.Info("fact deleted", "key", key)
	return nil
}

// ─── Activity Feed ───

// logActivity logs an activity to the feed.
func (tm *TeamMemory) logActivity(activityType ActivityType, agentID, message, relatedID string) {
	activityID := uuid.New().String()[:8]

	_, err := tm.db.Exec(`
		INSERT INTO team_activities (id, team_id, type, agent_id, message, related_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		activityID, tm.teamID, string(activityType), agentID, message, relatedID, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		tm.logger.Warn("failed to log activity", "type", activityType, "error", err)
	}
}

// GetActivities retrieves recent activities.
func (tm *TeamMemory) GetActivities(limit int) ([]*TeamActivity, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := tm.db.Query(`
		SELECT id, team_id, type, agent_id, message, related_id, created_at
		FROM team_activities WHERE team_id = ?
		ORDER BY created_at DESC LIMIT ?`,
		tm.teamID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get activities: %w", err)
	}
	defer rows.Close()

	var activities []*TeamActivity
	for rows.Next() {
		var activity TeamActivity
		var agentID, relatedID sql.NullString
		var createdAtStr string

		if err := rows.Scan(
			&activity.ID, &activity.TeamID, &activity.Type, &agentID,
			&activity.Message, &relatedID, &createdAtStr,
		); err != nil {
			continue
		}

		activity.AgentID = agentID.String
		activity.RelatedID = relatedID.String
		activity.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		activities = append(activities, &activity)
	}

	return activities, nil
}

// ─── Context Building ───

// BuildTeamContext creates a context string for agents with current team state.
func (tm *TeamMemory) BuildTeamContext(agentID string) (string, error) {
	var sb strings.Builder

	sb.WriteString("## Team Context\n\n")

	// Facts
	facts, err := tm.GetFacts()
	if err == nil && len(facts) > 0 {
		sb.WriteString("### Shared Facts\n")
		for _, f := range facts {
			sb.WriteString(fmt.Sprintf("- **%s**: %s (by %s)\n", f.Key, f.Value, f.Author))
		}
		sb.WriteString("\n")
	}

	// My tasks
	myTasks, err := tm.GetTasksForAgent(agentID)
	if err == nil && len(myTasks) > 0 {
		sb.WriteString("### Your Tasks\n")
		for _, t := range myTasks {
			if t.Status == TaskStatusDone || t.Status == TaskStatusCancelled {
				continue
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", t.Status, t.Title, t.ID))
		}
		sb.WriteString("\n")
	}

	// Pending messages
	pending, err := tm.GetPendingMessages(agentID, false)
	if err == nil && len(pending) > 0 {
		sb.WriteString("### Pending Messages\n")
		for _, m := range pending {
			sb.WriteString(fmt.Sprintf("- From %s: %s\n", m.FromAgent, truncateString(m.Content, 100)))
		}
		sb.WriteString("\n")
	}

	// Recent activity
	activities, err := tm.GetActivities(10)
	if err == nil && len(activities) > 0 {
		sb.WriteString("### Recent Activity\n")
		for _, a := range activities {
			sb.WriteString(fmt.Sprintf("- %s\n", a.Message))
		}
	}

	return sb.String(), nil
}

// ─── Standup ───

// GenerateStandup creates a daily standup summary.
func (tm *TeamMemory) GenerateStandup() (string, error) {
	now := time.Now()
	since := now.Add(-24 * time.Hour)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 DAILY STANDUP — %s\n\n", now.Format("Jan 02, 2006")))

	// Completed tasks
	rows, err := tm.db.Query(`
		SELECT title, created_by FROM team_tasks
		WHERE team_id = ? AND status = 'done' AND updated_at > ?
		ORDER BY updated_at DESC`,
		tm.teamID, since.Format(time.RFC3339),
	)
	if err == nil {
		defer rows.Close()
		sb.WriteString("✅ COMPLETED TODAY\n")
		hasCompleted := false
		for rows.Next() {
			var title, createdBy string
			if err := rows.Scan(&title, &createdBy); err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("• %s (by %s)\n", title, createdBy))
			hasCompleted = true
		}
		if !hasCompleted {
			sb.WriteString("• No tasks completed\n")
		}
		sb.WriteString("\n")
	}

	// In progress tasks
	inProgress, _ := tm.ListTasks(TaskStatusProgress, "")
	if len(inProgress) > 0 {
		sb.WriteString("🔄 IN PROGRESS\n")
		for _, t := range inProgress {
			assignees := strings.Join(t.Assignees, ", ")
			if assignees == "" {
				assignees = "unassigned"
			}
			sb.WriteString(fmt.Sprintf("• %s (%s)\n", t.Title, assignees))
		}
		sb.WriteString("\n")
	}

	// Blocked tasks
	blocked, _ := tm.ListTasks(TaskStatusBlocked, "")
	if len(blocked) > 0 {
		sb.WriteString("🚫 BLOCKED\n")
		for _, t := range blocked {
			reason := t.BlockedReason
			if reason == "" {
				reason = "no reason given"
			}
			sb.WriteString(fmt.Sprintf("• %s — %s\n", t.Title, reason))
		}
		sb.WriteString("\n")
	}

	// In review
	review, _ := tm.ListTasks(TaskStatusReview, "")
	if len(review) > 0 {
		sb.WriteString("👀 NEEDS REVIEW\n")
		for _, t := range review {
			sb.WriteString(fmt.Sprintf("• %s\n", t.Title))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// ─── Documents ───

// CreateDocument creates a new document linked to a task.
func (tm *TeamMemory) CreateDocument(title string, docType DocumentType, content, format, taskID, author string) (*TeamDocument, error) {
	doc := &TeamDocument{
		ID:        uuid.New().String()[:8],
		TeamID:    tm.teamID,
		TaskID:    taskID,
		Title:     title,
		DocType:   docType,
		Content:   content,
		Format:    format,
		Version:   1,
		Author:    author,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := tm.db.Exec(`
		INSERT INTO team_documents (id, team_id, task_id, title, doc_type, content, format, version, author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.TeamID, doc.TaskID, doc.Title, string(doc.DocType),
		doc.Content, doc.Format, doc.Version, doc.Author,
		doc.CreatedAt.Format(time.RFC3339), doc.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	tm.logActivity(ActivityDocumentCreated, author, fmt.Sprintf("Document created: %s", title), doc.ID)
	tm.logger.Info("document created", "doc_id", doc.ID, "title", title, "type", docType)
	return doc, nil
}

// GetDocument retrieves a document by ID.
func (tm *TeamMemory) GetDocument(docID string) (*TeamDocument, error) {
	var doc TeamDocument
	var taskID, filePath sql.NullString
	var createdAtStr, updatedAtStr string

	err := tm.db.QueryRow(`
		SELECT id, team_id, task_id, title, doc_type, content, format, file_path, version, author, created_at, updated_at
		FROM team_documents WHERE id = ? AND team_id = ?`,
		docID, tm.teamID,
	).Scan(
		&doc.ID, &doc.TeamID, &taskID, &doc.Title, &doc.DocType,
		&doc.Content, &doc.Format, &filePath, &doc.Version, &doc.Author,
		&createdAtStr, &updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}

	doc.TaskID = taskID.String
	doc.FilePath = filePath.String
	doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	doc.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	return &doc, nil
}

// UpdateDocument updates document content, incrementing version.
func (tm *TeamMemory) UpdateDocument(docID, content, updatedBy string) error {
	now := time.Now().Format(time.RFC3339)

	result, err := tm.db.Exec(`
		UPDATE team_documents SET content = ?, version = version + 1, updated_at = ? WHERE id = ? AND team_id = ?`,
		content, now, docID, tm.teamID,
	)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("document %s not found", docID)
	}

	tm.logActivity(ActivityDocumentUpdated, updatedBy, fmt.Sprintf("Document updated: %s", docID), docID)
	tm.logger.Info("document updated", "doc_id", docID, "by", updatedBy)
	return nil
}

// ListDocuments lists documents with optional filters.
func (tm *TeamMemory) ListDocuments(taskID string, docType DocumentType) ([]*TeamDocument, error) {
	query := `SELECT id, team_id, task_id, title, doc_type, content, format, file_path, version, author, created_at, updated_at
              FROM team_documents WHERE team_id = ?`
	args := []interface{}{tm.teamID}

	if taskID != "" {
		query += " AND task_id = ?"
		args = append(args, taskID)
	}
	if docType != "" {
		query += " AND doc_type = ?"
		args = append(args, string(docType))
	}
	query += " ORDER BY updated_at DESC LIMIT 50"

	rows, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []*TeamDocument
	for rows.Next() {
		var doc TeamDocument
		var taskID, filePath sql.NullString
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(
			&doc.ID, &doc.TeamID, &taskID, &doc.Title, &doc.DocType,
			&doc.Content, &doc.Format, &filePath, &doc.Version, &doc.Author,
			&createdAtStr, &updatedAtStr,
		); err != nil {
			continue
		}

		doc.TaskID = taskID.String
		doc.FilePath = filePath.String
		doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		doc.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		docs = append(docs, &doc)
	}

	return docs, nil
}

// DeleteDocument removes a document.
func (tm *TeamMemory) DeleteDocument(docID string) error {
	_, err := tm.db.Exec(`DELETE FROM team_documents WHERE id = ? AND team_id = ?`, docID, tm.teamID)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	tm.logger.Info("document deleted", "doc_id", docID)
	return nil
}

// ─── Thread Subscriptions ───

// SubscribeToThread subscribes an agent to a thread.
func (tm *TeamMemory) SubscribeToThread(threadID, agentID string, reason SubscriptionReason) error {
	now := time.Now().Format(time.RFC3339)
	subID := uuid.New().String()[:8]

	_, err := tm.db.Exec(`
		INSERT OR IGNORE INTO team_thread_subscriptions (id, team_id, thread_id, agent_id, subscribed_at, reason)
		VALUES (?, ?, ?, ?, ?, ?)`,
		subID, tm.teamID, threadID, agentID, now, string(reason),
	)
	if err != nil {
		return fmt.Errorf("subscribe to thread: %w", err)
	}

	tm.logger.Debug("agent subscribed to thread", "agent_id", agentID, "thread_id", threadID, "reason", reason)
	return nil
}

// UnsubscribeFromThread removes an agent's subscription.
func (tm *TeamMemory) UnsubscribeFromThread(threadID, agentID string) error {
	_, err := tm.db.Exec(`
		DELETE FROM team_thread_subscriptions WHERE team_id = ? AND thread_id = ? AND agent_id = ?`,
		tm.teamID, threadID, agentID,
	)
	if err != nil {
		return fmt.Errorf("unsubscribe from thread: %w", err)
	}
	tm.logger.Debug("agent unsubscribed from thread", "agent_id", agentID, "thread_id", threadID)
	return nil
}

// GetThreadSubscribers returns all agents subscribed to a thread.
func (tm *TeamMemory) GetThreadSubscribers(threadID string) ([]string, error) {
	rows, err := tm.db.Query(`
		SELECT agent_id FROM team_thread_subscriptions WHERE team_id = ? AND thread_id = ?`,
		tm.teamID, threadID,
	)
	if err != nil {
		return nil, fmt.Errorf("get thread subscribers: %w", err)
	}
	defer rows.Close()

	var subscribers []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			continue
		}
		subscribers = append(subscribers, agentID)
	}

	return subscribers, nil
}

// GetSubscribedThreads returns all threads an agent is subscribed to.
func (tm *TeamMemory) GetSubscribedThreads(agentID string) ([]string, error) {
	rows, err := tm.db.Query(`
		SELECT thread_id FROM team_thread_subscriptions WHERE team_id = ? AND agent_id = ?`,
		tm.teamID, agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get subscribed threads: %w", err)
	}
	defer rows.Close()

	var threads []string
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			continue
		}
		threads = append(threads, threadID)
	}

	return threads, nil
}

// NotifySubscribers adds pending messages for all thread subscribers.
func (tm *TeamMemory) NotifySubscribers(threadID, fromAgent, content string, excludeAgents []string) error {
	subscribers, err := tm.GetThreadSubscribers(threadID)
	if err != nil {
		return err
	}

	excludeMap := make(map[string]bool)
	for _, a := range excludeAgents {
		excludeMap[a] = true
	}

	for _, agentID := range subscribers {
		if excludeMap[agentID] {
			continue
		}

		// Add pending message
		tm.addPendingMessage(agentID, fromAgent, content, threadID)

		// Trigger immediate run via TeamManager if available
		if tm.teamManager != nil {
			notification := fmt.Sprintf("New activity in thread %s", threadID)
			go func() {
				// Use timeout to prevent goroutine leak on shutdown
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				tm.teamManager.SendToAgent(ctx, agentID, fromAgent, notification)
			}()
		}
	}

	return nil
}

// Helper
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DeleteAllData removes all data for a team (used when deleting a team).
func (tm *TeamMemory) DeleteAllData() error {
	// Delete all related data
	tables := []string{
		"team_tasks",
		"team_messages",
		"team_facts",
		"team_activities",
		"team_documents",
		"team_thread_subscriptions",
		"team_pending_messages",
		"agent_working_state",
	}

	for _, table := range tables {
		// Add team_id condition only for tables that have it
		var query string
		switch table {
		case "agent_working_state":
			// This table doesn't have team_id, need subquery
			query = fmt.Sprintf("DELETE FROM %s WHERE team_id = ?", table)
		default:
			query = fmt.Sprintf("DELETE FROM %s WHERE team_id = ?", table)
		}
		_, err := tm.db.Exec(query, tm.teamID)
		if err != nil {
			tm.logger.Warn("failed to delete from table during cleanup", "table", table, "error", err)
		}
	}

	tm.logger.Info("all team data deleted", "team_id", tm.teamID)
	return nil
}
