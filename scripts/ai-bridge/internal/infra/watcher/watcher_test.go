package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_ConsumesRequest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	// Wait for the goroutine to complete its startup check and enter the event loop.
	time.Sleep(100 * time.Millisecond)

	// Write a request file — this must be consumed via the fsnotify event loop.
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, []byte(`{"prompt":"hi"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case consumed := <-ch:
		if consumed == "" {
			t.Fatal("consumed path is empty")
		}
		// The original file should no longer exist.
		if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
			t.Error("request.json should be renamed after consume")
		}
		// The consumed file should exist.
		if _, err := os.Stat(consumed); err != nil {
			t.Errorf("consumed file should exist: %v", err)
		}
		// Clean up consumed file.
		_ = os.Remove(consumed)
	case <-ctx.Done():
		t.Fatal("timed out waiting for consumed request")
	}
}

func TestWatch_NoDuplicateConsume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	// Write one request file.
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, []byte(`{"prompt":"once"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Collect all consumed events.
	var consumed []string
	done := make(chan struct{})
	go func() {
		for path := range ch {
			consumed = append(consumed, path)
			_ = os.Remove(path)
		}
		close(done)
	}()

	<-done

	if len(consumed) != 1 {
		t.Errorf("expected exactly 1 consume event, got %d", len(consumed))
	}
}

func TestWatch_StopsOnCancel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithCancel(context.Background())
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	cancel()

	// Channel should be closed after cancel.
	for range ch {
		// drain
	}
}

func TestWatch_CancelDuringBlockedSend(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithCancel(context.Background())
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	// Wait for the goroutine to complete its startup check and enter the event loop.
	time.Sleep(100 * time.Millisecond)

	// Write a request so the watcher consumes it via the event loop and tries to send on ch.
	if writeErr := os.WriteFile(filepath.Join(dir, "request.json"), []byte(`{}`), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Wait for watcher to detect, consume, and block on unbuffered ch send.
	time.Sleep(200 * time.Millisecond)

	// Cancel while send is blocked — should take the inner ctx.Done() path.
	cancel()

	// Channel should be closed.
	for range ch {
	}
}

func TestWatch_NoFileNoPanic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	// No file written — should just close without events.
	for range ch {
		t.Error("unexpected event when no file exists")
	}
}

func TestWatch_InvalidDirReturnsError(t *testing.T) {
	t.Parallel()
	w := New("/nonexistent/dir/that/does/not/exist")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := w.Watch(ctx)
	if err == nil {
		t.Fatal("expected error for invalid directory, got nil")
	}
}

func TestTryConsume_RenameError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	// Create request.json.
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make dir read-only so rename fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	consumed, ok := w.tryConsume()
	if ok {
		t.Errorf("expected tryConsume to fail, got %s", consumed)
	}
}

func TestWatch_IgnoresNonRequestFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := New(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	// Wait for the goroutine to enter the event loop.
	time.Sleep(100 * time.Millisecond)

	// Write a non-request file — should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// No events should be received before timeout.
	for range ch {
		t.Error("unexpected event for non-request file")
	}
}

func TestWatch_ExistingFileConsumedOnStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write request.json before starting the watcher.
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, []byte(`{"prompt":"pre-existing"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	w := New(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		t.Fatal(watchErr)
	}

	select {
	case consumed := <-ch:
		if consumed == "" {
			t.Fatal("consumed path is empty")
		}
		_ = os.Remove(consumed)
	case <-ctx.Done():
		t.Fatal("timed out waiting for pre-existing request to be consumed")
	}
}
