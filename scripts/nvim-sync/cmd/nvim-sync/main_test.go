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
	t.Run("env overrides resolve to absolute src", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("NVIM_SYNC_SRC", dir)
		t.Setenv("NVIM_SYNC_CONTAINER", "custom-box")
		t.Setenv("NVIM_SYNC_DEST", "/custom/dest")

		s, err := newSyncer()
		if err != nil {
			t.Fatalf("newSyncer() error = %v", err)
		}
		if s.Container != "custom-box" {
			t.Errorf("Container = %q, want %q", s.Container, "custom-box")
		}
		if s.DestRoot != "/custom/dest" {
			t.Errorf("DestRoot = %q, want %q", s.DestRoot, "/custom/dest")
		}
		if !filepath.IsAbs(s.SrcRoot) {
			t.Errorf("SrcRoot = %q, want absolute", s.SrcRoot)
		}
		if s.Run == nil {
			t.Error("Run is nil, want defaultRunner")
		}
	})

	t.Run("defaults apply when env unset", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("NVIM_SYNC_SRC", dir)
		t.Setenv("NVIM_SYNC_CONTAINER", "")
		t.Setenv("NVIM_SYNC_DEST", "")

		s, err := newSyncer()
		if err != nil {
			t.Fatalf("newSyncer() error = %v", err)
		}
		if s.Container != defaultContainer {
			t.Errorf("Container = %q, want %q", s.Container, defaultContainer)
		}
		if s.DestRoot != defaultDest {
			t.Errorf("DestRoot = %q, want %q", s.DestRoot, defaultDest)
		}
	})

	t.Run("non-directory src is rejected", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("NVIM_SYNC_SRC", file)

		if _, err := newSyncer(); err == nil {
			t.Error("newSyncer() = nil error, want error for non-directory src")
		}
	})

	t.Run("missing src is rejected", func(t *testing.T) {
		t.Setenv("NVIM_SYNC_SRC", filepath.Join(t.TempDir(), "does-not-exist"))
		if _, err := newSyncer(); err == nil {
			t.Error("newSyncer() = nil error, want error for missing src")
		}
	})
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
	t.Run("copies only lua files recursively", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "init.lua"))
		mustWrite(t, filepath.Join(root, "notes.txt"))
		mustWrite(t, filepath.Join(root, "lua", "ai_bridge.lua"))

		rec := &recordingRunner{}
		s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: root, DestRoot: "/dest", Run: rec.run}
		if err := runSync(s); err != nil {
			t.Fatalf("runSync() error = %v", err)
		}
		want := []string{filepath.Join(root, "init.lua"), filepath.Join(root, "lua", "ai_bridge.lua")}
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

	t.Run("propagates copy failure", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "init.lua"))
		rec := &recordingRunner{err: errors.New("boom")}
		s := &syncer.Syncer{Container: "nvim-dev", SrcRoot: root, DestRoot: "/dest", Run: rec.run}
		if err := runSync(s); err == nil {
			t.Error("runSync() = nil, want error when copy fails")
		}
	})
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
