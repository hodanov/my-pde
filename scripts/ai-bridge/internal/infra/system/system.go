// Package system provides thin adapters over OS facilities (executable path
// resolution and PATH lookup) used by ai-bridge.
package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Executable resolves the absolute path of the running binary. It implements
// the usecase.Executable port.
type Executable struct{}

// Path returns the absolute path of the running binary.
func (Executable) Path() (string, error) {
	exe, exeErr := os.Executable()
	if exeErr != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", exeErr)
	}
	abs, absErr := filepath.Abs(exe)
	if absErr != nil {
		return "", fmt.Errorf("cannot resolve absolute path: %w", absErr)
	}
	return abs, nil
}

// CommandLocator resolves executables in PATH via os/exec. It implements the
// usecase.CommandLocator port.
type CommandLocator struct{}

// LookPath resolves file in PATH.
func (CommandLocator) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
