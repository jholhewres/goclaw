package memory

import (
	"testing"
)

func TestJaccardSimilarity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a        map[string]bool
		b        map[string]bool
		expected float64
	}{
		{
			name:     "identical sets",
			a:        map[string]bool{"hello": true, "world": true},
			b:        map[string]bool{"hello": true, "world": true},
			expected: 1.0,
		},
		{
			name:     "no overlap",
			a:        map[string]bool{"foo": true, "bar": true},
			b:        map[string]bool{"baz": true, "qux": true},
			expected: 0.0,
		},
		{
			name:     "partial overlap",
			a:        map[string]bool{"hello": true, "world": true, "foo": true},
			b:        map[string]bool{"hello": true, "world": true, "bar": true},
			expected: 0.5, // 2 intersection / 4 union
		},
		{
			name:     "empty both",
			a:        map[string]bool{},
			b:        map[string]bool{},
			expected: 1.0,
		},
		{
			name:     "empty one",
			a:        map[string]bool{"hello": true},
			b:        map[string]bool{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jaccardSimilarity(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, got)
			}
		})
	}
}

func TestApplyMMR_Disabled(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "a.md", Text: "apple banana", Score: 1.0},
		{FileID: "b.md", Text: "cherry date", Score: 0.9},
		{FileID: "c.md", Text: "elderberry fig", Score: 0.8},
	}

	cfg := MMRConfig{Enabled: false, Lambda: 0.7}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 2)

	// When disabled, should return unchanged (but truncated to maxResults)
	if len(got) != 3 {
		t.Errorf("expected 3 results when disabled, got %d", len(got))
	}
	if got[0].FileID != "a.md" {
		t.Errorf("expected first result a.md, got %s", got[0].FileID)
	}
}

func TestApplyMMR_SelectsDiverseResults(t *testing.T) {
	t.Parallel()

	// Two very similar results and one different
	results := []SearchResult{
		{FileID: "a.md", Text: "apple banana cherry", Score: 1.0},
		{FileID: "b.md", Text: "apple banana date", Score: 0.95}, // Similar to a
		{FileID: "c.md", Text: "elephant fox giraffe", Score: 0.8}, // Very different
	}

	cfg := MMRConfig{Enabled: true, Lambda: 0.7}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 2)

	// Should select 'a' (highest relevance) and 'c' (most diverse)
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}

	// First should be 'a' (highest score)
	if got[0].FileID != "a.md" {
		t.Errorf("expected first result a.md, got %s", got[0].FileID)
	}

	// Second should be 'c' (most diverse) not 'b' (similar to a)
	if got[1].FileID != "c.md" {
		t.Errorf("expected second result c.md (diverse), got %s", got[1].FileID)
	}
}

func TestApplyMMR_RespectsLambda(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "a.md", Text: "apple banana", Score: 1.0},
		{FileID: "b.md", Text: "apple banana", Score: 0.6}, // Similar but lower score
		{FileID: "c.md", Text: "xylophone zebra", Score: 0.5}, // Different
	}

	// High lambda (0.9) = favor relevance over diversity
	cfgHigh := MMRConfig{Enabled: true, Lambda: 0.9}
	gotHigh := (&SQLiteStore{}).ApplyMMR(results, cfgHigh, 2)

	// With high lambda, might still pick similar high-scoring results
	if len(gotHigh) != 2 {
		t.Errorf("expected 2 results, got %d", len(gotHigh))
	}

	// Low lambda (0.3) = favor diversity over relevance
	cfgLow := MMRConfig{Enabled: true, Lambda: 0.3}
	gotLow := (&SQLiteStore{}).ApplyMMR(results, cfgLow, 2)

	// With low lambda, should strongly prefer diverse results
	if len(gotLow) != 2 {
		t.Errorf("expected 2 results, got %d", len(gotLow))
	}

	// First should always be highest relevance
	if gotLow[0].FileID != "a.md" {
		t.Errorf("expected first result a.md, got %s", gotLow[0].FileID)
	}
}

func TestApplyMMR_FewerResultsThanMax(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "a.md", Text: "apple", Score: 1.0},
	}

	cfg := MMRConfig{Enabled: true, Lambda: 0.7}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 5)

	// Should return all available results
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d", len(got))
	}
}

func TestApplyMMR_EmptyResults(t *testing.T) {
	t.Parallel()

	results := []SearchResult{}
	cfg := MMRConfig{Enabled: true, Lambda: 0.7}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 5)

	if len(got) != 0 {
		t.Errorf("expected 0 results, got %d", len(got))
	}
}

func TestApplyMMR_DefaultLambda(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "a.md", Text: "apple banana", Score: 1.0},
		{FileID: "b.md", Text: "cherry date", Score: 0.9},
	}

	// Lambda = 0 should be defaulted to 0.7
	cfg := MMRConfig{Enabled: true, Lambda: 0}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 2)

	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}

func TestApplyMMR_LambdaClamped(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "a.md", Text: "apple", Score: 1.0},
		{FileID: "b.md", Text: "banana", Score: 0.9},
	}

	// Lambda > 1 should be clamped to 1
	cfg := MMRConfig{Enabled: true, Lambda: 5.0}
	got := (&SQLiteStore{}).ApplyMMR(results, cfg, 2)

	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}
