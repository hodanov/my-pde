package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTree materializes a minimal ai-agents root with the given skill and
// agent files, returning the root path. skills/agents map a name to file body.
func writeTree(t *testing.T, skills, agents map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range skills {
		write(t, filepath.Join(root, "skills", name, "SKILL.md"), body)
	}
	for name, body := range agents {
		write(t, filepath.Join(root, "agents", name+".md"), body)
	}
	// Ensure both subtrees exist even when a map is empty.
	mustMkdir(t, filepath.Join(root, "skills"))
	mustMkdir(t, filepath.Join(root, "agents"))
	return root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if writeErr := os.WriteFile(path, []byte(body), 0o644); writeErr != nil {
		t.Fatalf("write %s: %v", path, writeErr)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		t.Fatalf("mkdir %s: %v", dir, mkErr)
	}
}

const (
	okSkill    = "---\nname: ok\ndescription: A fine skill.\n---\n# body\n"
	warnSkill  = "---\nname: ok\ndescription: A fine skill.\nbogus: x\n---\n# body\n"
	errSkill   = "---\nname: mismatch\ndescription: A fine skill.\n---\n# body\n" // name != dir "ok"
	validAgent = "---\nname: helper\ndescription: A fine agent.\ntools: Read\nmodel: sonnet\n---\nbody\n"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		skills   map[string]string
		strict   bool
		extraArg bool
		badFlag  bool
		wantCode int
	}{
		{name: "clean tree", skills: map[string]string{"ok": okSkill}, wantCode: 0},
		{name: "error fails", skills: map[string]string{"ok": errSkill}, wantCode: 1},
		{name: "warn passes without strict", skills: map[string]string{"ok": warnSkill}, wantCode: 0},
		{name: "warn fails under strict", skills: map[string]string{"ok": warnSkill}, strict: true, wantCode: 1},
		{name: "unexpected positional arg", skills: map[string]string{"ok": okSkill}, extraArg: true, wantCode: 2},
		{name: "unknown flag", skills: map[string]string{"ok": okSkill}, badFlag: true, wantCode: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, tt.skills, map[string]string{"helper": validAgent})
			args := []string{"--root", root}
			if tt.strict {
				args = append(args, "--strict")
			}
			if tt.extraArg {
				args = append(args, "leftover")
			}
			if tt.badFlag {
				args = append(args, "--nope")
			}
			var out, errOut bytes.Buffer
			code := execute(args, &out, &errOut)
			if code != tt.wantCode {
				t.Fatalf("execute code = %d, want %d (out=%q err=%q)", code, tt.wantCode, out.String(), errOut.String())
			}
		})
	}
}
