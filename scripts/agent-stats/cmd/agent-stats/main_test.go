package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const line = `{"type":"assistant","timestamp":"%s","cwd":"%s","message":{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"x.go"}}]}}` + "\n"

func writeTranscript(t *testing.T, dir, name, ts, cwd string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(line, ts, cwd))
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectFilters(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	writeTranscript(t, filepath.Join(root, "proj-a"), "recent.jsonl", "2026-07-30T11:30:00Z", "/home/u/proj-a")
	writeTranscript(t, filepath.Join(root, "proj-b"), "old.jsonl", "2026-07-01T09:00:00Z", "/home/u/proj-b")

	all, err := collect(root, 0, "", now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("collect all = %d sessions, want 2", len(all))
	}

	recent, err := collect(root, time.Hour, "", now)
	if err != nil {
		t.Fatalf("collect since: %v", err)
	}
	if len(recent) != 1 || recent[0].File != "recent.jsonl" {
		t.Fatalf("since filter = %+v, want only recent.jsonl", recent)
	}

	byProject, err := collect(root, 0, "proj-b", now)
	if err != nil {
		t.Fatalf("collect project: %v", err)
	}
	if len(byProject) != 1 || byProject[0].File != "old.jsonl" {
		t.Fatalf("project filter = %+v, want only old.jsonl", byProject)
	}
}

func TestCollectMissingDir(t *testing.T) {
	t.Parallel()
	got, err := collect(filepath.Join(t.TempDir(), "nope"), 0, "", time.Now())
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing dir should yield nil, got %+v", got)
	}
}

func TestRunSmoke(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTranscript(t, root, "s.jsonl", "2026-07-30T11:30:00Z", "/home/u/x")
	if err := run([]string{"summary", "--dir", root}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := run([]string{"--dir", root, "--json"}); err != nil {
		t.Fatalf("run json: %v", err)
	}
}
