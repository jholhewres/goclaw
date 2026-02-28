package copilot

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewMediaRegistry(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("empty providers", func(t *testing.T) {
		t.Parallel()
		r := NewMediaRegistry(nil, nil, 0, logger)
		if r.HasVisionProviders() {
			t.Error("HasVisionProviders should be false")
		}
		if r.HasTranscriptionProviders() {
			t.Error("HasTranscriptionProviders should be false")
		}
	})

	t.Run("with vision providers", func(t *testing.T) {
		t.Parallel()
		providers := []MediaProviderConfig{
			{Provider: "openai", APIKey: "key1", Model: "gpt-4o", Priority: 1},
			{Provider: "anthropic", APIKey: "key2", Model: "claude-sonnet-4-20250514", Priority: 0},
		}
		r := NewMediaRegistry(providers, nil, 2, logger)
		if !r.HasVisionProviders() {
			t.Error("HasVisionProviders should be true")
		}
		if r.HasTranscriptionProviders() {
			t.Error("HasTranscriptionProviders should be false")
		}
		// Verify priority ordering: anthropic (0) should come first.
		if len(r.vision) != 2 {
			t.Fatalf("vision entries = %d, want 2", len(r.vision))
		}
		if r.vision[0].config.Provider != "anthropic" {
			t.Errorf("first vision provider = %q, want %q", r.vision[0].config.Provider, "anthropic")
		}
		if r.vision[1].config.Provider != "openai" {
			t.Errorf("second vision provider = %q, want %q", r.vision[1].config.Provider, "openai")
		}
	})

	t.Run("with transcription providers", func(t *testing.T) {
		t.Parallel()
		providers := []MediaProviderConfig{
			{Provider: "openai", APIKey: "key1", Model: "whisper-1", Priority: 0},
		}
		r := NewMediaRegistry(nil, providers, 0, logger)
		if r.HasVisionProviders() {
			t.Error("HasVisionProviders should be false")
		}
		if !r.HasTranscriptionProviders() {
			t.Error("HasTranscriptionProviders should be true")
		}
	})

	t.Run("default concurrency limit", func(t *testing.T) {
		t.Parallel()
		r := NewMediaRegistry(nil, nil, 0, logger)
		if cap(r.concurrency) != 3 {
			t.Errorf("concurrency cap = %d, want 3", cap(r.concurrency))
		}
	})

	t.Run("custom concurrency limit", func(t *testing.T) {
		t.Parallel()
		r := NewMediaRegistry(nil, nil, 5, logger)
		if cap(r.concurrency) != 5 {
			t.Errorf("concurrency cap = %d, want 5", cap(r.concurrency))
		}
	})
}

func TestBuildEntries_SortsByPriority(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	configs := []MediaProviderConfig{
		{Provider: "c", Priority: 10},
		{Provider: "a", Priority: 0},
		{Provider: "b", Priority: 5},
	}
	entries := buildEntries(configs, logger)

	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	expected := []string{"a", "b", "c"}
	for i, e := range entries {
		if e.config.Provider != expected[i] {
			t.Errorf("entry[%d].Provider = %q, want %q", i, e.config.Provider, expected[i])
		}
	}
}

func TestNewMediaLLMClient(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		client := newMediaLLMClient(MediaProviderConfig{}, logger)
		if client.baseURL != "https://api.openai.com/v1" {
			t.Errorf("baseURL = %q", client.baseURL)
		}
		if client.model != "gpt-4o" {
			t.Errorf("model = %q", client.model)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		t.Parallel()
		client := newMediaLLMClient(MediaProviderConfig{
			Provider: "anthropic",
			BaseURL:  "https://api.anthropic.com/v1",
			APIKey:   "sk-test",
			Model:    "claude-sonnet-4-20250514",
		}, logger)
		if client.provider != "anthropic" {
			t.Errorf("provider = %q", client.provider)
		}
		if client.apiKey != "sk-test" {
			t.Errorf("apiKey = %q", client.apiKey)
		}
		if client.model != "claude-sonnet-4-20250514" {
			t.Errorf("model = %q", client.model)
		}
	})
}

func TestMediaRegistry_DescribeImageWithFallback_NoProviders(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewMediaRegistry(nil, nil, 0, logger)

	_, err := r.DescribeImageWithFallback(t.Context(), "", "base64data", "image/jpeg", "describe", "auto")
	if err == nil {
		t.Error("expected error for no vision providers")
	}
}

func TestMediaRegistry_TranscribeAudioWithFallback_NoProviders(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewMediaRegistry(nil, nil, 0, logger)

	_, err := r.TranscribeAudioWithFallback(t.Context(), []byte("audio"), "audio.ogg", "whisper-1", MediaConfig{})
	if err == nil {
		t.Error("expected error for no transcription providers")
	}
}

func TestMediaRegistry_ConcurrencyLimiting(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewMediaRegistry(nil, nil, 2, logger)

	// Acquire both slots.
	r.acquire()
	r.acquire()

	// Verify the semaphore is full (non-blocking check).
	select {
	case r.concurrency <- struct{}{}:
		t.Error("should not be able to acquire a third slot")
		<-r.concurrency // clean up
	default:
		// Expected: channel is full.
	}

	// Release one.
	r.release()

	// Now we should be able to acquire again.
	select {
	case r.concurrency <- struct{}{}:
		<-r.concurrency // clean up
	default:
		t.Error("should be able to acquire after release")
	}

	r.release() // release the other
}

func TestSetGetGlobalMediaRegistry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewMediaRegistry(nil, nil, 0, logger)

	SetGlobalMediaRegistry(r)
	got := GetGlobalMediaRegistry()
	if got != r {
		t.Error("GetGlobalMediaRegistry returned different instance")
	}
}
