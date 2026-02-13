// Package copilot – config.go defines all configuration structures
// for the GoClaw Copilot assistant.
package copilot

import (
	"github.com/jholhewres/goclaw/pkg/goclaw/channels/whatsapp"
	"github.com/jholhewres/goclaw/pkg/goclaw/plugins"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
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

	// Logging configures log output.
	Logging LoggingConfig `yaml:"logging"`
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

	// Discord config is loaded via plugin; these are just YAML values
	// passed to the plugin on init.
	Discord map[string]any `yaml:"discord"`

	// Telegram config passed to the plugin on init.
	Telegram map[string]any `yaml:"telegram"`
}

// MemoryConfig configures the memory and persistence system.
type MemoryConfig struct {
	// Type is the storage type ("sqlite", "postgres", "memory").
	Type string `yaml:"type"`

	// Path is the database file path (for sqlite).
	Path string `yaml:"path"`

	// MaxMessages is the max messages kept per session.
	MaxMessages int `yaml:"max_messages"`

	// CompressionStrategy defines memory compression
	// ("summarize", "truncate", "semantic").
	CompressionStrategy string `yaml:"compression_strategy"`
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
		},
		Security: SecurityConfig{
			MaxInputLength:      4096,
			RateLimit:           30,
			EnablePIIDetection:  false,
			EnableURLValidation: true,
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
		Heartbeat: DefaultHeartbeatConfig(),
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
