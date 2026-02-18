// Package sandbox provides secure script execution for DevClaw.
//
// It supports multiple isolation levels for running Python, Node.js,
// and Shell scripts from skills (including ClawdHub community skills):
//
//   - none:       Direct exec.Command (trusted/builtin skills only)
//   - restricted: Linux namespaces + resource limits (community skills)
//   - container:  Docker-based full isolation (untrusted scripts)
//
// The sandbox enforces:
//   - Execution timeouts
//   - Memory and CPU limits
//   - Filesystem scoping (read-only workspace, writable tmpdir)
//   - Environment variable filtering (blocks dangerous vars)
//   - Command allowlisting
//   - Output size limits
package sandbox

import (
	"context"
	"fmt"
	"time"
)

// IsolationLevel defines how strictly a script is sandboxed.
type IsolationLevel string

const (
	// IsolationNone runs scripts directly via exec.Command.
	// Use only for trusted/builtin skills.
	IsolationNone IsolationLevel = "none"

	// IsolationRestricted uses Linux namespaces, seccomp, and cgroups
	// for lightweight isolation without Docker.
	IsolationRestricted IsolationLevel = "restricted"

	// IsolationContainer runs scripts inside a Docker container
	// with a purpose-built sandbox image.
	IsolationContainer IsolationLevel = "container"
)

// Runtime identifies the script interpreter.
type Runtime string

const (
	RuntimePython Runtime = "python"
	RuntimeNode   Runtime = "node"
	RuntimeShell  Runtime = "shell"
	RuntimeBinary Runtime = "binary"
)

// Config holds the sandbox configuration.
type Config struct {
	// DefaultIsolation is the isolation level for skills that don't
	// specify one. Defaults to IsolationRestricted.
	DefaultIsolation IsolationLevel `yaml:"default_isolation"`

	// Timeout is the maximum execution time for a single script run.
	// Defaults to 60s.
	Timeout time.Duration `yaml:"timeout"`

	// MaxOutputBytes limits stdout+stderr capture size.
	// Defaults to 1MB.
	MaxOutputBytes int64 `yaml:"max_output_bytes"`

	// MaxMemoryMB limits memory usage (restricted/container modes).
	// Defaults to 256MB.
	MaxMemoryMB int `yaml:"max_memory_mb"`

	// MaxCPUPercent limits CPU usage as percentage (0-100).
	// Defaults to 50.
	MaxCPUPercent int `yaml:"max_cpu_percent"`

	// WorkDir is the working directory for script execution.
	// Scripts get read-only access to this directory.
	WorkDir string `yaml:"work_dir"`

	// TempDir is the writable temporary directory for scripts.
	// Each execution gets its own subdirectory.
	TempDir string `yaml:"temp_dir"`

	// Docker contains container-specific settings.
	Docker DockerConfig `yaml:"docker"`

	// AllowedEnv is a list of environment variable names that scripts
	// are allowed to access. If empty, uses the default safe list.
	AllowedEnv []string `yaml:"allowed_env"`

	// BlockedEnv is a list of environment variable names that are
	// always stripped. Takes precedence over AllowedEnv.
	BlockedEnv []string `yaml:"blocked_env"`

	// AllowNetwork controls whether scripts can make network requests.
	// Defaults to false for restricted, true for none.
	AllowNetwork *bool `yaml:"allow_network"`

	// Runtimes maps Runtime to interpreter paths.
	// Defaults: python→python3, node→node, shell→/bin/sh
	Runtimes map[Runtime]string `yaml:"runtimes"`
}

// DockerConfig holds Docker-specific sandbox settings.
type DockerConfig struct {
	// Image is the Docker image to use for sandboxed execution.
	// Defaults to "devclaw-sandbox:latest".
	Image string `yaml:"image"`

	// BuildOnStart builds the sandbox image on startup if missing.
	BuildOnStart bool `yaml:"build_on_start"`

	// Network is the Docker network mode.
	// Defaults to "none" (no network access).
	Network string `yaml:"network"`

	// ExtraVolumes are additional volume mounts (host:container:mode).
	ExtraVolumes []string `yaml:"extra_volumes"`
}

// ExecRequest describes a script execution request.
type ExecRequest struct {
	// Runtime is the script interpreter to use.
	Runtime Runtime

	// Isolation overrides the default isolation level.
	// If empty, uses Config.DefaultIsolation.
	Isolation IsolationLevel

	// Script is the path to the script file to execute.
	Script string

	// Args are command-line arguments passed to the script.
	Args []string

	// Stdin provides data to the script's standard input.
	Stdin string

	// Env are additional environment variables for this execution.
	// Subject to filtering by the security policy.
	Env map[string]string

	// WorkDir overrides the working directory for this execution.
	WorkDir string

	// Timeout overrides the default timeout for this execution.
	Timeout time.Duration

	// SkillDir is the skill's base directory (replaces {baseDir}).
	SkillDir string
}

// ExecResult holds the outcome of a script execution.
type ExecResult struct {
	// Stdout is the captured standard output.
	Stdout string

	// Stderr is the captured standard error.
	Stderr string

	// ExitCode is the process exit code.
	ExitCode int

	// Duration is how long the execution took.
	Duration time.Duration

	// Killed is true if the process was killed (timeout, OOM, etc.).
	Killed bool

	// KillReason explains why the process was killed.
	KillReason string

	// OutputFiles lists files created in the temp directory.
	OutputFiles []string
}

// Executor is the interface for sandbox implementations.
type Executor interface {
	// Execute runs a script with the given request.
	Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error)

	// Available reports whether this executor is usable on the system.
	Available() bool

	// Name returns the executor name for logging.
	Name() string

	// Close releases resources held by the executor.
	Close() error
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	allowNet := false
	return Config{
		DefaultIsolation: IsolationRestricted,
		Timeout:          60 * time.Second,
		MaxOutputBytes:   1 * 1024 * 1024, // 1MB
		MaxMemoryMB:      256,
		MaxCPUPercent:     50,
		TempDir:          "/tmp/devclaw-sandbox",
		AllowNetwork:     &allowNet,
		Docker: DockerConfig{
			Image:        "devclaw-sandbox:latest",
			BuildOnStart: true,
			Network:      "none",
		},
		Runtimes: map[Runtime]string{
			RuntimePython: "python3",
			RuntimeNode:   "node",
			RuntimeShell:  "/bin/sh",
		},
		BlockedEnv: defaultBlockedEnv(),
	}
}

// defaultBlockedEnv returns environment variables that are always
// stripped from script execution, as they can be used for injection.
func defaultBlockedEnv() []string {
	return []string{
		// Node.js injection vectors
		"NODE_OPTIONS",
		"NODE_PATH",
		// Python injection vectors
		"PYTHONHOME",
		"PYTHONPATH",
		"PYTHONSTARTUP",
		// Ruby/Perl injection vectors
		"RUBYOPT",
		"PERL5LIB",
		"PERL5OPT",
		// Dynamic linker injection
		"LD_PRELOAD",
		"LD_LIBRARY_PATH",
		"DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH",
		// Shell injection
		"BASH_ENV",
		"ENV",
		"CDPATH",
		// OC-09: prevent PATH override (credential theft vector)
		"PATH",
	}
}

// blockedEnvPrefixes are environment variable prefixes that are always
// stripped. Catches families of dangerous vars (e.g. LD_*, DYLD_*).
var blockedEnvPrefixes = []string{
	"LD_",
	"DYLD_",
}

// DetectRuntime guesses the runtime from a script path.
func DetectRuntime(path string) Runtime {
	switch {
	case hasSuffix(path, ".py"):
		return RuntimePython
	case hasSuffix(path, ".js", ".mjs", ".cjs", ".ts", ".mts"):
		return RuntimeNode
	case hasSuffix(path, ".sh", ".bash"):
		return RuntimeShell
	default:
		return RuntimeBinary
	}
}

func hasSuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

// Validate checks that the config is valid.
func (c *Config) Validate() error {
	switch c.DefaultIsolation {
	case IsolationNone, IsolationRestricted, IsolationContainer:
		// ok
	default:
		return fmt.Errorf("invalid isolation level: %q", c.DefaultIsolation)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if c.MaxOutputBytes <= 0 {
		return fmt.Errorf("max_output_bytes must be positive")
	}
	if c.MaxMemoryMB <= 0 {
		return fmt.Errorf("max_memory_mb must be positive")
	}
	return nil
}
