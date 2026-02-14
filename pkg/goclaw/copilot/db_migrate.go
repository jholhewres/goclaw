// Package copilot – db_migrate.go handles one-time migration of legacy
// JSON/JSONL/text data to the central goclaw.db SQLite database.
// After a successful migration, the old files are renamed to .bak.
package copilot

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
)

// MigrateToSQLite imports legacy JSON/JSONL data into the central database.
// It is safe to call multiple times: once the .bak file exists, migration
// is skipped for that component. Called once from assistant.Start().
func MigrateToSQLite(db *sql.DB, dataDir string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	migrateSchedulerJSON(db, dataDir, logger)
	migrateSessionJSONL(db, dataDir, logger)
	migrateAuditLog(db, dataDir, logger)
}

// ---------- Scheduler ----------

// migrateSchedulerJSON reads scheduler.json and inserts each job into the jobs table.
func migrateSchedulerJSON(db *sql.DB, dataDir string, logger *slog.Logger) {
	jsonPath := filepath.Join(dataDir, "scheduler.json")
	bakPath := jsonPath + ".migrated.bak"

	// Skip if already migrated or source doesn't exist.
	if _, err := os.Stat(bakPath); err == nil {
		return
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return // No file to migrate.
	}

	var jobs map[string]*scheduler.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		logger.Warn("migrate: failed to parse scheduler.json", "err", err)
		return
	}

	storage := scheduler.NewSQLiteJobStorage(db)
	migrated := 0
	for _, job := range jobs {
		if err := storage.Save(job); err != nil {
			logger.Warn("migrate: failed to save job", "id", job.ID, "err", err)
			continue
		}
		migrated++
	}

	// Rename original file.
	if err := os.Rename(jsonPath, bakPath); err != nil {
		logger.Warn("migrate: failed to rename scheduler.json", "err", err)
	}

	logger.Info("migrate: scheduler jobs imported", "count", migrated)
}

// ---------- Sessions ----------

// migrateSessionJSONL reads all .jsonl session files and imports them.
func migrateSessionJSONL(db *sql.DB, dataDir string, logger *slog.Logger) {
	sessDir := filepath.Join(dataDir, "sessions")
	markerPath := filepath.Join(sessDir, ".migrated")

	// Skip if already migrated.
	if _, err := os.Stat(markerPath); err == nil {
		return
	}
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return // No sessions directory.
	}

	persist := NewSQLiteSessionPersistence(db, logger)
	totalEntries := 0
	totalSessions := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".jsonl")
		if base == "" {
			continue
		}

		// Read JSONL entries.
		jsonlPath := filepath.Join(sessDir, e.Name())
		convEntries := readJSONLFile(jsonlPath, logger)

		// Read facts if they exist.
		factsPath := filepath.Join(sessDir, base+".facts.json")
		facts := readFactsFile(factsPath)

		// Read meta if it exists.
		metaPath := filepath.Join(sessDir, base+".meta.json")
		channel, chatID, config, activeSkills := readMetaFile(metaPath)

		// Determine session ID (use the base name as-is since older files
		// may have used phone-number-based IDs or hashed IDs).
		sessionID := base

		// Save meta.
		if channel != "" || chatID != "" {
			_ = persist.SaveMeta(sessionID, channel, chatID, config, activeSkills)
		}

		// Save entries.
		for _, ce := range convEntries {
			if err := persist.SaveEntry(sessionID, ce); err != nil {
				logger.Warn("migrate: failed to save session entry",
					"session", sessionID, "err", err)
			}
			totalEntries++
		}

		// Save facts.
		if len(facts) > 0 {
			_ = persist.SaveFacts(sessionID, facts)
		}

		totalSessions++
	}

	if totalSessions > 0 {
		// Write marker file.
		_ = os.WriteFile(markerPath, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
		logger.Info("migrate: sessions imported",
			"sessions", totalSessions,
			"entries", totalEntries,
		)
	}
}

// ---------- Audit log ----------

// migrateAuditLog reads the plain-text audit.log and imports it.
func migrateAuditLog(db *sql.DB, dataDir string, logger *slog.Logger) {
	auditPath := filepath.Join(dataDir, "audit.log")
	bakPath := auditPath + ".migrated.bak"

	// Skip if already migrated.
	if _, err := os.Stat(bakPath); err == nil {
		return
	}
	f, err := os.Open(auditPath)
	if err != nil {
		return // No audit log.
	}
	defer f.Close()

	auditLogger := NewSQLiteAuditLogger(db, logger)
	migrated := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Parse basic fields from the text format:
		// [2024-01-01 12:00:00] tool=bash caller=... level=owner allowed=true args=... result=...
		tool := extractField(line, "tool=")
		caller := extractField(line, "caller=")
		level := extractField(line, "level=")
		allowedStr := extractField(line, "allowed=")
		allowed := allowedStr == "true"

		if tool != "" {
			auditLogger.Log(tool, caller, level, allowed, "", "migrated from text log")
			migrated++
		}
	}

	// Rename original file.
	if migrated > 0 {
		if err := os.Rename(auditPath, bakPath); err != nil {
			logger.Warn("migrate: failed to rename audit.log", "err", err)
		}
		logger.Info("migrate: audit entries imported", "count", migrated)
	}
}

// ---------- Helpers ----------

func readJSONLFile(path string, logger *slog.Logger) []ConversationEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []ConversationEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var je struct {
			TS        string `json:"ts"`
			User      string `json:"user"`
			Assistant string `json:"assistant"`
		}
		if err := json.Unmarshal([]byte(line), &je); err != nil {
			logger.Warn("migrate: skip invalid jsonl line", "err", err)
			continue
		}
		ts, _ := time.Parse(time.RFC3339, je.TS)
		entries = append(entries, ConversationEntry{
			UserMessage:       je.User,
			AssistantResponse: je.Assistant,
			Timestamp:         ts,
		})
	}
	return entries
}

func readFactsFile(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var facts []string
	_ = json.Unmarshal(b, &facts)
	return facts
}

func readMetaFile(path string) (channel, chatID string, config SessionConfig, activeSkills []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", SessionConfig{}, nil
	}
	var mf struct {
		Channel      string        `json:"channel"`
		ChatID       string        `json:"chat_id"`
		Config       SessionConfig `json:"config"`
		ActiveSkills []string      `json:"active_skills"`
	}
	if json.Unmarshal(b, &mf) == nil {
		return mf.Channel, mf.ChatID, mf.Config, mf.ActiveSkills
	}
	return "", "", SessionConfig{}, nil
}

// extractField extracts a value from a log line like "key=value ".
func extractField(line, key string) string {
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	start := idx + len(key)
	end := strings.IndexByte(line[start:], ' ')
	if end < 0 {
		return line[start:]
	}
	return line[start : start+end]
}
