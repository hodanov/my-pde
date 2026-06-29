package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-bridge/internal/launcher"
	"ai-bridge/internal/testutil"
)

// --- LoadConfig ---

func TestLoadConfig(t *testing.T) {
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

	// NOTE: サブテストには t.Parallel() を付けない。
	// t.Setenv はパラレルテストで使用できないため。
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AI_BRIDGE_DIR", tt.envDir)
			t.Setenv("AI_BRIDGE_CLI", tt.envCLI)
			t.Setenv("AI_BRIDGE_LAUNCHER", tt.envLauncher)

			cfg, err := LoadConfig()

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

// --- parseRequest ---

func TestParseRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		content       string
		noFile        bool
		wantPrompt    string
		wantCWD       string
		wantTimestamp int64
		wantErr       bool
	}{
		{
			name:          "valid request returns parsed fields",
			content:       `{"prompt":"hello","cwd":"/tmp","timestamp":1234}`,
			wantPrompt:    "hello",
			wantCWD:       "/tmp",
			wantTimestamp: 1234,
		},
		{
			name:    "missing prompt returns error",
			content: `{"cwd":"/tmp","timestamp":1234}`,
			wantErr: true,
		},
		{
			name:    "missing cwd returns error",
			content: `{"prompt":"hello","timestamp":1234}`,
			wantErr: true,
		},
		{
			name:    "empty prompt returns error",
			content: `{"prompt":"","cwd":"/tmp"}`,
			wantErr: true,
		},
		{
			name:    "empty cwd returns error",
			content: `{"prompt":"hello","cwd":""}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON returns error",
			content: `not json`,
			wantErr: true,
		},
		{
			name:    "file not found returns error",
			noFile:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reqFile string
			if tt.noFile {
				reqFile = "/nonexistent/request.json"
			} else {
				dir := t.TempDir()
				reqFile = filepath.Join(dir, "request.json")
				if writeErr := os.WriteFile(reqFile, []byte(tt.content), 0o644); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			req, err := parseRequest(reqFile)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if req.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tt.wantPrompt)
			}
			if req.CWD != tt.wantCWD {
				t.Errorf("CWD = %q, want %q", req.CWD, tt.wantCWD)
			}
			if req.Timestamp != tt.wantTimestamp {
				t.Errorf("Timestamp = %d, want %d", req.Timestamp, tt.wantTimestamp)
			}
		})
	}
}

// --- generateScript ---

func TestGenerateScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cli          string
		prompt       string
		wantContains []string
	}{
		{
			name:   "basic script has shebang, cli, prompt and self-delete",
			cli:    "claude",
			prompt: "hello world",
			wantContains: []string{
				"#!/bin/bash\n",
				"claude",
				"hello world",
				"rm -f",
			},
		},
		{
			name:   "special chars in prompt are safely quoted",
			cli:    "claude",
			prompt: `it's a "test" with $vars`,
			wantContains: []string{
				"it",
				"$vars",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, genScriptErr := generateScript(tt.cli, tt.prompt)
			if genScriptErr != nil {
				t.Fatal(genScriptErr)
			}
			defer func() { _ = os.Remove(path) }()

			data, _ := os.ReadFile(path)
			content := string(data)

			for _, want := range tt.wantContains {
				if !strings.Contains(content, want) {
					t.Errorf("script missing %q\ncontent:\n%s", want, content)
				}
			}

			info, _ := os.Stat(path)
			if info.Mode().Perm()&0o100 == 0 {
				t.Error("script should be executable")
			}
		})
	}
}

// --- shellQuote ---

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"it's", "'it'\"'\"'s'"},
		{"hello world", "'hello world'"},
		{"$var", "'$var'"},
		{"", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Run (daemon loop integration) ---

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		setupBridgeDir    func(t *testing.T) string
		makeReq           func(cwd string) string
		stubErr           error
		cancelImmediately bool
		wantRunErr        bool
		wantLaunches      int
	}{
		{
			name: "valid request triggers launch",
			makeReq: func(cwd string) string {
				return fmt.Sprintf(`{"prompt":"test prompt","cwd":%q,"timestamp":1234}`, cwd)
			},
			wantLaunches: 1,
		},
		{
			name:         "invalid JSON request is skipped",
			makeReq:      func(_ string) string { return `{"cwd":"/tmp"}` },
			wantLaunches: 0,
		},
		{
			name:         "nonexistent cwd is skipped",
			makeReq:      func(_ string) string { return `{"prompt":"hi","cwd":"/nonexistent/dir"}` },
			wantLaunches: 0,
		},
		{
			name: "launcher error is logged but does not crash",
			makeReq: func(cwd string) string {
				return fmt.Sprintf(`{"prompt":"test","cwd":%q,"timestamp":1234}`, cwd)
			},
			stubErr:      fmt.Errorf("launcher failed"),
			wantLaunches: 1,
		},
		{
			name: "MkdirAll error returns error immediately",
			setupBridgeDir: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				if writeErr := os.WriteFile(blocker, []byte("x"), 0o644); writeErr != nil {
					t.Fatal(writeErr)
				}
				return filepath.Join(blocker, "sub")
			},
			cancelImmediately: true,
			wantRunErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var dir string
			if tt.setupBridgeDir != nil {
				dir = tt.setupBridgeDir(t)
			} else {
				dir = t.TempDir()
			}
			cwd := t.TempDir()

			stub := testutil.NewExecStub(tt.stubErr)
			l, _ := launcher.New("wezterm", stub.Run)

			cfg := &Config{
				BridgeDir: dir,
				CLI:       "claude",
				Launcher:  "wezterm",
			}

			ctx, cancel := context.WithCancel(context.Background())

			if tt.cancelImmediately {
				cancel()
			} else {
				go func() {
					time.Sleep(200 * time.Millisecond)
					if tt.makeReq != nil {
						if writeErr := os.WriteFile(filepath.Join(dir, "request.json"), []byte(tt.makeReq(cwd)), 0o644); writeErr != nil {
							t.Error(writeErr)
						}
					}
					time.Sleep(1 * time.Second)
					cancel()
				}()
			}

			err := Run(ctx, cfg, l)

			if tt.wantRunErr {
				if err == nil {
					t.Fatal("expected error from Run, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := len(stub.Records()); got != tt.wantLaunches {
				t.Errorf("launch count = %d, want %d", got, tt.wantLaunches)
			}
		})
	}
}
