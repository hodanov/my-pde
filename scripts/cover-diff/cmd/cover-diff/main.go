// Command cover-diff reports which statements a change added or rewrote without
// any test ever executing them -- patch coverage, not module coverage.
//
// It is read-only: it writes no file into the repository, touches no Issue or
// PR, and keeps the coverage profiles it generates in a temporary directory it
// deletes on the way out. Given --diff and --profile it runs neither git nor
// go, which is what lets the whole report be pinned by fixtures.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"cover-diff/internal/cover"
	"cover-diff/internal/patch"
	"cover-diff/internal/report"
	"cover-diff/internal/udiff"
)

// Exit codes. Measurement failure is deliberately distinct from a threshold
// miss: "could not measure" must never read as "measured and passed".
const (
	exitOK       = 0
	exitBelow    = 1
	exitUnusable = 2
	defaultBase  = "origin/main"
	stdinPath    = "-"
	formatText   = "text"
	formatJSON   = "json"
)

func main() {
	os.Exit(execute(os.Args[1:], defaultRunner, os.Stdin, os.Stdout))
}

// defaultRunner is the only place the process shells out.
func defaultRunner(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// options are the flag values one run was given.
type options struct {
	root          string
	base          string
	mod           string
	diffPath      string
	profiles      profileFlag
	exclude       string
	coverpkg      bool
	thresholdText string
	format        string
}

func execute(args []string, run patch.Runner, stdin io.Reader, out io.Writer) int {
	opt, parseErr := parseArgs(args, out)
	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			return exitUnusable
		}
		_, _ = fmt.Fprintln(out, "cover-diff:", parseErr)
		return exitUnusable
	}

	r, buildErr := build(opt, run, stdin)
	if buildErr != nil {
		_, _ = fmt.Fprintln(out, "cover-diff:", buildErr)
		return exitUnusable
	}

	if renderErr := render(r, opt.format, out); renderErr != nil {
		_, _ = fmt.Fprintln(out, "cover-diff:", renderErr)
		return exitUnusable
	}

	if r.Threshold != nil && len(r.Unmeasured) > 0 {
		return exitUnusable
	}
	if report.Below(r) {
		return exitBelow
	}
	return exitOK
}

func parseArgs(args []string, out io.Writer) (*options, error) {
	opt := &options{profiles: profileFlag{}}
	fset := flag.NewFlagSet("cover-diff", flag.ContinueOnError)
	fset.SetOutput(out)
	fset.StringVar(&opt.root, "root", "", "repository root; defaults to `git rev-parse --show-toplevel`")
	fset.StringVar(&opt.base, "base", defaultBase, "revision to diff HEAD against, at their merge base")
	fset.StringVar(&opt.mod, "mod", "", "only report modules whose name contains this substring")
	fset.StringVar(&opt.diffPath, "diff", "", "read a unified diff from this path instead of running git (- for stdin)")
	fset.Var(&opt.profiles, "profile", "use an existing coverage profile: `<module>=<path>`; repeatable")
	fset.StringVar(&opt.exclude, "exclude", "", "drop changed files whose path matches this `regexp`")
	fset.BoolVar(&opt.coverpkg, "coverpkg", false, "measure every package of the module, not only each package's own tests")
	fset.StringVar(&opt.thresholdText, "threshold", "", "fail below this patch coverage `percentage`; unset never fails")
	fset.StringVar(&opt.format, "format", formatText, "output: text or json")
	if flagErr := fset.Parse(args); flagErr != nil {
		return nil, flagErr
	}
	if opt.format != formatText && opt.format != formatJSON {
		return nil, fmt.Errorf("unknown --format %q (want %s or %s)", opt.format, formatText, formatJSON)
	}
	return opt, nil
}

// build turns the flags into a report, running git and go only for the inputs
// that were not injected.
func build(opt *options, run patch.Runner, stdin io.Reader) (*patch.Report, error) {
	joinOpt := patch.Options{ModFilter: opt.mod}
	if opt.exclude != "" {
		exclude, compileErr := regexp.Compile(opt.exclude)
		if compileErr != nil {
			return nil, fmt.Errorf("invalid --exclude %q: %w", opt.exclude, compileErr)
		}
		joinOpt.Exclude = exclude
	}
	var threshold *float64
	if opt.thresholdText != "" {
		value, thresholdErr := parsePercentage(opt.thresholdText)
		if thresholdErr != nil {
			return nil, thresholdErr
		}
		threshold = &value
	}

	root := rootResolver{explicit: opt.root, run: run}
	diff, diffErr := readDiff(opt, &root, run, stdin)
	if diffErr != nil {
		return nil, diffErr
	}

	changed := udiff.Parse(diff)
	selected := patch.Select(changed.Files, joinOpt)

	profiles, warnings, profileErr := loadProfiles(opt, &root, run, patch.Modules(selected))
	if profileErr != nil {
		return nil, profileErr
	}

	r := patch.Join(selected, profiles, append(changed.Warnings, warnings...))
	r.Threshold = threshold
	return &r, nil
}

func readDiff(opt *options, root *rootResolver, run patch.Runner, stdin io.Reader) ([]byte, error) {
	if opt.diffPath != "" {
		if opt.diffPath == stdinPath {
			return io.ReadAll(stdin)
		}
		return os.ReadFile(opt.diffPath)
	}
	dir, rootErr := root.resolve()
	if rootErr != nil {
		return nil, rootErr
	}
	return patch.Diff(run, dir, opt.base)
}

// loadProfiles returns one profile per module, preferring an injected path and
// otherwise running the module's tests into a temporary directory. A module
// whose profile cannot be produced is left out, which Join reports as
// unmeasured rather than as fully uncovered.
func loadProfiles(opt *options, root *rootResolver, run patch.Runner, modules []string) (profiles map[string]cover.Profile, warnings []string, err error) {
	profiles = map[string]cover.Profile{}

	tmp, dir, prepareErr := opt.prepareProfileRun(root, modules)
	if prepareErr != nil {
		return nil, nil, prepareErr
	}
	if tmp != "" {
		defer func() { _ = os.RemoveAll(tmp) }()
	}

	for _, module := range modules {
		source, injected := opt.profiles[module]
		if !injected {
			source = filepath.Join(tmp, module+".out")
			if writeErr := patch.WriteProfile(run, dir, module, source, opt.coverpkg); writeErr != nil {
				warnings = append(warnings, writeErr.Error())
				continue
			}
		}
		raw, readErr := os.ReadFile(source)
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("read profile for %s: %v", module, readErr))
			continue
		}
		profile, coverErr := cover.Parse(raw)
		if coverErr != nil {
			warnings = append(warnings, fmt.Sprintf("parse profile for %s: %v", module, coverErr))
			continue
		}
		profiles[module] = profile
	}
	return profiles, warnings, nil
}

// prepareProfileRun resolves what running the missing profiles needs: the
// repository root and a temporary directory to write them into. It returns an
// empty directory when every module already has an injected profile, which is
// what keeps the fixture-driven path from touching git or the filesystem.
func (o *options) prepareProfileRun(root *rootResolver, modules []string) (tmpDir, repoRoot string, err error) {
	needed := false
	for _, module := range modules {
		if _, injected := o.profiles[module]; !injected {
			needed = true
			break
		}
	}
	if !needed {
		return "", "", nil
	}
	dir, rootErr := root.resolve()
	if rootErr != nil {
		return "", "", rootErr
	}
	tmp, tmpErr := os.MkdirTemp("", "cover-diff-")
	if tmpErr != nil {
		return "", "", fmt.Errorf("create temporary directory: %w", tmpErr)
	}
	return tmp, dir, nil
}

// rootResolver asks git for the repository root at most once, and only when a
// step actually needs it.
type rootResolver struct {
	explicit string
	run      patch.Runner
	resolved string
}

func (r *rootResolver) resolve() (string, error) {
	if r.explicit != "" {
		return r.explicit, nil
	}
	if r.resolved != "" {
		return r.resolved, nil
	}
	root, rootErr := patch.RepoRoot(r.run)
	if rootErr != nil {
		return "", rootErr
	}
	r.resolved = root
	return root, nil
}

// parsePercentage reads a --threshold value, tolerating a trailing percent sign.
func parsePercentage(text string) (float64, error) {
	value, convErr := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
	if convErr != nil {
		return 0, fmt.Errorf("invalid --threshold %q: %w", text, convErr)
	}
	if value < 0 || value > 100 {
		return 0, fmt.Errorf("invalid --threshold %q: want a percentage between 0 and 100", text)
	}
	return value, nil
}

func render(r *patch.Report, format string, out io.Writer) error {
	if format == formatJSON {
		encoded, jsonErr := report.JSON(r)
		if jsonErr != nil {
			return jsonErr
		}
		_, writeErr := fmt.Fprintf(out, "%s\n", encoded)
		return writeErr
	}
	_, writeErr := io.WriteString(out, report.Text(r))
	return writeErr
}

// profileFlag collects repeatable `--profile <module>=<path>` values.
type profileFlag map[string]string

func (f profileFlag) String() string {
	pairs := make([]string, 0, len(f))
	for module, path := range f {
		pairs = append(pairs, module+"="+path)
	}
	return strings.Join(pairs, ",")
}

func (f profileFlag) Set(value string) error {
	module, path, ok := strings.Cut(value, "=")
	if !ok || module == "" || path == "" {
		return fmt.Errorf("want <module>=<path>, got %q", value)
	}
	f[module] = path
	return nil
}
