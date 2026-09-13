package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

func fixturePath(name string) string { return filepath.Join("..", "..", "testdata", name) }

func TestExecuteGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dir    string
		format string
		golden string
	}{
		{name: "markdown report", dir: fixturePath("usage"), format: "md", golden: "report.md.golden"},
		{name: "json report", dir: fixturePath("usage"), format: "json", golden: "report.json.golden"},
		{name: "missing log directory", dir: fixturePath("missing"), format: "md", golden: "empty.md.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			args := []string{"--dir", tt.dir, "--now", "2026-09-03T00:00:00Z", "--since", "30d", "--format", tt.format}
			if code := execute(args, &stdout, &stderr); code != 0 {
				t.Fatalf("execute code = %d, stderr = %s", code, stderr.String())
			}
			path := fixturePath(tt.golden)
			if *update {
				if err := os.WriteFile(path, stdout.Bytes(), 0o644); err != nil {
					t.Fatalf("update %s: %v", path, err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if stdout.String() != string(want) {
				t.Errorf("output does not match %s (run `go test ./... -update` to refresh)\n--- got ---\n%s", tt.golden, stdout.String())
			}
		})
	}
}

func TestExecuteRejectsInvalidOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown format", args: []string{"--format", "csv"}},
		{name: "unparsable window", args: []string{"--since", "month"}},
		{name: "negative days", args: []string{"--since", "-1d"}},
		{name: "unparsable now", args: []string{"--now", "yesterday"}},
		{name: "positional argument", args: []string{"report"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			args := append([]string{"--dir", fixturePath("usage")}, tt.args...)
			if code := execute(args, &stdout, &stderr); code != 2 {
				t.Errorf("execute code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestParseWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "days suffix", input: "30d", want: 30 * 24 * time.Hour},
		{name: "go duration", input: "36h", want: 36 * time.Hour},
		{name: "zero keeps everything", input: "0", want: 0},
		{name: "negative days", input: "-1d", wantErr: true},
		{name: "negative duration", input: "-1h", wantErr: true},
		{name: "non-numeric days", input: "xd", wantErr: true},
		{name: "unknown unit", input: "1w", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseWindow(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWindow(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseWindow(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
