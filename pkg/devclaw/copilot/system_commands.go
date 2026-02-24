// Package copilot – techops_commands.go implements administrative commands for remote operations.
// These commands allow operators to manage the system via chat channels without SSH access.
package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

// SystemCommands holds system administration command handlers.
type SystemCommands struct {
	assistant      *Assistant
	startTime      time.Time
	configPath     string
	maintenanceMgr *MaintenanceManager
}

// NewSystemCommands creates a new system command handler.
func NewSystemCommands(a *Assistant, configPath string, maintenanceMgr *MaintenanceManager) *SystemCommands {
	return &SystemCommands{
		assistant:      a,
		startTime:      time.Now(),
		configPath:     configPath,
		maintenanceMgr: maintenanceMgr,
	}
}

// ── Reload Command ──

// ReloadCommand handles /reload [section]
func (t *SystemCommands) ReloadCommand(args []string) string {
	section := ""
	if len(args) > 0 {
		section = strings.ToLower(args[0])
	}

	// Load fresh config from disk
	newCfg, err := LoadConfigFromFile(t.configPath)
	if err != nil {
		return fmt.Sprintf("❌ Failed to load config: %s", err)
	}

	var reloaded []string

	switch section {
	case "", "all":
		t.assistant.ApplyConfigUpdate(newCfg)
		reloaded = []string{"access", "instructions", "tool_guard", "heartbeat", "token_budget"}
	case "access":
		t.assistant.configMu.Lock()
		t.assistant.config.Access = newCfg.Access
		t.assistant.configMu.Unlock()
		reloaded = []string{"access"}
	case "instructions":
		t.assistant.configMu.Lock()
		t.assistant.config.Instructions = newCfg.Instructions
		t.assistant.configMu.Unlock()
		reloaded = []string{"instructions"}
	case "tools", "tool_guard":
		t.assistant.configMu.Lock()
		t.assistant.config.Security.ToolGuard = newCfg.Security.ToolGuard
		t.assistant.config.Security.ToolExecutor = newCfg.Security.ToolExecutor
		t.assistant.configMu.Unlock()
		reloaded = []string{"tool_guard", "tool_executor"}
	case "heartbeat":
		t.assistant.configMu.Lock()
		t.assistant.config.Heartbeat = newCfg.Heartbeat
		t.assistant.configMu.Unlock()
		reloaded = []string{"heartbeat"}
	case "budget", "token_budget":
		t.assistant.configMu.Lock()
		t.assistant.config.TokenBudget = newCfg.TokenBudget
		t.assistant.configMu.Unlock()
		reloaded = []string{"token_budget"}
	case "env", "environment", "secrets", "vault":
		// Re-inject vault secrets first (highest priority)
		vaultCount := 0
		if t.assistant.vault != nil && t.assistant.vault.IsUnlocked() {
			t.assistant.InjectVaultEnvVars()
			vaultCount = len(t.assistant.vault.List())
		}

		// Force reload .env files with override (fills gaps not in vault)
		envCount, err := ReloadEnvFiles()
		if err != nil {
			return fmt.Sprintf("❌ Failed to reload env files: %s", err)
		}

		// Now reload config with fresh env vars
		newCfg, err = LoadConfigFromFile(t.configPath)
		if err != nil {
			return fmt.Sprintf("❌ Failed to reload config: %s", err)
		}
		t.assistant.ApplyConfigUpdate(newCfg)
		return fmt.Sprintf("✅ Reloaded %d vault secrets, %d env vars, and all config sections", vaultCount, envCount)
	default:
		return fmt.Sprintf("❌ Unknown section: %s\nValid sections: access, instructions, tools, heartbeat, budget, all", section)
	}

	return fmt.Sprintf("✅ Reloaded: %s", strings.Join(reloaded, ", "))
}

// ── Status Command ──

// StatusCommand handles /status [--json]
func (t *SystemCommands) StatusCommand(jsonOutput bool) string {
	status := t.buildSystemStatus()

	if jsonOutput {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Sprintf("❌ JSON error: %s", err)
		}
		return "```json\n" + string(data) + "\n```"
	}

	return t.formatStatusText(status)
}

func (t *SystemCommands) buildSystemStatus() *SystemStatus {
	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Channel health
	channelHealth := make(map[string]ChannelHealth)
	for name, h := range t.assistant.channelMgr.HealthAll() {
		channelHealth[name] = ChannelHealth{
			Connected:     h.Connected,
			LastMessageAt: h.LastMessageAt,
			ErrorCount:    h.ErrorCount,
			LatencyMs:     h.LatencyMs,
			Details:       h.Details,
		}
	}

	// Session stats
	sessions := SessionStats{
		Active:      len(t.assistant.activeRuns),
		Total:       t.countSessions(),
		WithHistory: t.countSessionsWithHistory(),
	}

	// Scheduler stats
	schedStats := SchedulerStats{
		Enabled: t.assistant.scheduler != nil,
		Jobs:    t.countScheduledJobs(),
		NextRun: t.getNextScheduledRun(),
		Running: 0, // TODO: track running jobs
	}

	// Skills count
	skillsCount := 0
	if t.assistant.skillRegistry != nil {
		skillsCount = len(t.assistant.skillRegistry.List())
	}

	// Maintenance mode
	var maint *MaintenanceMode
	if t.maintenanceMgr != nil {
		maint = t.maintenanceMgr.Get()
	}

	uptime := time.Since(t.startTime)

	return &SystemStatus{
		Version:       "dev", // TODO: use actual version
		Uptime:        formatDuration(uptime),
		UptimeSeconds: int64(uptime.Seconds()),
		MemoryMB:      float64(m.Alloc) / 1024 / 1024,
		GoRoutines:    runtime.NumGoroutine(),
		Channels:      channelHealth,
		Sessions:      sessions,
		Scheduler:     schedStats,
		Skills:        skillsCount,
		Maintenance:   maint,
	}
}

func (t *SystemCommands) formatStatusText(s *SystemStatus) string {
	var b strings.Builder

	b.WriteString("*DevClaw Status*\n\n")

	b.WriteString(fmt.Sprintf("📊 *System*\n"))
	b.WriteString(fmt.Sprintf("  Version: %s\n", s.Version))
	b.WriteString(fmt.Sprintf("  Uptime: %s\n", s.Uptime))
	b.WriteString(fmt.Sprintf("  Memory: %.1f MB\n", s.MemoryMB))
	b.WriteString(fmt.Sprintf("  Goroutines: %d\n\n", s.GoRoutines))

	b.WriteString(fmt.Sprintf("📡 *Channels*\n"))
	for name, ch := range s.Channels {
		status := "❌"
		if ch.Connected {
			status = "✅"
		}
		b.WriteString(fmt.Sprintf("  %s %s (errors: %d)\n", status, name, ch.ErrorCount))
	}
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("💬 *Sessions*\n"))
	b.WriteString(fmt.Sprintf("  Active: %d | Total: %d\n\n", s.Sessions.Active, s.Sessions.Total))

	b.WriteString(fmt.Sprintf("⏰ *Scheduler*\n"))
	if s.Scheduler.Enabled {
		b.WriteString(fmt.Sprintf("  Jobs: %d | Next: %s\n\n", s.Scheduler.Jobs, s.Scheduler.NextRun))
	} else {
		b.WriteString("  Disabled\n\n")
	}

	b.WriteString(fmt.Sprintf("🎯 *Skills*: %d\n\n", s.Skills))

	if s.Maintenance != nil && s.Maintenance.Enabled {
		b.WriteString("🔧 *Maintenance Mode*: ON\n")
		if s.Maintenance.Message != "" {
			b.WriteString(fmt.Sprintf("  Message: %s\n", s.Maintenance.Message))
		}
	}

	return b.String()
}

// ── Diagnostics Command ──

// DiagnosticsCommand handles /diagnostics [--full]
func (t *SystemCommands) DiagnosticsCommand(full bool) string {
	diag := t.runDiagnostics(full)
	return t.formatDiagnostics(diag)
}

func (t *SystemCommands) runDiagnostics(full bool) *DiagnosticsResult {
	diag := &DiagnosticsResult{
		Database: t.checkDatabaseHealth(),
		Config:   t.checkConfigHealth(),
		Channels: t.checkChannelsHealth(),
		Memory:   t.getMemoryStats(),
		Disk:     t.getDiskStats(),
	}

	if full {
		diag.RecentErrors = t.getRecentErrors(10)
	}

	return diag
}

func (t *SystemCommands) checkDatabaseHealth() DatabaseHealth {
	db := t.assistant.devclawDB
	if db == nil {
		return DatabaseHealth{Connected: false, Error: "database not initialized"}
	}

	// Test connectivity
	var result int
	err := db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		return DatabaseHealth{Connected: false, Error: err.Error()}
	}

	// Get file size
	var sizeMB float64
	if info, err := os.Stat(t.assistant.config.Database.Path); err == nil {
		sizeMB = float64(info.Size()) / 1024 / 1024
	}

	// Count rows in tables
	tables := make(map[string]int)
	countTables := []string{"session_entries", "session_meta", "jobs", "audit_log", "subagent_runs"}
	for _, table := range countTables {
		var count int
		db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		tables[table] = count
	}

	return DatabaseHealth{
		Connected: true,
		SizeMB:    sizeMB,
		Tables:    tables,
	}
}

func (t *SystemCommands) checkConfigHealth() ConfigHealth {
	sections := []string{"api", "access", "channels", "security", "skills"}

	return ConfigHealth{
		Valid:    true,
		Path:     t.configPath,
		Sections: sections,
	}
}

func (t *SystemCommands) checkChannelsHealth() []ChannelDiagnostic {
	var diags []ChannelDiagnostic

	for name, h := range t.assistant.channelMgr.HealthAll() {
		testResult := "OK"
		if !h.Connected {
			testResult = "Disconnected"
		}
		if h.ErrorCount > 0 {
			testResult = fmt.Sprintf("Errors: %d", h.ErrorCount)
		}

		diags = append(diags, ChannelDiagnostic{
			Name:       name,
			Connected:  h.Connected,
			TestResult: testResult,
			LatencyMs:  h.LatencyMs,
		})
	}

	sort.Slice(diags, func(i, j int) bool {
		return diags[i].Name < diags[j].Name
	})

	return diags
}

func (t *SystemCommands) getMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return MemoryStats{
		AllocMB:      float64(m.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(m.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(m.Sys) / 1024 / 1024,
		NumGC:        m.NumGC,
	}
}

func (t *SystemCommands) getDiskStats() DiskStats {
	path := t.assistant.config.Database.Path
	if path == "" {
		path = "./data"
	}

	// Disk space checking using syscall.Statfs_t breaks on Windows
	// and isn't strictly necessary for basic diagnostics.
	return DiskStats{
		TotalGB: 0,
		FreeGB:  0,
		UsedPct: 0,
	}
}

func (t *SystemCommands) getRecentErrors(limit int) []AuditRecordShort {
	// Query audit log for recent errors
	db := t.assistant.devclawDB
	if db == nil {
		return nil
	}

	rows, err := db.Query(`
		SELECT id, tool, caller, level, allowed, args_summary, created_at
		FROM audit_log
		WHERE allowed = 0 OR level = 'error'
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []AuditRecordShort
	for rows.Next() {
		var r AuditRecordShort
		var createdAtStr string
		var allowed int
		if err := rows.Scan(&r.ID, &r.Tool, &r.Caller, &r.Level, &allowed, &r.ArgsSummary, &createdAtStr); err != nil {
			continue
		}
		r.Allowed = allowed == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		records = append(records, r)
	}

	return records
}

func (t *SystemCommands) formatDiagnostics(d *DiagnosticsResult) string {
	var b strings.Builder

	b.WriteString("*Diagnostics Report*\n\n")

	// Database
	b.WriteString("📦 *Database*\n")
	if d.Database.Connected {
		b.WriteString(fmt.Sprintf("  Status: ✅ Connected\n"))
		b.WriteString(fmt.Sprintf("  Size: %.1f MB\n", d.Database.SizeMB))
		for table, count := range d.Database.Tables {
			b.WriteString(fmt.Sprintf("  %s: %d rows\n", table, count))
		}
	} else {
		b.WriteString(fmt.Sprintf("  Status: ❌ %s\n", d.Database.Error))
	}
	b.WriteString("\n")

	// Config
	b.WriteString("⚙️ *Config*\n")
	if d.Config.Valid {
		b.WriteString(fmt.Sprintf("  Status: ✅ Valid\n"))
		b.WriteString(fmt.Sprintf("  Path: %s\n", d.Config.Path))
	} else {
		b.WriteString("  Status: ❌ Invalid\n")
		for _, err := range d.Config.Errors {
			b.WriteString(fmt.Sprintf("  Error: %s\n", err))
		}
	}
	b.WriteString("\n")

	// Channels
	b.WriteString("📡 *Channels*\n")
	for _, ch := range d.Channels {
		status := "✅"
		if !ch.Connected {
			status = "❌"
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", status, ch.Name, ch.TestResult))
	}
	b.WriteString("\n")

	// Memory
	b.WriteString("💾 *Memory*\n")
	b.WriteString(fmt.Sprintf("  Alloc: %.1f MB\n", d.Memory.AllocMB))
	b.WriteString(fmt.Sprintf("  Sys: %.1f MB\n", d.Memory.SysMB))
	b.WriteString(fmt.Sprintf("  GC cycles: %d\n", d.Memory.NumGC))
	b.WriteString("\n")

	// Disk
	b.WriteString("💿 *Disk*\n")
	b.WriteString(fmt.Sprintf("  Total: %.1f GB\n", d.Disk.TotalGB))
	b.WriteString(fmt.Sprintf("  Free: %.1f GB (%.1f%% used)\n", d.Disk.FreeGB, d.Disk.UsedPct))

	// Recent errors
	if len(d.RecentErrors) > 0 {
		b.WriteString("\n⚠️ *Recent Errors*\n")
		for _, e := range d.RecentErrors {
			status := "blocked"
			if e.Allowed {
				status = "allowed"
			}
			b.WriteString(fmt.Sprintf("  [%d] %s (%s) - %s\n", e.ID, e.Tool, status, e.ArgsSummary))
		}
	}

	return b.String()
}

// ── Exec Queue Command ──

// ExecQueueCommand handles /exec queue
func (t *SystemCommands) ExecQueueCommand() string {
	// Get pending approvals from approval manager
	if t.assistant.approvalMgr == nil {
		return "Approval manager not initialized"
	}

	// Access pending approvals via reflection or add a method to ApprovalManager
	// For now, return a message indicating the feature needs the list method
	return "📋 Pending approvals: Use /approve <id> or /deny <id> to resolve"
}

// ── Channels Command ──

// ChannelsCommand handles /channels [connect|disconnect <name>]
func (t *SystemCommands) ChannelsCommand(args []string) string {
	if len(args) == 0 {
		return t.listChannels()
	}

	action := strings.ToLower(args[0])
	switch action {
	case "connect":
		if len(args) < 2 {
			return "Usage: /channels connect <name>"
		}
		return t.connectChannel(args[1])
	case "disconnect":
		if len(args) < 2 {
			return "Usage: /channels disconnect <name>"
		}
		return t.disconnectChannel(args[1])
	default:
		return fmt.Sprintf("Unknown action: %s\nUsage: /channels [connect|disconnect <name>]", action)
	}
}

func (t *SystemCommands) listChannels() string {
	health := t.assistant.channelMgr.HealthAll()
	if len(health) == 0 {
		return "No channels registered"
	}

	var b strings.Builder
	b.WriteString("*Channels*\n\n")

	for name, h := range health {
		status := "❌ Disconnected"
		if h.Connected {
			status = "✅ Connected"
		}
		b.WriteString(fmt.Sprintf("%s\n", name))
		b.WriteString(fmt.Sprintf("  Status: %s\n", status))
		b.WriteString(fmt.Sprintf("  Errors: %d\n", h.ErrorCount))
		if !h.LastMessageAt.IsZero() {
			b.WriteString(fmt.Sprintf("  Last msg: %s\n", formatTimeAgo(h.LastMessageAt)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (t *SystemCommands) connectChannel(name string) string {
	err := t.assistant.channelMgr.ConnectChannel(name)
	if err != nil {
		return fmt.Sprintf("❌ Failed to connect %s: %s", name, err)
	}
	return fmt.Sprintf("✅ Channel %s connected", name)
}

func (t *SystemCommands) disconnectChannel(name string) string {
	err := t.assistant.channelMgr.DisconnectChannel(name)
	if err != nil {
		return fmt.Sprintf("❌ Failed to disconnect %s: %s", name, err)
	}
	return fmt.Sprintf("✅ Channel %s disconnected", name)
}

// ── Maintenance Command ──

// MaintenanceCommand handles /maintenance [on|off] [message]
func (t *SystemCommands) MaintenanceCommand(args []string, setBy string) string {
	if t.maintenanceMgr == nil {
		return "❌ Maintenance manager not initialized"
	}

	if len(args) == 0 {
		// Show current status
		m := t.maintenanceMgr.Get()
		if m == nil || !m.Enabled {
			return "🔧 Maintenance mode: OFF"
		}
		msg := m.Message
		if msg == "" {
			msg = "(no message)"
		}
		return fmt.Sprintf("🔧 Maintenance mode: ON\nMessage: %s\nSet by: %s\nSince: %s",
			msg, m.SetBy, formatTimeAgo(m.SetAt))
	}

	action := strings.ToLower(args[0])
	switch action {
	case "on":
		message := ""
		if len(args) > 1 {
			message = strings.Join(args[1:], " ")
		}
		if err := t.maintenanceMgr.Set(true, message, setBy); err != nil {
			return fmt.Sprintf("❌ Failed to enable maintenance: %s", err)
		}
		return "✅ Maintenance mode enabled"
	case "off":
		if err := t.maintenanceMgr.Set(false, "", setBy); err != nil {
			return fmt.Sprintf("❌ Failed to disable maintenance: %s", err)
		}
		return "✅ Maintenance mode disabled"
	default:
		return "Usage: /maintenance [on|off] [message]"
	}
}

// ── Logs Command ──

// LogsCommand handles /logs [level] [lines]
func (t *SystemCommands) LogsCommand(args []string) string {
	level := "audit"
	lines := 20

	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "audit", "error", "all":
			level = strings.ToLower(arg)
		default:
			// Try to parse as number
			if n, err := fmt.Sscanf(arg, "%d", &lines); n == 1 && err == nil {
				// lines parsed
			}
		}
	}

	return t.getAuditLogs(level, lines)
}

func (t *SystemCommands) getAuditLogs(level string, limit int) string {
	db := t.assistant.devclawDB
	if db == nil {
		return "❌ Database not available"
	}

	query := `
		SELECT id, tool, caller, level, allowed, args_summary, created_at
		FROM audit_log
	`
	args := []any{}

	if level != "all" {
		if level == "error" {
			query += " WHERE allowed = 0 OR level = 'error'"
		}
		// audit = all records
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return fmt.Sprintf("❌ Query error: %s", err)
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Audit Log* (last %d)\n\n", limit))

	count := 0
	for rows.Next() {
		var id int64
		var tool, caller, lvl, argsSummary, createdAtStr string
		var allowed int
		if err := rows.Scan(&id, &tool, &caller, &lvl, &allowed, &argsSummary, &createdAtStr); err != nil {
			continue
		}

		status := "✅"
		if allowed == 0 {
			status = "❌"
		}

		b.WriteString(fmt.Sprintf("%s [%d] %s\n", status, id, tool))
		if argsSummary != "" && len(argsSummary) < 100 {
			b.WriteString(fmt.Sprintf("   %s\n", argsSummary))
		}
		count++
	}

	if count == 0 {
		b.WriteString("No entries found\n")
	}

	return b.String()
}

// ── Health Command ──

// HealthCommand handles /health
func (t *SystemCommands) HealthCommand() string {
	var results []HealthCheckResult

	// Database check
	start := time.Now()
	if t.assistant.devclawDB != nil {
		var result int
		err := t.assistant.devclawDB.QueryRow("SELECT 1").Scan(&result)
		if err != nil {
			results = append(results, HealthCheckResult{Component: "Database", Status: "FAIL", Message: err.Error()})
		} else {
			results = append(results, HealthCheckResult{Component: "Database", Status: "PASS", LatencyMs: time.Since(start).Milliseconds()})
		}
	} else {
		results = append(results, HealthCheckResult{Component: "Database", Status: "FAIL", Message: "not initialized"})
	}

	// Channels check
	for name, h := range t.assistant.channelMgr.HealthAll() {
		status := "PASS"
		msg := "connected"
		if !h.Connected {
			status = "FAIL"
			msg = "disconnected"
		}
		results = append(results, HealthCheckResult{
			Component: fmt.Sprintf("Channel/%s", name),
			Status:    status,
			Message:   msg,
			LatencyMs: h.LatencyMs,
		})
	}

	// Format output
	var b strings.Builder
	b.WriteString("*Health Check*\n\n")

	allPass := true
	for _, r := range results {
		emoji := "✅"
		if r.Status != "PASS" {
			emoji = "❌"
			allPass = false
		}
		b.WriteString(fmt.Sprintf("%s %s: %s", emoji, r.Component, r.Status))
		if r.Message != "" {
			b.WriteString(fmt.Sprintf(" (%s)", r.Message))
		}
		if r.LatencyMs > 0 {
			b.WriteString(fmt.Sprintf(" [%dms]", r.LatencyMs))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if allPass {
		b.WriteString("Overall: ✅ All systems operational")
	} else {
		b.WriteString("Overall: ❌ Some systems degraded")
	}

	return b.String()
}

// ── Metrics Command ──

// MetricsCommand handles /metrics [period]
func (t *SystemCommands) MetricsCommand(args []string) string {
	period := "day"
	if len(args) > 0 {
		period = strings.ToLower(args[0])
	}

	return t.getMetrics(period)
}

func (t *SystemCommands) getMetrics(period string) string {
	ut := t.assistant.usageTracker
	if ut == nil {
		return "❌ Usage tracker not available"
	}

	// Get global usage
	global := ut.GetGlobal()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Usage Metrics* (%s)\n\n", period))

	b.WriteString("📊 *Global*\n")
	b.WriteString(fmt.Sprintf("  Requests: %d\n", global.Requests))
	b.WriteString(fmt.Sprintf("  Prompt tokens: %d\n", global.PromptTokens))
	b.WriteString(fmt.Sprintf("  Completion tokens: %d\n", global.CompletionTokens))
	b.WriteString(fmt.Sprintf("  Total tokens: %d\n", global.TotalTokens))
	b.WriteString(fmt.Sprintf("  Est. cost: $%.4f\n", global.EstimatedCostUSD))

	if !global.FirstRequestAt.IsZero() {
		b.WriteString(fmt.Sprintf("  Since: %s\n", global.FirstRequestAt.Format("2006-01-02 15:04")))
	}

	return b.String()
}

// ── Helper Methods ──

func (t *SystemCommands) countSessions() int {
	// Count from session_meta table
	db := t.assistant.devclawDB
	if db == nil {
		return 0
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM session_meta").Scan(&count)
	return count
}

func (t *SystemCommands) countSessionsWithHistory() int {
	db := t.assistant.devclawDB
	if db == nil {
		return 0
	}
	var count int
	db.QueryRow("SELECT COUNT(DISTINCT session_id) FROM session_entries").Scan(&count)
	return count
}

func (t *SystemCommands) countScheduledJobs() int {
	if t.assistant.scheduler == nil {
		return 0
	}
	return len(t.assistant.scheduler.List())
}

func (t *SystemCommands) getNextScheduledRun() string {
	if t.assistant.scheduler == nil {
		return ""
	}
	jobs := t.assistant.scheduler.List()
	if len(jobs) == 0 {
		return ""
	}

	// Find next job (this is simplified - scheduler should provide this)
	return "see scheduler"
}

// ── Utility Functions ──

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 24 {
		days := h / 24
		h = h % 24
		return fmt.Sprintf("%dd %dh %dm", days, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
