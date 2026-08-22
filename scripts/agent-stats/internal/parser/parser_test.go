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
	if s.Cwd != "/home/u/proj" || s.GitBranch != "main" {
		t.Errorf("cwd/branch = %q/%q", s.Cwd, s.GitBranch)
	}
	if s.AssistantTurns() != 3 {
		t.Errorf("AssistantTurns = %d, want 3", s.AssistantTurns())
	}
	want := Tokens{Input: 155, Output: 61, CacheRead: 10, CacheCreation: 5}
	if got := s.Tokens(); got != want {
		t.Errorf("Tokens = %+v, want %+v", got, want)
	}
	// Every line lacks isSidechain, so all of it is main-loop work.
	if s.Sub.AssistantTurns != 0 {
		t.Errorf("Sub.AssistantTurns = %d, want 0", s.Sub.AssistantTurns)
	}
	if u := s.Main.ByModel["claude-opus-4-8"]; u.Turns != 3 || u.Tokens != want {
		t.Errorf("ByModel[claude-opus-4-8] = %+v, want turns 3 and %+v", u, want)
	}
	if s.Main.ToolCounts["Edit"] != 2 || s.Main.ToolCounts["Bash"] != 1 {
		t.Errorf("ToolCounts = %v, want Edit:2 Bash:1", s.Main.ToolCounts)
	}
	if s.Main.FileCounts["a.go"] != 2 {
		t.Errorf("FileCounts[a.go] = %d, want 2", s.Main.FileCounts["a.go"])
	}
	if got := s.Span(); got != 2*time.Minute {
		t.Errorf("Span = %s, want 2m0s", got)
	}
}

func TestParseReaderEmpty(t *testing.T) {
	t.Parallel()
	s := ParseReader("empty.jsonl", strings.NewReader(""))
	if s.AssistantTurns() != 0 {
		t.Errorf("AssistantTurns = %d, want 0", s.AssistantTurns())
	}
	if s.Span() != 0 || s.ActiveDuration != 0 {
		t.Errorf("Span/ActiveDuration = %s/%s, want 0/0", s.Span(), s.ActiveDuration)
	}
	if len(s.Main.ToolCounts) != 0 || len(s.Main.FileCounts) != 0 {
		t.Errorf("expected empty maps, got tools=%v files=%v", s.Main.ToolCounts, s.Main.FileCounts)
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
	if s.AssistantTurns() != 3 {
		t.Errorf("AssistantTurns = %d, want 3 (msg_1 once + msg_2 once + id-less line)", s.AssistantTurns())
	}
	// msg_1's usage must be counted once despite appearing on 3 lines.
	want := Tokens{Input: 14, Output: 8, CacheRead: 2, CacheCreation: 1}
	if got := s.Tokens(); got != want {
		t.Errorf("Tokens = %+v, want %+v (msg_1 usage counted once, not 3x)", got, want)
	}
	// Per-model attribution must be deduped on the same basis as the totals.
	if u := s.Main.ByModel["claude-sonnet-5"]; u.Turns != 3 || u.Tokens != want {
		t.Errorf("ByModel[claude-sonnet-5] = %+v, want turns 3 and %+v", u, want)
	}
	// tool_use only appears on one of msg_1's lines, so it must still be counted.
	if s.Main.ToolCounts["Edit"] != 1 {
		t.Errorf("ToolCounts[Edit] = %d, want 1", s.Main.ToolCounts["Edit"])
	}
	if s.Main.FileCounts["a.go"] != 1 {
		t.Errorf("FileCounts[a.go] = %d, want 1", s.Main.FileCounts["a.go"])
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
	if s.Main.ToolCounts["Skill"] != 4 {
		t.Errorf("ToolCounts[Skill] = %d, want 4", s.Main.ToolCounts["Skill"])
	}
	if s.Main.SkillCounts["dev-workflow"] != 2 {
		t.Errorf("SkillCounts[dev-workflow] = %d, want 2", s.Main.SkillCounts["dev-workflow"])
	}
	if s.Main.SkillCounts["review"] != 1 {
		t.Errorf("SkillCounts[review] = %d, want 1", s.Main.SkillCounts["review"])
	}
	if len(s.Main.SkillCounts) != 2 {
		t.Errorf("SkillCounts = %v, want only dev-workflow and review (no entry for the id-less call)", s.Main.SkillCounts)
	}
	// A non-Skill tool must never leak into SkillCounts.
	if _, ok := s.Main.SkillCounts["ls"]; ok {
		t.Errorf("SkillCounts must not contain non-Skill tool data, got %v", s.Main.SkillCounts)
	}
}

// sidechainFixture mixes main-loop and subagent lines. Claude Code sets
// isSidechain on every line a subagent produces; a line that omits the flag is
// main-loop work.
const sidechainFixture = `
{"type":"assistant","timestamp":"2026-08-22T10:00:00Z","isSidechain":false,"message":{"id":"m1","model":"claude-opus-5","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"tool_use","name":"Agent","input":{}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:01Z","isSidechain":true,"message":{"id":"m2","model":"claude-sonnet-5","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"tool_use","name":"Grep","input":{}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:02Z","isSidechain":true,"message":{"id":"m3","model":"claude-sonnet-5","usage":{"input_tokens":7,"output_tokens":3},"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"sub.go"}},{"type":"tool_use","name":"Skill","input":{"skill":"review"}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:03Z","message":{"id":"m4","model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"text","text":"done"}]}}
`

func TestParseReaderSplitsSidechain(t *testing.T) {
	t.Parallel()
	s := ParseReader("sidechain.jsonl", strings.NewReader(sidechainFixture))

	if s.Main.AssistantTurns != 2 || s.Sub.AssistantTurns != 2 {
		t.Errorf("turns main/sub = %d/%d, want 2/2", s.Main.AssistantTurns, s.Sub.AssistantTurns)
	}
	wantMain := Tokens{Input: 11, Output: 6}
	if s.Main.Tokens != wantMain {
		t.Errorf("Main.Tokens = %+v, want %+v", s.Main.Tokens, wantMain)
	}
	wantSub := Tokens{Input: 107, Output: 53}
	if s.Sub.Tokens != wantSub {
		t.Errorf("Sub.Tokens = %+v, want %+v", s.Sub.Tokens, wantSub)
	}
	if got := s.Tokens(); got != (Tokens{Input: 118, Output: 59}) {
		t.Errorf("Tokens = %+v, want the two scopes combined", got)
	}
	// Tool use must land in the scope that made the call, not be pooled.
	if s.Main.ToolCounts["Agent"] != 1 || len(s.Main.ToolCounts) != 1 {
		t.Errorf("Main.ToolCounts = %v, want only Agent:1", s.Main.ToolCounts)
	}
	if s.Sub.ToolCounts["Grep"] != 1 || s.Sub.ToolCounts["Edit"] != 1 || s.Sub.ToolCounts["Skill"] != 1 {
		t.Errorf("Sub.ToolCounts = %v, want Grep:1 Edit:1 Skill:1", s.Sub.ToolCounts)
	}
	if len(s.Main.FileCounts) != 0 || s.Sub.FileCounts["sub.go"] != 1 {
		t.Errorf("FileCounts main/sub = %v/%v, want empty/sub.go:1", s.Main.FileCounts, s.Sub.FileCounts)
	}
	if len(s.Main.SkillCounts) != 0 || s.Sub.SkillCounts["review"] != 1 {
		t.Errorf("SkillCounts main/sub = %v/%v, want empty/review:1", s.Main.SkillCounts, s.Sub.SkillCounts)
	}
}

// modelFixture has two real models in one session plus a "<synthetic>" turn,
// which the CLI generates itself rather than by inference.
const modelFixture = `
{"type":"assistant","timestamp":"2026-08-22T10:00:00Z","message":{"id":"m1","model":"claude-opus-5","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2}}}
{"type":"assistant","timestamp":"2026-08-22T10:00:01Z","message":{"id":"m2","model":"claude-sonnet-5","usage":{"input_tokens":20,"output_tokens":7}}}
{"type":"assistant","timestamp":"2026-08-22T10:00:02Z","message":{"id":"m3","model":"claude-sonnet-5","usage":{"input_tokens":1,"output_tokens":1}}}
{"type":"assistant","timestamp":"2026-08-22T10:00:03Z","message":{"id":"m4","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}
`

func TestParseReaderTokensByModel(t *testing.T) {
	t.Parallel()
	s := ParseReader("models.jsonl", strings.NewReader(modelFixture))

	if len(s.Main.ByModel) != 2 {
		t.Fatalf("ByModel = %v, want only the two real models", s.Main.ByModel)
	}
	wantOpus := Usage{Turns: 1, Tokens: Tokens{Input: 10, Output: 5, CacheRead: 2}}
	if got := s.Main.ByModel["claude-opus-5"]; got != wantOpus {
		t.Errorf("ByModel[claude-opus-5] = %+v, want %+v", got, wantOpus)
	}
	wantSonnet := Usage{Turns: 2, Tokens: Tokens{Input: 21, Output: 8}}
	if got := s.Main.ByModel["claude-sonnet-5"]; got != wantSonnet {
		t.Errorf("ByModel[claude-sonnet-5] = %+v, want %+v", got, wantSonnet)
	}
	if _, ok := s.Main.ByModel["<synthetic>"]; ok {
		t.Errorf("placeholder model must not appear in ByModel: %v", s.Main.ByModel)
	}
	// Excluded from attribution, but still counted so the gap is visible.
	if s.SyntheticTurns != 1 {
		t.Errorf("SyntheticTurns = %d, want 1", s.SyntheticTurns)
	}
	if s.AssistantTurns() != 4 {
		t.Errorf("AssistantTurns = %d, want 4 (synthetic turns still count as turns)", s.AssistantTurns())
	}
}

// durationFixture carries the CLI's own turn timings. Other system subtypes
// also have a durationMs field, so only turn_duration may contribute.
const durationFixture = `
{"type":"system","subtype":"turn_duration","timestamp":"2026-08-22T10:00:00Z","durationMs":1500}
{"type":"system","subtype":"turn_duration","timestamp":"2026-08-22T10:00:10Z","durationMs":2500}
{"type":"system","subtype":"turn_duration","timestamp":"2026-08-22T10:00:20Z","durationMs":0}
{"type":"system","subtype":"stop_hook_summary","timestamp":"2026-08-22T10:00:30Z","durationMs":9999}
{"type":"assistant","timestamp":"2026-08-22T10:01:00Z","message":{"id":"m1","model":"claude-sonnet-5","usage":{"input_tokens":1,"output_tokens":1}}}
`

func TestParseReaderActiveDuration(t *testing.T) {
	t.Parallel()
	s := ParseReader("duration.jsonl", strings.NewReader(durationFixture))

	if s.ActiveDuration != 4*time.Second {
		t.Errorf("ActiveDuration = %s, want 4s (1.5s + 2.5s)", s.ActiveDuration)
	}
	// A zero duration is no measurement, and stop_hook_summary is not a turn.
	if s.ActiveTurns != 2 {
		t.Errorf("ActiveTurns = %d, want 2", s.ActiveTurns)
	}
	if s.Span() != time.Minute {
		t.Errorf("Span = %s, want 1m0s", s.Span())
	}
}

func TestSpanGuards(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	start := end.Add(time.Hour)
	s := Session{Start: start, End: end} // end before start
	if s.Span() != 0 {
		t.Errorf("Span with end<start = %s, want 0", s.Span())
	}
}

func TestScopeMergeIntoZeroValue(t *testing.T) {
	t.Parallel()
	// A zero-value Scope must be safe to merge into: report aggregates and
	// JSON-restored summaries both start out with nil maps.
	src := NewScope()
	src.Tokens = Tokens{Input: 3, Output: 2}
	src.AssistantTurns = 1
	src.addModelUsage("claude-sonnet-5", src.Tokens)
	src.ToolCounts["Bash"] = 4
	src.FileCounts["a.go"] = 1
	src.SkillCounts["review"] = 1

	var dst Scope
	dst.Merge(&src)
	dst.Merge(&src)

	if dst.Tokens != (Tokens{Input: 6, Output: 4}) || dst.AssistantTurns != 2 {
		t.Errorf("merged tokens/turns = %+v/%d, want doubled", dst.Tokens, dst.AssistantTurns)
	}
	want := Usage{Turns: 2, Tokens: Tokens{Input: 6, Output: 4}}
	if got := dst.ByModel["claude-sonnet-5"]; got != want {
		t.Errorf("merged ByModel = %+v, want %+v", got, want)
	}
	if dst.ToolCounts["Bash"] != 8 || dst.FileCounts["a.go"] != 2 || dst.SkillCounts["review"] != 2 {
		t.Errorf("merged counts = %v/%v/%v", dst.ToolCounts, dst.FileCounts, dst.SkillCounts)
	}
}
