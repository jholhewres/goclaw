// Package copilot – config.go defines all configuration structures
// for the GoClaw Copilot assistant.
package copilot

import (
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/discord"
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/slack"
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/telegram"
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/whatsapp"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
	"github.com/jholhewres/goclaw/pkg/goclaw/plugins"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
	"github.com/jholhewres/goclaw/pkg/goclaw/webui"
)

// Config holds all assistant configuration.
type Config struct {
	// Name is the assistant name shown in responses.
	Name string `yaml:"name"`

	// Trigger is the keyword that activates the bot (e.g. "@copilot").
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

	// Media configures vision and audio transcription.
	Media MediaConfig `yaml:"media"`

	// Logging configures log output.
	Logging LoggingConfig `yaml:"logging"`

	// Queue configures message debouncing for bursts.
	Queue QueueConfig `yaml:"queue"`

	// Database configures the central SQLite database (goclaw.db).
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
}

// DatabaseConfig configures the central goclaw.db SQLite database.
type DatabaseConfig struct {
	// Path is the database file path (default: "./data/goclaw.db").
	Path string `yaml:"path"`
}

// GatewayConfig configures the HTTP API gateway.
type GatewayConfig struct {
	// Enabled turns the gateway on/off (default: false).
	Enabled bool `yaml:"enabled"`

	// Address is the listen address (default: ":8080").
	Address string `yaml:"address"`

	// AuthToken is the Bearer token for /api/* and /v1/* auth (empty = no auth).
	AuthToken string `yaml:"auth_token"`

	// CORSOrigins lists allowed origins for CORS (empty = no CORS).
	CORSOrigins []string `yaml:"cors_origins"`
}

// QueueConfig configures the message queue for handling bursts.
type QueueConfig struct {
	// DebounceMs is the debounce delay in ms before draining queued messages (default: 1000).
	DebounceMs int `yaml:"debounce_ms"`

	// MaxPending is the max queued messages per session before dropping oldest (default: 20).
	MaxPending int `yaml:"max_pending"`
}

// MediaConfig configures vision and audio transcription capabilities.
type MediaConfig struct {
	// VisionEnabled enables image understanding via LLM vision (default: true).
	VisionEnabled bool `yaml:"vision_enabled"`

	// VisionDetail controls quality: "auto", "low", "high" (default: "auto").
	VisionDetail string `yaml:"vision_detail"`

	// TranscriptionEnabled enables audio transcription (default: true).
	TranscriptionEnabled bool `yaml:"transcription_enabled"`

	// TranscriptionModel is the model for audio transcription (default: "whisper-1").
	TranscriptionModel string `yaml:"transcription_model"`

	// TranscriptionBaseURL is the base URL for the Whisper-compatible transcription API.
	// If empty, defaults to "https://api.openai.com/v1" when the main provider doesn't
	// support audio transcription (e.g. Z.AI/GLM, Anthropic, xAI).
	// Only OpenAI and compatible providers support the /audio/transcriptions endpoint.
	TranscriptionBaseURL string `yaml:"transcription_base_url"`

	// TranscriptionAPIKey is the API key for the transcription provider.
	// If empty, falls back to the main API key. Useful when the main provider
	// doesn't support Whisper and you need a separate OpenAI key for transcription.
	TranscriptionAPIKey string `yaml:"transcription_api_key"`

	// MaxImageSize is the max image size in bytes to process (default: 20MB).
	MaxImageSize int64 `yaml:"max_image_size"`

	// MaxAudioSize is the max audio size in bytes (default: 25MB - Whisper limit).
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

// FallbackConfig configures model fallback and retry behavior.
type FallbackConfig struct {
	// Models is the ordered list of fallback models to try on failure.
	Models []string `yaml:"models"`

	// MaxRetries per model before moving to next (default: 2).
	MaxRetries int `yaml:"max_retries"`

	// InitialBackoffMs is the initial retry delay in ms (default: 1000).
	InitialBackoffMs int `yaml:"initial_backoff_ms"`

	// MaxBackoffMs caps the backoff (default: 30000).
	MaxBackoffMs int `yaml:"max_backoff_ms"`

	// RetryOnStatusCodes lists HTTP codes that trigger retry (default: [429, 500, 502, 503, 529]).
	RetryOnStatusCodes []int `yaml:"retry_on_status_codes"`
}

// DefaultFallbackConfig returns sensible defaults for model fallback.
func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		Models:             nil,
		MaxRetries:         2,
		InitialBackoffMs:   1000,
		MaxBackoffMs:       30000,
		RetryOnStatusCodes: []int{429, 500, 502, 503, 529},
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
		out.RetryOnStatusCodes = []int{429, 500, 502, 503, 529}
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
	// Can also be set via the GOCLAW_API_KEY environment variable.
	APIKey string `yaml:"api_key"`

	// Provider hints which SDK to use ("openai", "anthropic", "glm").
	// Auto-detected from base_url if omitted.
	Provider string `yaml:"provider"`
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
}

// ToolExecutorConfig configures tool execution behavior.
type ToolExecutorConfig struct {
	// Parallel enables parallel execution of independent tools (default: true).
	Parallel bool `yaml:"parallel"`

	// MaxParallel is the max concurrent tool executions when parallel is enabled (default: 5).
	MaxParallel int `yaml:"max_parallel"`
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

	// Storage is the path to the scheduler database.
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
		Name:    "Copilot",
		Trigger: "@copilot",
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
			Path:                "./data/memory.db",
			MaxMessages:         100,
			CompressionStrategy: "summarize",
			Embedding:           memory.DefaultEmbeddingConfig(),
			Search: SearchConfig{
				HybridWeightVector: 0.7,
				HybridWeightBM25:   0.3,
				MaxResults:         6,
				MinScore:           0.1,
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
				Parallel:    true,
				MaxParallel: 5,
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
			Dir: "./plugins",
		},
		Sandbox: sandbox.DefaultConfig(),
		Skills: SkillsConfig{
			Builtin: []string{"calculator", "web-fetch", "datetime"},
		},
		Scheduler: SchedulerConfig{
			Enabled: true,
			Storage: "./data/scheduler.db",
		},
		Heartbeat:  DefaultHeartbeatConfig(),
		Subagents:  DefaultSubagentConfig(),
		Agent:      DefaultAgentConfig(),
		Fallback:   DefaultFallbackConfig(),
		Media:      DefaultMediaConfig(),
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Database: DatabaseConfig{
			Path: "./data/goclaw.db",
		},
		Gateway: GatewayConfig{
			Enabled: false,
			Address: ":8080",
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
