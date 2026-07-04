package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-verify/internal/runner"
)

// passRunner is a stub Runner where every check succeeds.
func passRunner(_, _ string, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

// failRunner is a stub Runner where golangci-lint fails with output.
func failRunner(_, name string, _ ...string) ([]byte, error) {
	if name == "golangci-lint" {
		return []byte("pkg.go:1: issue"), errors.New("exit 1")
	}
	return []byte(""), nil
}

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		modules    []string
		only       string
		mod        string
		run        runner.Runner
		rootFn     func(root string) string // overrides the module root when set
		wantCode   int
		wantOutput []string
	}{
		{
			name:       "all checks pass",
			modules:    []string{"a"},
			run:        passRunner,
			wantCode:   0,
			wantOutput: []string{"PASS  goimports", "PASS  golangci-lint", "PASS  go test"},
		},
		{
			name:       "check failure exits 1 with indented detail",
			modules:    []string{"a"},
			run:        failRunner,
			wantCode:   1,
			wantOutput: []string{"FAIL  golangci-lint", "  pkg.go:1: issue"},
		},
		{
			name:       "mod filter narrows modules",
			modules:    []string{"alpha", "beta"},
			only:       "test",
			mod:        "beta",
			run:        passRunner,
			wantCode:   0,
			wantOutput: []string{"PASS  go test        beta"},
		},
		{
			name:       "unknown only value exits 2",
			modules:    []string{"a"},
			only:       "bogus",
			run:        passRunner,
			wantCode:   2,
			wantOutput: []string{"unknown -only value"},
		},
		{
			name:       "no modules found exits 2",
			modules:    nil,
			run:        passRunner,
			wantCode:   2,
			wantOutput: []string{"no Go modules found"},
		},
		{
			name:     "discover error exits 2",
			modules:  nil,
			run:      passRunner,
			rootFn:   func(root string) string { return filepath.Join(root, "does-not-exist") },
			wantCode: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeModules(t, root, tt.modules)
			if tt.rootFn != nil {
				root = tt.rootFn(root)
			}

			var out bytes.Buffer
			code := execute(root, tt.only, tt.mod, tt.run, &out)
			if code != tt.wantCode {
				t.Fatalf("execute code = %d, want %d (output: %q)", code, tt.wantCode, out.String())
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("execute output = %q, want it to contain %q", out.String(), want)
				}
			}
		})
	}
}

func TestExecuteFilteredModuleDoesNotFail(t *testing.T) {
	t.Parallel()
	// A failure in a filtered-out module must not affect the exit code.
	root := t.TempDir()
	writeModules(t, root, []string{"alpha", "beta"})
	run := func(dir, _ string, _ ...string) ([]byte, error) {
		if filepath.Base(dir) == "alpha" {
			return []byte("broken"), errors.New("exit 1")
		}
		return []byte(""), nil
	}

	var out bytes.Buffer
	code := execute(root, "", "beta", run, &out)
	if code != 0 {
		t.Fatalf("execute code = %d, want 0 (alpha is filtered out; output: %q)", code, out.String())
	}
}

func TestWriteSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		checks []runner.Check
		want   string
	}{
		{
			name:   "pass line",
			checks: []runner.Check{{Mod: "a", Name: "go test", OK: true}},
			want:   "PASS  go test        a\n",
		},
		{
			name:   "fail line with detail",
			checks: []runner.Check{{Mod: "a", Name: "goimports", OK: false, Output: "x.go\ny.go"}},
			want:   "FAIL  goimports      a\n  x.go\n  y.go\n",
		},
		{
			name:   "fail line without detail",
			checks: []runner.Check{{Mod: "a", Name: "go test", OK: false}},
			want:   "FAIL  go test        a\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			writeSummary(&out, tt.checks)
			if out.String() != tt.want {
				t.Fatalf("writeSummary = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestIndent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single line", in: "a", want: "  a"},
		{name: "multi line", in: "a\nb", want: "  a\n  b"},
		{name: "empty", in: "", want: "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := indent(tt.in); got != tt.want {
				t.Fatalf("indent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultRunner(t *testing.T) {
	t.Parallel()
	// go is guaranteed to exist because it is running this test.
	out, runErr := defaultRunner(t.TempDir(), "go", "version")
	if runErr != nil {
		t.Fatalf("defaultRunner(go version): %v", runErr)
	}
	if !strings.Contains(string(out), "go version") {
		t.Fatalf("defaultRunner output = %q, want it to contain %q", out, "go version")
	}

	if _, missErr := defaultRunner(t.TempDir(), "go-verify-no-such-command"); missErr == nil {
		t.Fatal("defaultRunner with missing command: want error, got nil")
	}
}

// writeModules creates a go.mod under each relative path below root.
func writeModules(t *testing.T, root string, rels []string) {
	t.Helper()
	for _, rel := range rels {
		dir := filepath.Join(root, rel)
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			t.Fatalf("MkdirAll %s: %v", dir, mkErr)
		}
		mod := filepath.Join(dir, "go.mod")
		if writeErr := os.WriteFile(mod, []byte("module x\n\ngo 1.26\n"), 0o644); writeErr != nil {
			t.Fatalf("WriteFile %s: %v", mod, writeErr)
		}
	}
}
