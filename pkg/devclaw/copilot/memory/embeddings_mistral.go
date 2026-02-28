// Package memory – embeddings_mistral.go implements Mistral embedding provider.
// Uses the OpenAI-compatible /embeddings endpoint.
package memory

import (
	"context"
	"net/http"
)

const (
	defaultMistralBaseURL = "https://api.mistral.ai/v1"
	defaultMistralModel   = "mistral-embed"
	defaultMistralDims    = 1024
)

// MistralEmbedder generates embeddings using the Mistral API.
// Mistral uses a fully OpenAI-compatible request/response format.
type MistralEmbedder struct {
	cfg    openAICompatibleConfig
	client *http.Client
}

// NewMistralEmbedder creates a Mistral embedding provider.
func NewMistralEmbedder(cfg EmbeddingConfig) *MistralEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultMistralBaseURL
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = defaultMistralDims
	}
	model := cfg.Model
	if model == "" {
		model = defaultMistralModel
	}
	apiKey := resolveAPIKey(cfg.APIKey, "MISTRAL_API_KEY")

	return &MistralEmbedder{
		cfg: openAICompatibleConfig{
			name:       "mistral",
			apiKey:     apiKey,
			model:      model,
			dimensions: dims,
			baseURL:    baseURL,
		},
		client: newEmbedHTTPClient(),
	}
}

// Embed generates embeddings for a batch of texts.
func (e *MistralEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedOpenAICompatible(ctx, e.client, e.cfg, texts)
}

// Dimensions returns the output vector dimensionality.
func (e *MistralEmbedder) Dimensions() int { return e.cfg.dimensions }

// Name returns the provider name.
func (e *MistralEmbedder) Name() string { return "mistral" }

// Model returns the model name.
func (e *MistralEmbedder) Model() string { return e.cfg.model }
