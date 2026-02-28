// Package copilot – agent_router.go implements agent routing based on
// channel, user, or group. This allows different agents with different
// models, instructions, and skill sets to handle messages from different sources.
package copilot

import (
	"log/slog"
	"slices"
	"strings"
)

// AgentProfileConfig defines a specialized agent configuration.
type AgentProfileConfig struct {
	// ID is the unique identifier for this agent profile.
	ID string `yaml:"id"`

	// Model is the LLM model to use (e.g., "gpt-4o", "claude-sonnet-4").
	Model string `yaml:"model"`

	// Instructions override the base system prompt for this agent.
	Instructions string `yaml:"instructions"`

	// Skills are the skill names to enable for this agent.
	Skills []string `yaml:"skills"`

	// Channels route messages from these channels to this agent.
	Channels []string `yaml:"channels"`

	// Users route messages from these users to this agent.
	Users []string `yaml:"users"`

	// Groups route messages from these groups to this agent.
	Groups []string `yaml:"groups"`

	// MaxTurns is the max LLM turns for this agent (0 = unlimited).
	MaxTurns int `yaml:"max_turns"`

	// RunTimeoutSeconds is the max run time for this agent.
	RunTimeoutSeconds int `yaml:"run_timeout_seconds"`

	// Identity overrides the global identity for this agent profile.
	Identity *IdentityConfig `yaml:"identity,omitempty"`
}

// RoutingConfig defines how messages are routed to agents.
type RoutingConfig struct {
	// Default is the default agent ID to use when no match is found.
	Default string `yaml:"default"`
}

// AgentsConfig holds all agent profiles and routing configuration.
type AgentsConfig struct {
	// Profiles is the list of agent profiles.
	Profiles []AgentProfileConfig `yaml:"profiles"`

	// Routing defines how messages are routed.
	Routing RoutingConfig `yaml:"routing"`
}

// AgentRouter routes messages to the appropriate agent profile.
type AgentRouter struct {
	profiles   map[string]*AgentProfileConfig
	byChannel  map[string]string // channel -> profile ID
	byUser     map[string]string // user JID -> profile ID
	byGroup    map[string]string // group JID -> profile ID
	defaultID  string
	logger     *slog.Logger
}

// NewAgentRouter creates a new agent router from configuration.
func NewAgentRouter(cfg AgentsConfig, logger *slog.Logger) *AgentRouter {
	r := &AgentRouter{
		profiles:  make(map[string]*AgentProfileConfig),
		byChannel: make(map[string]string),
		byUser:    make(map[string]string),
		byGroup:   make(map[string]string),
		defaultID: cfg.Routing.Default,
		logger:    logger,
	}

	// Index profiles by ID and build routing maps.
	for i := range cfg.Profiles {
		p := &cfg.Profiles[i]
		r.profiles[p.ID] = p

		// Channel routing.
		for _, ch := range p.Channels {
			r.byChannel[strings.ToLower(ch)] = p.ID
		}

		// User routing.
		for _, u := range p.Users {
			r.byUser[normalizeJID(u)] = p.ID
		}

		// Group routing.
		for _, g := range p.Groups {
			r.byGroup[normalizeJID(g)] = p.ID
		}
	}

	logger.Info("agent router initialized",
		"profiles", len(r.profiles),
		"channels", len(r.byChannel),
		"users", len(r.byUser),
		"groups", len(r.byGroup),
		"default", r.defaultID,
	)

	return r
}

// Route determines which agent profile should handle a message.
// Priority: user > group > channel > default.
func (r *AgentRouter) Route(channel string, userJID string, groupJID string) *AgentProfileConfig {
	// 1. Check user routing (highest priority).
	if userJID != "" {
		if profileID, ok := r.byUser[normalizeJID(userJID)]; ok {
			r.logger.Debug("routed by user", "user", userJID, "profile", profileID)
			return r.profiles[profileID]
		}
	}

	// 2. Check group routing.
	if groupJID != "" {
		if profileID, ok := r.byGroup[normalizeJID(groupJID)]; ok {
			r.logger.Debug("routed by group", "group", groupJID, "profile", profileID)
			return r.profiles[profileID]
		}
	}

	// 3. Check channel routing.
	if channel != "" {
		chLower := strings.ToLower(channel)
		if profileID, ok := r.byChannel[chLower]; ok {
			r.logger.Debug("routed by channel", "channel", channel, "profile", profileID)
			return r.profiles[profileID]
		}
	}

	// 4. Return default profile.
	if r.defaultID != "" {
		if profile, ok := r.profiles[r.defaultID]; ok {
			r.logger.Debug("routed to default", "profile", r.defaultID)
			return profile
		}
	}

	// No routing configured.
	return nil
}

// GetProfile returns a profile by ID.
func (r *AgentRouter) GetProfile(id string) *AgentProfileConfig {
	return r.profiles[id]
}

// ListProfiles returns all profile IDs.
func (r *AgentRouter) ListProfiles() []string {
	ids := make([]string, 0, len(r.profiles))
	for id := range r.profiles {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// DefaultProfileID returns the default profile ID.
func (r *AgentRouter) DefaultProfileID() string {
	return r.defaultID
}

// MergeConfig merges agent profile settings with the base config.
// Returns the model, instructions, and skills to use.
func (p *AgentProfileConfig) MergeConfig(baseModel, baseInstructions string, baseSkills []string) (model, instructions string, skills []string) {
	// Model: use profile model or fall back to base.
	if p.Model != "" {
		model = p.Model
	} else {
		model = baseModel
	}

	// Instructions: profile instructions replace base instructions.
	if p.Instructions != "" {
		instructions = p.Instructions
	} else {
		instructions = baseInstructions
	}

	// Skills: profile skills replace base skills (not merged).
	if len(p.Skills) > 0 {
		skills = p.Skills
	} else {
		skills = baseSkills
	}

	return model, instructions, skills
}
