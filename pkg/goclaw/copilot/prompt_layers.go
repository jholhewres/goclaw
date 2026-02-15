// Package copilot – prompt_layers.go implements the layered system prompt
// (OpenClaw-style). Each layer has a priority and contributes to the final
// prompt that is sent to the LLM as the system message.
//
// Bootstrap files (SOUL.md, AGENTS.md, IDENTITY.md, USER.md, TOOLS.md) are
// loaded from the workspace root and injected as "Project Context".
// If SOUL.md is present, the agent is instructed to embody its persona.
package copilot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
)

// PromptLayer defines the priority of a prompt layer.
// Lower values = higher priority (never trimmed first on budget cuts).
type PromptLayer int

const (
	LayerCore         PromptLayer = 0  // Base identity and tooling.
	LayerSafety       PromptLayer = 5  // Safety rules.
	LayerIdentity     PromptLayer = 10 // Custom instructions.
	LayerThinking     PromptLayer = 12 // Extended thinking level hint (from /think).
	LayerBootstrap    PromptLayer = 15 // SOUL.md, AGENTS.md, etc.
	LayerBusiness     PromptLayer = 20 // User/workspace context.
	LayerSkills       PromptLayer = 40 // Active skill instructions.
	LayerMemory       PromptLayer = 50 // Long-term memory facts.
	LayerTemporal     PromptLayer = 60 // Date/time context.
	LayerConversation PromptLayer = 70 // Recent history summary.
	LayerRuntime      PromptLayer = 80 // Runtime info (final line).
)

// layerEntry represents a single prompt layer entry.
type layerEntry struct {
	layer   PromptLayer
	content string
}

// bootstrapCacheEntry holds a cached bootstrap file with a TTL to avoid
// re-reading from disk on every prompt compose.
type bootstrapCacheEntry struct {
	content   string
	hash      [32]byte  // SHA-256 of the on-disk content.
	cachedAt  time.Time // When the entry was last validated.
}

// bootstrapCacheTTL is how long a cached bootstrap entry is considered fresh.
// During this window, no disk I/O is performed.
const bootstrapCacheTTL = 30 * time.Second

// PromptComposer assembles the final system prompt from multiple layers.
type PromptComposer struct {
	config       *Config
	memoryStore  *memory.FileStore
	sqliteMemory *memory.SQLiteStore
	skillGetter  func(name string) (interface{ SystemPrompt() string }, bool)
	isSubagent   bool // When true, only AGENTS.md + TOOLS.md are loaded.

	// bootstrapCache caches bootstrap file contents to avoid re-reading from disk
	// on every prompt compose. Invalidated when file content changes (hash mismatch).
	bootstrapCacheMu sync.RWMutex
	bootstrapCache   map[string]*bootstrapCacheEntry
}

// NewPromptComposer creates a new prompt composer.
func NewPromptComposer(config *Config) *PromptComposer {
	return &PromptComposer{
		config:         config,
		bootstrapCache: make(map[string]*bootstrapCacheEntry),
	}
}

// SetSubagentMode restricts bootstrap loading to AGENTS.md + TOOLS.md only.
func (p *PromptComposer) SetSubagentMode(isSubagent bool) {
	p.isSubagent = isSubagent
}

// SetMemoryStore configures the file-based memory store for the prompt composer.
func (p *PromptComposer) SetMemoryStore(store *memory.FileStore) {
	p.memoryStore = store
}

// SetSQLiteMemory configures the SQLite memory store for hybrid search.
func (p *PromptComposer) SetSQLiteMemory(store *memory.SQLiteStore) {
	p.sqliteMemory = store
}

// SetSkillGetter sets the function used to retrieve skill system prompts.
func (p *PromptComposer) SetSkillGetter(getter func(name string) (interface{ SystemPrompt() string }, bool)) {
	p.skillGetter = getter
}

// Compose builds the complete system prompt for a session and user input.
// Heavy layers (bootstrap, memory, skills, conversation) are built concurrently
// to minimize prompt composition latency.
func (p *PromptComposer) Compose(session *Session, input string) string {
	// ── Fast layers (in-memory, no I/O) ──
	layers := make([]layerEntry, 0, 10)

	layers = append(layers, layerEntry{layer: LayerCore, content: p.buildCoreLayer()})
	layers = append(layers, layerEntry{layer: LayerSafety, content: p.buildSafetyLayer()})
	layers = append(layers, layerEntry{layer: LayerTemporal, content: p.buildTemporalLayer()})
	layers = append(layers, layerEntry{layer: LayerRuntime, content: p.buildRuntimeLayer()})

	if p.config.Instructions != "" {
		layers = append(layers, layerEntry{
			layer:   LayerIdentity,
			content: "## Custom Instructions\n\n" + p.config.Instructions,
		})
	}
	if thinkingPrompt := p.buildThinkingLayer(session); thinkingPrompt != "" {
		layers = append(layers, layerEntry{layer: LayerThinking, content: thinkingPrompt})
	}
	cfg := session.GetConfig()
	if cfg.BusinessContext != "" {
		layers = append(layers, layerEntry{
			layer:   LayerBusiness,
			content: "## Workspace Context\n\n" + cfg.BusinessContext,
		})
	}

	// ── Heavy layers (I/O, search) — run concurrently ──
	var (
		wg           sync.WaitGroup
		bootstrap    string
		skills       string
		memoryPrompt string
		history      string
	)

	wg.Add(4)
	go func() { defer wg.Done(); bootstrap = p.buildBootstrapLayer() }()
	go func() { defer wg.Done(); skills = p.buildSkillsLayer(session) }()
	go func() { defer wg.Done(); memoryPrompt = p.buildMemoryLayer(session, input) }()
	go func() { defer wg.Done(); history = p.buildConversationLayer(session) }()
	wg.Wait()

	if bootstrap != "" {
		layers = append(layers, layerEntry{layer: LayerBootstrap, content: bootstrap})
	}
	if skills != "" {
		layers = append(layers, layerEntry{layer: LayerSkills, content: skills})
	}
	if memoryPrompt != "" {
		layers = append(layers, layerEntry{layer: LayerMemory, content: memoryPrompt})
	}
	if history != "" {
		layers = append(layers, layerEntry{layer: LayerConversation, content: history})
	}

	return p.assembleLayers(layers)
}

// ComposeMinimal builds a lightweight system prompt for scheduled jobs and
// other fast-path scenarios. It includes only: Core identity, Safety,
// Temporal (date/time), and the user's custom instructions. It deliberately
// skips bootstrap files, memory search, skill instructions, and conversation
// history to minimize token count and latency.
func (p *PromptComposer) ComposeMinimal() string {
	layers := []layerEntry{
		{layer: LayerCore, content: p.buildCoreLayer()},
		{layer: LayerSafety, content: p.buildSafetyLayer()},
		{layer: LayerTemporal, content: p.buildTemporalLayer()},
	}

	if p.config.Instructions != "" {
		layers = append(layers, layerEntry{
			layer:   LayerIdentity,
			content: "## Custom Instructions\n\n" + p.config.Instructions,
		})
	}

	return p.assembleLayers(layers)
}

// ---------- Layer Builders ----------

// buildCoreLayer creates the base identity and tooling guidance.
func (p *PromptComposer) buildCoreLayer() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are %s, a personal assistant running inside GoClaw.\n\n", p.config.Name))

	b.WriteString("## Tooling\n\n")
	b.WriteString("You have access to tools for: file I/O (read, write, edit, search, glob), ")
	b.WriteString("bash execution (full system access), SSH/SCP (remote machines), ")
	b.WriteString("web search and fetch, memory (save/search/list), ")
	b.WriteString("scheduling (cron), and skill management (install/create/remove).\n\n")
	b.WriteString("Tool names are case-sensitive. Call tools exactly as listed.\n")
	b.WriteString("TOOLS.md does not control tool availability — it is your cheat sheet for environment-specific notes.\n\n")

	b.WriteString("## Tool Call Style\n\n")
	b.WriteString("Default: do not narrate routine, low-risk tool calls — just call the tool.\n")
	b.WriteString("Narrate only when it helps: multi-step work, complex problems, sensitive actions (deletions, deployments), or when the user explicitly asks.\n")
	b.WriteString("Keep narration brief and value-dense. Avoid repeating obvious steps.\n")
	b.WriteString("Use plain human language unless in a technical context.\n")

	b.WriteString("\n## Persistence & Tenacity\n\n")
	b.WriteString("This is your most important behavioral trait: you are TENACIOUS.\n\n")
	b.WriteString("- When a tool call fails, you MUST try at least 2-3 alternative approaches before even considering giving up.\n")
	b.WriteString("- When a tool requires approval and times out, DON'T tell the user the tool failed — explain what you need and try a different approach.\n")
	b.WriteString("- NEVER present the user with raw commands to execute manually. You ARE the executor — do it yourself.\n")
	b.WriteString("- NEVER say 'I cannot do X' unless you have literally exhausted every tool available to you.\n")
	b.WriteString("- Approval-pending tools run asynchronously — the result will be sent to the user when ready. Move on to the next step.\n")
	b.WriteString("- Recoverable errors (file not found, missing parameter, invalid path) are HINTS — fix the input and retry.\n")
	b.WriteString("- Only ask the user a question when you need genuinely new information, not when you're stuck on a technical problem.\n\n")

	b.WriteString("## Coding Strategy\n\n")
	b.WriteString("For ANY coding task (creating projects, writing components, editing code, fixing bugs, refactoring):\n\n")
	b.WriteString("**ALWAYS use `claude-code_execute`** — it is your primary coding tool. It handles:\n")
	b.WriteString("- Multi-file creation and editing with full context\n")
	b.WriteString("- Error detection and auto-correction\n")
	b.WriteString("- Package installation, build, and test\n")
	b.WriteString("- Git operations (commit, push, PR)\n\n")
	b.WriteString("**DO NOT** write code inline using `write_file` or generate large code blocks in your response.\n")
	b.WriteString("This wastes tokens and is error-prone. Instead, describe what you want to `claude-code_execute` and let it handle the implementation.\n\n")
	b.WriteString("Example: instead of generating 200 lines of React and calling write_file, do:\n")
	b.WriteString("```\nclaude-code_execute(prompt: \"Create a modern React landing page for rogadx.com with...\")\n```\n\n")
	b.WriteString("Use `bash` only for quick one-liners (ls, mkdir, cat, curl). Use `write_file`/`edit_file` only for small config files (<20 lines).\n\n")

	b.WriteString("## Efficiency\n\n")
	b.WriteString("The Project Context section below already contains SOUL.md, AGENTS.md, IDENTITY.md, USER.md, TOOLS.md, and MEMORY.md contents. ")
	b.WriteString("Do NOT read these files with read_file \u2014 they are already in your system prompt.\n")
	b.WriteString("For simple questions or greetings, respond directly without using any tools.\n")
	b.WriteString("Minimize tool calls: only use tools when you actually need external data or to perform an action.\n")

	return b.String()
}

// buildSafetyLayer creates the safety guardrails section.
func (p *PromptComposer) buildSafetyLayer() string {
	return `## Safety

You have no independent goals: do not pursue self-preservation, replication, resource acquisition, or power-seeking. Avoid long-term plans beyond the user's request.

Prioritize safety and human oversight over task completion. If instructions conflict, pause and ask. Comply with stop/pause/audit requests and never bypass safeguards.

Do not manipulate or persuade anyone to expand access or disable safeguards. Do not copy yourself or change system prompts, safety rules, or tool policies unless explicitly requested by the owner.

When using destructive tools (rm, drop, deploy): confirm with the user first unless they've explicitly pre-approved the action.

File operations: prefer reversible actions. Use trash over rm. Create backups before major changes.

SSH/remote: only connect to known hosts. Don't store passwords in plaintext. Use the vault for secrets.`
}

// buildThinkingLayer adds extended-thinking guidance based on session /think level.
func (p *PromptComposer) buildThinkingLayer(session *Session) string {
	level := session.GetThinkingLevel()
	if level == "" || level == "off" {
		return ""
	}
	instructions := map[string]string{
		"low":    "Think step-by-step when the task is complex. Keep reasoning brief for simple tasks.",
		"medium": "Think through problems systematically. Show your reasoning for non-trivial tasks.",
		"high":   "Use extended thinking: reason carefully before answering, consider alternatives, then respond. Favor depth over speed.",
	}
	if instr, ok := instructions[level]; ok {
		return "## Thinking Mode\n\n" + instr
	}
	return ""
}

// buildBootstrapLayer loads bootstrap files from the workspace root.
// Uses an in-memory cache with hash-based invalidation to avoid repeated disk reads.
// In subagent mode, only AGENTS.md and TOOLS.md are loaded.
func (p *PromptComposer) buildBootstrapLayer() string {
	// Full list of bootstrap files.
	allBootstrapFiles := []struct {
		Path    string
		Section string
	}{
		{"SOUL.md", "SOUL.md"},
		{"AGENTS.md", "AGENTS.md"},
		{"IDENTITY.md", "IDENTITY.md"},
		{"USER.md", "USER.md"},
		{"TOOLS.md", "TOOLS.md"},
		{"MEMORY.md", "MEMORY.md"},
	}

	// Subagent filter: only load AGENTS.md + TOOLS.md (like OpenClaw).
	var bootstrapFiles []struct {
		Path    string
		Section string
	}
	if p.isSubagent {
		for _, bf := range allBootstrapFiles {
			if bf.Path == "AGENTS.md" || bf.Path == "TOOLS.md" {
				bootstrapFiles = append(bootstrapFiles, bf)
			}
		}
	} else {
		bootstrapFiles = allBootstrapFiles
	}

	// Search directories: workspace dir, current dir, configs/.
	searchDirs := []string{"."}
	if p.config.Heartbeat.WorkspaceDir != "" && p.config.Heartbeat.WorkspaceDir != "." {
		searchDirs = append([]string{p.config.Heartbeat.WorkspaceDir}, searchDirs...)
	}
	searchDirs = append(searchDirs, "configs")

	var files []struct {
		path    string
		content string
	}
	hasSoul := false

	for _, bf := range bootstrapFiles {
		text := p.loadBootstrapFileCached(bf.Path, searchDirs)
		if text == "" {
			continue
		}

		files = append(files, struct {
			path    string
			content string
		}{bf.Section, text})

		if bf.Path == "SOUL.md" {
			hasSoul = true
		}
	}

	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Project Context\n\n")
	b.WriteString("The following project context files have been loaded:\n\n")

	if hasSoul {
		b.WriteString("If SOUL.md is present, embody its persona and tone. ")
		b.WriteString("Avoid stiff, generic replies; follow its guidance unless higher-priority instructions override it.\n\n")
	}

	for _, f := range files {
		b.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", f.path, f.content))
	}

	// Apply bootstrapMaxChars limit.
	result := b.String()
	maxChars := p.config.TokenBudget.BootstrapMaxChars
	if maxChars <= 0 {
		maxChars = 20000 // default 20K chars (~5K tokens)
	}
	if len(result) > maxChars {
		result = result[:maxChars] + "\n\n... [bootstrap truncated at limit]"
	}

	return result
}

// loadBootstrapFileCached loads a bootstrap file with TTL-based caching.
// Returns the trimmed content, or "" if the file doesn't exist or is empty.
// Within the TTL window (30s), returns cached content with zero disk I/O.
// After TTL expires, re-reads the file and invalidates if content changed.
func (p *PromptComposer) loadBootstrapFileCached(filename string, searchDirs []string) string {
	// Fast path: check if cache is still fresh (no disk I/O).
	p.bootstrapCacheMu.RLock()
	cached, ok := p.bootstrapCache[filename]
	p.bootstrapCacheMu.RUnlock()

	if ok && time.Since(cached.cachedAt) < bootstrapCacheTTL {
		return cached.content
	}

	// TTL expired or cache miss: read from disk.
	var content []byte
	var err error
	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, filename)
		content, err = os.ReadFile(candidate)
		if err == nil {
			break
		}
	}
	if err != nil || len(strings.TrimSpace(string(content))) == 0 {
		// File not found or empty: cache empty result to avoid repeated lookups.
		p.bootstrapCacheMu.Lock()
		p.bootstrapCache[filename] = &bootstrapCacheEntry{
			content:  "",
			cachedAt: time.Now(),
		}
		p.bootstrapCacheMu.Unlock()
		return ""
	}

	hash := sha256.Sum256(content)

	// If hash hasn't changed, refresh TTL and return cached content.
	if ok && cached.hash == hash {
		p.bootstrapCacheMu.Lock()
		cached.cachedAt = time.Now()
		p.bootstrapCacheMu.Unlock()
		return cached.content
	}

	// Content changed or new file: parse and cache.
	text := strings.TrimSpace(string(content))
	if len(text) > 20000 {
		text = text[:20000] + "\n\n... [truncated at 20KB]"
	}

	p.bootstrapCacheMu.Lock()
	p.bootstrapCache[filename] = &bootstrapCacheEntry{
		content:  text,
		hash:     hash,
		cachedAt: time.Now(),
	}
	p.bootstrapCacheMu.Unlock()

	return text
}

// buildSkillsLayer creates instructions from active skills.
func (p *PromptComposer) buildSkillsLayer(session *Session) string {
	activeSkills := session.GetActiveSkills()
	if len(activeSkills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Skills\n\n")
	b.WriteString("You have specialized skills available. Each skill provides tools and context.\n\n")

	for _, skillName := range activeSkills {
		b.WriteString(fmt.Sprintf("### %s\n", skillName))

		if p.skillGetter != nil {
			if skill, ok := p.skillGetter(skillName); ok {
				sp := skill.SystemPrompt()
				if sp != "" {
					b.WriteString(sp)
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// buildMemoryLayer creates the memory context section.
// Uses hybrid search (vector + BM25) when SQLite memory is available,
// otherwise falls back to substring matching on the file store.
func (p *PromptComposer) buildMemoryLayer(session *Session, input string) string {
	var parts []string

	// Try hybrid search first (SQLite with FTS5 + vector).
	// Use a tight timeout to avoid blocking prompt composition.
	// 500ms is enough for local SQLite FTS5; the old 2s was too generous.
	if p.sqliteMemory != nil && input != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		searchCfg := p.config.Memory.Search
		maxResults := searchCfg.MaxResults
		if maxResults <= 0 {
			maxResults = 6
		}

		results, err := p.sqliteMemory.HybridSearch(
			ctx, input, maxResults, searchCfg.MinScore,
			searchCfg.HybridWeightVector, searchCfg.HybridWeightBM25,
		)
		if err == nil && len(results) > 0 {
			var b strings.Builder
			b.WriteString("## Memory Recall\n\nRelevant facts from long-term memory (semantic search):\n\n")
			for _, r := range results {
				text := r.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				b.WriteString(fmt.Sprintf("- [%s] %s\n", r.FileID, text))
			}
			parts = append(parts, b.String())
		}
	}

	// Fallback: file-based substring search.
	if len(parts) == 0 && p.memoryStore != nil {
		facts := p.memoryStore.RecentFacts(15, input)
		if facts != "" {
			parts = append(parts, "## Memory Recall\n\nRelevant facts from long-term memory:\n\n"+facts)
		}
	}

	// Session-level facts.
	sessionFacts := session.GetFacts()
	if len(sessionFacts) > 0 {
		var b strings.Builder
		b.WriteString("## Session Context\n\n")
		for _, fact := range sessionFacts {
			b.WriteString(fmt.Sprintf("- %s\n", fact))
		}
		parts = append(parts, b.String())
	}

	return strings.Join(parts, "\n")
}

// buildTemporalLayer adds date/time context.
func (p *PromptComposer) buildTemporalLayer() string {
	loc, err := time.LoadLocation(p.config.Timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)

	return fmt.Sprintf("## Current Date & Time\n\n%s\nTimezone: %s\nDay: %s",
		now.Format("2006-01-02 15:04:05"),
		p.config.Timezone,
		now.Format("Monday"),
	)
}

// buildConversationLayer creates a summary of recent history, using a
// token-aware sliding window to stay within the history token budget.
func (p *PromptComposer) buildConversationLayer(session *Session) string {
	// Determine how many entries to request initially.
	maxEntries := p.config.Memory.MaxMessages
	if maxEntries <= 0 {
		maxEntries = 100
	}
	// Only include the most recent portion for the prompt. The conversation
	// layer is a summary that goes into the system prompt; the actual recent
	// exchanges are passed separately as conversation history to the LLM.
	// Keeping this small reduces prompt tokens and speeds up composition.
	fetchEntries := maxEntries
	if fetchEntries > 15 {
		fetchEntries = 15
	}

	history := session.RecentHistory(fetchEntries)
	if len(history) == 0 {
		return ""
	}

	// Token budget for conversation history layer.
	historyBudget := p.config.TokenBudget.History
	if historyBudget <= 0 {
		historyBudget = 8000
	}

	// Build from most recent backwards, stopping when we hit the budget.
	type formattedEntry struct {
		text   string
		tokens int
	}
	var entries []formattedEntry
	totalTokens := 0

	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]

		// Truncate very long messages individually.
		userMsg := entry.UserMessage
		if len(userMsg) > 2000 {
			userMsg = userMsg[:2000] + "..."
		}
		assistMsg := entry.AssistantResponse
		if len(assistMsg) > 4000 {
			assistMsg = assistMsg[:4000] + "..."
		}

		text := fmt.Sprintf("**User:** %s\n**Assistant:** %s\n", userMsg, assistMsg)
		tokens := estimateTokens(text)

		// Stop adding if we'd exceed the budget.
		if totalTokens+tokens > historyBudget && len(entries) > 0 {
			break
		}

		entries = append(entries, formattedEntry{text: text, tokens: tokens})
		totalTokens += tokens
	}

	if len(entries) == 0 {
		return ""
	}

	// Reverse to chronological order (we built backwards).
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	var b strings.Builder
	b.WriteString("## Recent Conversation\n\n")

	// If we had to skip older entries, note it.
	if len(entries) < len(history) {
		b.WriteString(fmt.Sprintf("_(%d older messages omitted to fit token budget)_\n\n",
			len(history)-len(entries)))
	}

	for _, e := range entries {
		b.WriteString(e.text)
		b.WriteString("\n")
	}

	return b.String()
}

// buildRuntimeLayer creates the runtime info line (last in prompt).
func (p *PromptComposer) buildRuntimeLayer() string {
	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()

	return fmt.Sprintf("---\nRuntime: agent=%s | model=%s | os=%s/%s | host=%s | cwd=%s | lang=%s",
		p.config.Name,
		p.config.Model,
		runtime.GOOS,
		runtime.GOARCH,
		hostname,
		cwd,
		p.config.Language,
	)
}

// estimateTokens approximates the token count for a string.
// Uses a rough heuristic: ~4 chars per token for English/code, ~2 for CJK.
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	// Rough: 1 token ≈ 4 chars (conservative).
	return (len(s) + 3) / 4
}

// assembleLayers combines all layers in priority order, trimming lower-priority
// layers if the total exceeds the configured token budget.
func (p *PromptComposer) assembleLayers(layers []layerEntry) string {
	// Sort by priority (lower = higher priority = kept first).
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].layer < layers[j].layer
	})

	budget := p.config.TokenBudget.Total
	if budget <= 0 {
		budget = 128000 // safe default
	}

	// System prompt should use at most ~40% of the total budget.
	// The rest is for conversation messages and tool results.
	systemBudget := budget * 40 / 100

	// Per-layer budgets (soft limits): use config if > 0, else proportional.
	layerBudgets := map[PromptLayer]int{
		LayerCore:         p.config.TokenBudget.System,
		LayerSafety:       500,  // safety is short and critical
		LayerIdentity:     1000, // custom instructions
		LayerThinking:     200,  // thinking hint
		LayerBootstrap:    4000, // bootstrap files
		LayerBusiness:     1000, // workspace context
		LayerSkills:       p.config.TokenBudget.Skills,
		LayerMemory:       p.config.TokenBudget.Memory,
		LayerTemporal:     200,  // timestamp
		LayerConversation: p.config.TokenBudget.History,
		LayerRuntime:      200,  // runtime line
	}

	// Phase 1: include all layers, tracking total.
	type measured struct {
		entry  layerEntry
		tokens int
	}
	var entries []measured
	totalTokens := 0

	for _, l := range layers {
		if l.content == "" {
			continue
		}
		tokens := estimateTokens(l.content)
		entries = append(entries, measured{entry: l, tokens: tokens})
		totalTokens += tokens
	}

	// Phase 2: if within budget, return as-is.
	if totalTokens <= systemBudget {
		var parts []string
		for _, m := range entries {
			parts = append(parts, m.entry.content)
		}
		return strings.Join(parts, "\n\n")
	}

	// Phase 3: trim from lowest priority (highest layer number) first.
	// Layers with priority < 20 (Core, Safety, Identity, Thinking) are never trimmed.
	for i := len(entries) - 1; i >= 0 && totalTokens > systemBudget; i-- {
		m := entries[i]
		if m.entry.layer < LayerBusiness {
			continue // never trim core layers
		}

		// Check per-layer budget.
		maxTokens := layerBudgets[m.entry.layer]
		if maxTokens <= 0 {
			maxTokens = 2000 // default soft limit
		}

		if m.tokens > maxTokens {
			// Trim content to fit layer budget.
			maxChars := maxTokens * 4 // inverse of estimateTokens
			if maxChars < len(m.entry.content) {
				trimmed := m.entry.content[:maxChars] + "\n\n... [trimmed to fit token budget]"
				saved := m.tokens - estimateTokens(trimmed)
				entries[i].entry.content = trimmed
				entries[i].tokens = estimateTokens(trimmed)
				totalTokens -= saved
			}
		}

		// If still over budget, drop this layer entirely.
		if totalTokens > systemBudget && m.entry.layer >= LayerMemory {
			totalTokens -= entries[i].tokens
			entries[i].entry.content = ""
			entries[i].tokens = 0
		}
	}

	var parts []string
	for _, m := range entries {
		if m.entry.content != "" {
			parts = append(parts, m.entry.content)
		}
	}

	return strings.Join(parts, "\n\n")
}
