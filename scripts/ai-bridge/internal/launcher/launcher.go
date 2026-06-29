// Package launcher opens a new terminal tab and runs a script in it.
package launcher

import (
	"fmt"
	"os/exec"
)

// Launcher opens a new terminal tab and runs a script.
type Launcher interface {
	Launch(cwd, scriptPath string) error
}

// CommandRunner executes an external command.
type CommandRunner func(name string, args ...string) error

// DefaultRunner executes commands via os/exec.
func DefaultRunner(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// New creates a Launcher for the given name.
func New(name string, runner CommandRunner) (Launcher, error) {
	switch name {
	case "wezterm":
		return &WezTerm{run: runner}, nil
	case "tmux":
		return &Tmux{run: runner}, nil
	default:
		return nil, fmt.Errorf("unknown launcher: %s", name)
	}
}
