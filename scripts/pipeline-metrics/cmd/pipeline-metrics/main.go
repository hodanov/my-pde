// Command pipeline-metrics aggregates the flow metrics of the autonomous
// improvement pipeline (scan -> triage -> PR -> merge) from `gh` JSON exports.
//
// It is a pure aggregator: it never calls `gh`, never touches the network and
// never writes anything back to GitHub. Feed it the two JSON files the digest
// workflow already fetches and it prints the warning block, the digest's flow
// section, or the machine-readable report.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"pipeline-metrics/internal/metrics"
	"pipeline-metrics/internal/model"
	"pipeline-metrics/internal/render"
)

// defaultSince is the day the `rejected`-on-close convention settled in; issues
// opened before it are counted but never enter a rate denominator.
const defaultSince = "2026-06-28"

// stdinPath is the value that makes --issues / --prs read standard input.
const stdinPath = "-"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pipeline-metrics:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fset := flag.NewFlagSet("pipeline-metrics", flag.ContinueOnError)
	issuesPath := fset.String("issues", "", "path to `gh issue list --json ...` output (- for stdin)")
	prsPath := fset.String("prs", "", "path to `gh pr list --json ...` output (- for stdin)")
	since := fset.String("since", defaultSince, "aggregation start date (YYYY-MM-DD); earlier issues are counted but excluded from rates")
	windowDays := fset.Int("window", metrics.DefaultWindowDays, "rolling rate window in days measured back from --now; 0 disables it")
	minSample := fset.Int("min-sample", metrics.DefaultMinSample, "smallest denominator a rate is reported for")
	alertMinSample := fset.Int("alert-min-sample", metrics.DefaultAlertMinSample, "smallest denominator a threshold alert may fire on")
	format := fset.String("format", "markdown", "output: markdown (flow section), alerts (warning block, empty when quiet) or json")
	nowFlag := fset.String("now", "", "evaluation time in RFC3339; defaults to the current time (injectable for tests)")
	if parseErr := fset.Parse(args); parseErr != nil {
		return parseErr
	}

	if *issuesPath == "" || *prsPath == "" {
		return errors.New("--issues and --prs are required")
	}
	if *issuesPath == stdinPath && *prsPath == stdinPath {
		return errors.New("only one of --issues / --prs can read stdin")
	}

	sinceTime, sinceErr := time.Parse(time.DateOnly, *since)
	if sinceErr != nil {
		return fmt.Errorf("invalid --since %q: %w", *since, sinceErr)
	}
	now := time.Now().UTC()
	if *nowFlag != "" {
		parsed, nowErr := time.Parse(time.RFC3339, *nowFlag)
		if nowErr != nil {
			return fmt.Errorf("invalid --now %q: %w", *nowFlag, nowErr)
		}
		now = parsed.UTC()
	}

	issuesJSON, issuesErr := readInput(*issuesPath, stdin)
	if issuesErr != nil {
		return fmt.Errorf("read issues: %w", issuesErr)
	}
	prsJSON, prsErr := readInput(*prsPath, stdin)
	if prsErr != nil {
		return fmt.Errorf("read prs: %w", prsErr)
	}

	dataset, parseErr := model.Parse(issuesJSON, prsJSON)
	if parseErr != nil {
		return parseErr
	}

	opt := metrics.DefaultOptions(now, sinceTime)
	opt.WindowDays = *windowDays
	opt.MinSample = *minSample
	opt.AlertMinSample = *alertMinSample
	report := metrics.Compute(&dataset, &opt)

	switch *format {
	case "markdown":
		flow, flowErr := render.Flow(&report)
		if flowErr != nil {
			return flowErr
		}
		_, writeErr := io.WriteString(stdout, flow)
		return writeErr
	case "alerts":
		_, writeErr := io.WriteString(stdout, render.Alerts(&report))
		return writeErr
	case "json":
		out, jsonErr := render.JSON(&report)
		if jsonErr != nil {
			return jsonErr
		}
		_, writeErr := fmt.Fprintf(stdout, "%s\n", out)
		return writeErr
	default:
		return fmt.Errorf("unknown --format %q (want markdown, alerts or json)", *format)
	}
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == stdinPath {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}
