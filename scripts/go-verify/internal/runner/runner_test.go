package runner

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectChecks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		only    string
		want    []string
		wantErr bool
	}{
		{name: "empty runs all", only: "", want: []string{"goimports", "golangci-lint", "go test"}},
		{name: "lint subset", only: "lint", want: []string{"goimports", "golangci-lint"}},
		{name: "test subset", only: "test", want: []string{"go test"}},
		{name: "unknown errors", only: "bogus", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SelectChecks(tt.only)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SelectChecks(%q) err = %v, wantErr %v", tt.only, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SelectChecks(%q) = %v, want %v", tt.only, got, tt.want)
			}
		})
	}
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Layout: two modules (a, b/c) plus decoys that must be skipped.
	writeModules(t, root, []string{
		"a",
		filepath.Join("b", "c"),
		filepath.Join("vendor", "d"),   // pruned: vendor
		filepath.Join("testdata", "e"), // pruned: testdata
		filepath.Join(".hidden", "f"),  // pruned: dot-dir
	})

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{
		filepath.Join(root, "a"),
		filepath.Join(root, "b", "c"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover = %v, want %v", got, want)
	}
}

func TestRunCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		check      string
		out        []byte
		runErr     error
		wantOK     bool
		wantOutput string
	}{
		{name: "goimports clean", check: "goimports", out: []byte(""), wantOK: true},
		{name: "goimports diff", check: "goimports", out: []byte("main.go\n"), wantOK: false, wantOutput: "main.go"},
		{name: "golangci pass", check: "golangci-lint", out: []byte(""), wantOK: true},
		{name: "golangci fail", check: "golangci-lint", out: []byte("issue"), runErr: errors.New("exit 1"), wantOK: false, wantOutput: "issue\nexit 1"},
		{name: "go test pass", check: "go test", out: []byte("ok"), wantOK: true},
		{name: "go test fail", check: "go test", out: []byte("FAIL"), runErr: errors.New("exit 1"), wantOK: false, wantOutput: "FAIL\nexit 1"},
		{name: "unknown check", check: "bogus", wantOK: false, wantOutput: "unknown check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := func(_, _ string, _ ...string) ([]byte, error) {
				return tt.out, tt.runErr
			}
			got := runCheck(tt.check, "/mod", run)
			if got.OK != tt.wantOK {
				t.Fatalf("runCheck(%q) OK = %v, want %v", tt.check, got.OK, tt.wantOK)
			}
			if got.Output != tt.wantOutput {
				t.Fatalf("runCheck(%q) Output = %q, want %q", tt.check, got.Output, tt.wantOutput)
			}
		})
	}
}

func TestVerifyAll(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeModules(t, root, []string{"a", "b"})

	// Stub runner: goimports clean everywhere; golangci-lint fails only in "a".
	run := func(dir, name string, _ ...string) ([]byte, error) {
		if name == "golangci-lint" && filepath.Base(dir) == "a" {
			return []byte("bad"), errors.New("exit 1")
		}
		return []byte(""), nil
	}

	checks, err := VerifyAll(root, run, []string{"goimports", "golangci-lint"}, nil)
	if err != nil {
		t.Fatalf("VerifyAll: %v", err)
	}
	if len(checks) != 4 {
		t.Fatalf("VerifyAll returned %d checks, want 4", len(checks))
	}
	if !AnyFailed(checks) {
		t.Fatal("AnyFailed = false, want true (golangci-lint fails in module a)")
	}
	// Confirm exactly the golangci-lint check in module "a" failed.
	for _, c := range checks {
		wantFail := c.Mod == "a" && c.Name == "golangci-lint"
		if c.OK == wantFail {
			t.Fatalf("check %s/%s OK = %v, want %v", c.Mod, c.Name, c.OK, !wantFail)
		}
	}
}

func TestVerifyAllModFilter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeModules(t, root, []string{"a", "b"})

	run := func(_, _ string, _ ...string) ([]byte, error) { return []byte(""), nil }

	checks, err := VerifyAll(root, run, []string{"goimports"}, []string{"b"})
	if err != nil {
		t.Fatalf("VerifyAll: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("VerifyAll with filter returned %d checks, want 1", len(checks))
	}
	if checks[0].Mod != "b" {
		t.Fatalf("filtered check Mod = %q, want %q", checks[0].Mod, "b")
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
