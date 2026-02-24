//go:build !windows

// Package sandbox – exec_direct.go implements the direct executor
// (IsolationNone). Runs scripts via os/exec without any sandboxing.
// Use only for trusted/builtin skills.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// DirectExecutor runs scripts directly via exec.Command.
type DirectExecutor struct {
	cfg    Config
	logger *slog.Logger
}

// NewDirectExecutor creates a new direct executor.
func NewDirectExecutor(cfg Config, logger *slog.Logger) *DirectExecutor {
	return &DirectExecutor{cfg: cfg, logger: logger}
}

// Execute runs the script directly.
func (e *DirectExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
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

// Available always returns true — direct execution is always possible.
func (e *DirectExecutor) Available() bool { return true }

// Name returns the executor name.
func (e *DirectExecutor) Name() string { return "direct" }

// Close is a no-op for the direct executor.
func (e *DirectExecutor) Close() error { return nil }

// buildCommand constructs the exec.Cmd for the request.
func (e *DirectExecutor) buildCommand(ctx context.Context, req *ExecRequest) (*exec.Cmd, error) {
	bin, args := e.resolveCommand(req)

	cmd := exec.CommandContext(ctx, bin, args...)

	// Set working directory.
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else if e.cfg.WorkDir != "" {
		cmd.Dir = e.cfg.WorkDir
	}

	// Build environment.
	cmd.Env = e.buildEnv(req)

	// Set process group so we can kill the whole tree on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Cancel kills the process group.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	return cmd, nil
}

// resolveCommand determines the binary and arguments based on runtime.
func (e *DirectExecutor) resolveCommand(req *ExecRequest) (string, []string) {
	interpreter := e.cfg.Runtimes[req.Runtime]

	switch req.Runtime {
	case RuntimePython:
		if interpreter == "" {
			interpreter = "python3"
		}
		args := append([]string{"-u", req.Script}, req.Args...)
		return interpreter, args

	case RuntimeNode:
		if interpreter == "" {
			interpreter = "node"
		}
		args := append([]string{req.Script}, req.Args...)
		return interpreter, args

	case RuntimeShell:
		if interpreter == "" {
			interpreter = "/bin/sh"
		}
		args := append([]string{req.Script}, req.Args...)
		return interpreter, args

	case RuntimeBinary:
		return req.Script, req.Args

	default:
		return req.Script, req.Args
	}
}

// buildEnv constructs the environment for the subprocess.
func (e *DirectExecutor) buildEnv(req *ExecRequest) []string {
	// Start from current env.
	env := os.Environ()

	// Add request-specific env vars.
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}
