package patch

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Runner executes a command in dir and returns its combined output.
type Runner func(dir, name string, args ...string) ([]byte, error)

// RepoRoot asks git for the top level of the working tree, so the tool works
// from any directory -- including scripts/cover-diff, where `mise run
// cover-diff:run` starts it.
func RepoRoot(run Runner) (string, error) {
	out, runErr := run(".", "git", "rev-parse", "--show-toplevel")
	if runErr != nil {
		return "", fmt.Errorf("locate repository root: %w: %s", runErr, strings.TrimSpace(string(out)))
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("locate repository root: git reported no top level")
	}
	return root, nil
}

// Diff returns the merge-base diff between base and HEAD with no context lines.
// quotePath is turned off so paths outside ASCII arrive verbatim.
func Diff(run Runner, root, base string) ([]byte, error) {
	out, runErr := run(root, "git", "-c", "core.quotePath=false", "diff", "--unified=0", base+"...HEAD")
	if runErr != nil {
		return nil, fmt.Errorf("git diff against %s failed (fetch it first with `git fetch origin`): %w: %s",
			base, runErr, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// WriteProfile runs the module's tests with coverage recording into dest.
// coverpkg widens the measured set to every package of the module, which trades
// the per-package blind spot for a wider denominator.
func WriteProfile(run Runner, root, module, dest string, coverpkg bool) error {
	args := []string{"test", "./...", "-count=1", "-coverprofile=" + dest}
	if coverpkg {
		args = append(args, "-coverpkg=./...")
	}
	dir := filepath.Join(root, ScriptsDir, module)
	out, runErr := run(dir, "go", args...)
	if runErr != nil {
		return fmt.Errorf("go test in %s/%s failed: %w: %s", ScriptsDir, module, runErr, strings.TrimSpace(string(out)))
	}
	return nil
}
