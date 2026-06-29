package launchd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- generatePlist ---

func TestGeneratePlist(t *testing.T) {
	t.Parallel()
	t.Run("matches expected snapshot", func(t *testing.T) {
		t.Parallel()
		got, genPlistErr := generatePlist("/path/to/ai-bridge", "/usr/local/bin:/usr/bin:/bin", "claude", "wezterm")
		if genPlistErr != nil {
			t.Fatal(genPlistErr)
		}
		expected, readErr := os.ReadFile(filepath.Join("..", "..", "testdata", "plist", "expected.plist"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if got != string(expected) {
			t.Errorf("plist mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, string(expected))
		}
	})

	contentTests := []struct {
		name         string
		binaryPath   string
		cli          string
		launcherName string
		wantContains []string
	}{
		{
			name:         "custom values are embedded in output",
			binaryPath:   "/usr/local/bin/ai-bridge",
			cli:          "cursor",
			launcherName: "tmux",
			wantContains: []string{
				"/usr/local/bin/ai-bridge",
				"<string>cursor</string>",
				"<string>tmux</string>",
			},
		},
		{
			name:         "output has required plist structure",
			binaryPath:   "/bin/test",
			cli:          "claude",
			launcherName: "wezterm",
			wantContains: []string{
				`<?xml version="1.0" encoding="UTF-8"?>`,
				`<key>Label</key>`,
				`<string>com.ai-bridge.daemon</string>`,
				`<key>ProgramArguments</key>`,
				`<string>daemon</string>`,
				`<key>RunAtLoad</key>`,
				`<true/>`,
				`<key>KeepAlive</key>`,
				`<key>StandardOutPath</key>`,
				`<key>StandardErrorPath</key>`,
			},
		},
	}

	for _, tt := range contentTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, genPlistErr := generatePlist(tt.binaryPath, "/usr/local/bin:/usr/bin:/bin", tt.cli, tt.launcherName)
			if genPlistErr != nil {
				t.Fatal(genPlistErr)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("plist missing %q", want)
				}
			}
		})
	}
}

// --- installTo ---

func TestInstallTo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		binaryPath  string
		setupDstDir func(t *testing.T) string
		ctlErrOnN   int // 1-indexed call number to fail on; 0 = never fail
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "success: writes plist and calls unload then load",
			binaryPath:  "/path/to/ai-bridge",
			setupDstDir: func(t *testing.T) string { t.Helper(); return t.TempDir() },
		},
		{
			name:        "launchctl load error is returned",
			binaryPath:  "/path/to/ai-bridge",
			setupDstDir: func(t *testing.T) string { t.Helper(); return t.TempDir() },
			ctlErrOnN:   2,
			wantErr:     true,
			wantErrMsg:  "launchctl load",
		},
		{
			name: "MkdirAll error when dstDir is under a file",
			setupDstDir: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				if writeErr := os.WriteFile(blocker, []byte("x"), 0o644); writeErr != nil {
					t.Fatal(writeErr)
				}
				return filepath.Join(blocker, "sub")
			},
			wantErr: true,
		},
		{
			name: "WriteFile error on read-only directory",
			setupDstDir: func(t *testing.T) string {
				t.Helper()
				dir := filepath.Join(t.TempDir(), "readonly")
				if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
					t.Fatal(mkdirErr)
				}
				if chmodErr := os.Chmod(dir, 0o444); chmodErr != nil {
					t.Fatal(chmodErr)
				}
				t.Cleanup(func() {
					if chmodErr := os.Chmod(dir, 0o755); chmodErr != nil {
						t.Errorf("failed to restore dir permissions: %v", chmodErr)
					}
				})
				return dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var launchctlCalls [][]string
			callCount := 0
			fakeCtl := func(args ...string) error {
				callCount++
				cp := make([]string, len(args))
				copy(cp, args)
				launchctlCalls = append(launchctlCalls, cp)
				if tt.ctlErrOnN > 0 && callCount == tt.ctlErrOnN {
					return fmt.Errorf("launchctl load failed")
				}
				return nil
			}

			binaryPath := tt.binaryPath
			if binaryPath == "" {
				binaryPath = "/bin/test"
			}
			dstDir := tt.setupDstDir(t)

			err := installTo(binaryPath, "/usr/local/bin:/usr/bin:/bin", "claude", "wezterm", dstDir, fakeCtl)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			// Verify plist file was written with the binary path.
			dst := filepath.Join(dstDir, plistName)
			data, readErr := os.ReadFile(dst)
			if readErr != nil {
				t.Fatalf("plist not written: %v", readErr)
			}
			if !strings.Contains(string(data), binaryPath) {
				t.Error("plist missing binary path")
			}

			// Verify launchctl was called: unload then load.
			if len(launchctlCalls) != 2 {
				t.Fatalf("expected 2 launchctl calls, got %d", len(launchctlCalls))
			}
			if launchctlCalls[0][0] != "unload" {
				t.Errorf("first call = %q, want unload", launchctlCalls[0][0])
			}
			if launchctlCalls[1][0] != "load" {
				t.Errorf("second call = %q, want load", launchctlCalls[1][0])
			}
		})
	}
}
