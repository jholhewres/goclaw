// Package copilot – markdown.go converts standard Markdown to channel-specific
// formats. WhatsApp supports a limited subset; other channels may get plain
// text or passthrough.
package copilot

import (
	"fmt"
	"regexp"
	"strings"
)

// FormatForWhatsApp converts standard Markdown to WhatsApp-compatible formatting.
// WhatsApp supports: *bold*, _italic_, ~strikethrough~, `monospace`, ```code blocks```.
// Headers become bold; links are flattened; images become [Image: alt]; lists use •.
func FormatForWhatsApp(text string) string {
	type codeBlock struct {
		placeholder string
		content     string
	}
	var blocks []codeBlock
	blockIdx := 0
	nextPH := func() string {
		ph := fmt.Sprintf("<<<GOCLAW_BLOCK_%d>>>", blockIdx)
		blockIdx++
		return ph
	}

	// Replace code blocks with placeholders first
	codeBlockRe := regexp.MustCompile("(?s)```[a-zA-Z0-9]*\\n?(.*?)```")
	text = codeBlockRe.ReplaceAllStringFunc(text, func(m string) string {
		inner := m
		inner = strings.TrimPrefix(inner, "```")
		inner = strings.TrimSuffix(inner, "```")
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\n' {
				inner = inner[i+1:]
				break
			}
			if (inner[i] >= 'a' && inner[i] <= 'z') || (inner[i] >= 'A' && inner[i] <= 'Z') || (inner[i] >= '0' && inner[i] <= '9') {
				continue
			}
			break
		}
		ph := nextPH()
		blocks = append(blocks, codeBlock{ph, "```\n" + strings.TrimSpace(inner) + "\n```"})
		return ph
	})

	// Protect inline code `...`
	inlineCodeRe := regexp.MustCompile("`[^`]+`")
	text = inlineCodeRe.ReplaceAllStringFunc(text, func(m string) string {
		ph := nextPH()
		blocks = append(blocks, codeBlock{ph, m})
		return ph
	})

	// Links: [text](url) → text (url)
	linkRe := regexp.MustCompile("\\[([^]]*)\\]\\(([^)]*)\\)")
	text = linkRe.ReplaceAllString(text, "$1 ($2)")

	// Images: ![alt](url) → [Image: alt]
	imgRe := regexp.MustCompile("!\\[([^]]*)\\]\\([^)]*\\)")
	text = imgRe.ReplaceAllString(text, "[Image: $1]")

	// Headers: # ## ### Heading → *Heading*
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			j := 0
			for j < len(trimmed) && trimmed[j] == '#' {
				j++
			}
			for j < len(trimmed) && trimmed[j] == ' ' {
				j++
			}
			heading := trimmed[j:]
			idx := strings.Index(line, trimmed)
			prefix := ""
			if idx > 0 {
				prefix = strings.TrimLeft(line[:idx], " \t")
			}
			lines[i] = prefix + "*" + heading + "*"
		}
	}
	text = strings.Join(lines, "\n")

	// Bold: **text** and __text__ → *text*
	text = regexp.MustCompile("\\*\\*([^*]+)\\*\\*").ReplaceAllString(text, "*$1*")
	text = regexp.MustCompile("__([^_]+)__").ReplaceAllString(text, "*$1*")

	// Unordered list: - item or * item → • item (before italic, so * item isn't treated as italic)
	text = regexp.MustCompile("(?m)^[-]\\s+").ReplaceAllString(text, "• ")
	text = regexp.MustCompile("(?m)^\\*\\s+").ReplaceAllString(text, "• ")

	// Italic: *text* (single asterisk) → _text_
	// Only convert to underscore italic if the text doesn't contain hyphens,
	// underscores, or other chars that break WhatsApp's italic rendering.
	italicRe := regexp.MustCompile("\\*([^*\n]+)\\*")
	text = italicRe.ReplaceAllStringFunc(text, func(m string) string {
		inner := m[1 : len(m)-1]
		// Skip conversion if text contains chars that break WhatsApp underscore italic.
		if strings.ContainsAny(inner, "-_/\\@#.") {
			return m // Leave as *text* — WhatsApp renders it as bold, which is acceptable.
		}
		return "_" + inner + "_"
	})

	// Strikethrough: ~~text~~ → ~text~
	text = regexp.MustCompile("~~([^~]+)~~").ReplaceAllString(text, "~$1~")

	// Horizontal rules: --- or *** → ───────
	text = regexp.MustCompile("(?m)^[-]{3,}\\s*$").ReplaceAllString(text, "───────")
	text = regexp.MustCompile("(?m)^[\\*]{3,}\\s*$").ReplaceAllString(text, "───────")

	// Tables: simplify separator lines
	text = formatMarkdownTables(text)

	// Restore protected blocks
	for _, b := range blocks {
		text = strings.ReplaceAll(text, b.placeholder, b.content)
	}

	return strings.TrimSpace(text)
}

func formatMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "|") {
			// Table separator (|---|---|)
			sepOnly := true
			for _, c := range trimmed {
				if c != '|' && c != '-' && c != ':' && c != ' ' {
					sepOnly = false
					break
				}
			}
			if sepOnly && strings.Count(trimmed, "-") > 1 {
				result = append(result, "─────────────────")
				continue
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// FormatForPlainText strips all Markdown, leaving only plain text.
func FormatForPlainText(text string) string {
	codeBlockRe := regexp.MustCompile("(?s)```[a-zA-Z0-9]*\\n?(.*?)```")
	text = codeBlockRe.ReplaceAllString(text, "$1")

	text = regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("\\[([^]]*)\\]\\([^)]*\\)").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("!\\[([^]]*)\\]\\([^)]*\\)").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("(?m)^#+\\s+").ReplaceAllString(text, "")
	text = regexp.MustCompile("\\*\\*([^*]+)\\*\\*").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("__([^_]+)__").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("\\*([^*]+)\\*").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("_([^_]+)_").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("~~([^~]+)~~").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("(?m)^[-*]{3,}\\s*$").ReplaceAllString(text, "")
	text = regexp.MustCompile("(?m)^>\\s*").ReplaceAllString(text, "")
	text = regexp.MustCompile("\\|").ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// FormatForChannel dispatches to the appropriate formatter based on channel.
func FormatForChannel(text, channel string) string {
	ch := strings.ToLower(strings.TrimSpace(channel))
	switch ch {
	case "whatsapp":
		return FormatForWhatsApp(text)
	case "telegram":
		return FormatForTelegram(text)
	case "discord":
		return text // Discord uses standard Markdown natively.
	case "slack":
		return FormatForSlack(text)
	case "plain", "sms":
		return FormatForPlainText(text)
	default:
		return text
	}
}

// FormatForTelegram converts Markdown to Telegram HTML.
// Telegram supports: <b>, <i>, <code>, <pre>, <a href="">, <s>, <u>.
func FormatForTelegram(text string) string {
	// Escape HTML entities first.
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// Bold: **text** -> <b>text</b>
	for {
		start := strings.Index(text, "**")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		inner := text[start+2 : end]
		text = text[:start] + "<b>" + inner + "</b>" + text[end+2:]
	}

	// Italic: *text* -> <i>text</i> (only single asterisks, not inside bold)
	for {
		start := strings.Index(text, "*")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+1:], "*")
		if end == -1 {
			break
		}
		end += start + 1
		inner := text[start+1 : end]
		text = text[:start] + "<i>" + inner + "</i>" + text[end+1:]
	}

	// Inline code: `text` -> <code>text</code>
	for {
		start := strings.Index(text, "`")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1
		inner := text[start+1 : end]
		text = text[:start] + "<code>" + inner + "</code>" + text[end+1:]
	}

	// Code blocks: ```lang\ncode\n``` -> <pre>code</pre>
	for {
		start := strings.Index(text, "```")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+3:], "```")
		if end == -1 {
			break
		}
		end += start + 3
		inner := text[start+3 : end]
		// Strip the optional language identifier on the first line.
		if nl := strings.Index(inner, "\n"); nl != -1 {
			inner = inner[nl+1:]
		}
		text = text[:start] + "<pre>" + strings.TrimSpace(inner) + "</pre>" + text[end+3:]
	}

	// Strikethrough: ~~text~~ -> <s>text</s>
	for {
		start := strings.Index(text, "~~")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+2:], "~~")
		if end == -1 {
			break
		}
		end += start + 2
		inner := text[start+2 : end]
		text = text[:start] + "<s>" + inner + "</s>" + text[end+2:]
	}

	return text
}

// FormatForSlack converts Markdown to Slack's mrkdwn format.
// Slack uses: *bold*, _italic_, ~strike~, `code`, ```preformatted```.
func FormatForSlack(text string) string {
	// Bold: **text** → *text*
	for {
		start := strings.Index(text, "**")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		inner := text[start+2 : end]
		text = text[:start] + "*" + inner + "*" + text[end+2:]
	}

	// Strikethrough: ~~text~~ → ~text~
	for {
		start := strings.Index(text, "~~")
		if start == -1 {
			break
		}
		end := strings.Index(text[start+2:], "~~")
		if end == -1 {
			break
		}
		end += start + 2
		inner := text[start+2 : end]
		text = text[:start] + "~" + inner + "~" + text[end+2:]
	}

	// Links: [text](url) → <url|text>
	for {
		start := strings.Index(text, "[")
		if start == -1 {
			break
		}
		mid := strings.Index(text[start:], "](")
		if mid == -1 {
			break
		}
		mid += start
		end := strings.Index(text[mid:], ")")
		if end == -1 {
			break
		}
		end += mid
		label := text[start+1 : mid]
		url := text[mid+2 : end]
		text = text[:start] + "<" + url + "|" + label + ">" + text[end+1:]
	}

	return text
}
