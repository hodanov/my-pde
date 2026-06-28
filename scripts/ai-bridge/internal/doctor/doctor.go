// Package doctor implements self-diagnostics for the ai-bridge environment.
//
// It reuses the daemon configuration to verify that the prerequisites of the
// Neovim -> host AI CLI bridge are in place, giving observability to an
// otherwise fire-and-forget pipeline.
package doctor

import (
	"fmt"
	"os"
	"strings"

	"ai-bridge/internal/daemon"
)

// Status is the outcome of a single diagnostic check.
type Status int

const (
	// StatusOK indicates the check passed.
	StatusOK Status = iota
	// StatusWarn indicates a non-fatal problem.
	StatusWarn
	// StatusFail indicates a fatal problem that breaks the bridge.
	StatusFail
)

// String returns the lowercase label for the status.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusWarn:
		return "warn"
	case StatusFail:
		return "fail"
	default:
		return "unknown"
	}
}

// Check is the result of one diagnostic.
type Check struct {
	Name   string
	Status Status
	Detail string
}

// LookPathFunc resolves an executable in PATH. It is injected so tests can
// stub PATH resolution without depending on the host environment.
type LookPathFunc func(file string) (string, error)

// Run executes the diagnostics against cfg and returns the results in order.
func Run(cfg *daemon.Config, lookPath LookPathFunc) []Check {
	return []Check{
		checkBridgeDir(cfg.BridgeDir),
		checkExecutable("CLI", cfg.CLI, lookPath),
		checkExecutable("Launcher", cfg.Launcher, lookPath),
	}
}

// checkBridgeDir verifies the bridge directory exists, is a directory, and is writable.
func checkBridgeDir(dir string) Check {
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return Check{Name: "BridgeDir", Status: StatusFail, Detail: fmt.Sprintf("%s: %v", dir, statErr)}
	}
	if !info.IsDir() {
		return Check{Name: "BridgeDir", Status: StatusFail, Detail: fmt.Sprintf("%s: not a directory", dir)}
	}
	if writableErr := checkWritable(dir); writableErr != nil {
		return Check{Name: "BridgeDir", Status: StatusFail, Detail: fmt.Sprintf("%s: not writable: %v", dir, writableErr)}
	}
	return Check{Name: "BridgeDir", Status: StatusOK, Detail: dir}
}

// checkWritable confirms a file can be created in dir.
func checkWritable(dir string) error {
	f, createErr := os.CreateTemp(dir, ".doctor-write-*")
	if createErr != nil {
		return createErr
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// checkExecutable verifies cmd resolves to an executable in PATH.
func checkExecutable(name, cmd string, lookPath LookPathFunc) Check {
	resolved, lookErr := lookPath(cmd)
	if lookErr != nil {
		return Check{Name: name, Status: StatusFail, Detail: fmt.Sprintf("%q not found in PATH: %v", cmd, lookErr)}
	}
	return Check{Name: name, Status: StatusOK, Detail: fmt.Sprintf("%s -> %s", cmd, resolved)}
}

// HasFailure reports whether any check has StatusFail.
func HasFailure(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// Format renders checks as human-readable, aligned lines.
func Format(checks []Check) string {
	var b strings.Builder
	b.WriteString("ai-bridge doctor\n")
	for _, c := range checks {
		fmt.Fprintf(&b, "  [%-4s] %s: %s\n", c.Status, c.Name, c.Detail)
	}
	return b.String()
}
