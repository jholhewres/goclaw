package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------- OpenAI-Compatible Provider Tests ----------

func TestOpenAIEmbedder(t *testing.T) {
	t.Parallel()

	srv := newMockOpenAIServer(t, "openai-model", [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(EmbeddingConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "openai-model",
	})

	if e.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", e.Name(), "openai")
	}
	if e.Model() != "openai-model" {
		t.Errorf("Model() = %q, want %q", e.Model(), "openai-model")
	}
	if e.Dimensions() != 1536 {
		t.Errorf("Dimensions() = %d, want 1536", e.Dimensions())
	}

	result, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Embed() returned %d embeddings, want 2", len(result))
	}
	assertFloat32Slice(t, result[0], []float32{0.1, 0.2, 0.3})
	assertFloat32Slice(t, result[1], []float32{0.4, 0.5, 0.6})
}

func TestVoyageEmbedder(t *testing.T) {
	t.Parallel()

	srv := newMockOpenAIServer(t, "voyage-3-lite", [][]float32{
		{0.7, 0.8, 0.9},
	})
	defer srv.Close()

	e := NewVoyageEmbedder(EmbeddingConfig{
		APIKey:  "voyage-key",
		BaseURL: srv.URL,
	})

	if e.Name() != "voyage" {
		t.Errorf("Name() = %q, want %q", e.Name(), "voyage")
	}
	if e.Model() != "voyage-3-lite" {
		t.Errorf("Model() = %q, want %q", e.Model(), "voyage-3-lite")
	}
	if e.Dimensions() != 1024 {
		t.Errorf("Dimensions() = %d, want 1024", e.Dimensions())
	}

	result, err := e.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Embed() returned %d embeddings, want 1", len(result))
	}
	assertFloat32Slice(t, result[0], []float32{0.7, 0.8, 0.9})
}

func TestMistralEmbedder(t *testing.T) {
	t.Parallel()

	srv := newMockOpenAIServer(t, "mistral-embed", [][]float32{
		{1.0, 2.0},
		{3.0, 4.0},
		{5.0, 6.0},
	})
	defer srv.Close()

	e := NewMistralEmbedder(EmbeddingConfig{
		APIKey:  "mistral-key",
		BaseURL: srv.URL,
	})

	if e.Name() != "mistral" {
		t.Errorf("Name() = %q, want %q", e.Name(), "mistral")
	}
	if e.Model() != "mistral-embed" {
		t.Errorf("Model() = %q, want %q", e.Model(), "mistral-embed")
	}

	result, err := e.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("Embed() returned %d embeddings, want 3", len(result))
	}
}

func TestOpenAICompatibleEmptyInput(t *testing.T) {
	t.Parallel()

	e := NewOpenAIEmbedder(EmbeddingConfig{APIKey: "key"})
	result, err := e.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed(nil) error: %v", err)
	}
	if result != nil {
		t.Errorf("Embed(nil) = %v, want nil", result)
	}

	result, err = e.Embed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("Embed([]) error: %v", err)
	}
	if result != nil {
		t.Errorf("Embed([]) = %v, want nil", result)
	}
}

func TestOpenAICompatibleAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintln(w, `{"error":{"message":"rate limit exceeded"}}`)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(EmbeddingConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := e.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Embed() should return error on 429")
	}
}

// ---------- Gemini Provider Tests ----------

func TestGeminiEmbedderSingle(t *testing.T) {
	t.Parallel()

	srv := newMockGeminiServer(t, [][]float32{{0.1, 0.2, 0.3}})
	defer srv.Close()

	e := NewGeminiEmbedder(EmbeddingConfig{
		APIKey:  "gemini-key",
		BaseURL: srv.URL,
		Model:   "gemini-embedding-001",
	})

	if e.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", e.Name(), "gemini")
	}

	result, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Embed() returned %d embeddings, want 1", len(result))
	}
	assertFloat32Slice(t, result[0], []float32{0.1, 0.2, 0.3})
}

func TestGeminiEmbedderBatch(t *testing.T) {
	t.Parallel()

	srv := newMockGeminiServer(t, [][]float32{{0.1, 0.2}, {0.3, 0.4}})
	defer srv.Close()

	e := NewGeminiEmbedder(EmbeddingConfig{
		APIKey:  "gemini-key",
		BaseURL: srv.URL,
	})

	result, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Embed() returned %d embeddings, want 2", len(result))
	}
}

func TestGeminiEmbedderEmpty(t *testing.T) {
	t.Parallel()

	e := NewGeminiEmbedder(EmbeddingConfig{APIKey: "key"})
	result, err := e.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed(nil) error: %v", err)
	}
	if result != nil {
		t.Errorf("Embed(nil) = %v, want nil", result)
	}
}

// ---------- Null Embedder Tests ----------

func TestNullEmbedder(t *testing.T) {
	t.Parallel()

	e := &NullEmbedder{}
	if e.Name() != "none" {
		t.Errorf("Name() = %q, want %q", e.Name(), "none")
	}
	if e.Model() != "none" {
		t.Errorf("Model() = %q, want %q", e.Model(), "none")
	}
	if e.Dimensions() != 0 {
		t.Errorf("Dimensions() = %d, want 0", e.Dimensions())
	}

	result, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if result != nil {
		t.Errorf("Embed() = %v, want nil", result)
	}
}

// ---------- Fallback Embedder Tests ----------

func TestFallbackEmbedder_PrimarySucceeds(t *testing.T) {
	t.Parallel()

	primary := &mockEmbedder{name: "primary", embeddings: [][]float32{{1, 2, 3}}}
	fallback := &mockEmbedder{name: "fallback", embeddings: [][]float32{{4, 5, 6}}}

	fb := NewFallbackEmbedder(primary, fallback, nil)

	result, err := fb.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	assertFloat32Slice(t, result[0], []float32{1, 2, 3})
	if fallback.called {
		t.Error("fallback should not be called when primary succeeds")
	}
}

func TestFallbackEmbedder_PrimaryFailsFallbackSucceeds(t *testing.T) {
	t.Parallel()

	primary := &mockEmbedder{name: "primary", err: fmt.Errorf("primary down")}
	fallback := &mockEmbedder{name: "fallback", embeddings: [][]float32{{4, 5, 6}}}

	fb := NewFallbackEmbedder(primary, fallback, nil)

	result, err := fb.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	assertFloat32Slice(t, result[0], []float32{4, 5, 6})
	if !fallback.called {
		t.Error("fallback should be called when primary fails")
	}
}

func TestFallbackEmbedder_BothFail(t *testing.T) {
	t.Parallel()

	primary := &mockEmbedder{name: "primary", err: fmt.Errorf("primary down")}
	fallback := &mockEmbedder{name: "fallback", err: fmt.Errorf("fallback down")}

	fb := NewFallbackEmbedder(primary, fallback, nil)

	_, err := fb.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Embed() should return error when both providers fail")
	}
}

func TestFallbackEmbedder_Metadata(t *testing.T) {
	t.Parallel()

	primary := &mockEmbedder{name: "openai", model: "text-embedding-3-small", dims: 1536}
	fallback := &mockEmbedder{name: "gemini", model: "gemini-embedding-001", dims: 768}

	fb := NewFallbackEmbedder(primary, fallback, nil)

	if fb.Name() != "fallback:openai" {
		t.Errorf("Name() = %q, want %q", fb.Name(), "fallback:openai")
	}
	if fb.Model() != "text-embedding-3-small" {
		t.Errorf("Model() = %q, want %q", fb.Model(), "text-embedding-3-small")
	}
	if fb.Dimensions() != 1536 {
		t.Errorf("Dimensions() = %d, want 1536", fb.Dimensions())
	}
}

// ---------- Factory Tests ----------

func TestNewEmbeddingProvider_NoneReturnsNull(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "none"})
	if _, ok := p.(*NullEmbedder); !ok {
		t.Errorf("Provider 'none' should return *NullEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_UnknownReturnsNull(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "unknown-provider"})
	if _, ok := p.(*NullEmbedder); !ok {
		t.Errorf("Unknown provider should return *NullEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_OpenAI(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "openai", APIKey: "key"})
	if _, ok := p.(*OpenAIEmbedder); !ok {
		t.Errorf("Provider 'openai' should return *OpenAIEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_Gemini(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "gemini", APIKey: "key"})
	if _, ok := p.(*GeminiEmbedder); !ok {
		t.Errorf("Provider 'gemini' should return *GeminiEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_Google(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "google", APIKey: "key"})
	if _, ok := p.(*GeminiEmbedder); !ok {
		t.Errorf("Provider 'google' should return *GeminiEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_Voyage(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "voyage", APIKey: "key"})
	if _, ok := p.(*VoyageEmbedder); !ok {
		t.Errorf("Provider 'voyage' should return *VoyageEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_Mistral(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{Provider: "mistral", APIKey: "key"})
	if _, ok := p.(*MistralEmbedder); !ok {
		t.Errorf("Provider 'mistral' should return *MistralEmbedder, got %T", p)
	}
}

func TestNewEmbeddingProvider_WithFallback(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{
		Provider:       "openai",
		APIKey:         "key",
		Fallback:       "gemini",
		FallbackAPIKey: "gkey",
	})
	fb, ok := p.(*FallbackEmbedder)
	if !ok {
		t.Fatalf("Provider with fallback should return *FallbackEmbedder, got %T", p)
	}
	if fb.primary.Name() != "openai" {
		t.Errorf("primary.Name() = %q, want %q", fb.primary.Name(), "openai")
	}
	if fb.fallback.Name() != "gemini" {
		t.Errorf("fallback.Name() = %q, want %q", fb.fallback.Name(), "gemini")
	}
}

func TestNewEmbeddingProvider_FallbackNoneIgnored(t *testing.T) {
	t.Parallel()

	p := NewEmbeddingProvider(EmbeddingConfig{
		Provider: "openai",
		APIKey:   "key",
		Fallback: "none",
	})
	if _, ok := p.(*FallbackEmbedder); ok {
		t.Error("Fallback 'none' should not wrap with FallbackEmbedder")
	}
}

// ---------- Auto-Select Tests ----------

func TestAutoEmbedder_WithAPIKeyAndOpenAIBaseURL(t *testing.T) {
	t.Parallel()

	p := newAutoEmbedder(EmbeddingConfig{
		APIKey:  "key",
		BaseURL: "https://api.openai.com/v1",
	})
	if _, ok := p.(*OpenAIEmbedder); !ok {
		t.Errorf("auto with OpenAI URL should return *OpenAIEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_WithAPIKeyAndGeminiBaseURL(t *testing.T) {
	t.Parallel()

	p := newAutoEmbedder(EmbeddingConfig{
		APIKey:  "key",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
	})
	if _, ok := p.(*GeminiEmbedder); !ok {
		t.Errorf("auto with Gemini URL should return *GeminiEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_WithAPIKeyAndVoyageBaseURL(t *testing.T) {
	t.Parallel()

	p := newAutoEmbedder(EmbeddingConfig{
		APIKey:  "key",
		BaseURL: "https://api.voyageai.com/v1",
	})
	if _, ok := p.(*VoyageEmbedder); !ok {
		t.Errorf("auto with Voyage URL should return *VoyageEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_WithAPIKeyAndMistralBaseURL(t *testing.T) {
	t.Parallel()

	p := newAutoEmbedder(EmbeddingConfig{
		APIKey:  "key",
		BaseURL: "https://api.mistral.ai/v1",
	})
	if _, ok := p.(*MistralEmbedder); !ok {
		t.Errorf("auto with Mistral URL should return *MistralEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_WithAPIKeyNoURL(t *testing.T) {
	t.Parallel()

	p := newAutoEmbedder(EmbeddingConfig{APIKey: "key"})
	if _, ok := p.(*OpenAIEmbedder); !ok {
		t.Errorf("auto with key but no URL should default to *OpenAIEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_NoKeysReturnsNull(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel.
	for _, p := range autoProviderOrder {
		t.Setenv(p.envVar, "")
	}

	p := newAutoEmbedder(EmbeddingConfig{})
	if _, ok := p.(*NullEmbedder); !ok {
		t.Errorf("auto with no keys should return *NullEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_DetectsOpenAIEnvVar(t *testing.T) {
	for _, p := range autoProviderOrder {
		t.Setenv(p.envVar, "")
	}
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	p := newAutoEmbedder(EmbeddingConfig{})
	if _, ok := p.(*OpenAIEmbedder); !ok {
		t.Errorf("auto with OPENAI_API_KEY should return *OpenAIEmbedder, got %T", p)
	}
}

func TestAutoEmbedder_DetectsGoogleEnvVar(t *testing.T) {
	for _, p := range autoProviderOrder {
		t.Setenv(p.envVar, "")
	}
	t.Setenv("GOOGLE_API_KEY", "test-google-key")

	p := newAutoEmbedder(EmbeddingConfig{})
	if _, ok := p.(*GeminiEmbedder); !ok {
		t.Errorf("auto with GOOGLE_API_KEY should return *GeminiEmbedder, got %T", p)
	}
}

// ---------- Helpers ----------

func TestResolveAPIKey(t *testing.T) {
	if got := resolveAPIKey("explicit", "DUMMY_VAR"); got != "explicit" {
		t.Errorf("resolveAPIKey(explicit) = %q, want %q", got, "explicit")
	}

	t.Setenv("TEST_RESOLVE_KEY", "from-env")
	if got := resolveAPIKey("", "TEST_RESOLVE_KEY"); got != "from-env" {
		t.Errorf("resolveAPIKey(env) = %q, want %q", got, "from-env")
	}
}

func TestDefaultEmbeddingConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultEmbeddingConfig()
	if cfg.Provider != "none" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "none")
	}
	if !cfg.Cache {
		t.Error("Cache should be true by default")
	}
}

// ---------- Mock Helpers ----------

// mockEmbedder is a test double for EmbeddingProvider.
type mockEmbedder struct {
	name       string
	model      string
	dims       int
	embeddings [][]float32
	err        error
	called     bool
}

func (m *mockEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	m.called = true
	return m.embeddings, m.err
}
func (m *mockEmbedder) Dimensions() int { return m.dims }
func (m *mockEmbedder) Name() string    { return m.name }
func (m *mockEmbedder) Model() string   { return m.model }

// newMockOpenAIServer creates a test server that responds with OpenAI-compatible embeddings.
func newMockOpenAIServer(t *testing.T, expectedModel string, embeddings [][]float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if model, _ := req["model"].(string); model != expectedModel {
			t.Errorf("request model = %q, want %q", model, expectedModel)
		}

		data := make([]map[string]any, len(embeddings))
		for i, emb := range embeddings {
			floats := make([]float64, len(emb))
			for j, v := range emb {
				floats[j] = float64(v)
			}
			data[i] = map[string]any{
				"embedding": floats,
				"index":     i,
			}
		}

		resp := map[string]any{"data": data}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// newMockGeminiServer creates a test server that responds with Gemini-compatible embeddings.
func newMockGeminiServer(t *testing.T, embeddings [][]float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if len(embeddings) == 1 {
			// Single embed response.
			floats := make([]float64, len(embeddings[0]))
			for j, v := range embeddings[0] {
				floats[j] = float64(v)
			}
			resp := map[string]any{
				"embedding": map[string]any{
					"values": floats,
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Batch embed response.
		embList := make([]map[string]any, len(embeddings))
		for i, emb := range embeddings {
			floats := make([]float64, len(emb))
			for j, v := range emb {
				floats[j] = float64(v)
			}
			embList[i] = map[string]any{"values": floats}
		}
		resp := map[string]any{"embeddings": embList}
		json.NewEncoder(w).Encode(resp)
	}))
}

// assertFloat32Slice compares two float32 slices.
func assertFloat32Slice(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("len = %d, want %d", len(got), len(want))
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %f, want %f", i, got[i], want[i])
		}
	}
}
