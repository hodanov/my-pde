// Command go-verify discovers the Go modules under a root directory and runs the
// same checks as CI (goimports diff gate, golangci-lint, go test) against each,
// printing an aggregated pass/fail summary and exiting non-zero on any failure.
//
// It is a shift-left quality gate: run it before pushing to catch formatting,
// lint and test failures locally instead of waiting for CI.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"go-verify/internal/runner"
)

func main() {
	root := flag.String("root", ".", "root directory to search for Go modules")
	only := flag.String("only", "", "run only a subset of checks: lint|test (default: all)")
	mod := flag.String("mod", "", "only verify modules whose path contains this substring")
	flag.Parse()

	os.Exit(execute(*root, *only, *mod, os.Stdout))
}

// execute runs the verification and writes a summary to out, returning the
// process exit code (0 = all passed, 1 = a check failed, 2 = usage/setup error).
func execute(root, only, mod string, out io.Writer) int {
	names, selErr := runner.SelectChecks(only)
	if selErr != nil {
		_, _ = fmt.Fprintln(out, "go-verify:", selErr)
		return 2
	}

	var filter []string
	if mod != "" {
		filter = []string{mod}
	}

	checks, verifyErr := runner.VerifyAll(root, defaultRunner, names, filter)
	if verifyErr != nil {
		_, _ = fmt.Fprintln(out, "go-verify:", verifyErr)
		return 2
	}
	if len(checks) == 0 {
		_, _ = fmt.Fprintln(out, "go-verify: no Go modules found under", root)
		return 2
	}

	writeSummary(out, checks)
	if runner.AnyFailed(checks) {
		return 1
	}
	return 0
}

// writeSummary prints a human-readable module × check pass/fail report.
func writeSummary(out io.Writer, checks []runner.Check) {
	for _, c := range checks {
		status := "PASS"
		if !c.OK {
			status = "FAIL"
		}
		_, _ = fmt.Fprintf(out, "%s  %-14s %s\n", status, c.Name, c.Mod)
		if !c.OK && c.Output != "" {
			_, _ = fmt.Fprintln(out, indent(c.Output))
		}
	}
}

// indent prefixes every line of s with two spaces for nested output.
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}

// defaultRunner executes a command in dir and returns its combined output.
func defaultRunner(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
