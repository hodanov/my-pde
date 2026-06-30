// Command ai-bridge bridges AI agent requests to local terminal launches.
//
// main is the composition root: it parses the subcommand, loads configuration,
// constructs the infrastructure adapters and injects them into the use cases.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/infra/config"
	"ai-bridge/internal/infra/fsrepo"
	"ai-bridge/internal/infra/launchd"
	"ai-bridge/internal/infra/launcher"
	"ai-bridge/internal/infra/system"
	"ai-bridge/internal/infra/watcher"
	"ai-bridge/internal/usecase"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cfg, loadConfigErr := config.Load()
	if loadConfigErr != nil {
		slog.Error("fatal", "error", loadConfigErr)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "daemon":
		err = runDaemon(cfg)
	case "install-launchd":
		err = runInstallLaunchd(cfg)
	case "doctor":
		runDoctor(cfg)
	case "history":
		err = runHistory(cfg)
	case "replay":
		err = runReplay(cfg)
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
	fmt.Fprintln(os.Stderr, "  history [-n N]    List recent requests (default 20)")
	fmt.Fprintln(os.Stderr, "  replay [--last]   Re-send the most recent request")
}

func runDaemon(cfg *domain.Config) error {
	l, newLauncherErr := launcher.New(cfg.Launcher, launcher.DefaultRunner)
	if newLauncherErr != nil {
		return newLauncherErr
	}

	dir := fsrepo.Dir{}
	process := usecase.NewProcessRequest(fsrepo.RequestRepository{}, dir, fsrepo.ScriptStore{}, l, fsrepo.HistoryRepository{}, cfg.BridgeDir, cfg.CLI)
	daemon := usecase.NewRunDaemon(dir, watcher.New(cfg.BridgeDir), process, cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	return daemon.Run(ctx)
}

func runDoctor(cfg *domain.Config) {
	checks := usecase.NewDiagnose(fsrepo.Dir{}, system.CommandLocator{}, cfg).Run()
	fmt.Print(domain.FormatChecks(checks))
	if domain.HasFailure(checks) {
		os.Exit(1)
	}
}

func runInstallLaunchd(cfg *domain.Config) error {
	return usecase.NewInstallAgent(system.Executable{}, launchd.Installer{}, cfg).Run()
}

func runHistory(cfg *domain.Config) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	n := fs.Int("n", 20, "number of recent entries to show")
	if parseErr := fs.Parse(os.Args[2:]); parseErr != nil {
		return parseErr
	}

	records, runErr := usecase.NewListHistory(fsrepo.HistoryRepository{}, cfg.BridgeDir).Run()
	if runErr != nil {
		return runErr
	}
	fmt.Print(domain.FormatHistory(records, *n))
	return nil
}

func runReplay(cfg *domain.Config) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	// --last is accepted for explicitness; the most recent request is replayed.
	_ = fs.Bool("last", false, "replay the most recent request")
	if parseErr := fs.Parse(os.Args[2:]); parseErr != nil {
		return parseErr
	}

	now := func() int64 { return time.Now().Unix() }
	return usecase.NewReplayRequest(fsrepo.HistoryRepository{}, fsrepo.RequestWriter{}, cfg.BridgeDir, now).Run()
}
