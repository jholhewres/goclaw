// Package channels – manager.go orchestrates multiple communication channels,
// providing a single entry point to receive messages from all platforms
// and route responses to the correct channel.
package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Manager orchestrates multiple communication channels, aggregating
// incoming messages into a single stream and routing responses.
type Manager struct {
	channels map[string]Channel
	messages chan *IncomingMessage
	logger   *slog.Logger
	listenWg sync.WaitGroup

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a new channel manager.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		channels: make(map[string]Channel),
		messages: make(chan *IncomingMessage, 256),
		logger:   logger,
	}
}

// Register adds a channel. Must be called before Start.
func (m *Manager) Register(ch Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := ch.Name()
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	m.channels[name] = ch
	m.logger.Info("channel registered", "channel", name)
	return nil
}

// Start connects all registered channels and begins listening for messages.
// Channels that fail to connect are logged but don't block others.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	m.mu.RLock()
	snapshot := make(map[string]Channel, len(m.channels))
	for k, v := range m.channels {
		snapshot[k] = v
	}
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		m.logger.Warn("no channels registered, running without messaging")
		return nil
	}

	var connected int
	for name, ch := range snapshot {
		if err := ch.Connect(m.ctx); err != nil {
			m.logger.Error("failed to connect channel",
				"channel", name, "error", err)
			continue
		}

		connected++
		m.logger.Info("channel connected", "channel", name)

		m.listenWg.Add(1)
		go func(c Channel) {
			defer m.listenWg.Done()
			m.listenChannel(c)
		}(ch)
	}

	if connected == 0 {
		return fmt.Errorf("no channel connected successfully")
	}

	m.logger.Info("manager started", "channels_connected", connected)
	return nil
}

// Stop gracefully disconnects all channels.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}

	// Disconnect channels first — this closes their Receive() channels,
	// which unblocks listenChannel goroutines.
	m.mu.RLock()
	for name, ch := range m.channels {
		if err := ch.Disconnect(); err != nil {
			m.logger.Error("error disconnecting channel",
				"channel", name, "error", err)
		}
	}
	m.mu.RUnlock()

	// Now wait for listener goroutines to finish.
	m.listenWg.Wait()

	close(m.messages)
	m.logger.Info("manager stopped")
}

// Messages returns the aggregated message stream from all channels.
func (m *Manager) Messages() <-chan *IncomingMessage {
	return m.messages
}

// Send sends a message through the specified channel.
func (m *Manager) Send(ctx context.Context, channelName, to string, msg *OutgoingMessage) error {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %q not found", channelName)
	}

	if !ch.IsConnected() {
		return fmt.Errorf("channel %q disconnected", channelName)
	}

	return ch.Send(ctx, to, msg)
}

// SendMedia sends a media message through the specified channel.
// Returns ErrMediaNotSupported if the channel doesn't support media.
func (m *Manager) SendMedia(ctx context.Context, channelName, to string, media *MediaMessage) error {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %q not found", channelName)
	}

	mc, ok := ch.(MediaChannel)
	if !ok {
		return ErrMediaNotSupported
	}

	return mc.SendMedia(ctx, to, media)
}

// SendTyping sends a typing indicator on the specified channel.
// Silently does nothing if the channel doesn't support presence.
func (m *Manager) SendTyping(ctx context.Context, channelName, to string) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if pc, ok := ch.(PresenceChannel); ok {
		_ = pc.SendTyping(ctx, to)
	}
}

// MarkRead marks messages as read on the specified channel.
// Silently does nothing if the channel doesn't support presence.
func (m *Manager) MarkRead(ctx context.Context, channelName, chatID string, messageIDs []string) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if pc, ok := ch.(PresenceChannel); ok {
		_ = pc.MarkRead(ctx, chatID, messageIDs)
	}
}

// Channel returns a specific channel by name.
func (m *Manager) Channel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// HealthAll returns health status for all registered channels.
func (m *Manager) HealthAll() map[string]HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]HealthStatus, len(m.channels))
	for name, ch := range m.channels {
		statuses[name] = ch.Health()
	}
	return statuses
}

// HasChannels returns true if at least one channel is registered.
func (m *Manager) HasChannels() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels) > 0
}

// listenChannel listens for messages from a channel and forwards them
// to the aggregated stream. Exits when the channel closes or context is cancelled.
func (m *Manager) listenChannel(ch Channel) {
	incoming := ch.Receive()
	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				return // Channel closed.
			}
			select {
			case m.messages <- msg:
			case <-m.ctx.Done():
				return
			}
		case <-m.ctx.Done():
			return
		}
	}
}
