// Command agent-stats summarises local AI CLI session transcripts (read-only).
//
// It walks Claude Code's ~/.claude/projects/**/*.jsonl transcripts and reports
// token usage, tool-call counts and frequently touched files per the aggregate.
// It never writes to or mutates the transcripts.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-stats/internal/parser"
	"agent-stats/internal/report"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agent-stats:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Accept an optional "summary" subcommand for forward compatibility.
	if len(args) > 0 && args[0] == "summary" {
		args = args[1:]
	}

	fset := flag.NewFlagSet("agent-stats", flag.ContinueOnError)
	dir := fset.String("dir", defaultDir(), "directory of Claude Code transcripts to scan")
	since := fset.Duration("since", 0, "only include sessions active within this window (e.g. 24h); 0 = all")
	project := fset.String("project", "", "only include sessions whose cwd or file path contains this substring")
	asJSON := fset.Bool("json", false, "emit machine-readable JSON instead of a table")
	if err := fset.Parse(args); err != nil {
		return err
	}

	sessions, err := collect(*dir, *since, *project, time.Now())
	if err != nil {
		return err
	}
	summary := report.Summarize(sessions)

	if *asJSON {
		out, err := report.RenderJSON(&summary)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}
	fmt.Print(report.RenderTable(&summary))
	return nil
}

// collect finds *.jsonl transcripts under dir and returns the sessions that
// pass the since/project filters. A missing dir yields an empty result rather
// than an error, so the tool is useful before any CLI has written transcripts.
func collect(dir string, since time.Duration, project string, now time.Time) ([]parser.Session, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}

	var sessions []parser.Session
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		s, perr := parser.ParseFile(path)
		if perr != nil {
			return perr
		}
		if !cutoff.IsZero() && !s.End.IsZero() && s.End.Before(cutoff) {
			return nil
		}
		// A session matches the project filter when either its file path or its
		// recorded cwd contains the substring.
		if project != "" && !strings.Contains(path, project) && !strings.Contains(s.Cwd, project) {
			return nil
		}
		sessions = append(sessions, s)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return sessions, nil
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/projects"
	}
	return filepath.Join(home, ".claude", "projects")
}
