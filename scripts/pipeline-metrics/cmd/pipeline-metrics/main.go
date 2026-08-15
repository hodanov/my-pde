// Command pipeline-metrics reports how well the autonomous improvement pipeline
// is working (read-only).
//
// It reads `gh issue list` / `gh pr list` JSON from files or stdin and emits the
// per-scan adoption, PR and merge figures plus threshold alerts. It never calls
// gh itself and never writes anything back to GitHub: fetching is the caller's
// job, which keeps this a pure function that tests can drive from fixtures.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"pipeline-metrics/internal/metrics"
	"pipeline-metrics/internal/model"
	"pipeline-metrics/internal/render"
)

// policyStart is the day the adopted/rejected label convention took hold.
// Issues created before it cannot be triaged retroactively, so they are counted
// but kept out of every denominator.
const policyStart = "2026-06-28"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pipeline-metrics:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fset := flag.NewFlagSet("pipeline-metrics", flag.ContinueOnError)
	issuesPath := fset.String("issues", "", "path to `gh issue list --json ...` output, or - for stdin")
	prsPath := fset.String("prs", "", "path to `gh pr list --json ...` output, or - for stdin")
	since := fset.String("since", policyStart, "ignore issues created before this date (YYYY-MM-DD)")
	nowFlag := fset.String("now", "", "evaluation time as RFC3339; defaults to the current time")
	windowDays := fset.Int("window-days", int(metrics.DefaultWindow/(24*time.Hour)), "flow window in days")
	months := fset.Int("months", metrics.DefaultMonths, "how many month cohorts the trend table covers")
	minSample := fset.Int("min-sample", metrics.DefaultMinSample, "smallest denominator that still gets a rate")
	format := fset.String("format", "markdown", "output format: markdown, alerts or json")
	if parseErr := fset.Parse(args); parseErr != nil {
		return parseErr
	}

	if *issuesPath == "" || *prsPath == "" {
		return fmt.Errorf("both --issues and --prs are required")
	}
	if *issuesPath == "-" && *prsPath == "-" {
		return fmt.Errorf("only one of --issues/--prs can read stdin")
	}

	sinceAt, sinceErr := time.Parse(time.DateOnly, *since)
	if sinceErr != nil {
		return fmt.Errorf("parse --since: %w", sinceErr)
	}
	now := time.Now().UTC()
	if *nowFlag != "" {
		parsed, nowErr := time.Parse(time.RFC3339, *nowFlag)
		if nowErr != nil {
			return fmt.Errorf("parse --now: %w", nowErr)
		}
		now = parsed.UTC()
	}

	issues, issuesErr := readIssues(*issuesPath, stdin)
	if issuesErr != nil {
		return issuesErr
	}
	prs, prsErr := readPullRequests(*prsPath, stdin)
	if prsErr != nil {
		return prsErr
	}

	summary := metrics.Summarize(issues, prs, metrics.Options{
		Now:       now,
		Since:     sinceAt,
		Window:    time.Duration(*windowDays) * 24 * time.Hour,
		Months:    *months,
		MinSample: *minSample,
	})

	switch *format {
	case "markdown":
		out, renderErr := render.Markdown(&summary)
		if renderErr != nil {
			return renderErr
		}
		_, writeErr := io.WriteString(stdout, out)
		return writeErr
	case "alerts":
		_, writeErr := io.WriteString(stdout, render.Alerts(&summary))
		return writeErr
	case "json":
		out, marshalErr := render.JSON(&summary)
		if marshalErr != nil {
			return marshalErr
		}
		_, writeErr := fmt.Fprintln(stdout, string(out))
		return writeErr
	default:
		return fmt.Errorf("unknown --format: %s", *format)
	}
}

func readIssues(path string, stdin io.Reader) ([]model.Issue, error) {
	r, closeFn, openErr := open(path, stdin)
	if openErr != nil {
		return nil, openErr
	}
	defer closeFn()
	return model.DecodeIssues(r)
}

func readPullRequests(path string, stdin io.Reader) ([]model.PullRequest, error) {
	r, closeFn, openErr := open(path, stdin)
	if openErr != nil {
		return nil, openErr
	}
	defer closeFn()
	return model.DecodePullRequests(r)
}

// open resolves a path to a reader, treating "-" as stdin. The returned closer
// is a no-op for stdin so callers can defer it unconditionally.
func open(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return stdin, func() {}, nil
	}
	f, openErr := os.Open(path)
	if openErr != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, openErr)
	}
	return f, func() { _ = f.Close() }, nil
}
