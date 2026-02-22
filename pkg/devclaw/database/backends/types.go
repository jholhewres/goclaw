// Package backends provides database backend implementations.
package backends

// VectorConfig configures vector search capabilities for database backends.
// Used by PostgreSQL (pgvector) and future vector-enabled backends.
type VectorConfig struct {
	// Enabled activates vector search support
	Enabled bool

	// Dimensions of the embedding vectors (default: 1536 for OpenAI embeddings)
	Dimensions int

	// IndexType specifies the index algorithm: "hnsw" or "ivfflat"
	IndexType string

	// IVFLists is the number of lists for IVFFlat index (default: 100)
	IVFLists int

	// HNSWM is the M parameter for HNSW index (default: 16)
	HNSWM int
}

// DefaultVectorConfig returns the default vector configuration.
func DefaultVectorConfig() VectorConfig {
	return VectorConfig{
		Enabled:    false,
		Dimensions: 1536,
		IndexType:  "hnsw",
		IVFLists:   100,
		HNSWM:      16,
	}
}

// Effective returns a copy with defaults applied for zero fields.
func (c VectorConfig) Effective() VectorConfig {
	out := c
	if out.Dimensions == 0 {
		out.Dimensions = 1536
	}
	if out.IndexType == "" {
		out.IndexType = "hnsw"
	}
	if out.IVFLists == 0 {
		out.IVFLists = 100
	}
	if out.HNSWM == 0 {
		out.HNSWM = 16
	}
	return out
}

// SearchResult represents a single vector search result with score and metadata.
type SearchResult struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Text     string         `json:"text,omitempty"`
}
