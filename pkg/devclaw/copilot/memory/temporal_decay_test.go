package memory

import (
	"testing"
	"time"
)

func TestExtractDateFromFileID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileID   string
		wantNil  bool
		expected string // YYYY-MM-DD format if not nil
	}{
		{
			name:     "dated file in memory dir",
			fileID:   "memory/2026-02-25.md",
			wantNil:  false,
			expected: "2026-02-25",
		},
		{
			name:     "dated file without dir",
			fileID:   "2026-01-15.md",
			wantNil:  false,
			expected: "2026-01-15",
		},
		{
			name:    "MEMORY.md evergreen",
			fileID:  "memory/MEMORY.md",
			wantNil: true,
		},
		{
			name:    "MEMORY.md in root",
			fileID:  "MEMORY.md",
			wantNil: true,
		},
		{
			name:    "non-dated file",
			fileID:  "notes/project.md",
			wantNil: true,
		},
		{
			name:    "invalid date format",
			fileID:  "memory/25-02-2026.md",
			wantNil: true,
		},
		{
			name:    "empty string",
			fileID:  "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDateFromFileID(tt.fileID)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected non-nil date, got nil")
					return
				}
				got := result.Format("2006-01-02")
				if got != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, got)
				}
			}
		})
	}
}

func TestApplyTemporalDecay_Disabled(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "memory/2026-01-01.md", Text: "old note", Score: 1.0},
		{FileID: "memory/2026-02-25.md", Text: "new note", Score: 1.0},
	}

	cfg := TemporalDecayConfig{Enabled: false, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	// Scores should be unchanged when disabled
	for i := range got {
		if got[i].Score != 1.0 {
			t.Errorf("expected score 1.0, got %f", got[i].Score)
		}
	}
}

func TestApplyTemporalDecay_EvergreenNoDecay(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{FileID: "memory/MEMORY.md", Text: "evergreen", Score: 1.0},
	}

	cfg := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	// MEMORY.md should not decay
	if got[0].Score != 1.0 {
		t.Errorf("expected evergreen score 1.0, got %f", got[0].Score)
	}
}

func TestApplyTemporalDecay_OldFileDecays(t *testing.T) {
	t.Parallel()

	// Create a result from 60 days ago (2 half-lives = 0.25 score)
	now := time.Now()
	oldDate := now.AddDate(0, 0, -60)
	oldFileID := "memory/" + oldDate.Format("2006-01-02") + ".md"

	results := []SearchResult{
		{FileID: oldFileID, Text: "old note", Score: 1.0},
	}

	cfg := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	// After 60 days (2 half-lives), score should be ~0.25
	// Allow some tolerance for date calculation
	if got[0].Score < 0.2 || got[0].Score > 0.3 {
		t.Errorf("expected score ~0.25 after 2 half-lives, got %f", got[0].Score)
	}
}

func TestApplyTemporalDecay_RecentFileMinimalDecay(t *testing.T) {
	t.Parallel()

	// Create a result from 1 day ago
	now := time.Now()
	recentDate := now.AddDate(0, 0, -1)
	recentFileID := "memory/" + recentDate.Format("2006-01-02") + ".md"

	results := []SearchResult{
		{FileID: recentFileID, Text: "recent note", Score: 1.0},
	}

	cfg := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	// After 1 day, decay should be minimal (~0.977)
	if got[0].Score < 0.95 || got[0].Score > 1.0 {
		t.Errorf("expected score ~0.97 after 1 day, got %f", got[0].Score)
	}
}

func TestApplyTemporalDecay_MixedResults(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oldDate := now.AddDate(0, 0, -60)
	recentDate := now.AddDate(0, 0, -1)

	results := []SearchResult{
		{FileID: "memory/MEMORY.md", Text: "evergreen", Score: 1.0},
		{FileID: "memory/" + oldDate.Format("2006-01-02") + ".md", Text: "old", Score: 1.0},
		{FileID: "memory/" + recentDate.Format("2006-01-02") + ".md", Text: "recent", Score: 1.0},
	}

	cfg := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	// Find each result and verify relative ordering
	var evergreen, old, recent float64
	for _, r := range got {
		switch r.FileID {
		case "memory/MEMORY.md":
			evergreen = r.Score
		case "memory/" + oldDate.Format("2006-01-02") + ".md":
			old = r.Score
		case "memory/" + recentDate.Format("2006-01-02") + ".md":
			recent = r.Score
		}
	}

	if evergreen != 1.0 {
		t.Errorf("evergreen should be 1.0, got %f", evergreen)
	}
	if recent <= old {
		t.Errorf("recent (%f) should be > old (%f)", recent, old)
	}
	if old < 0.2 || old > 0.3 {
		t.Errorf("old should be ~0.25, got %f", old)
	}
}

func TestApplyTemporalDecay_EmptyResults(t *testing.T) {
	t.Parallel()

	results := []SearchResult{}
	cfg := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	got := (&SQLiteStore{}).ApplyTemporalDecay(results, cfg)

	if len(got) != 0 {
		t.Errorf("expected empty results, got %d", len(got))
	}
}
