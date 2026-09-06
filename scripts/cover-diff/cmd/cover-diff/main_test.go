package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cover-diff/internal/patch"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

// refuseToRun is the Runner every fixture-driven test is given: with --diff and
// a --profile for each module, no test may shell out. If one does, it fails
// here instead of quietly depending on the developer's checkout.
func refuseToRun(t *testing.T) patch.Runner {
	t.Helper()
	return func(dir, name string, args ...string) ([]byte, error) {
		t.Errorf("unexpected command: %s %s (in %s)", name, strings.Join(args, " "), dir)
		return nil, nil
	}
}

func fixturePath(name string) string { return filepath.Join("..", "..", "testdata", name) }

func fixtureArgs(extra ...string) []string {
	return append([]string{
		"--diff", fixturePath("sample.diff"),
		"--profile", "sample-app=" + fixturePath("sample-app.profile"),
		"--profile", "other-app=" + fixturePath("other-app.profile"),
		"--profile", "third-app=" + fixturePath("broken.profile"),
	}, extra...)
}

func runFixture(t *testing.T, args []string) (output string, exitCode int) {
	t.Helper()
	var out bytes.Buffer
	code := execute(args, refuseToRun(t), strings.NewReader(""), &out)
	return out.String(), code
}

func TestExecuteGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "text report", args: fixtureArgs(), golden: "report.golden"},
		{name: "excluded mock package", args: fixtureArgs("--exclude", "/mock/"), golden: "excluded.golden"},
		{name: "single module", args: fixtureArgs("--mod", "other"), golden: "filtered.golden"},
		{name: "machine readable report", args: fixtureArgs("--format", "json"), golden: "report.json.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := runFixture(t, tt.args)
			path := fixturePath(tt.golden)
			if *update {
				if writeErr := os.WriteFile(path, []byte(got), 0o644); writeErr != nil {
					t.Fatalf("update %s: %v", path, writeErr)
				}
				return
			}
			want, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			if got != string(want) {
				t.Errorf("output does not match %s (run `go test ./... -update` to refresh)\n--- got ---\n%s", tt.golden, got)
			}
		})
	}
}

// TestExecuteIgnoresLinesWithoutStatements pins the rule the report lives or
// dies by: the fixture changes an import, a blank line and a comment, and none
// of them may reach the denominator.
func TestExecuteIgnoresLinesWithoutStatements(t *testing.T) {
	t.Parallel()
	got, _ := runFixture(t, fixtureArgs())
	if !strings.Contains(got, "internal/core/core.go:20-21\n") {
		t.Errorf("uncovered statements are missing:\n%s", got)
	}
	for _, unwanted := range []string{"core.go:3", "core.go:4", "core.go:5", "core.go:3-5"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("declaration line %q is reported as uncovered:\n%s", unwanted, got)
		}
	}
}

func TestExecuteIsDeterministic(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			first, _ := runFixture(t, fixtureArgs("--format", format))
			for i := range 5 {
				if got, _ := runFixture(t, fixtureArgs("--format", format)); got != first {
					t.Fatalf("run %d differs from the first run", i+2)
				}
			}
		})
	}
}

// TestJSONContract pins the field names a later consumer reads. Renaming any of
// them is a breaking change for that consumer.
func TestJSONContract(t *testing.T) {
	t.Parallel()
	out, _ := runFixture(t, fixtureArgs("--format", "json"))
	var doc map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &doc); unmarshalErr != nil {
		t.Fatalf("report JSON does not unmarshal: %v", unmarshalErr)
	}
	for _, key := range []string{"modules", "total", "unmeasured_modules", "warnings", "threshold"} {
		if _, found := doc[key]; !found {
			t.Errorf("top-level field %q is missing", key)
		}
	}

	modules, ok := doc["modules"].([]any)
	if !ok || len(modules) == 0 {
		t.Fatalf("modules is not a non-empty array: %#v", doc["modules"])
	}
	module, ok := modules[0].(map[string]any)
	if !ok {
		t.Fatalf("module entry is not an object: %#v", modules[0])
	}
	for _, key := range []string{"module", "total", "files"} {
		if _, found := module[key]; !found {
			t.Errorf("module field %q is missing", key)
		}
	}

	total, ok := doc["total"].(map[string]any)
	if !ok {
		t.Fatalf("total is not an object: %#v", doc["total"])
	}
	for _, key := range []string{"statements", "covered", "coverage"} {
		if _, found := total[key]; !found {
			t.Errorf("total field %q is missing", key)
		}
	}

	files, ok := module["files"].([]any)
	if !ok || len(files) == 0 {
		t.Fatalf("files is not a non-empty array: %#v", module["files"])
	}
	file, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("file entry is not an object: %#v", files[0])
	}
	for _, key := range []string{"path", "statements", "covered", "uncovered_lines"} {
		if _, found := file[key]; !found {
			t.Errorf("file field %q is missing", key)
		}
	}
}

func TestExecuteExitCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "no threshold never fails", args: fixtureArgs(), want: exitOK},
		{name: "below the threshold fails", args: fixtureArgs("--mod", "sample", "--threshold", "80"), want: exitBelow},
		{name: "above the threshold passes", args: fixtureArgs("--mod", "other", "--threshold", "80"), want: exitOK},
		{name: "exactly at the threshold passes", args: fixtureArgs("--mod", "other", "--threshold", "100"), want: exitOK},
		{
			name: "an unmeasured module cannot satisfy a threshold",
			args: fixtureArgs("--mod", "third-app", "--threshold", "0"),
			want: exitUnusable,
		},
		{
			name: "an unmeasured module fails even when the measured ones pass",
			args: fixtureArgs("--threshold", "0"),
			want: exitUnusable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, got := runFixture(t, tt.args); got != tt.want {
				t.Errorf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExecuteRejectsBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{name: "unknown format", args: fixtureArgs("--format", "html"), wantOut: `unknown --format "html"`},
		{name: "invalid exclude", args: fixtureArgs("--exclude", "["), wantOut: `invalid --exclude "["`},
		{name: "non numeric threshold", args: fixtureArgs("--threshold", "high"), wantOut: `invalid --threshold "high"`},
		{name: "threshold out of range", args: fixtureArgs("--threshold", "101"), wantOut: "between 0 and 100"},
		{name: "malformed profile pair", args: []string{"--profile", "alpha"}, wantOut: "want <module>=<path>"},
		{name: "missing diff file", args: []string{"--diff", fixturePath("nope.diff")}, wantOut: "no such file"},
		{name: "unknown flag", args: []string{"--nope"}, wantOut: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, code := runFixture(t, tt.args)
			if code != exitUnusable {
				t.Errorf("exit code = %d, want %d", code, exitUnusable)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out, tt.wantOut)
			}
		})
	}
}

func TestExecuteAcceptsAPercentSuffixedThreshold(t *testing.T) {
	t.Parallel()
	out, code := runFixture(t, fixtureArgs("--mod", "other", "--threshold", "80%"))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (output: %s)", code, exitOK, out)
	}
	if !strings.Contains(out, "threshold 80% -> PASS") {
		t.Errorf("output = %q, want a passing verdict", out)
	}
}

func TestExecuteReadsDiffFromStdin(t *testing.T) {
	t.Parallel()
	diff, readErr := os.ReadFile(fixturePath("sample.diff"))
	if readErr != nil {
		t.Fatalf("read fixture: %v", readErr)
	}
	var out bytes.Buffer
	args := []string{
		"--diff", "-",
		"--profile", "sample-app=" + fixturePath("sample-app.profile"),
		"--profile", "other-app=" + fixturePath("other-app.profile"),
		"--profile", "third-app=" + fixturePath("broken.profile"),
	}
	if code := execute(args, refuseToRun(t), bytes.NewReader(diff), &out); code != exitOK {
		t.Fatalf("exit code = %d, want %d (output: %s)", code, exitOK, out.String())
	}
	if !strings.Contains(out.String(), "patch coverage: 5/11") {
		t.Errorf("stdin diff produced no report:\n%s", out.String())
	}
}

func TestExecuteReportsNothingToMeasure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.diff")
	if writeErr := os.WriteFile(empty, nil, 0o600); writeErr != nil {
		t.Fatalf("write fixture: %v", writeErr)
	}
	var out bytes.Buffer
	code := execute([]string{"--diff", empty}, refuseToRun(t), strings.NewReader(""), &out)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if want := "no changed statements under scripts/\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// TestExecuteDefaultPath drives the path the tool actually takes when nothing
// is injected: resolve the repository root, ask git for the diff, then run each
// changed module's tests into a temporary profile. The runner stands in for git
// and go, so the orchestration is checked without a checkout of its own.
func TestExecuteDefaultPath(t *testing.T) {
	t.Parallel()
	diff, readErr := os.ReadFile(fixturePath("sample.diff"))
	if readErr != nil {
		t.Fatalf("read fixture: %v", readErr)
	}
	profiles := map[string]string{
		"other-app":  "mode: set\nother-app/main.go:5.2,7.1 2 1\n",
		"sample-app": "mode: set\nsample-app/cmd/sample-app/main.go:9.2,12.1 3 0\n",
	}

	var commands []string
	run := func(dir, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		switch {
		case name == "git" && args[0] == "rev-parse":
			return []byte("/repo\n"), nil
		case name == "git":
			return diff, nil
		case name == "go":
			module := filepath.Base(dir)
			dest := strings.TrimPrefix(args[len(args)-1], "-coverprofile=")
			content, known := profiles[module]
			if !known {
				return []byte("no test files"), errors.New("exit 1")
			}
			return nil, os.WriteFile(dest, []byte(content), 0o600)
		}
		return nil, nil
	}

	var out bytes.Buffer
	if code := execute(nil, run, strings.NewReader(""), &out); code != exitOK {
		t.Fatalf("exit code = %d, want %d (output: %s)", code, exitOK, out.String())
	}

	want := []string{
		"git rev-parse --show-toplevel",
		"git -c core.quotePath=false diff --unified=0 origin/main...HEAD",
		"go test ./... -count=1",
		"go test ./... -count=1",
		"go test ./... -count=1",
	}
	if len(commands) != len(want) {
		t.Fatalf("commands = %v, want %d of them", commands, len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(commands[i], prefix) {
			t.Errorf("command %d = %q, want it to start with %q", i, commands[i], prefix)
		}
	}
	if !strings.Contains(out.String(), "patch coverage: 2/5") {
		t.Errorf("report is missing the expected total:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "unmeasured modules (no coverage profile): third-app") {
		t.Errorf("a module whose tests failed must be reported as unmeasured:\n%s", out.String())
	}
}

// TestExecuteResolvesTheRootOnlyOnce guards the resolver's cache: git must not
// be asked for the repository root once per module.
func TestExecuteResolvesTheRootOnlyOnce(t *testing.T) {
	t.Parallel()
	rootCalls := 0
	run := func(dir, name string, args ...string) ([]byte, error) {
		if name == "git" && args[0] == "rev-parse" {
			rootCalls++
			return []byte("/repo\n"), nil
		}
		if name == "git" {
			return os.ReadFile(fixturePath("sample.diff"))
		}
		return []byte("no test files"), errors.New("exit 1")
	}
	var out bytes.Buffer
	execute(nil, run, strings.NewReader(""), &out)
	if rootCalls != 1 {
		t.Errorf("git rev-parse ran %d times, want 1", rootCalls)
	}
}

func TestExecuteReportsAnExplicitRootWithoutAskingGit(t *testing.T) {
	t.Parallel()
	run := func(dir, name string, args ...string) ([]byte, error) {
		if name == "git" && args[0] == "rev-parse" {
			t.Error("git rev-parse ran even though --root was given")
			return nil, nil
		}
		if dir != "/explicit" {
			t.Errorf("git ran in %q, want the explicit root", dir)
		}
		return nil, errors.New("exit 128")
	}
	var out bytes.Buffer
	code := execute([]string{"--root", "/explicit"}, run, strings.NewReader(""), &out)
	if code != exitUnusable {
		t.Errorf("exit code = %d, want %d", code, exitUnusable)
	}
	if !strings.Contains(out.String(), "git fetch origin") {
		t.Errorf("output = %q, want a fetch hint", out.String())
	}
}
