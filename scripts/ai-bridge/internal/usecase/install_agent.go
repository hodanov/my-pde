package usecase

import (
	"fmt"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// InstallAgent resolves the running binary and installs the launchd agent that
// runs it as a daemon with the configured CLI and launcher.
type InstallAgent struct {
	exe       port.Executable
	installer port.AgentInstaller
	cfg       *domain.Config
}

// NewInstallAgent wires the dependencies of the install use case.
func NewInstallAgent(exe port.Executable, installer port.AgentInstaller, cfg *domain.Config) *InstallAgent {
	return &InstallAgent{exe: exe, installer: installer, cfg: cfg}
}

// Run installs and loads the launchd agent.
func (uc *InstallAgent) Run() error {
	binaryPath, pathErr := uc.exe.Path()
	if pathErr != nil {
		return fmt.Errorf("cannot determine executable path: %w", pathErr)
	}
	return uc.installer.Install(binaryPath, uc.cfg.CLI, uc.cfg.Launcher)
}
