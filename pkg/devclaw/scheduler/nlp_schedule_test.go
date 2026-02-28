package scheduler

import (
	"testing"
)

func TestParseNaturalLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		schedule string
		jobType  string
		matched  bool
	}{
		// "every N unit" patterns
		{"every 5 minutes", "@every 5m", "every", true},
		{"every 30 seconds", "@every 30s", "every", true},
		{"every 2 hours", "@every 2h", "every", true},
		{"every 1 minute", "@every 1m", "every", true},
		{"every 10 mins", "@every 10m", "every", true},
		{"every 60 secs", "@every 60s", "every", true},

		// "every unit" (singular)
		{"every minute", "@every 1m", "every", true},
		{"every hour", "@every 1h", "every", true},
		{"every day", "@every 24h", "every", true},
		{"every second", "@every 1s", "every", true},

		// "daily" patterns
		{"daily", "0 0 * * *", "cron", true},
		{"daily at 9:00", "0 9 * * *", "cron", true},
		{"daily at 14:30", "30 14 * * *", "cron", true},
		{"daily at 9am", "0 9 * * *", "cron", true},
		{"daily at 3pm", "0 15 * * *", "cron", true},
		{"daily at 3:30pm", "30 15 * * *", "cron", true},
		{"daily at 12am", "0 0 * * *", "cron", true},
		{"daily at 12pm", "0 12 * * *", "cron", true},

		// "hourly"
		{"hourly", "@every 1h", "every", true},

		// "weekly on DOW" patterns
		{"weekly on monday", "0 0 * * 1", "cron", true},
		{"weekly on friday", "0 0 * * 5", "cron", true},
		{"weekly on sunday at 9:00", "0 9 * * 0", "cron", true},
		{"weekly on wed at 14:30", "30 14 * * 3", "cron", true},
		{"weekly on saturday at 8am", "0 8 * * 6", "cron", true},

		// "in N unit" patterns
		{"in 5 minutes", "5m", "at", true},
		{"in 30 seconds", "30s", "at", true},
		{"in 2 hours", "2h", "at", true},

		// Case insensitivity
		{"Every 5 Minutes", "@every 5m", "every", true},
		{"DAILY AT 9:00", "0 9 * * *", "cron", true},
		{"In 30 Seconds", "30s", "at", true},

		// Edge cases: zero/invalid intervals
		{"every 0 minutes", "", "", false},
		{"in 0 seconds", "", "", false},

		// Edge cases: invalid time components
		{"daily at 25:00", "", "", false},
		{"daily at 9:99", "", "", false},
		{"daily at -1:00", "", "", false},

		// Edge cases: invalid day of week
		{"weekly on invalidday", "", "", false},
		{"weekly on xyz at 9:00", "", "", false},

		// No match (passthrough)
		{"0 9 * * *", "", "", false},
		{"@every 5m", "", "", false},
		{"5m", "", "", false},
		{"", "", "", false},
		{"something random", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result, matched := ParseNaturalLanguage(tt.input)
			if matched != tt.matched {
				t.Errorf("ParseNaturalLanguage(%q) matched = %v, want %v", tt.input, matched, tt.matched)
				return
			}
			if !matched {
				return
			}
			if result.Schedule != tt.schedule {
				t.Errorf("ParseNaturalLanguage(%q).Schedule = %q, want %q", tt.input, result.Schedule, tt.schedule)
			}
			if result.Type != tt.jobType {
				t.Errorf("ParseNaturalLanguage(%q).Type = %q, want %q", tt.input, result.Type, tt.jobType)
			}
		})
	}
}

func TestParseTimeComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input  string
		hour   int
		minute int
	}{
		{"9:00", 9, 0},
		{"14:30", 14, 30},
		{"9am", 9, 0},
		{"3pm", 15, 0},
		{"3:30pm", 15, 30},
		{"12am", 0, 0},
		{"12pm", 12, 0},
		{"0:00", 0, 0},
		{"23:59", 23, 59},
		// Edge cases
		{"12:30am", 0, 30},
		{"12:30pm", 12, 30},
		{"", -1, 0},
		{"abc", -1, 0},
		{"24:00", -1, 0},
		{"9:60", -1, 0},
		{"-1:00", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			hour, minute := parseTimeComponents(tt.input)
			if hour != tt.hour || minute != tt.minute {
				t.Errorf("parseTimeComponents(%q) = (%d, %d), want (%d, %d)",
					tt.input, hour, minute, tt.hour, tt.minute)
			}
		})
	}
}

func TestParseDayOfWeek(t *testing.T) {
	t.Parallel()

	tests := []struct {
		day string
		dow int
	}{
		{"sunday", 0}, {"sun", 0},
		{"monday", 1}, {"mon", 1},
		{"tuesday", 2}, {"tue", 2},
		{"wednesday", 3}, {"wed", 3},
		{"thursday", 4}, {"thu", 4},
		{"friday", 5}, {"fri", 5},
		{"saturday", 6}, {"sat", 6},
		{"invalid", -1},
	}

	for _, tt := range tests {
		t.Run(tt.day, func(t *testing.T) {
			t.Parallel()
			got := parseDayOfWeek(tt.day)
			if got != tt.dow {
				t.Errorf("parseDayOfWeek(%q) = %d, want %d", tt.day, got, tt.dow)
			}
		})
	}
}

func TestNormalizeTimeUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		unit string
		want string
	}{
		{"second", "s"}, {"seconds", "s"}, {"sec", "s"}, {"secs", "s"},
		{"minute", "m"}, {"minutes", "m"}, {"min", "m"}, {"mins", "m"},
		{"hour", "h"}, {"hours", "h"},
		{"day", "d"}, {"days", "d"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			t.Parallel()
			got := normalizeTimeUnit(tt.unit)
			if got != tt.want {
				t.Errorf("normalizeTimeUnit(%q) = %q, want %q", tt.unit, got, tt.want)
			}
		})
	}
}
