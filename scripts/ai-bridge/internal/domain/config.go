// Package domain holds the enterprise business rules of ai-bridge.
//
// It is a pure layer: it performs no I/O (no filesystem, environment, process,
// or clock access) and depends only on the standard library. All values that
// cross a layer boundary as side effects are abstracted behind ports defined in
// the usecase layer; the domain layer only models data and the rules over it.
package domain

import (
	"fmt"
	"regexp"
)

// Default configuration values used when an environment variable is unset.
const (
	// DefaultCLI is the AI CLI invoked when AI_BRIDGE_CLI is unset.
	DefaultCLI = "claude"
	// DefaultLauncher is the terminal launcher used when AI_BRIDGE_LAUNCHER is unset.
	DefaultLauncher = "wezterm"
	// DefaultBridgeDirName is the bridge directory name under the home directory
	// used when AI_BRIDGE_DIR is unset.
	DefaultBridgeDirName = ".ai-bridge"
)

var (
	cliPattern      = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_./-]*$`)
	launcherPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// Config holds validated daemon configuration.
type Config struct {
	BridgeDir string
	CLI       string
	Launcher  string
}

// NewConfig validates the given values and returns a Config. The CLI name must
// match a conservative command-name pattern and the launcher must be a simple
// lowercase token; both guard against injection into the generated script and
// launcher arguments.
func NewConfig(bridgeDir, cli, launcher string) (*Config, error) {
	if !cliPattern.MatchString(cli) {
		return nil, fmt.Errorf("invalid CLI command name: %s", cli)
	}
	if !launcherPattern.MatchString(launcher) {
		return nil, fmt.Errorf("invalid launcher name: %s (only [a-z0-9_-] allowed)", launcher)
	}
	return &Config{BridgeDir: bridgeDir, CLI: cli, Launcher: launcher}, nil
}
