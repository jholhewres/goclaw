// Package copilot - settings.go manages application settings stored separately from config.
// This includes tool profiles and other infrequently-changed settings.
package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Settings holds application-wide settings loaded from settings.yaml.
type Settings struct {
	// ToolProfiles contains custom tool profiles defined by the user.
	// Built-in profiles (minimal, coding, messaging, team, full) are always available.
	ToolProfiles map[string]ToolProfile `yaml:"tool_profiles"`
}

// SettingsManager manages loading and saving of settings.
type SettingsManager struct {
	path     string
	settings *Settings
	mu       sync.RWMutex
}

// NewSettingsManager creates a new settings manager.
func NewSettingsManager() *SettingsManager {
	return &SettingsManager{
		path: findSettingsFile(),
	}
}

// findSettingsFile searches for settings.yaml in standard locations.
func findSettingsFile() string {
	candidates := []string{
		"configs/settings.yaml",
		"settings.yaml",
		".devclaw/settings.yaml",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "configs/settings.yaml" // Default location
}

// Load reads settings from the YAML file.
func (sm *SettingsManager) Load() (*Settings, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.settings != nil {
		return sm.settings, nil
	}

	data, err := os.ReadFile(sm.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty settings if file doesn't exist
			sm.settings = &Settings{
				ToolProfiles: make(map[string]ToolProfile),
			}
			return sm.settings, nil
		}
		return nil, err
	}

	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	if settings.ToolProfiles == nil {
		settings.ToolProfiles = make(map[string]ToolProfile)
	}

	sm.settings = &settings
	return sm.settings, nil
}

// Save writes settings to the YAML file.
func (sm *SettingsManager) Save(settings *Settings) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(sm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}

	if err := os.WriteFile(sm.path, data, 0644); err != nil {
		return err
	}

	sm.settings = settings
	return nil
}

// GetAllProfiles returns built-in + custom profiles merged.
func (sm *SettingsManager) GetAllProfiles() map[string]ToolProfile {
	settings, _ := sm.Load()

	// Start with built-in profiles
	result := make(map[string]ToolProfile)
	for k, v := range BuiltInProfiles {
		result[k] = v
	}

	// Add custom profiles
	for k, v := range settings.ToolProfiles {
		result[k] = v
	}

	return result
}

// ToolProfileInfo contains profile info for API responses.
type ToolProfileInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
	Builtin     bool     `json:"builtin"`
}

// ListProfilesInfo returns all profiles with info about built-in status.
func (sm *SettingsManager) ListProfilesInfo() []ToolProfileInfo {
	settings, _ := sm.Load()

	// Collect all profile names
	names := make(map[string]bool)
	for name := range BuiltInProfiles {
		names[name] = true
	}
	for name := range settings.ToolProfiles {
		names[name] = true
	}

	result := make([]ToolProfileInfo, 0, len(names))
	for name := range names {
		info := ToolProfileInfo{Name: name}

		// Check if built-in
		if profile, ok := BuiltInProfiles[name]; ok {
			info.Description = profile.Description
			info.Allow = profile.Allow
			info.Deny = profile.Deny
			info.Builtin = true
		} else if profile, ok := settings.ToolProfiles[name]; ok {
			info.Description = profile.Description
			info.Allow = profile.Allow
			info.Deny = profile.Deny
			info.Builtin = false
		}

		result = append(result, info)
	}

	return result
}

// AddProfile adds or updates a custom profile.
func (sm *SettingsManager) AddProfile(profile ToolProfile) error {
	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}

	// Check if it's a built-in profile
	if _, isBuiltin := BuiltInProfiles[profile.Name]; isBuiltin {
		return fmt.Errorf("cannot modify built-in profile: %s", profile.Name)
	}

	settings, _ := sm.Load()
	settings.ToolProfiles[profile.Name] = profile
	return sm.Save(settings)
}

// UpdateProfile updates an existing custom profile.
func (sm *SettingsManager) UpdateProfile(profile ToolProfile) error {
	return sm.AddProfile(profile) // Same logic
}

// DeleteProfile removes a custom profile.
// Returns error if trying to delete a built-in profile.
func (sm *SettingsManager) DeleteProfile(name string) error {
	// Check if it's a built-in profile
	if _, isBuiltin := BuiltInProfiles[name]; isBuiltin {
		return fmt.Errorf("cannot delete built-in profile: %s", name)
	}

	settings, _ := sm.Load()
	delete(settings.ToolProfiles, name)
	return sm.Save(settings)
}

// GetSettingsPath returns the path to the settings file.
func (sm *SettingsManager) GetSettingsPath() string {
	return sm.path
}
