// Package copilot – session_persistence_sqlite.go implements session persistence
// backed by the central devclaw.db SQLite database. It is a drop-in replacement
// for the JSONL-based SessionPersistence.
package copilot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// SQLiteSessionPersistence stores session data in the devclaw.db tables:
// session_entries, session_meta, session_facts.
type SQLiteSessionPersistence struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLiteSessionPersistence creates a SQLite-backed session persistence.
// The tables must already exist (created by OpenDatabase).
func NewSQLiteSessionPersistence(db *sql.DB, logger *slog.Logger) *SQLiteSessionPersistence {
	if logger == nil {
		logger = slog.Default()
	}
	return &SQLiteSessionPersistence{db: db, logger: logger}
}

// SaveEntry appends a conversation entry for the given session.
func (p *SQLiteSessionPersistence) SaveEntry(sessionID string, entry ConversationEntry) error {
	_, err := p.db.Exec(`
		INSERT INTO session_entries (session_id, user_message, assistant_response, created_at, meta)
		VALUES (?, ?, ?, ?, '{}')`,
		sessionID,
		entry.UserMessage,
		entry.AssistantResponse,
		entry.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		p.logger.Error("failed to save session entry", "session", sessionID, "err", err)
		return fmt.Errorf("save session entry: %w", err)
	}
	return nil
}

// LoadSession reads all entries and facts for a session.
func (p *SQLiteSessionPersistence) LoadSession(sessionID string) ([]ConversationEntry, []string, error) {
	// Load entries.
	rows, err := p.db.Query(`
		SELECT user_message, assistant_response, created_at
		FROM session_entries
		WHERE session_id = ?
		ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("load session entries: %w", err)
	}
	defer rows.Close()

	var entries []ConversationEntry
	for rows.Next() {
		var (
			e         ConversationEntry
			createdAt string
		)
		if err := rows.Scan(&e.UserMessage, &e.AssistantResponse, &createdAt); err != nil {
			return nil, nil, fmt.Errorf("scan session entry: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate session entries: %w", err)
	}

	// Load facts.
	factRows, err := p.db.Query(`
		SELECT fact FROM session_facts
		WHERE session_id = ?
		ORDER BY id ASC`, sessionID)
	if err != nil {
		return entries, nil, fmt.Errorf("load session facts: %w", err)
	}
	defer factRows.Close()

	var facts []string
	for factRows.Next() {
		var fact string
		if err := factRows.Scan(&fact); err != nil {
			return entries, nil, fmt.Errorf("scan session fact: %w", err)
		}
		facts = append(facts, fact)
	}

	return entries, facts, factRows.Err()
}

// LoadAll scans all sessions from the database and returns SessionData for each.
func (p *SQLiteSessionPersistence) LoadAll() (map[string]*SessionData, error) {
	// Collect all unique session IDs from entries.
	rows, err := p.db.Query("SELECT DISTINCT session_id FROM session_entries")
	if err != nil {
		return nil, fmt.Errorf("list session ids: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids[id] = true
	}

	// Also include sessions that have meta but no entries.
	metaRows, err := p.db.Query("SELECT session_id FROM session_meta")
	if err == nil {
		defer metaRows.Close()
		for metaRows.Next() {
			var id string
			if err := metaRows.Scan(&id); err == nil {
				ids[id] = true
			}
		}
	}

	result := make(map[string]*SessionData)
	for id := range ids {
		entries, facts, err := p.LoadSession(id)
		if err != nil {
			p.logger.Warn("failed to load session, skipping", "id", id, "err", err)
			continue
		}

		channel, chatID, config, activeSkills := p.loadMeta(id)
		result[id] = &SessionData{
			ID:           id,
			Channel:      channel,
			ChatID:       chatID,
			History:      entries,
			Facts:        facts,
			Config:       config,
			ActiveSkills: activeSkills,
		}
	}

	return result, nil
}

// SaveFacts replaces all facts for the given session.
func (p *SQLiteSessionPersistence) SaveFacts(sessionID string, facts []string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing facts.
	if _, err := tx.Exec("DELETE FROM session_facts WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete old facts: %w", err)
	}

	// Insert new facts.
	now := time.Now().UTC().Format(time.RFC3339)
	for _, fact := range facts {
		if _, err := tx.Exec(
			"INSERT INTO session_facts (session_id, fact, created_at) VALUES (?, ?, ?)",
			sessionID, fact, now,
		); err != nil {
			return fmt.Errorf("insert fact: %w", err)
		}
	}

	return tx.Commit()
}

// SaveMeta persists session metadata (channel, chatID, config, activeSkills).
func (p *SQLiteSessionPersistence) SaveMeta(sessionID, channel, chatID string, config SessionConfig, activeSkills []string) error {
	configJSON, _ := json.Marshal(config)
	skillsJSON, _ := json.Marshal(activeSkills)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := p.db.Exec(`
		INSERT OR REPLACE INTO session_meta
			(session_id, channel, chat_id, config, active_skills, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, channel, chatID,
		string(configJSON), string(skillsJSON), now,
	)
	if err != nil {
		p.logger.Error("failed to save session meta", "session", sessionID, "err", err)
		return fmt.Errorf("save session meta: %w", err)
	}
	return nil
}

// DeleteSession removes all data for a session (entries, facts, meta).
func (p *SQLiteSessionPersistence) DeleteSession(sessionID string) error {
	for _, table := range []string{"session_entries", "session_facts", "session_meta"} {
		if _, err := p.db.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE session_id = ?", table), sessionID,
		); err != nil {
			p.logger.Warn("failed to delete from table", "table", table, "session", sessionID, "err", err)
		}
	}
	return nil
}

// Rotate keeps only the latest maxLines entries for a session.
func (p *SQLiteSessionPersistence) Rotate(sessionID string, maxLines int) error {
	// Count entries.
	var count int
	if err := p.db.QueryRow(
		"SELECT COUNT(*) FROM session_entries WHERE session_id = ?", sessionID,
	).Scan(&count); err != nil {
		return fmt.Errorf("count entries: %w", err)
	}

	if count <= maxLines {
		return nil // No rotation needed.
	}

	// Delete oldest entries, keeping only the latest maxLines.
	_, err := p.db.Exec(`
		DELETE FROM session_entries
		WHERE session_id = ? AND id NOT IN (
			SELECT id FROM session_entries
			WHERE session_id = ?
			ORDER BY id DESC
			LIMIT ?
		)`, sessionID, sessionID, maxLines)
	if err != nil {
		return fmt.Errorf("rotate session entries: %w", err)
	}

	removed := count - maxLines
	p.logger.Info("session rotated", "session", sessionID, "kept", maxLines, "removed", removed)
	return nil
}

// SaveCompaction appends a compaction summary entry for the session.
func (p *SQLiteSessionPersistence) SaveCompaction(sessionID string, entry CompactionEntry) error {
	metaJSON, _ := json.Marshal(map[string]interface{}{
		"type":            entry.Type,
		"summary":         entry.Summary,
		"compacted_at":    entry.CompactedAt.Format(time.RFC3339),
		"messages_before": entry.MessagesBefore,
		"messages_after":  entry.MessagesAfter,
	})

	_, err := p.db.Exec(`
		INSERT INTO session_entries (session_id, user_message, assistant_response, created_at, meta)
		VALUES (?, '', '', ?, ?)`,
		sessionID,
		entry.CompactedAt.UTC().Format(time.RFC3339),
		string(metaJSON),
	)
	if err != nil {
		p.logger.Error("failed to save compaction entry", "session", sessionID, "err", err)
		return fmt.Errorf("save compaction entry: %w", err)
	}
	return nil
}

// Close is a no-op; the shared *sql.DB is closed at the application level.
func (p *SQLiteSessionPersistence) Close() error {
	return nil
}

// loadMeta reads session metadata from the session_meta table.
func (p *SQLiteSessionPersistence) loadMeta(sessionID string) (channel, chatID string, config SessionConfig, activeSkills []string) {
	var configJSON, skillsJSON string
	err := p.db.QueryRow(`
		SELECT channel, chat_id, config, active_skills
		FROM session_meta WHERE session_id = ?`, sessionID,
	).Scan(&channel, &chatID, &configJSON, &skillsJSON)
	if err != nil {
		return "", "", SessionConfig{}, nil
	}
	_ = json.Unmarshal([]byte(configJSON), &config)
	_ = json.Unmarshal([]byte(skillsJSON), &activeSkills)
	return channel, chatID, config, activeSkills
}
