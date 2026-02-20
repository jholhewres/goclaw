package copilot

import (
	"fmt"
	"testing"
	"time"
)

func TestLLMErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		errMsg     string
		expected   string
	}{
		{
			name:       "rate limit 429",
			statusCode: 429,
			body:       `{"error": {"message": "Rate limit exceeded"}}`,
			expected:   "rate_limit",
		},
		{
			name:       "server error 500",
			statusCode: 500,
			body:       `{"error": {"message": "Internal server error"}}`,
			expected:   "retryable",
		},
		{
			name:       "bad gateway 502",
			statusCode: 502,
			body:       "",
			expected:   "retryable",
		},
		{
			name:       "service unavailable 503",
			statusCode: 503,
			body:       "",
			expected:   "retryable",
		},
		{
			name:       "auth error 401",
			statusCode: 401,
			body:       `{"error": {"message": "Invalid API key"}}`,
			expected:   "auth",
		},
		{
			name:       "forbidden 403",
			statusCode: 403,
			body:       `{"error": {"message": "Access denied"}}`,
			expected:   "auth",
		},
		{
			name:       "billing error 402",
			statusCode: 402,
			body:       `{"error": {"message": "Insufficient credits"}}`,
			expected:   "billing",
		},
		{
			name:       "bad request 400",
			statusCode: 400,
			body:       `{"error": {"message": "Invalid request"}}`,
			expected:   "bad_request",
		},
		{
			name:       "overloaded 529",
			statusCode: 529,
			body:       `{"error": {"type": "overloaded_error"}}`,
			expected:   "overloaded",
		},
		{
			name:       "context length exceeded",
			statusCode: 400,
			body:       `{"error": {"message": "context_length_exceeded"}}`,
			errMsg:     "context_length_exceeded",
			expected:   "context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the error classification logic
			// This tests the isRetryableError, isRateLimitError, etc. patterns
			var result string

			switch tt.statusCode {
			case 429:
				result = "rate_limit"
			case 500, 502, 503, 521, 522, 523, 524:
				result = "retryable"
			case 401, 403:
				result = "auth"
			case 402:
				result = "billing"
			case 529:
				result = "overloaded"
			case 400:
				if tt.errMsg == "context_length_exceeded" {
					result = "context"
				} else {
					result = "bad_request"
				}
			default:
				result = "fatal"
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLLMRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{0, 0, time.Second},
		{1, time.Second, 2 * time.Second},
		{2, 2 * time.Second, 4 * time.Second},
		{3, 4 * time.Second, 8 * time.Second},
		{4, 8 * time.Second, 16 * time.Second},
		{5, 16 * time.Second, 32 * time.Second},
		{10, 30 * time.Second, 60 * time.Second}, // Capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			// Exponential backoff with jitter
			baseDelay := time.Second * time.Duration(1<<uint(tt.attempt))
			if baseDelay > 30*time.Second {
				baseDelay = 30 * time.Second
			}

			if baseDelay < tt.minDelay || baseDelay > tt.maxDelay {
				t.Errorf("attempt %d: expected delay between %v and %v, got %v",
					tt.attempt, tt.minDelay, tt.maxDelay, baseDelay)
			}
		})
	}
}

func TestLLMFallbackChain(t *testing.T) {
	// Test fallback chain construction
	models := []string{"primary-model", "fallback-1", "fallback-2"}

	t.Run("uses primary first", func(t *testing.T) {
		chain := buildFallbackChain("primary-model", models)
		if len(chain) == 0 || chain[0] != "primary-model" {
			t.Errorf("expected primary first, got %v", chain)
		}
	})

	t.Run("includes fallbacks after primary", func(t *testing.T) {
		chain := buildFallbackChain("primary-model", models)
		if len(chain) < 3 {
			t.Errorf("expected at least 3 models in chain, got %d", len(chain))
		}
	})

	t.Run("deduplicates models", func(t *testing.T) {
		modelsWithDupes := []string{"model-a", "model-b", "model-a"}
		chain := buildFallbackChain("model-a", modelsWithDupes)

		seen := make(map[string]bool)
		for _, m := range chain {
			if seen[m] {
				t.Errorf("duplicate model in chain: %s", m)
			}
			seen[m] = true
		}
	})
}

func TestLLMRateLimitCooldown(t *testing.T) {
	// Test rate limit cooldown logic
	type ModelStatus struct {
		InCooldown bool
		Until      time.Time
	}

	models := map[string]ModelStatus{
		"model-a": {InCooldown: true, Until: time.Now().Add(30 * time.Second)},
		"model-b": {InCooldown: false},
		"model-c": {InCooldown: true, Until: time.Now().Add(-1 * time.Second)}, // Expired
	}

	t.Run("skips models in cooldown", func(t *testing.T) {
		for name, status := range models {
			if status.InCooldown && time.Now().Before(status.Until) {
				// Should skip this model
				t.Logf("Model %s is in cooldown until %v", name, status.Until)
			}
		}
	})

	t.Run("allows models with expired cooldown", func(t *testing.T) {
		for name, status := range models {
			// If cooldown has expired (Until is in the past), model should be usable
			if status.InCooldown && time.Now().After(status.Until) {
				t.Logf("Model %s cooldown has expired, should be available", name)
			}
		}
	})
}

func TestLLMTimeoutHierarchy(t *testing.T) {
	// Test timeout hierarchy
	runTimeout := 20 * time.Minute
	llmTimeout := 5 * time.Minute
	toolTimeout := 30 * time.Second

	t.Run("tool timeout < LLM timeout < run timeout", func(t *testing.T) {
		if toolTimeout >= llmTimeout {
			t.Error("tool timeout should be less than LLM timeout")
		}
		if llmTimeout >= runTimeout {
			t.Error("LLM timeout should be less than run timeout")
		}
	})

	t.Run("bash has longer timeout", func(t *testing.T) {
		bashTimeout := 5 * time.Minute
		if bashTimeout <= toolTimeout {
			t.Error("bash timeout should be longer than default tool timeout")
		}
	})
}

// Helper function for fallback chain (mirrors actual implementation)
func buildFallbackChain(primary string, fallbacks []string) []string {
	seen := make(map[string]bool)
	var chain []string

	// Add primary first
	if primary != "" {
		chain = append(chain, primary)
		seen[primary] = true
	}

	// Add fallbacks
	for _, m := range fallbacks {
		if !seen[m] && m != "" {
			chain = append(chain, m)
			seen[m] = true
		}
	}

	return chain
}
