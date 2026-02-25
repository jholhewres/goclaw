// Package copilot – abort.go implements abort trigger detection for stopping
// active agent runs using natural language phrases in multiple languages.
package copilot

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// abortTriggers contains phrases that trigger an abort when sent as standalone messages.
// Based on OpenClaw's abort.ts with multilingual support.
var abortTriggers = map[string]bool{
	// English
	"stop": true, "esc": true, "abort": true, "wait": true, "exit": true, "interrupt": true,
	"halt": true, "please stop": true, "stop please": true,
	"stop devclaw": true, "devclaw stop": true,
	"stop action": true, "stop current action": true,
	"stop run": true, "stop current run": true,
	"stop agent": true, "stop the agent": true,
	"stop don't do anything": true, "stop dont do anything": true,
	"stop do not do anything": true, "stop doing anything": true,
	"do not do that": true,

	// Portuguese
	"pare": true, "parar": true, "pare devclaw": true,
	"pare por favor": true, "por favor pare": true,
	"pare agora": true, "interromper": true,
	"cancela": true, "cancelar": true,

	// Spanish
	"detente": true, "deten": true, "detén": true,
	"detente devclaw": true, "para": true, "alto": true,

	// French
	"arrete": true, "arrête": true, "arreter": true, "arrêter": true, "arretez": true,

	// German
	"stopp": true, "anhalten": true, "aufhören": true, "hoer auf": true, "hör auf": true,

	// Chinese
	"停止": true, "停": true,

	// Japanese
	"やめて": true, "止めて": true, "ストップ": true,

	// Hindi
	"रुको": true, "रुकिए": true, "बंद": true,

	// Arabic
	"توقف": true, "قف": true,

	// Russian
	"стоп": true, "остановись": true, "останови": true,
	"остановить": true, "прекрати": true, "стой": true,
}

// trailingPunctuationRE matches trailing punctuation that should be stripped
// from abort trigger text (multilingual punctuation support).
var trailingPunctuationRE = regexp.MustCompile(`[.!?…,，。;；:：'"'"'")\]}]+$`)

// IsAbortTrigger checks if the given text is a standalone abort trigger.
// It normalizes the text by:
// - Converting to lowercase
// - Normalizing unicode (NFKC)
// - Stripping mentions and structural prefixes
// - Removing trailing punctuation
// - Collapsing whitespace
func IsAbortTrigger(text string) bool {
	normalized := normalizeAbortText(text)
	if normalized == "" {
		return false
	}

	// Check for /stop command
	if normalized == "/stop" {
		return true
	}

	// Check if it's a known abort trigger
	return abortTriggers[normalized]
}

// normalizeAbortText normalizes text for abort trigger matching.
func normalizeAbortText(text string) string {
	// Unicode NFKC normalization (handles full-width chars, etc.)
	normalized := norm.NFKC.String(text)

	// Convert to lowercase
	normalized = strings.ToLower(normalized)

	// Normalize apostrophes (curly -> straight)
	normalized = strings.Map(func(r rune) rune {
		if r == '\u2019' || r == '\u2018' || r == '`' { // ' ' curly quotes
			return '\''
		}
		return r
	}, normalized)

	// Strip structural prefixes (like @mentions)
	normalized = stripStructuralPrefixes(normalized)

	// Remove trailing punctuation
	normalized = trailingPunctuationRE.ReplaceAllString(normalized, "")

	// Collapse whitespace
	normalized = strings.Join(strings.Fields(normalized), " ")

	return strings.TrimSpace(normalized)
}

// stripStructuralPrefixes removes common structural prefixes from text.
func stripStructuralPrefixes(text string) string {
	// Remove @mentions at the start
	fields := strings.Fields(text)
	var filtered []string
	for _, f := range fields {
		if !strings.HasPrefix(f, "@") {
			filtered = append(filtered, f)
		}
	}
	return strings.Join(filtered, " ")
}

// IsAbortRequestText checks if text is an abort request, handling both
// /stop command and natural language triggers.
func IsAbortRequestText(text string) bool {
	if text == "" {
		return false
	}

	normalized := normalizeAbortText(text)
	if normalized == "" {
		return false
	}

	// Check for /stop command (with or without trailing punctuation)
	if normalized == "/stop" || strings.HasPrefix(normalized, "/stop") {
		return true
	}

	// Check abort triggers
	return abortTriggers[normalized]
}

// FormatAbortReply formats the reply message after an abort.
func FormatAbortReply(stoppedSubagents int) string {
	if stoppedSubagents <= 0 {
		return "Agent stopped."
	}
	if stoppedSubagents == 1 {
		return "Agent stopped. 1 sub-agent also stopped."
	}
	return "Agent stopped. Stopped sub-agents: stopped."
}

// HasAbortPrefix checks if text starts with an abort-related prefix.
// Useful for early detection in message processing.
func HasAbortPrefix(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))

	// Common abort prefixes
	prefixes := []string{
		"/stop", "stop", "pare", "detente", "detén",
		"arrête", "stopp", "停止", "やめて", "стоп",
		"توقف", "रुको", "abort", "halt", "exit",
	}

	for _, p := range prefixes {
		if strings.HasPrefix(text, p) {
			// Make sure it's standalone or followed by space/punctuation
			rest := text[len(p):]
			if rest == "" || unicode.IsSpace(rune(rest[0])) || unicode.IsPunct(rune(rest[0])) {
				return true
			}
		}
	}
	return false
}
