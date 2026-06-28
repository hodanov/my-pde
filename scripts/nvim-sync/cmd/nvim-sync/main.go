// Command nvim-sync mirrors host-side Neovim config changes into a running
// nvim container, removing the manual `docker cp` step from the config edit
// loop documented in scripts/ai-bridge/README.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nvim-sync/internal/syncer"
	"nvim-sync/internal/watcher"
)

const (
	defaultContainer = "nvim-dev"
	defaultSrc       = "nvim/config"
	defaultDest      = "/root/.config/nvim"
	debounceWindow   = 300 * time.Millisecond
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "watch":
		err = runWatch()
	case "sync":
		err = runSync()
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: nvim-sync <command>")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  watch   Watch nvim config and docker cp changes into the container")
	fmt.Fprintln(os.Stderr, "  sync    Copy all nvim config files into the container once")
	fmt.Fprintln(os.Stderr, "Environment:")
	fmt.Fprintln(os.Stderr, "  NVIM_SYNC_CONTAINER  target container (default nvim-dev)")
	fmt.Fprintln(os.Stderr, "  NVIM_SYNC_SRC        host config dir (default nvim/config)")
	fmt.Fprintln(os.Stderr, "  NVIM_SYNC_DEST       container config dir (default /root/.config/nvim)")
}

// getenv returns the env var value or def when unset/empty.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// newSyncer builds a Syncer from the environment, resolving SrcRoot to an
// absolute path so relative-path computation is stable.
func newSyncer() (*syncer.Syncer, error) {
	src := getenv("NVIM_SYNC_SRC", defaultSrc)
	absSrc, absErr := filepath.Abs(src)
	if absErr != nil {
		return nil, fmt.Errorf("resolve src %q: %w", src, absErr)
	}
	if info, statErr := os.Stat(absSrc); statErr != nil || !info.IsDir() {
		return nil, fmt.Errorf("src is not a directory: %s", absSrc)
	}
	return &syncer.Syncer{
		Container: getenv("NVIM_SYNC_CONTAINER", defaultContainer),
		SrcRoot:   absSrc,
		DestRoot:  getenv("NVIM_SYNC_DEST", defaultDest),
		Run:       defaultRunner,
	}, nil
}

// defaultRunner executes a command, surfacing combined output on failure.
func defaultRunner(name string, args ...string) error {
	out, runErr := exec.Command(name, args...).CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("%w: %s", runErr, string(out))
	}
	return nil
}

func runWatch() error {
	s, syncerErr := newSyncer()
	if syncerErr != nil {
		return syncerErr
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	w := watcher.New(s.SrcRoot, debounceWindow)
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		return fmt.Errorf("start watcher: %w", watchErr)
	}

	slog.Info("nvim-sync: watching", "container", s.Container, "src", s.SrcRoot, "dest", s.DestRoot)
	for changed := range ch {
		for _, file := range changed {
			if copyErr := s.Copy(file); copyErr != nil {
				slog.Warn("sync failed", "file", file, "error", copyErr)
				continue
			}
			slog.Info("synced", "file", file)
		}
	}
	return nil
}

func runSync() error {
	s, syncerErr := newSyncer()
	if syncerErr != nil {
		return syncerErr
	}

	slog.Info("nvim-sync: initial sync", "container", s.Container, "src", s.SrcRoot, "dest", s.DestRoot)
	count := 0
	walkErr := filepath.WalkDir(s.SrcRoot, func(p string, d os.DirEntry, walkDirErr error) error {
		if walkDirErr != nil {
			return walkDirErr
		}
		if d.IsDir() || filepath.Ext(p) != ".lua" {
			return nil
		}
		if copyErr := s.Copy(p); copyErr != nil {
			return copyErr
		}
		slog.Info("synced", "file", p)
		count++
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("sync: %w", walkErr)
	}
	slog.Info("nvim-sync: done", "files", count)
	return nil
}
