// Command permission-audit lists allow-rule candidates from the prompts the
// permission-ledger hook recorded, their outcomes in Claude Code transcripts,
// and repositories' local allow lists. It only reads.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"agent-stats/internal/parser"
	"agent-stats/internal/permission"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "permission-audit:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fset := flag.NewFlagSet("permission-audit", flag.ContinueOnError)
	settings := fset.String("settings", "", "settings file whose allow/ask/deny rules count as already decided (required)")
	ledger := fset.String("ledger", statePath("ledger.jsonl"), "ledger written by the permission-ledger hook")
	declined := fset.String("declined", statePath("declined.txt"), "rules declined in earlier reviews, one per line")
	dir := fset.String("dir", transcriptDir(), "directory of Claude Code transcripts")
	since := fset.Duration("since", 30*24*time.Hour, "only consider prompts and transcripts within this window; 0 = all")
	asJSON := fset.Bool("json", false, "emit JSON instead of a table")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if *settings == "" {
		return errors.New("--settings is required")
	}

	var cutoff time.Time
	if *since > 0 {
		cutoff = time.Now().Add(-*since)
	}

	known, err := knownRules(*settings, *declined)
	if err != nil {
		return err
	}
	entries, err := readLedger(*ledger, cutoff)
	if err != nil {
		return err
	}
	audit := permission.NewAudit(entries)
	if scanErr := scanTranscripts(*dir, cutoff, audit); scanErr != nil {
		return scanErr
	}
	repoOf := repoResolver()
	report := audit.Report(known, readLocalRules(audit.Cwds(), repoOf), repoOf)

	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	_, err = io.WriteString(out, renderTable(&report))
	return err
}

func knownRules(settingsPath, declinedPath string) (permission.RuleSet, error) {
	f, err := os.Open(settingsPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	configured, err := permission.SettingsRules(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", settingsPath, err)
	}

	d, err := os.Open(declinedPath)
	if errors.Is(err, fs.ErrNotExist) {
		return permission.NewRuleSet(configured), nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = d.Close() }()
	declined, err := permission.ReadDeclined(d)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", declinedPath, err)
	}
	return permission.NewRuleSet(configured, declined), nil
}

func readLedger(path string, cutoff time.Time) ([]permission.Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	entries, err := permission.ReadLedger(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var kept []permission.Entry
	for i := range entries {
		if entries[i].Timestamp.Before(cutoff) {
			continue
		}
		kept = append(kept, entries[i])
	}
	return kept, nil
}

func scanTranscripts(dir string, cutoff time.Time, audit *permission.Audit) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				return fs.SkipAll
			}
			fmt.Fprintf(os.Stderr, "permission-audit: skipping %s: %v\n", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "permission-audit: skipping %s: %v\n", path, err)
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}
		idx := parser.ToolCallIndex{}
		if err := idx.AppendFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "permission-audit: skipping %s: %v\n", path, err)
			return nil
		}
		audit.AddTranscript(path, idx)
		return nil
	})
}

func repoResolver() func(cwd string) string {
	roots := map[string]string{}
	return func(cwd string) string {
		if cwd == "" {
			return ""
		}
		if root, ok := roots[cwd]; ok {
			return root
		}
		root := mainCheckoutRoot(cwd)
		roots[cwd] = root
		return root
	}
}

func mainCheckoutRoot(cwd string) string {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) != ".git" {
		return ""
	}
	return filepath.Dir(common)
}

func readLocalRules(cwds []string, repoOf func(string) string) []permission.LocalRules {
	seen := map[string]struct{}{}
	var locals []permission.LocalRules
	for _, cwd := range cwds {
		repo := repoOf(cwd)
		if repo == "" {
			continue
		}
		if _, dup := seen[repo]; dup {
			continue
		}
		seen[repo] = struct{}{}
		allow, err := localAllowRules(filepath.Join(repo, ".claude", "settings.local.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "permission-audit: skipping %s: %v\n", repo, err)
			continue
		}
		if len(allow) > 0 {
			locals = append(locals, permission.LocalRules{Repo: repo, Allow: allow})
		}
	}
	return locals
}

func localAllowRules(path string) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return permission.LocalAllowRules(f)
}

func renderTable(report *permission.Report) string {
	const maxColumn = 60
	sections := []struct {
		title      string
		candidates []permission.Candidate
	}{
		{"Suggested rules", report.Candidates},
		{"Drafted rules (no suggestion from Claude Code; refine before promoting)", report.Unsuggested},
	}
	var b strings.Builder
	for _, s := range sections {
		fmt.Fprintf(&b, "%s: %d\n", s.title, len(s.candidates))
		if len(s.candidates) == 0 {
			fmt.Fprintln(&b)
			continue
		}
		width := len("RULE")
		for i := range s.candidates {
			width = max(width, len([]rune(truncate(s.candidates[i].Rule, maxColumn))))
		}
		fmt.Fprintf(&b, "  %-*s  %3s %3s %4s %7s %4s %5s %5s  %-10s  %s\n",
			width, "RULE", "OK", "NG", "PEND", "PAST-NG", "SESS", "REPOS", "LOCAL", "LAST", "EXAMPLE")
		for i := range s.candidates {
			c := &s.candidates[i]
			last := ""
			if !c.LastSeen.IsZero() {
				last = c.LastSeen.Local().Format(time.DateOnly)
			}
			example := ""
			if len(c.Examples) > 0 {
				example = truncate(c.Examples[0], maxColumn)
			}
			fmt.Fprintf(&b, "  %-*s  %3d %3d %4d %7d %4d %5d %5d  %-10s  %s\n",
				width, truncate(c.Rule, maxColumn), c.Approved, c.Rejected, c.Pending, c.FamilyRejections,
				c.Sessions, len(c.Repos), len(c.LocalRepos), last, example)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func truncate(s string, limit int) string {
	if runes := []rune(s); len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return s
}

func statePath(name string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return name
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "claude-permission-ledger", name)
}

func transcriptDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".claude", "projects")
	}
	return filepath.Join(home, ".claude", "projects")
}
