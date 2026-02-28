// Package memory – query_expansion.go handles keyword extraction and FTS5 query building.
// Supports stop words in English, Portuguese, Spanish, and French.
// Filters out pure numbers, all-punctuation tokens, and very short tokens.
package memory

import (
	"strings"
	"unicode"
)

// extractKeywords extracts meaningful keywords from a conversational query
// by removing stop words, short tokens, pure numbers, and punctuation-only tokens.
func extractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}*`~@#$%&_-+=<>/\\|")
		if !isValidKeyword(w) {
			continue
		}
		keywords = append(keywords, w)
	}
	return keywords
}

// isValidKeyword checks whether a token is a meaningful keyword.
// Rejects stop words, pure numbers, all-punctuation, and tokens < 2 chars.
func isValidKeyword(w string) bool {
	if len(w) < 2 {
		return false
	}
	if stopWords[w] {
		return false
	}
	// Reject pure numbers (e.g. "123", "42").
	allDigits := true
	for _, r := range w {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	if allDigits {
		return false
	}
	// Reject all-punctuation tokens.
	allPunct := true
	for _, r := range w {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			allPunct = false
			break
		}
	}
	if allPunct {
		return false
	}
	return true
}

// expandQueryForFTS converts extracted keywords into an FTS5 query.
// Uses prefix matching (keyword*) for partial matches and OR to combine.
func expandQueryForFTS(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	var parts []string
	for _, kw := range keywords {
		s := sanitizeFTS5Query(kw)
		if s != "" {
			parts = append(parts, s)
		}
		// Add prefix match for keywords >= 3 chars.
		if len(kw) >= 3 {
			clean := sanitizeFTS5Keyword(kw)
			if clean != "" {
				parts = append(parts, clean+"*")
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// Deduplicate parts.
	seen := make(map[string]bool, len(parts))
	var unique []string
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	return strings.Join(unique, " OR ")
}

// sanitizeFTS5Keyword strips FTS5 operators from a single keyword
// without wrapping in quotes (for prefix matching).
func sanitizeFTS5Keyword(kw string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '"', '(', ')', '*', '^', ':', '{', '}':
			return -1
		default:
			return r
		}
	}, kw)
	return strings.TrimSpace(cleaned)
}

// mergeSearchResults deduplicates and merges two result sets.
func mergeSearchResults(a, b []SearchResult, maxResults int) []SearchResult {
	seen := make(map[string]bool)
	var merged []SearchResult
	for _, r := range a {
		key := r.FileID + "|" + r.Text
		if !seen[key] {
			seen[key] = true
			merged = append(merged, r)
		}
	}
	for _, r := range b {
		key := r.FileID + "|" + r.Text
		if !seen[key] {
			seen[key] = true
			merged = append(merged, r)
		}
	}
	if len(merged) > maxResults {
		merged = merged[:maxResults]
	}
	return merged
}

// stopWords are common words filtered out during keyword extraction.
// Covers English, Portuguese, Spanish, and French.
var stopWords = map[string]bool{
	// Common 2-letter stop words (multi-language)
	"to": true, "of": true, "in": true, "is": true, "it": true,
	"an": true, "as": true, "at": true, "be": true, "by": true,
	"do": true, "go": true, "he": true, "if": true, "me": true,
	"my": true, "no": true, "on": true, "or": true, "so": true,
	"up": true, "we": true, "am": true,
	"de": true, "se": true, "eu": true, "em": true, "ou": true, // PT/ES/FR
	"la": true, "le": true, "un": true, "en": true, "ya": true, // ES/FR
	"du": true, "et": true, "il": true, "je": true, "ce": true, // FR
	"el": true, "lo": true, "mi": true, "si": true, "tu": true, // ES

	// English
	"the": true, "and": true, "for": true, "are": true, "but": true,
	"not": true, "you": true, "all": true, "can": true, "had": true,
	"her": true, "was": true, "one": true, "our": true, "out": true,
	"has": true, "its": true, "let": true, "may": true, "who": true,
	"did": true, "get": true, "got": true, "him": true, "his": true,
	"how": true, "man": true, "new": true, "now": true, "old": true,
	"see": true, "way": true, "day": true, "too": true, "use": true,
	"that": true, "with": true, "have": true, "this": true, "will": true,
	"your": true, "from": true, "they": true, "been": true, "said": true,
	"each": true, "which": true, "their": true, "what": true, "about": true,
	"would": true, "there": true, "when": true, "make": true, "like": true,
	"time": true, "just": true, "know": true, "take": true, "come": true,
	"could": true, "than": true, "look": true, "only": true, "into": true,
	"over": true, "such": true, "also": true, "back": true, "some": true,
	"them": true, "then": true, "these": true, "thing": true, "where": true,
	"much": true, "should": true, "well": true, "after": true,
	"very": true, "does": true, "here": true, "were": true,
	"more": true, "most": true, "many": true, "other": true, "those": true,
	"still": true, "even": true, "both": true, "same": true, "every": true,

	// Portuguese
	"que": true, "não": true, "nao": true, "com": true, "uma": true, "para": true,
	"por": true, "mais": true, "como": true, "mas": true, "dos": true,
	"das": true, "nos": true, "nas": true, "foi": true, "ser": true,
	"tem": true, "são": true, "sao": true, "seu": true, "sua": true, "isso": true,
	"este": true, "esta": true, "esse": true, "essa": true, "aqui": true,
	"ele": true, "ela": true, "eles": true, "elas": true, "nós": true,
	"vocé": true, "voce": true, "você": true, "também": true, "tambem": true,
	"onde": true, "quando": true, "quem": true, "qual": true, "quais": true,
	"tudo": true, "todos": true, "toda": true, "todas": true,
	"muito": true, "muita": true, "muitos": true, "muitas": true,
	"outro": true, "outra": true, "outros": true, "outras": true,
	"sobre": true, "entre": true, "depois": true, "ainda": true,
	"desde": true, "até": true, "ate": true, "seus": true, "suas": true,
	"meu": true, "minha": true, "meus": true, "minhas": true,

	// Spanish (excluding words already in PT/EN above)
	"los": true, "las": true, "del": true, "uno": true,
	"con": true, "más": true, "pero": true,
	"sin": true, "sus": true, "les": true, "fue": true, "son": true,
	"han": true, "hay": true, "está": true,
	"todo": true, "ese": true, "eso": true, "así": true, "asi": true,
	"cada": true, "bien": true, "puede": true, "tiene": true,
	"donde": true, "cuando": true, "quien": true, "cual": true,
	"porque": true, "aunque": true, "después": true, "despues": true,
	"antes": true, "hasta": true, "aquí": true,
	"algo": true, "mismo": true, "misma": true,

	// French (excluding words already above)
	"des": true, "une": true, "dans": true, "pour": true,
	"avec": true, "sur": true, "pas": true, "qui": true, "est": true,
	"par": true, "plus": true, "sont": true, "ont": true,
	"aux": true, "été": true, "ete": true, "ces": true, "ses": true,
	"fait": true, "tout": true, "même": true, "meme": true,
	"être": true, "etre": true, "avoir": true, "comme": true,
	"aussi": true, "après": true, "apres": true, "encore": true,
	"donc": true, "quand": true, "chez": true,
	"leur": true, "leurs": true, "autre": true, "autres": true,
}
