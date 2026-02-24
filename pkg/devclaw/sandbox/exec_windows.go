//go:build windows

package sandbox

import (
	"context"
	"errors"
	"log/slog"
)

var ErrWindowsSandboxNotSupported = errors.New("sandbox execution is not supported on Windows natively yet")

type DirectExecutor struct {
	cfg    Config
	logger *slog.Logger
}

func NewDirectExecutor(cfg Config, logger *slog.Logger) *DirectExecutor {
	return &DirectExecutor{cfg: cfg, logger: logger}
}

func (e *DirectExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	return nil, ErrWindowsSandboxNotSupported
}

func (e *DirectExecutor) Available() bool { return false }
func (e *DirectExecutor) Name() string    { return "direct" }
func (e *DirectExecutor) Close() error    { return nil }

type RestrictedExecutor struct {
	cfg    Config
	logger *slog.Logger
}

func NewRestrictedExecutor(cfg Config, logger *slog.Logger) *RestrictedExecutor {
	return &RestrictedExecutor{cfg: cfg, logger: logger}
}

func (e *RestrictedExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	return nil, ErrWindowsSandboxNotSupported
}

func (e *RestrictedExecutor) Available() bool { return false }
func (e *RestrictedExecutor) Name() string    { return "restricted" }
func (e *RestrictedExecutor) Close() error    { return nil }
