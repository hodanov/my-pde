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

// splitTurnFixture mirrors real Claude Code transcripts: one logical assistant
// turn (thinking -> text -> tool_use) is written as multiple JSONL lines that
// share the same message.id and each repeat the whole turn's usage.
const splitTurnFixture = `
{"type":"assistant","timestamp":"2026-08-06T10:00:00Z","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2,"cache_creation_input_tokens":1},"content":[{"type":"thinking","text":"..."}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:01Z","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2,"cache_creation_input_tokens":1},"content":[{"type":"text","text":"ok"}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:02Z","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2,"cache_creation_input_tokens":1},"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"a.go"}}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:03Z","message":{"id":"msg_2","model":"claude-sonnet-5","usage":{"input_tokens":3,"output_tokens":2},"content":[{"type":"text","text":"done"}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:04Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"text","text":"legacy, no id"}]}}
`

func TestParseReaderDedupesSplitTurns(t *testing.T) {
	t.Parallel()
	s := ParseReader("split.jsonl", strings.NewReader(splitTurnFixture))

	// msg_1 (3 lines) + msg_2 (1 line) count as 2 real turns; the id-less line
	// cannot be deduped, so it is counted as a 3rd turn.
	if s.AssistantTurns != 3 {
		t.Errorf("AssistantTurns = %d, want 3 (msg_1 once + msg_2 once + id-less line)", s.AssistantTurns)
	}
	// msg_1's usage must be counted once despite appearing on 3 lines.
	want := Tokens{Input: 14, Output: 8, CacheRead: 2, CacheCreation: 1}
	if s.Tokens != want {
		t.Errorf("Tokens = %+v, want %+v (msg_1 usage counted once, not 3x)", s.Tokens, want)
	}
	// tool_use only appears on one of msg_1's lines, so it must still be counted.
	if s.ToolCounts["Edit"] != 1 {
		t.Errorf("ToolCounts[Edit] = %d, want 1", s.ToolCounts["Edit"])
	}
	if s.FileCounts["a.go"] != 1 {
		t.Errorf("FileCounts[a.go] = %d, want 1", s.FileCounts["a.go"])
	}
}

// skillFixture mirrors how the Skill tool records which skill was invoked:
// tool_use name "Skill" with an input.skill field. Not every Skill call is
// guaranteed to carry that field (schema drift), so one line omits it.
const skillFixture = `
{"type":"assistant","timestamp":"2026-08-06T10:00:00Z","message":{"id":"msg_1","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Skill","input":{"skill":"dev-workflow"}}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:01Z","message":{"id":"msg_2","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Skill","input":{"skill":"dev-workflow","args":"do the thing"}}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:02Z","message":{"id":"msg_3","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Skill","input":{"skill":"review"}}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:03Z","message":{"id":"msg_4","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Skill","input":{}}]}}
{"type":"assistant","timestamp":"2026-08-06T10:00:04Z","message":{"id":"msg_5","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
`

func TestParseReaderSkillCounts(t *testing.T) {
	t.Parallel()
	s := ParseReader("skills.jsonl", strings.NewReader(skillFixture))

	// Every Skill invocation still counts toward the generic tool tally,
	// including the one with no input.skill field.
	if s.ToolCounts["Skill"] != 4 {
		t.Errorf("ToolCounts[Skill] = %d, want 4", s.ToolCounts["Skill"])
	}
	if s.SkillCounts["dev-workflow"] != 2 {
		t.Errorf("SkillCounts[dev-workflow] = %d, want 2", s.SkillCounts["dev-workflow"])
	}
	if s.SkillCounts["review"] != 1 {
		t.Errorf("SkillCounts[review] = %d, want 1", s.SkillCounts["review"])
	}
	if len(s.SkillCounts) != 2 {
		t.Errorf("SkillCounts = %v, want only dev-workflow and review (no entry for the id-less call)", s.SkillCounts)
	}
	// A non-Skill tool must never leak into SkillCounts.
	if _, ok := s.SkillCounts["ls"]; ok {
		t.Errorf("SkillCounts must not contain non-Skill tool data, got %v", s.SkillCounts)
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
