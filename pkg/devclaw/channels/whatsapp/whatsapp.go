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
//   - Automatic reconnection with backoff
//   - Connection state management and events
//
// This is a core channel (compiled into the binary, not a plugin).
package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	// Ignored if DatabasePath is set.
	SessionDir string `yaml:"session_dir"`

	// DatabasePath is the path to the SQLite database file for session storage.
	// If set, the WhatsApp session tables (prefixed with whatsmeow_) will be
	// stored in this database alongside other devclaw data.
	// If empty, defaults to {SessionDir}/whatsapp.db.
	DatabasePath string `yaml:"database_path"`

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

	// ReconnectBackoff is the initial backoff duration for reconnection.
	ReconnectBackoff time.Duration `yaml:"reconnect_backoff"`

	// MaxReconnectAttempts is the maximum number of reconnection attempts (0 = unlimited).
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`

	// HealthMonitor configures proactive connection health monitoring.
	HealthMonitor HealthMonitorConfig `yaml:"health_monitor"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		SessionDir:          "./sessions/whatsapp",
		Trigger:             "@devclaw",
		RespondToGroups:     true,
		RespondToDMs:        true,
		AutoRead:            true,
		SendTyping:          true,
		MediaDir:            "./data/media",
		MaxMediaSizeMB:      16,
		ReconnectBackoff:    5 * time.Second,
		MaxReconnectAttempts: 10,
		HealthMonitor:       DefaultHealthMonitorConfig(),
	}
}

// QREvent represents a QR code event sent to observers.
type QREvent struct {
	// Type is "code", "success", "timeout", "error", or "refresh".
	Type string `json:"type"`
	// Code is the raw QR code string (only for Type == "code").
	Code string `json:"code,omitempty"`
	// Message is a human-readable description.
	Message string `json:"message,omitempty"`
	// ExpiresAt is when the QR code expires.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// SecondsLeft is seconds until expiration.
	SecondsLeft int `json:"seconds_left,omitempty"`
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

	// state tracks detailed connection state.
	state atomic.Value // ConnectionState

	// lastMsg tracks the last message timestamp for health.
	lastMsg atomic.Value // time.Time

	// errorCount tracks consecutive errors.
	errorCount atomic.Int64

	// reconnectAttempts tracks reconnection tries (thread-safe).
	reconnectAttempts atomic.Int32

	// qrObservers receives QR events (for web UI).
	qrObservers   []chan QREvent
	qrObserversMu sync.Mutex
	// lastQR caches the most recent QR code so late-joining observers get it.
	lastQR *QREvent
	// qrGeneratedAt tracks when QR was generated for expiration.
	qrGeneratedAt time.Time

	// connObservers receives connection state changes.
	connObservers   []ConnectionObserver
	connObserversMu sync.Mutex

	// ctx and cancel for lifecycle management.
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex

	// reconnectGuard prevents multiple concurrent reconnection attempts.
	reconnectGuard atomic.Bool

	// messagesClosed tracks if the messages channel has been closed.
	// This prevents sending to a closed channel which would cause a panic.
	messagesClosed atomic.Bool
}

// New creates a new WhatsApp channel instance.
func New(cfg Config, logger *slog.Logger) *WhatsApp {
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults.
	if cfg.ReconnectBackoff == 0 {
		cfg.ReconnectBackoff = 5 * time.Second
	}

	w := &WhatsApp{
		cfg:       cfg,
		logger:    logger.With("component", "whatsapp"),
		messages:  make(chan *channels.IncomingMessage, 256),
	}
	w.setState(StateDisconnected)
	return w
}

// ---------- State Management ----------

// getState returns the current connection state.
func (w *WhatsApp) getState() ConnectionState {
	if v := w.state.Load(); v != nil {
		return v.(ConnectionState)
	}
	return StateDisconnected
}

// setState updates the connection state.
func (w *WhatsApp) setState(state ConnectionState) {
	w.state.Store(state)
}

// GetState returns the current connection state (public API).
func (w *WhatsApp) GetState() ConnectionState {
	return w.getState()
}

// getClientJID returns the current client JID if connected.
func (w *WhatsApp) getClientJID() string {
	if w.client != nil && w.client.Store.ID != nil {
		return w.client.Store.ID.String()
	}
	return ""
}

// getClientPlatform returns the current platform.
func (w *WhatsApp) getClientPlatform() string {
	if w.client != nil && w.client.Store.Platform != "" {
		return w.client.Store.Platform
	}
	return ""
}

// ---------- QR Code Subscription ----------

// SubscribeQR registers a channel to receive QR code events.
// Returns an unsubscribe function.
func (w *WhatsApp) SubscribeQR() (chan QREvent, func()) {
	ch := make(chan QREvent, 8)
	w.qrObserversMu.Lock()
	w.qrObservers = append(w.qrObservers, ch)
	// Replay the last QR code to the new observer so it doesn't miss it.
	if w.lastQR != nil {
		// Calculate remaining time.
		evt := *w.lastQR
		if !w.qrGeneratedAt.IsZero() {
			elapsed := time.Since(w.qrGeneratedAt)
			evt.SecondsLeft = max(0, 60-int(elapsed.Seconds()))
		}
		select {
		case ch <- evt:
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
		w.qrGeneratedAt = time.Now()
	} else {
		// Clear cache on success/timeout/error — QR is no longer valid.
		w.lastQR = nil
		w.qrGeneratedAt = time.Time{}
	}

	for _, ch := range w.qrObservers {
		select {
		case ch <- evt:
		default:
			// Observer too slow, skip.
		}
	}
}

// ---------- Connection Observer ----------

// AddConnectionObserver registers a connection observer.
func (w *WhatsApp) AddConnectionObserver(obs ConnectionObserver) {
	w.connObserversMu.Lock()
	defer w.connObserversMu.Unlock()
	w.connObservers = append(w.connObservers, obs)
}

// notifyConnectionChange notifies all connection observers.
func (w *WhatsApp) notifyConnectionChange(evt ConnectionEvent) {
	w.connObserversMu.Lock()
	observers := make([]ConnectionObserver, len(w.connObservers))
	copy(observers, w.connObservers)
	w.connObserversMu.Unlock()

	for _, obs := range observers {
		go func(o ConnectionObserver) {
			defer func() {
				if r := recover(); r != nil {
					w.logger.Warn("whatsapp: connection observer panic", "error", r)
				}
			}()
			o.OnConnectionChange(evt)
		}(obs)
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

	w.setState(StateConnecting)
	w.logger.Info("whatsapp: initializing connection...")

	// Initialize session store (SQLite).
	// Use DatabasePath if provided, otherwise fall back to SessionDir/whatsapp.db.
	dbPath := w.cfg.DatabasePath
	if dbPath == "" {
		dbPath = w.cfg.SessionDir + "/whatsapp.db"
		w.logger.Info("whatsapp: using standalone session database", "path", dbPath)
	} else {
		w.logger.Info("whatsapp: using shared devclaw database for sessions", "path", dbPath)
	}
	container, err := sqlstore.New(w.ctx, "sqlite3",
		fmt.Sprintf("file:%s?_foreign_keys=1&_journal_mode=WAL", dbPath),
		waLog.Noop)
	if err != nil {
		w.setState(StateDisconnected)
		return fmt.Errorf("creating session store: %w", err)
	}

	// Get or create device.
	device, err := w.getDevice(w.ctx, container)
	if err != nil {
		w.setState(StateDisconnected)
		return fmt.Errorf("getting device: %w", err)
	}

	// Set device name shown in WhatsApp linked devices list.
	store.SetOSInfo("DevClaw", [3]uint32{1, 0, 0})

	// Create client.
	w.client = whatsmeow.NewClient(device, waLog.Noop)
	w.client.AddEventHandler(w.handleEvent)

	// Enable whatsmeow's built-in auto-reconnect for resilient connections.
	// This handles network hiccups, server-initiated disconnects, and
	// keepalive failures automatically.
	w.client.EnableAutoReconnect = true
	w.client.InitialAutoReconnect = true

	// Connect.
	if w.client.Store.ID == nil {
		// First login — start QR process in background (non-blocking).
		w.setState(StateWaitingQR)
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
		w.setState(StateDisconnected)
		return fmt.Errorf("connecting: %w", err)
	}

	w.connected.Store(true)
	w.logger.Info("whatsapp: connected (existing session)",
		"jid", w.getClientJID())

	// Start health monitoring.
	w.StartHealthMonitor(w.ctx, w.cfg.HealthMonitor)

	return nil
}

// Disconnect gracefully closes the WhatsApp connection.
func (w *WhatsApp) Disconnect() error {
	previous := w.getState()
	w.setState(StateDisconnected)
	w.connected.Store(false)

	if w.cancel != nil {
		w.cancel()
	}
	if w.client != nil {
		w.client.Disconnect()
	}

	// Mark channel as closed before actually closing to prevent
	// race condition with emitMessage trying to send to closed channel.
	if w.messagesClosed.CompareAndSwap(false, true) {
		close(w.messages)
	}

	w.logger.Info("whatsapp: disconnected")

	// Notify observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "user_request",
	})

	return nil
}

// Logout logs out and clears the session.
func (w *WhatsApp) Logout() error {
	if w.client == nil {
		return nil
	}

	previous := w.getState()
	w.setState(StateLoggingOut)
	w.connected.Store(false)

	// Logout from WhatsApp (this also disconnects and deletes store).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := w.client.Logout(ctx)
	if err != nil {
		w.logger.Warn("whatsapp: logout error, forcing cleanup", "error", err)
		w.client.Disconnect()
		if w.client.Store != nil {
			if delErr := w.client.Store.Delete(ctx); delErr != nil {
				w.logger.Warn("whatsapp: failed to delete store", "error", delErr)
			}
		}
	}

	w.setState(StateDisconnected)
	w.lastQR = nil

	w.logger.Info("whatsapp: logged out, session cleared")

	// Notify observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "logout",
		Details: map[string]any{
			"session_cleared": true,
			"needs_qr":        true,
		},
	})

	return nil
}

// attemptReconnect tries to reconnect with exponential backoff.
// Uses a guard pattern to prevent multiple concurrent reconnection attempts.
// This runs in a continuous loop until reconnection succeeds or max attempts reached.
func (w *WhatsApp) attemptReconnect() {
	// Guard: prevent multiple concurrent reconnection attempts.
	if !w.reconnectGuard.CompareAndSwap(false, true) {
		w.logger.Debug("whatsapp: reconnect already in progress, skipping")
		return
	}
	defer w.reconnectGuard.Store(false)

	previous := w.getState()
	w.setState(StateReconnecting)

	for {
		if w.ctx.Err() != nil {
			w.logger.Debug("whatsapp: reconnect cancelled, context done")
			return
		}

		attempts := w.reconnectAttempts.Add(1)
		if w.cfg.MaxReconnectAttempts > 0 && attempts > int32(w.cfg.MaxReconnectAttempts) {
			w.logger.Error("whatsapp: max reconnect attempts reached",
				"attempts", attempts)
			w.setState(StateDisconnected)
			w.notifyConnectionChange(ConnectionEvent{
				State:     StateDisconnected,
				Timestamp: time.Now(),
				Reason:    "max_reconnect_attempts",
				Details: map[string]any{
					"attempts": attempts,
				},
			})
			return
		}

		backoff := min(w.cfg.ReconnectBackoff*time.Duration(attempts), 5*time.Minute)

		w.logger.Info("whatsapp: attempting reconnect",
			"attempt", attempts,
			"backoff", backoff)

		// Notify observers.
		w.notifyConnectionChange(ConnectionEvent{
			State:     StateReconnecting,
			Previous:  previous,
			Timestamp: time.Now(),
			Reason:    "connection_lost",
			Details: map[string]any{
				"attempt":     attempts,
				"backoff_sec": backoff.Seconds(),
			},
		})

		// Wait for backoff period.
		select {
		case <-time.After(backoff):
		case <-w.ctx.Done():
			w.logger.Debug("whatsapp: reconnect cancelled during backoff")
			return
		}

		if w.client == nil {
			w.logger.Warn("whatsapp: client is nil, cannot reconnect")
			return
		}

		// Disconnect first to clear any stale websocket state.
		// This fixes "websocket is already connected" error on reconnect.
		if w.client.IsConnected() {
			w.client.Disconnect()
			time.Sleep(100 * time.Millisecond) // Brief pause to allow cleanup.
		}

		err := w.client.Connect()
		if err != nil {
			w.logger.Warn("whatsapp: reconnect attempt failed, will retry",
				"attempt", attempts,
				"error", err)
			// Continue loop to retry
			continue
		}

		// Connection succeeded - the Connected event will update state.
		w.logger.Info("whatsapp: reconnect connection initiated, waiting for confirmation")
		return
	}
}

// Send sends a text message to the specified JID.
func (w *WhatsApp) Send(ctx context.Context, to string, msg *channels.OutgoingMessage) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	// Suppress reasoning/thinking messages to prevent leaking internal thoughts.
	// This handles both explicit IsReasoning flag and content starting with "Reasoning:".
	if msg.IsReasoning || isReasoningContent(msg.Content) {
		w.logger.Debug("whatsapp: suppressing reasoning message", "content_preview", truncateString(msg.Content, 50))
		return nil // Successfully suppressed, not an error
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

// isReasoningContent checks if content appears to be reasoning/thinking output
// that should be suppressed from end-user delivery.
func isReasoningContent(content string) bool {
	if content == "" {
		return false
	}
	// Check for common reasoning prefixes
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "Reasoning:") ||
		strings.HasPrefix(trimmed, "Thinking:") ||
		strings.HasPrefix(trimmed, "<thinking>") ||
		strings.HasPrefix(trimmed, "<reasoning>")
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Receive returns the incoming messages channel.
func (w *WhatsApp) Receive() <-chan *channels.IncomingMessage {
	return w.messages
}

// IsConnected returns true if WhatsApp is connected.
func (w *WhatsApp) IsConnected() bool {
	return w.connected.Load()
}

// NeedsQR returns true if the WhatsApp session is not linked (needs QR scan).
func (w *WhatsApp) NeedsQR() bool {
	return w.client != nil && w.client.Store.ID == nil && !w.connected.Load()
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
	h.Details["state"] = string(w.getState())
	if w.client != nil && w.client.Store.ID != nil {
		h.Details["jid"] = w.client.Store.ID.String()
		h.Details["platform"] = w.client.Store.Platform
	}
	h.Details["reconnect_attempts"] = w.reconnectAttempts.Load()
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
	qrChan, err := w.client.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("getting QR channel: %w", err)
	}

	err = w.client.Connect()
	if err != nil {
		return fmt.Errorf("connecting for QR: %w", err)
	}

	w.setState(StateWaitingQR)
	w.logger.Info("whatsapp: waiting for QR code scan via web UI")

	qrAttempts := 0

	for {
		select {
		case <-ctx.Done():
			w.setState(StateDisconnected)
			return ctx.Err()
		case evt, ok := <-qrChan:
			if !ok {
				return fmt.Errorf("QR channel closed unexpectedly")
			}

			switch evt.Event {
			case "code":
				qrAttempts++
				w.setState(StateWaitingQR)
				w.logger.Info("whatsapp: QR code ready",
					"attempt", qrAttempts,
					"url", "/channels/whatsapp")

				w.notifyQR(QREvent{
					Type:    "code",
					Code:    evt.Code,
					Message: "Scan the QR code with WhatsApp to link your device",
				})

			case "success":
				w.connected.Store(true)
				w.reconnectAttempts.Store(0)
				w.setState(StateConnected)
				w.logger.Info("whatsapp: login successful!")
				w.notifyQR(QREvent{
					Type:    "success",
					Message: "WhatsApp linked successfully!",
				})
				return nil

			case "timeout":
				w.setState(StateDisconnected)
				w.logger.Warn("whatsapp: QR code expired")
				w.notifyQR(QREvent{
					Type:    "timeout",
					Message: "QR code expired — click refresh to try again",
				})
				return fmt.Errorf("QR code timeout")

			default:
				if evt.Error != nil {
					w.setState(StateDisconnected)
					w.logger.Error("whatsapp: QR login error", "error", evt.Error)
					w.notifyQR(QREvent{
						Type:    "error",
						Message: fmt.Sprintf("Error: %s", evt.Error.Error()),
					})
					return fmt.Errorf("QR login error: %v", evt.Error)
				}
			}
		}
	}
}

// RequestNewQR disconnects and reconnects to generate a fresh QR code.
// This is used when the web UI needs a new QR after timeout.
// A default timeout of 2 minutes is applied if the context has no deadline.
func (w *WhatsApp) RequestNewQR(ctx context.Context) error {
	if w.connected.Load() {
		return fmt.Errorf("already connected")
	}
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Disconnect current attempt if any.
	w.client.Disconnect()
	w.lastQR = nil

	// Notify that we're refreshing.
	w.notifyQR(QREvent{
		Type:    "refresh",
		Message: "Generating new QR code...",
	})

	// Re-login with QR in a goroutine (non-blocking for the web handler).
	go func() {
		// Apply default timeout if context has no deadline.
		qrCtx := ctx
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			qrCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
		}

		if err := w.loginWithQR(qrCtx); err != nil {
			w.logger.Error("whatsapp: QR re-login failed", "error", err)
		}
	}()

	return nil
}

// emitMessage sends a message to the incoming messages channel.
func (w *WhatsApp) emitMessage(msg *channels.IncomingMessage) {
	// Check if channel is already closed to prevent panic.
	if w.messagesClosed.Load() {
		return
	}

	select {
	case w.messages <- msg:
		w.lastMsg.Store(time.Now())
	case <-w.ctx.Done():
	default:
		w.logger.Warn("whatsapp: message channel full, dropping message",
			"from", msg.From, "type", msg.Type)
	}
}
