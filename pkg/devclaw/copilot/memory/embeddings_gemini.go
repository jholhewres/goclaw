// Package memory – embeddings_gemini.go implements Google Gemini embedding provider.
// Uses the Gemini REST API with support for single and batch embedding requests.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiModel   = "gemini-embedding-001"
	defaultGeminiDims    = 768
)

// GeminiEmbedder generates embeddings using the Google Gemini API.
type GeminiEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	baseURL    string
	client     *http.Client
}

// NewGeminiEmbedder creates a Gemini embedding provider.
func NewGeminiEmbedder(cfg EmbeddingConfig) *GeminiEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	dims := cfg.Dimensions
	if dims <= 0 {
		dims = defaultGeminiDims
	}
	model := cfg.Model
	if model == "" {
		model = defaultGeminiModel
	}
	apiKey := resolveAPIKey(cfg.APIKey, "GOOGLE_API_KEY")

	return &GeminiEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: dims,
		baseURL:    baseURL,
		client:     newEmbedHTTPClient(),
	}
}

// geminiEmbedRequest is the Gemini embedContent request body.
type geminiEmbedRequest struct {
	Model                string       `json:"model"`
	Content              geminiContent `json:"content"`
	TaskType             string       `json:"taskType,omitempty"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

// geminiBatchEmbedRequest is the Gemini batchEmbedContents request body.
type geminiBatchEmbedRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

// geminiContent wraps text parts for the Gemini API.
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

// geminiPart is a single text part.
type geminiPart struct {
	Text string `json:"text"`
}

// geminiEmbedResponse is the single embedContent response.
type geminiEmbedResponse struct {
	Embedding *geminiEmbeddingValues `json:"embedding"`
	Error     *geminiError           `json:"error,omitempty"`
}

// geminiBatchEmbedResponse is the batchEmbedContents response.
type geminiBatchEmbedResponse struct {
	Embeddings []geminiEmbeddingValues `json:"embeddings"`
	Error      *geminiError            `json:"error,omitempty"`
}

// geminiEmbeddingValues holds the embedding vector.
type geminiEmbeddingValues struct {
	Values []float32 `json:"values"`
}

// geminiError represents a Gemini API error.
type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Embed generates embeddings for a batch of texts.
// Uses batchEmbedContents for multiple texts, embedContent for a single text.
func (e *GeminiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	if len(texts) == 1 {
		return e.embedSingle(ctx, texts[0])
	}

	return e.embedBatch(ctx, texts)
}

// embedSingle calls the single embedContent endpoint.
func (e *GeminiEmbedder) embedSingle(ctx context.Context, text string) ([][]float32, error) {
	reqBody := geminiEmbedRequest{
		Model:    "models/" + e.model,
		Content:  geminiContent{Parts: []geminiPart{{Text: text}}},
		TaskType: "RETRIEVAL_DOCUMENT",
	}
	if e.dimensions > 0 {
		reqBody.OutputDimensionality = e.dimensions
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal embed request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", e.baseURL, e.model, e.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini: create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: embed API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: embed API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result geminiEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("gemini: unmarshal embed response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("gemini: embed API error: %s", result.Error.Message)
	}
	if result.Embedding == nil {
		return nil, fmt.Errorf("gemini: empty embedding response")
	}

	return [][]float32{result.Embedding.Values}, nil
}

// embedBatch calls the batchEmbedContents endpoint.
func (e *GeminiEmbedder) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	requests := make([]geminiEmbedRequest, len(texts))
	for i, text := range texts {
		requests[i] = geminiEmbedRequest{
			Model:    "models/" + e.model,
			Content:  geminiContent{Parts: []geminiPart{{Text: text}}},
			TaskType: "RETRIEVAL_DOCUMENT",
		}
		if e.dimensions > 0 {
			requests[i].OutputDimensionality = e.dimensions
		}
	}

	reqBody := geminiBatchEmbedRequest{Requests: requests}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal batch embed request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", e.baseURL, e.model, e.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini: create batch embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: batch embed API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read batch embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: batch embed API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result geminiBatchEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("gemini: unmarshal batch embed response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("gemini: batch embed API error: %s", result.Error.Message)
	}

	embeddings := make([][]float32, len(texts))
	for i := range texts {
		if i < len(result.Embeddings) {
			embeddings[i] = result.Embeddings[i].Values
		}
	}

	return embeddings, nil
}

// Dimensions returns the output vector dimensionality.
func (e *GeminiEmbedder) Dimensions() int { return e.dimensions }

// Name returns the provider name.
func (e *GeminiEmbedder) Name() string { return "gemini" }

// Model returns the model name.
func (e *GeminiEmbedder) Model() string { return e.model }
