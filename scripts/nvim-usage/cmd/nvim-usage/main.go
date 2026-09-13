// Command nvim-usage aggregates the Neovim usage log written by
// nvim/config/lua/usage_recorder.lua (read-only): how often each keymap is
// typed, keys pressed over and over, frequent key sequences and Ex command
// frequency. The nvim-usage-review skill reads its output.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"nvim-usage/internal/analyze"
	"nvim-usage/internal/event"
	"nvim-usage/internal/report"
)

var renderers = map[string]func(*analyze.Report) (string, error){
	"md": func(r *analyze.Report) (string, error) { return report.RenderMarkdown(r), nil },
	"json": func(r *analyze.Report) (string, error) {
		var out strings.Builder
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		err := enc.Encode(r)
		return out.String(), err
	},
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nvim-usage:", err)
		return 2
	}
	out, err := opts.render()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nvim-usage:", err)
		return 1
	}
	if _, writeErr := io.WriteString(stdout, out); writeErr != nil {
		return 1
	}
	return 0
}

type options struct {
	dir    string
	cutoff time.Time
	format string
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	formats := slices.Sorted(maps.Keys(renderers))
	fset := flag.NewFlagSet("nvim-usage", flag.ContinueOnError)
	fset.SetOutput(stderr)
	dir := fset.String("dir", defaultDir(), "usage log directory written by usage_recorder.lua")
	since := fset.String("since", "30d", "only include events within this window: <n>d or a Go duration, 0 for all")
	now := fset.String("now", "", "evaluation time in RFC3339 (default: the current time)")
	format := fset.String("format", "md", "output format: "+strings.Join(formats, ", "))
	if err := fset.Parse(args); err != nil {
		return options{}, err
	}
	if fset.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fset.Args(), " "))
	}
	if _, ok := renderers[*format]; !ok {
		return options{}, fmt.Errorf("--format must be one of %s", strings.Join(formats, ", "))
	}
	window, err := parseWindow(*since)
	if err != nil {
		return options{}, err
	}
	at := time.Now()
	if *now != "" {
		if at, err = time.Parse(time.RFC3339, *now); err != nil {
			return options{}, fmt.Errorf("--now: %w", err)
		}
	}
	opts := options{dir: *dir, format: *format}
	if window > 0 {
		opts.cutoff = at.Add(-window)
	}
	return opts, nil
}

func parseWindow(s string) (time.Duration, error) {
	invalid := fmt.Errorf("--since %q: want <n>d or a Go duration", s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, invalid
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, invalid
	}
	return d, nil
}

func (o *options) render() (string, error) {
	l, err := event.ReadDir(o.dir, o.cutoff)
	if err != nil {
		return "", err
	}
	snap, err := event.ReadKeymaps(o.dir)
	if err != nil {
		return "", err
	}
	r := analyze.Build(l, snap)
	return renderers[o.format](&r)
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nvim-usage"
	}
	return filepath.Join(home, ".nvim-usage")
}
