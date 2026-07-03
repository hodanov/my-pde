// Package runner discovers Go modules under a root directory and runs the same
// checks as CI (goimports diff gate, golangci-lint, go test) against each one.
//
// It is deliberately dependency-free: external commands are executed through an
// injectable Runner so the discovery, check ordering, and result aggregation can
// be unit-tested without a real toolchain or network access.
package runner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Runner executes a command in dir and returns its combined output.
type Runner func(dir, name string, args ...string) ([]byte, error)

// Check is the result of a single check run against one module.
type Check struct {
	// Mod is the module directory (relative to the search root when possible).
	Mod string
	// Name is the check identifier: "goimports", "golangci-lint" or "go test".
	Name string
	// OK reports whether the check passed.
	OK bool
	// Output holds diff/log detail, populated on failure.
	Output string
}

// checkOrder lists the checks in the order CI runs them.
var checkOrder = []string{"goimports", "golangci-lint", "go test"}

// SelectChecks maps an -only value ("", "lint", "test") to the check names to
// run. An empty value runs every check. An unknown value returns an error.
func SelectChecks(only string) ([]string, error) {
	switch only {
	case "":
		return append([]string(nil), checkOrder...), nil
	case "lint":
		return []string{"goimports", "golangci-lint"}, nil
	case "test":
		return []string{"go test"}, nil
	default:
		return nil, fmt.Errorf("unknown -only value %q (want lint|test or empty)", only)
	}
}

// Discover returns the directories under root that contain a go.mod file,
// sorted for stable output. vendor, testdata and dot-directories are skipped.
func Discover(root string) ([]string, error) {
	var mods []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "go.mod" {
			mods = append(mods, filepath.Dir(path))
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(mods)
	return mods, nil
}

// skipDir reports whether a directory should be pruned from the walk.
func skipDir(name string) bool {
	return name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")
}

// Verify runs the named checks against a single module directory and returns one
// Check per name, preserving the given order.
func Verify(mod string, run Runner, names []string) []Check {
	checks := make([]Check, 0, len(names))
	for _, name := range names {
		checks = append(checks, runCheck(name, mod, run))
	}
	return checks
}

// VerifyAll discovers modules under root, filters them by modFilter (a path
// substring; empty matches all), runs the given checks against each, and
// returns the aggregated results. Mod labels are made relative to root.
func VerifyAll(root string, run Runner, names, modFilter []string) ([]Check, error) {
	mods, discErr := Discover(root)
	if discErr != nil {
		return nil, discErr
	}
	var checks []Check
	for _, mod := range mods {
		if !matchesFilter(mod, modFilter) {
			continue
		}
		label := relLabel(root, mod)
		for _, c := range Verify(mod, run, names) {
			c.Mod = label
			checks = append(checks, c)
		}
	}
	return checks, nil
}

// AnyFailed reports whether at least one check failed.
func AnyFailed(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return true
		}
	}
	return false
}

// matchesFilter reports whether mod contains every requested substring. An empty
// filter list matches everything.
func matchesFilter(mod string, filter []string) bool {
	for _, f := range filter {
		if f != "" && !strings.Contains(mod, f) {
			return false
		}
	}
	return true
}

// relLabel returns mod relative to root, falling back to mod on failure.
func relLabel(root, mod string) string {
	rel, relErr := filepath.Rel(root, mod)
	if relErr != nil {
		return mod
	}
	if rel == "." {
		return filepath.Base(mod)
	}
	return rel
}

// runCheck runs a single named check in moddir via the injected runner.
func runCheck(name, moddir string, run Runner) Check {
	switch name {
	case "goimports":
		// goimports -l lists files whose formatting differs; any output is a fail.
		out, importsErr := run(moddir, "goimports", "-l", ".")
		listed := strings.TrimSpace(string(out))
		return result(name, importsErr == nil && listed == "", listed, importsErr)
	case "golangci-lint":
		out, lintErr := run(moddir, "golangci-lint", "run", "./...")
		return result(name, lintErr == nil, strings.TrimSpace(string(out)), lintErr)
	case "go test":
		out, testErr := run(moddir, "go", "test", "./...", "-count=1")
		return result(name, testErr == nil, strings.TrimSpace(string(out)), testErr)
	default:
		return Check{Name: name, OK: false, Output: "unknown check"}
	}
}

// result builds a Check, leaving Output empty on success and populating it with
// the failure detail otherwise.
func result(name string, ok bool, output string, cmdErr error) Check {
	if ok {
		return Check{Name: name, OK: true}
	}
	return Check{Name: name, OK: false, Output: detail(output, cmdErr)}
}

// detail combines command output with the exec error for a failure message.
func detail(output string, cmdErr error) string {
	switch {
	case output != "" && cmdErr != nil:
		return output + "\n" + cmdErr.Error()
	case output != "":
		return output
	case cmdErr != nil:
		return cmdErr.Error()
	default:
		return ""
	}
}
