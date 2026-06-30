// Package watcher watches the bridge directory for incoming request files.
package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a directory for request.json using fsnotify and atomically consumes it.
type Watcher struct {
	dir string
}

// New creates a Watcher that monitors dir for "request.json" using filesystem events.
func New(dir string) *Watcher {
	return &Watcher{dir: dir}
}

// isRequestEvent returns true if the event targets request.json with a Create or Write op.
func isRequestEvent(event fsnotify.Event) bool {
	if filepath.Base(event.Name) != "request.json" {
		return false
	}
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write)
}

// Watch monitors for request.json using fsnotify. When found, it atomically renames
// the file and sends the consumed path on the returned channel. The channel is closed
// when ctx is cancelled. Returns an error if the watcher cannot be initialized.
func (w *Watcher) Watch(ctx context.Context) (<-chan string, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify: create watcher: %w", err)
	}

	if addErr := fsw.Add(w.dir); addErr != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("fsnotify: watch dir %s: %w", w.dir, addErr)
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		defer func() { _ = fsw.Close() }()

		// Check for existing request.json before entering event loop.
		if consumed, ok := w.tryConsume(); ok {
			select {
			case ch <- consumed:
			case <-ctx.Done():
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-fsw.Events:
				if !ok {
					return
				}
				if !isRequestEvent(event) {
					continue
				}
				consumed, found := w.tryConsume()
				if !found {
					continue
				}
				select {
				case ch <- consumed:
				case <-ctx.Done():
					return
				}
			case fsErr, ok := <-fsw.Errors:
				if !ok {
					return
				}
				slog.Warn("fsnotify: error", "error", fsErr)
			}
		}
	}()
	return ch, nil
}

// tryConsume checks for request.json and atomically renames it.
// Returns the consumed path and true on success.
func (w *Watcher) tryConsume() (string, bool) {
	path := filepath.Join(w.dir, "request.json")
	if _, statErr := os.Stat(path); statErr != nil {
		return "", false
	}
	consumed := fmt.Sprintf("%s.%d.%d.consumed", path, time.Now().Unix(), rand.Intn(100000))
	if renameErr := os.Rename(path, consumed); renameErr != nil {
		slog.Debug("consume failed (likely race)", "error", renameErr)
		return "", false
	}
	return consumed, true
}
