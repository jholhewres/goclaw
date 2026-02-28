// Package copilot – media_registry.go provides a multi-provider registry
// for media processing (vision and transcription) with priority-based fallback.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

// MediaCapability represents a type of media processing.
type MediaCapability string

const (
	CapabilityVision        MediaCapability = "vision"
	CapabilityTranscription MediaCapability = "transcription"

	// perProviderTimeout limits each individual provider attempt during fallback.
	perProviderTimeout = 30 * time.Second
)

// MediaProviderConfig configures a single media provider.
type MediaProviderConfig struct {
	// Provider is the provider name (e.g. "openai", "anthropic", "gemini").
	Provider string `yaml:"provider"`

	// BaseURL is the API base URL.
	BaseURL string `yaml:"base_url"`

	// APIKey is the API key for this provider.
	APIKey string `yaml:"api_key"`

	// Model is the model to use (e.g. "gpt-4o", "claude-sonnet-4-20250514").
	Model string `yaml:"model"`

	// Priority determines the order (lower = tried first). Default: 0.
	Priority int `yaml:"priority"`
}

// mediaProviderEntry holds a provider config with its pre-built LLM client.
type mediaProviderEntry struct {
	config MediaProviderConfig
	client *LLMClient
}

// MediaRegistry holds multiple providers per capability and tries them
// in priority order with fallback on failure.
type MediaRegistry struct {
	vision        []mediaProviderEntry
	transcription []mediaProviderEntry
	concurrency   chan struct{} // semaphore for concurrency limiting
	logger        *slog.Logger
}

// NewMediaRegistry creates a registry from provider configs.
// concurrencyLimit limits simultaneous media API calls (0 = default 3).
func NewMediaRegistry(visionProviders, transcriptionProviders []MediaProviderConfig, concurrencyLimit int, logger *slog.Logger) *MediaRegistry {
	if concurrencyLimit <= 0 {
		concurrencyLimit = 3
	}

	r := &MediaRegistry{
		concurrency: make(chan struct{}, concurrencyLimit),
		logger:      logger.With("component", "media_registry"),
	}

	r.vision = buildEntries(visionProviders, logger)
	r.transcription = buildEntries(transcriptionProviders, logger)

	return r
}

// buildEntries creates sorted provider entries from configs.
func buildEntries(configs []MediaProviderConfig, logger *slog.Logger) []mediaProviderEntry {
	entries := make([]mediaProviderEntry, 0, len(configs))
	for _, cfg := range configs {
		client := newMediaLLMClient(cfg, logger)
		entries = append(entries, mediaProviderEntry{
			config: cfg,
			client: client,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].config.Priority < entries[j].config.Priority
	})
	return entries
}

// newMediaLLMClient creates a lightweight LLMClient for media API calls.
func newMediaLLMClient(cfg MediaProviderConfig, logger *slog.Logger) *LLMClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	provider := cfg.Provider
	if provider == "" {
		provider = detectProvider(baseURL)
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	return &LLMClient{
		baseURL:  baseURL,
		provider: provider,
		apiKey:   cfg.APIKey,
		model:    normalizeGeminiModelID(model),
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:          5,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 120 * time.Second,
			},
		},
		logger: logger.With("component", "llm_media", "provider", provider),
	}
}

// HasVisionProviders returns true if vision providers are configured.
func (r *MediaRegistry) HasVisionProviders() bool {
	return len(r.vision) > 0
}

// HasTranscriptionProviders returns true if transcription providers are configured.
func (r *MediaRegistry) HasTranscriptionProviders() bool {
	return len(r.transcription) > 0
}

// DescribeImageWithFallback tries vision providers in priority order until one succeeds.
func (r *MediaRegistry) DescribeImageWithFallback(ctx context.Context, systemPrompt, imageBase64, mimeType, userPrompt, detail string) (string, error) {
	if len(r.vision) == 0 {
		return "", fmt.Errorf("no vision providers configured")
	}

	r.acquire()
	defer r.release()

	var lastErr error
	for _, entry := range r.vision {
		r.logger.Debug("trying vision provider",
			"provider", entry.config.Provider,
			"model", entry.config.Model,
			"priority", entry.config.Priority,
		)

		providerCtx, cancel := context.WithTimeout(ctx, perProviderTimeout)
		result, err := entry.client.CompleteWithVision(providerCtx, systemPrompt, imageBase64, mimeType, userPrompt, detail)
		cancel()
		if err == nil {
			return result, nil
		}

		lastErr = err
		r.logger.Warn("vision provider failed, trying next",
			"provider", entry.config.Provider,
			"error", err,
		)
	}

	return "", fmt.Errorf("all vision providers failed (tried %d): %w", len(r.vision), lastErr)
}

// TranscribeAudioWithFallback tries transcription providers in priority order until one succeeds.
func (r *MediaRegistry) TranscribeAudioWithFallback(ctx context.Context, audioData []byte, filename, model string, mediaCfg MediaConfig) (string, error) {
	if len(r.transcription) == 0 {
		return "", fmt.Errorf("no transcription providers configured")
	}

	r.acquire()
	defer r.release()

	var lastErr error
	for _, entry := range r.transcription {
		r.logger.Debug("trying transcription provider",
			"provider", entry.config.Provider,
			"model", entry.config.Model,
			"priority", entry.config.Priority,
		)

		// Use the entry's model if set, otherwise fall back to the provided model.
		m := model
		if entry.config.Model != "" {
			m = entry.config.Model
		}

		providerCtx, cancel := context.WithTimeout(ctx, perProviderTimeout)
		result, err := entry.client.TranscribeAudio(providerCtx, audioData, filename, m, mediaCfg)
		cancel()
		if err == nil {
			return result, nil
		}

		lastErr = err
		r.logger.Warn("transcription provider failed, trying next",
			"provider", entry.config.Provider,
			"error", err,
		)
	}

	return "", fmt.Errorf("all transcription providers failed (tried %d): %w", len(r.transcription), lastErr)
}

// acquire blocks until a concurrency slot is available.
func (r *MediaRegistry) acquire() {
	r.concurrency <- struct{}{}
}

// release frees a concurrency slot.
func (r *MediaRegistry) release() {
	<-r.concurrency
}

var (
	globalMediaRegistry   *MediaRegistry
	globalMediaRegistryMu sync.RWMutex
)

// SetGlobalMediaRegistry sets the global media registry instance.
func SetGlobalMediaRegistry(r *MediaRegistry) {
	globalMediaRegistryMu.Lock()
	globalMediaRegistry = r
	globalMediaRegistryMu.Unlock()
}

// GetGlobalMediaRegistry returns the global media registry, if set.
func GetGlobalMediaRegistry() *MediaRegistry {
	globalMediaRegistryMu.RLock()
	defer globalMediaRegistryMu.RUnlock()
	return globalMediaRegistry
}
