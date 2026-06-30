// Package usecase holds the application business rules of ai-bridge. It
// orchestrates the domain layer and depends only on domain types and the
// interfaces declared in the port subpackage.
package usecase

import (
	"fmt"
	"log/slog"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// ProcessRequest handles a single consumed request file end to end: load and
// validate it, generate a launch script, and launch it in a terminal.
type ProcessRequest struct {
	requests port.RequestRepository
	dirs     port.DirVerifier
	scripts  port.ScriptStore
	launcher port.Launcher
	cli      string
}

// NewProcessRequest wires the dependencies of the request-processing use case.
func NewProcessRequest(requests port.RequestRepository, dirs port.DirVerifier, scripts port.ScriptStore, launcher port.Launcher, cli string) *ProcessRequest {
	return &ProcessRequest{requests: requests, dirs: dirs, scripts: scripts, launcher: launcher, cli: cli}
}

// Handle processes the request at consumedPath. The consumed file is always
// removed, whether or not processing succeeds.
func (uc *ProcessRequest) Handle(consumedPath string) error {
	req, loadErr := uc.requests.Load(consumedPath)
	uc.requests.Remove(consumedPath)
	if loadErr != nil {
		return fmt.Errorf("invalid request: %w", loadErr)
	}

	if !uc.dirs.IsDir(req.CWD) {
		return fmt.Errorf("cwd is not a valid directory: %s", req.CWD)
	}

	scriptPath, saveErr := uc.scripts.Save(func(scriptPath string) string {
		return domain.BuildScript(uc.cli, req.Prompt, scriptPath)
	})
	if saveErr != nil {
		return fmt.Errorf("generate script: %w", saveErr)
	}

	slog.Info("ai-bridge-daemon: launching", "cwd", req.CWD)
	if launchErr := uc.launcher.Launch(req.CWD, scriptPath); launchErr != nil {
		uc.scripts.Remove(scriptPath)
		return fmt.Errorf("launcher failed: %w", launchErr)
	}
	return nil
}
