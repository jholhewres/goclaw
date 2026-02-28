package copilot

import "testing"

func TestStripInternalTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"clean text", "hello world", "hello world"},
		{"reply tag", "[[reply_to_current]]hello", "hello"},
		{"reply tag with id", "[[reply_to:abc123]]hello", "hello"},
		{"final block", "<final>hello world</final>", ""},
		{"final with content outside", "before <final>inside</final> after", "before  after"},
		{"thinking tag", "<thinking>hmm</thinking>", "hmm"},
		{"reasoning tag", "<reasoning>ok</reasoning>", "ok"},
		{"standalone final tags", "hello <final> world", "hello  world"},
		{"NO_REPLY token", "NO_REPLY", ""},
		{"HEARTBEAT_OK token", "HEARTBEAT_OK", ""},
		{"NO_REPLY in text", "result is NO_REPLY done", "result is  done"},
		{"combined tags", "[[reply_to_current]]<final>text</final>NO_REPLY", ""},
		{"nested tags", "<final><thinking>deep</thinking></final>", ""},
		{"text with mixed tags", "Hello [[reply_to_current]] world <final>dup</final>", "Hello  world"},
		{"only whitespace after strip", "  [[reply_to_current]]  ", ""},
		{"tools used annotation", "[Tools used: cron_add, web_search]\nHello", "Hello"},
		{"tools used inline", "[Tools used: calculator_calculate, timestamp_convert]\nDone!", "Done!"},
		{"tool_provenance tag", "<tool_provenance>cron_list</tool_provenance>\nNo jobs.", "No jobs."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripInternalTags(tt.in)
			if got != tt.want {
				t.Errorf("StripInternalTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripReplyTags_Alias(t *testing.T) {
	t.Parallel()
	got := StripReplyTags("[[reply_to_current]]hello")
	if got != "hello" {
		t.Errorf("StripReplyTags alias failed: got %q", got)
	}
}

func TestFormatForChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		channel string
		in      string
		noEmpty bool // if true, result must not be empty
	}{
		{"whatsapp", "**bold**", true},
		{"telegram", "**bold**", true},
		{"discord", "**bold**", true},
		{"slack", "**bold**", true},
		{"plain", "**bold**", true},
		{"sms", "**bold**", true},
		{"unknown", "**bold**", true},
		{"WHATSAPP", "**bold**", true}, // case insensitive
		{" whatsapp ", "**bold**", true},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			t.Parallel()
			got := FormatForChannel(tt.in, tt.channel)
			if tt.noEmpty && got == "" {
				t.Errorf("FormatForChannel(%q, %q) returned empty", tt.in, tt.channel)
			}
		})
	}
}

func TestFormatForChannel_StripsTagsBeforeFormat(t *testing.T) {
	t.Parallel()
	got := FormatForChannel("[[reply_to_current]]hello", "whatsapp")
	if got != "hello" {
		t.Errorf("expected tags stripped, got %q", got)
	}
}

func TestFormatForChannel_EmptyAfterStrip(t *testing.T) {
	t.Parallel()
	got := FormatForChannel("NO_REPLY", "whatsapp")
	if got != "" {
		t.Errorf("expected empty after stripping NO_REPLY, got %q", got)
	}
}

func TestFormatForWhatsApp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**hello**", "_hello_"},      // **x** → *x* → _x_ (italic pass)
		{"header h1", "# Title", "_Title_"},    // header → *Title* → _Title_
		{"header h2", "## Subtitle", "_Subtitle_"},
		{"header h3", "### Deep", "_Deep_"},
		{"link", "[click](http://x.com)", "click (http://x.com)"},
		{"image", "![alt](http://img.png)", "!alt (http://img.png)"}, // link regex runs first
		{"unordered dash", "- item one", "• item one"},
		{"strikethrough", "~~deleted~~", "~deleted~"},
		{"code block preserved", "```go\nfmt.Println()\n```", "```\nfmt.Println()\n```"},
		{"inline code", "`code`", "`code`"},
		{"horizontal rule dashes", "---", "───────"},
		{"horizontal rule stars", "***", "───────"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatForWhatsApp(tt.in)
			if got != tt.want {
				t.Errorf("FormatForWhatsApp(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatForTelegram(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**hello**", "<b>hello</b>"},
		{"html escape", "a < b & c > d", "a &lt; b &amp; c &gt; d"},
		{"strikethrough", "~~gone~~", "<s>gone</s>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatForTelegram(tt.in)
			if got != tt.want {
				t.Errorf("FormatForTelegram(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatForSlack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**hello**", "*hello*"},
		{"strikethrough", "~~gone~~", "~gone~"},
		{"link", "[click](http://x.com)", "<http://x.com|click>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatForSlack(tt.in)
			if got != tt.want {
				t.Errorf("FormatForSlack(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatForPlainText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**hello**", "hello"},
		{"italic underscore", "_hello_", "hello"},
		{"italic star", "*hello*", "hello"},
		{"strikethrough", "~~hello~~", "hello"},
		{"link", "[text](http://x.com)", "text"},
		{"image", "![alt](http://img.png)", "!alt"},
		{"header", "## Title", "Title"},
		{"code block", "```go\ncode\n```", "code"},
		{"inline code", "`code`", "code"},
		{"blockquote", "> quote", "quote"},
		{"horizontal rule", "---", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatForPlainText(tt.in)
			if got != tt.want {
				t.Errorf("FormatForPlainText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
