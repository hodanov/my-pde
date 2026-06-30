// Package launcher opens a new terminal tab and runs a script in it.
//
// It is an infrastructure adapter: WezTerm and Tmux implement the
// usecase.Launcher port over os/exec.
package launcher

import (
	"fmt"
	"os/exec"

	"ai-bridge/internal/usecase"
)

// CommandRunner executes an external command.
type CommandRunner func(name string, args ...string) error

// DefaultRunner executes commands via os/exec.
func DefaultRunner(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// New creates a launcher implementing usecase.Launcher for the given name.
func New(name string, runner CommandRunner) (usecase.Launcher, error) {
	switch name {
	case "wezterm":
		return &WezTerm{run: runner}, nil
	case "tmux":
		return &Tmux{run: runner}, nil
	default:
		return nil, fmt.Errorf("unknown launcher: %s", name)
	}
}
