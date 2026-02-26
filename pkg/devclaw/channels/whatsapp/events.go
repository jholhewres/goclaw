// Package whatsapp – events.go processes incoming WhatsApp events from
// whatsmeow and converts them into unified DevClaw IncomingMessage types.
package whatsapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

// ConnectionState represents the current connection state.
type ConnectionState string

const (
	StateDisconnected  ConnectionState = "disconnected"
	StateConnecting    ConnectionState = "connecting"
	StateConnected     ConnectionState = "connected"
	StateReconnecting  ConnectionState = "reconnecting"
	StateWaitingQR     ConnectionState = "waiting_qr"
	StateQRScanned     ConnectionState = "qr_scanned"
	StateLoggingOut    ConnectionState = "logging_out"
	StateBanned        ConnectionState = "banned"
)

// ConnectionEvent represents a connection state change event.
type ConnectionEvent struct {
	State     ConnectionState `json:"state"`
	Previous  ConnectionState `json:"previous,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Reason    string          `json:"reason,omitempty"`
	Details   map[string]any  `json:"details,omitempty"`
}

// QREventEnhanced represents an enhanced QR code event with more details.
type QREventEnhanced struct {
	Type        string    `json:"type"`                    // "code", "success", "timeout", "error", "refresh"
	Code        string    `json:"code,omitempty"`          // Raw QR code string
	Message     string    `json:"message"`                 // Human-readable message
	ExpiresAt   time.Time `json:"expires_at,omitempty"`    // When QR code expires
	SecondsLeft int       `json:"seconds_left,omitempty"`  // Seconds until expiration
	Attempts    int       `json:"attempts,omitempty"`      // Number of QR attempts
}

// ConnectionObserver receives connection state changes.
type ConnectionObserver interface {
	OnConnectionChange(evt ConnectionEvent)
}

// handleEvent is the main whatsmeow event dispatcher.
func (w *WhatsApp) handleEvent(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		w.handleMessageEvt(evt)

	case *events.Receipt:
		w.handleReceipt(evt)

	case *events.Connected:
		w.handleConnected(evt)

	case *events.Disconnected:
		w.handleDisconnected(evt)

	case *events.StreamReplaced:
		w.handleStreamReplaced(evt)

	case *events.LoggedOut:
		w.handleLoggedOut(evt)

	case *events.TemporaryBan:
		w.handleTemporaryBan(evt)

	case *events.KeepAliveTimeout:
		w.handleKeepAliveTimeout(evt)

	case *events.KeepAliveRestored:
		w.handleKeepAliveRestored(evt)

	case *events.ConnectFailure:
		w.handleConnectFailure(evt)

	case *events.StreamError:
		w.handleStreamError(evt)

	case *events.HistorySync:
		w.logger.Debug("whatsapp: history sync received")

	case *events.PushName:
		w.logger.Debug("whatsapp: push name update",
			"jid", evt.JID, "name", evt.NewPushName)

	case *events.PairSuccess:
		w.handlePairSuccess(evt)

	case *events.QRScannedWithoutMultidevice:
		w.logger.Warn("whatsapp: QR scanned but multidevice not enabled")
	}
}

// handleConnected handles successful connection.
func (w *WhatsApp) handleConnected(_ *events.Connected) {
	previous := w.getState()
	w.setState(StateConnected)
	w.errorCount.Store(0)
	w.reconnectAttempts.Store(0)

	// Update last activity time for health monitoring.
	w.UpdateLastMsgTime()

	w.logger.Info("whatsapp: connected",
		"jid", w.getClientJID(),
		"platform", w.getClientPlatform())

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateConnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Details: map[string]any{
			"jid":      w.getClientJID(),
			"platform": w.getClientPlatform(),
		},
	})

	// Clear any QR state.
	w.notifyQR(QREvent{
		Type:    "success",
		Message: "WhatsApp connected successfully!",
	})
}

// handleDisconnected handles disconnection.
func (w *WhatsApp) handleDisconnected(_ *events.Disconnected) {
	previous := w.getState()
	w.setState(StateDisconnected)

	w.logger.Warn("whatsapp: disconnected",
		"was_connected", w.connected.Load())

	w.connected.Store(false)

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "connection_lost",
	})

	// Attempt reconnection if not intentional.
	if previous == StateConnected && w.ctx.Err() == nil {
		go w.attemptReconnect()
	}
}

// handleStreamReplaced handles when another device takes over.
func (w *WhatsApp) handleStreamReplaced(_ *events.StreamReplaced) {
	previous := w.getState()
	w.setState(StateDisconnected)
	w.connected.Store(false)

	w.logger.Error("whatsapp: stream replaced - another device connected")

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "stream_replaced",
		Details: map[string]any{
			"message": "Another device has connected to this WhatsApp account",
		},
	})
}

// handleLoggedOut handles session invalidation.
func (w *WhatsApp) handleLoggedOut(evt *events.LoggedOut) {
	previous := w.getState()
	w.setState(StateDisconnected)
	w.connected.Store(false)

	reason := "unknown"
	if evt.Reason != 0 {
		reason = evt.Reason.String()
	}

	w.logger.Error("whatsapp: logged out",
		"reason", reason,
		"on_connect", evt.OnConnect)

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "logged_out",
		Details: map[string]any{
			"reason":      reason,
			"needs_qr":    true,
			"message":     "Session invalidated, please scan QR code again",
		},
	})

	// Request new QR code.
	w.lastQR = nil
	go func() {
		if err := w.loginWithQR(w.ctx); err != nil {
			w.logger.Warn("whatsapp: QR re-login failed", "error", err)
		}
	}()
}

// handleTemporaryBan handles temporary bans.
func (w *WhatsApp) handleTemporaryBan(evt *events.TemporaryBan) {
	previous := w.getState()
	w.setState(StateBanned)
	w.connected.Store(false)

	w.logger.Error("whatsapp: temporary ban",
		"code", evt.Code,
		"expire", evt.Expire)

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateBanned,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "temporary_ban",
		Details: map[string]any{
			"code":   evt.Code.String(),
			"expire": evt.Expire.String(),
			"message": fmt.Sprintf("WhatsApp temporary ban. Expires: %s", evt.Expire),
		},
	})
}

// handleKeepAliveTimeout handles keep-alive failures.
func (w *WhatsApp) handleKeepAliveTimeout(evt *events.KeepAliveTimeout) {
	w.logger.Warn("whatsapp: keep-alive timeout",
		"error_count", evt.ErrorCount,
		"last_success", evt.LastSuccess)

	// Increase error count for health monitoring.
	w.errorCount.Add(1)

	// Trigger reconnection if keepalive fails consistently (3+ errors).
	// This handles "half-open" connections where the socket appears
	// connected but is actually dead.
	if evt.ErrorCount >= 3 && w.getState() == StateConnected {
		w.logger.Error("whatsapp: keep-alive failed multiple times, forcing reconnection",
			"error_count", evt.ErrorCount)
		w.setState(StateReconnecting)
		w.connected.Store(false)
		go w.attemptReconnect()
	}
}

// handleKeepAliveRestored handles keep-alive recovery.
func (w *WhatsApp) handleKeepAliveRestored(_ *events.KeepAliveRestored) {
	w.logger.Info("whatsapp: keep-alive restored")
	w.errorCount.Store(0)
}

// handleConnectFailure handles connection failure events from WhatsApp server.
func (w *WhatsApp) handleConnectFailure(evt *events.ConnectFailure) {
	previous := w.getState()
	w.setState(StateDisconnected)
	w.connected.Store(false)

	reason := "unknown"
	if evt.Reason != 0 {
		reason = evt.Reason.String()
	}

	permanent := evt.PermanentDisconnectDescription()

	w.logger.Error("whatsapp: connect failure",
		"reason", reason,
		"message", evt.Message,
		"permanent", permanent)

	// Notify connection observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "connect_failure",
		Details: map[string]any{
			"reason":    reason,
			"message":   evt.Message,
			"permanent": permanent,
		},
	})

	// Only attempt reconnect if not permanent and context is valid.
	if permanent == "" && w.ctx.Err() == nil {
		go w.attemptReconnect()
	}
}

// handleStreamError handles stream error events from WhatsApp server.
func (w *WhatsApp) handleStreamError(evt *events.StreamError) {
	previous := w.getState()

	w.logger.Error("whatsapp: stream error",
		"code", evt.Code)

	// Stream errors often indicate connection issues.
	// Check if this is a disconnect-type error.
	isDisconnect := evt.Code == "540" || evt.Code == "541" || evt.Code == "503"

	if isDisconnect {
		w.setState(StateDisconnected)
		w.connected.Store(false)

		// Notify connection observers.
		w.notifyConnectionChange(ConnectionEvent{
			State:     StateDisconnected,
			Previous:  previous,
			Timestamp: time.Now(),
			Reason:    "stream_error",
			Details: map[string]any{
				"code":   evt.Code,
				"message": "Stream error, connection lost",
			},
		})

		// Attempt reconnect if context is valid.
		if w.ctx.Err() == nil {
			go w.attemptReconnect()
		}
	} else {
		// Non-disconnect stream error - just log and notify.
		w.notifyConnectionChange(ConnectionEvent{
			State:     previous,
			Previous:  previous,
			Timestamp: time.Now(),
			Reason:    "stream_error_non_fatal",
			Details: map[string]any{
				"code":    evt.Code,
				"message": "Non-fatal stream error",
			},
		})
	}
}

// handlePairSuccess handles successful device pairing.
func (w *WhatsApp) handlePairSuccess(evt *events.PairSuccess) {
	w.logger.Info("whatsapp: device paired",
		"jid", evt.ID,
		"platform", evt.Platform,
		"business", evt.BusinessName)

	// Notify QR observers of success.
	w.notifyQR(QREvent{
		Type:    "success",
		Message: fmt.Sprintf("Paired with %s successfully!", evt.ID.String()),
	})
}

// handleMessageEvt processes an incoming WhatsApp message event.
func (w *WhatsApp) handleMessageEvt(evt *events.Message) {
	// Update last activity time for health monitoring.
	w.UpdateLastMsgTime()

	// Skip messages from self.
	if evt.Info.IsFromMe {
		return
	}

	// Skip status broadcasts.
	if evt.Info.Chat.Server == "broadcast" {
		return
	}

	// Check group/DM filtering.
	isGroup := evt.Info.IsGroup
	if isGroup && !w.cfg.RespondToGroups {
		return
	}
	if !isGroup && !w.cfg.RespondToDMs {
		return
	}

	// Resolve sender JID — WhatsApp may use LID (Linked Identity) format
	// instead of phone numbers. We resolve to phone JID for access control.
	senderJID := evt.Info.Sender
	resolvedSender := senderJID.String()
	if senderJID.Server == "lid" && w.client != nil && w.client.Store != nil {
		if altJID, err := w.client.Store.GetAltJID(w.ctx, senderJID); err == nil && !altJID.IsEmpty() {
			resolvedSender = altJID.String()
			w.logger.Debug("whatsapp: resolved LID to phone",
				"lid", senderJID.String(), "phone", resolvedSender)
		}
	}

	// Resolve chat JID (for DMs, chat may also be in LID format).
	chatJID := evt.Info.Chat
	resolvedChat := chatJID.String()
	if chatJID.Server == "lid" && w.client != nil && w.client.Store != nil {
		if altJID, err := w.client.Store.GetAltJID(w.ctx, chatJID); err == nil && !altJID.IsEmpty() {
			resolvedChat = altJID.String()
		}
	}

	// Build the incoming message.
	msg := &channels.IncomingMessage{
		ID:        string(evt.Info.ID),
		Channel:   "whatsapp",
		From:      resolvedSender,
		FromName:  evt.Info.PushName,
		ChatID:    resolvedChat,
		IsGroup:   isGroup,
		Timestamp: evt.Info.Timestamp,
		Metadata: map[string]any{
			"sender_jid":   senderJID.String(),
			"sender_lid":   senderJID.String(),
			"sender_phone": resolvedSender,
			"chat_jid":     chatJID.String(),
			"push_name":    evt.Info.PushName,
		},
	}

	// Extract message content by type.
	w.extractMessageContent(evt.Message, msg)

	// Handle quoted/reply messages.
	w.extractQuotedMessage(evt.Message, msg)

	// Auto-read if configured.
	if w.cfg.AutoRead {
		go func() {
			_ = w.MarkRead(w.ctx, msg.ChatID, []string{msg.ID})
		}()
	}

	// Emit the message.
	w.emitMessage(msg)
}

// extractMessageContent extracts the text/media content from a WhatsApp message.
func (w *WhatsApp) extractMessageContent(waMsg *waE2E.Message, msg *channels.IncomingMessage) {
	if waMsg == nil {
		return
	}

	// Text message (simple conversation).
	if waMsg.Conversation != nil {
		msg.Type = channels.MessageText
		msg.Content = waMsg.GetConversation()
		return
	}

	// Extended text message (with preview, formatting, etc.).
	if ext := waMsg.ExtendedTextMessage; ext != nil {
		msg.Type = channels.MessageText
		msg.Content = ext.GetText()
		return
	}

	// Image message.
	if img := waMsg.ImageMessage; img != nil {
		msg.Type = channels.MessageImage
		msg.Content = img.GetCaption()
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageImage,
			MimeType:      img.GetMimetype(),
			FileSize:      img.GetFileLength(),
			Caption:       img.GetCaption(),
			Width:         img.GetWidth(),
			Height:        img.GetHeight(),
			URL:           img.GetURL(),
			DirectPath:    img.GetDirectPath(),
			MediaKey:      img.GetMediaKey(),
			FileSHA256:    img.GetFileSHA256(),
			FileEncSHA256: img.GetFileEncSHA256(),
		}
		return
	}

	// Audio message (voice note or audio file).
	if audio := waMsg.AudioMessage; audio != nil {
		msg.Type = channels.MessageAudio
		msg.Content = "[audio]"
		if audio.GetPTT() {
			msg.Content = "[voice note]"
		}
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageAudio,
			MimeType:      audio.GetMimetype(),
			FileSize:      audio.GetFileLength(),
			Duration:      audio.GetSeconds(),
			URL:           audio.GetURL(),
			DirectPath:    audio.GetDirectPath(),
			MediaKey:      audio.GetMediaKey(),
			FileSHA256:    audio.GetFileSHA256(),
			FileEncSHA256: audio.GetFileEncSHA256(),
		}
		return
	}

	// Video message.
	if video := waMsg.VideoMessage; video != nil {
		msg.Type = channels.MessageVideo
		msg.Content = video.GetCaption()
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageVideo,
			MimeType:      video.GetMimetype(),
			FileSize:      video.GetFileLength(),
			Caption:       video.GetCaption(),
			Duration:      video.GetSeconds(),
			Width:         video.GetWidth(),
			Height:        video.GetHeight(),
			URL:           video.GetURL(),
			DirectPath:    video.GetDirectPath(),
			MediaKey:      video.GetMediaKey(),
			FileSHA256:    video.GetFileSHA256(),
			FileEncSHA256: video.GetFileEncSHA256(),
		}
		return
	}

	// Document message.
	if doc := waMsg.DocumentMessage; doc != nil {
		msg.Type = channels.MessageDocument
		msg.Content = doc.GetCaption()
		if msg.Content == "" {
			msg.Content = fmt.Sprintf("[document: %s]", doc.GetFileName())
		}
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageDocument,
			MimeType:      doc.GetMimetype(),
			Filename:      doc.GetFileName(),
			FileSize:      doc.GetFileLength(),
			Caption:       doc.GetCaption(),
			URL:           doc.GetURL(),
			DirectPath:    doc.GetDirectPath(),
			MediaKey:      doc.GetMediaKey(),
			FileSHA256:    doc.GetFileSHA256(),
			FileEncSHA256: doc.GetFileEncSHA256(),
		}
		return
	}

	// Sticker message.
	if sticker := waMsg.StickerMessage; sticker != nil {
		msg.Type = channels.MessageSticker
		msg.Content = "[sticker]"
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageSticker,
			MimeType:      sticker.GetMimetype(),
			FileSize:      sticker.GetFileLength(),
			Width:         sticker.GetWidth(),
			Height:        sticker.GetHeight(),
			URL:           sticker.GetURL(),
			DirectPath:    sticker.GetDirectPath(),
			MediaKey:      sticker.GetMediaKey(),
			FileSHA256:    sticker.GetFileSHA256(),
			FileEncSHA256: sticker.GetFileEncSHA256(),
		}
		return
	}

	// Location message.
	if loc := waMsg.LocationMessage; loc != nil {
		msg.Type = channels.MessageLocation
		msg.Content = fmt.Sprintf("[location: %.6f, %.6f]",
			loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		msg.Location = &channels.LocationInfo{
			Latitude:  loc.GetDegreesLatitude(),
			Longitude: loc.GetDegreesLongitude(),
			Name:      loc.GetName(),
			Address:   loc.GetAddress(),
			URL:       loc.GetURL(),
			AccuracyM: loc.GetAccuracyInMeters(),
		}
		return
	}

	// Live location.
	if loc := waMsg.LiveLocationMessage; loc != nil {
		msg.Type = channels.MessageLocation
		msg.Content = fmt.Sprintf("[live location: %.6f, %.6f]",
			loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		msg.Location = &channels.LocationInfo{
			Latitude:  loc.GetDegreesLatitude(),
			Longitude: loc.GetDegreesLongitude(),
			AccuracyM: loc.GetAccuracyInMeters(),
		}
		return
	}

	// Contact message.
	if contact := waMsg.ContactMessage; contact != nil {
		msg.Type = channels.MessageContact
		msg.Content = fmt.Sprintf("[contact: %s]", contact.GetDisplayName())
		msg.Contact = &channels.ContactInfo{
			DisplayName: contact.GetDisplayName(),
			VCard:       contact.GetVcard(),
		}
		return
	}

	// Reaction message.
	if reaction := waMsg.ReactionMessage; reaction != nil {
		msg.Type = channels.MessageReaction
		msg.Content = reaction.GetText()
		msg.Reaction = &channels.ReactionInfo{
			Emoji:     reaction.GetText(),
			MessageID: reaction.GetKey().GetID(),
			Remove:    reaction.GetText() == "",
		}
		return
	}

	// Fallback: unknown message type.
	msg.Type = channels.MessageText
	msg.Content = "[unsupported message type]"
}

// extractQuotedMessage extracts reply/quoted context from a message.
func (w *WhatsApp) extractQuotedMessage(waMsg *waE2E.Message, msg *channels.IncomingMessage) {
	if waMsg == nil {
		return
	}

	// Collect context info from any message type that supports quoting.
	var ctxInfo *waE2E.ContextInfo

	switch {
	case waMsg.ExtendedTextMessage != nil:
		ctxInfo = waMsg.ExtendedTextMessage.GetContextInfo()
	case waMsg.ImageMessage != nil:
		ctxInfo = waMsg.ImageMessage.GetContextInfo()
	case waMsg.AudioMessage != nil:
		ctxInfo = waMsg.AudioMessage.GetContextInfo()
	case waMsg.VideoMessage != nil:
		ctxInfo = waMsg.VideoMessage.GetContextInfo()
	case waMsg.DocumentMessage != nil:
		ctxInfo = waMsg.DocumentMessage.GetContextInfo()
	case waMsg.StickerMessage != nil:
		ctxInfo = waMsg.StickerMessage.GetContextInfo()
	}

	if ctxInfo == nil {
		return
	}

	if ctxInfo.StanzaID != nil {
		msg.ReplyTo = ctxInfo.GetStanzaID()
	}
	if quoted := ctxInfo.QuotedMessage; quoted != nil {
		msg.QuotedContent = extractQuotedText(quoted)
	}
}

// extractQuotedText gets the text from a quoted message.
func extractQuotedText(quoted *waE2E.Message) string {
	if quoted.Conversation != nil {
		return quoted.GetConversation()
	}
	if ext := quoted.ExtendedTextMessage; ext != nil {
		return ext.GetText()
	}
	if img := quoted.ImageMessage; img != nil {
		return "[image] " + img.GetCaption()
	}
	if vid := quoted.VideoMessage; vid != nil {
		return "[video] " + vid.GetCaption()
	}
	if doc := quoted.DocumentMessage; doc != nil {
		return "[document: " + doc.GetFileName() + "]"
	}
	if audio := quoted.AudioMessage; audio != nil {
		if audio.GetPTT() {
			return "[voice note]"
		}
		return "[audio]"
	}
	return "[message]"
}

// handleReceipt processes read/delivery receipts.
func (w *WhatsApp) handleReceipt(evt *events.Receipt) {
	switch evt.Type {
	case types.ReceiptTypeRead:
		w.logger.Debug("whatsapp: message read",
			"from", evt.Chat, "ids", evt.MessageIDs)
	case types.ReceiptTypeDelivered:
		w.logger.Debug("whatsapp: message delivered",
			"from", evt.Chat, "ids", evt.MessageIDs)
	}
}

// ---------- Helpers ----------

// parseJID converts a string JID to types.JID.
// Accepts formats: "5511999999999" or "5511999999999@s.whatsapp.net"
// or group IDs like "123456789-1234@g.us".
func parseJID(s string) (types.JID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return types.JID{}, fmt.Errorf("empty JID")
	}

	// Already a full JID with server.
	if strings.Contains(s, "@") {
		return types.ParseJID(s)
	}

	// Bare phone number — add @s.whatsapp.net.
	// Remove any non-digit characters.
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)

	if len(digits) < 10 {
		return types.JID{}, fmt.Errorf("phone number too short: %s", s)
	}

	return types.NewJID(digits, types.DefaultUserServer), nil
}
