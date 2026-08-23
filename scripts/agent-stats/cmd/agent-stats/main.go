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
	detail := fset.Bool("detail", false, "with --json, include the full per-session list (much larger output)")
	if err := fset.Parse(args); err != nil {
		return err
	}

	sessions, err := collect(*dir, *since, *project, time.Now())
	if err != nil {
		return err
	}
	summary := report.Summarize(sessions)

	if *asJSON {
		out, err := report.RenderJSON(&summary, *detail)
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

	groups, order, err := groupTranscripts(dir)
	if err != nil {
		return nil, err
	}

	var sessions []parser.Session
	for _, key := range order {
		s := parser.NewSession(filepath.Base(key))
		read := 0
		for _, path := range groups[key] {
			if perr := parser.AppendFile(&s, path); perr != nil {
				// A single unreadable transcript (permission error, race with a
				// deleted file, ...) shouldn't blank out every other session's
				// results; skip it and keep going.
				fmt.Fprintf(os.Stderr, "agent-stats: skipping %s: %v\n", path, perr)
				continue
			}
			read++
		}
		if read == 0 {
			continue
		}
		if !cutoff.IsZero() && !s.End.IsZero() && s.End.Before(cutoff) {
			continue
		}
		// A session matches the project filter when either its transcript path
		// or its recorded cwd contains the substring.
		if project != "" && !strings.Contains(key, project) && !strings.Contains(s.Cwd, project) {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// subagentDir is the directory name Claude Code writes subagent transcripts to.
const subagentDir = "subagents"

// sessionKey maps a transcript path to the logical session it belongs to.
// Claude Code writes a session's own turns to <project>/<id>.jsonl and each
// subagent it spawns to <project>/<id>/subagents/agent-*.jsonl. Counting those
// as separate sessions roughly doubles the session count and double-counts
// wall-clock, since a subagent's transcript overlaps its parent's.
func sessionKey(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) != subagentDir {
		return path
	}
	return filepath.Dir(dir) + ".jsonl"
}

// groupTranscripts walks dir and buckets every transcript by the session it
// belongs to, returning the buckets and their first-seen order so the result
// stays deterministic.
func groupTranscripts(dir string) (groups map[string][]string, order []string, err error) {
	groups = map[string][]string{}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory shouldn't blank out the whole scan, the
			// same way an unreadable file doesn't.
			fmt.Fprintf(os.Stderr, "agent-stats: skipping %s: %v\n", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		key := sessionKey(path)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], path)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return groups, order, nil
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/projects"
	}
	return filepath.Join(home, ".claude", "projects")
}
