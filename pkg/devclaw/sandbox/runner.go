// Package sandbox – runner.go implements the main ScriptRunner that
// dispatches execution to the appropriate sandbox backend.
package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner is the main script execution manager. It selects the
// appropriate sandbox backend based on isolation level and dispatches
// execution requests.
type Runner struct {
	cfg      Config
	policy   *Policy
	logger   *slog.Logger
	executors map[IsolationLevel]Executor
	mu       sync.RWMutex
}

// NewRunner creates a new script runner with the given configuration.
func NewRunner(cfg Config, logger *slog.Logger) (*Runner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid sandbox config: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}

	r := &Runner{
		cfg:       cfg,
		policy:    NewPolicy(cfg),
		logger:    logger.With("component", "sandbox"),
		executors: make(map[IsolationLevel]Executor),
	}

	// Always register the direct executor (IsolationNone).
	r.executors[IsolationNone] = NewDirectExecutor(cfg, logger)

	// Try to register restricted executor (Linux namespaces).
	restricted := NewRestrictedExecutor(cfg, logger)
	if restricted.Available() {
		r.executors[IsolationRestricted] = restricted
		logger.Info("sandbox: restricted executor available (Linux namespaces)")
	} else {
		logger.Warn("sandbox: restricted executor not available, falling back to direct")
	}

	// Try to register container executor (Docker).
	docker := NewDockerExecutor(cfg, logger)
	if docker.Available() {
		r.executors[IsolationContainer] = docker
		logger.Info("sandbox: container executor available (Docker)")
	} else {
		logger.Warn("sandbox: container executor not available")
	}

	return r, nil
}

// Run executes a script with the configured sandbox.
func (r *Runner) Run(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	// Apply defaults.
	if req.Isolation == "" {
		req.Isolation = r.cfg.DefaultIsolation
	}
	if req.Timeout == 0 {
		req.Timeout = r.cfg.Timeout
	}
	if req.Runtime == "" && req.Script != "" {
		req.Runtime = DetectRuntime(req.Script)
	}

	// Replace {baseDir} in script path and args.
	if req.SkillDir != "" {
		req.Script = strings.ReplaceAll(req.Script, "{baseDir}", req.SkillDir)
		for i, arg := range req.Args {
			req.Args[i] = strings.ReplaceAll(arg, "{baseDir}", req.SkillDir)
		}
	}

	// Validate the request against the security policy.
	if err := r.policy.Validate(req); err != nil {
		return &ExecResult{
			ExitCode:   1,
			Stderr:     fmt.Sprintf("policy violation: %s", err),
			Killed:     true,
			KillReason: "policy_violation",
		}, err
	}

	// Preflight scan: read script content and check for dangerous patterns.
	// Only scan Python and Node scripts where shell-style env vars are a
	// language confusion signal (not shell scripts where $VAR is valid).
	if req.Script != "" && (req.Runtime == RuntimePython || req.Runtime == RuntimeNode) {
		if content, err := os.ReadFile(req.Script); err == nil {
			results := r.policy.ScanScript(string(content))
			if HasCritical(results) {
				var msgs []string
				for _, res := range results {
					if res.Severity == "critical" {
						msgs = append(msgs, fmt.Sprintf("line %d: %s (%s)", res.Line, res.Message, res.Content))
					}
				}
				errMsg := fmt.Sprintf("script preflight blocked: %s", strings.Join(msgs, "; "))
				r.logger.Warn("sandbox: preflight scan blocked script",
					"script", req.Script, "findings", len(results))
				return &ExecResult{
					ExitCode:   1,
					Stderr:     errMsg,
					Killed:     true,
					KillReason: "preflight_blocked",
				}, fmt.Errorf("%s", errMsg)
			}
		}
	}

	// Filter environment variables.
	req.Env = r.policy.FilterEnv(req.Env)

	// Prepare temp directory for this execution.
	tmpDir, err := r.prepareTempDir(req)
	if err != nil {
		return nil, fmt.Errorf("preparing temp dir: %w", err)
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env["DEVCLAW_TMPDIR"] = tmpDir
	req.Env["TMPDIR"] = tmpDir
	req.Env["HOME"] = tmpDir

	// Find the executor for the requested isolation level.
	r.mu.RLock()
	executor, ok := r.executors[req.Isolation]
	r.mu.RUnlock()

	if !ok {
		// Fall back to the next available level.
		executor = r.fallbackExecutor(req.Isolation)
		if executor == nil {
			return nil, fmt.Errorf("no executor available for isolation level %q", req.Isolation)
		}
		r.logger.Warn("sandbox: falling back executor",
			"requested", req.Isolation,
			"using", executor.Name())
	}

	// Apply timeout.
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	r.logger.Info("sandbox: executing script",
		"script", req.Script,
		"runtime", req.Runtime,
		"isolation", req.Isolation,
		"executor", executor.Name(),
		"timeout", req.Timeout,
	)

	start := time.Now()
	result, err := executor.Execute(execCtx, req)
	if result != nil {
		result.Duration = time.Since(start)

		// Collect output files.
		result.OutputFiles = collectOutputFiles(tmpDir)

		// Enforce output size limit.
		r.truncateOutput(result)
	}

	if err != nil {
		r.logger.Error("sandbox: execution failed",
			"script", req.Script,
			"error", err,
			"duration", time.Since(start),
		)
	}

	return result, err
}

// RunPython is a convenience method for running a Python script.
func (r *Runner) RunPython(ctx context.Context, script string, args []string, skillDir string) (*ExecResult, error) {
	return r.Run(ctx, &ExecRequest{
		Runtime:  RuntimePython,
		Script:   script,
		Args:     args,
		SkillDir: skillDir,
	})
}

// RunNode is a convenience method for running a Node.js script.
func (r *Runner) RunNode(ctx context.Context, script string, args []string, skillDir string) (*ExecResult, error) {
	return r.Run(ctx, &ExecRequest{
		Runtime:  RuntimeNode,
		Script:   script,
		Args:     args,
		SkillDir: skillDir,
	})
}

// RunShell is a convenience method for running a shell script.
func (r *Runner) RunShell(ctx context.Context, script string, args []string, skillDir string) (*ExecResult, error) {
	return r.Run(ctx, &ExecRequest{
		Runtime:  RuntimeShell,
		Script:   script,
		Args:     args,
		SkillDir: skillDir,
	})
}

// Close releases all executor resources.
func (r *Runner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []string
	for level, exec := range r.executors {
		if err := exec.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", level, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing executors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ---------- Internal ----------

// fallbackExecutor finds the best available executor when the
// requested one isn't available.
func (r *Runner) fallbackExecutor(requested IsolationLevel) Executor {
	// Fallback order: container → restricted → none
	order := []IsolationLevel{IsolationContainer, IsolationRestricted, IsolationNone}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, level := range order {
		if exec, ok := r.executors[level]; ok {
			return exec
		}
	}
	return nil
}

// prepareTempDir creates a temporary directory for this execution.
func (r *Runner) prepareTempDir(req *ExecRequest) (string, error) {
	baseDir := r.cfg.TempDir
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), "devclaw-sandbox")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(baseDir, "exec-*")
}

// truncateOutput enforces the MaxOutputBytes limit.
func (r *Runner) truncateOutput(result *ExecResult) {
	max := int(r.cfg.MaxOutputBytes)
	if max <= 0 {
		return
	}
	if len(result.Stdout) > max {
		result.Stdout = result.Stdout[:max] + "\n... [output truncated]"
	}
	if len(result.Stderr) > max {
		result.Stderr = result.Stderr[:max] + "\n... [output truncated]"
	}
}

// collectOutputFiles lists files created in the temp directory.
func collectOutputFiles(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}
