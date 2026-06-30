package launcher

import (
	"reflect"
	"testing"
)

func TestTmux_Args(t *testing.T) {
	t.Parallel()
	tm := &Tmux{}
	tests := []struct {
		name       string
		cwd        string
		scriptPath string
		want       []string
	}{
		{
			name:       "basic",
			cwd:        "/home/user/project",
			scriptPath: "/tmp/ai-bridge-12345.sh",
			want:       []string{"new-window", "-c", "/home/user/project", "bash '/tmp/ai-bridge-12345.sh'"},
		},
		{
			name:       "path with single quote",
			cwd:        "/tmp",
			scriptPath: "/tmp/ai-bridge-it's.sh",
			want:       []string{"new-window", "-c", "/tmp", "bash '/tmp/ai-bridge-it'\"'\"'s.sh'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tm.args(tt.cwd, tt.scriptPath)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Args() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTmux_Launch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		cwd        string
		scriptPath string
		wantName   string
		wantArgs   []string
	}{
		{
			name:       "basic launch passes correct command and args",
			cwd:        "/tmp",
			scriptPath: "/tmp/script.sh",
			wantName:   "tmux",
			wantArgs:   []string{"new-window", "-c", "/tmp", "bash '/tmp/script.sh'"},
		},
		{
			name:       "path with single quote is shell-quoted correctly",
			cwd:        "/home/user/project",
			scriptPath: "/tmp/ai-bridge-it's.sh",
			wantName:   "tmux",
			wantArgs:   []string{"new-window", "-c", "/home/user/project", "bash '/tmp/ai-bridge-it'\"'\"'s.sh'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calledName string
			var calledArgs []string
			runner := func(name string, args ...string) error {
				calledName = name
				calledArgs = args
				return nil
			}

			tm := &Tmux{run: runner}
			if launchErr := tm.Launch(tt.cwd, tt.scriptPath); launchErr != nil {
				t.Fatal(launchErr)
			}

			if calledName != tt.wantName {
				t.Errorf("command = %q, want %q", calledName, tt.wantName)
			}
			if !reflect.DeepEqual(calledArgs, tt.wantArgs) {
				t.Errorf("args = %v, want %v", calledArgs, tt.wantArgs)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	noop := func(string, ...string) error { return nil }

	tests := []struct {
		name         string
		launcherName string
		wantType     string // "wezterm" | "tmux" | "" for error cases
		wantErr      bool
	}{
		{
			name:         "wezterm returns *WezTerm launcher",
			launcherName: "wezterm",
			wantType:     "wezterm",
		},
		{
			name:         "tmux returns *Tmux launcher",
			launcherName: "tmux",
			wantType:     "tmux",
		},
		{
			name:         "unknown name returns error",
			launcherName: "unknown",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, err := New(tt.launcherName, noop)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch tt.wantType {
			case "wezterm":
				if _, ok := l.(*WezTerm); !ok {
					t.Errorf("expected *WezTerm, got %T", l)
				}
			case "tmux":
				if _, ok := l.(*Tmux); !ok {
					t.Errorf("expected *Tmux, got %T", l)
				}
			}
		})
	}
}
