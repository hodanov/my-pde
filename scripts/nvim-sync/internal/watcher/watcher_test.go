package watcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestIsLuaFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "lua file", path: "/x/init.lua", want: true},
		{name: "nested lua", path: "/x/lua/lsp/init.lua", want: true},
		{name: "non-lua", path: "/x/notes.txt", want: false},
		{name: "no extension", path: "/x/Makefile", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isLuaFile(tt.path); got != tt.want {
				t.Errorf("isLuaFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBatchDrain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		adds []string
		want []string
	}{
		{name: "empty", adds: nil, want: nil},
		{name: "single", adds: []string{"/a.lua"}, want: []string{"/a.lua"}},
		{
			name: "dedup and sort",
			adds: []string{"/b.lua", "/a.lua", "/b.lua", "/a.lua"},
			want: []string{"/a.lua", "/b.lua"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBatch()
			for _, p := range tt.adds {
				b.add(p)
			}
			got := b.drain()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("drain() = %v, want %v", got, tt.want)
			}
			// A second drain after the first must be empty.
			if again := b.drain(); again != nil {
				t.Errorf("second drain() = %v, want nil", again)
			}
		})
	}
}

func TestWatchEmitsChangedLuaFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if mkErr := os.MkdirAll(filepath.Join(root, "lua"), 0o755); mkErr != nil {
		t.Fatalf("setup: %v", mkErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(root, 50*time.Millisecond)
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatalf("Watch: %v", watchErr)
	}

	target := filepath.Join(root, "lua", "ai_bridge.lua")
	if writeErr := os.WriteFile(target, []byte("-- x"), 0o644); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	select {
	case batch := <-ch:
		found := false
		for _, p := range batch {
			if p == target {
				found = true
			}
		}
		if !found {
			t.Errorf("batch %v does not contain %q", batch, target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for change batch")
	}
}

func TestWatchIgnoresNonLua(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(root, 50*time.Millisecond)
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatalf("Watch: %v", watchErr)
	}

	if writeErr := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	select {
	case batch := <-ch:
		t.Errorf("unexpected batch for non-lua file: %v", batch)
	case <-time.After(300 * time.Millisecond):
		// expected: no emission
	}
}
