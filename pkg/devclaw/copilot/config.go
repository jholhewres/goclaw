// Package copilot – config.go defines all configuration structures
// for the DevClaw Copilot assistant.
package copilot

import (
	"path/filepath"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels/discord"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/slack"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/telegram"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/whatsapp"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/security"
	"github.com/jholhewres/devclaw/pkg/devclaw/database"
	"github.com/jholhewres/devclaw/pkg/devclaw/paths"
	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
	"github.com/jholhewres/devclaw/pkg/devclaw/sandbox"
	"github.com/jholhewres/devclaw/pkg/devclaw/webui"
)

// ProviderKeyNames maps provider IDs to their standard API key variable names.
// These follow industry conventions (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.)
var ProviderKeyNames = map[string]string{
	"openai":      "OPENAI_API_KEY",
	"anthropic":   "ANTHROPIC_API_KEY",
	"google":      "GOOGLE_API_KEY",
	"xai":         "XAI_API_KEY",
	"groq":        "GROQ_API_KEY",
	"zai":         "ZAI_API_KEY",
	"mistral":     "MISTRAL_API_KEY",
	"openrouter":  "OPENROUTER_API_KEY",
	"cerebras":    "CEREBRAS_API_KEY",
	"minimax":     "MINIMAX_API_KEY",
	"huggingface": "HUGGINGFACE_API_KEY",
	"deepseek":    "DEEPSEEK_API_KEY",
	"custom":      "CUSTOM_API_KEY",
}

// GetProviderKeyName returns the standard API key variable name for a provider.
// Falls back to "API_KEY" for unknown providers.
func GetProviderKeyName(provider string) string {
	if name, ok := ProviderKeyNames[strings.ToLower(provider)]; ok {
		return name
	}
	return "API_KEY"
}

// Config holds all assistant configuration.
type Config struct {
	// Name is the assistant name shown in responses.
	Name string `yaml:"name"`

	// Trigger is the keyword that activates the bot (e.g. "@devclaw").
	Trigger string `yaml:"trigger"`

	// Model is the LLM model to use (e.g. "glm-4.7-flash").
	Model string `yaml:"model"`

	// API configures the LLM provider endpoint.
	API APIConfig `yaml:"api"`

	// Instructions are the base system prompt instructions.
	Instructions string `yaml:"instructions"`

	// Timezone is the user's timezone (e.g. "America/Sao_Paulo").
	Timezone string `yaml:"timezone"`

	// Language is the preferred response language (e.g. "pt-BR").
	Language string `yaml:"language"`

	// Access configures who can use the bot (allowlist/blocklist).
	Access AccessConfig `yaml:"access"`

	// Workspaces configures isolated profiles/contexts.
	Workspaces WorkspaceConfig `yaml:"workspaces"`

	// Channels configures communication channels.
	Channels ChannelsConfig `yaml:"channels"`

	// Memory configures the memory system.
	Memory MemoryConfig `yaml:"memory"`

	// Security configures security guardrails.
	Security SecurityConfig `yaml:"security"`

	// TokenBudget configures per-layer token limits.
	TokenBudget TokenBudgetConfig `yaml:"token_budget"`

	// Plugins configures the plugin loader.
	Plugins plugins.Config `yaml:"plugins"`

	// Sandbox configures the script sandbox.
	Sandbox sandbox.Config `yaml:"sandbox"`

	// Skills configures which skills are enabled.
	Skills SkillsConfig `yaml:"skills"`

	// Scheduler configures the task scheduler.
	Scheduler SchedulerConfig `yaml:"scheduler"`

	// Heartbeat configures the proactive heartbeat system.
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// Subagents configures the subagent orchestration system.
	Subagents SubagentConfig `yaml:"subagents"`

	// Agent configures the agent loop parameters (turns, timeouts, auto-continue).
	Agent AgentConfig `yaml:"agent"`

	// Fallback configures model fallback with retry and backoff.
	Fallback FallbackConfig `yaml:"fallback"`

	// Budget configures monthly cost tracking and limits.
	Budget BudgetConfig `yaml:"budget"`

	// Team configures multi-user mode.
	Team TeamConfig `yaml:"team"`

	// Media configures vision and audio transcription.
	Media MediaConfig `yaml:"media"`

	// Logging configures log output.
	Logging LoggingConfig `yaml:"logging"`

	// Queue configures message debouncing for bursts.
	Queue QueueConfig `yaml:"queue"`

	// Database configures the central SQLite database (devclaw.db).
	Database DatabaseConfig `yaml:"database"`

	// Gateway configures the HTTP API gateway.
	Gateway GatewayConfig `yaml:"gateway"`

	// BlockStream configures progressive message delivery (stream text to channel
	// in chunks instead of waiting for the complete response).
	BlockStream BlockStreamConfig `yaml:"block_stream"`

	// WebSearch configures the web search tool provider.
	WebSearch WebSearchConfig `yaml:"web_search"`

	// TTS configures text-to-speech synthesis.
	TTS TTSConfig `yaml:"tts"`

	// WebUI configures the web dashboard.
	WebUI webui.Config `yaml:"webui"`

	// Group configures group chat behavior.
	Group GroupConfig `yaml:"group"`

	// Agents configures specialized agent profiles and routing.
	Agents AgentsConfig `yaml:"agents"`

	// Groups configures group-specific policies and activation modes.
	Groups GroupsPolicyConfig `yaml:"groups"`

	// Hooks configures lifecycle hooks and webhooks.
	Hooks HooksConfig `yaml:"hooks"`

	// MCP configures Model Context Protocol servers.
	MCP MCPConfig `yaml:"mcp"`

	// Routines configures background routines (metrics, memory indexer, etc).
	Routines RoutinesConfig `yaml:"routines"`

	// NativeMedia configures the native media handling system.
	NativeMedia NativeMediaConfig `yaml:"native_media"`

	// Browser configures browser automation tools.
	Browser BrowserConfig `yaml:"browser"`
}

// RoutinesConfig configures background routines for metrics and memory indexing.
type RoutinesConfig struct {
	// Metrics configures the metrics collector.
	Metrics MetricsCollectorConfig `yaml:"metrics"`

	// MemoryIndexer configures the background memory indexer.
	MemoryIndexer MemoryIndexerConfig `yaml:"memory_indexer"`
}

// DefaultRoutinesConfig returns sensible defaults for background routines.
func DefaultRoutinesConfig() RoutinesConfig {
	return RoutinesConfig{
		Metrics:       DefaultMetricsCollectorConfig(),
		MemoryIndexer: DefaultMemoryIndexerConfig(),
	}
}

// NativeMediaConfig configures the native media handling system.
type NativeMediaConfig struct {
	// Enabled activates native media features (default: true after setup).
	Enabled bool `yaml:"enabled"`

	// Store configures media storage.
	Store NativeMediaStoreConfig `yaml:"store"`

	// Service configures the media service.
	Service NativeMediaServiceConfig `yaml:"service"`

	// Enrichment configures automatic media enrichment.
	Enrichment NativeMediaEnrichmentConfig `yaml:"enrichment"`
}

// NativeMediaStoreConfig configures media storage.
type NativeMediaStoreConfig struct {
	// BaseDir is the permanent storage directory.
	BaseDir string `yaml:"base_dir"`

	// TempDir is the temporary storage directory.
	TempDir string `yaml:"temp_dir"`

	// MaxFileSize is the maximum file size in bytes.
	MaxFileSize int64 `yaml:"max_file_size"`
}

// NativeMediaServiceConfig configures the media service.
type NativeMediaServiceConfig struct {
	// MaxImageSize is the maximum image size in bytes.
	MaxImageSize int64 `yaml:"max_image_size"`

	// MaxAudioSize is the maximum audio size in bytes.
	MaxAudioSize int64 `yaml:"max_audio_size"`

	// MaxDocSize is the maximum document size in bytes.
	MaxDocSize int64 `yaml:"max_doc_size"`

	// TempTTL is the time-to-live for temporary files.
	TempTTL string `yaml:"temp_ttl"`

	// CleanupEnabled enables automatic cleanup of expired files.
	CleanupEnabled bool `yaml:"cleanup_enabled"`

	// CleanupInterval is the interval between cleanup runs.
	CleanupInterval string `yaml:"cleanup_interval"`
}

// NativeMediaEnrichmentConfig configures automatic media enrichment.
type NativeMediaEnrichmentConfig struct {
	// AutoEnrichImages runs vision on received images.
	AutoEnrichImages bool `yaml:"auto_enrich_images"`

	// AutoEnrichAudio transcribes received audio.
	AutoEnrichAudio bool `yaml:"auto_enrich_audio"`

	// AutoEnrichDocuments extracts text from documents.
	AutoEnrichDocuments bool `yaml:"auto_enrich_documents"`
}

// DefaultNativeMediaConfig returns sensible defaults for native media.
// Note: The enrichment flags (AutoEnrichImages, AutoEnrichAudio) are set to true
// by default, but they will only work if the corresponding MediaConfig capabilities
// (VisionEnabled, TranscriptionEnabled) are also enabled. Documents always work
// as they don't depend on external APIs.
func DefaultNativeMediaConfig() NativeMediaConfig {
	mediaDir := paths.ResolveMediaDir()
	return NativeMediaConfig{
		Enabled: true,
		Store: NativeMediaStoreConfig{
			BaseDir:     mediaDir,
			TempDir:     filepath.Join(mediaDir, "temp"),
			MaxFileSize: 50 * 1024 * 1024, // 50MB
		},
		Service: NativeMediaServiceConfig{
			MaxImageSize:    20 * 1024 * 1024, // 20MB
			MaxAudioSize:    25 * 1024 * 1024, // 25MB (Whisper limit)
			MaxDocSize:      50 * 1024 * 1024, // 50MB
			TempTTL:         "24h",
			CleanupEnabled:  true,
			CleanupInterval: "1h",
		},
		Enrichment: NativeMediaEnrichmentConfig{
			// These flags request enrichment, but actual enrichment
			// depends on MediaConfig.VisionEnabled and TranscriptionEnabled
			AutoEnrichImages:    true,
			AutoEnrichAudio:     true,
			AutoEnrichDocuments: true,
		},
	}
}

// DatabaseConfig configures the central database using the Database Hub.
// Supports SQLite (default), PostgreSQL, and MySQL backends.
type DatabaseConfig struct {
	// Path is the database file path for SQLite (default: "./data/devclaw.db").
	// Kept for backward compatibility with existing configs.
	Path string `yaml:"path"`

	// Hub enables the new Database Hub system with multi-backend support.
	// When Hub.Backend is not set, falls back to Path for SQLite.
	Hub database.HubConfig `yaml:"hub"`
}

// Effective returns the effective Hub configuration, applying defaults.
func (c DatabaseConfig) Effective() database.HubConfig {
	if c.Hub.Backend != "" {
		return c.Hub.Effective()
	}

	// Fallback to legacy Path-based config
	hub := database.DefaultHubConfig()
	if c.Path != "" {
		hub.SQLite.Path = c.Path
	}
	return hub
}

// GatewayConfig configures the HTTP API gateway.
type GatewayConfig struct {
	// Enabled turns the gateway on/off (default: false).
	Enabled bool `yaml:"enabled"`

	// Address is the listen address (default: ":8085").
	Address string `yaml:"address"`

	// AuthToken is the Bearer token for /api/* and /v1/* auth (empty = no auth).
	AuthToken string `yaml:"auth_token"`

	// CORSOrigins lists allowed origins for CORS (empty = no CORS).
	CORSOrigins []string `yaml:"cors_origins"`
}

// QueueConfig configures the message queue for handling bursts.
type QueueConfig struct {
	// DebounceMs is the debounce delay in ms before draining queued messages (default: 200).
	DebounceMs int `yaml:"debounce_ms"`

	// MaxPending is the max queued messages per session before dropping oldest (default: 20).
	MaxPending int `yaml:"max_pending"`

	// DefaultMode is the default queue mode for all channels (default: "collect").
	DefaultMode QueueMode `yaml:"default_mode"`

	// ByChannel overrides the default mode per channel name.
	ByChannel map[string]QueueMode `yaml:"by_channel"`

	// DropPolicy controls what happens when the queue exceeds MaxPending (default: "old").
	DropPolicy QueueDropPolicy `yaml:"drop_policy"`
}

// MediaConfig configures vision and audio transcription capabilities.
type MediaConfig struct {
	// VisionEnabled enables image understanding via LLM vision (default: true).
	VisionEnabled bool `yaml:"vision_enabled"`

	// VisionModel overrides the model used for image/video understanding.
	// If empty, uses the main chat model. Examples: "glm-4.6v", "gpt-4o", "claude-sonnet-4-20250514".
	VisionModel string `yaml:"vision_model"`

	// VisionDetail controls quality: "auto", "low", "high" (default: "auto").
	VisionDetail string `yaml:"vision_detail"`

	// TranscriptionEnabled enables audio transcription (default: true).
	TranscriptionEnabled bool `yaml:"transcription_enabled"`

	// TranscriptionModel is the model for audio transcription (default: "whisper-1").
	// Examples: "whisper-1", "glm-asr-2512", "gpt-4o-transcribe", "whisper-large-v3".
	TranscriptionModel string `yaml:"transcription_model"`

	// TranscriptionBaseURL is the base URL for the transcription API.
	// Examples:
	//   Z.AI:   "https://api.z.ai/api/paas/v4"
	//   Groq:   "https://api.groq.com/openai/v1"
	//   OpenAI: "https://api.openai.com/v1" (default)
	TranscriptionBaseURL string `yaml:"transcription_base_url"`

	// TranscriptionAPIKey is the API key for the transcription provider.
	// If empty, falls back to the main API key.
	TranscriptionAPIKey string `yaml:"transcription_api_key"`

	// TranscriptionLanguage hints the expected language (ISO 639-1, e.g. "pt", "en", "es").
	// For Whisper: passed as the "language" field.
	// For Z.AI GLM-ASR: used as a prompt hint for auto-detection.
	TranscriptionLanguage string `yaml:"transcription_language"`

	// MaxImageSize is the max image size in bytes to process (default: 20MB).
	MaxImageSize int64 `yaml:"max_image_size"`

	// MaxAudioSize is the max audio size in bytes (default: 25MB).
	MaxAudioSize int64 `yaml:"max_audio_size"`
}

// DefaultMediaConfig returns sensible defaults for media processing.
func DefaultMediaConfig() MediaConfig {
	return MediaConfig{
		VisionEnabled:        true,
		VisionDetail:         "auto",
		TranscriptionEnabled: true,
		TranscriptionModel:   "whisper-1",
		MaxImageSize:         20 * 1024 * 1024, // 20MB
		MaxAudioSize:         25 * 1024 * 1024, // 25MB (Whisper limit)
	}
}

// Effective returns a copy with default values filled in for zero fields.
func (m MediaConfig) Effective() MediaConfig {
	out := m
	if out.MaxImageSize == 0 {
		out.MaxImageSize = 20 * 1024 * 1024
	}
	if out.MaxAudioSize == 0 {
		out.MaxAudioSize = 25 * 1024 * 1024
	}
	if out.VisionDetail == "" {
		out.VisionDetail = "auto"
	}
	if out.TranscriptionModel == "" {
		out.TranscriptionModel = "whisper-1"
	}
	return out
}

// ResolveForProvider fills in transcription defaults based on the main API
// provider so users don't have to configure transcription separately when
// their provider already supports it.
func (m *MediaConfig) ResolveForProvider(provider, baseURL string) {
	if m.TranscriptionBaseURL != "" {
		return
	}
	switch {
	case provider == "openai" || provider == "openrouter":
		// OpenAI natively supports /audio/transcriptions
	case isZAIProvider(provider, baseURL):
		m.TranscriptionBaseURL = "https://api.z.ai/api/paas/v4"
		if m.TranscriptionModel == "whisper-1" {
			m.TranscriptionModel = "glm-asr-2512"
		}
	case provider == "groq":
		m.TranscriptionBaseURL = "https://api.groq.com/openai/v1"
		if m.TranscriptionModel == "whisper-1" {
			m.TranscriptionModel = "whisper-large-v3"
		}
	}
}

func isZAIProvider(provider, baseURL string) bool {
	return strings.Contains(baseURL, "z.ai") || strings.Contains(baseURL, "zhipu") ||
		strings.HasPrefix(provider, "zai") || strings.HasPrefix(provider, "zhipu")
}

// FallbackConfig configures model fallback and retry behavior.
type FallbackConfig struct {
	// Models is the ordered list of fallback models to try on failure.
	// Supports N providers: primary -> fallback1 -> fallback2 -> ... -> local.
	Models []string `yaml:"models"`

	// Chain defines provider-specific fallback with separate base_url/api_key.
	// Each entry is a complete provider config tried in order on failure.
	Chain []ProviderChainEntry `yaml:"chain"`

	// MaxRetries per model before moving to next (default: 2).
	MaxRetries int `yaml:"max_retries"`

	// InitialBackoffMs is the initial retry delay in ms (default: 1000).
	InitialBackoffMs int `yaml:"initial_backoff_ms"`

	// MaxBackoffMs caps the backoff (default: 30000).
	MaxBackoffMs int `yaml:"max_backoff_ms"`

	// RetryOnStatusCodes lists HTTP codes that trigger retry (default: [429, 500, 502, 503, 529]).
	RetryOnStatusCodes []int `yaml:"retry_on_status_codes"`
}

// ProviderChainEntry defines a single provider in the fallback chain.
type ProviderChainEntry struct {
	Provider string `yaml:"provider"`           // Provider name (openai, anthropic, ollama, etc.)
	BaseURL  string `yaml:"base_url"`           // API endpoint
	APIKey   string `yaml:"api_key,omitempty"`  // API key (can use ${VAR} references)
	Model    string `yaml:"model"`              // Model to use from this provider
}

// BudgetConfig configures monthly cost tracking and limits.
type BudgetConfig struct {
	// MonthlyLimitUSD is the maximum monthly spend (0 = unlimited).
	MonthlyLimitUSD float64 `yaml:"monthly_limit_usd"`

	// WarnAtPercent triggers a warning when this % of budget is reached (default: 80).
	WarnAtPercent int `yaml:"warn_at_percent"`

	// ActionAtLimit defines behavior when limit is reached: "warn", "block", "fallback_local".
	ActionAtLimit string `yaml:"action_at_limit"`
}

// DefaultBudgetConfig returns sensible defaults for budget tracking.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MonthlyLimitUSD: 0,
		WarnAtPercent:   80,
		ActionAtLimit:   "warn",
	}
}

// DefaultFallbackConfig returns sensible defaults for model fallback.
func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		Models:             nil,
		MaxRetries:         2,
		InitialBackoffMs:   1000,
		MaxBackoffMs:       30000,
		RetryOnStatusCodes: []int{429, 500, 502, 503, 521, 522, 523, 524, 529},
	}
}

// Effective returns a copy with default values filled in for zero fields.
func (f FallbackConfig) Effective() FallbackConfig {
	out := f
	if out.MaxRetries == 0 {
		out.MaxRetries = 2
	}
	if out.InitialBackoffMs == 0 {
		out.InitialBackoffMs = 1000
	}
	if out.MaxBackoffMs == 0 {
		out.MaxBackoffMs = 30000
	}
	if len(out.RetryOnStatusCodes) == 0 {
		out.RetryOnStatusCodes = []int{429, 500, 502, 503, 521, 522, 523, 524, 529}
	}
	return out
}

// APIConfig configures the LLM provider endpoint and credentials.
type APIConfig struct {
	// BaseURL is the API base URL (OpenAI-compatible endpoint).
	// Examples:
	//   https://api.openai.com/v1           (OpenAI)
	//   https://api.z.ai/api/anthropic      (GLM / Anthropic proxy)
	//   https://api.anthropic.com/v1        (Anthropic direct)
	BaseURL string `yaml:"base_url"`

	// APIKey is the authentication key for the provider.
	// Can also be set via the DEVCLAW_API_KEY environment variable.
	APIKey string `yaml:"api_key"`

	// Provider hints which SDK to use ("openai", "anthropic", "glm").
	// Auto-detected from base_url if omitted.
	Provider string `yaml:"provider"`

	// Params holds provider-specific parameters:
	//   context1m: true   — enable Anthropic 1M context beta for Opus/Sonnet
	//   tool_stream: true — enable real-time tool call streaming (Z.AI)
	Params map[string]any `yaml:"params"`
}

// ChannelsConfig holds configuration for all channels.
type ChannelsConfig struct {
	// WhatsApp is the WhatsApp channel config (core).
	WhatsApp whatsapp.Config `yaml:"whatsapp"`

	// Telegram is the Telegram channel config (core).
	Telegram telegram.Config `yaml:"telegram"`

	// Discord is the Discord channel config (core).
	Discord discord.Config `yaml:"discord"`

	// Slack is the Slack channel config (core).
	Slack slack.Config `yaml:"slack"`
}

// MemoryConfig configures the memory and persistence system.
type MemoryConfig struct {
	// Type is the storage type ("sqlite", "file").
	// "sqlite" enables FTS5 + vector search; "file" is the legacy fallback.
	Type string `yaml:"type"`

	// Path is the database file path (for sqlite).
	Path string `yaml:"path"`

	// MaxMessages is the max messages kept per session.
	MaxMessages int `yaml:"max_messages"`

	// CompressionStrategy defines memory compression
	// ("summarize", "truncate", "semantic").
	CompressionStrategy string `yaml:"compression_strategy"`

	// Embedding configures the embedding provider for semantic search.
	Embedding memory.EmbeddingConfig `yaml:"embedding"`

	// Search configures hybrid search behavior.
	Search SearchConfig `yaml:"search"`

	// Index configures automatic indexing.
	Index IndexConfig `yaml:"index"`

	// SessionMemory configures automatic session summarization.
	SessionMemory SessionMemoryConfig `yaml:"session_memory"`
}

// SearchConfig configures hybrid search behavior.
type SearchConfig struct {
	// HybridWeightVector is the weight for vector search (default: 0.7).
	HybridWeightVector float64 `yaml:"hybrid_weight_vector"`

	// HybridWeightBM25 is the weight for BM25 keyword search (default: 0.3).
	HybridWeightBM25 float64 `yaml:"hybrid_weight_bm25"`

	// MaxResults is the max results returned (default: 6).
	MaxResults int `yaml:"max_results"`

	// MinScore is the minimum score threshold (default: 0.1).
	MinScore float64 `yaml:"min_score"`

	// TemporalDecay configures time-based score decay for memory search.
	TemporalDecay TemporalDecayConfig `yaml:"temporal_decay"`

	// MMR configures Maximal Marginal Relevance for result diversification.
	MMR MMRConfig `yaml:"mmr"`
}

// TemporalDecayConfig configures exponential score decay based on memory age.
type TemporalDecayConfig struct {
	// Enabled activates temporal decay (default: false).
	Enabled bool `yaml:"enabled"`

	// HalfLifeDays is the number of days for score to halve (default: 30).
	HalfLifeDays float64 `yaml:"half_life_days"`
}

// MMRConfig configures Maximal Marginal Relevance for search diversification.
type MMRConfig struct {
	// Enabled activates MMR re-ranking (default: false).
	Enabled bool `yaml:"enabled"`

	// Lambda balances relevance vs diversity (default: 0.7).
	// 0 = max diversity, 1 = max relevance.
	Lambda float64 `yaml:"lambda"`
}

// IndexConfig configures automatic memory indexing.
type IndexConfig struct {
	// Auto enables automatic re-indexing on file changes (default: true).
	Auto bool `yaml:"auto"`

	// ChunkMaxTokens is the max tokens per chunk (default: 500).
	ChunkMaxTokens int `yaml:"chunk_max_tokens"`
}

// SessionMemoryConfig configures automatic session summarization.
type SessionMemoryConfig struct {
	// Enabled turns session memory on/off (default: false).
	Enabled bool `yaml:"enabled"`

	// Messages is the number of recent messages to include in summaries (default: 15).
	Messages int `yaml:"messages"`
}

// SecurityConfig configures security guardrails.
type SecurityConfig struct {
	// MaxInputLength is the max input size in characters.
	MaxInputLength int `yaml:"max_input_length"`

	// RateLimit is max messages per minute per user.
	RateLimit int `yaml:"rate_limit"`

	// EnablePIIDetection enables PII detection in outputs.
	EnablePIIDetection bool `yaml:"enable_pii_detection"`

	// EnableURLValidation enables URL validation in outputs.
	EnableURLValidation bool `yaml:"enable_url_validation"`

	// ToolGuard configures per-tool access control, command safety,
	// path protection, SSH allowlist, and audit logging.
	ToolGuard ToolGuardConfig `yaml:"tool_guard"`

	// ToolExecutor configures parallel tool execution.
	ToolExecutor ToolExecutorConfig `yaml:"tool_executor"`

	// SSRF configures URL validation for web_fetch (private IPs, metadata, etc.).
	SSRF security.SSRFConfig `yaml:"ssrf"`

	// ExecAnalysis configures command risk analysis for bash/exec tools.
	ExecAnalysis ExecAnalysisConfig `yaml:"exec_analysis"`
}

// ToolExecutorConfig configures tool execution behavior.
type ToolExecutorConfig struct {
	// Parallel enables parallel execution of independent tools (default: true).
	Parallel bool `yaml:"parallel"`

	// MaxParallel is the max concurrent tool executions when parallel is enabled (default: 5).
	MaxParallel int `yaml:"max_parallel"`

	// BashTimeoutSeconds is the executor-level timeout for bash/ssh/scp/exec tools (default: 300).
	BashTimeoutSeconds int `yaml:"bash_timeout_seconds"`

	// DefaultTimeoutSeconds is the executor-level timeout for all other tools (default: 30).
	DefaultTimeoutSeconds int `yaml:"default_timeout_seconds"`
}

// TokenBudgetConfig configures per-layer token allocation.
type TokenBudgetConfig struct {
	Total    int `yaml:"total"`
	Reserved int `yaml:"reserved"`
	System   int `yaml:"system"`
	Skills   int `yaml:"skills"`
	Memory   int `yaml:"memory"`
	History  int `yaml:"history"`
	Tools    int `yaml:"tools"`

	// BootstrapMaxChars is the max total characters for all bootstrap files
	// combined (SOUL.md, IDENTITY.md, etc.). Default: 20000 (~5K tokens).
	BootstrapMaxChars int `yaml:"bootstrap_max_chars"`
}

// SkillsConfig configures the skills system.
type SkillsConfig struct {
	// Builtin lists built-in skills to enable.
	Builtin []string `yaml:"builtin"`

	// Installed lists installed skill names.
	Installed []string `yaml:"installed"`

	// ClawdHubDirs lists directories with ClawdHub SKILL.md skills.
	ClawdHubDirs []string `yaml:"clawdhub_dirs"`
}

// SchedulerConfig configures the task scheduler.
type SchedulerConfig struct {
	// Enabled turns the scheduler on/off.
	Enabled bool `yaml:"enabled"`

	// Storage is the path to the scheduler storage file (legacy/fallback).
	// When devclawDB is available, jobs are stored in the "jobs" table in devclaw.db.
	// This field is only used as a fallback for file-based storage.
	Storage string `yaml:"storage"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	// Level is the log level ("debug", "info", "warn", "error").
	Level string `yaml:"level"`

	// Format is the log format ("json", "text").
	Format string `yaml:"format"`
}

// DefaultConfig returns the default assistant configuration.
func DefaultConfig() *Config {
	return &Config{
		Name:    "DevClaw",
		Trigger: "@devclaw",
		Model:   "gpt-5-mini",
		API: APIConfig{
			BaseURL: "https://api.openai.com/v1",
		},
		Instructions: "You are a helpful personal assistant. Be concise and practical.",
		Timezone:     "America/Sao_Paulo",
		Language:     "pt-BR",
		Access:     DefaultAccessConfig(),
		Workspaces: DefaultWorkspaceConfig(),
		Channels: ChannelsConfig{
			WhatsApp: whatsapp.DefaultConfig(),
		},
		Memory: MemoryConfig{
			Type:                "sqlite",
			Path:                paths.ResolveDatabasePath("memory.db"),
			MaxMessages:         100,
			CompressionStrategy: "summarize",
			Embedding:           memory.DefaultEmbeddingConfig(),
			Search: SearchConfig{
				HybridWeightVector: 0.7,
				HybridWeightBM25:   0.3,
				MaxResults:         6,
				MinScore:           0.1,
				TemporalDecay: TemporalDecayConfig{
					Enabled:      false,
					HalfLifeDays: 30,
				},
				MMR: MMRConfig{
					Enabled: false,
					Lambda:  0.7,
				},
			},
			Index: IndexConfig{
				Auto:           true,
				ChunkMaxTokens: 500,
			},
			SessionMemory: SessionMemoryConfig{
				Enabled:  false,
				Messages: 15,
			},
		},
		Security: SecurityConfig{
			MaxInputLength:      4096,
			RateLimit:           30,
			EnablePIIDetection:  false,
			EnableURLValidation: true,
			ToolGuard:           DefaultToolGuardConfig(),
			ToolExecutor: ToolExecutorConfig{
				Parallel:              true,
				MaxParallel:           5,
				BashTimeoutSeconds:    300,
				DefaultTimeoutSeconds: 30,
			},
		},
		TokenBudget: TokenBudgetConfig{
			Total:    128000,
			Reserved: 4096,
			System:   500,
			Skills:   2000,
			Memory:   1000,
			History:  8000,
			Tools:    4000,
		},
		Plugins: plugins.Config{
			Dir: paths.ResolvePluginsDir(),
		},
		Sandbox: sandbox.DefaultConfig(),
		Skills: SkillsConfig{
			Builtin: []string{"calculator", "web-fetch", "datetime", "skill-db"},
		},
		Scheduler: SchedulerConfig{
			Enabled: true,
			Storage: paths.ResolveDatabasePath("scheduler.db"),
		},
		Heartbeat:  DefaultHeartbeatConfig(),
		Subagents:  DefaultSubagentConfig(),
		Agent:      DefaultAgentConfig(),
		Fallback:   DefaultFallbackConfig(),
		Budget:     DefaultBudgetConfig(),
		Team:       DefaultTeamConfig(),
		Media:      DefaultMediaConfig(),
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Database: DatabaseConfig{
			Path: paths.ResolveDatabasePath("devclaw.db"),
			Hub:  database.DefaultHubConfig(),
		},
		Gateway: GatewayConfig{
			Enabled: false,
			Address: ":8085",
		},
		BlockStream: DefaultBlockStreamConfig(),
		WebSearch: WebSearchConfig{
			Provider:   "duckduckgo",
			MaxResults: 8,
		},
		TTS: TTSConfig{
			Provider: "openai",
			Voice:    "nova",
			Model:    "tts-1",
			AutoMode: "off",
		},
		WebUI: webui.Config{
			Enabled: false,
			Address: ":8090",
		},
		Browser: DefaultBrowserConfig(),
	}
}

// WebSearchConfig configures the web search tool.
type WebSearchConfig struct {
	// Provider is the search engine to use: "duckduckgo" (default) or "brave".
	Provider string `yaml:"provider"`

	// BraveAPIKey is the Brave Search API subscription token.
	// Can also be set via BRAVE_API_KEY env var.
	BraveAPIKey string `yaml:"brave_api_key"`

	// MaxResults is the maximum number of results to return (default: 8).
	MaxResults int `yaml:"max_results"`
}

// TTSConfig configures text-to-speech synthesis.
type TTSConfig struct {
	// Enabled activates TTS for assistant responses.
	Enabled bool `yaml:"enabled"`

	// Provider is the TTS provider to use: "openai" (default), "edge", "auto".
	// "auto" tries OpenAI first, falls back to Edge TTS if OpenAI is unavailable.
	Provider string `yaml:"provider"`

	// Voice is the voice to use.
	//   OpenAI: alloy, echo, fable, onyx, nova, shimmer
	//   Edge: pt-BR-FranciscaNeural, en-US-JennyNeural, etc.
	Voice string `yaml:"voice"`

	// EdgeVoice is the voice to use specifically for Edge TTS (when provider is "auto").
	// If empty, falls back to Voice.
	EdgeVoice string `yaml:"edge_voice"`

	// Model is the TTS model: "tts-1" (fast) or "tts-1-hd" (high quality).
	// Only used for OpenAI provider.
	Model string `yaml:"model"`

	// AutoMode controls when TTS is used:
	//   "off"     - disabled (default)
	//   "always"  - always generate audio alongside text
	//   "inbound" - generate audio only when the user sent a voice note
	AutoMode string `yaml:"auto_mode"`
}

// GroupConfig configures group chat behavior.
type GroupConfig struct {
	// ActivationMode controls when the bot responds in groups:
	//   "always"  — responds to all messages (default)
	//   "mention" — only when mentioned by name/trigger
	//   "reply"   — only when replied to directly
	ActivationMode string `yaml:"activation_mode"`

	// IntroMessage is sent when the bot joins a new group.
	// Empty = no intro. Supports template variables: {{name}}, {{trigger}}.
	IntroMessage string `yaml:"intro_message"`

	// ContextInjection adds group-specific context to the system prompt.
	// Useful for per-group instructions, rules, or personas.
	ContextInjection map[string]string `yaml:"context_injection"`

	// MaxParticipants limits context tracking for group participants.
	// Names of the last N participants are included in the prompt for
	// natural multi-party conversation (default: 20).
	MaxParticipants int `yaml:"max_participants"`

	// QuietHours defines time ranges when the bot won't respond in groups
	// (e.g. "23:00-07:00"). Empty = always active.
	QuietHours string `yaml:"quiet_hours"`

	// IgnorePatterns are regex patterns for messages the bot should ignore
	// even when activated (e.g. forwarded messages, bot commands for other bots).
	IgnorePatterns []string `yaml:"ignore_patterns"`
}
