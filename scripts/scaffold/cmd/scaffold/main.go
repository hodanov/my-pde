// Command scaffold generates a new scripts/ Go module together with its CI
// workflow and mise task block, so adding a module no longer means copying and
// hand-editing files. The CI workflow and mise block are rendered from a live
// template module (config-diff by default) so pinned action SHAs and workflow
// structure follow the repository instead of drifting from a hard-coded copy.
//
// Usage:
//
//	scaffold new <name> [--from <module>] [--root <dir>]
//
// Generation is additive: it never overwrites an existing file and exits
// non-zero if any target already exists. The mise task block is printed to
// stdout for manual append (mise.toml is not edited in place).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"scaffold/internal/gen"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses arguments and drives generation, returning the process exit code
// (0 = generated, 1 = generation error/collision, 2 = usage error).
func run(args []string, out, errOut io.Writer) int {
	// Expect: new <name> [flags]. Taking the name before flag parsing lets the
	// name precede flags (Go's flag package stops at the first positional arg).
	if len(args) < 2 || args[0] != "new" || strings.HasPrefix(args[1], "-") {
		usage(errOut)
		return 2
	}
	name := args[1]

	fs := flag.NewFlagSet("scaffold new", flag.ContinueOnError)
	fs.SetOutput(errOut)
	from := fs.String("from", "config-diff", "existing module to use as the CI/mise template")
	root := fs.String("root", ".", "repository root to read templates from and write into")
	if parseErr := fs.Parse(args[2:]); parseErr != nil {
		return 2
	}
	if fs.NArg() != 0 {
		usage(errOut)
		return 2
	}

	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(*root, rel)) }
	exists := func(rel string) bool {
		_, statErr := os.Stat(filepath.Join(*root, rel))
		return statErr == nil
	}

	spec, specErr := gen.NewSpec(name, *from, exists)
	if specErr != nil {
		_, _ = fmt.Fprintln(errOut, "scaffold:", specErr)
		return 1
	}

	result, planErr := spec.Plan(read, exists)
	if planErr != nil {
		_, _ = fmt.Fprintln(errOut, "scaffold:", planErr)
		return 1
	}

	if writeErr := writeFiles(*root, result.Files); writeErr != nil {
		_, _ = fmt.Fprintln(errOut, "scaffold:", writeErr)
		return 1
	}

	report(out, name, result)
	return 0
}

// writeFiles creates each planned file under root, making parent directories.
func writeFiles(root string, files []gen.File) error {
	for _, f := range files {
		abs := filepath.Join(root, f.Path)
		if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
			return fmt.Errorf("mkdir for %s: %w", f.Path, mkErr)
		}
		if writeErr := os.WriteFile(abs, []byte(f.Content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", f.Path, writeErr)
		}
	}
	return nil
}

// report prints the created files, the mise block to paste, and next steps.
func report(out io.Writer, name string, result gen.Result) {
	_, _ = fmt.Fprintf(out, "Created module %s:\n", name)
	for _, f := range result.Files {
		_, _ = fmt.Fprintf(out, "  %s\n", f.Path)
	}
	_, _ = fmt.Fprintln(out, "\nAppend this block to mise.toml:")
	_, _ = fmt.Fprintln(out, result.MiseBlock)
	_, _ = fmt.Fprintf(out, "Then add %s:test / %s:lint to the go:test / go:lint depends lists.\n", name, name)
}

// usage prints command usage.
func usage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "Usage: scaffold new <name> [--from <module>] [--root <dir>]")
	_, _ = fmt.Fprintln(out, "  Generates scripts/<name>/, .github/workflows/ci_<name>.yml, and a mise block.")
}
