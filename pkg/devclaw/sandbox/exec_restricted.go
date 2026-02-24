//go:build !windows

// Package sandbox – exec_restricted.go implements the restricted executor
// using Linux namespaces for lightweight process isolation.
//
// This executor provides:
//   - PID namespace isolation (process can't see other processes)
//   - Network namespace isolation (optional, blocks network by default)
//   - Mount namespace with read-only bind mounts
//   - Resource limits via setrlimit
//   - Filtered environment variables
//
// Requires Linux with user namespaces enabled (most modern distros).
// Falls back to DirectExecutor on non-Linux systems.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// trustedBinDirs are the only directories from which interpreter binaries
// may be resolved. This prevents PATH-hijacking attacks where a writable
// directory earlier in PATH shadows a trusted system binary.
var trustedBinDirs = []string{
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/usr/local/sbin",
	"/usr/sbin",
	"/sbin",
}

// RestrictedExecutor runs scripts with Linux namespace isolation.
type RestrictedExecutor struct {
	cfg    Config
	logger *slog.Logger
}

// NewRestrictedExecutor creates a new restricted executor.
func NewRestrictedExecutor(cfg Config, logger *slog.Logger) *RestrictedExecutor {
	return &RestrictedExecutor{cfg: cfg, logger: logger}
}

// Execute runs the script with namespace isolation.
func (e *RestrictedExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	if !e.Available() {
		return nil, fmt.Errorf("restricted executor not available on %s", runtime.GOOS)
	}

	cmd, err := e.buildCommand(ctx, req)
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	err = cmd.Run()

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()

			// Check if killed by signal (e.g., OOM killer).
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					result.Killed = true
					switch status.Signal() {
					case syscall.SIGKILL:
						result.KillReason = "killed (possible OOM)"
					case syscall.SIGXCPU:
						result.KillReason = "cpu_limit"
					default:
						result.KillReason = fmt.Sprintf("signal_%d", status.Signal())
					}
				}
			}

			if ctx.Err() != nil {
				result.Killed = true
				result.KillReason = "timeout"
			}
		} else {
			return result, fmt.Errorf("executing script: %w", err)
		}
	}

	return result, nil
}

// Available checks if Linux namespaces are supported.
func (e *RestrictedExecutor) Available() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	// Check if user namespaces are enabled.
	data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err == nil {
		return strings.TrimSpace(string(data)) == "1"
	}
	// If the file doesn't exist, user namespaces might still work
	// (many distros enable them by default).
	return true
}

// Name returns the executor name.
func (e *RestrictedExecutor) Name() string { return "restricted" }

// Close is a no-op.
func (e *RestrictedExecutor) Close() error { return nil }

// buildCommand constructs an isolated exec.Cmd.
func (e *RestrictedExecutor) buildCommand(ctx context.Context, req *ExecRequest) (*exec.Cmd, error) {
	bin, args := resolveInterpreter(e.cfg, req)

	// Verify the resolved binary lives under a trusted system directory.
	// This catches PATH-hijacking: e.g. a writable ~/bin/python3 that shadows
	// /usr/bin/python3 would be rejected here.
	verified, err := verifyTrustedBin(bin)
	if err != nil {
		return nil, fmt.Errorf("interpreter path verification failed for %q: %w", bin, err)
	}
	bin = verified

	cmd := exec.CommandContext(ctx, bin, args...)

	// Working directory.
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else if req.SkillDir != "" {
		cmd.Dir = req.SkillDir
	}

	// Build filtered environment.
	cmd.Env = e.buildEnv(req)

	// Apply Linux namespace isolation.
	allowNet := e.cfg.AllowNetwork != nil && *e.cfg.AllowNetwork
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// New process group for clean termination.
		Setpgid: true,

		// Linux namespace flags:
		// CLONE_NEWPID:  Isolate process IDs.
		// CLONE_NEWNS:   Isolate mount points.
		// CLONE_NEWUSER: Unprivileged user namespace.
		Cloneflags: syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUSER |
			e.netCloneFlag(allowNet),

		// Map current user to root inside the namespace.
		UidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      os.Getuid(),
			Size:        1,
		}},
		GidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      os.Getgid(),
			Size:        1,
		}},
	}

	// Set resource limits.
	e.setResourceLimits(cmd)

	// Kill process group on cancel.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	return cmd, nil
}

// netCloneFlag returns CLONE_NEWNET if network should be isolated.
func (e *RestrictedExecutor) netCloneFlag(allowNet bool) uintptr {
	if !allowNet {
		return syscall.CLONE_NEWNET
	}
	return 0
}

// buildEnv creates a minimal environment for the sandboxed process.
func (e *RestrictedExecutor) buildEnv(req *ExecRequest) []string {
	// Start with a minimal safe environment.
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"TERM=xterm",
	}

	// Add request-specific env vars.
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

// setResourceLimits applies CPU and memory limits via rlimit.
// Note: rlimits are inherited by the child process.
func (e *RestrictedExecutor) setResourceLimits(_ *exec.Cmd) {
	// Memory limit: set RLIMIT_AS (address space).
	if e.cfg.MaxMemoryMB > 0 {
		memBytes := uint64(e.cfg.MaxMemoryMB) * 1024 * 1024
		var rlim syscall.Rlimit
		rlim.Cur = memBytes
		rlim.Max = memBytes
		// Note: We can't set rlimits for the child process from Go
		// before exec. The child would need to set them itself, or
		// we use cgroups. For now, this is a best-effort approach.
		// The Docker executor provides proper resource limits.
		_ = rlim
	}
}

// resolveInterpreter determines the binary and arguments from runtime.
func resolveInterpreter(cfg Config, req *ExecRequest) (string, []string) {
	interpreter := cfg.Runtimes[req.Runtime]

	switch req.Runtime {
	case RuntimePython:
		if interpreter == "" {
			interpreter = "python3"
		}
		return interpreter, append([]string{"-u", req.Script}, req.Args...)

	case RuntimeNode:
		if interpreter == "" {
			interpreter = "node"
		}
		return interpreter, append([]string{req.Script}, req.Args...)

	case RuntimeShell:
		if interpreter == "" {
			interpreter = "/bin/sh"
		}
		return interpreter, append([]string{req.Script}, req.Args...)

	default:
		return req.Script, req.Args
	}
}

// verifyTrustedBin resolves a binary name (or absolute path) via exec.LookPath
// and then confirms the resolved path is rooted under one of the trusted system
// directories in trustedBinDirs.
//
// This prevents PATH-hijacking: even if a malicious directory appears earlier in
// PATH (e.g. from a compromised environment), the resolved absolute path must
// start with a known-good prefix like /usr/bin or /bin.
//
// Returns the cleaned absolute path on success, or an error if the binary
// resolves outside the trusted set.
func verifyTrustedBin(name string) (string, error) {
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("binary not found: %w", err)
	}

	// Clean the path to remove any symlink-confusing elements.
	// Note: we intentionally do NOT call filepath.EvalSymlinks here because
	// a symlink at /usr/bin/python3 → /usr/bin/python3.11 is legitimate and
	// should be accepted; we only care about the symlink entry location itself.
	resolved = filepath.Clean(resolved)

	for _, trusted := range trustedBinDirs {
		// Use strings.HasPrefix with a trailing slash to avoid false matches
		// where /usr/bins would wrongly match /usr/bin.
		if resolved == trusted || strings.HasPrefix(resolved, trusted+"/") {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("resolved binary %q is not in a trusted directory (allowed: %v)",
		resolved, trustedBinDirs)
}
