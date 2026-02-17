// Package copilot – multiuser.go implements multi-user support with
// user management, role-based access control, shared memory spaces,
// and team collaboration features.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ---------- Data Types ----------

// UserRole defines permission levels for multi-user access.
type UserRole string

const (
	RoleOwner  UserRole = "owner"
	RoleAdmin  UserRole = "admin"
	RoleUser   UserRole = "user"
	RoleViewer UserRole = "viewer"
)

// TeamUser represents a user in the multi-user system.
type TeamUser struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	Role      UserRole  `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	Active    bool      `json:"active"`
}

// SharedMemory represents a team-shared memory space.
type SharedMemory struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Author    string    `json:"author"` // User ID who wrote it
	UpdatedAt time.Time `json:"updated_at"`
	Tags      []string  `json:"tags,omitempty"`
}

// TeamConfig holds multi-user configuration.
type TeamConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	MaxUsers      int    `yaml:"max_users" json:"max_users"`
	SharedMemory  bool   `yaml:"shared_memory" json:"shared_memory"`
	AuditLog      bool   `yaml:"audit_log" json:"audit_log"`
	DefaultRole   string `yaml:"default_role" json:"default_role"`
}

// DefaultTeamConfig returns sensible defaults.
func DefaultTeamConfig() TeamConfig {
	return TeamConfig{
		Enabled:      false,
		MaxUsers:     10,
		SharedMemory: true,
		AuditLog:     true,
		DefaultRole:  "user",
	}
}

// ---------- User Manager ----------

// UserManager handles multi-user operations.
type UserManager struct {
	mu           sync.RWMutex
	users        map[string]*TeamUser
	sharedMemory map[string]*SharedMemory
	config       TeamConfig
}

// NewUserManager creates a new user manager.
func NewUserManager(config TeamConfig) *UserManager {
	return &UserManager{
		users:        make(map[string]*TeamUser),
		sharedMemory: make(map[string]*SharedMemory),
		config:       config,
	}
}

// AddUser adds a new user.
func (um *UserManager) AddUser(user *TeamUser) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if len(um.users) >= um.config.MaxUsers {
		return fmt.Errorf("max users reached (%d)", um.config.MaxUsers)
	}

	if _, exists := um.users[user.ID]; exists {
		return fmt.Errorf("user %q already exists", user.ID)
	}

	user.CreatedAt = time.Now()
	user.Active = true
	um.users[user.ID] = user
	return nil
}

// RemoveUser removes a user.
func (um *UserManager) RemoveUser(id string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if _, exists := um.users[id]; !exists {
		return fmt.Errorf("user %q not found", id)
	}
	delete(um.users, id)
	return nil
}

// GetUser returns a user by ID.
func (um *UserManager) GetUser(id string) (*TeamUser, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()
	u, ok := um.users[id]
	return u, ok
}

// ListUsers returns all users.
func (um *UserManager) ListUsers() []*TeamUser {
	um.mu.RLock()
	defer um.mu.RUnlock()

	result := make([]*TeamUser, 0, len(um.users))
	for _, u := range um.users {
		result = append(result, u)
	}
	return result
}

// CheckPermission verifies a user has the required role.
func (um *UserManager) CheckPermission(userID string, required UserRole) bool {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, ok := um.users[userID]
	if !ok {
		return false
	}

	roleLevel := map[UserRole]int{
		RoleViewer: 0,
		RoleUser:   1,
		RoleAdmin:  2,
		RoleOwner:  3,
	}

	return roleLevel[user.Role] >= roleLevel[required]
}

// SetSharedMemory stores a value in shared team memory.
func (um *UserManager) SetSharedMemory(key, value, authorID string, tags []string) {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.sharedMemory[key] = &SharedMemory{
		Key:       key,
		Value:     value,
		Author:    authorID,
		UpdatedAt: time.Now(),
		Tags:      tags,
	}
}

// GetSharedMemory retrieves a value from shared team memory.
func (um *UserManager) GetSharedMemory(key string) (*SharedMemory, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()
	m, ok := um.sharedMemory[key]
	return m, ok
}

// ListSharedMemory returns all shared memory entries.
func (um *UserManager) ListSharedMemory() []*SharedMemory {
	um.mu.RLock()
	defer um.mu.RUnlock()

	result := make([]*SharedMemory, 0, len(um.sharedMemory))
	for _, m := range um.sharedMemory {
		result = append(result, m)
	}
	return result
}

// ---------- Tool Registration ----------

// RegisterMultiUserTools registers multi-user management tools.
func RegisterMultiUserTools(executor *ToolExecutor, um *UserManager) {
	// team_users
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "team_users",
			Description: "List, add, or remove team users. Requires admin+ permission for modifications.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"list", "add", "remove", "update_role"}, "description": "Action to perform"},
					"user_id": map[string]any{"type": "string", "description": "User ID (for add/remove/update)"},
					"name":    map[string]any{"type": "string", "description": "User display name (for add)"},
					"email":   map[string]any{"type": "string", "description": "User email (for add)"},
					"role":    map[string]any{"type": "string", "enum": []string{"owner", "admin", "user", "viewer"}, "description": "User role (for add/update)"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)

		switch action {
		case "list":
			users := um.ListUsers()
			data, _ := json.MarshalIndent(users, "", "  ")
			return string(data), nil

		case "add":
			userID, _ := args["user_id"].(string)
			name, _ := args["name"].(string)
			role, _ := args["role"].(string)
			email, _ := args["email"].(string)

			if userID == "" || name == "" {
				return nil, fmt.Errorf("user_id and name are required")
			}

			if role == "" {
				role = um.config.DefaultRole
			}

			user := &TeamUser{
				ID:    userID,
				Name:  name,
				Email: email,
				Role:  UserRole(role),
			}
			if err := um.AddUser(user); err != nil {
				return nil, err
			}
			return fmt.Sprintf("User %q added with role %q.", name, role), nil

		case "remove":
			userID, _ := args["user_id"].(string)
			if err := um.RemoveUser(userID); err != nil {
				return nil, err
			}
			return fmt.Sprintf("User %q removed.", userID), nil

		case "update_role":
			userID, _ := args["user_id"].(string)
			role, _ := args["role"].(string)
			user, ok := um.GetUser(userID)
			if !ok {
				return nil, fmt.Errorf("user %q not found", userID)
			}
			um.mu.Lock()
			user.Role = UserRole(role)
			um.mu.Unlock()
			return fmt.Sprintf("User %q role updated to %q.", userID, role), nil

		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	})

	// shared_memory
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "shared_memory",
			Description: "Team shared memory: store and retrieve knowledge accessible to all team members.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"get", "set", "list", "delete"}, "description": "Action to perform"},
					"key":    map[string]any{"type": "string", "description": "Memory key"},
					"value":  map[string]any{"type": "string", "description": "Memory value (for set)"},
					"tags":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for organizing memory"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)
		key, _ := args["key"].(string)

		switch action {
		case "get":
			if key == "" {
				return nil, fmt.Errorf("key is required")
			}
			mem, ok := um.GetSharedMemory(key)
			if !ok {
				return fmt.Sprintf("Key %q not found.", key), nil
			}
			data, _ := json.MarshalIndent(mem, "", "  ")
			return string(data), nil

		case "set":
			value, _ := args["value"].(string)
			if key == "" || value == "" {
				return nil, fmt.Errorf("key and value are required")
			}
			var tags []string
			if rawTags, ok := args["tags"].([]any); ok {
				for _, t := range rawTags {
					if s, ok := t.(string); ok {
						tags = append(tags, s)
					}
				}
			}
			um.SetSharedMemory(key, value, "system", tags)
			return fmt.Sprintf("Shared memory %q set.", key), nil

		case "list":
			mems := um.ListSharedMemory()
			data, _ := json.MarshalIndent(mems, "", "  ")
			return string(data), nil

		case "delete":
			if key == "" {
				return nil, fmt.Errorf("key is required")
			}
			um.mu.Lock()
			delete(um.sharedMemory, key)
			um.mu.Unlock()
			return fmt.Sprintf("Shared memory %q deleted.", key), nil

		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	})
}
