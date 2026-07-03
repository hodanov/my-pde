package diff

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStateString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state State
		want  string
	}{
		{name: "ok", state: StateOK, want: "ok"},
		{name: "drift", state: StateDrift, want: "drift"},
		{name: "missing", state: StateMissing, want: "missing"},
		{name: "unknown", state: State(99), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.state.String(); got != tt.want {
				t.Fatalf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestClassifyAgents(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dest := t.TempDir()
	// Source has two .md agents and one non-md file that must be ignored.
	writeFile(t, filepath.Join(src, "a.md"), "alpha")
	writeFile(t, filepath.Join(src, "b.md"), "beta")
	writeFile(t, filepath.Join(src, "ignore.txt"), "nope")
	// dest: a.md matches, b.md missing.
	writeFile(t, filepath.Join(dest, "a.md"), "alpha")

	entries, err := Classify("agents", src, dest)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	got := stateByLabel(entries)
	want := map[string]State{"a.md": StateOK, "b.md": StateMissing}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Classify agents = %v, want %v (ignore.txt must be excluded)", got, want)
	}
}

func TestClassifySettings(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dest := t.TempDir()
	// Recursive files; label is the path relative to src.
	writeFile(t, filepath.Join(src, "settings.json"), "{}")
	writeFile(t, filepath.Join(src, "hooks", "a.sh"), "echo a")
	writeFile(t, filepath.Join(src, "hooks", "b.sh"), "echo b")
	// dest: settings.json drifts, hooks/a.sh ok, hooks/b.sh missing.
	writeFile(t, filepath.Join(dest, "settings.json"), "{changed}")
	writeFile(t, filepath.Join(dest, "hooks", "a.sh"), "echo a")

	entries, err := Classify("settings", src, dest)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	got := stateByLabel(entries)
	want := map[string]State{
		"settings.json":                StateDrift,
		filepath.Join("hooks", "a.sh"): StateOK,
		filepath.Join("hooks", "b.sh"): StateMissing,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Classify settings = %v, want %v", got, want)
	}
}

func TestClassifySkills(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dest := t.TempDir()
	// One skill dir that is ok, one that drifts (a file changed + an extra file
	// in dest), one that is missing entirely.
	writeFile(t, filepath.Join(src, "ok-skill", "SKILL.md"), "ok")
	writeFile(t, filepath.Join(src, "drift-skill", "SKILL.md"), "v1")
	writeFile(t, filepath.Join(src, "missing-skill", "SKILL.md"), "m")

	writeFile(t, filepath.Join(dest, "ok-skill", "SKILL.md"), "ok")
	writeFile(t, filepath.Join(dest, "drift-skill", "SKILL.md"), "v2")
	writeFile(t, filepath.Join(dest, "drift-skill", "stale.txt"), "orphan")

	entries, err := Classify("skills", src, dest)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	got := stateByLabel(entries)
	want := map[string]State{
		"ok-skill":      StateOK,
		"drift-skill":   StateDrift,
		"missing-skill": StateMissing,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Classify skills = %v, want %v", got, want)
	}
	// The drift note must call out both the changed file and the extra file.
	note := noteByLabel(entries, "drift-skill")
	if note != "drift: SKILL.md, extra: stale.txt" {
		t.Fatalf("drift-skill note = %q, want %q", note, "drift: SKILL.md, extra: stale.txt")
	}
}

func TestClassifySettingsSkipsSymlinks(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dest := t.TempDir()
	// A regular file plus a symlink to it. copy-entries.sh `find -type f` never
	// enumerates the symlink, so config-diff must not report it either.
	writeFile(t, filepath.Join(src, "real.txt"), "content")
	if err := os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	entries, err := Classify("settings", src, dest)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	got := stateByLabel(entries)
	want := map[string]State{"real.txt": StateMissing}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Classify settings = %v, want %v (symlink link.txt must be excluded)", got, want)
	}
}

func TestClassifyUnknownMode(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	if _, err := Classify("bogus", src, src); err == nil {
		t.Fatal("Classify with unknown mode: want error, got nil")
	}
}

func TestClassifyMissingSource(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := Classify("agents", missing, t.TempDir()); err == nil {
		t.Fatal("Classify with missing source: want error, got nil")
	}
}

func TestAnyDivergent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []Entry
		want    bool
	}{
		{name: "all ok", entries: []Entry{{State: StateOK}, {State: StateOK}}, want: false},
		{name: "has drift", entries: []Entry{{State: StateOK}, {State: StateDrift}}, want: true},
		{name: "has missing", entries: []Entry{{State: StateMissing}}, want: true},
		{name: "empty", entries: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := AnyDivergent(tt.entries); got != tt.want {
				t.Fatalf("AnyDivergent = %v, want %v", got, tt.want)
			}
		})
	}
}

// stateByLabel indexes entry states by label for order-independent comparison.
func stateByLabel(entries []Entry) map[string]State {
	m := make(map[string]State, len(entries))
	for _, e := range entries {
		m[e.Label] = e.State
	}
	return m
}

// noteByLabel returns the note for the entry with the given label.
func noteByLabel(entries []Entry, label string) string {
	for _, e := range entries {
		if e.Label == label {
			return e.Note
		}
	}
	return ""
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
