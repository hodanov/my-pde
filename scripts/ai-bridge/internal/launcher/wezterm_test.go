package launcher

import (
	"reflect"
	"testing"
)

func TestWezTerm_Args(t *testing.T) {
	t.Parallel()
	w := &WezTerm{}
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
			want:       []string{"cli", "spawn", "--cwd", "/home/user/project", "--", "bash", "/tmp/ai-bridge-12345.sh"},
		},
		{
			name:       "path with spaces",
			cwd:        "/home/user/my project",
			scriptPath: "/tmp/ai-bridge-99999.sh",
			want:       []string{"cli", "spawn", "--cwd", "/home/user/my project", "--", "bash", "/tmp/ai-bridge-99999.sh"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := w.args(tt.cwd, tt.scriptPath)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Args() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWezTerm_Launch(t *testing.T) {
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
			wantName:   "wezterm",
			wantArgs:   []string{"cli", "spawn", "--cwd", "/tmp", "--", "bash", "/tmp/script.sh"},
		},
		{
			name:       "path with spaces is passed correctly",
			cwd:        "/home/user/my project",
			scriptPath: "/tmp/ai-bridge-99999.sh",
			wantName:   "wezterm",
			wantArgs:   []string{"cli", "spawn", "--cwd", "/home/user/my project", "--", "bash", "/tmp/ai-bridge-99999.sh"},
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

			w := &WezTerm{run: runner}
			if launchErr := w.Launch(tt.cwd, tt.scriptPath); launchErr != nil {
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
