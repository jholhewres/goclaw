// Package memory – embeddings.go implements embedding generation for semantic search.
// Supports multiple providers: OpenAI, Gemini, Voyage, Mistral, and a zero-cost null fallback.
// Embeddings are cached by content hash + provider + model to avoid redundant API calls.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"
)

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	// Embed generates embeddings for a batch of texts.
	// Returns one float32 vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the output vectors.
	Dimensions() int

	// Name returns the provider name (for cache key derivation).
	Name() string

	// Model returns the model name (for cache key derivation).
	Model() string
}

// EmbeddingConfig configures the embedding provider.
type EmbeddingConfig struct {
	// Provider is the embedding provider ("openai", "gemini", "voyage", "mistral", "auto", "none").
	Provider string `yaml:"provider"`

	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string `yaml:"model"`

	// Dimensions is the output vector dimensionality (default: auto from model).
	Dimensions int `yaml:"dimensions"`

	// APIKey is the API key for the embedding provider. If empty, falls back to
	// the main LLM API key or provider-specific env vars.
	APIKey string `yaml:"api_key"`

	// BaseURL is the API base URL. If empty, uses the provider default.
	BaseURL string `yaml:"base_url"`

	// Cache enables embedding caching in SQLite (default: true).
	Cache bool `yaml:"cache"`

	// Fallback is the fallback provider when the primary fails ("openai", "gemini", etc., or "none").
	Fallback string `yaml:"fallback"`

	// FallbackAPIKey is the API key for the fallback provider.
	FallbackAPIKey string `yaml:"fallback_api_key"`

	// FallbackBaseURL is the base URL for the fallback provider.
	FallbackBaseURL string `yaml:"fallback_base_url"`

	// FallbackModel is the model for the fallback provider.
	FallbackModel string `yaml:"fallback_model"`
}

// DefaultEmbeddingConfig returns sensible defaults.
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		Provider:   "none",
		Model:      "text-embedding-3-small",
		Dimensions: 1536,
		Cache:      true,
	}
}

// ---------- OpenAI-Compatible Embedding Helper ----------

// openAICompatibleConfig holds configuration for any OpenAI-compatible embedding endpoint.
// OpenAI, Voyage AI, and Mistral all share this request/response format.
type openAICompatibleConfig struct {
	name       string
	apiKey     string
	model      string
	dimensions int
	baseURL    string
	extraBody  map[string]any // provider-specific fields (e.g. Voyage input_type)
}

// openaiEmbedResponse is the OpenAI-compatible embeddings API response.
type openaiEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// embedOpenAICompatible calls an OpenAI-compatible /embeddings endpoint.
// Shared by OpenAI, Voyage, and Mistral providers.
func embedOpenAICompatible(ctx context.Context, client *http.Client, cfg openAICompatibleConfig, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Build request body.
	body := map[string]any{
		"model": cfg.model,
		"input": texts,
	}
	if cfg.dimensions > 0 {
		body["dimensions"] = cfg.dimensions
	}
	// Merge provider-specific fields (e.g. Voyage input_type).
	maps.Copy(body, cfg.extraBody)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal embed request: %w", cfg.name, err)
	}

	endpoint := strings.TrimRight(cfg.baseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: create embed request: %w", cfg.name, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: embed API call: %w", cfg.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read embed response: %w", cfg.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: embed API error (status %d): %s", cfg.name, resp.StatusCode, string(respBody))
	}

	var result openaiEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("%s: unmarshal embed response: %w", cfg.name, err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("%s: embed API error: %s", cfg.name, result.Error.Message)
	}

	// Sort by index to match input order.
	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

// ---------- OpenAI Embedding Provider ----------

// OpenAIEmbedder generates embeddings using the OpenAI Embeddings API.
type OpenAIEmbedder struct {
	cfg    openAICompatibleConfig
	client *http.Client
}

// NewOpenAIEmbedder creates an OpenAI embedding provider.
func NewOpenAIEmbedder(cfg EmbeddingConfig) *OpenAIEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = 1536
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	apiKey := resolveAPIKey(cfg.APIKey, "OPENAI_API_KEY")
	return &OpenAIEmbedder{
		cfg: openAICompatibleConfig{
			name:       "openai",
			apiKey:     apiKey,
			model:      model,
			dimensions: dims,
			baseURL:    baseURL,
		},
		client: newEmbedHTTPClient(),
	}
}

// Embed generates embeddings for a batch of texts.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedOpenAICompatible(ctx, e.client, e.cfg, texts)
}

// Dimensions returns the output vector dimensionality.
func (e *OpenAIEmbedder) Dimensions() int { return e.cfg.dimensions }

// Name returns the provider name.
func (e *OpenAIEmbedder) Name() string { return "openai" }

// Model returns the model name.
func (e *OpenAIEmbedder) Model() string { return e.cfg.model }

// ---------- Null Embedding Provider ----------

// NullEmbedder is a no-op provider that disables semantic search.
// Used when no embedding provider is configured.
type NullEmbedder struct{}

// Embed returns nil (no embeddings).
func (e *NullEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

// Dimensions returns 0.
func (e *NullEmbedder) Dimensions() int { return 0 }

// Name returns "none".
func (e *NullEmbedder) Name() string { return "none" }

// Model returns "none".
func (e *NullEmbedder) Model() string { return "none" }

// ---------- Factory ----------

// NewEmbeddingProvider creates an embedding provider from config.
// When a fallback is configured, wraps with FallbackEmbedder for automatic failover.
func NewEmbeddingProvider(cfg EmbeddingConfig) EmbeddingProvider {
	primary := newEmbeddingProviderByName(cfg.Provider, cfg)

	// Wrap with fallback if configured.
	if cfg.Fallback != "" && cfg.Fallback != "none" {
		fallbackCfg := EmbeddingConfig{
			Provider:   cfg.Fallback,
			APIKey:     cfg.FallbackAPIKey,
			BaseURL:    cfg.FallbackBaseURL,
			Model:      cfg.FallbackModel,
			Dimensions: cfg.Dimensions,
			Cache:      cfg.Cache,
		}
		fallback := newEmbeddingProviderByName(cfg.Fallback, fallbackCfg)
		if _, isNull := fallback.(*NullEmbedder); !isNull {
			return NewFallbackEmbedder(primary, fallback, nil)
		}
	}

	return primary
}

// newEmbeddingProviderByName creates a provider by name.
func newEmbeddingProviderByName(name string, cfg EmbeddingConfig) EmbeddingProvider {
	switch strings.ToLower(name) {
	case "openai":
		return NewOpenAIEmbedder(cfg)
	case "gemini", "google":
		return NewGeminiEmbedder(cfg)
	case "voyage":
		return NewVoyageEmbedder(cfg)
	case "mistral":
		return NewMistralEmbedder(cfg)
	case "auto":
		return newAutoEmbedder(cfg)
	default:
		return &NullEmbedder{}
	}
}

// autoProviderOrder defines the priority for auto-selecting an embedding provider.
var autoProviderOrder = []struct {
	name   string
	envVar string
}{
	{"openai", "OPENAI_API_KEY"},
	{"gemini", "GOOGLE_API_KEY"},
	{"voyage", "VOYAGE_API_KEY"},
	{"mistral", "MISTRAL_API_KEY"},
}

// newAutoEmbedder creates an embedding provider by auto-detecting available API keys.
// Tries providers in order: OpenAI > Gemini > Voyage > Mistral.
// If the config has an explicit APIKey, tries to match it to a provider via BaseURL.
func newAutoEmbedder(cfg EmbeddingConfig) EmbeddingProvider {
	// If explicit API key is provided, try to detect provider from base URL.
	if cfg.APIKey != "" {
		if cfg.BaseURL != "" {
			lower := strings.ToLower(cfg.BaseURL)
			switch {
			case strings.Contains(lower, "openai"):
				cfg.Provider = "openai"
			case strings.Contains(lower, "googleapis") || strings.Contains(lower, "gemini"):
				cfg.Provider = "gemini"
			case strings.Contains(lower, "voyageai"):
				cfg.Provider = "voyage"
			case strings.Contains(lower, "mistral"):
				cfg.Provider = "mistral"
			default:
				// Unknown URL but has key — try OpenAI-compatible format.
				cfg.Provider = "openai"
			}
			return newEmbeddingProviderByName(cfg.Provider, cfg)
		}
		// Key but no URL — default to OpenAI.
		cfg.Provider = "openai"
		return newEmbeddingProviderByName("openai", cfg)
	}

	// No explicit key — check env vars in priority order.
	for _, p := range autoProviderOrder {
		if key := os.Getenv(p.envVar); key != "" {
			autoCfg := cfg
			autoCfg.APIKey = key
			autoCfg.Provider = p.name
			return newEmbeddingProviderByName(p.name, autoCfg)
		}
	}

	// No keys available — degrade to null (FTS-only).
	return &NullEmbedder{}
}

// ---------- Helpers ----------

// resolveAPIKey returns the configured key, falling back to the given env var.
func resolveAPIKey(configured, envVar string) string {
	if configured != "" {
		return configured
	}
	return os.Getenv(envVar)
}

// newEmbedHTTPClient creates a shared HTTP client for embedding providers.
func newEmbedHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// ---------- Fallback Embedder ----------

// FallbackEmbedder wraps a primary and fallback provider.
// On primary failure, automatically retries with the fallback.
// If both fail, returns an error; callers should degrade to FTS-only search.
type FallbackEmbedder struct {
	primary  EmbeddingProvider
	fallback EmbeddingProvider
	logger   *slog.Logger
}

// NewFallbackEmbedder creates a fallback-enabled embedder.
func NewFallbackEmbedder(primary, fallback EmbeddingProvider, logger *slog.Logger) *FallbackEmbedder {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackEmbedder{
		primary:  primary,
		fallback: fallback,
		logger:   logger,
	}
}

// Embed tries the primary provider, falling back on error.
func (f *FallbackEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result, err := f.primary.Embed(ctx, texts)
	if err == nil {
		return result, nil
	}

	f.logger.Warn("embedding primary failed, trying fallback",
		"primary", f.primary.Name(),
		"fallback", f.fallback.Name(),
		"error", err,
	)

	result, fallbackErr := f.fallback.Embed(ctx, texts)
	if fallbackErr == nil {
		return result, nil
	}

	f.logger.Warn("embedding fallback also failed, degrading to FTS-only",
		"fallback", f.fallback.Name(),
		"error", fallbackErr,
	)

	// Both failed — return nil to degrade to FTS-only search.
	return nil, fmt.Errorf("embedding: primary (%s) failed: %w; fallback (%s) failed: %v",
		f.primary.Name(), err, f.fallback.Name(), fallbackErr)
}

// Dimensions returns the primary provider's dimensions.
func (f *FallbackEmbedder) Dimensions() int { return f.primary.Dimensions() }

// Name returns "fallback:{primary}" — only the primary is used for cache keys
// since embeddings from different models are not compatible.
func (f *FallbackEmbedder) Name() string {
	return "fallback:" + f.primary.Name()
}

// Model returns the primary provider's model.
func (f *FallbackEmbedder) Model() string { return f.primary.Model() }
