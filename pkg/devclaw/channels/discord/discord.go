// Package discord implements the Discord channel for DevClaw using discordgo.
//
// Features:
//   - Send/receive text, images, audio, video, documents
//   - Typing indicators
//   - Reactions (emoji)
//   - Thread support (auto-thread mode)
//   - Embed messages for long responses
//   - Guild and channel allowlists
//   - Interactive components (buttons, select menus) with Reusable and AllowedUsers
//   - Automatic reconnection via discordgo's gateway
package discord

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// Config holds Discord channel configuration.
type Config struct {
	// Token is the Discord bot token.
	Token string `yaml:"token"`

	// AllowedGuilds restricts which guild (server) IDs the bot responds in.
	// Empty means respond in all guilds.
	AllowedGuilds []string `yaml:"allowed_guilds"`

	// AllowedChannels restricts which channel IDs the bot responds in.
	// Empty means respond in all channels.
	AllowedChannels []string `yaml:"allowed_channels"`

	// RespondToThreads enables responding inside threads.
	RespondToThreads bool `yaml:"respond_to_threads"`

	// AutoThread creates a new thread per conversation for cleanliness.
	AutoThread bool `yaml:"auto_thread"`

	// SendTyping sends "typing..." indicators while processing.
	SendTyping bool `yaml:"send_typing"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		RespondToThreads: true,
		SendTyping:       true,
	}
}

// Discord implements channels.Channel, channels.MediaChannel,
// channels.PresenceChannel, and channels.ReactionChannel.
type Discord struct {
	cfg     Config
	logger  *slog.Logger
	session *discordgo.Session

	// messages is the channel for incoming messages forwarded to the assistant.
	messages chan *channels.IncomingMessage

	// connected tracks connection state.
	connected atomic.Bool

	// lastMsg tracks the last message timestamp for health.
	lastMsg atomic.Value // time.Time

	// errorCount tracks consecutive errors.
	errorCount atomic.Int64

	// httpClient is used for downloading attachments.
	httpClient *http.Client

	// components manages interactive component registration and TTL cleanup.
	components *ComponentRegistry

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// New creates a new Discord channel instance.
func New(cfg Config, logger *slog.Logger) *Discord {
	if logger == nil {
		logger = slog.Default()
	}
	l := logger.With("component", "discord")
	return &Discord{
		cfg:        cfg,
		logger:     l,
		messages:   make(chan *channels.IncomingMessage, 256),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		components:  NewComponentRegistry(l),
	}
}

// ---------- Channel Interface ----------

// Name returns "discord".
func (d *Discord) Name() string { return "discord" }

// Connect opens the Discord gateway WebSocket connection.
func (d *Discord) Connect(ctx context.Context) error {
	if d.cfg.Token == "" {
		return fmt.Errorf("discord: bot token is required")
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	session, err := discordgo.New("Bot " + d.cfg.Token)
	if err != nil {
		return fmt.Errorf("discord: creating session: %w", err)
	}

	// Set intents.
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuildMessageReactions

	// Register handlers.
	session.AddHandler(d.onMessageCreate)
	session.AddHandler(d.onInteractionCreate)

	// Open the WebSocket connection.
	if err := session.Open(); err != nil {
		return fmt.Errorf("discord: opening gateway: %w", err)
	}

	d.session = session
	d.connected.Store(true)

	user := session.State.User
	d.logger.Info("discord: connected", "bot", user.Username+"#"+user.Discriminator, "id", user.ID)

	return nil
}

// Disconnect closes the Discord gateway connection.
func (d *Discord) Disconnect() error {
	if d.cancel != nil {
		d.cancel()
	}
	if d.components != nil {
		d.components.Stop()
	}
	if d.session != nil {
		d.session.Close()
	}
	d.connected.Store(false)
	d.logger.Info("discord: disconnected")
	return nil
}

// Send sends a text message to the specified channel.
func (d *Discord) Send(ctx context.Context, to string, message *channels.OutgoingMessage) error {
	if d.session == nil {
		return channels.ErrChannelDisconnected
	}

	content := message.Content

	// Discord has a 2000 character limit per message.
	if len(content) <= 2000 {
		msgSend := &discordgo.MessageSend{Content: content}
		if message.ReplyTo != "" {
			msgSend.Reference = &discordgo.MessageReference{MessageID: message.ReplyTo}
		}
		_, err := d.session.ChannelMessageSendComplex(to, msgSend)
		return err
	}

	// For long messages, split into chunks.
	chunks := splitDiscordMessage(content, 2000)
	for i, chunk := range chunks {
		msgSend := &discordgo.MessageSend{Content: chunk}
		if i == 0 && message.ReplyTo != "" {
			msgSend.Reference = &discordgo.MessageReference{MessageID: message.ReplyTo}
		}
		if _, err := d.session.ChannelMessageSendComplex(to, msgSend); err != nil {
			return err
		}
	}
	return nil
}

// Receive returns the incoming messages channel.
func (d *Discord) Receive() <-chan *channels.IncomingMessage {
	return d.messages
}

// IsConnected returns true if the bot is connected.
func (d *Discord) IsConnected() bool { return d.connected.Load() }

// Health returns the channel health status.
func (d *Discord) Health() channels.HealthStatus {
	var lastAt time.Time
	if v := d.lastMsg.Load(); v != nil {
		lastAt = v.(time.Time)
	}
	return channels.HealthStatus{
		Connected:     d.connected.Load(),
		LastMessageAt: lastAt,
		ErrorCount:    int(d.errorCount.Load()),
	}
}

// ---------- MediaChannel Interface ----------

// SendMedia sends a file/media attachment to the specified channel.
func (d *Discord) SendMedia(ctx context.Context, to string, media *channels.MediaMessage) error {
	if d.session == nil {
		return channels.ErrChannelDisconnected
	}

	filename := media.Filename
	if filename == "" {
		filename = "file"
	}

	var reader io.Reader
	if len(media.Data) > 0 {
		reader = bytes.NewReader(media.Data)
	} else if media.URL != "" {
		// Download from URL first.
		resp, err := d.httpClient.Get(media.URL)
		if err != nil {
			return fmt.Errorf("discord: download media: %w", err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("discord: reading media: %w", err)
		}
		reader = bytes.NewReader(data)
	} else {
		return fmt.Errorf("discord: no media data or URL")
	}

	msgSend := &discordgo.MessageSend{
		Files: []*discordgo.File{
			{Name: filename, Reader: reader},
		},
	}
	if media.Caption != "" {
		msgSend.Content = media.Caption
	}
	if media.ReplyTo != "" {
		msgSend.Reference = &discordgo.MessageReference{MessageID: media.ReplyTo}
	}

	_, err := d.session.ChannelMessageSendComplex(to, msgSend)
	return err
}

// DownloadMedia downloads an attachment from an incoming message.
func (d *Discord) DownloadMedia(ctx context.Context, msg *channels.IncomingMessage) ([]byte, string, error) {
	if msg.Media == nil || msg.Media.URL == "" {
		return nil, "", channels.ErrMediaDownloadFailed
	}

	resp, err := d.httpClient.Get(msg.Media.URL)
	if err != nil {
		return nil, "", fmt.Errorf("discord: download: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("discord: reading attachment: %w", err)
	}

	return data, msg.Media.MimeType, nil
}

// ---------- PresenceChannel Interface ----------

// SendTyping sends a typing indicator to the channel.
func (d *Discord) SendTyping(ctx context.Context, to string) error {
	if d.session == nil {
		return nil
	}
	return d.session.ChannelTyping(to)
}

// SendPresence updates the bot's status.
func (d *Discord) SendPresence(ctx context.Context, available bool) error {
	if d.session == nil {
		return nil
	}
	status := "online"
	if !available {
		status = "idle"
	}
	return d.session.UpdateStatusComplex(discordgo.UpdateStatusData{Status: status})
}

// MarkRead is a no-op for Discord (bots don't mark messages as read).
func (d *Discord) MarkRead(ctx context.Context, chatID string, messageIDs []string) error {
	return nil
}

// ---------- ReactionChannel Interface ----------

// SendReaction adds a reaction emoji to a message.
func (d *Discord) SendReaction(ctx context.Context, chatID, messageID, emoji string) error {
	if d.session == nil {
		return nil
	}
	return d.session.MessageReactionAdd(chatID, messageID, emoji)
}

// ---------- Event Handlers ----------

// onMessageCreate handles incoming Discord messages.
func (d *Discord) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Ignore bot messages.
	if m.Author.Bot {
		return
	}

	// Apply guild filter.
	if len(d.cfg.AllowedGuilds) > 0 && m.GuildID != "" {
		allowed := false
		for _, id := range d.cfg.AllowedGuilds {
			if id == m.GuildID {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	// Apply channel filter.
	if len(d.cfg.AllowedChannels) > 0 {
		allowed := false
		for _, id := range d.cfg.AllowedChannels {
			if id == m.ChannelID {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	// Determine if it's a group (guild) or DM.
	isGroup := m.GuildID != ""

	// Build the incoming message.
	incoming := &channels.IncomingMessage{
		ID:        m.ID,
		Channel:   "discord",
		From:      m.Author.ID,
		FromName:  m.Author.Username,
		ChatID:    m.ChannelID,
		IsGroup:   isGroup,
		Type:      channels.MessageText,
		Content:   m.Content,
		Timestamp: m.Timestamp,
	}

	// Handle replies.
	if m.ReferencedMessage != nil {
		incoming.ReplyTo = m.ReferencedMessage.ID
		incoming.QuotedContent = m.ReferencedMessage.Content
	}

	// Handle attachments.
	if len(m.Attachments) > 0 {
		att := m.Attachments[0] // Use first attachment.
		mediaType := inferMediaType(att.ContentType)
		incoming.Type = mediaType
		incoming.Media = &channels.MediaInfo{
			Type:     mediaType,
			URL:      att.URL,
			MimeType: att.ContentType,
			FileSize: uint64(att.Size),
			Filename: att.Filename,
			Width:    uint32(att.Width),
			Height:   uint32(att.Height),
		}
	}

	d.lastMsg.Store(time.Now())
	d.errorCount.Store(0)

	select {
	case d.messages <- incoming:
	default:
		d.logger.Warn("discord: message buffer full, dropping message", "msg_id", incoming.ID)
	}
}

// onInteractionCreate handles button clicks and select menu choices.
func (d *Discord) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()
	customID := data.CustomID
	if customID == "" {
		return
	}

	spec, ok := d.components.Get(customID)
	if !ok {
		respondEphemeral(s, i, "Unknown or expired component.")
		return
	}

	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}
	if userID == "" {
		respondEphemeral(s, i, "Could not identify user.")
		return
	}

	if !spec.IsAllowed(userID) {
		respondEphemeral(s, i, "You are not allowed to use this component.")
		return
	}

	evt := &InteractionEvent{
		CustomID:     customID,
		UserID:       userID,
		ChannelID:    i.ChannelID,
		GuildID:      i.GuildID,
		MessageID:    i.Message.ID,
		Values:       data.Values,
		ComponentType: data.ComponentType,
	}
	if i.Member != nil && i.Member.User != nil {
		evt.Username = i.Member.User.Username
	} else if i.User != nil {
		evt.Username = i.User.Username
	}

	// Acknowledge immediately to satisfy Discord's 3s limit.
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		d.logger.Warn("discord: failed to ack interaction", "custom_id", customID, "error", err)
		return
	}

	// Run handler then edit the message.
	go func() {
		ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
		defer cancel()

		content, err := spec.Handler(ctx, evt)
		if err != nil {
			content = "Error: " + err.Error()
			d.logger.Warn("discord: component handler error", "custom_id", customID, "error", err)
		}

		var components []discordgo.MessageComponent
		if spec.Reusable && i.Message != nil && len(i.Message.Components) > 0 {
			components = i.Message.Components
		}
		// For non-reusable, components stays nil/empty.

		edit := &discordgo.WebhookEdit{Content: &content}
		if len(components) > 0 {
			edit.Components = &components
		} else {
			empty := []discordgo.MessageComponent{}
			edit.Components = &empty
		}

		if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
			d.logger.Warn("discord: failed to edit interaction response", "custom_id", customID, "error", err)
		}
	}()
}

// respondEphemeral sends an ephemeral (visible only to the user) response.
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	flags := discordgo.MessageFlagsEphemeral
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})
}

// ---------- Interactive Components ----------

// ComponentRegistry returns the component registry for registering buttons and selects.
func (d *Discord) ComponentRegistry() *ComponentRegistry {
	return d.components
}

// SendWithComponents sends a message with interactive components (buttons, selects).
// Register components with d.ComponentRegistry().Register(customID, spec) before calling.
func (d *Discord) SendWithComponents(ctx context.Context, to string, content string, components []discordgo.MessageComponent) error {
	if d.session == nil {
		return channels.ErrChannelDisconnected
	}

	msgSend := &discordgo.MessageSend{
		Content:    content,
		Components:  components,
	}
	_, err := d.session.ChannelMessageSendComplex(to, msgSend)
	return err
}

// ---------- Helpers ----------

// inferMediaType maps MIME types to DevClaw message types.
func inferMediaType(contentType string) channels.MessageType {
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return channels.MessageImage
	case strings.HasPrefix(ct, "audio/"):
		return channels.MessageAudio
	case strings.HasPrefix(ct, "video/"):
		return channels.MessageVideo
	default:
		return channels.MessageDocument
	}
}

// splitDiscordMessage splits a message into chunks respecting the 2000 char limit.
func splitDiscordMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}
		// Try to split at a newline.
		cutAt := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
			cutAt = idx + 1
		}
		chunks = append(chunks, text[:cutAt])
		text = text[cutAt:]
	}
	return chunks
}

// Compile-time interface verification.
var (
	_ channels.Channel         = (*Discord)(nil)
	_ channels.MediaChannel    = (*Discord)(nil)
	_ channels.PresenceChannel = (*Discord)(nil)
	_ channels.ReactionChannel = (*Discord)(nil)
)
