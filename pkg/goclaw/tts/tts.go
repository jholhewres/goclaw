// Package tts provides text-to-speech synthesis for GoClaw.
// Supports multiple providers: OpenAI TTS (paid, high quality),
// Edge TTS (free, Microsoft Azure voices), and auto-fallback.
package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Provider is the interface for TTS backends.
type Provider interface {
	// Synthesize converts text to audio.
	// Returns audio bytes, MIME type (e.g. "audio/ogg"), and error.
	Synthesize(ctx context.Context, text, voice string) ([]byte, string, error)
}

// OpenAIProvider implements TTS via the OpenAI TTS API.
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewOpenAIProvider creates an OpenAI TTS provider.
func NewOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "tts-1"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Synthesize converts text to audio using the OpenAI TTS API.
// Returns audio in Opus format (ideal for voice notes on WhatsApp/Telegram).
func (p *OpenAIProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	if voice == "" {
		voice = "nova"
	}

	// Truncate very long text (TTS has a 4096 char limit).
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}

	payload := map[string]any{
		"model":           p.model,
		"input":           text,
		"voice":           voice,
		"response_format": "opus",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("tts: marshal request: %w", err)
	}

	url := p.baseURL + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("tts: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("tts: API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("tts: API returned %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("tts: reading audio: %w", err)
	}

	return audio, "audio/ogg", nil
}

// ============================================================
// Edge TTS Provider (free, Microsoft Azure voices via edge-tts REST)
// ============================================================

// EdgeProvider implements TTS via Microsoft Edge's speech synthesis service.
// Uses the same underlying Azure Cognitive Services as the edge-tts Python
// package, but via direct HTTP calls — no external dependencies needed.
//
// Popular voices:
//   - pt-BR-FranciscaNeural (Brazilian Portuguese, female)
//   - pt-BR-AntonioNeural (Brazilian Portuguese, male)
//   - en-US-JennyNeural (US English, female)
//   - en-US-GuyNeural (US English, male)
//   - es-ES-ElviraNeural (Spanish, female)
type EdgeProvider struct {
	client *http.Client
	logger *slog.Logger
}

// NewEdgeProvider creates an Edge TTS provider.
func NewEdgeProvider(logger *slog.Logger) *EdgeProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &EdgeProvider{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger.With("component", "edge-tts"),
	}
}

// edgeTTSEndpoint is the Microsoft Edge speech synthesis endpoint.
const edgeTTSEndpoint = "https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/naturaltts/v1"

// Synthesize converts text to audio using Microsoft Edge TTS.
// Returns audio in MP3 format.
func (p *EdgeProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	if voice == "" {
		voice = "pt-BR-FranciscaNeural"
	}

	// Truncate long text.
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}

	// Escape XML special chars in text for SSML.
	text = escapeXML(text)

	// Build SSML payload.
	ssml := fmt.Sprintf(`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'><voice name='%s'><prosody pitch='+0Hz' rate='+0%%' volume='+0%%'>%s</prosody></voice></speak>`,
		voice, text)

	// Edge TTS uses a WebSocket protocol normally, but we can use the REST
	// endpoint that the Bing Translator and Edge Read Aloud feature use.
	url := edgeTTSEndpoint + "?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4&ConnectionId=gen&Enc=mp3&OutputFormat=audio-24khz-48kbitrate-mono-mp3"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(ssml))
	if err != nil {
		return nil, "", fmt.Errorf("edge-tts: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")
	req.Header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("edge-tts: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("edge-tts: HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("edge-tts: reading audio: %w", err)
	}

	if len(audio) == 0 {
		return nil, "", fmt.Errorf("edge-tts: empty audio response")
	}

	// The response might include WebSocket-like binary headers.
	// Strip them to get pure MP3.
	audio = stripEdgeHeaders(audio)

	return audio, "audio/mpeg", nil
}

// stripEdgeHeaders strips any WebSocket binary framing from the response.
// Edge TTS REST responses sometimes include header bytes before the MP3 data.
func stripEdgeHeaders(data []byte) []byte {
	// Look for MP3 sync word (0xFF 0xFB or 0xFF 0xF3).
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			return data[i:]
		}
	}
	// If it starts with a header length (2 bytes big-endian), try to skip.
	if len(data) > 2 {
		headerLen := int(binary.BigEndian.Uint16(data[:2]))
		if headerLen > 0 && headerLen < len(data) {
			return data[headerLen:]
		}
	}
	return data
}

// escapeXML escapes special XML characters in text for SSML.
func escapeXML(text string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(text)
}

// ============================================================
// Fallback Provider (tries primary, falls back to secondary)
// ============================================================

// FallbackProvider tries the primary provider and falls back to the
// secondary if the primary fails. Useful for "auto" mode where OpenAI
// is preferred but Edge TTS is the free fallback.
type FallbackProvider struct {
	primary       Provider
	secondary     Provider
	primaryVoice  string
	secondaryVoice string
	logger        *slog.Logger
}

// NewFallbackProvider creates a provider that tries primary first, then secondary.
func NewFallbackProvider(primary, secondary Provider, primaryVoice, secondaryVoice string, logger *slog.Logger) *FallbackProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackProvider{
		primary:        primary,
		secondary:      secondary,
		primaryVoice:   primaryVoice,
		secondaryVoice: secondaryVoice,
		logger:         logger.With("component", "tts-fallback"),
	}
}

// Synthesize tries the primary provider, falling back to secondary on failure.
func (p *FallbackProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	// Try primary with its configured voice.
	primaryV := voice
	if primaryV == "" {
		primaryV = p.primaryVoice
	}
	audio, mime, err := p.primary.Synthesize(ctx, text, primaryV)
	if err == nil {
		return audio, mime, nil
	}

	p.logger.Warn("primary TTS failed, trying fallback", "error", err)

	// Fallback with secondary voice.
	secondaryV := p.secondaryVoice
	if secondaryV == "" {
		secondaryV = voice
	}
	return p.secondary.Synthesize(ctx, text, secondaryV)
}
