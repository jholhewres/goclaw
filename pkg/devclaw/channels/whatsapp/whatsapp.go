// Package whatsapp implements the WhatsApp channel for DevClaw using
// whatsmeow — a native Go WhatsApp Web API library. No Node.js, no Baileys.
//
// Features:
//   - QR code login with persistent session
//   - Send/receive text, images, audio, video, documents, stickers
//   - Group message support
//   - Reply and quoting
//   - Reactions (emoji)
//   - Typing indicators and read receipts
//   - Media upload/download with encryption
//   - Automatic reconnection
//
// This is a core channel (compiled into the binary, not a plugin).
package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for session store.
)

// Config holds WhatsApp channel configuration.
type Config struct {
	// SessionDir is the directory for session persistence (SQLite).
	SessionDir string `yaml:"session_dir"`

	// Trigger is the keyword that activates the bot (e.g. "@devclaw").
	Trigger string `yaml:"trigger"`

	// RespondToGroups enables responding in group chats.
	RespondToGroups bool `yaml:"respond_to_groups"`

	// RespondToDMs enables responding in direct messages.
	RespondToDMs bool `yaml:"respond_to_dms"`

	// AutoRead marks incoming messages as read.
	AutoRead bool `yaml:"auto_read"`

	// SendTyping sends typing indicators while processing.
	SendTyping bool `yaml:"send_typing"`

	// MediaDir is the directory for downloaded media files.
	MediaDir string `yaml:"media_dir"`

	// MaxMediaSizeMB is the maximum media file size to process.
	MaxMediaSizeMB int `yaml:"max_media_size_mb"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		SessionDir:      "./sessions/whatsapp",
		Trigger:         "@devclaw",
		RespondToGroups: true,
		RespondToDMs:    true,
		AutoRead:        true,
		SendTyping:      true,
		MediaDir:        "./data/media",
		MaxMediaSizeMB:  16,
	}
}

// QREvent represents a QR code event sent to observers.
type QREvent struct {
	// Type is "code", "success", "timeout", or "error".
	Type string `json:"type"`
	// Code is the raw QR code string (only for Type == "code").
	Code string `json:"code,omitempty"`
	// Message is a human-readable description.
	Message string `json:"message,omitempty"`
}

// WhatsApp implements the channels.Channel, channels.MediaChannel,
// channels.PresenceChannel, and channels.ReactionChannel interfaces.
type WhatsApp struct {
	cfg    Config
	client *whatsmeow.Client
	logger *slog.Logger

	// messages is the channel for incoming messages.
	messages chan *channels.IncomingMessage

	// connected tracks connection state.
	connected atomic.Bool

	// lastMsg tracks the last message timestamp for health.
	lastMsg atomic.Value // time.Time

	// errorCount tracks consecutive errors.
	errorCount atomic.Int64

	// qrObservers receives QR events (for web UI).
	qrObservers   []chan QREvent
	qrObserversMu sync.Mutex
	// lastQR caches the most recent QR code so late-joining observers get it.
	lastQR *QREvent

	// ctx and cancel for lifecycle management.
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
}

// SubscribeQR registers a channel to receive QR code events.
// Returns an unsubscribe function.
func (w *WhatsApp) SubscribeQR() (chan QREvent, func()) {
	ch := make(chan QREvent, 8)
	w.qrObserversMu.Lock()
	w.qrObservers = append(w.qrObservers, ch)
	// Replay the last QR code to the new observer so it doesn't miss it.
	if w.lastQR != nil {
		select {
		case ch <- *w.lastQR:
		default:
		}
	}
	w.qrObserversMu.Unlock()

	return ch, func() {
		w.qrObserversMu.Lock()
		defer w.qrObserversMu.Unlock()
		for i, obs := range w.qrObservers {
			if obs == ch {
				w.qrObservers = append(w.qrObservers[:i], w.qrObservers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// notifyQR sends a QR event to all observers.
func (w *WhatsApp) notifyQR(evt QREvent) {
	w.qrObserversMu.Lock()
	defer w.qrObserversMu.Unlock()
	// Cache the latest QR code for late-joining observers.
	if evt.Type == "code" {
		w.lastQR = &evt
	} else {
		// Clear cache on success/timeout/error — QR is no longer valid.
		w.lastQR = nil
	}
	for _, ch := range w.qrObservers {
		select {
		case ch <- evt:
		default:
			// Observer too slow, skip.
		}
	}
}

// NeedsQR returns true if the WhatsApp session is not linked (needs QR scan).
func (w *WhatsApp) NeedsQR() bool {
	return w.client != nil && w.client.Store.ID == nil && !w.connected.Load()
}

// New creates a new WhatsApp channel instance.
func New(cfg Config, logger *slog.Logger) *WhatsApp {
	if logger == nil {
		logger = slog.Default()
	}
	return &WhatsApp{
		cfg:      cfg,
		logger:   logger.With("component", "whatsapp"),
		messages: make(chan *channels.IncomingMessage, 256),
	}
}

// ---------- Channel Interface ----------

// Name returns "whatsapp".
func (w *WhatsApp) Name() string { return "whatsapp" }

// Connect establishes the WhatsApp Web connection via whatsmeow.
// If no existing session is found, the QR login process runs in the
// background (non-blocking) so the server can start immediately.
// The QR code is streamed to web UI observers for scanning via browser.
func (w *WhatsApp) Connect(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Initialize session store (SQLite).
	dbPath := w.cfg.SessionDir + "/whatsapp.db"
	container, err := sqlstore.New(w.ctx, "sqlite3",
		fmt.Sprintf("file:%s?_foreign_keys=1&_journal_mode=WAL", dbPath),
		waLog.Noop)
	if err != nil {
		return fmt.Errorf("creating session store: %w", err)
	}

	// Get or create device.
	device, err := w.getDevice(w.ctx, container)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	// Set device name shown in WhatsApp linked devices list.
	store.SetOSInfo("DevClaw", [3]uint32{1, 0, 0})

	// Create client.
	w.client = whatsmeow.NewClient(device, waLog.Noop)
	w.client.AddEventHandler(w.handleEvent)

	// Connect.
	if w.client.Store.ID == nil {
		// First login — start QR process in background (non-blocking).
		// The QR code is delivered to web UI observers via notifyQR.
		// The user scans it via the web dashboard, not the terminal.
		w.logger.Info("whatsapp: no existing session, QR code required — scan via web UI")
		go func() {
			if err := w.loginWithQR(w.ctx); err != nil {
				w.logger.Warn("whatsapp: QR login pending", "error", err)
			}
		}()
		return nil
	}

	// Existing session — reconnect.
	err = w.client.Connect()
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}

	w.connected.Store(true)
	w.logger.Info("whatsapp: connected (existing session)")
	return nil
}

// Disconnect gracefully closes the WhatsApp connection.
func (w *WhatsApp) Disconnect() error {
	w.connected.Store(false)
	if w.cancel != nil {
		w.cancel()
	}
	if w.client != nil {
		w.client.Disconnect()
	}
	close(w.messages)
	w.logger.Info("whatsapp: disconnected")
	return nil
}

// Send sends a text message to the specified JID.
func (w *WhatsApp) Send(ctx context.Context, to string, msg *channels.OutgoingMessage) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	jid, err := parseJID(to)
	if err != nil {
		return fmt.Errorf("invalid JID %q: %w", to, err)
	}

	waMsg := buildTextMessage(msg.Content, msg.ReplyTo)

	_, err = w.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		w.errorCount.Add(1)
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

// Receive returns the incoming messages channel.
func (w *WhatsApp) Receive() <-chan *channels.IncomingMessage {
	return w.messages
}

// IsConnected returns true if WhatsApp is connected.
func (w *WhatsApp) IsConnected() bool {
	return w.connected.Load()
}

// Health returns the WhatsApp channel health status.
func (w *WhatsApp) Health() channels.HealthStatus {
	h := channels.HealthStatus{
		Connected:  w.connected.Load(),
		ErrorCount: int(w.errorCount.Load()),
		Details:    make(map[string]any),
	}
	if t, ok := w.lastMsg.Load().(time.Time); ok {
		h.LastMessageAt = t
	}
	if w.client != nil && w.client.Store.ID != nil {
		h.Details["jid"] = w.client.Store.ID.String()
		h.Details["platform"] = w.client.Store.Platform
	}
	return h
}

// ---------- MediaChannel Interface ----------

// SendMedia sends a media message (image, audio, video, document, sticker).
func (w *WhatsApp) SendMedia(ctx context.Context, to string, media *channels.MediaMessage) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	jid, err := parseJID(to)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	waMsg, err := w.buildMediaMessage(ctx, media)
	if err != nil {
		return fmt.Errorf("building media message: %w", err)
	}

	_, err = w.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		w.errorCount.Add(1)
		return fmt.Errorf("sending media: %w", err)
	}

	return nil
}

// DownloadMedia downloads media from an incoming message.
func (w *WhatsApp) DownloadMedia(ctx context.Context, msg *channels.IncomingMessage) ([]byte, string, error) {
	if msg.Media == nil {
		return nil, "", fmt.Errorf("message has no media")
	}
	return w.downloadMediaFromInfo(ctx, msg.Media)
}

// ---------- PresenceChannel Interface ----------

// SendTyping sends a typing indicator.
func (w *WhatsApp) SendTyping(ctx context.Context, to string) error {
	if !w.connected.Load() {
		return nil
	}
	jid, err := parseJID(to)
	if err != nil {
		return err
	}
	return w.client.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
}

// SendPresence updates the bot's online/offline status.
func (w *WhatsApp) SendPresence(ctx context.Context, available bool) error {
	if !w.connected.Load() {
		return nil
	}
	if available {
		return w.client.SendPresence(ctx, types.PresenceAvailable)
	}
	return w.client.SendPresence(ctx, types.PresenceUnavailable)
}

// MarkRead marks messages as read.
func (w *WhatsApp) MarkRead(ctx context.Context, chatID string, messageIDs []string) error {
	if !w.connected.Load() {
		return nil
	}
	jid, err := parseJID(chatID)
	if err != nil {
		return err
	}

	ids := make([]types.MessageID, len(messageIDs))
	for i, id := range messageIDs {
		ids[i] = types.MessageID(id)
	}

	return w.client.MarkRead(ctx, ids, time.Now(), jid, jid)
}

// ---------- ReactionChannel Interface ----------

// SendReaction sends an emoji reaction to a message.
func (w *WhatsApp) SendReaction(ctx context.Context, chatID, messageID, emoji string) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	jid, err := parseJID(chatID)
	if err != nil {
		return err
	}

	waMsg := buildReactionMessage(messageID, jid, emoji)
	_, err = w.client.SendMessage(ctx, jid, waMsg)
	return err
}

// ---------- Internal ----------

// getDevice retrieves an existing device or creates a new one.
func (w *WhatsApp) getDevice(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	if len(devices) > 0 {
		return devices[0], nil
	}
	return container.NewDevice(), nil
}

// loginWithQR handles the QR code login flow.
// QR codes are delivered exclusively to web UI observers (no terminal output).
// This is designed for headless/server deployments managed via the web dashboard.
func (w *WhatsApp) loginWithQR(ctx context.Context) error {
	qrChan, _ := w.client.GetQRChannel(ctx)
	err := w.client.Connect()
	if err != nil {
		return fmt.Errorf("connecting for QR: %w", err)
	}

	w.logger.Info("whatsapp: waiting for QR code scan via web UI")

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			w.logger.Info("whatsapp: QR code ready, scan via web UI at /channels/whatsapp")
			w.notifyQR(QREvent{Type: "code", Code: evt.Code, Message: "Scan the QR code to link WhatsApp"})

		case "success":
			w.connected.Store(true)
			w.logger.Info("whatsapp: login successful!")
			w.notifyQR(QREvent{Type: "success", Message: "WhatsApp linked successfully!"})
			return nil

		case "timeout":
			w.notifyQR(QREvent{Type: "timeout", Message: "QR code expired — refresh to try again"})
			return fmt.Errorf("QR code timeout — scan via web UI to retry")

		default:
			if evt.Error != nil {
				w.notifyQR(QREvent{Type: "error", Message: evt.Error.Error()})
				return fmt.Errorf("QR login error: %v", evt.Error)
			}
		}
	}

	return fmt.Errorf("QR channel closed unexpectedly")
}

// RequestNewQR disconnects and reconnects to generate a fresh QR code.
// This is used when the web UI needs a new QR after timeout.
func (w *WhatsApp) RequestNewQR(ctx context.Context) error {
	if w.connected.Load() {
		return fmt.Errorf("already connected")
	}
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Disconnect current attempt if any.
	w.client.Disconnect()

	// Re-login with QR in a goroutine (non-blocking for the web handler).
	go func() {
		if err := w.loginWithQR(ctx); err != nil {
			w.logger.Error("whatsapp: QR re-login failed", "error", err)
			w.notifyQR(QREvent{Type: "error", Message: err.Error()})
		}
	}()

	return nil
}

// emitMessage sends a message to the incoming messages channel.
func (w *WhatsApp) emitMessage(msg *channels.IncomingMessage) {
	select {
	case w.messages <- msg:
		w.lastMsg.Store(time.Now())
	case <-w.ctx.Done():
	default:
		w.logger.Warn("whatsapp: message channel full, dropping message",
			"from", msg.From, "type", msg.Type)
	}
}
