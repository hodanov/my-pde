package domain

import (
	"strings"
	"testing"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		bridgeDir  string
		cli        string
		launcher   string
		wantErrMsg string
	}{
		{
			name:      "valid simple values",
			bridgeDir: "/tmp/bridge",
			cli:       "claude",
			launcher:  "wezterm",
		},
		{
			name:      "CLI with slash is valid",
			bridgeDir: "/tmp/bridge",
			cli:       "path/to/claude",
			launcher:  "tmux",
		},
		{
			name:       "invalid CLI returns error",
			bridgeDir:  "/tmp/bridge",
			cli:        "bad command!",
			launcher:   "wezterm",
			wantErrMsg: "invalid CLI command name",
		},
		{
			name:       "invalid launcher returns error",
			bridgeDir:  "/tmp/bridge",
			cli:        "claude",
			launcher:   "Bad Launcher",
			wantErrMsg: "invalid launcher name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := NewConfig(tt.bridgeDir, tt.cli, tt.launcher)

			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.BridgeDir != tt.bridgeDir {
				t.Errorf("BridgeDir = %q, want %q", cfg.BridgeDir, tt.bridgeDir)
			}
			if cfg.CLI != tt.cli {
				t.Errorf("CLI = %q, want %q", cfg.CLI, tt.cli)
			}
			if cfg.Launcher != tt.launcher {
				t.Errorf("Launcher = %q, want %q", cfg.Launcher, tt.launcher)
			}
		})
	}
}
