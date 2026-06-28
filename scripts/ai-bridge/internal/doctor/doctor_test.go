package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-bridge/internal/daemon"
)

func TestStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "ok", status: StatusOK, want: "ok"},
		{name: "warn", status: StatusWarn, want: "warn"},
		{name: "fail", status: StatusFail, want: "fail"},
		{name: "unknown", status: Status(99), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckExecutable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		lookPath   LookPathFunc
		wantStatus Status
	}{
		{
			name:       "found",
			lookPath:   func(file string) (string, error) { return "/usr/bin/" + file, nil },
			wantStatus: StatusOK,
		},
		{
			name:       "missing",
			lookPath:   func(string) (string, error) { return "", errors.New("not found") },
			wantStatus: StatusFail,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkExecutable("CLI", "claude", tt.lookPath)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.Name != "CLI" {
				t.Errorf("name = %q, want CLI", got.Name)
			}
		})
	}
}

func TestCheckBridgeDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "afile")
	if writeErr := os.WriteFile(filePath, []byte("x"), 0o600); writeErr != nil {
		t.Fatalf("setup: %v", writeErr)
	}
	tests := []struct {
		name       string
		path       string
		wantStatus Status
	}{
		{name: "writable dir", path: dir, wantStatus: StatusOK},
		{name: "missing", path: filepath.Join(dir, "does-not-exist"), wantStatus: StatusFail},
		{name: "not a dir", path: filePath, wantStatus: StatusFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkBridgeDir(tt.path)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %v (%s), want %v", got.Status, got.Detail, tt.wantStatus)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name        string
		cfg         *daemon.Config
		lookPath    LookPathFunc
		wantStatus  []Status
		wantFailure bool
	}{
		{
			name:        "all ok",
			cfg:         &daemon.Config{BridgeDir: dir, CLI: "claude", Launcher: "wezterm"},
			lookPath:    func(file string) (string, error) { return "/usr/bin/" + file, nil },
			wantStatus:  []Status{StatusOK, StatusOK, StatusOK},
			wantFailure: false,
		},
		{
			name:        "missing launcher",
			cfg:         &daemon.Config{BridgeDir: dir, CLI: "claude", Launcher: "wezterm"},
			lookPath:    func(file string) (string, error) { return "", errors.New("nope") },
			wantStatus:  []Status{StatusOK, StatusFail, StatusFail},
			wantFailure: true,
		},
		{
			name:        "missing bridge dir",
			cfg:         &daemon.Config{BridgeDir: filepath.Join(dir, "nope"), CLI: "claude", Launcher: "tmux"},
			lookPath:    func(file string) (string, error) { return "/usr/bin/" + file, nil },
			wantStatus:  []Status{StatusFail, StatusOK, StatusOK},
			wantFailure: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checks := Run(tt.cfg, tt.lookPath)
			if len(checks) != len(tt.wantStatus) {
				t.Fatalf("got %d checks, want %d", len(checks), len(tt.wantStatus))
			}
			for i, want := range tt.wantStatus {
				if checks[i].Status != want {
					t.Errorf("check %d (%s) status = %v, want %v", i, checks[i].Name, checks[i].Status, want)
				}
			}
			if got := HasFailure(checks); got != tt.wantFailure {
				t.Errorf("HasFailure() = %v, want %v", got, tt.wantFailure)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()
	checks := []Check{
		{Name: "BridgeDir", Status: StatusOK, Detail: "/home/x/.ai-bridge"},
		{Name: "CLI", Status: StatusFail, Detail: `"claude" not found`},
	}
	out := Format(checks)
	for _, want := range []string{"ai-bridge doctor", "BridgeDir", "[ok", "CLI", "[fail"} {
		if !strings.Contains(out, want) {
			t.Errorf("Format() output missing %q:\n%s", want, out)
		}
	}
}
