package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndLoadRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	records := []Record{
		{Prompt: "first", CWD: "/a", Timestamp: 1},
		{Prompt: "second", CWD: "/b", Timestamp: 2},
		{Prompt: "third", CWD: "/c", Timestamp: 3},
	}
	for _, rec := range records {
		if appendErr := Append(path, rec); appendErr != nil {
			t.Fatalf("Append(%q): %v", rec.Prompt, appendErr)
		}
	}

	got, loadErr := Load(path)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if len(got) != len(records) {
		t.Fatalf("got %d records, want %d", len(got), len(records))
	}
	// newest-first
	if got[0].Prompt != "third" || got[2].Prompt != "first" {
		t.Errorf("unexpected order: %q ... %q", got[0].Prompt, got[2].Prompt)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string // nil sentinel handled via write flag
		write   bool
		wantLen int
		wantErr bool
	}{
		{name: "missing file", write: false, wantLen: 0},
		{name: "empty file", write: true, content: "", wantLen: 0},
		{name: "blank lines skipped", write: true, content: "\n\n", wantLen: 0},
		{
			name:    "corrupt line skipped",
			write:   true,
			content: `{"prompt":"ok","cwd":"/x","timestamp":1}` + "\nnot json\n" + `{"prompt":"ok2","cwd":"/y","timestamp":2}` + "\n",
			wantLen: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "history.jsonl")
			if tt.write {
				if writeErr := os.WriteFile(path, []byte(tt.content), 0o600); writeErr != nil {
					t.Fatalf("setup: %v", writeErr)
				}
			}
			got, loadErr := Load(path)
			if (loadErr != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, wantErr %v", loadErr, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d records, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestWriteRequest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	rec := Record{Prompt: "replay me", CWD: "/work", Timestamp: 42}
	if writeErr := WriteRequest(path, rec); writeErr != nil {
		t.Fatalf("WriteRequest: %v", writeErr)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	var got Record
	if unmarshalErr := json.Unmarshal(data, &got); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	if got != rec {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, rec)
	}

	// No leftover temp files in the directory.
	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatalf("readdir: %v", readDirErr)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestFormatList(t *testing.T) {
	t.Parallel()
	records := []Record{
		{Prompt: "newest\nsecond line", CWD: "/a", Timestamp: 100},
		{Prompt: "older", CWD: "/b", Timestamp: 50},
	}
	tests := []struct {
		name     string
		records  []Record
		limit    int
		contains []string
		excludes []string
	}{
		{
			name:     "all",
			records:  records,
			limit:    0,
			contains: []string{"newest", "older", "/a", "/b", "  0  ", "  1  "},
			excludes: []string{"second line"},
		},
		{
			name:     "limit 1",
			records:  records,
			limit:    1,
			contains: []string{"newest", "/a"},
			excludes: []string{"older", "/b"},
		},
		{
			name:     "empty",
			records:  nil,
			limit:    0,
			contains: []string{"(no history)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := FormatList(tt.records, tt.limit)
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			for _, no := range tt.excludes {
				if strings.Contains(out, no) {
					t.Errorf("output unexpectedly contains %q:\n%s", no, out)
				}
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 150)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single line", in: "hello", want: "hello"},
		{name: "multi line", in: "a\nb", want: "a"},
		{name: "truncated", in: long, want: long[:100] + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstLine(tt.in); got != tt.want {
				t.Errorf("firstLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
