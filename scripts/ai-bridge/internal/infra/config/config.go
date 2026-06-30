// Package config loads ai-bridge configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"ai-bridge/internal/domain"
)

// Load resolves configuration from environment variables, applying defaults for
// unset variables and validating the result via the domain layer. It is the
// single source of truth for the AI_BRIDGE_* defaults, shared by the daemon and
// the launchd installer.
func Load() (*domain.Config, error) {
	home, homeDirErr := os.UserHomeDir()
	if homeDirErr != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", homeDirErr)
	}

	bridgeDir := os.Getenv("AI_BRIDGE_DIR")
	if bridgeDir == "" {
		bridgeDir = filepath.Join(home, domain.DefaultBridgeDirName)
	}

	cli := os.Getenv("AI_BRIDGE_CLI")
	if cli == "" {
		cli = domain.DefaultCLI
	}

	launcher := os.Getenv("AI_BRIDGE_LAUNCHER")
	if launcher == "" {
		launcher = domain.DefaultLauncher
	}

	return domain.NewConfig(bridgeDir, cli, launcher)
}
