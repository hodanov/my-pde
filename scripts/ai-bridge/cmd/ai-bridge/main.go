package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-bridge/internal/daemon"
	"ai-bridge/internal/history"
	"ai-bridge/internal/launchd"
	"ai-bridge/internal/launcher"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "daemon":
		err = runDaemon()
	case "install-launchd":
		err = runInstallLaunchd()
	case "history":
		err = runHistory()
	case "replay":
		err = runReplay()
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
	fmt.Fprintln(os.Stderr, "Usage: ai-bridge <command>")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  daemon            Start the ai-bridge daemon")
	fmt.Fprintln(os.Stderr, "  install-launchd   Install and load the launchd agent")
	fmt.Fprintln(os.Stderr, "  history [-n N]    List recent requests (default 20)")
	fmt.Fprintln(os.Stderr, "  replay [--last]   Re-send the most recent request")
}

func runHistory() error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	n := fs.Int("n", 20, "number of recent entries to show")
	if parseErr := fs.Parse(os.Args[2:]); parseErr != nil {
		return parseErr
	}

	cfg, loadConfigErr := daemon.LoadConfig()
	if loadConfigErr != nil {
		return loadConfigErr
	}

	records, loadErr := history.Load(history.Path(cfg.BridgeDir))
	if loadErr != nil {
		return loadErr
	}
	fmt.Print(history.FormatList(records, *n))
	return nil
}

func runReplay() error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	// --last is accepted for explicitness; the most recent request is replayed.
	_ = fs.Bool("last", false, "replay the most recent request")
	if parseErr := fs.Parse(os.Args[2:]); parseErr != nil {
		return parseErr
	}

	cfg, loadConfigErr := daemon.LoadConfig()
	if loadConfigErr != nil {
		return loadConfigErr
	}

	records, loadErr := history.Load(history.Path(cfg.BridgeDir))
	if loadErr != nil {
		return loadErr
	}
	if len(records) == 0 {
		return fmt.Errorf("no history to replay")
	}

	rec := records[0]
	rec.Timestamp = time.Now().Unix()
	reqPath := filepath.Join(cfg.BridgeDir, "request.json")
	if writeErr := history.WriteRequest(reqPath, rec); writeErr != nil {
		return fmt.Errorf("write request: %w", writeErr)
	}

	slog.Info("ai-bridge: replayed request", "cwd", rec.CWD)
	return nil
}

func runDaemon() error {
	cfg, loadConfigErr := daemon.LoadConfig()
	if loadConfigErr != nil {
		return loadConfigErr
	}

	l, newLauncherErr := launcher.New(cfg.Launcher, launcher.DefaultRunner)
	if newLauncherErr != nil {
		return newLauncherErr
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	return daemon.Run(ctx, cfg, l)
}

func runInstallLaunchd() error {
	exe, executableErr := os.Executable()
	if executableErr != nil {
		return fmt.Errorf("cannot determine executable path: %w", executableErr)
	}
	binaryPath, absErr := filepath.Abs(exe)
	if absErr != nil {
		return fmt.Errorf("cannot resolve absolute path: %w", absErr)
	}

	cli := os.Getenv("AI_BRIDGE_CLI")
	if cli == "" {
		cli = "claude"
	}
	launcherName := os.Getenv("AI_BRIDGE_LAUNCHER")
	if launcherName == "" {
		launcherName = "wezterm"
	}

	return launchd.Install(binaryPath, cli, launcherName)
}
