package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const line = `{"type":"assistant","timestamp":"%s","cwd":"%s","message":{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"x.go"}}]}}` + "\n"

// sidechainLine is what a subagent's own transcript contains: every line is
// flagged isSidechain.
const sidechainLine = `{"type":"assistant","timestamp":"%s","cwd":"%s","isSidechain":true,"message":{"model":"claude-sonnet-5","usage":{"input_tokens":4,"output_tokens":2},"content":[{"type":"tool_use","name":"Grep","input":{}}]}}` + "\n"

func writeTranscript(t *testing.T, dir, name, ts, cwd string) {
	t.Helper()
	writeLines(t, dir, name, fmt.Appendf(nil, line, ts, cwd))
}

func writeLines(t *testing.T, dir, name string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestCollectSkipsUnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permissions are not enforced")
	}
	root := t.TempDir()
	writeTranscript(t, root, "good.jsonl", "2026-07-30T11:30:00Z", "/home/u/proj")
	badPath := filepath.Join(root, "bad.jsonl")
	writeTranscript(t, root, "bad.jsonl", "2026-07-30T11:30:00Z", "/home/u/proj")
	if err := os.Chmod(badPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(badPath, 0o644) })

	got, err := collect(root, 0, "", time.Now())
	if err != nil {
		t.Fatalf("collect should skip the unreadable file rather than error: %v", err)
	}
	if len(got) != 1 || got[0].File != "good.jsonl" {
		t.Fatalf("collect = %+v, want only good.jsonl", got)
	}
}

func TestSessionKey(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/p/proj/abc.jsonl":                   "/p/proj/abc.jsonl",
		"/p/proj/abc/subagents/agent-1.jsonl": "/p/proj/abc.jsonl",
		// Only the subagents directory is folded; other nesting is left alone.
		"/p/proj/abc/other/x.jsonl": "/p/proj/abc/other/x.jsonl",
	}
	for in, want := range cases {
		if got := sessionKey(in); got != want {
			t.Errorf("sessionKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollectFoldsSubagentTranscripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	writeTranscript(t, proj, "sess.jsonl", "2026-08-22T10:00:00Z", "/home/u/proj")
	subs := filepath.Join(proj, "sess", subagentDir)
	writeLines(t, subs, "agent-a.jsonl", fmt.Appendf(nil, sidechainLine, "2026-08-22T10:01:00Z", "/home/u/proj"))
	writeLines(t, subs, "agent-b.jsonl", fmt.Appendf(nil, sidechainLine, "2026-08-22T10:02:00Z", "/home/u/proj"))

	got, err := collect(root, 0, "", time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("collect = %d sessions, want 1 (a subagent transcript is not a session)", len(got))
	}
	s := got[0]
	if s.File != "sess.jsonl" {
		t.Errorf("File = %q, want sess.jsonl", s.File)
	}
	if s.Main.AssistantTurns != 1 || s.Sub.AssistantTurns != 2 {
		t.Errorf("turns main/sub = %d/%d, want 1/2", s.Main.AssistantTurns, s.Sub.AssistantTurns)
	}
	if s.Sub.ToolCounts["Grep"] != 2 {
		t.Errorf("Sub.ToolCounts = %v, want Grep:2", s.Sub.ToolCounts)
	}
	// One timeline, not three: the subagents ran inside the parent's span.
	if s.Span() != 2*time.Minute {
		t.Errorf("Span = %s, want 2m", s.Span())
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
