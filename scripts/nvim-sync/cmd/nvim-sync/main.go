// Command nvim-sync mirrors host-side Neovim config changes into a running
// nvim container, removing the manual `docker cp` step from the config edit
// loop documented in scripts/ai-bridge/README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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

	if err := run(os.Args[1:], os.Stderr); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// errUsage signals that the command line was invalid and usage was printed.
var errUsage = errors.New("invalid command")

// run dispatches a subcommand and returns an error instead of exiting so it can
// be exercised from tests. main() is the only place that calls os.Exit.
func run(args []string, stderr io.Writer) error {
	if len(args) < 1 {
		usage(stderr)
		return errUsage
	}

	switch args[0] {
	case "watch":
		s, err := newSyncer()
		if err != nil {
			return err
		}
		return runWatch(s)
	case "sync":
		s, err := newSyncer()
		if err != nil {
			return err
		}
		return runSync(s)
	default:
		usage(stderr)
		return errUsage
	}
}

func usage(w io.Writer) {
	const msg = `Usage: nvim-sync <command>
Commands:
  watch   Watch nvim config and docker cp changes into the container
  sync    Copy all nvim config files into the container once
Environment:
  NVIM_SYNC_CONTAINER  target container (default nvim-dev)
  NVIM_SYNC_SRC        host config dir (default nvim/config)
  NVIM_SYNC_DEST       container config dir (default /root/.config/nvim)
`
	_, _ = fmt.Fprint(w, msg)
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

func runWatch(s *syncer.Syncer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	return watchLoop(ctx, s)
}

// watchLoop watches s.SrcRoot and syncs debounced batches until ctx is
// cancelled. Split from runWatch so the loop is testable without OS signals.
func watchLoop(ctx context.Context, s *syncer.Syncer) error {
	w := watcher.New(s.SrcRoot, debounceWindow)
	ch, watchErr := w.Watch(ctx)
	if watchErr != nil {
		return fmt.Errorf("start watcher: %w", watchErr)
	}

	slog.Info("nvim-sync: watching", "container", s.Container, "src", s.SrcRoot, "dest", s.DestRoot)
	for changed := range ch {
		syncBatch(s, changed)
	}
	return nil
}

// syncBatch copies a debounced batch of changed files, logging per-file results
// and continuing past individual failures.
func syncBatch(s *syncer.Syncer, changed []string) {
	for _, file := range changed {
		if copyErr := s.Copy(file); copyErr != nil {
			slog.Warn("sync failed", "file", file, "error", copyErr)
			continue
		}
		slog.Info("synced", "file", file)
	}
}

func runSync(s *syncer.Syncer) error {
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
