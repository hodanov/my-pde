package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTemplates lays down the minimal template files run() reads from root.
func writeTemplates(t *testing.T, root string) {
	t.Helper()
	ci := "name: CI config-diff\n" +
		"on:\n  pull_request:\n    paths:\n      - \"scripts/config-diff/**\"\n" +
		"jobs:\n  ci:\n    uses: ./.github/workflows/go_module_ci.yml\n    with:\n      module: config-diff\n"
	mise := "# ---- config-diff (Go) ----\n\n" +
		"[tasks.\"config-diff:build\"]\ndir = \"scripts/config-diff\"\n\n" +
		"# ---- next (Go) ----\n"
	mustWrite(t, filepath.Join(root, ".github", "workflows", "ci_config_diff.yml"), ci)
	mustWrite(t, filepath.Join(root, "mise.toml"), mise)
	// NewSpec requires the --from module to exist under scripts/.
	mustWrite(t, filepath.Join(root, "scripts", "config-diff", "go.mod"), "module config-diff\n")
}

func mustWrite(t *testing.T, abs, content string) {
	t.Helper()
	if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(abs), mkErr)
	}
	if writeErr := os.WriteFile(abs, []byte(content), 0o644); writeErr != nil {
		t.Fatalf("WriteFile %s: %v", abs, writeErr)
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     func(root string) []string
		setup    func(t *testing.T, root string)
		wantCode int
		wantFile string // repository-relative path expected to exist after a successful run
	}{
		{
			name:     "generates module",
			args:     func(root string) []string { return []string{"new", "log-tail", "--root", root} },
			setup:    writeTemplates,
			wantCode: 0,
			wantFile: "scripts/log-tail/cmd/log-tail/main.go",
		},
		{
			name:     "no subcommand is usage error",
			args:     func(string) []string { return nil },
			setup:    func(*testing.T, string) {},
			wantCode: 2,
		},
		{
			name:     "wrong arg count is usage error",
			args:     func(root string) []string { return []string{"new", "--root", root} },
			setup:    writeTemplates,
			wantCode: 2,
		},
		{
			name:     "missing templates is generation error",
			args:     func(root string) []string { return []string{"new", "log-tail", "--root", root} },
			setup:    func(*testing.T, string) {},
			wantCode: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tt.setup(t, root)

			var out, errOut bytes.Buffer
			code := run(tt.args(root), &out, &errOut)
			if code != tt.wantCode {
				t.Fatalf("run code = %d, want %d (stderr: %q)", code, tt.wantCode, errOut.String())
			}
			if tt.wantFile != "" {
				if _, statErr := os.Stat(filepath.Join(root, tt.wantFile)); statErr != nil {
					t.Fatalf("expected %s to exist: %v", tt.wantFile, statErr)
				}
			}
		})
	}
}

func TestRunRefusesOverwrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTemplates(t, root)
	// Pre-create a target file so generation must refuse.
	mustWrite(t, filepath.Join(root, "scripts", "log-tail", "go.mod"), "module log-tail\n")

	var out, errOut bytes.Buffer
	code := run([]string{"new", "log-tail", "--root", root}, &out, &errOut)
	if code != 1 {
		t.Fatalf("run code = %d, want 1", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("already exists")) {
		t.Fatalf("stderr = %q, want it to mention already exists", errOut.String())
	}
}
