package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	// NOTE: t.Parallel() を付けない。
	// Go 1.22 以降、親テストが Parallel の場合でもサブテストで t.Setenv が使えなくなる。
	home, _ := os.UserHomeDir()

	tests := []struct {
		name         string
		envDir       string
		envCLI       string
		envLauncher  string
		wantDir      string
		wantCLI      string
		wantLauncher string
		wantErrMsg   string
	}{
		{
			name:         "defaults when env vars are empty",
			wantDir:      filepath.Join(home, ".ai-bridge"),
			wantCLI:      "claude",
			wantLauncher: "wezterm",
		},
		{
			name:         "custom env vars override defaults",
			envDir:       "/tmp/test-bridge",
			envCLI:       "cursor",
			envLauncher:  "tmux",
			wantDir:      "/tmp/test-bridge",
			wantCLI:      "cursor",
			wantLauncher: "tmux",
		},
		{
			name:         "CLI with slash is valid",
			envCLI:       "path/to/claude",
			wantDir:      filepath.Join(home, ".ai-bridge"),
			wantCLI:      "path/to/claude",
			wantLauncher: "wezterm",
		},
		{
			name:       "invalid CLI returns error",
			envCLI:     "bad command!",
			wantErrMsg: "invalid CLI command name",
		},
		{
			name:        "invalid launcher returns error",
			envLauncher: "Bad Launcher",
			wantErrMsg:  "invalid launcher name",
		},
	}

	// NOTE: サブテストには t.Parallel() を付けない。t.Setenv はパラレルテストで使用できないため。
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AI_BRIDGE_DIR", tt.envDir)
			t.Setenv("AI_BRIDGE_CLI", tt.envCLI)
			t.Setenv("AI_BRIDGE_LAUNCHER", tt.envLauncher)

			cfg, err := Load()

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
			if cfg.BridgeDir != tt.wantDir {
				t.Errorf("BridgeDir = %q, want %q", cfg.BridgeDir, tt.wantDir)
			}
			if cfg.CLI != tt.wantCLI {
				t.Errorf("CLI = %q, want %q", cfg.CLI, tt.wantCLI)
			}
			if cfg.Launcher != tt.wantLauncher {
				t.Errorf("Launcher = %q, want %q", cfg.Launcher, tt.wantLauncher)
			}
		})
	}
}
