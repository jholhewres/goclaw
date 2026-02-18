// Package discord - components.go provides reusable interactive component
// handling for Discord (buttons, select menus) with registration, TTL-based
// cleanup, AllowedUsers restriction, and Reusable behavior.
// See ComponentRegistry, ComponentSpec, and BuildButtonRow/BuildSelectRow.
package discord

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ComponentKind identifies the type of component.
type ComponentKind string

const (
	ComponentKindButton ComponentKind = "button"
	ComponentKindSelect ComponentKind = "select"
)

// ComponentSpec defines the behavior of a registered interactive component.
// Register components before sending messages that include them.
type ComponentSpec struct {
	// Kind is the component type (button or select).
	Kind ComponentKind

	// Reusable, when true, means clicking does NOT consume/disable the component.
	// It can be clicked multiple times. Default is false.
	Reusable bool

	// AllowedUsers restricts who can interact. If nil or empty, anyone can use it.
	AllowedUsers []string

	// TTL is how long the component stays registered. After expiry it is removed
	// and interactions are ignored. Zero means no expiry.
	TTL time.Duration

	// Handler is called when an authorized user interacts. The handler may return
	// content to display in the interaction response. If the handler returns
	// an error, an error message is shown to the user.
	Handler ComponentHandler
}

// ComponentHandler processes a component interaction. It receives the interaction
// context and returns optional response content and an optional error.
// For select menus, evt.Values contains the selected option values.
type ComponentHandler func(ctx context.Context, evt *InteractionEvent) (content string, err error)

// InteractionEvent carries data from a component interaction.
type InteractionEvent struct {
	CustomID   string
	UserID     string
	Username   string
	ChannelID  string
	GuildID    string
	MessageID  string
	Values     []string // For select menus
	ComponentType discordgo.ComponentType
}

// registeredComponent wraps a spec with metadata for TTL and registry lookup.
type registeredComponent struct {
	Spec       ComponentSpec
	RegisteredAt time.Time
}

// ComponentRegistry stores component specs by custom_id and handles TTL cleanup.
type ComponentRegistry struct {
	mu       sync.RWMutex
	components map[string]*registeredComponent
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewComponentRegistry creates a registry and starts background TTL cleanup.
func NewComponentRegistry(logger *slog.Logger) *ComponentRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	r := &ComponentRegistry{
		components: make(map[string]*registeredComponent),
		logger:     logger.With("component", "discord_components"),
		stopCh:     make(chan struct{}),
	}
	go r.cleanupLoop()
	return r
}

// Register adds or overwrites a component spec for the given custom_id.
// Call this before sending a message that includes the component.
func (r *ComponentRegistry) Register(customID string, spec ComponentSpec) {
	if customID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.components[customID] = &registeredComponent{
		Spec:         spec,
		RegisteredAt: time.Now(),
	}
}

// Unregister removes a component spec.
func (r *ComponentRegistry) Unregister(customID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.components, customID)
}

// Get retrieves the component spec if it exists and is not expired.
func (r *ComponentRegistry) Get(customID string) (*ComponentSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.components[customID]
	if !ok || reg == nil {
		return nil, false
	}
	if reg.Spec.TTL > 0 && time.Since(reg.RegisteredAt) > reg.Spec.TTL {
		return nil, false
	}
	spec := reg.Spec
	return &spec, true
}

// IsAllowed checks if the user is in AllowedUsers. If AllowedUsers is nil/empty, returns true.
func (s *ComponentSpec) IsAllowed(userID string) bool {
	if len(s.AllowedUsers) == 0 {
		return true
	}
	for _, id := range s.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// cleanupLoop periodically removes expired components.
func (r *ComponentRegistry) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanupExpired()
		}
	}
}

// cleanupExpired removes components that have exceeded their TTL.
func (r *ComponentRegistry) cleanupExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var expired []string
	for id, reg := range r.components {
		if reg.Spec.TTL > 0 && now.Sub(reg.RegisteredAt) > reg.Spec.TTL {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(r.components, id)
	}
	if len(expired) > 0 {
		r.logger.Debug("discord: cleaned up expired components", "count", len(expired), "ids", expired)
	}
}

// Stop halts the cleanup loop. Call when shutting down the Discord channel.
func (r *ComponentRegistry) Stop() {
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
}

// BuildButtonRow creates a discordgo ActionsRow containing a single button.
// Use with Register(customID, spec) before sending the message.
func BuildButtonRow(customID, label string, style discordgo.ButtonStyle) discordgo.MessageComponent {
	if style == 0 {
		style = discordgo.PrimaryButton
	}
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: customID,
				Label:    label,
				Style:    style,
			},
		},
	}
}

// BuildSelectRow creates a discordgo ActionsRow containing a string select menu.
// Use with Register(customID, spec) before sending the message.
func BuildSelectRow(customID string, options []discordgo.SelectMenuOption, placeholder string) discordgo.MessageComponent {
	menu := discordgo.SelectMenu{
		CustomID:    customID,
		Options:     options,
		Placeholder: placeholder,
	}
	if len(menu.Options) > 25 {
		menu.Options = menu.Options[:25]
	}
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{&menu},
	}
}

// Copy returns a snapshot of all registered custom IDs (for tests/debug).
func (r *ComponentRegistry) Copy() map[string]ComponentSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ComponentSpec, len(r.components))
	for k, v := range r.components {
		if v != nil {
			out[k] = v.Spec
		}
	}
	return out
}
