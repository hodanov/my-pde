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

	"ai-bridge/internal/daemon"
	"ai-bridge/internal/doctor"
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
	case "doctor":
		err = runDoctor()
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
	fmt.Fprintln(os.Stderr, "  doctor            Diagnose the ai-bridge environment")
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

func runDoctor() error {
	cfg, loadConfigErr := daemon.LoadConfig()
	if loadConfigErr != nil {
		return loadConfigErr
	}

	checks := doctor.Run(cfg, exec.LookPath)
	fmt.Print(doctor.Format(checks))
	if doctor.HasFailure(checks) {
		os.Exit(1)
	}
	return nil
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
