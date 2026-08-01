package parser

import (
	"strings"
	"testing"
	"time"
)

const fixture = `
{"type":"user","timestamp":"2026-07-30T10:00:00Z","cwd":"/home/u/proj","gitBranch":"main","message":{"content":"hi"}}
{"type":"assistant","timestamp":"2026-07-30T10:00:05Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":10,"cache_creation_input_tokens":5},"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Edit","input":{"file_path":"a.go"}}]}}
{"type":"assistant","timestamp":"2026-07-30T10:01:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":50,"output_tokens":20},"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"a.go"}},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
this is not json and must be skipped
{"type":"assistant","timestamp":"2026-07-30T10:02:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":5,"output_tokens":1},"content":"a plain string content"}}
`

func TestParseReader(t *testing.T) {
	t.Parallel()
	s := ParseReader("session.jsonl", strings.NewReader(fixture))

	if s.File != "session.jsonl" {
		t.Errorf("File = %q, want session.jsonl", s.File)
	}
	if s.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want claude-opus-4-8", s.Model)
	}
	if s.Cwd != "/home/u/proj" || s.GitBranch != "main" {
		t.Errorf("cwd/branch = %q/%q", s.Cwd, s.GitBranch)
	}
	if s.AssistantTurns != 3 {
		t.Errorf("AssistantTurns = %d, want 3", s.AssistantTurns)
	}
	want := Tokens{Input: 155, Output: 61, CacheRead: 10, CacheCreation: 5}
	if s.Tokens != want {
		t.Errorf("Tokens = %+v, want %+v", s.Tokens, want)
	}
	if s.ToolCounts["Edit"] != 2 || s.ToolCounts["Bash"] != 1 {
		t.Errorf("ToolCounts = %v, want Edit:2 Bash:1", s.ToolCounts)
	}
	if s.FileCounts["a.go"] != 2 {
		t.Errorf("FileCounts[a.go] = %d, want 2", s.FileCounts["a.go"])
	}
	if got := s.Duration(); got != 2*time.Minute {
		t.Errorf("Duration = %s, want 2m0s", got)
	}
}

func TestParseReaderEmpty(t *testing.T) {
	t.Parallel()
	s := ParseReader("empty.jsonl", strings.NewReader(""))
	if s.AssistantTurns != 0 {
		t.Errorf("AssistantTurns = %d, want 0", s.AssistantTurns)
	}
	if s.Duration() != 0 {
		t.Errorf("Duration = %s, want 0", s.Duration())
	}
	if len(s.ToolCounts) != 0 || len(s.FileCounts) != 0 {
		t.Errorf("expected empty maps, got tools=%v files=%v", s.ToolCounts, s.FileCounts)
	}
}

func TestDurationGuards(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	start := end.Add(time.Hour)
	s := Session{Start: start, End: end} // end before start
	if s.Duration() != 0 {
		t.Errorf("Duration with end<start = %s, want 0", s.Duration())
	}
}
