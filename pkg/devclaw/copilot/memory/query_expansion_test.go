package memory

import (
	"testing"
)

func TestExtractKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "simple english",
			query:    "how to configure the database",
			expected: []string{"configure", "database"},
		},
		{
			name:     "portuguese conversational",
			query:    "como eu faço para configurar o banco de dados",
			expected: []string{"faço", "configurar", "banco", "dados"},
		},
		{
			name:     "spanish conversational",
			query:    "cómo puedo configurar la base de datos",
			expected: []string{"cómo", "puedo", "configurar", "base", "datos"},
		},
		{
			name:     "mixed with punctuation",
			query:    "what's the error? (timeout, maybe)",
			expected: []string{"what's", "error", "timeout", "maybe"},
		},
		{
			name:     "empty query",
			query:    "",
			expected: nil,
		},
		{
			name:     "only stop words",
			query:    "the and for with this that",
			expected: nil,
		},
		{
			name:     "pure numbers filtered",
			query:    "error 404 at module 123",
			expected: []string{"error", "module"},
		},
		{
			name:     "short tokens filtered",
			query:    "a b c database",
			expected: []string{"database"},
		},
		{
			name:     "single char tokens",
			query:    "want find x",
			expected: []string{"want", "find"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractKeywords(tt.query)
			if len(got) != len(tt.expected) {
				t.Errorf("extractKeywords(%q) = %v (len %d), want %v (len %d)",
					tt.query, got, len(got), tt.expected, len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("extractKeywords(%q)[%d] = %q, want %q",
						tt.query, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestIsValidKeyword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		word  string
		valid bool
	}{
		{"database", true},
		{"config", true},
		{"a", false},        // too short
		{"", false},         // empty
		{"the", false},      // english stop word
		{"para", false},     // portuguese stop word
		{"123", false},      // pure number
		{"42", false},       // pure number
		{"...", false},      // all punctuation
		{"---", false},      // all punctuation
		{"db2", true},       // mixed alphanumeric
		{"v3", true},        // mixed alphanumeric
		{"não", false},      // portuguese stop word
		{"avec", false},     // french stop word
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			t.Parallel()
			got := isValidKeyword(tt.word)
			if got != tt.valid {
				t.Errorf("isValidKeyword(%q) = %v, want %v", tt.word, got, tt.valid)
			}
		})
	}
}

func TestExpandQueryForFTS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keywords []string
		wantLen  int // number of OR-separated parts (approx)
	}{
		{
			name:     "empty",
			keywords: nil,
			wantLen:  0,
		},
		{
			name:     "single keyword",
			keywords: []string{"database"},
			wantLen:  2, // "database" OR database*
		},
		{
			name:     "multiple keywords",
			keywords: []string{"user", "auth"},
			wantLen:  4, // "user" OR user* OR "auth" OR auth*
		},
		{
			name:     "short keyword no prefix",
			keywords: []string{"db"},
			wantLen:  1, // only "db", no prefix (len < 3)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expandQueryForFTS(tt.keywords)
			if tt.wantLen == 0 {
				if got != "" {
					t.Errorf("expandQueryForFTS(%v) = %q, want empty", tt.keywords, got)
				}
				return
			}
			if got == "" {
				t.Errorf("expandQueryForFTS(%v) = empty, want non-empty", tt.keywords)
				return
			}
			// Verify it contains OR-separated parts.
			parts := len(splitOR(got))
			if parts != tt.wantLen {
				t.Errorf("expandQueryForFTS(%v) has %d parts, want %d: %q",
					tt.keywords, parts, tt.wantLen, got)
			}
		})
	}
}

func TestExpandQueryForFTS_ContainsPrefixMatch(t *testing.T) {
	t.Parallel()

	got := expandQueryForFTS([]string{"config"})
	if got == "" {
		t.Fatal("expected non-empty result")
	}
	// Should contain both phrase match and prefix match.
	if !containsSubstring(got, "config*") {
		t.Errorf("expandQueryForFTS should contain prefix match, got: %q", got)
	}
}

func TestMergeSearchResults(t *testing.T) {
	t.Parallel()

	a := []SearchResult{
		{FileID: "f1", Text: "hello", Score: 0.9},
		{FileID: "f2", Text: "world", Score: 0.8},
	}
	b := []SearchResult{
		{FileID: "f2", Text: "world", Score: 0.7}, // duplicate
		{FileID: "f3", Text: "test", Score: 0.6},
	}

	merged := mergeSearchResults(a, b, 10)
	if len(merged) != 3 {
		t.Errorf("mergeSearchResults returned %d results, want 3", len(merged))
	}

	// Verify no duplicates.
	seen := make(map[string]bool)
	for _, r := range merged {
		key := r.FileID + "|" + r.Text
		if seen[key] {
			t.Errorf("duplicate result: %s", key)
		}
		seen[key] = true
	}
}

func TestMergeSearchResults_Truncates(t *testing.T) {
	t.Parallel()

	a := []SearchResult{
		{FileID: "f1", Text: "a", Score: 0.9},
		{FileID: "f2", Text: "b", Score: 0.8},
		{FileID: "f3", Text: "c", Score: 0.7},
	}
	b := []SearchResult{
		{FileID: "f4", Text: "d", Score: 0.6},
	}

	merged := mergeSearchResults(a, b, 2)
	if len(merged) != 2 {
		t.Errorf("mergeSearchResults returned %d results, want 2", len(merged))
	}
}

func TestSanitizeFTS5Keyword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello*world", "helloworld"},
		{`"quoted"`, "quoted"},
		{"(parens)", "parens"},
		{"clean", "clean"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := sanitizeFTS5Keyword(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFTS5Keyword(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Helper: split string by " OR ".
func splitOR(s string) []string {
	parts := []string{}
	for _, p := range split(s, " OR ") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func split(s, sep string) []string {
	var result []string
	for {
		i := indexOf(s, sep)
		if i < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:i])
		s = s[i+len(sep):]
	}
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsSubstring(s, sub string) bool {
	return indexOf(s, sub) >= 0
}
