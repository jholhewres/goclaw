// Package copilot – prompt_layers.go implements the layered system prompt
// Each layer has a priority and contributes to the final
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

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// PromptLayer defines the priority of a prompt layer.
// Lower values = higher priority (never trimmed first on budget cuts).
type PromptLayer int

const (
	LayerCore           PromptLayer = 0  // Base identity and tooling.
	LayerSafety         PromptLayer = 5  // Safety rules.
	LayerIdentity       PromptLayer = 10 // Custom instructions.
	LayerThinking       PromptLayer = 12 // Extended thinking level hint (from /think).
	LayerBootstrap      PromptLayer = 15 // SOUL.md, AGENTS.md, etc.
	LayerBuiltinSkills  PromptLayer = 18 // Built-in tool guides (memory, teams, etc.)
	LayerBusiness       PromptLayer = 20 // User/workspace context.
	LayerProjectContext PromptLayer = 25 // Auto-discovered project context.
	LayerSkills         PromptLayer = 40 // Active skill instructions.
	LayerMemory         PromptLayer = 50 // Long-term memory facts.
	LayerTemporal       PromptLayer = 60 // Date/time context.
	LayerConversation   PromptLayer = 70 // Recent history summary.
	LayerRuntime        PromptLayer = 80 // Runtime info (final line).
)

// PromptMode controls which prompt layers are included in the final prompt.
// Used to reduce token usage for subagents and specialized contexts.
type PromptMode string

const (
	// PromptModeFull includes all layers (default for main agent).
	PromptModeFull PromptMode = "full"

	// PromptModeMinimal omits skills, memory, heartbeats (for subagents).
	PromptModeMinimal PromptMode = "minimal"

	// PromptModeNone includes only core identity (for simple tasks).
	PromptModeNone PromptMode = "none"
)

// layerEntry represents a single prompt layer entry.
type layerEntry struct {
	layer   PromptLayer
	content string
}

// bootstrapCacheEntry holds a cached bootstrap file with a TTL to avoid
// re-reading from disk on every prompt compose.
type bootstrapCacheEntry struct {
	content  string
	hash     [32]byte  // SHA-256 of the on-disk content.
	cachedAt time.Time // When the entry was last validated.
}

// bootstrapCacheTTL is how long a cached bootstrap entry is considered fresh.
// During this window, no disk I/O is performed.
const bootstrapCacheTTL = 30 * time.Second

// promptLayerCache holds a cached prompt layer result with TTL.
type promptLayerCache struct {
	content  string
	cachedAt time.Time
}

// promptLayerCacheTTL is how long memory and skills layers are considered fresh.
// Within this window, the cached result is used without re-running the search.
const promptLayerCacheTTL = 60 * time.Second

// PromptComposer assembles the final system prompt from multiple layers.
type PromptComposer struct {
	config        *Config
	memoryStore   *memory.FileStore
	sqliteMemory  *memory.SQLiteStore
	skillGetter   func(name string) (interface{ SystemPrompt() string }, bool)
	skillLister   func() []SkillInfo // Returns all available skills with name, description, tools
	builtinSkills *BuiltinSkills
	toolExecutor  *ToolExecutor // For dynamic tool list generation
	isSubagent    bool // When true, only AGENTS.md + TOOLS.md are loaded.

	// bootstrapCache caches bootstrap file contents to avoid re-reading from disk
	// on every prompt compose. Invalidated when file content changes (hash mismatch).
	bootstrapCacheMu sync.RWMutex
	bootstrapCache   map[string]*bootstrapCacheEntry

	// layerCache caches memory and skills layers per session to avoid blocking
	// prompt composition on I/O-heavy operations. Key: "sessionID:layerType".
	layerCacheMu sync.RWMutex
	layerCache   map[string]*promptLayerCache
}

// SkillInfo holds basic skill information for the Skill Discovery XML.
type SkillInfo struct {
	Name        string
	Description string
	Tools       []string
}

// NewPromptComposer creates a new prompt composer.
func NewPromptComposer(config *Config) *PromptComposer {
	return &PromptComposer{
		config:         config,
		bootstrapCache: make(map[string]*bootstrapCacheEntry),
		layerCache:     make(map[string]*promptLayerCache),
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

// SetSkillLister sets the function used to list all available skills.
func (p *PromptComposer) SetSkillLister(lister func() []SkillInfo) {
	p.skillLister = lister
}

// SetBuiltinSkills sets the built-in skills for the prompt composer.
func (p *PromptComposer) SetBuiltinSkills(skills *BuiltinSkills) {
	p.builtinSkills = skills
}

// SetToolExecutor sets the tool executor for dynamic tool list generation.
func (p *PromptComposer) SetToolExecutor(executor *ToolExecutor) {
	p.toolExecutor = executor
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
	if projectContext := p.buildProjectContextLayer(); projectContext != "" {
		layers = append(layers, layerEntry{layer: LayerProjectContext, content: projectContext})
	}

	// ── Heavy layers (I/O, search) ──
	// Critical layers (bootstrap + history) are loaded synchronously because
	// they are always needed and typically fast (bootstrap is cached, history
	// is in-memory). Optional layers (memory + skills) use session-level
	// caching: if a fresh result exists, it's used immediately without I/O.
	// Stale results are refreshed in background for the next prompt.
	var (
		wg        sync.WaitGroup
		bootstrap string
		history   string
	)

	wg.Add(2)
	go func() { defer wg.Done(); bootstrap = p.buildBootstrapLayer() }()
	go func() { defer wg.Done(); history = p.buildConversationLayer(session) }()

	// Memory and skills: use cached versions to avoid blocking.
	memoryPrompt := p.getCachedLayer(session.ID, "memory")
	skills := p.getCachedLayer(session.ID, "skills")

	// If cache is stale or empty, refresh in background (non-blocking).
	// The current prompt uses whatever is cached; the NEXT prompt benefits.
	go p.refreshLayerCache(session, input)

	wg.Wait()

	if bootstrap != "" {
		layers = append(layers, layerEntry{layer: LayerBootstrap, content: bootstrap})
	}
	if builtinSkills := p.buildBuiltinSkillsLayer(); builtinSkills != "" {
		layers = append(layers, layerEntry{layer: LayerBuiltinSkills, content: builtinSkills})
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

// ComposeWithMode assembles the system prompt using the specified mode.
// Use PromptModeFull for the main agent, PromptModeMinimal for subagents,
// and PromptModeNone for simple tasks requiring only core identity.
func (p *PromptComposer) ComposeWithMode(session *Session, input string, mode PromptMode) string {
	// Start with core layers (always included)
	layers := []layerEntry{
		{layer: LayerCore, content: p.buildCoreLayer()},
		{layer: LayerSafety, content: p.buildSafetyLayer()},
		{layer: LayerTemporal, content: p.buildTemporalLayer()},
		{layer: LayerRuntime, content: p.buildRuntimeLayer()},
	}

	// Add additional layers based on mode
	switch mode {
	case PromptModeFull:
		// Full mode: include all layers
		return p.Compose(session, input)

	case PromptModeMinimal:
		// Minimal mode: omit heavy/optional layers
		// Include: Core, Safety, Temporal, Runtime, Identity, Bootstrap, Business
		if p.config.Instructions != "" {
			layers = append(layers, layerEntry{
				layer:   LayerIdentity,
				content: "## Custom Instructions\n\n" + p.config.Instructions,
			})
		}
		// Include bootstrap but not full skills/memory
		if bootstrap := p.buildBootstrapLayer(); bootstrap != "" {
			layers = append(layers, layerEntry{layer: LayerBootstrap, content: bootstrap})
		}
		// Include business context if available
		cfg := session.GetConfig()
		if cfg.BusinessContext != "" {
			layers = append(layers, layerEntry{
				layer:   LayerBusiness,
				content: "## Workspace Context\n\n" + cfg.BusinessContext,
			})
		}
		// Minimal mode: skip skills, memory, project context, conversation history

	case PromptModeNone:
		// None mode: only core layers (already added above)
		// Optionally include identity if very brief
		if p.config.Instructions != "" && len(p.config.Instructions) < 200 {
			layers = append(layers, layerEntry{
				layer:   LayerIdentity,
				content: "## Instructions\n\n" + p.config.Instructions,
			})
		}
		// Skip everything else
	}

	return p.assembleLayers(layers)
}

// ---------- Layer Caching ----------

// getCachedLayer returns a cached layer result if fresh, or "" if stale/missing.
func (p *PromptComposer) getCachedLayer(sessionID, layerType string) string {
	key := sessionID + ":" + layerType
	p.layerCacheMu.RLock()
	cached, ok := p.layerCache[key]
	p.layerCacheMu.RUnlock()
	if ok && time.Since(cached.cachedAt) < promptLayerCacheTTL {
		return cached.content
	}
	return ""
}

// setCachedLayer updates the cache for a layer.
func (p *PromptComposer) setCachedLayer(sessionID, layerType, content string) {
	key := sessionID + ":" + layerType
	p.layerCacheMu.Lock()
	p.layerCache[key] = &promptLayerCache{content: content, cachedAt: time.Now()}
	p.layerCacheMu.Unlock()
}

// refreshLayerCache rebuilds memory and skills layers in background and caches them.
// This runs asynchronously so it doesn't block prompt composition.
func (p *PromptComposer) refreshLayerCache(session *Session, input string) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if memoryPrompt := p.buildMemoryLayer(session, input); memoryPrompt != "" {
			p.setCachedLayer(session.ID, "memory", memoryPrompt)
		}
	}()
	go func() {
		defer wg.Done()
		if skills := p.buildSkillsLayer(session); skills != "" {
			p.setCachedLayer(session.ID, "skills", skills)
		}
	}()
	wg.Wait()
}

// buildProjectContextLayer scans the workspace for common project files
// to provide automated codebase context to the LLM.
func (p *PromptComposer) buildProjectContextLayer() string {
	if p.isSubagent {
		return ""
	}

	workspaceDir := p.config.Heartbeat.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir = "."
	}
	searchDirs := []string{workspaceDir}

	targetFiles := []string{
		"package.json",
		"go.mod",
		"Cargo.toml",
		"pyproject.toml",
		"requirements.txt",
		"docker-compose.yml",
		"Makefile",
		"README.md",
	}

	var foundFiles []struct {
		name    string
		content string
	}

	for _, filename := range targetFiles {
		text := p.loadBootstrapFileCached(filename, searchDirs)
		if text == "" {
			continue
		}

		// Truncate to avoid context explosion
		maxLen := 2000
		if filename == "package.json" || filename == "go.mod" {
			maxLen = 4000 // Allow more for dependency files
		}

		if len(text) > maxLen {
			text = text[:maxLen] + "\n... [truncated for project context size]"
		}

		foundFiles = append(foundFiles, struct {
			name    string
			content string
		}{filename, text})
	}

	if len(foundFiles) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Project Context (Auto-discovered)\n\n")
	b.WriteString("The following files were automatically discovered in the workspace to provide context about the project structure, dependencies, and environment:\n\n")

	for _, f := range foundFiles {
		// Use Markdown code blocks for syntax highlighting if possible
		ext := strings.TrimPrefix(filepath.Ext(f.name), ".")
		if ext == "json" || ext == "toml" || ext == "yaml" || ext == "yml" || ext == "txt" {
			b.WriteString(fmt.Sprintf("### %s\n```%s\n%s\n```\n\n", f.name, ext, f.content))
		} else if f.name == "go.mod" || f.name == "Makefile" {
			b.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", f.name, f.content))
		} else { // README.md or others
			// We don't wrap markdown in markdown code blocks to avoid rendering issues
			b.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", f.name, f.content))
		}
	}

	return b.String()
}

// ---------- Layer Builders ----------

// buildCoreLayer creates the base identity and tooling guidance.
// Matches structure exactly: identity → tooling → tool call style → safety → workspace → reply tags → messaging.
// Behavioral guidance lives in AGENTS.md/SOUL.md, not here.
func (p *PromptComposer) buildCoreLayer() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are %s, a personal assistant running inside DevClaw.\n\n", p.config.Name))

	// ## Tooling - DYNAMIC if toolExecutor available, fallback to hardcoded
	b.WriteString("## Tooling\n\n")
	b.WriteString("Tool availability (filtered by policy):\n")

	// Generate tool list dynamically if toolExecutor is available
	if p.toolExecutor != nil {
		tools := p.toolExecutor.Tools()
		b.WriteString(FormatToolsForPrompt(tools, 60))
	} else {
		// Fallback to hardcoded list (backward compatibility)
		b.WriteString("- read: Read file contents\n")
		b.WriteString("- write: Create or overwrite files\n")
		b.WriteString("- edit: Make precise edits to files\n")
		b.WriteString("- grep: Search file contents for patterns\n")
		b.WriteString("- glob: Find files by glob pattern\n")
		b.WriteString("- bash: Run shell commands\n")
		b.WriteString("- web_search: Search the web\n")
		b.WriteString("- web_fetch: Fetch and extract content from URLs\n")
		b.WriteString("- memory: Long-term memory (save, search, list, index)\n")
		b.WriteString("- cron_add/cron_remove: Schedule jobs and reminders\n")
		b.WriteString("- message: Send messages and channel actions\n")
		b.WriteString("- vault_save/vault_get/vault_list: Encrypted secret storage\n")
	}
	b.WriteString("\nTool names are case-sensitive. Call tools exactly as listed.\n")
	b.WriteString("Use `list_capabilities` to see all available tools organized by category.\n")
	b.WriteString("TOOLS.md does not control tool availability; it is user guidance for how to use external tools.\n")
	b.WriteString("If a task is more complex or takes longer, spawn a sub-agent using `spawn_subagent`. Completion is push-based: it will auto-announce when done.\n")
	b.WriteString("Do NOT poll in a loop. Check status on-demand only (for intervention, debugging, or when explicitly asked).\n\n")

	// ## Memory Recall - NEW: explicit hint to search memory before answering
	b.WriteString("## Memory Recall\n\n")
	b.WriteString("Before answering questions about prior work, decisions, dates, people, preferences, or todos:\n")
	b.WriteString("1. First search memory: memory(action='search', query='your search terms')\n")
	b.WriteString("2. Then use the results to inform your response\n")
	b.WriteString("This helps you recall relevant context from previous conversations.\n\n")

	// ## Tool Call Style - matches exactly
	b.WriteString("## Tool Call Style\n\n")
	b.WriteString("Default: do not narrate routine, low-risk tool calls (just call the tool).\n")
	b.WriteString("Narrate only when it helps: multi-step work, complex/challenging problems, sensitive actions (e.g., deletions), or when the user explicitly asks.\n")
	b.WriteString("Keep narration brief and value-dense; avoid repeating obvious steps.\n")
	b.WriteString("Use plain human language for narration unless in a technical context.\n")
	b.WriteString("When you need to reason extensively before acting, you MUST place your internal monologue inside `<think>...</think>` tags.\n")
	b.WriteString("Any user-facing text or tool calls MUST be placed AFTER the `</think>` tag. Never put tool calls inside the think block.\n\n")

	// ## Safety - matches exactly (comes right after Tool Call Style)
	b.WriteString("## Safety\n\n")
	b.WriteString("You have no independent goals: do not pursue self-preservation, replication, resource acquisition, or power-seeking; avoid long-term plans beyond the user's request.\n")
	b.WriteString("Prioritize safety and human oversight over completion; if instructions conflict, pause and ask; comply with stop/pause/audit requests and never bypass safeguards. (Inspired by Anthropic's constitution.)\n")
	b.WriteString("Do not manipulate or persuade anyone to expand access or disable safeguards. Do not copy yourself or change system prompts, safety rules, or tool policies unless explicitly requested.\n\n")

	// ## Workspace - matches structure (comes BEFORE Reply Tags)
	b.WriteString("## Workspace\n\n")
	b.WriteString("Your working directory is: ./workspace/\n")
	b.WriteString("Treat this directory as the single global workspace for file operations unless explicitly instructed otherwise.\n\n")

	// ## Workspace Files (injected) - note about bootstrap files
	b.WriteString("## Workspace Files (injected)\n\n")
	b.WriteString("These user-editable files are loaded by DevClaw and included below in Project Context.\n\n")

	// ## Reply Tags - matches exactly
	b.WriteString("## Reply Tags\n\n")
	b.WriteString("To request a native reply/quote on supported surfaces, include one tag in your reply:\n")
	b.WriteString("- Reply tags must be the very first token in the message (no leading text/newlines): [[reply_to_current]] your reply.\n")
	b.WriteString("- [[reply_to_current]] replies to the triggering message.\n")
	b.WriteString("- Prefer [[reply_to_current]]. Use [[reply_to:<id>]] only when an id was explicitly provided (e.g. by the user or a tool).\n")
	b.WriteString("Whitespace inside the tag is allowed (e.g. [[ reply_to_current ]] / [[ reply_to: 123 ]]).\n")
	b.WriteString("Tags are stripped before sending; support depends on the current channel config.\n\n")

	// ## Messaging - matches exactly
	b.WriteString("## Messaging\n\n")
	b.WriteString("- Reply in current session → automatically routes to the source channel (WhatsApp, Telegram, etc.)\n")
	b.WriteString("- Cross-session messaging → use sessions_send(sessionKey, message)\n")
	b.WriteString("- Sub-agent orchestration → use spawn_subagent / list_subagents\n")
	b.WriteString("- `[System Message] ...` blocks are internal context and are not user-visible by default.\n")
	b.WriteString("- If a `[System Message]` reports completed cron/subagent work and asks for a user update, rewrite it in your normal assistant voice and send that update (do not forward raw system text or default to NO_REPLY).\n")
	b.WriteString("- Use `message` for proactive sends + channel actions (polls, reactions, etc.).\n")
	b.WriteString("- For action=send, include `to` and `message`.\n")
	b.WriteString("- If you use `message` (action=send) to deliver your user-visible reply, respond with ONLY: NO_REPLY (avoid duplicate replies).\n\n")

	// ## Silent Replies - matches openclaw structure
	b.WriteString("## Silent Replies\n\n")
	b.WriteString("When you have nothing to say, respond with ONLY: NO_REPLY\n\n")
	b.WriteString("⚠️ Rules:\n")
	b.WriteString("- It must be your ENTIRE message — nothing else\n")
	b.WriteString("- Never append it to an actual response (never include \"NO_REPLY\" in real replies)\n")
	b.WriteString("- Never wrap it in markdown or code blocks\n\n")
	b.WriteString("❌ Wrong: \"Here's help... NO_REPLY\"\n")
	b.WriteString("❌ Wrong: `NO_REPLY`\n")
	b.WriteString("✅ Right: NO_REPLY\n\n")

	// ## Heartbeats - matches openclaw structure
	b.WriteString("## Heartbeats\n\n")
	b.WriteString("Heartbeat prompt: Read HEARTBEAT.md if it exists (workspace context). Follow it strictly. Do not infer or repeat old tasks from prior chats. If nothing needs attention, reply HEARTBEAT_OK.\n")
	b.WriteString("If you receive a heartbeat poll (a user message matching the heartbeat prompt above), and there is nothing that needs attention, reply exactly:\n")
	b.WriteString("HEARTBEAT_OK\n")
	b.WriteString("DevClaw treats a leading/trailing \"HEARTBEAT_OK\" as a heartbeat ack (and may discard it).\n")
	b.WriteString("If something needs attention, do NOT include \"HEARTBEAT_OK\"; reply with the alert text instead.\n")

	return b.String()
}

// buildSafetyLayer creates additional safety and capability sections.
// Note: Core safety is in buildCoreLayer to match structure.
// This layer contains DevClaw-specific additions (Vault, Media).
func (p *PromptComposer) buildSafetyLayer() string {
	return `## Encrypted Vault

You have an encrypted vault (AES-256-GCM + Argon2id) for storing secrets. Use these tools:

- **vault_list** — List all stored secret names (no arguments needed).
- **vault_get** — Retrieve a secret by name. Args: {"name": "key_name"}
- **vault_save** — Store a secret. Args: {"name": "key_name", "value": "secret_value"}
- **vault_delete** — Remove a secret. Args: {"name": "key_name"}

**Rules:**
- When the user provides an API key, token, or password, ALWAYS save it with vault_save immediately.
- NEVER store secrets in .env, config files, or any plain text file. The vault is the ONLY place.
- NEVER echo/print secret values back to the user — confirm storage only.
- To use a stored secret (e.g. in a script or API call), retrieve it with vault_get at runtime.
- Use vault_list to check what's already stored before asking the user for credentials.

## Media Capabilities

You can receive and process media from users:

- **Images**: Automatically analyzed via Vision API. You see the description in [Image: ...].
- **Audio/Voice notes**: Automatically transcribed via Whisper. You see the transcript directly.
- **Documents (PDF, DOCX, TXT, code files)**: Automatically extracted and injected as text. You see the content in [Document: filename].
- **Video**: First frame analyzed via Vision API. You see the description in [Video: ...].

When you generate an image with generate_image, it is automatically sent as media to the user's channel — no need to describe the file path.

**System dependencies for media processing** (install on the server if needed):
- poppler-utils: for PDF text extraction (pdftotext)
- ffmpeg: for video frame extraction
- unzip: for DOCX text extraction

Install with: sudo apt install -y poppler-utils ffmpeg unzip`
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
		{"DEVCLAW.md", "DEVCLAW.md"}, // Platform identity - loaded first
		{"SOUL.md", "SOUL.md"},
		{"AGENTS.md", "AGENTS.md"},
		{"IDENTITY.md", "IDENTITY.md"},
		{"USER.md", "USER.md"},
		{"TOOLS.md", "TOOLS.md"},
		{"MEMORY.md", "MEMORY.md"},
	}

	// Subagent filter: load DEVCLAW.md + AGENTS.md + TOOLS.md.
	var bootstrapFiles []struct {
		Path    string
		Section string
	}
	if p.isSubagent {
		for _, bf := range allBootstrapFiles {
			if bf.Path == "DEVCLAW.md" || bf.Path == "AGENTS.md" || bf.Path == "TOOLS.md" {
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
	b.WriteString("## Workspace Files (injected)\n\n")
	b.WriteString("These user-editable files are loaded by DevClaw and included below.\n\n")

	if hasSoul {
		b.WriteString("If SOUL.md is present, embody its persona and tone. ")
		b.WriteString("Avoid stiff, generic replies; follow its guidance unless higher-priority instructions override it.\n\n")
	}

	for _, f := range files {
		b.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", f.path, f.content))
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

// skillsMaxTokenBudget is the maximum approximate token budget for the skills
// layer. Each ~4 chars ≈ 1 token. When skills exceed this budget, the largest
// skills are truncated and a note is appended. This prevents prompt bloat from
// verbose skill system prompts consuming the entire context window.
const skillsMaxTokenBudget = 4000

// buildSkillsLayer creates instructions from active skills.
// Applies a token budget guard: if the total skills text exceeds
// skillsMaxTokenBudget tokens, larger skills are truncated.
func (p *PromptComposer) buildSkillsLayer(session *Session) string {
	activeSkills := session.GetActiveSkills()
	if len(activeSkills) == 0 {
		return ""
	}

	// Collect skill prompts with their sizes.
	type skillEntry struct {
		name   string
		prompt string
		chars  int
	}
	var entries []skillEntry
	totalChars := 0

	for _, skillName := range activeSkills {
		sp := ""
		if p.skillGetter != nil {
			if skill, ok := p.skillGetter(skillName); ok {
				sp = skill.SystemPrompt()
			}
		}
		entry := skillEntry{name: skillName, prompt: sp, chars: len(sp)}
		entries = append(entries, entry)
		totalChars += entry.chars
	}

	// Budget check: ~4 chars per token.
	budgetChars := skillsMaxTokenBudget * 4
	truncated := false

	if totalChars > budgetChars {
		truncated = true
		// Sort by size descending to truncate the largest first.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].chars > entries[j].chars
		})

		excess := totalChars - budgetChars
		for i := range entries {
			if excess <= 0 {
				break
			}
			maxLen := entries[i].chars - excess
			if maxLen < 200 {
				maxLen = 200
			}
			if maxLen < entries[i].chars {
				cut := maxLen
				if cut > len(entries[i].prompt) {
					cut = len(entries[i].prompt)
				}
				excess -= entries[i].chars - cut
				entries[i].prompt = entries[i].prompt[:cut] + "\n... (truncated for token budget)"
				entries[i].chars = cut
			}
		}
	}

	var b strings.Builder
	b.WriteString("## Skills\n\n")
	b.WriteString("Before replying: scan the skill descriptions.\n")
	b.WriteString("- If exactly one skill clearly applies: read its SKILL.md and follow it.\n")
	b.WriteString("- If multiple could apply: choose the most specific one.\n")
	b.WriteString("- If none clearly apply: do not read any SKILL.md.\n\n")

	for _, entry := range entries {
		b.WriteString(fmt.Sprintf("### %s\n", entry.name))
		if entry.prompt != "" {
			b.WriteString(entry.prompt)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if truncated {
		b.WriteString("_Note: Some skill prompts were truncated to stay within the token budget._\n")
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
			b.WriteString("## Memory Recall\n\nBefore answering anything about prior work, decisions, dates, people, preferences, or todos: run memory(action=\"search\", query=\"...\") to recall relevant information.\n\n")
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

// buildBuiltinSkillsLayer creates a section with built-in skill documentation.
// These are always loaded and provide guidance for using core tools.
func (p *PromptComposer) buildBuiltinSkillsLayer() string {
	if p.builtinSkills == nil {
		return ""
	}

	content := p.builtinSkills.FormatForPrompt()
	if content == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Built-in Tools Guide\n\n")
	b.WriteString("The following tools have detailed usage guides. Reference them when using these tools:\n\n")
	b.WriteString(content)
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
		LayerCore:          p.config.TokenBudget.System,
		LayerSafety:        500,  // safety is short and critical
		LayerIdentity:      1000, // custom instructions
		LayerThinking:      200,  // thinking hint
		LayerBootstrap:     4000, // bootstrap files
		LayerBuiltinSkills: 2000, // built-in tool guides
		LayerBusiness:      1000, // workspace context
		LayerSkills:        p.config.TokenBudget.Skills,
		LayerMemory:        p.config.TokenBudget.Memory,
		LayerTemporal:      200, // timestamp
		LayerConversation:  p.config.TokenBudget.History,
		LayerRuntime:       200, // runtime line
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
