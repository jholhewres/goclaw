package copilot

import (
	"testing"
	"time"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		baseURL  string
		expected string
	}{
		{"https://api.openai.com/v1", "openai"},
		{"https://api.openai.com", "openai"},
		{"https://api.anthropic.com/v1", "anthropic"},
		{"https://api.anthropic.com", "anthropic"},
		{"https://api.z.ai/api/coding", "zai-coding"},
		{"https://api.z.ai/api/paas", "zai"},
		{"https://api.z.ai/api/anthropic", "zai-anthropic"},
		{"https://openrouter.ai/api/v1", "openrouter"},
		{"https://api.x.ai/v1", "xai"},
		{"http://localhost:11434/v1", "ollama"},
		{"http://127.0.0.1:11434", "ollama"},
		{"http://myserver.com/ollama/v1", "ollama"},
		{"https://custom-llm.example.com/v1", "openai"}, // Default to openai-compatible
		{"https://api.example.com/chat", "openai"},      // Default to openai-compatible
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			result := detectProvider(tt.baseURL)
			if result != tt.expected {
				t.Errorf("detectProvider(%q) = %q, want %q", tt.baseURL, result, tt.expected)
			}
		})
	}
}

func TestLLMClientIsAnthropicAPI(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"anthropic", true},
		{"zai-anthropic", true},
		{"openai", false},
		{"zai", false},
		{"zai-coding", false},
		{"ollama", false},
		{"openrouter", false},
		{"xai", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{provider: tt.provider}
			result := client.isAnthropicAPI()
			if result != tt.expected {
				t.Errorf("isAnthropicAPI() for provider %q = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestLLMClientChatEndpoint(t *testing.T) {
	tests := []struct {
		baseURL  string
		provider string
		expected string
	}{
		{"https://api.openai.com/v1", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.anthropic.com", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"https://api.z.ai/api/coding", "zai-coding", "https://api.z.ai/api/coding/chat/completions"},
		{"https://api.z.ai/api/anthropic", "zai-anthropic", "https://api.z.ai/api/anthropic/v1/messages"},
		{"https://custom.example.com/api", "openai", "https://custom.example.com/api/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{
				baseURL:  tt.baseURL,
				provider: tt.provider,
			}
			result := client.chatEndpoint()
			if result != tt.expected {
				t.Errorf("chatEndpoint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLLMClientSupportsWhisper(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"openai", true},
		{"openrouter", true},
		{"ollama", false},
		{"anthropic", false},
		{"zai", false},
		{"zai-coding", false},
		{"zai-anthropic", false},
		{"xai", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{provider: tt.provider}
			result := client.supportsWhisper()
			if result != tt.expected {
				t.Errorf("supportsWhisper() for provider %q = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestLLMClientCooldownTracking(t *testing.T) {
	client := &LLMClient{
		provider:         "openai",
		probeMinInterval: 1 * time.Second,
	}

	t.Run("initial state has no cooldown", func(t *testing.T) {
		client.cooldownMu.Lock()
		expires := client.cooldownExpires
		client.cooldownMu.Unlock()

		if !expires.IsZero() {
			t.Error("expected no cooldown initially")
		}
	})

	t.Run("can set and check cooldown", func(t *testing.T) {
		client.cooldownMu.Lock()
		client.cooldownExpires = time.Now().Add(30 * time.Second)
		client.cooldownModel = "gpt-4"
		client.cooldownMu.Unlock()

		client.cooldownMu.Lock()
		if client.cooldownModel != "gpt-4" {
			t.Error("expected cooldown model to be set")
		}
		client.cooldownMu.Unlock()
	})
}
