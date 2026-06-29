package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nvim-sync/internal/syncer"
)

// recordingRunner captures docker invocations so tests stay off real Docker.
type recordingRunner struct {
	mu    sync.Mutex
	calls [][]string
	err   error
}

func (r *recordingRunner) run(name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.err
}

func (r *recordingRunner) sources() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var srcs []string
	for _, c := range r.calls {
		// docker cp <src> <dest>
		srcs = append(srcs, c[2])
	}
	sort.Strings(srcs)
	return srcs
}

func TestGetenv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "returns default when unset", value: "", want: "fallback"},
		{name: "returns value when set", value: "explicit", want: "explicit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NVIM_SYNC_TEST_KEY", tt.value)
			if got := getenv("NVIM_SYNC_TEST_KEY", "fallback"); got != tt.want {
				t.Errorf("getenv = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	usage(&buf)
	out := buf.String()
	for _, want := range []string{"Usage: nvim-sync", "watch", "sync", "NVIM_SYNC_CONTAINER"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage output missing %q\n%s", want, out)
		}
	}
}

func TestDefaultRunner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{name: "successful command", command: "true", wantErr: false},
		{name: "failing command", command: "false", wantErr: true},
		{name: "missing binary", command: "nvim-sync-no-such-binary", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := defaultRunner(tt.command); (err != nil) != tt.wantErr {
				t.Errorf("defaultRunner(%q) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestNewSyncer(t *testing.T) {
	// srcKind selects how the NVIM_SYNC_SRC path is materialised per case.
	tests := []struct {
		name          string
		srcKind       string // "dir", "file", or "missing"
		container     string
		dest          string
		wantErr       bool
		wantContainer string
		wantDest      string
	}{
		{
			name:          "env overrides resolve to absolute src",
			srcKind:       "dir",
			container:     "custom-box",
			dest:          "/custom/dest",
			wantContainer: "custom-box",
			wantDest:      "/custom/dest",
		},
		{
			name:          "defaults apply when env unset",
			srcKind:       "dir",
			container:     "",
			dest:          "",
			wantContainer: defaultContainer,
			wantDest:      defaultDest,
		},
		{
			name:    "non-directory src is rejected",
			srcKind: "file",
			wantErr: true,
		},
		{
			name:    "missing src is rejected",
			srcKind: "missing",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src string
			switch tt.srcKind {
			case "dir":
				src = t.TempDir()
			case "file":
				src = filepath.Join(t.TempDir(), "not-a-dir")
				if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				src = filepath.Join(t.TempDir(), "does-not-exist")
			}
			t.Setenv("NVIM_SYNC_SRC", src)
			t.Setenv("NVIM_SYNC_CONTAINER", tt.container)
			t.Setenv("NVIM_SYNC_DEST", tt.dest)

			s, err := newSyncer()
			if (err != nil) != tt.wantErr {
				t.Fatalf("newSyncer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if s.Container != tt.wantContainer {
				t.Errorf("Container = %q, want %q", s.Container, tt.wantContainer)
			}
			if s.DestRoot != tt.wantDest {
				t.Errorf("DestRoot = %q, want %q", s.DestRoot, tt.wantDest)
			}
			if !filepath.IsAbs(s.SrcRoot) {
				t.Errorf("SrcRoot = %q, want absolute", s.SrcRoot)
			}
			if s.Run == nil {
				t.Error("Run is nil, want defaultRunner")
			}
		})
	}
}

func TestSyncBatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		runErr error
	}{
		{name: "copies every file on success", runErr: nil},
		// Should not panic and should attempt both files despite errors.
		{name: "continues past a failing copy", runErr: errors.New("boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingRunner{err: tt.runErr}
			s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: "/src", DestRoot: "/dest", Run: rec.run}
			syncBatch(s, []string{"/src/a.lua", "/src/b.lua"})
			if got := len(rec.calls); got != 2 {
				t.Fatalf("calls = %d, want 2 (must attempt every file)", got)
			}
		})
	}
}

func TestRunSync(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		files       []string // empty files created under the temp root
		runErr      error
		wantErr     bool
		wantSources []string // paths relative to root; checked only when wantErr is false
	}{
		{
			name:        "copies only lua files recursively",
			files:       []string{"init.lua", "notes.txt", filepath.Join("lua", "ai_bridge.lua")},
			wantSources: []string{"init.lua", filepath.Join("lua", "ai_bridge.lua")},
		},
		{
			name:    "propagates copy failure",
			files:   []string{"init.lua"},
			runErr:  errors.New("boom"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, f := range tt.files {
				mustWrite(t, filepath.Join(root, f))
			}
			rec := &recordingRunner{err: tt.runErr}
			s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: root, DestRoot: "/dest", Run: rec.run}
			err := runSync(s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("runSync() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := make([]string, len(tt.wantSources))
			for i, src := range tt.wantSources {
				want[i] = filepath.Join(root, src)
			}
			sort.Strings(want)
			got := rec.sources()
			if len(got) != len(want) {
				t.Fatalf("copied %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("copied[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestWatchLoop(t *testing.T) {
	t.Parallel()
	t.Run("returns nil when context is cancelled", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := &recordingRunner{}
		s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: root, DestRoot: "/dest", Run: rec.run}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- watchLoop(ctx, s) }()

		// Give the watcher a moment to start, then cancel.
		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("watchLoop() = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("watchLoop did not return after cancel")
		}
	})

	t.Run("syncs a file written while watching", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := &recordingRunner{}
		s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: root, DestRoot: "/dest", Run: rec.run}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- watchLoop(ctx, s) }()

		time.Sleep(50 * time.Millisecond)
		mustWrite(t, filepath.Join(root, "init.lua"))

		// Wait for the debounced batch to be copied.
		deadline := time.After(3 * time.Second)
		for len(rec.sources()) == 0 {
			select {
			case <-deadline:
				t.Fatal("file write was not synced")
			case <-time.After(20 * time.Millisecond):
			}
		}
		cancel()
		<-done
	})

	t.Run("errors when src root is missing", func(t *testing.T) {
		t.Parallel()
		s := &syncer.Syncer{SrcRoot: filepath.Join(t.TempDir(), "missing")}
		if err := watchLoop(context.Background(), s); err == nil {
			t.Error("watchLoop() = nil, want error for missing src root")
		}
	})
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// src resolves the NVIM_SYNC_SRC value for the case; nil leaves it unset.
		src          func(t *testing.T) string
		wantUsageErr bool // err must be errUsage and usage must be printed
		wantErr      bool // err must be non-nil
	}{
		{
			name:         "no args prints usage and errors",
			args:         nil,
			wantUsageErr: true,
		},
		{
			name:         "unknown command prints usage and errors",
			args:         []string{"bogus"},
			wantUsageErr: true,
		},
		{
			name: "sync over empty dir succeeds without docker",
			args: []string{"sync"},
			// Empty dir => no .lua files => runSync makes no docker calls.
			src: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name:    "sync with invalid src errors",
			args:    []string{"sync"},
			src:     func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.src != nil {
				t.Setenv("NVIM_SYNC_SRC", tt.src(t))
			}
			var buf bytes.Buffer
			err := run(tt.args, &buf)
			switch {
			case tt.wantUsageErr:
				if !errors.Is(err, errUsage) {
					t.Errorf("run(%v) error = %v, want errUsage", tt.args, err)
				}
				if !strings.Contains(buf.String(), "Usage") {
					t.Errorf("run(%v) did not print usage", tt.args)
				}
			case tt.wantErr:
				if err == nil {
					t.Errorf("run(%v) = nil, want error", tt.args)
				}
			default:
				if err != nil {
					t.Errorf("run(%v) error = %v, want nil", tt.args, err)
				}
			}
		})
	}
}

// mustWrite creates an empty file, making parent dirs as needed.
func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
}
