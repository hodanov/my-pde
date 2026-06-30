package usecase

import (
	"fmt"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// Diagnose verifies the prerequisites of the bridge: the bridge directory and
// the configured CLI and launcher executables.
type Diagnose struct {
	dirs    port.DirInspector
	locator port.CommandLocator
	cfg     *domain.Config
}

// NewDiagnose wires the dependencies of the diagnostics use case.
func NewDiagnose(dirs port.DirInspector, locator port.CommandLocator, cfg *domain.Config) *Diagnose {
	return &Diagnose{dirs: dirs, locator: locator, cfg: cfg}
}

// Run executes the diagnostics and returns the results in a stable order.
func (uc *Diagnose) Run() []domain.Check {
	return []domain.Check{
		uc.checkBridgeDir(uc.cfg.BridgeDir),
		uc.checkExecutable("CLI", uc.cfg.CLI),
		uc.checkExecutable("Launcher", uc.cfg.Launcher),
	}
}

func (uc *Diagnose) checkBridgeDir(dir string) domain.Check {
	isDir, statErr := uc.dirs.StatDir(dir)
	if statErr != nil {
		return domain.Check{Name: "BridgeDir", Status: domain.StatusFail, Detail: fmt.Sprintf("%s: %v", dir, statErr)}
	}
	if !isDir {
		return domain.Check{Name: "BridgeDir", Status: domain.StatusFail, Detail: fmt.Sprintf("%s: not a directory", dir)}
	}
	if writableErr := uc.dirs.EnsureWritable(dir); writableErr != nil {
		return domain.Check{Name: "BridgeDir", Status: domain.StatusFail, Detail: fmt.Sprintf("%s: not writable: %v", dir, writableErr)}
	}
	return domain.Check{Name: "BridgeDir", Status: domain.StatusOK, Detail: dir}
}

func (uc *Diagnose) checkExecutable(name, cmd string) domain.Check {
	resolved, lookErr := uc.locator.LookPath(cmd)
	if lookErr != nil {
		return domain.Check{Name: name, Status: domain.StatusFail, Detail: fmt.Sprintf("%q not found in PATH: %v", cmd, lookErr)}
	}
	return domain.Check{Name: name, Status: domain.StatusOK, Detail: fmt.Sprintf("%s -> %s", cmd, resolved)}
}
