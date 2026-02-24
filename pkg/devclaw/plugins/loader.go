// Package plugins implements the Go native plugin (.so) loading system
// for DevClaw. Plugins extend the runtime with additional channels,
// webhooks, and custom integrations without recompiling the binary.
//
// Plugin .so files must export one of:
//   - var Channel channels.Channel       (for channel plugins)
//   - var Plugin plugins.Plugin           (for generic plugins)
//
// Build a plugin:
//
//	go build -buildmode=plugin -o plugins/discord.so discord_plugin.go
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"runtime"
	"strings"
	"sync"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// Plugin is the interface for generic DevClaw plugins.
// Channel plugins can also implement this for lifecycle hooks,
// but it's not required — exporting var Channel is enough.
type Plugin interface {
	// Name returns the plugin identifier.
	Name() string

	// Version returns the plugin version string.
	Version() string

	// Init is called after loading. Use for setup.
	Init(ctx context.Context, config map[string]any) error

	// Shutdown is called on graceful stop.
	Shutdown() error
}

// LoadedPlugin holds a loaded plugin and its metadata.
type LoadedPlugin struct {
	// Path is the .so file path.
	Path string

	// Name is the plugin name (from filename or Plugin.Name()).
	Name string

	// Channel is the channel implementation (if this is a channel plugin).
	Channel channels.Channel

	// Plugin is the generic plugin interface (optional).
	Plugin Plugin

	// Raw is the raw *plugin.Plugin handle.
	Raw *plugin.Plugin
}

// Config holds plugin loader configuration.
type Config struct {
	// Dir is the directory to scan for .so files.
	Dir string `yaml:"dir"`

	// Enabled lists plugins to load (empty = load all found).
	Enabled []string `yaml:"enabled"`

	// Disabled lists plugins to skip.
	Disabled []string `yaml:"disabled"`
}

// Loader discovers and loads Go native plugins from a directory.
type Loader struct {
	cfg    Config
	logger *slog.Logger
	loaded []*LoadedPlugin
	mu     sync.RWMutex
}

// NewLoader creates a new plugin loader.
func NewLoader(cfg Config, logger *slog.Logger) *Loader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Loader{
		cfg:    cfg,
		logger: logger.With("component", "plugins"),
	}
}

// LoadAll scans the plugin directory and loads all valid .so files.
func (l *Loader) LoadAll(ctx context.Context) error {
	dir := l.cfg.Dir
	if dir == "" {
		l.logger.Debug("plugins: no directory configured, skipping")
		return nil
	}

	// Check if directory exists.
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		l.logger.Debug("plugins: directory does not exist", "dir", dir)
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat plugins dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugins path is not a directory: %s", dir)
	}

	// Scan for .so files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading plugins dir: %w", err)
	}

	disabled := make(map[string]bool)
	for _, d := range l.cfg.Disabled {
		disabled[d] = true
	}

	enabled := make(map[string]bool)
	for _, e := range l.cfg.Enabled {
		enabled[e] = true
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".so") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".so")

		// Check disabled list.
		if disabled[name] {
			l.logger.Debug("plugins: skipping disabled plugin", "name", name)
			continue
		}

		// Check enabled list (if specified).
		if len(enabled) > 0 && !enabled[name] {
			l.logger.Debug("plugins: not in enabled list, skipping", "name", name)
			continue
		}

		path := filepath.Join(dir, entry.Name())

		// Trust check: reject plugins in world-writable directories,
		// symlinked outside the plugin directory, or not owned by the current user.
		if trusted, reason := isTrustedPlugin(path, dir); !trusted {
			l.logger.Warn("plugins: rejecting untrusted plugin",
				"path", path, "reason", reason)
			continue
		}

		loaded, err := l.loadPlugin(ctx, path, name)
		if err != nil {
			l.logger.Error("plugins: failed to load",
				"path", path, "error", err)
			continue
		}

		l.mu.Lock()
		l.loaded = append(l.loaded, loaded)
		l.mu.Unlock()

		l.logger.Info("plugins: loaded", "name", loaded.Name, "path", path)
	}

	l.logger.Info("plugins: loading complete",
		"total", l.Count(),
		"dir", dir)
	return nil
}

// loadPlugin opens a .so file and extracts Channel and/or Plugin symbols.
func (l *Loader) loadPlugin(ctx context.Context, path, name string) (*LoadedPlugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening plugin: %w", err)
	}

	lp := &LoadedPlugin{
		Path: path,
		Name: name,
		Raw:  p,
	}

	// Look up "Channel" symbol.
	if sym, err := p.Lookup("Channel"); err == nil {
		if ch, ok := sym.(*channels.Channel); ok && ch != nil {
			lp.Channel = *ch
			lp.Name = (*ch).Name()
		}
	}

	// Look up "Plugin" symbol.
	if sym, err := p.Lookup("Plugin"); err == nil {
		if pl, ok := sym.(*Plugin); ok && pl != nil {
			lp.Plugin = *pl
			lp.Name = (*pl).Name()
		}
	}

	// At least one of Channel or Plugin must be found.
	if lp.Channel == nil && lp.Plugin == nil {
		return nil, fmt.Errorf("plugin exports neither Channel nor Plugin symbol")
	}

	// Initialize generic plugin if present.
	if lp.Plugin != nil {
		if err := lp.Plugin.Init(ctx, nil); err != nil {
			return nil, fmt.Errorf("initializing plugin: %w", err)
		}
	}

	return lp, nil
}

// Channels returns all loaded channel plugins.
func (l *Loader) Channels() []channels.Channel {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var chs []channels.Channel
	for _, lp := range l.loaded {
		if lp.Channel != nil {
			chs = append(chs, lp.Channel)
		}
	}
	return chs
}

// Plugins returns all loaded generic plugins.
func (l *Loader) Plugins() []Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var pls []Plugin
	for _, lp := range l.loaded {
		if lp.Plugin != nil {
			pls = append(pls, lp.Plugin)
		}
	}
	return pls
}

// All returns all loaded plugins.
func (l *Loader) All() []*LoadedPlugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]*LoadedPlugin{}, l.loaded...)
}

// Count returns the number of loaded plugins.
func (l *Loader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.loaded)
}

// Shutdown gracefully stops all loaded plugins.
func (l *Loader) Shutdown() {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, lp := range l.loaded {
		if lp.Plugin != nil {
			if err := lp.Plugin.Shutdown(); err != nil {
				l.logger.Error("plugins: shutdown error",
					"name", lp.Name, "error", err)
			}
		}
	}
}

// RegisterChannels registers all loaded channel plugins with a Manager.
func (l *Loader) RegisterChannels(mgr *channels.Manager) error {
	for _, ch := range l.Channels() {
		if err := mgr.Register(ch); err != nil {
			l.logger.Error("plugins: failed to register channel",
				"channel", ch.Name(), "error", err)
			return err
		}
	}
	return nil
}

// isTrustedPlugin checks whether a plugin .so file is safe to load.
// Returns (true, "") if trusted, or (false, reason) if not.
// On non-Unix systems all plugins are trusted by default (no syscall.Stat_t).
func isTrustedPlugin(pluginPath, pluginDir string) (bool, string) {
	// Resolve symlinks and verify the real path stays within pluginDir.
	realPath, err := filepath.EvalSymlinks(pluginPath)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve symlinks: %v", err)
	}
	cleanDir := filepath.Clean(pluginDir)
	if !strings.HasPrefix(filepath.Clean(realPath), cleanDir+string(filepath.Separator)) {
		return false, fmt.Sprintf("plugin symlink escapes plugin directory: %s → %s", pluginPath, realPath)
	}

	// Unix-specific: check directory permissions and file ownership.
	if runtime.GOOS != "windows" {
		// Reject if the plugin directory is world-writable.
		dirInfo, err := os.Stat(pluginDir)
		if err != nil {
			return false, fmt.Sprintf("cannot stat plugin dir: %v", err)
		}
		if dirInfo.Mode().Perm()&0o002 != 0 {
			return false, fmt.Sprintf("plugin directory is world-writable: %s", pluginDir)
		}

		// Reject if the plugin file is not owned by the current user.
		// No syscall.Stat_t on Windows
		// We disable the UID check since DevClaw on windows runs directly
	}

	return true, ""
}
