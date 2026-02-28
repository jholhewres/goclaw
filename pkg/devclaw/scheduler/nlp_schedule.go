// Package scheduler – nlp_schedule.go parses natural language schedule expressions
// into cron/every/at formats. Falls through to the raw value if no pattern matches.
package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedSchedule holds the result of parsing a natural language schedule expression.
type ParsedSchedule struct {
	Schedule string // Parsed schedule string (cron expression, duration, or time).
	Type     string // Job type: "cron", "every", or "at".
}

// ParseNaturalLanguage attempts to interpret a natural language schedule expression.
// Returns the parsed schedule and job type. If no pattern matches, returns the input
// as-is with an empty type (caller should use the original type/schedule).
//
// Supported patterns:
//   - "every N minutes/hours/days" → @every Nm/Nh
//   - "every minute/hour/day" → @every 1m/1h/24h
//   - "daily at HH:MM" → 0 MM HH * * *
//   - "weekly on Monday [at HH:MM]" → 0 MM HH * * DOW
//   - "hourly" → @every 1h
//   - "in N minutes/hours" → type "at" with duration
func ParseNaturalLanguage(input string) (ParsedSchedule, bool) {
	normalized := strings.TrimSpace(strings.ToLower(input))
	if normalized == "" {
		return ParsedSchedule{}, false
	}

	// "every N minutes/hours/days/seconds"
	if m := reEveryInterval.FindStringSubmatch(normalized); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := normalizeTimeUnit(m[2])
		if n > 0 && unit != "" {
			// Go's time.ParseDuration does not support "d"; convert days to hours.
			if unit == "d" {
				n *= 24
				unit = "h"
			}
			return ParsedSchedule{
				Schedule: fmt.Sprintf("@every %d%s", n, unit),
				Type:     "every",
			}, true
		}
	}

	// "every second", "every minute", "every hour", "every day"
	if m := reEverySingular.FindStringSubmatch(normalized); m != nil {
		unit := normalizeTimeUnit(m[1])
		switch unit {
		case "s":
			return ParsedSchedule{Schedule: "@every 1s", Type: "every"}, true
		case "m":
			return ParsedSchedule{Schedule: "@every 1m", Type: "every"}, true
		case "h":
			return ParsedSchedule{Schedule: "@every 1h", Type: "every"}, true
		case "d":
			return ParsedSchedule{Schedule: "@every 24h", Type: "every"}, true
		}
	}

	// "daily at HH:MM" or "daily at H AM/PM"
	if m := reDailyAt.FindStringSubmatch(normalized); m != nil {
		hour, minute := parseTimeComponents(m[1])
		if hour >= 0 {
			return ParsedSchedule{
				Schedule: fmt.Sprintf("%d %d * * *", minute, hour),
				Type:     "cron",
			}, true
		}
	}

	// "daily" (no time specified → midnight)
	if normalized == "daily" {
		return ParsedSchedule{Schedule: "0 0 * * *", Type: "cron"}, true
	}

	// "hourly"
	if normalized == "hourly" {
		return ParsedSchedule{Schedule: "@every 1h", Type: "every"}, true
	}

	// "weekly on Monday [at HH:MM]"
	if m := reWeeklyOn.FindStringSubmatch(normalized); m != nil {
		dow := parseDayOfWeek(m[1])
		if dow >= 0 {
			hour, minute := 0, 0
			if m[2] != "" {
				hour, minute = parseTimeComponents(m[2])
				if hour < 0 {
					hour, minute = 0, 0
				}
			}
			return ParsedSchedule{
				Schedule: fmt.Sprintf("%d %d * * %d", minute, hour, dow),
				Type:     "cron",
			}, true
		}
	}

	// "in N minutes/hours/seconds"
	if m := reInDuration.FindStringSubmatch(normalized); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := normalizeTimeUnit(m[2])
		if n > 0 && unit != "" {
			return ParsedSchedule{
				Schedule: fmt.Sprintf("%d%s", n, unit),
				Type:     "at",
			}, true
		}
	}

	// No pattern matched.
	return ParsedSchedule{}, false
}

// ---------- Regex patterns ----------

var (
	reEveryInterval = regexp.MustCompile(`^every\s+(\d+)\s+(second|minute|hour|day|sec|min)s?$`)
	reEverySingular = regexp.MustCompile(`^every\s+(second|minute|hour|day)$`)
	reDailyAt       = regexp.MustCompile(`^daily\s+at\s+(.+)$`)
	reWeeklyOn      = regexp.MustCompile(`^weekly\s+on\s+(\w+)(?:\s+at\s+(.+))?$`)
	reInDuration    = regexp.MustCompile(`^in\s+(\d+)\s+(second|minute|hour|sec|min)s?$`)
)

// ---------- Helpers ----------

// normalizeTimeUnit converts a time unit word to its Go duration suffix.
func normalizeTimeUnit(unit string) string {
	unit = strings.TrimSuffix(strings.ToLower(unit), "s")
	switch unit {
	case "second", "sec":
		return "s"
	case "minute", "min":
		return "m"
	case "hour":
		return "h"
	case "day":
		return "d"
	default:
		return ""
	}
}

// parseTimeComponents parses a time string like "9:00", "14:30", "9am", "3:30pm".
// Returns hour (0-23) and minute, or (-1, 0) on failure.
func parseTimeComponents(s string) (int, int) {
	s = strings.TrimSpace(strings.ToLower(s))

	isPM := strings.HasSuffix(s, "pm")
	isAM := strings.HasSuffix(s, "am")
	if isPM {
		s = strings.TrimSuffix(s, "pm")
	} else if isAM {
		s = strings.TrimSuffix(s, "am")
	}
	s = strings.TrimSpace(s)

	parts := strings.SplitN(s, ":", 2)
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hour < 0 || hour > 23 {
		return -1, 0
	}

	minute := 0
	if len(parts) == 2 {
		minute, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || minute < 0 || minute > 59 {
			return -1, 0
		}
	}

	if isPM && hour < 12 {
		hour += 12
	}
	if isAM && hour == 12 {
		hour = 0
	}

	return hour, minute
}

// parseDayOfWeek converts a day name to cron day-of-week number (0=Sunday).
func parseDayOfWeek(day string) int {
	switch strings.ToLower(day) {
	case "sunday", "sun":
		return 0
	case "monday", "mon":
		return 1
	case "tuesday", "tue":
		return 2
	case "wednesday", "wed":
		return 3
	case "thursday", "thu":
		return 4
	case "friday", "fri":
		return 5
	case "saturday", "sat":
		return 6
	default:
		return -1
	}
}
