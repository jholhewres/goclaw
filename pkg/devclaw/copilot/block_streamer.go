// Package copilot – block_streamer.go implements progressive message delivery
// for channels. Instead of waiting for the full LLM response, text is coalesced
// into blocks and sent as they become available, giving the user near-real-time
// feedback as blocks become available.
//
// Coalescing rules:
//   - Wait until at least MinChars are accumulated.
//   - Flush when MaxChars is reached or the idle timer fires.
//   - Always try to flush at a natural boundary (newline, sentence end).
package copilot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// BlockStreamConfig configures the progressive message streaming behavior.
type BlockStreamConfig struct {
	// Enabled turns block streaming on/off (default: true).
	Enabled bool `yaml:"enabled"`

	// MinChars is the minimum characters to accumulate before sending a block (default: 20).
	// Kept low for near-instant first-block feedback.
	MinChars int `yaml:"min_chars"`

	// MaxChars is the maximum characters per block before a forced flush (default: 600).
	MaxChars int `yaml:"max_chars"`

	// IdleMs is the idle timeout in milliseconds: if no new tokens arrive within
	// this window, flush whatever is buffered (default: 200).
	IdleMs int `yaml:"idle_ms"`
}

// DefaultBlockStreamConfig returns sensible defaults for block streaming.
// Tuned for WhatsApp/chat UX: each flush = a new message, so we prioritize
// sending coherent paragraphs over low-latency fragments. MinChars must be
// high enough to avoid sending single-line fragments as separate messages.
func DefaultBlockStreamConfig() BlockStreamConfig {
	return BlockStreamConfig{
		Enabled:  true,
		MinChars: 200,  // ~40 words — avoids tiny fragments as separate messages
		MaxChars: 1500, // Full paragraph; WhatsApp supports up to 65K chars
		IdleMs:   1500, // Flush 1.5s after last token — allows sentences to complete
	}
}

// Effective returns a copy with defaults filled in for zero values.
func (c BlockStreamConfig) Effective() BlockStreamConfig {
	out := c
	if out.MinChars <= 0 {
		out.MinChars = 200
	}
	if out.MaxChars <= 0 {
		out.MaxChars = 1500
	}
	if out.IdleMs <= 0 {
		out.IdleMs = 1500
	}
	return out
}

// BlockStreamer accumulates LLM stream tokens and sends them progressively
// to a channel. It is tied to a single message exchange (one user message →
// one agent response).
type BlockStreamer struct {
	cfg        BlockStreamConfig
	channelMgr *channels.Manager
	channel    string
	chatID     string
	replyTo    string // original message ID for threading

	mu      sync.Mutex
	buf     strings.Builder
	sent    int  // total chars sent so far
	done    bool // Finish() was called
	flushed bool // at least one block was sent

	idleTimer *time.Timer
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewBlockStreamer creates a streamer that progressively sends blocks to the given channel.
func NewBlockStreamer(
	cfg BlockStreamConfig,
	channelMgr *channels.Manager,
	channel, chatID, replyTo string,
) *BlockStreamer {
	cfg = cfg.Effective()
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockStreamer{
		cfg:        cfg,
		channelMgr: channelMgr,
		channel:    channel,
		chatID:     chatID,
		replyTo:    replyTo,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// StreamCallback returns a StreamCallback function suitable for AgentRun.SetStreamCallback.
func (bs *BlockStreamer) StreamCallback() StreamCallback {
	return func(chunk string) {
		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.done {
			return
		}

		bs.buf.WriteString(chunk)

		// Reset idle timer on every token.
		bs.resetIdleTimer()

		// Check if we should flush.
		if bs.buf.Len() >= bs.cfg.MaxChars {
			bs.flushLocked()
		}
	}
}

// FlushNow immediately sends any buffered text to the channel, regardless of
// MinChars threshold. Use this before tool execution to ensure the user sees
// the LLM's intermediate text (thoughts/reasoning) before tools start running.
func (bs *BlockStreamer) FlushNow() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.done || bs.buf.Len() == 0 {
		return
	}
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}
	bs.flushLocked()
}

// Finish flushes any remaining buffer and marks the streamer as done.
func (bs *BlockStreamer) Finish() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.done {
		return // Already finished — idempotent.
	}

	bs.done = true
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}

	// IMPORTANT: Flush remaining text BEFORE cancelling the context.
	// The send operation uses bs.ctx, so cancelling first would silently
	// drop the final message — causing the user to never receive the response.
	if bs.buf.Len() > 0 {
		bs.flushLocked()
	}

	bs.cancel()
}

// HasSentBlocks returns true if at least one block was sent progressively.
func (bs *BlockStreamer) HasSentBlocks() bool {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.flushed
}

// idleMinChars is the minimum buffer size for an idle flush. When the LLM
// pauses briefly mid-sentence, we don't want to send tiny fragments like
// "(collect," as separate messages. If the buffer is below this threshold,
// the idle timer reschedules itself to wait longer.
const idleMinChars = 80

// resetIdleTimer resets the idle flush timer. Must be called with mu held.
// When the LLM pauses (tool call, thinking), the idle timer fires and flushes
// whatever is buffered so the user sees progress.
func (bs *BlockStreamer) resetIdleTimer() {
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}

	idleDuration := time.Duration(bs.cfg.IdleMs) * time.Millisecond
	bs.idleTimer = time.AfterFunc(idleDuration, func() {
		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.done || bs.buf.Len() == 0 {
			return
		}

		// If the buffer is too small, don't send a tiny fragment — reschedule
		// the timer to give the LLM more time to produce a coherent block.
		if bs.buf.Len() < idleMinChars {
			bs.idleTimer = time.AfterFunc(idleDuration, func() {
				bs.mu.Lock()
				defer bs.mu.Unlock()
				if !bs.done && bs.buf.Len() > 0 {
					bs.flushLocked()
				}
			})
			return
		}

		bs.flushLocked()
	})
}

// flushLocked sends the current buffer as a message block. Must be called with mu held.
func (bs *BlockStreamer) flushLocked() {
	text := bs.buf.String()
	if len(strings.TrimSpace(text)) == 0 {
		return
	}

	// Try to break at a natural boundary if we're mid-buffer and over MinChars.
	sendText := text
	remainder := ""

	if len(text) > bs.cfg.MinChars && !bs.done {
		// Look for a good break point near MinChars..MaxChars.
		breakIdx := findNaturalBreak(text, bs.cfg.MinChars, bs.cfg.MaxChars)
		if breakIdx > 0 && breakIdx < len(text) {
			sendText = text[:breakIdx]
			remainder = text[breakIdx:]
		}
	}

	// Quick format for <think> blocks to render beautifully for the user.
	// Since `<think>` is typically a single LLM token, it rarely cross-cuts flushes.
	sendText = strings.ReplaceAll(sendText, "<think>", "💭 *Thinking...*\n")
	sendText = strings.ReplaceAll(sendText, "</think>", "")

	// Format for channel (also strips reply tags like [[reply_to_current]]).
	sendText = FormatForChannel(sendText, bs.channel)
	if len(strings.TrimSpace(sendText)) == 0 {
		return // Empty after stripping tags — nothing to send.
	}

	msg := &channels.OutgoingMessage{
		Content: strings.TrimSpace(sendText),
		ReplyTo: bs.replyTo,
	}

	if err := bs.channelMgr.Send(bs.ctx, bs.channel, bs.chatID, msg); err != nil {
		// Silently ignore send errors during streaming — the final sendReply
		// will attempt to send the complete message as fallback.
		return
	}

	bs.flushed = true
	bs.sent += len(sendText)

	// Reset buffer with remainder.
	bs.buf.Reset()
	if remainder != "" {
		bs.buf.WriteString(remainder)
	}
}

// findNaturalBreak finds a good text break point between minIdx and maxIdx.
// Prefers paragraph breaks > sentence ends > word boundaries.
func findNaturalBreak(text string, minIdx, maxIdx int) int {
	if maxIdx > len(text) {
		maxIdx = len(text)
	}
	if minIdx >= maxIdx {
		return maxIdx
	}

	region := text[minIdx:maxIdx]

	// Look for paragraph break (double newline).
	if idx := strings.LastIndex(region, "\n\n"); idx >= 0 {
		return minIdx + idx + 2
	}

	// Look for single newline.
	if idx := strings.LastIndex(region, "\n"); idx >= 0 {
		return minIdx + idx + 1
	}

	// Look for sentence end (. ! ?).
	for i := len(region) - 1; i >= 0; i-- {
		ch := region[i]
		if (ch == '.' || ch == '!' || ch == '?') && i+1 < len(region) && region[i+1] == ' ' {
			return minIdx + i + 2
		}
	}

	// Look for word boundary (space).
	if idx := strings.LastIndex(region, " "); idx >= 0 {
		return minIdx + idx + 1
	}

	return maxIdx
}
