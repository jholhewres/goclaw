// Package memory – embeddings_voyage.go implements Voyage AI embedding provider.
// Uses the OpenAI-compatible /embeddings endpoint with Voyage-specific input_type field.
package memory

import (
	"context"
	"net/http"
)

const (
	defaultVoyageBaseURL = "https://api.voyageai.com/v1"
	defaultVoyageModel   = "voyage-3-lite"
	defaultVoyageDims    = 1024
)

// VoyageEmbedder generates embeddings using the Voyage AI API.
// Voyage uses an OpenAI-compatible format with an additional input_type field
// that improves relevance by distinguishing queries from documents.
type VoyageEmbedder struct {
	cfg    openAICompatibleConfig
	client *http.Client
}

// NewVoyageEmbedder creates a Voyage AI embedding provider.
func NewVoyageEmbedder(cfg EmbeddingConfig) *VoyageEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultVoyageBaseURL
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = defaultVoyageDims
	}
	model := cfg.Model
	if model == "" {
		model = defaultVoyageModel
	}
	apiKey := resolveAPIKey(cfg.APIKey, "VOYAGE_API_KEY")

	return &VoyageEmbedder{
		cfg: openAICompatibleConfig{
			name:       "voyage",
			apiKey:     apiKey,
			model:      model,
			dimensions: dims,
			baseURL:    baseURL,
			// Voyage-specific: input_type improves relevance for retrieval.
			// "document" is the default for indexing; "query" is used for search.
			// We default to nil (omit) since the provider works without it,
			// and distinguishing query vs document requires interface changes.
			extraBody: nil,
		},
		client: newEmbedHTTPClient(),
	}
}

// Embed generates embeddings for a batch of texts.
func (e *VoyageEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedOpenAICompatible(ctx, e.client, e.cfg, texts)
}

// Dimensions returns the output vector dimensionality.
func (e *VoyageEmbedder) Dimensions() int { return e.cfg.dimensions }

// Name returns the provider name.
func (e *VoyageEmbedder) Name() string { return "voyage" }

// Model returns the model name.
func (e *VoyageEmbedder) Model() string { return e.cfg.model }
