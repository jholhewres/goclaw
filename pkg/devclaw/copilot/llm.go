// Package copilot – llm.go implements the LLM client for chat completions
// with function calling / tool use support.
// Uses the OpenAI-compatible API format, which works with OpenAI, Anthropic
// proxies, GLM (api.z.ai), and any compatible endpoint.
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StreamCallback is called for each token/chunk during streaming.
type StreamCallback func(chunk string)

// ---------- Client ----------

// LLMClient handles communication with the LLM provider API.
type LLMClient struct {
	baseURL    string
	provider   string // "openai", "zai", "zai-coding", "zai-anthropic", "anthropic", ""
	apiKey     string
	model      string
	fallback   FallbackConfig
	params     map[string]any // provider-specific params (context1m, tool_stream, etc.)
	httpClient *http.Client
	logger     *slog.Logger

	// OAuth support (optional)
	oauthTokenManager OAuthTokenManager

	// Rate-limit cooldown tracking for auto-recovery.
	// When the primary model hits a rate limit, we record when the cooldown
	// expires and which fallback model we're using. Once the cooldown nears
	// expiry, we probe the primary model to see if it recovered.
	cooldownMu       sync.Mutex
	cooldownExpires  time.Time     // when primary model cooldown expires
	cooldownModel    string        // the model that was rate-limited
	lastProbeAt      time.Time     // avoid probe storms
	probeMinInterval time.Duration // min time between probe attempts
}

// OAuthTokenManager is the interface for OAuth token management.
type OAuthTokenManager interface {
	GetValidToken(provider string) (interface{}, error)
}

// NewLLMClient creates a new LLM client from config.
func NewLLMClient(cfg *Config, logger *slog.Logger) *LLMClient {
	baseURL := cfg.API.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Always detect the provider from the URL first — this correctly identifies
	// proxy providers (e.g. zai-anthropic) that require different auth headers.
	// Only fall back to the config's provider when auto-detection returns the
	// generic default ("openai") and the user explicitly specified one.
	provider := detectProvider(baseURL)
	if provider == "openai" && cfg.API.Provider != "" && cfg.API.Provider != "openai" {
		provider = cfg.API.Provider
	}

	return &LLMClient{
		baseURL:          baseURL,
		provider:         provider,
		apiKey:           cfg.API.APIKey,
		model:            normalizeGeminiModelID(cfg.Model),
		fallback:         cfg.Fallback.Effective(),
		params:           cfg.API.Params,
		probeMinInterval: 30 * time.Second,
		httpClient: &http.Client{
			// No global timeout here — each call uses context.WithTimeout
			// for precise per-call control. A global timeout would race with
			// streaming responses that can take several minutes.
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     120 * time.Second,
				// TLS handshake timeout prevents hung connections during setup.
				TLSHandshakeTimeout: 10 * time.Second,
				// ResponseHeaderTimeout is how long to wait for the server to
				// start sending response headers. For large contexts (many tool
				// results), providers can take 60-120s to begin streaming.
				ResponseHeaderTimeout: 180 * time.Second,
			},
		},
		logger: logger.With("component", "llm", "provider", provider),
	}
}

// detectProvider infers the provider from the base URL.
func detectProvider(baseURL string) string {
	switch {
	// OAuth providers (check first)
	case strings.Contains(baseURL, "chatgpt.com/backend-api"):
		return "chatgpt-oauth"
	case strings.Contains(baseURL, "cloudcode-pa.googleapis.com"):
		return "gemini-oauth"
	case strings.Contains(baseURL, "qwen.ai"):
		return "qwen-oauth"
	case strings.Contains(baseURL, "minimax.io"), strings.Contains(baseURL, "minimaxi.com"):
		return "minimax-oauth"
	// Standard providers
	case strings.Contains(baseURL, "z.ai/api/coding"):
		return "zai-coding"
	case strings.Contains(baseURL, "z.ai/api/paas"):
		return "zai"
	case strings.Contains(baseURL, "z.ai/api/anthropic"):
		return "zai-anthropic"
	case strings.Contains(baseURL, "anthropic.com"):
		return "anthropic"
	case strings.Contains(baseURL, "openai.com"):
		return "openai"
	case strings.Contains(baseURL, "openrouter.ai"):
		return "openrouter"
	case strings.Contains(baseURL, "api.x.ai"):
		return "xai"
	case strings.Contains(baseURL, "api.groq.com"):
		return "groq"
	case strings.Contains(baseURL, "cerebras.ai"):
		return "cerebras"
	case strings.Contains(baseURL, "mistral.ai"):
		return "mistral"
	case strings.Contains(baseURL, "generativelanguage.googleapis.com"):
		return "google"
	case strings.Contains(baseURL, "localhost:11434"),
		strings.Contains(baseURL, "127.0.0.1:11434"),
		strings.Contains(baseURL, "ollama"):
		return "ollama"
	case strings.Contains(baseURL, "localhost:1234"),
		strings.Contains(baseURL, "127.0.0.1:1234"),
		strings.Contains(baseURL, "lmstudio"):
		return "lmstudio"
	case strings.Contains(baseURL, "localhost:8000"),
		strings.Contains(baseURL, "127.0.0.1:8000"),
		strings.Contains(baseURL, "vllm"):
		return "vllm"
	case strings.Contains(baseURL, "huggingface.co"):
		return "huggingface"
	default:
		return "openai" // assume OpenAI-compatible
	}
}

// resolveAPIKey returns the API key to use for this client.
// Priority: 1) OAuth token (if OAuth provider), 2) explicitly set key,
// 3) provider-specific env var, 4) generic API_KEY.
func (c *LLMClient) resolveAPIKey() string {
	// Priority 0: OAuth token (if using OAuth provider)
	if c.oauthTokenManager != nil && strings.HasSuffix(c.provider, "-oauth") {
		baseProvider := strings.TrimSuffix(c.provider, "-oauth")
		if cred, err := c.oauthTokenManager.GetValidToken(baseProvider); err == nil {
			// The credential should have an AccessToken field
			if token, ok := cred.(interface{ GetAccessToken() string }); ok {
				return token.GetAccessToken()
			}
			c.logger.Warn("OAuth credential does not implement GetAccessToken", "provider", baseProvider)
		} else {
			c.logger.Warn("OAuth token not available, falling back to API key",
				"provider", baseProvider, "error", err)
		}
	}

	// Priority 1: Key explicitly set in config (backwards compat)
	if c.apiKey != "" {
		return c.apiKey
	}

	// Priority 2: Provider-specific env var (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.)
	keyName := GetProviderKeyName(c.provider)
	if key := os.Getenv(keyName); key != "" {
		return key
	}

	// Priority 3: Generic API_KEY fallback
	if key := os.Getenv("API_KEY"); key != "" {
		return key
	}

	return c.apiKey // returns empty string if nothing found
}

// SetOAuthTokenManager sets the OAuth token manager for this client.
func (c *LLMClient) SetOAuthTokenManager(tm OAuthTokenManager) {
	c.oauthTokenManager = tm
}

// IsOAuthProvider returns true if this client is configured for OAuth.
func (c *LLMClient) IsOAuthProvider() bool {
	return strings.HasSuffix(c.provider, "-oauth")
}

// OAuthBaseProvider returns the base provider name for OAuth providers.
func (c *LLMClient) OAuthBaseProvider() string {
	return strings.TrimSuffix(c.provider, "-oauth")
}

// normalizeGeminiModelID converts short Gemini model aliases to their full API names.
// This allows users to specify "gemini-3.1-pro" and get "gemini-3.1-pro-preview".
// If the model is not a Gemini model or doesn't need normalization, returns unchanged.
func normalizeGeminiModelID(model string) string {
	if !strings.HasPrefix(model, "gemini-") {
		return model
	}

	// Gemini 3.1 aliases
	switch model {
	case "gemini-3.1-pro":
		return "gemini-3.1-pro-preview"
	case "gemini-3.1-flash":
		return "gemini-3.1-flash-preview"
	}

	// Gemini 3 aliases
	switch model {
	case "gemini-3-pro":
		return "gemini-3-pro-preview"
	case "gemini-3-flash":
		return "gemini-3-flash-preview"
	}

	return model
}

// providerForModel returns a human-readable provider name for error messages.
// Uses the configured provider if available, otherwise infers from the model name.
func (c *LLMClient) providerForModel(model string) string {
	// If provider is set, use it (with nicer formatting)
	if c.provider != "" && c.provider != "openai" {
		switch c.provider {
		case "zai", "zai-coding", "zai-anthropic":
			return "Z.AI"
		case "anthropic":
			return "Anthropic"
		case "openai":
			return "OpenAI"
		case "openrouter":
			return "OpenRouter"
		case "xai":
			return "xAI"
		case "groq":
			return "Groq"
		case "cerebras":
			return "Cerebras"
		case "mistral":
			return "Mistral"
		case "google":
			return "Google"
		case "ollama":
			return "Ollama"
		case "lmstudio":
			return "LM Studio"
		case "vllm":
			return "vLLM"
		case "huggingface":
			return "Hugging Face"
		default:
			return c.provider
		}
	}

	// Fall back to inferring from model name
	modelLower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(modelLower, "gpt-"), strings.HasPrefix(modelLower, "o1-"), strings.HasPrefix(modelLower, "o3-"):
		return "OpenAI"
	case strings.HasPrefix(modelLower, "claude-"):
		return "Anthropic"
	case strings.HasPrefix(modelLower, "gemini-"):
		return "Google"
	case strings.HasPrefix(modelLower, "llama-"), strings.HasPrefix(modelLower, "mixtral-"), strings.HasPrefix(modelLower, "mistral-"):
		return "Mistral"
	case strings.HasPrefix(modelLower, "grok-"):
		return "xAI"
	case strings.HasPrefix(modelLower, "glm-"):
		return "Z.AI"
	default:
		return "LLM"
	}
}

// isAnthropicAPI returns true if the provider uses the Anthropic Messages API format.
func (c *LLMClient) isAnthropicAPI() bool {
	return c.provider == "zai-anthropic" || c.provider == "anthropic"
}

// chatEndpoint returns the chat completions URL for the configured provider.
func (c *LLMClient) chatEndpoint() string {
	if c.isAnthropicAPI() {
		return c.baseURL + "/v1/messages"
	}
	return c.baseURL + "/chat/completions"
}

// audioEndpoint returns the audio transcriptions URL.
// Only OpenAI and compatible providers support the /audio/transcriptions endpoint.
// For providers that don't support Whisper (Z.AI/GLM, Anthropic, xAI), we route
// to OpenAI's endpoint or the configured TranscriptionBaseURL.
func (c *LLMClient) audioEndpoint(media *MediaConfig) string {
	// If a specific transcription base URL is configured, use it.
	if media != nil && media.TranscriptionBaseURL != "" {
		return strings.TrimRight(media.TranscriptionBaseURL, "/") + "/audio/transcriptions"
	}

	// For providers that support Whisper natively, use the main base URL.
	if c.supportsWhisper() {
		return c.baseURL + "/audio/transcriptions"
	}

	// Fallback to OpenAI's endpoint for providers that don't support Whisper.
	return "https://api.openai.com/v1/audio/transcriptions"
}

// audioAPIKey returns the API key to use for audio transcription.
// If a specific transcription API key is configured, use it.
// Otherwise falls back to the main API key.
func (c *LLMClient) audioAPIKey(media *MediaConfig) string {
	if media != nil && media.TranscriptionAPIKey != "" {
		return media.TranscriptionAPIKey
	}
	return c.resolveAPIKey()
}

// supportsWhisper returns true if the provider natively supports
// the OpenAI Whisper-compatible /audio/transcriptions endpoint.
func (c *LLMClient) supportsWhisper() bool {
	switch c.provider {
	case "openai", "openrouter":
		return true
	default:
		return false
	}
}

// Provider returns the detected or configured provider name.
func (c *LLMClient) Provider() string {
	return c.provider
}

// ---------- Wire Types (OpenAI-compatible) ----------

// contentPart represents a single part of multimodal message content.
// Used for vision: array of {"type":"text","text":"..."} and {"type":"image_url","image_url":{"url":"data:..."}}.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL holds the URL (including data:...) and optional detail for vision.
type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// cacheControl marks a message or content block as cacheable (Anthropic prompt caching).
type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// chatMessage represents a message in the OpenAI chat format.
// Supports user, system, assistant (with optional tool_calls), and tool result messages.
// Content is either a string (text-only) or []contentPart (multimodal, e.g. image+text).
type chatMessage struct {
	Role         string        `json:"role"`
	Content      any           `json:"content"` // string or []contentPart
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	CacheControl *cacheControl `json:"cache_control,omitempty"` // Anthropic prompt caching
}

// chatRequest is the OpenAI-compatible chat completions request.
type chatRequest struct {
	Model               string           `json:"model"`
	Messages            []chatMessage    `json:"messages"`
	Tools               []ToolDefinition `json:"tools,omitempty"`
	Stream              bool             `json:"stream,omitempty"`
	Temperature         *float64         `json:"temperature,omitempty"`
	MaxTokens           *int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int             `json:"max_completion_tokens,omitempty"` // OpenAI o1/o3/o4/gpt-5 models
	ToolStream          *bool            `json:"tool_stream,omitempty"`           // Z.AI: real-time tool call streaming
}

// modelDefaults holds per-model/provider behavior overrides.
type modelDefaults struct {
	// SupportsTemperature indicates if the model accepts the temperature param.
	SupportsTemperature bool
	// DefaultTemperature is the default temperature to use (0 = omit).
	DefaultTemperature float64
	// MaxOutputTokens is the default max_tokens for output (0 = omit / let server decide).
	MaxOutputTokens int
	// SupportsTools indicates if the model supports function/tool calling.
	SupportsTools bool
	// UsesMaxCompletionTokens indicates the model requires max_completion_tokens instead of max_tokens.
	UsesMaxCompletionTokens bool
}

// getModelDefaults returns the known defaults for a given model and provider.
func getModelDefaults(model, provider string) modelDefaults {
	// Default: supports everything (OpenAI-compatible baseline).
	d := modelDefaults{
		SupportsTemperature: true,
		DefaultTemperature:  0.7,
		MaxOutputTokens:     0, // let server decide
		SupportsTools:       true,
	}

	switch {
	// ── OpenAI models ──
	// gpt-5-mini and gpt-5-nano only support default temperature (1.0)
	case strings.HasPrefix(model, "gpt-5-mini"), strings.HasPrefix(model, "gpt-5-nano"):
		d.SupportsTemperature = false // only default (1.0) supported
		d.MaxOutputTokens = 16384
		d.UsesMaxCompletionTokens = true
	case strings.HasPrefix(model, "gpt-5"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 16384
		d.UsesMaxCompletionTokens = true
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		d.SupportsTemperature = false // o-series only supports default (1.0)
		d.MaxOutputTokens = 100000
		d.UsesMaxCompletionTokens = true
	case strings.HasPrefix(model, "gpt-4o"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 16384
	case strings.HasPrefix(model, "gpt-4.5"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 16384

	// ── Anthropic models ──
	case strings.HasPrefix(model, "claude-opus-4"):
		d.DefaultTemperature = 1.0
		d.MaxOutputTokens = 16384
	case strings.HasPrefix(model, "claude-sonnet-4-6"),
		strings.HasPrefix(model, "claude-sonnet-4.6"):
		d.DefaultTemperature = 1.0
		d.MaxOutputTokens = 16384
	case strings.HasPrefix(model, "claude-sonnet-4"):
		d.DefaultTemperature = 1.0
		d.MaxOutputTokens = 16384
	case strings.HasPrefix(model, "claude-3"):
		d.DefaultTemperature = 1.0
		d.MaxOutputTokens = 4096

	// ── GLM models (Z.AI) ──
	case strings.HasPrefix(model, "glm-5"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 8192
	case strings.HasPrefix(model, "glm-4"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 4096

	// ── xAI (Grok) models ──
	case strings.HasPrefix(model, "grok"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 16384

	// ── Ollama / local models ──
	case strings.HasPrefix(model, "llama"),
		strings.HasPrefix(model, "mistral"),
		strings.HasPrefix(model, "qwen"),
		strings.HasPrefix(model, "gemma"),
		strings.HasPrefix(model, "phi"),
		strings.HasPrefix(model, "deepseek"),
		strings.HasPrefix(model, "codellama"),
		strings.HasPrefix(model, "command-r"):
		d.DefaultTemperature = 0.7
		d.MaxOutputTokens = 4096
	}

	// Provider-level overrides.
	switch provider {
	case "zai-anthropic":
		d.DefaultTemperature = 1.0
	case "ollama":
		// Ollama models generally support tools but have smaller context windows.
		// Let the server decide max tokens unless model-specific above.
		if d.MaxOutputTokens == 0 {
			d.MaxOutputTokens = 4096
		}
	}

	return d
}

// applyModelDefaults populates a chatRequest with model-specific defaults.
func (c *LLMClient) applyModelDefaults(req *chatRequest) {
	d := getModelDefaults(req.Model, c.provider)

	if d.SupportsTemperature && d.DefaultTemperature > 0 && req.Temperature == nil {
		t := d.DefaultTemperature
		req.Temperature = &t
	}
	// Set max output tokens - use max_completion_tokens for newer OpenAI models
	if d.MaxOutputTokens > 0 {
		if d.UsesMaxCompletionTokens {
			if req.MaxCompletionTokens == nil {
				req.MaxCompletionTokens = &d.MaxOutputTokens
			}
		} else {
			if req.MaxTokens == nil {
				req.MaxTokens = &d.MaxOutputTokens
			}
		}
	}
	// Strip tools if the model doesn't support them.
	if !d.SupportsTools {
		req.Tools = nil
	}

	// Prompt caching: mark system messages with cache_control for supported providers.
	// Anthropic and Z.AI (anthropic proxy) support prompt caching via cache_control.
	if c.supportsCacheControl() {
		c.applyPromptCaching(req)
	}

	// Z.AI tool_stream: enable real-time tool call streaming by default.
	// Opt-out via params.tool_stream: false.
	if c.isZAI() && req.ToolStream == nil {
		enabled := !c.paramBool("tool_stream_disabled")
		if enabled {
			req.ToolStream = &enabled
		}
	}
}

// supportsCacheControl returns true if the provider supports prompt caching.
func (c *LLMClient) supportsCacheControl() bool {
	switch c.provider {
	case "anthropic", "zai-anthropic":
		return true
	default:
		return false
	}
}

// isZAI returns true if the provider is Z.AI (GLM or Z.AI coding).
func (c *LLMClient) isZAI() bool {
	return c.provider == "zai" || c.provider == "zai-coding"
}

// setProviderHeaders adds provider-specific HTTP headers to the request.
// OpenRouter requires attribution headers; other providers may need custom auth.
func (c *LLMClient) setProviderHeaders(req *http.Request) {
	switch c.provider {
	case "openrouter":
		req.Header.Set("X-Title", "DevClaw")
		req.Header.Set("HTTP-Referer", "https://github.com/jholhewres/devclaw")
	}
}

// setAnthropicBetaHeaders adds opt-in beta headers for Anthropic models.
// Currently supports context1m (1M token context window) for Opus/Sonnet.
func (c *LLMClient) setAnthropicBetaHeaders(req *http.Request, model string) {
	if c.paramBool("context1m") && isAnthropic1MModel(model) {
		req.Header.Set("anthropic-beta", "context-1m-2025-08-07")
	}
}

// isAnthropic1MModel returns true if the model supports the 1M context beta.
func isAnthropic1MModel(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4") ||
		strings.HasPrefix(model, "claude-sonnet-4")
}

// paramBool reads a boolean parameter from the provider params map.
func (c *LLMClient) paramBool(key string) bool {
	if c.params == nil {
		return false
	}
	v, ok := c.params[key]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
}

// applyPromptCaching marks the system message and last user message with
// cache_control for providers that support prompt caching. This allows the
// provider to cache the system prompt across requests, reducing costs by up to
// 90% for repeated conversations with the same system prompt.
func (c *LLMClient) applyPromptCaching(req *chatRequest) {
	if len(req.Messages) == 0 {
		return
	}

	// Mark the system message as cacheable (it rarely changes).
	for i := range req.Messages {
		if req.Messages[i].Role == "system" {
			req.Messages[i].CacheControl = &cacheControl{Type: "ephemeral"}
			break
		}
	}

	// Mark the last user message as a cache breakpoint.
	// This creates a two-tier cache: stable system prompt + recent conversation prefix.
	userCount := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userCount++
			if userCount == 2 { // second-to-last user message = good breakpoint
				req.Messages[i].CacheControl = &cacheControl{Type: "ephemeral"}
				break
			}
		}
	}
}

// streamChoice represents a single choice in a streaming chunk.
type streamChoice struct {
	Index int `json:"index"`
	Delta struct {
		Content   string           `json:"content"`
		ToolCalls []streamToolCall `json:"tool_calls,omitempty"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// streamToolCall represents a tool call delta (partial; id, name, arguments come in chunks).
type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// streamResponse is the SSE chunk format.
type streamResponse struct {
	Choices []streamChoice `json:"choices"`
	Usage   *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// chatResponse is the OpenAI-compatible chat completions response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// ---------- Anthropic Messages API Types ----------

// anthropicRequest is the Anthropic Messages API request format.
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

// anthropicMessage is a message in the Anthropic format.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthropicContent
}

// anthropicContent represents a content block in Anthropic format.
type anthropicContent struct {
	Type      string          `json:"type"`                  // "text", "tool_use", "tool_result", "image"
	Text      string          `json:"text,omitempty"`        // for type=text
	ID        string          `json:"id,omitempty"`          // for type=tool_use
	Name      string          `json:"name,omitempty"`        // for type=tool_use
	Input     json.RawMessage `json:"input,omitempty"`       // for type=tool_use
	ToolUseID string          `json:"tool_use_id,omitempty"` // for type=tool_result
	Content   string          `json:"content,omitempty"`     // for type=tool_result (string shorthand)
	Source    *anthropicImage `json:"source,omitempty"`      // for type=image
}

// anthropicImage holds base64 image data for vision.
type anthropicImage struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg", "image/png"
	Data      string `json:"data"`
}

// anthropicTool is a tool definition in the Anthropic format.
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicResponse is the Anthropic Messages API response.
type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// anthropicStreamEvent is a Server-Sent Events chunk from the Anthropic streaming API.
type anthropicStreamEvent struct {
	Type         string             `json:"type"` // "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"
	Message      *anthropicResponse `json:"message,omitempty"`
	Index        int                `json:"index,omitempty"`
	ContentBlock *anthropicContent  `json:"content_block,omitempty"`
	Delta        *struct {
		Type        string `json:"type,omitempty"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"` // for thinking_delta events
		PartialJSON string `json:"partial_json,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Usage *struct {
		OutputTokens int `json:"output_tokens,omitempty"`
	} `json:"usage,omitempty"`
}

// convertToAnthropicRequest converts OpenAI-format messages and tools to Anthropic format.
func convertToAnthropicRequest(model string, messages []chatMessage, tools []ToolDefinition, temp *float64, maxTokens *int) *anthropicRequest {
	req := &anthropicRequest{
		Model:       model,
		MaxTokens:   8192,
		Temperature: temp,
	}
	if maxTokens != nil && *maxTokens > 0 {
		req.MaxTokens = *maxTokens
	}

	// Extract system message (Anthropic uses a top-level field, not a message).
	var anthropicMsgs []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			switch v := m.Content.(type) {
			case string:
				if req.System != "" {
					req.System += "\n\n"
				}
				req.System += v
			}
			continue
		}

		if m.Role == "tool" {
			// Tool results: merge into the previous user message or create a new one.
			toolResult := anthropicContent{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
			}
			switch v := m.Content.(type) {
			case string:
				toolResult.Content = v
			}
			// Wrap in a user message.
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "user",
				Content: []anthropicContent{toolResult},
			})
			continue
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Assistant with tool calls → content blocks.
			var blocks []anthropicContent
			// Text content first (if any).
			if content, ok := m.Content.(string); ok && content != "" {
				blocks = append(blocks, anthropicContent{Type: "text", Text: content})
			}
			// Tool use blocks.
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				})
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		// Regular user or assistant message.
		anthropicMsgs = append(anthropicMsgs, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Anthropic requires alternating user/assistant. Merge consecutive same-role messages.
	req.Messages = mergeConsecutiveAnthropicMessages(anthropicMsgs)

	// Convert tools.
	for _, t := range tools {
		req.Tools = append(req.Tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	return req
}

// mergeConsecutiveAnthropicMessages merges consecutive messages with the same role.
// Anthropic API requires strictly alternating user/assistant roles.
func mergeConsecutiveAnthropicMessages(msgs []anthropicMessage) []anthropicMessage {
	if len(msgs) == 0 {
		return msgs
	}
	result := []anthropicMessage{msgs[0]}
	for i := 1; i < len(msgs); i++ {
		last := &result[len(result)-1]
		if msgs[i].Role == last.Role {
			// Merge: convert both to content arrays and concatenate.
			lastBlocks := toAnthropicContentBlocks(last.Content)
			newBlocks := toAnthropicContentBlocks(msgs[i].Content)
			last.Content = append(lastBlocks, newBlocks...)
		} else {
			result = append(result, msgs[i])
		}
	}
	return result
}

// toAnthropicContentBlocks converts any content to []anthropicContent.
func toAnthropicContentBlocks(content any) []anthropicContent {
	switch v := content.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []anthropicContent{{Type: "text", Text: v}}
	case []anthropicContent:
		return v
	default:
		// Try JSON re-marshal for interface{} types from unmarshaling.
		data, err := json.Marshal(content)
		if err != nil {
			return nil
		}
		var blocks []anthropicContent
		if err := json.Unmarshal(data, &blocks); err != nil {
			return []anthropicContent{{Type: "text", Text: string(data)}}
		}
		return blocks
	}
}

// convertFromAnthropicResponse converts an Anthropic response to the internal LLMResponse format.
func convertFromAnthropicResponse(resp *anthropicResponse) *LLMResponse {
	var content string
	var toolCalls []ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if content != "" {
				content += "\n"
			}
			content += block.Text
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	finishReason := resp.StopReason
	switch finishReason {
	case "end_turn":
		finishReason = "stop"
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	return &LLMResponse{
		Content:      strings.TrimSpace(content),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		ModelUsed:    resp.Model,
		Usage: LLMUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// ---------- Tool Calling Types ----------

// ToolDefinition is an OpenAI-compatible tool definition for function calling.
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a callable function exposed to the LLM.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and serialized arguments from the LLM.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ---------- Response Types ----------

// LLMResponse holds the parsed response from a chat completion.
type LLMResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        LLMUsage
	ModelUsed    string // The model that actually produced the response
}

// LLMUsage holds token usage information from the API response.
type LLMUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ---------- Error Classification ----------

// LLMErrorKind classifies API errors for retry/fallback decisions.
// Granular classification enables smarter retry behavior.
type LLMErrorKind int

const (
	LLMErrorRetryable  LLMErrorKind = iota // generic retryable (transient 5xx)
	LLMErrorRateLimit                      // 429 — rate limited, should respect Retry-After
	LLMErrorOverloaded                     // 529 or "overloaded" in body
	LLMErrorTimeout                        // request timeout / deadline exceeded
	LLMErrorAuth                           // 401, 403 — invalid/expired API key
	LLMErrorBilling                        // 402 or billing-related in body
	LLMErrorContext                        // context_length_exceeded
	LLMErrorBadRequest                     // 400 — malformed request
	LLMErrorFatal                          // everything else
)

// String returns a human-readable label for the error kind.
func (k LLMErrorKind) String() string {
	switch k {
	case LLMErrorRetryable:
		return "retryable"
	case LLMErrorRateLimit:
		return "rate_limit"
	case LLMErrorOverloaded:
		return "overloaded"
	case LLMErrorTimeout:
		return "timeout"
	case LLMErrorAuth:
		return "auth"
	case LLMErrorBilling:
		return "billing"
	case LLMErrorContext:
		return "context"
	case LLMErrorBadRequest:
		return "bad_request"
	case LLMErrorFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// IsRetryableKind returns true if the error kind warrants retrying.
func (k LLMErrorKind) IsRetryableKind() bool {
	return k == LLMErrorRetryable || k == LLMErrorRateLimit || k == LLMErrorOverloaded || k == LLMErrorTimeout
}

// apiError captures HTTP status, body, and optional Retry-After for 429.
type apiError struct {
	statusCode    int
	body          string
	retryAfterSec int    // from Retry-After header, 0 if not set
	model         string // model that produced the error (for billing/auth errors)
	provider      string // provider name (e.g., "OpenAI", "Anthropic")
}

func (e *apiError) Error() string {
	// For billing/auth errors, include model and provider for user clarity.
	if e.statusCode == 402 || e.statusCode == 401 || e.statusCode == 403 {
		prefix := ""
		if e.provider != "" {
			prefix = e.provider
			if e.model != "" {
				prefix += " (" + e.model + ")"
			}
			prefix += ": "
		} else if e.model != "" {
			prefix = e.model + ": "
		}
		return fmt.Sprintf("%sAPI returned %d: %s", prefix, e.statusCode, truncate(e.body, 200))
	}
	return fmt.Sprintf("API returned %d: %s", e.statusCode, truncate(e.body, 200))
}

// classifyAPIError determines the error kind from status code and response body.
// Classifies: rate_limit, billing, auth,
// overloaded, timeout, context, transient HTTP.
func classifyAPIError(statusCode int, body string) LLMErrorKind {
	bodyLower := strings.ToLower(body)

	// Context overflow — highest priority check.
	if strings.Contains(bodyLower, "context_length_exceeded") ||
		strings.Contains(bodyLower, "maximum context length") {
		return LLMErrorContext
	}

	// Billing / quota exhausted.
	if statusCode == 402 ||
		strings.Contains(bodyLower, "billing") ||
		strings.Contains(bodyLower, "quota") ||
		strings.Contains(bodyLower, "insufficient_quota") ||
		strings.Contains(bodyLower, "payment required") {
		return LLMErrorBilling
	}

	// Rate limit.
	if statusCode == 429 ||
		strings.Contains(bodyLower, "rate_limit") ||
		strings.Contains(bodyLower, "rate limit") ||
		strings.Contains(bodyLower, "too many requests") {
		return LLMErrorRateLimit
	}

	// Overloaded.
	if statusCode == 529 ||
		strings.Contains(bodyLower, "overloaded") ||
		strings.Contains(bodyLower, "capacity") {
		return LLMErrorOverloaded
	}

	// Timeout.
	if strings.Contains(bodyLower, "timeout") ||
		strings.Contains(bodyLower, "deadline") ||
		strings.Contains(bodyLower, "timed out") {
		return LLMErrorTimeout
	}

	switch statusCode {
	case 400:
		return LLMErrorBadRequest
	case 401, 403:
		return LLMErrorAuth
	case 500, 502, 503, 521, 522, 523, 524:
		return LLMErrorRetryable
	default:
		if statusCode >= 500 {
			return LLMErrorRetryable
		}
		return LLMErrorFatal
	}
}

// ---------- Public Methods ----------

// Complete sends a simple chat completion request (no tools) and returns the text.
// Convenience wrapper around CompleteWithTools for non-agentic use cases.
func (c *LLMClient) Complete(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, error) {
	messages := make([]chatMessage, 0, len(history)*2+2)

	if systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, entry := range history {
		messages = append(messages, chatMessage{
			Role:    "user",
			Content: entry.UserMessage,
		})
		if entry.AssistantResponse != "" {
			messages = append(messages, chatMessage{
				Role:    "assistant",
				Content: entry.AssistantResponse,
			})
		}
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: userMessage,
	})

	resp, err := c.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// CompleteWithVision sends an image plus optional text to the LLM vision API
// and returns the model's description or response.
// imageBase64 is the raw base64-encoded image bytes (without data URL prefix).
// mimeType is e.g. "image/jpeg", "image/png".
// detail is "auto", "low", or "high" (empty defaults to "auto").
// CompleteWithVision sends an image plus optional text to a vision-capable model.
// visionModel overrides the model; if empty, uses the main chat model.
func (c *LLMClient) CompleteWithVision(ctx context.Context, systemPrompt, imageBase64, mimeType, userPrompt, detail string, visionModel ...string) (string, error) {
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, imageBase64)
	if detail == "" {
		detail = "auto"
	}

	parts := []contentPart{
		{
			Type: "image_url",
			ImageURL: &imageURL{
				URL:    dataURL,
				Detail: detail,
			},
		},
	}
	if userPrompt != "" {
		parts = append([]contentPart{{Type: "text", Text: userPrompt}}, parts...)
	}

	messages := make([]chatMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: parts,
	})

	model := c.model
	if len(visionModel) > 0 && visionModel[0] != "" {
		model = visionModel[0]
	}

	resp, err := c.completeOnce(ctx, model, messages, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// TranscribeAudio sends audio data to a Whisper-compatible API and returns the transcript.
// filename is used as the form field name (e.g. "audio.ogg", "voice.mp3").
// model defaults to "whisper-1" if empty.
// media is optional; if provided, it may override the transcription endpoint and API key
// for providers that don't natively support Whisper (e.g. Z.AI/GLM, Anthropic).
func (c *LLMClient) TranscribeAudio(ctx context.Context, audioData []byte, filename, model string, media ...MediaConfig) (string, error) {
	if filename == "" {
		filename = "audio.webm"
	}
	if model == "" {
		model = "whisper-1"
	}

	// Resolve media config for transcription routing.
	var mediaCfg *MediaConfig
	if len(media) > 0 {
		m := media[0]
		mediaCfg = &m
	}

	endpoint := c.audioEndpoint(mediaCfg)
	apiKey := c.audioAPIKey(mediaCfg)

	// If the provider doesn't support Whisper and there's no separate API key,
	// the main API key likely won't work at OpenAI's endpoint.
	if !c.supportsWhisper() && (mediaCfg == nil || mediaCfg.TranscriptionAPIKey == "") && (mediaCfg == nil || mediaCfg.TranscriptionBaseURL == "") {
		c.logger.Warn("current provider does not support Whisper audio transcription",
			"provider", c.provider,
			"hint", "set media.transcription_base_url and media.transcription_api_key in config, or use OPENAI_API_KEY env var",
		)
		// Try OPENAI_API_KEY from environment as last resort.
		if envKey := envOrEmpty("OPENAI_API_KEY"); envKey != "" {
			apiKey = envKey
			c.logger.Info("using OPENAI_API_KEY from environment for transcription")
		} else {
			return "", fmt.Errorf(
				"audio transcription not available: provider %q does not support Whisper. "+
					"Configure media.transcription_api_key with an OpenAI API key, or set OPENAI_API_KEY env var",
				c.provider,
			)
		}
	}

	// Z.AI GLM-ASR only supports .wav and .mp3. Convert other formats.
	if needsAudioConversion(model, filename) {
		converted, convErr := convertAudioToMP3(ctx, audioData, filename)
		if convErr != nil {
			c.logger.Warn("audio conversion failed, sending original", "error", convErr)
		} else {
			audioData = converted
			filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".mp3"
			c.logger.Debug("converted audio for Z.AI ASR", "new_filename", filename, "new_size", len(audioData))
		}
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Add file part.
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("writing audio data: %w", err)
	}

	// Add model part.
	if err := w.WriteField("model", model); err != nil {
		return "", fmt.Errorf("writing model field: %w", err)
	}

	// Pass language hint when available.
	if mediaCfg != nil && mediaCfg.TranscriptionLanguage != "" {
		lang := mediaCfg.TranscriptionLanguage
		if strings.HasPrefix(model, "glm-asr") {
			// Z.AI uses "prompt" for language hints since it has no language param.
			langMap := map[string]string{
				"pt": "Portuguese", "en": "English", "es": "Spanish",
				"fr": "French", "de": "German", "ja": "Japanese",
				"ko": "Korean", "zh": "Chinese", "it": "Italian",
			}
			langName := langMap[lang]
			if langName == "" {
				langName = lang
			}
			_ = w.WriteField("prompt", "Language: "+langName)
		} else {
			_ = w.WriteField("language", lang)
		}
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)

	c.logger.Debug("sending audio transcription request",
		"filename", filename,
		"size_bytes", len(audioData),
		"endpoint", endpoint,
		"provider", c.provider,
		"using_separate_key", apiKey != c.resolveAPIKey(),
	)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	bodyStr := string(respBody)

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("transcription API error",
			"status", resp.StatusCode,
			"body", truncate(bodyStr, 500),
			"endpoint", endpoint,
		)
		return "", fmt.Errorf("transcription API returned %d: %s", resp.StatusCode, truncate(bodyStr, 200))
	}

	// Response is either plain text (transcript) or JSON with "text" field.
	text := bodyStr
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "{") {
		var j struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(respBody, &j); err == nil && j.Text != "" {
			text = j.Text
		}
	}

	c.logger.Info("audio transcription done",
		"duration_ms", time.Since(start).Milliseconds(),
		"transcript_len", len(text),
	)

	return strings.TrimSpace(text), nil
}

// envOrEmpty returns the environment variable value or empty string.
func envOrEmpty(key string) string {
	return os.Getenv(key)
}

// completeOnce performs a single chat completion request. Returns *apiError on HTTP errors
// so the caller can classify and decide retry/fallback.
func (c *LLMClient) completeOnce(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	if c.isAnthropicAPI() {
		return c.completeOnceAnthropic(ctx, model, messages, tools)
	}
	return c.completeOnceOpenAI(ctx, model, messages, tools)
}

// completeOnceAnthropic performs a single request using the Anthropic Messages API.
func (c *LLMClient) completeOnceAnthropic(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	defaults := getModelDefaults(model, c.provider)
	var temp *float64
	if defaults.SupportsTemperature && defaults.DefaultTemperature > 0 {
		t := defaults.DefaultTemperature
		temp = &t
	}
	maxTok := defaults.MaxOutputTokens
	if maxTok == 0 {
		maxTok = 8192
	}

	reqBody := convertToAnthropicRequest(model, messages, tools, temp, &maxTok)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.chatEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	c.setAnthropicBetaHeaders(req, model)
	// Z.Ai Anthropic Proxy expects Authorization: Bearer; native Anthropic uses x-api-key.
	if c.provider == "zai-anthropic" {
		req.Header.Set("Authorization", "Bearer "+c.resolveAPIKey())
	} else {
		req.Header.Set("x-api-key", c.resolveAPIKey())
	}

	c.logger.Debug("sending anthropic chat completion",
		"model", model,
		"messages", len(reqBody.Messages),
		"tools", len(reqBody.Tools),
		"endpoint", endpoint,
		"system_len", len(reqBody.System),
	)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	duration := time.Since(start)
	bodyStr := string(respBody)

	if resp.StatusCode != http.StatusOK {
		apierr := &apiError{statusCode: resp.StatusCode, body: bodyStr}
		if resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if sec, err := strconv.Atoi(ra); err == nil && sec > 0 {
					apierr.retryAfterSec = sec
				}
			}
		}
		c.logger.Error("API error",
			"model", model,
			"status", resp.StatusCode,
			"body", truncate(bodyStr, 500),
		)
		return nil, apierr
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return nil, fmt.Errorf("parsing anthropic response: %w (body: %s)", err, truncate(bodyStr, 200))
	}

	if anthResp.Error != nil {
		kind := classifyAPIError(resp.StatusCode, anthResp.Error.Message)
		if kind == LLMErrorContext {
			return nil, &apiError{statusCode: 400, body: anthResp.Error.Message}
		}
		return nil, fmt.Errorf("API error: %s", anthResp.Error.Message)
	}

	result := convertFromAnthropicResponse(&anthResp)

	c.logger.Info("anthropic chat completion done",
		"model", model,
		"duration_ms", duration.Milliseconds(),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
		"finish_reason", result.FinishReason,
		"tool_calls", len(result.ToolCalls),
	)

	return result, nil
}

// completeOnceOpenAI performs a single request using the OpenAI chat completions API.
func (c *LLMClient) completeOnceOpenAI(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	reqBody := chatRequest{
		Model:    model,
		Messages: messages,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}
	c.applyModelDefaults(&reqBody)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.chatEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.resolveAPIKey())
	c.setProviderHeaders(req)

	c.logger.Debug("sending chat completion",
		"model", model,
		"messages", len(messages),
		"tools", len(tools),
		"endpoint", endpoint,
	)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	duration := time.Since(start)
	bodyStr := string(respBody)

	if resp.StatusCode != http.StatusOK {
		apierr := &apiError{statusCode: resp.StatusCode, body: bodyStr}
		if resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if sec, err := strconv.Atoi(ra); err == nil && sec > 0 {
					apierr.retryAfterSec = sec
				}
			}
		}
		c.logger.Error("API error",
			"model", model,
			"status", resp.StatusCode,
			"body", truncate(bodyStr, 500),
		)
		return nil, apierr
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if chatResp.Error != nil {
		// Error in JSON body - check for context_length_exceeded
		kind := classifyAPIError(resp.StatusCode, chatResp.Error.Message)
		if kind == LLMErrorContext {
			return nil, &apiError{statusCode: 400, body: chatResp.Error.Message}
		}
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from model")
	}

	choice := chatResp.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)

	c.logger.Info("chat completion done",
		"model", model,
		"duration_ms", duration.Milliseconds(),
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"finish_reason", choice.FinishReason,
		"tool_calls", len(choice.Message.ToolCalls),
	)

	return &LLMResponse{
		Content:      content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		ModelUsed:    model,
		Usage: LLMUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}

// isRetryable returns true if the error is retryable per fallback config.
func (c *LLMClient) isRetryable(statusCode int) bool {
	for _, code := range c.fallback.RetryOnStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// CompleteWithTools sends a chat completion request with optional tool definitions.
// Delegates to CompleteWithFallback for retry and model fallback.
// Returns a structured response that may include tool calls the LLM wants to execute.
func (c *LLMClient) CompleteWithTools(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	return c.CompleteWithToolsUsingModel(ctx, "", messages, tools)
}

// CompleteWithToolsUsingModel is like CompleteWithTools but uses modelOverride as the
// primary model when non-empty. Empty string means use the default config model.
func (c *LLMClient) CompleteWithToolsUsingModel(ctx context.Context, modelOverride string, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	return c.CompleteWithFallbackUsingModel(ctx, modelOverride, messages, tools)
}

// CompleteWithToolsStream sends a streaming chat completion request. For each text delta,
// onChunk is called. Tool calls are accumulated silently. Falls back to non-streaming
// if the provider does not support streaming or returns an error.
func (c *LLMClient) CompleteWithToolsStream(ctx context.Context, messages []chatMessage, tools []ToolDefinition, onChunk StreamCallback) (*LLMResponse, error) {
	return c.CompleteWithToolsStreamUsingModel(ctx, "", messages, tools, onChunk)
}

// CompleteWithToolsStreamUsingModel is like CompleteWithToolsStream but uses modelOverride
// when non-empty. Empty = use c.model. Includes retry for transient HTTP errors
// before falling back to non-streaming.
func (c *LLMClient) CompleteWithToolsStreamUsingModel(ctx context.Context, modelOverride string, messages []chatMessage, tools []ToolDefinition, onChunk StreamCallback) (*LLMResponse, error) {
	if c.resolveAPIKey() == "" && c.provider != "ollama" {
		return nil, fmt.Errorf("API key not configured. Set %s in vault or environment", GetProviderKeyName(c.provider))
	}

	model := c.model
	if modelOverride != "" {
		model = modelOverride
	}

	// Try streaming with 1 retry for transient errors.
	const maxStreamRetries = 1
	const transientRetryDelay = 2500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxStreamRetries; attempt++ {
		resp, err := c.completeOnceStream(ctx, model, messages, tools, onChunk)
		if err == nil {
			// Check if streaming returned empty content (some providers like Z.AI
			// may have SSE format incompatibilities that result in empty responses)
			if resp.Content == "" && len(resp.ToolCalls) == 0 && resp.Usage.CompletionTokens == 0 {
				c.logger.Warn("streaming returned empty response, falling back to non-streaming",
					"model", model,
					"provider", c.provider,
				)
				// Fall back to non-streaming
				return c.CompleteWithFallbackUsingModel(ctx, modelOverride, messages, tools)
			}
			return resp, nil
		}
		lastErr = err

		// Check if the error is transient (retryable HTTP status).
		if apierr, ok := err.(*apiError); ok && c.isRetryable(apierr.statusCode) {
			c.logger.Info("transient streaming error, retrying",
				"model", model,
				"attempt", attempt+1,
				"status", apierr.statusCode,
				"error", err,
			)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during stream retry: %w", ctx.Err())
			case <-time.After(transientRetryDelay):
				continue
			}
		}

		// Non-transient error: break out and fall back.
		break
	}

	// Fallback to non-streaming with full retry/fallback chain.
	c.logger.Debug("streaming failed, falling back to non-streaming", "error", lastErr)
	return c.CompleteWithFallbackUsingModel(ctx, modelOverride, messages, tools)
}

// completeOnceStream performs a single streaming chat completion. Uses SSE parsing.
func (c *LLMClient) completeOnceStream(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition, onChunk StreamCallback) (*LLMResponse, error) {
	if c.isAnthropicAPI() {
		return c.completeOnceStreamAnthropic(ctx, model, messages, tools, onChunk)
	}
	return c.completeOnceStreamOpenAI(ctx, model, messages, tools, onChunk)
}

// completeOnceStreamAnthropic handles Anthropic streaming with event types.
func (c *LLMClient) completeOnceStreamAnthropic(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition, onChunk StreamCallback) (*LLMResponse, error) {
	defaults := getModelDefaults(model, c.provider)
	var temp *float64
	if defaults.SupportsTemperature && defaults.DefaultTemperature > 0 {
		t := defaults.DefaultTemperature
		temp = &t
	}
	maxTok := defaults.MaxOutputTokens
	if maxTok == 0 {
		maxTok = 8192
	}

	reqBody := convertToAnthropicRequest(model, messages, tools, temp, &maxTok)
	reqBody.Stream = true

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.chatEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "text/event-stream")
	c.setAnthropicBetaHeaders(req, model)
	// Z.Ai Anthropic Proxy expects Authorization: Bearer; native Anthropic uses x-api-key.
	if c.provider == "zai-anthropic" {
		req.Header.Set("Authorization", "Bearer "+c.resolveAPIKey())
	} else {
		req.Header.Set("x-api-key", c.resolveAPIKey())
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &apiError{statusCode: resp.StatusCode, body: string(body)}
	}

	var contentBuilder strings.Builder
	toolCallsAccum := make(map[int]*ToolCall)       // index -> tool call being built
	toolArgsAccum := make(map[int]*strings.Builder) // index -> partial JSON args
	thinkingBlocks := make(map[int]bool)            // blockIdx -> true if this is a thinking block
	finishReason := ""
	var usage LLMUsage
	blockIdx := 0

	// thinkingActive tracks whether a thinking block is currently open.
	// Used to deduplicate thinking_end signals — ignore content_block_stop
	// for a thinking block if thinking has already been ended.
	thinkingActive := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var eventType string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		// Use eventType from the "event:" line if the JSON type is empty.
		evType := event.Type
		if evType == "" {
			evType = eventType
		}

		switch evType {
		case "message_start":
			if event.Message != nil {
				usage.PromptTokens = event.Message.Usage.InputTokens
			}

		// Native thinking_start event: marks the beginning of an extended
		// thinking block. Sent by some API versions alongside content_block_start.
		case "thinking_start":
			if !thinkingActive {
				thinkingActive = true
			}

		// Native thinking_delta event: contains partial thinking text.
		// We forward it to the onChunk callback so that partial text streaming
		// remains active even while the model is in reasoning mode.
		case "thinking_delta":
			if event.Delta != nil && event.Delta.Thinking != "" {
				if onChunk != nil {
					onChunk(event.Delta.Thinking)
				}
			}

		// Native thinking_end event: marks the end of an extended thinking block.
		// Deduplicate: only close if thinking is currently active.
		case "thinking_end":
			if thinkingActive {
				thinkingActive = false
			}

		case "content_block_start":
			// IMPORTANT: update blockIdx FIRST so that the tool call and its
			// args builder are stored at the correct index. Subsequent
			// content_block_delta and content_block_stop events use blockIdx
			// to look up the accumulator — if we update after storing, the
			// indices mismatch and tool arguments are silently lost.
			blockIdx = event.Index
			if event.ContentBlock != nil {
				switch event.ContentBlock.Type {
				case "tool_use":
					toolCallsAccum[blockIdx] = &ToolCall{
						ID:       event.ContentBlock.ID,
						Type:     "function",
						Function: FunctionCall{Name: event.ContentBlock.Name},
					}
					toolArgsAccum[blockIdx] = &strings.Builder{}
				case "thinking":
					// Mark this block index as a thinking block and open thinking.
					thinkingBlocks[blockIdx] = true
					if !thinkingActive {
						thinkingActive = true
					}
				}
			}

		case "content_block_delta":
			if event.Delta != nil {
				switch event.Delta.Type {
				case "text_delta":
					contentBuilder.WriteString(event.Delta.Text)
					if onChunk != nil {
						onChunk(event.Delta.Text)
					}
				case "thinking_delta":
					// Forward thinking deltas to the stream callback so that partial
					// text continues to flow to the UI even during reasoning. The
					// text itself is not included in the final assistant response.
					if event.Delta.Thinking != "" && onChunk != nil {
						onChunk(event.Delta.Thinking)
					}
				case "input_json_delta":
					if b, ok := toolArgsAccum[blockIdx]; ok {
						b.WriteString(event.Delta.PartialJSON)
					}
				}
			}

		case "content_block_stop":
			if thinkingBlocks[blockIdx] {
				// Deduplicate: only mark thinking as ended if still active.
				if thinkingActive {
					thinkingActive = false
				}
			} else if tc, ok := toolCallsAccum[blockIdx]; ok {
				// Finalize tool args.
				if b, ok := toolArgsAccum[blockIdx]; ok {
					tc.Function.Arguments = b.String()
					if tc.Function.Arguments == "" {
						tc.Function.Arguments = "{}"
					}
				}
			}

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				finishReason = event.Delta.StopReason
			}
			if event.Usage != nil {
				usage.CompletionTokens = event.Usage.OutputTokens
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	// Map Anthropic stop reasons to OpenAI finish reasons.
	switch finishReason {
	case "end_turn":
		finishReason = "stop"
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	// Build ordered tool calls.
	indices := make([]int, 0, len(toolCallsAccum))
	for k := range toolCallsAccum {
		indices = append(indices, k)
	}
	sort.Ints(indices)
	var toolCalls []ToolCall
	for _, i := range indices {
		if tc, ok := toolCallsAccum[i]; ok && (tc.ID != "" || tc.Function.Name != "") {
			toolCalls = append(toolCalls, *tc)
		}
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	c.logger.Info("anthropic streaming done",
		"model", model,
		"duration_ms", time.Since(start).Milliseconds(),
		"finish_reason", finishReason,
		"tool_calls", len(toolCalls),
	)

	return &LLMResponse{
		Content:      strings.TrimSpace(contentBuilder.String()),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		ModelUsed:    model,
		Usage:        usage,
	}, nil
}

// completeOnceStreamOpenAI performs streaming using the OpenAI SSE format.
func (c *LLMClient) completeOnceStreamOpenAI(ctx context.Context, model string, messages []chatMessage, tools []ToolDefinition, onChunk StreamCallback) (*LLMResponse, error) {
	reqBody := chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}
	c.applyModelDefaults(&reqBody)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.chatEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.resolveAPIKey())
	req.Header.Set("Accept", "text/event-stream")
	c.setProviderHeaders(req)

	c.logger.Debug("sending streaming chat completion",
		"model", model,
		"messages", len(messages),
		"tools", len(tools),
	)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		return nil, &apiError{statusCode: resp.StatusCode, body: bodyStr}
	}

	// Accumulated response
	var contentBuilder strings.Builder
	toolCallsAccum := make(map[int]*ToolCall) // index -> accumulated tool call
	finishReason := ""

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max line

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk streamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			c.logger.Debug("failed to parse SSE chunk, skipping", "payload", truncate(payload, 100), "error", err)
			continue
		}

		for _, choice := range chunk.Choices {
			// Text delta
			if choice.Delta.Content != "" {
				contentBuilder.WriteString(choice.Delta.Content)
				if onChunk != nil {
					onChunk(choice.Delta.Content)
				}
			}

			// Tool call deltas
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				acc, ok := toolCallsAccum[idx]
				if !ok {
					acc = &ToolCall{Type: "function"}
					toolCallsAccum[idx] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.Function.Arguments += tc.Function.Arguments
				}
			}

			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}

		// Usage in final chunk
		if chunk.Usage != nil {
			// Could capture usage if needed
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	content := strings.TrimSpace(contentBuilder.String())

	// Build ordered tool calls from accumulated map (by index)
	indices := make([]int, 0, len(toolCallsAccum))
	for k := range toolCallsAccum {
		indices = append(indices, k)
	}
	sort.Ints(indices)
	toolCalls := make([]ToolCall, 0, len(indices))
	for _, i := range indices {
		if acc, ok := toolCallsAccum[i]; ok && (acc.ID != "" || acc.Function.Name != "") {
			toolCalls = append(toolCalls, *acc)
		}
	}

	duration := time.Since(start)
	c.logger.Info("streaming chat completion done",
		"model", model,
		"duration_ms", duration.Milliseconds(),
		"finish_reason", finishReason,
		"tool_calls", len(toolCalls),
	)

	return &LLMResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		ModelUsed:    model,
		Usage:        LLMUsage{},
	}, nil
}

// CompleteWithFallback tries the primary model, then fallback models, with retry and
// exponential backoff on retryable errors. Returns the first successful response.
func (c *LLMClient) CompleteWithFallback(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	return c.CompleteWithFallbackUsingModel(ctx, "", messages, tools)
}

// --- Rate-limit cooldown auto-recovery ---

// setCooldown records that a model hit a rate limit and sets the cooldown expiry.
func (c *LLMClient) setCooldown(model string, retryAfterSec int) {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()

	duration := time.Duration(retryAfterSec) * time.Second
	if duration < 30*time.Second {
		duration = 60 * time.Second // minimum 1 minute cooldown
	}
	if duration > 10*time.Minute {
		duration = 10 * time.Minute // cap at 10 minutes
	}

	c.cooldownModel = model
	c.cooldownExpires = time.Now().Add(duration)
	c.logger.Info("model rate-limited, entering cooldown",
		"model", model,
		"cooldown_seconds", int(duration.Seconds()),
		"expires_at", c.cooldownExpires.Format(time.RFC3339),
	)
}

// shouldProbePrimary returns true if the primary model was rate-limited, the
// cooldown is near expiry (within 10s) or already expired, and we haven't
// probed too recently. This allows auto-recovery without restart.
func (c *LLMClient) shouldProbePrimary(primary string) bool {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()

	if c.cooldownModel != primary || c.cooldownExpires.IsZero() {
		return false // no active cooldown for this model
	}

	now := time.Now()

	// Only probe if cooldown is near expiry (within 10s) or already expired.
	timeUntilExpiry := time.Until(c.cooldownExpires)
	if timeUntilExpiry > 10*time.Second {
		return false
	}

	// Throttle probes to avoid storms.
	if now.Sub(c.lastProbeAt) < c.probeMinInterval {
		return false
	}

	return true
}

// markProbed records that a probe was attempted.
func (c *LLMClient) markProbed() {
	c.cooldownMu.Lock()
	c.lastProbeAt = time.Now()
	c.cooldownMu.Unlock()
}

// clearCooldown removes cooldown for a model (recovery successful).
func (c *LLMClient) clearCooldown(model string) {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()

	if c.cooldownModel == model {
		c.logger.Info("primary model recovered from rate-limit", "model", model)
		c.cooldownModel = ""
		c.cooldownExpires = time.Time{}
	}
}

// isInCooldown returns true if the given model is currently rate-limited.
func (c *LLMClient) isInCooldown(model string) bool {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()

	if c.cooldownModel != model {
		return false
	}
	return time.Now().Before(c.cooldownExpires)
}

// CompleteWithFallbackUsingModel is like CompleteWithFallback but uses modelOverride
// as the primary model when non-empty. Empty = use c.model.
// Includes auto-recovery: when the primary model hits a rate limit, subsequent
// calls use fallback models. Near cooldown expiry, a probe is sent to the
// primary model to check if it recovered. On success, cooldown is cleared.
func (c *LLMClient) CompleteWithFallbackUsingModel(ctx context.Context, modelOverride string, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	if c.resolveAPIKey() == "" && c.provider != "ollama" {
		return nil, fmt.Errorf("API key not configured. Set %s in vault or environment", GetProviderKeyName(c.provider))
	}

	primary := c.model
	if modelOverride != "" {
		primary = modelOverride
	}

	models := make([]string, 0, 1+len(c.fallback.Models))
	models = append(models, primary)
	models = append(models, c.fallback.Models...)

	initialBackoff := time.Duration(c.fallback.InitialBackoffMs) * time.Millisecond
	maxBackoff := time.Duration(c.fallback.MaxBackoffMs) * time.Millisecond

	// Auto-recovery probe: if the primary model was rate-limited and cooldown
	// is near expiry, try a probe call to see if it recovered before using
	// fallbacks. This avoids staying on fallback models indefinitely.
	if c.shouldProbePrimary(primary) {
		c.markProbed()
		c.logger.Info("probing primary model for recovery", "model", primary)

		probeCtx, probeCancel := context.WithTimeout(ctx, 30*time.Second)
		resp, err := c.completeOnce(probeCtx, primary, messages, tools)
		probeCancel()

		if err == nil {
			c.clearCooldown(primary)
			return resp, nil
		}

		// Probe failed — stay on fallback models.
		apierr, isAPI := err.(*apiError)
		if isAPI && apierr.statusCode == 429 {
			retryAfterSec := apierr.retryAfterSec
			if retryAfterSec <= 0 {
				retryAfterSec = 60
			}
			c.setCooldown(primary, retryAfterSec)
		}
		c.logger.Info("primary model still rate-limited", "model", primary, "error", err)
	}

	var lastErr error
	for _, model := range models {
		// Skip models currently in cooldown (rate-limited).
		if c.isInCooldown(model) {
			c.logger.Debug("skipping model in cooldown", "model", model)
			continue
		}

		for attempt := 0; attempt <= c.fallback.MaxRetries; attempt++ {
			resp, err := c.completeOnce(ctx, model, messages, tools)
			if err == nil {
				// If this is the primary model recovering, clear cooldown.
				if model == primary {
					c.clearCooldown(model)
				}
				return resp, nil
			}

			lastErr = err

			// Extract status code and body for classification
			statusCode := 0
			body := ""
			retryAfterSec := 0
			if apierr, ok := err.(*apiError); ok {
				statusCode = apierr.statusCode
				body = apierr.body
				retryAfterSec = apierr.retryAfterSec
			}
			kind := classifyAPIError(statusCode, body)

			// Rate limit: set cooldown and move to next model immediately.
			if kind == LLMErrorRateLimit {
				if retryAfterSec <= 0 {
					retryAfterSec = 60
				}
				c.setCooldown(model, retryAfterSec)
				break // skip remaining retries for this model
			}

			// Non-retryable: fail immediately (auth, billing, context, bad request)
			if !kind.IsRetryableKind() || !c.isRetryable(statusCode) {
				c.logger.Warn("non-retryable LLM error, failing immediately",
					"model", model,
					"attempt", attempt+1,
					"kind", kind.String(),
					"error", err,
				)
				// Enhance billing/auth errors with model info for user clarity.
				if kind == LLMErrorBilling || kind == LLMErrorAuth {
					provider := c.providerForModel(model)
					return nil, fmt.Errorf("%s (%s): %w", provider, model, err)
				}
				return nil, err
			}

			// Retryable but no more attempts for this model
			if attempt >= c.fallback.MaxRetries {
				c.logger.Warn("exhausted retries for model, trying next fallback",
					"model", model,
					"attempts", attempt+1,
					"error", err,
				)
				break
			}

			// Compute backoff: min(initial * 2^attempt, maxBackoff)
			backoff := initialBackoff
			for i := 0; i < attempt; i++ {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
					break
				}
			}

			// Respect Retry-After header for 429
			retryAfter := backoff
			if apierr, ok := err.(*apiError); ok && apierr.statusCode == 429 && apierr.retryAfterSec > 0 {
				serverDelay := time.Duration(apierr.retryAfterSec) * time.Second
				if serverDelay > maxBackoff {
					serverDelay = maxBackoff
				}
				if serverDelay > retryAfter {
					retryAfter = serverDelay
				}
			}

			c.logger.Info("retrying after retryable error",
				"model", model,
				"attempt", attempt+1,
				"next_attempt", attempt+2,
				"kind", kind.String(),
				"backoff_ms", retryAfter.Milliseconds(),
				"error", err,
			)

			// Wait with context cancellation
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			case <-time.After(retryAfter):
				// proceed to next attempt
			}
		}
	}

	return nil, fmt.Errorf("all models and retries exhausted: %w", lastErr)
}

// needsAudioConversion returns true if the audio format is not natively
// supported by the transcription model and should be converted to MP3.
func needsAudioConversion(model, filename string) bool {
	if !strings.HasPrefix(model, "glm-asr") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp3", ".wav":
		return false
	default:
		return true
	}
}

// convertAudioToMP3 uses ffmpeg to convert audio data to MP3 format.
func convertAudioToMP3(ctx context.Context, data []byte, filename string) ([]byte, error) {
	tmpIn, err := os.CreateTemp("", "dcaudio-*"+filepath.Ext(filename))
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpIn.Name())
	// Restrict to owner-only: audio data may contain sensitive content.
	if err := os.Chmod(tmpIn.Name(), 0o600); err != nil {
		tmpIn.Close()
		return nil, err
	}

	if _, err := tmpIn.Write(data); err != nil {
		tmpIn.Close()
		return nil, err
	}
	tmpIn.Close()

	// Pre-create the output file with os.CreateTemp so the name is random
	// and the file exists with owner-only permissions before ffmpeg writes to
	// it. We capture the inode to detect TOCTOU replacement after ffmpeg.
	tmpOutFile, err := os.CreateTemp("", "dcaudio-out-*.mp3")
	if err != nil {
		return nil, err
	}
	tmpOutPath := tmpOutFile.Name()
	defer os.Remove(tmpOutPath)
	if err := os.Chmod(tmpOutPath, 0o600); err != nil {
		tmpOutFile.Close()
		return nil, err
	}
	var preStat os.FileInfo
	if preStat, err = tmpOutFile.Stat(); err != nil {
		tmpOutFile.Close()
		return nil, err
	}
	tmpOutFile.Close()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn.Name(),
		"-vn", "-acodec", "libmp3lame", "-q:a", "4", tmpOutPath)
	cmd.Stderr = nil
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	// TOCTOU guard: confirm the output file is still the one we created.
	var postStat os.FileInfo
	if postStat, err = os.Stat(tmpOutPath); err != nil {
		return nil, fmt.Errorf("audio output temp file missing after ffmpeg: %w", err)
	}
	if !os.SameFile(preStat, postStat) {
		return nil, fmt.Errorf("audio output temp file inode changed — possible TOCTOU attack")
	}

	return os.ReadFile(tmpOutPath)
}
