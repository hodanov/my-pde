// Package watcher recursively watches a Neovim config tree and emits debounced
// batches of changed Lua file paths.
package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher recursively watches root for Lua file changes.
type Watcher struct {
	root     string
	debounce time.Duration
}

// New creates a Watcher for root with the given debounce window.
func New(root string, debounce time.Duration) *Watcher {
	return &Watcher{root: root, debounce: debounce}
}

// isLuaFile reports whether name is a Lua source file.
func isLuaFile(name string) bool {
	return filepath.Ext(name) == ".lua"
}

// batch coalesces changed paths into a unique, sorted set so a burst of saves
// collapses into a single sync.
type batch struct {
	mu    sync.Mutex
	items map[string]struct{}
}

func newBatch() *batch {
	return &batch{items: make(map[string]struct{})}
}

func (b *batch) add(p string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[p] = struct{}{}
}

// drain returns the accumulated paths sorted and clears the set.
func (b *batch) drain() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == 0 {
		return nil
	}
	out := make([]string, 0, len(b.items))
	for p := range b.items {
		out = append(out, p)
	}
	b.items = make(map[string]struct{})
	sort.Strings(out)
	return out
}

// addRecursive registers root and every subdirectory with fsw.
func addRecursive(fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if addErr := fsw.Add(p); addErr != nil {
				return fmt.Errorf("watch %s: %w", p, addErr)
			}
		}
		return nil
	})
}

// Watch recursively watches the tree and sends debounced batches of changed Lua
// file paths until ctx is cancelled. The channel is closed on cancellation.
func (w *Watcher) Watch(ctx context.Context) (<-chan []string, error) {
	fsw, newErr := fsnotify.NewWatcher()
	if newErr != nil {
		return nil, fmt.Errorf("create watcher: %w", newErr)
	}
	if addErr := addRecursive(fsw, w.root); addErr != nil {
		_ = fsw.Close()
		return nil, addErr
	}

	out := make(chan []string)
	b := newBatch()
	go func() {
		defer close(out)
		defer func() { _ = fsw.Close() }()

		timer := time.NewTimer(w.debounce)
		if !timer.Stop() {
			<-timer.C
		}

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-fsw.Events:
				if !ok {
					return
				}
				w.handleEvent(fsw, event, b, timer)
			case <-timer.C:
				if changed := b.drain(); len(changed) > 0 {
					select {
					case out <- changed:
					case <-ctx.Done():
						return
					}
				}
			case fsErr, ok := <-fsw.Errors:
				if !ok {
					return
				}
				slog.Warn("watch error", "error", fsErr)
			}
		}
	}()
	return out, nil
}

// handleEvent records relevant changes and (re)arms the debounce timer.
func (w *Watcher) handleEvent(fsw *fsnotify.Watcher, event fsnotify.Event, b *batch, timer *time.Timer) {
	if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
		return
	}
	// Watch directories created after startup so their files are not missed.
	if event.Has(fsnotify.Create) {
		if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
			if addErr := addRecursive(fsw, event.Name); addErr != nil {
				slog.Warn("watch new dir failed", "dir", event.Name, "error", addErr)
			}
			return
		}
	}
	if !isLuaFile(event.Name) {
		return
	}
	b.add(event.Name)
	timer.Reset(w.debounce)
}
