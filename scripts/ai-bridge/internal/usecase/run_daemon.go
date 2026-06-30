package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// RunDaemon is the daemon main loop: it ensures the bridge directory exists,
// watches it for requests, and dispatches each consumed request to the
// request-processing use case.
type RunDaemon struct {
	dirs    port.BridgeDirEnsurer
	watcher port.Watcher
	process *ProcessRequest
	cfg     *domain.Config
}

// NewRunDaemon wires the dependencies of the daemon loop.
func NewRunDaemon(dirs port.BridgeDirEnsurer, watcher port.Watcher, process *ProcessRequest, cfg *domain.Config) *RunDaemon {
	return &RunDaemon{dirs: dirs, watcher: watcher, process: process, cfg: cfg}
}

// Run starts the loop and blocks until ctx is cancelled.
func (uc *RunDaemon) Run(ctx context.Context) error {
	if ensureErr := uc.dirs.Ensure(uc.cfg.BridgeDir); ensureErr != nil {
		return fmt.Errorf("create bridge dir: %w", ensureErr)
	}

	slog.Info("ai-bridge-daemon: started",
		"cli", uc.cfg.CLI,
		"launcher", uc.cfg.Launcher,
		"watching", filepath.Join(uc.cfg.BridgeDir, "request.json"),
	)

	ch, watchErr := uc.watcher.Watch(ctx)
	if watchErr != nil {
		return fmt.Errorf("start watcher: %w", watchErr)
	}

	for consumedPath := range ch {
		if processErr := uc.process.Handle(consumedPath); processErr != nil {
			slog.Warn("request failed", "error", processErr)
		}
	}

	return nil
}
