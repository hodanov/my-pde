// Command agents-lint statically validates the AI agent skill and subagent
// definitions under ai-agents/ (frontmatter schema, naming conventions,
// name/dir agreement, uniqueness and skill->subagent reference integrity). It
// is read-only and prints a warn/error summary, exiting non-zero when a
// definition is broken — a deploy-time cut before pushing bad config to the
// AI CLIs.
//
// Usage:
//
//	agents-lint [--root ai-agents] [--strict]
//
// Exit codes: 0 = clean, 1 = lint violations (any error, or any warn under
// --strict), 2 = usage/IO error.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"agents-lint/internal/lint"
)

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

// execute is the testable entry point: it returns the process exit code so
// tests can drive the command without spawning a process.
func execute(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("agents-lint", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", "ai-agents", "root directory holding skills/ and agents/")
	strict := fs.Bool("strict", false, "treat warnings as failures")
	if parseErr := fs.Parse(args); parseErr != nil {
		return 2
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintln(errOut, "agents-lint: unexpected arguments:", fs.Args())
		return 2
	}

	report, runErr := lint.Run(os.DirFS(*root))
	if runErr != nil {
		_, _ = fmt.Fprintln(errOut, "agents-lint:", runErr)
		return 2
	}

	report.Render(out)

	if report.HasError() {
		return 1
	}
	if *strict && report.HasWarn() {
		return 1
	}
	return 0
}
