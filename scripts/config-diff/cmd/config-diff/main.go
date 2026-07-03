// Command config-diff compares an AI CLI configuration source directory with its
// deployed copy read-only, printing an ok/drift/missing summary and exiting
// non-zero when anything has drifted or is not yet deployed.
//
// It takes the same arguments as ai-agents/scripts/copy-entries.sh:
//
//	config-diff <mode> <src> <dest>    # mode: skills | agents | settings
//
// so a Makefile can place a diff/check target next to each copy target using the
// same *_SRC / *_DEST variables.
package main

import (
	"fmt"
	"io"
	"os"

	"config-diff/internal/diff"
)

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout))
}

// execute parses args, runs the comparison and writes a summary to out,
// returning the process exit code (0 = all ok, 1 = drift/missing, 2 = usage/error).
func execute(args []string, out io.Writer) int {
	if len(args) != 3 {
		_, _ = fmt.Fprintln(out, "Usage: config-diff <mode> <src> <dest>")
		_, _ = fmt.Fprintln(out, "  mode: skills | agents | settings")
		return 2
	}
	mode, src, dest := args[0], args[1], args[2]

	entries, classifyErr := diff.Classify(mode, src, dest)
	if classifyErr != nil {
		_, _ = fmt.Fprintln(out, "config-diff:", classifyErr)
		return 2
	}

	writeSummary(out, mode, dest, entries)
	if diff.AnyDivergent(entries) {
		return 1
	}
	return 0
}

// writeSummary prints one line per entry plus drift detail.
func writeSummary(out io.Writer, mode, dest string, entries []diff.Entry) {
	_, _ = fmt.Fprintf(out, "%s -> %s\n", mode, dest)
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "  (no entries)")
		return
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(out, "  %-8s %s\n", e.State, e.Label)
		if e.State == diff.StateDrift && e.Note != "" {
			_, _ = fmt.Fprintf(out, "           %s\n", e.Note)
		}
	}
}
