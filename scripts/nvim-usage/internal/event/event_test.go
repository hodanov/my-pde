package event

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadDir(t *testing.T) {
	t.Parallel()
	day1 := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	keysLine := func(at time.Time, family string, keys string) string {
		return fmt.Sprintf(`{"kind":"keys","ts":%d,"family":%q,"ft":"go","keys":%s}`, at.Unix(), family, keys)
	}
	cmdLine := func(at time.Time, name string) string {
		return fmt.Sprintf(`{"kind":"cmd","ts":%d,"name":%q}`, at.Unix(), name)
	}
	searchLine := func(at time.Time) string {
		return fmt.Sprintf(`{"kind":"search","ts":%d}`, at.Unix())
	}
	days := func(dates ...string) map[string]struct{} {
		m := map[string]struct{}{}
		for _, d := range dates {
			m[d] = struct{}{}
		}
		return m
	}
	tests := []struct {
		name  string
		files map[string][]string
		since time.Time
		want  *Log
	}{
		{
			name: "keys, commands and searches are read",
			files: map[string][]string{"2026-09-01.jsonl": {
				keysLine(day1, FamilyNormal, `["j","<Space>y"]`),
				cmdLine(day1.Add(time.Minute), "vsplit"),
				searchLine(day1.Add(2 * time.Minute)),
			}},
			want: &Log{
				Chunks:   []Chunk{{At: day1, Family: FamilyNormal, Filetype: "go", Keys: []string{"j", "<Space>y"}}},
				Commands: map[string]int{"vsplit": 1},
				Searches: 1,
				First:    day1,
				Last:     day1.Add(2 * time.Minute),
				days:     days("2026-09-01"),
			},
		},
		{
			name: "malformed lines, unknown kinds, unknown families and empty chunks are skipped",
			files: map[string][]string{"2026-09-01.jsonl": {
				"not json",
				`{"kind":"unknown","ts":1}`,
				keysLine(day1, "insert", `["h","i"]`),
				keysLine(day1, FamilyNormal, `[]`),
				cmdLine(day1, ""),
				cmdLine(day1, "write"),
			}},
			want: &Log{
				Commands: map[string]int{"write": 1},
				First:    day1,
				Last:     day1,
				days:     days("2026-09-01"),
			},
		},
		{
			name: "events before since are dropped and files span days",
			files: map[string][]string{
				"2026-09-01.jsonl": {cmdLine(day1.Add(-time.Hour), "edit"), cmdLine(day1, "edit")},
				"2026-09-02.jsonl": {keysLine(day2, FamilyVisual, `["y"]`)},
			},
			since: day1,
			want: &Log{
				Chunks:   []Chunk{{At: day2, Family: FamilyVisual, Filetype: "go", Keys: []string{"y"}}},
				Commands: map[string]int{"edit": 1},
				First:    day1,
				Last:     day2,
				days:     days("2026-09-01", "2026-09-02"),
			},
		},
		{
			name:  "missing directory yields an empty log",
			files: nil,
			want:  &Log{Commands: map[string]int{}, days: map[string]struct{}{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "usage")
			for name, lines := range tt.files {
				path := filepath.Join(dir, "events", name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadDir(dir, tt.since)
			if err != nil {
				t.Fatalf("ReadDir returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadDir =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestReadKeymaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    *Snapshot
		wantErr bool
	}{
		{
			name:    "keymaps are read",
			content: `{"nvim":"0.12.5","keymaps":[{"mode":"n","lhs":"<Space>ff","desc":"Find Files","buffer_local":false}]}`,
			want: &Snapshot{Nvim: "0.12.5", Keymaps: []Keymap{
				{Mode: "n", LHS: "<Space>ff", Desc: "Find Files"},
			}},
		},
		{name: "missing file yields an empty snapshot", want: &Snapshot{}},
		{name: "malformed file is an error", content: "{", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "keymaps.json"), []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadKeymaps(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadKeymaps error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadKeymaps = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestKeymapFamily(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "normal mapping", mode: "n", want: FamilyNormal},
		{name: "operator-pending mapping is typed in normal chunks", mode: "o", want: FamilyNormal},
		{name: "visual mapping", mode: "x", want: FamilyVisual},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			k := Keymap{Mode: tt.mode}
			if got := k.Family(); got != tt.want {
				t.Errorf("Family() = %q, want %q", got, tt.want)
			}
		})
	}
}
