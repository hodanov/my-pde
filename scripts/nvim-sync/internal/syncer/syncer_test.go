package syncer

import (
	"errors"
	"sync"
	"testing"
)

// recordingRunner captures invocations for verification.
type recordingRunner struct {
	mu    sync.Mutex
	calls [][]string
	err   error
}

func (r *recordingRunner) run(name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	return r.err
}

func TestDest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		srcRoot  string
		destRoot string
		changed  string
		wantDest string
		wantOK   bool
	}{
		{
			name:     "top-level file",
			srcRoot:  "/repo/nvim/config",
			destRoot: "/root/.config/nvim",
			changed:  "/repo/nvim/config/init.lua",
			wantDest: "nvim-dev:/root/.config/nvim/init.lua",
			wantOK:   true,
		},
		{
			name:     "nested file",
			srcRoot:  "/repo/nvim/config",
			destRoot: "/root/.config/nvim",
			changed:  "/repo/nvim/config/lua/ai_bridge.lua",
			wantDest: "nvim-dev:/root/.config/nvim/lua/ai_bridge.lua",
			wantOK:   true,
		},
		{
			name:     "deeply nested file",
			srcRoot:  "/repo/nvim/config",
			destRoot: "/root/.config/nvim",
			changed:  "/repo/nvim/config/lua/lsp/init.lua",
			wantDest: "nvim-dev:/root/.config/nvim/lua/lsp/init.lua",
			wantOK:   true,
		},
		{
			name:     "outside src root",
			srcRoot:  "/repo/nvim/config",
			destRoot: "/root/.config/nvim",
			changed:  "/repo/other/file.lua",
			wantDest: "",
			wantOK:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Syncer{Container: "nvim-dev", SrcRoot: tt.srcRoot, DestRoot: tt.destRoot}
			got, ok := s.Dest(tt.changed)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantDest {
				t.Errorf("dest = %q, want %q", got, tt.wantDest)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		changed  string
		runErr   error
		wantArgs []string
		wantErr  bool
	}{
		{
			name:     "success runs docker cp",
			changed:  "/repo/nvim/config/lua/ai_bridge.lua",
			wantArgs: []string{"docker", "cp", "/repo/nvim/config/lua/ai_bridge.lua", "nvim-dev:/root/.config/nvim/lua/ai_bridge.lua"},
		},
		{
			name:     "runner error propagates",
			changed:  "/repo/nvim/config/init.lua",
			runErr:   errors.New("boom"),
			wantArgs: []string{"docker", "cp", "/repo/nvim/config/init.lua", "nvim-dev:/root/.config/nvim/init.lua"},
			wantErr:  true,
		},
		{
			name:    "outside src root is rejected without running",
			changed: "/elsewhere/x.lua",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingRunner{err: tt.runErr}
			s := &Syncer{
				Container: "nvim-dev",
				SrcRoot:   "/repo/nvim/config",
				DestRoot:  "/root/.config/nvim",
				Run:       rec.run,
			}
			copyErr := s.Copy(tt.changed)
			if (copyErr != nil) != tt.wantErr {
				t.Fatalf("Copy error = %v, wantErr %v", copyErr, tt.wantErr)
			}
			if tt.wantArgs == nil {
				if len(rec.calls) != 0 {
					t.Errorf("expected no docker invocation, got %v", rec.calls)
				}
				return
			}
			if len(rec.calls) != 1 {
				t.Fatalf("expected 1 invocation, got %d: %v", len(rec.calls), rec.calls)
			}
			got := rec.calls[0]
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", got, tt.wantArgs)
			}
			for i := range tt.wantArgs {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg %d = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}
