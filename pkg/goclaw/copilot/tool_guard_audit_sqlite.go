// Package copilot – tool_guard_audit_sqlite.go provides a SQLite-backed audit
// logger for the ToolGuard. It writes tool execution records to the audit_log
// table in the central goclaw.db and auto-prunes entries older than 30 days.
package copilot

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SQLiteAuditLogger writes tool execution audit records to the goclaw.db
// audit_log table. It replaces the plain-text append-only file.
type SQLiteAuditLogger struct {
	db     *sql.DB
	logger *slog.Logger

	// pruneOnce ensures the auto-prune runs at most once per startup.
	pruneOnce sync.Once
}

// NewSQLiteAuditLogger creates an audit logger backed by SQLite.
func NewSQLiteAuditLogger(db *sql.DB, logger *slog.Logger) *SQLiteAuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	a := &SQLiteAuditLogger{db: db, logger: logger}
	// Auto-prune old entries in the background on first use.
	go a.autoPrune()
	return a
}

// Log records a tool execution in the audit_log table.
func (a *SQLiteAuditLogger) Log(toolName, caller, level string, allowed bool, argsSummary, resultSummary string) {
	allowedInt := 0
	if allowed {
		allowedInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// Truncate large summaries for storage efficiency.
	if len(argsSummary) > 500 {
		argsSummary = argsSummary[:500] + "...[truncated]"
	}
	if len(resultSummary) > 500 {
		resultSummary = resultSummary[:500] + "...[truncated]"
	}

	_, err := a.db.Exec(`
		INSERT INTO audit_log (tool, caller, level, allowed, args_summary, result_summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		toolName, caller, level, allowedInt, argsSummary, resultSummary, now,
	)
	if err != nil {
		a.logger.Warn("failed to write audit log", "tool", toolName, "err", err)
	}
}

// autoPrune deletes audit entries older than 30 days.
func (a *SQLiteAuditLogger) autoPrune() {
	cutoff := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	result, err := a.db.Exec("DELETE FROM audit_log WHERE created_at < ?", cutoff)
	if err != nil {
		a.logger.Warn("audit log prune failed", "err", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		a.logger.Info("audit log pruned", "removed", n)
	}
}

// Count returns the total number of audit log entries.
func (a *SQLiteAuditLogger) Count() int {
	var count int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM audit_log").Scan(&count)
	return count
}

// Recent returns the last N audit log entries as formatted strings.
func (a *SQLiteAuditLogger) Recent(n int) []string {
	rows, err := a.db.Query(`
		SELECT tool, caller, level, allowed, args_summary, result_summary, created_at
		FROM audit_log
		ORDER BY id DESC
		LIMIT ?`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []string
	for rows.Next() {
		var (
			tool, caller, level, argsSummary, resultSummary, createdAt string
			allowed                                                    int
		)
		if err := rows.Scan(&tool, &caller, &level, &allowed, &argsSummary, &resultSummary, &createdAt); err != nil {
			continue
		}
		allowedStr := "BLOCKED"
		if allowed != 0 {
			allowedStr = "OK"
		}
		entries = append(entries, fmt.Sprintf("[%s] tool=%s caller=%s level=%s %s args=%s result=%s",
			createdAt, tool, caller, level, allowedStr, argsSummary, resultSummary))
	}
	return entries
}

// Close is a no-op; the shared *sql.DB is closed at the application level.
func (a *SQLiteAuditLogger) Close() {}
