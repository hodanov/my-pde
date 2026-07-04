package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"config-diff/internal/diff"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, src, dest string)
		args       func(src, dest string) []string
		wantCode   int
		wantOutput []string
	}{
		{
			name: "all entries ok",
			setup: func(t *testing.T, src, dest string) {
				t.Helper()
				writeFile(t, filepath.Join(src, "a.md"), "alpha")
				writeFile(t, filepath.Join(dest, "a.md"), "alpha")
			},
			args:       func(src, dest string) []string { return []string{"agents", src, dest} },
			wantCode:   0,
			wantOutput: []string{"ok       a.md"},
		},
		{
			name: "drift exits 1 with note",
			setup: func(t *testing.T, src, dest string) {
				t.Helper()
				writeFile(t, filepath.Join(src, "a.md"), "v1")
				writeFile(t, filepath.Join(dest, "a.md"), "v2")
			},
			args:       func(src, dest string) []string { return []string{"agents", src, dest} },
			wantCode:   1,
			wantOutput: []string{"drift    a.md", "content differs"},
		},
		{
			name: "missing exits 1",
			setup: func(t *testing.T, src, dest string) {
				t.Helper()
				writeFile(t, filepath.Join(src, "a.md"), "alpha")
			},
			args:       func(src, dest string) []string { return []string{"agents", src, dest} },
			wantCode:   1,
			wantOutput: []string{"missing  a.md"},
		},
		{
			name:       "wrong arg count exits 2 with usage",
			setup:      func(_ *testing.T, _, _ string) {},
			args:       func(_, _ string) []string { return []string{"agents", "only-two"} },
			wantCode:   2,
			wantOutput: []string{"Usage: config-diff <mode> <src> <dest>"},
		},
		{
			name:  "classify error exits 2",
			setup: func(_ *testing.T, _, _ string) {},
			args: func(src, dest string) []string {
				return []string{"agents", filepath.Join(src, "does-not-exist"), dest}
			},
			wantCode:   2,
			wantOutput: []string{"config-diff:", "source directory not found"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := t.TempDir()
			dest := t.TempDir()
			tt.setup(t, src, dest)

			var out bytes.Buffer
			code := execute(tt.args(src, dest), &out)
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

func TestWriteSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []diff.Entry
		want    string
	}{
		{
			name:    "no entries",
			entries: nil,
			want:    "agents -> /dest\n  (no entries)\n",
		},
		{
			name:    "ok entry",
			entries: []diff.Entry{{Label: "a.md", State: diff.StateOK}},
			want:    "agents -> /dest\n  ok       a.md\n",
		},
		{
			name:    "drift entry with note",
			entries: []diff.Entry{{Label: "s", State: diff.StateDrift, Note: "drift: SKILL.md"}},
			want:    "agents -> /dest\n  drift    s\n           drift: SKILL.md\n",
		},
		{
			name:    "drift entry without note",
			entries: []diff.Entry{{Label: "a.md", State: diff.StateDrift}},
			want:    "agents -> /dest\n  drift    a.md\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			writeSummary(&out, "agents", "/dest", tt.entries)
			if out.String() != tt.want {
				t.Fatalf("writeSummary = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

// writeFile writes content to path, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), mkErr)
	}
	if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
		t.Fatalf("WriteFile %s: %v", path, writeErr)
	}
}
