package copilot

import (
	"os"
	"testing"
)

func TestGetProviderKeyName(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"openai", "OPENAI_API_KEY"},
		{"OPENAI", "OPENAI_API_KEY"},
		{"OpenAI", "OPENAI_API_KEY"},
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"google", "GOOGLE_API_KEY"},
		{"xai", "XAI_API_KEY"},
		{"groq", "GROQ_API_KEY"},
		{"zai", "ZAI_API_KEY"},
		{"mistral", "MISTRAL_API_KEY"},
		{"openrouter", "OPENROUTER_API_KEY"},
		{"cerebras", "CEREBRAS_API_KEY"},
		{"minimax", "MINIMAX_API_KEY"},
		{"huggingface", "HUGGINGFACE_API_KEY"},
		{"deepseek", "DEEPSEEK_API_KEY"},
		{"custom", "CUSTOM_API_KEY"},
		{"unknown", "API_KEY"},      // fallback
		{"", "API_KEY"},             // empty fallback
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			result := GetProviderKeyName(tt.provider)
			if result != tt.expected {
				t.Errorf("GetProviderKeyName(%q) = %q, want %q", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestProviderKeyNamesMap(t *testing.T) {
	// Verify all expected providers are in the map
	expectedProviders := []string{
		"openai", "anthropic", "google", "xai", "groq",
		"zai", "mistral", "openrouter", "cerebras", "minimax",
		"huggingface", "deepseek", "custom",
	}

	for _, provider := range expectedProviders {
		if _, ok := ProviderKeyNames[provider]; !ok {
			t.Errorf("ProviderKeyNames missing entry for %q", provider)
		}
	}

	// Verify all values end with _API_KEY (except custom)
	for provider, keyName := range ProviderKeyNames {
		if provider != "custom" && keyName != "CUSTOM_API_KEY" {
			if len(keyName) < 8 || keyName[len(keyName)-8:] != "_API_KEY" {
				t.Errorf("ProviderKeyNames[%q] = %q doesn't follow _API_KEY convention", provider, keyName)
			}
		}
	}
}

func TestResolveAPIKeyFromEnv(t *testing.T) {
	// Save original env
	origOpenAI := os.Getenv("OPENAI_API_KEY")
	origAPIKey := os.Getenv("API_KEY")
	defer func() {
		os.Setenv("OPENAI_API_KEY", origOpenAI)
		os.Setenv("API_KEY", origAPIKey)
	}()

	// Clear env
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("API_KEY")

	// Test 1: No key set
	client := &LLMClient{provider: "openai", apiKey: ""}
	if key := client.resolveAPIKey(); key != "" {
		t.Errorf("expected empty key when no env set, got %q", key)
	}

	// Test 2: Provider-specific env var
	os.Setenv("OPENAI_API_KEY", "test-openai-key")
	if key := client.resolveAPIKey(); key != "test-openai-key" {
		t.Errorf("expected key from OPENAI_API_KEY, got %q", key)
	}

	// Test 3: Config key takes priority
	clientWithConfig := &LLMClient{provider: "openai", apiKey: "config-key"}
	if key := clientWithConfig.resolveAPIKey(); key != "config-key" {
		t.Errorf("expected config key to take priority, got %q", key)
	}

	// Test 4: Generic API_KEY fallback
	os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("API_KEY", "generic-key")
	clientNoConfig := &LLMClient{provider: "openai", apiKey: ""}
	if key := clientNoConfig.resolveAPIKey(); key != "generic-key" {
		t.Errorf("expected generic API_KEY fallback, got %q", key)
	}

	// Test 5: Different provider uses different env var
	os.Unsetenv("API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	anthropicClient := &LLMClient{provider: "anthropic", apiKey: ""}
	if key := anthropicClient.resolveAPIKey(); key != "test-anthropic-key" {
		t.Errorf("expected key from ANTHROPIC_API_KEY, got %q", key)
	}
}
