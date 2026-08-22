package parser

import (
	"slices"
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
	if s.Span() != 0 || s.ActiveDuration() != 0 {
		t.Errorf("Span/ActiveDuration = %s/%s, want 0/0", s.Span(), s.ActiveDuration())
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

	if s.ActiveDuration() != 4*time.Second {
		t.Errorf("ActiveDuration = %s, want 4s (1.5s + 2.5s)", s.ActiveDuration())
	}
	// A zero duration is no measurement, and stop_hook_summary is not a turn.
	if s.ActiveTurns() != 2 {
		t.Errorf("ActiveTurns = %d, want 2", s.ActiveTurns())
	}
	// The individual timings are kept, not just their sum: they are far too
	// skewed for the sum alone to be meaningful.
	want := []time.Duration{1500 * time.Millisecond, 2500 * time.Millisecond}
	if !slices.Equal(s.TurnDurations, want) {
		t.Errorf("TurnDurations = %v, want %v", s.TurnDurations, want)
	}
	if s.Span() != time.Minute {
		t.Errorf("Span = %s, want 1m0s", s.Span())
	}
}

func TestLeadingCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cmd     string
		want    string
		wantCd  bool
		comment string
	}{
		{name: "bare", cmd: "ls", want: "ls"},
		{name: "with args", cmd: "git status --short", want: "git"},
		{name: "cd prefix", cmd: "cd /home/u/proj && rg -n foo", want: "rg", wantCd: true,
			comment: "attributed to the command doing the work, not to the cd"},
		{name: "cd only", cmd: "cd /home/u/proj", want: "cd", wantCd: true},
		{name: "cd with semicolon", cmd: "cd /a; ls -la", want: "ls", wantCd: true},
		{name: "subshell", cmd: "(cd /a && ls)", want: "ls", wantCd: true},
		{name: "env assignment", cmd: "FOO=1 BAR=2 go test ./...", want: "go"},
		{name: "env command", cmd: "env FOO=1 mise run lint", want: "mise"},
		{name: "absolute path", cmd: "/usr/bin/grep -n foo x.go", want: "grep",
			comment: "same bucket as a bare grep"},
		{name: "pipeline", cmd: "git log | grep fix", want: "git",
			comment: "the grep filters another command; only the leading command counts"},
		{name: "heredoc", cmd: "cat <<'EOF' > /tmp/x\n# a comment\ngrep not-a-call\nEOF", want: "cat",
			comment: "only the first line is a command; the body is data"},
		{name: "substitution", cmd: "$TMPDIR/bin/tool --flag", want: "",
			comment: "an unexpanded variable names no command we can attribute"},
		{name: "empty", cmd: "   ", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, gotCd := LeadingCommand(tc.cmd)
			if got != tc.want || gotCd != tc.wantCd {
				t.Errorf("LeadingCommand(%q) = %q/%v, want %q/%v (%s)",
					tc.cmd, got, gotCd, tc.want, tc.wantCd, tc.comment)
			}
		})
	}
}

// bashFixture exercises the shapes that make a naive breakdown wrong: a heredoc
// whose body looks like commands, a cd prefix, and a Bash call with no command
// field at all.
const bashFixture = `
{"type":"assistant","timestamp":"2026-08-22T10:00:00Z","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{"command":"cat <<'EOF' > /tmp/x\n# a comment\ngrep not-a-call\nEOF"}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:01Z","message":{"id":"m2","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{"command":"cd /home/u/proj && rg -n foo"}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:02Z","message":{"id":"m3","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{"command":"git status"}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:03Z","message":{"id":"m4","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{"command":"cd /home/u/proj && git diff"}}]}}
{"type":"assistant","timestamp":"2026-08-22T10:00:04Z","message":{"id":"m5","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","name":"Bash","input":{}}]}}
`

func TestParseReaderBashBreakdown(t *testing.T) {
	t.Parallel()
	s := ParseReader("bash.jsonl", strings.NewReader(bashFixture))

	// Every call still counts as a Bash call, including the one we cannot
	// attribute, so the breakdown never poses as the whole.
	if s.Main.ToolCounts["Bash"] != 5 {
		t.Errorf("ToolCounts[Bash] = %d, want 5", s.Main.ToolCounts["Bash"])
	}
	want := map[string]int{"cat": 1, "rg": 1, "git": 2}
	if len(s.Main.BashCounts) != len(want) {
		t.Fatalf("BashCounts = %v, want %v", s.Main.BashCounts, want)
	}
	for name, n := range want {
		if s.Main.BashCounts[name] != n {
			t.Errorf("BashCounts[%s] = %d, want %d", name, s.Main.BashCounts[name], n)
		}
	}
	// The heredoc body must not be mistaken for commands.
	for _, leaked := range []string{"grep", "#", "EOF", "cd"} {
		if _, ok := s.Main.BashCounts[leaked]; ok {
			t.Errorf("BashCounts must not contain %q: %v", leaked, s.Main.BashCounts)
		}
	}
	if s.Main.BashWithCd != 2 {
		t.Errorf("BashWithCd = %d, want 2", s.Main.BashWithCd)
	}
}

// errorFixture covers the tool results the CLI feeds back. The content is
// usually a plain string but can be an array of blocks, and a user turn may
// carry no tool result at all.
const errorFixture = `
{"type":"user","timestamp":"2026-08-22T10:00:00Z","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"user","timestamp":"2026-08-22T10:00:01Z","message":{"content":[{"type":"tool_result","is_error":true,"content":"The user doesn't want to proceed with this tool use. The tool use was rejected."}]}}
{"type":"user","timestamp":"2026-08-22T10:00:02Z","message":{"content":[{"type":"tool_result","is_error":true,"content":"PreToolUse:Bash hook blocked this call"}]}}
{"type":"user","timestamp":"2026-08-22T10:00:03Z","message":{"content":[{"type":"tool_result","is_error":true,"content":[{"type":"text","text":"exit code 1: no such file"}]}]}}
{"type":"user","timestamp":"2026-08-22T10:00:04Z","isSidechain":true,"message":{"content":[{"type":"tool_result","is_error":true,"content":"boom"}]}}
{"type":"user","timestamp":"2026-08-22T10:00:05Z","message":{"content":"a plain string, no tool result"}}
`

func TestParseReaderToolErrors(t *testing.T) {
	t.Parallel()
	s := ParseReader("errors.jsonl", strings.NewReader(errorFixture))

	if s.Main.ToolResults != 4 {
		t.Errorf("Main.ToolResults = %d, want 4", s.Main.ToolResults)
	}
	wantMain := map[string]int{ErrPermission: 1, ErrHook: 1, ErrFailure: 1}
	if len(s.Main.ToolErrors) != len(wantMain) {
		t.Fatalf("Main.ToolErrors = %v, want %v", s.Main.ToolErrors, wantMain)
	}
	for kind, n := range wantMain {
		if s.Main.ToolErrors[kind] != n {
			t.Errorf("Main.ToolErrors[%s] = %d, want %d", kind, s.Main.ToolErrors[kind], n)
		}
	}
	// A subagent's failures belong to the subagent scope.
	if s.Sub.ToolResults != 1 || s.Sub.ToolErrors[ErrFailure] != 1 {
		t.Errorf("Sub results/errors = %d/%v, want 1/failure:1", s.Sub.ToolResults, s.Sub.ToolErrors)
	}
	// A user turn is never an assistant turn, whatever it carries.
	if s.AssistantTurns() != 0 {
		t.Errorf("AssistantTurns = %d, want 0", s.AssistantTurns())
	}
}

func TestClassifyToolErrorFallsBackToFailure(t *testing.T) {
	t.Parallel()
	// The classification matches on CLI prose, so unrecognised wording must land
	// in failure rather than be dropped: the total stays exact even as the
	// breakdown degrades.
	if got := classifyToolError("some wording the CLI has since changed"); got != ErrFailure {
		t.Errorf("classifyToolError = %q, want %q", got, ErrFailure)
	}
	if got := classifyToolError(""); got != ErrFailure {
		t.Errorf("classifyToolError of empty text = %q, want %q", got, ErrFailure)
	}
}

// compactFixture records context compactions. A boundary line without metadata
// carries no measurement and must be ignored rather than counted as a zero.
const compactFixture = `
{"type":"system","subtype":"compact_boundary","timestamp":"2026-08-22T10:00:00Z","compactMetadata":{"trigger":"manual","preTokens":200000,"postTokens":50000,"cumulativeDroppedTokens":150000}}
{"type":"system","subtype":"compact_boundary","timestamp":"2026-08-22T11:00:00Z","compactMetadata":{"trigger":"auto","preTokens":180000,"postTokens":60000,"cumulativeDroppedTokens":270000}}
{"type":"system","subtype":"compact_boundary","timestamp":"2026-08-22T12:00:00Z"}
{"type":"system","subtype":"stop_hook_summary","timestamp":"2026-08-22T12:00:01Z"}
`

func TestParseReaderCompactions(t *testing.T) {
	t.Parallel()
	s := ParseReader("compact.jsonl", strings.NewReader(compactFixture))

	if len(s.Compactions) != 2 {
		t.Fatalf("Compactions = %+v, want 2 (the metadata-less boundary is not a measurement)", s.Compactions)
	}
	want := []Compaction{
		{Trigger: "manual", PreTokens: 200000, PostTokens: 50000},
		{Trigger: "auto", PreTokens: 180000, PostTokens: 60000},
	}
	for i := range want {
		if s.Compactions[i] != want[i] {
			t.Errorf("Compactions[%d] = %+v, want %+v", i, s.Compactions[i], want[i])
		}
	}
	// Dropped is per event. The transcript's own cumulativeDroppedTokens is a
	// running session total and would multiply-count if summed, so it is unused.
	if got := s.Compactions[0].Dropped() + s.Compactions[1].Dropped(); got != 270000 {
		t.Errorf("dropped total = %d, want 270000", got)
	}
}

func TestCompactionDroppedGuard(t *testing.T) {
	t.Parallel()
	// A compaction that did not shrink the context dropped nothing; never report
	// a negative.
	if got := (Compaction{PreTokens: 10, PostTokens: 20}).Dropped(); got != 0 {
		t.Errorf("Dropped with post>pre = %d, want 0", got)
	}
}

// labelFixture carries every label the CLI records for a session, plus a
// subagent line whose own label must not be mistaken for the session's.
const labelFixture = `
{"type":"user","timestamp":"2026-08-22T10:00:00Z","slug":"84-pr-terraform-plan-ci","message":{"content":"hi"}}
{"type":"agent-name","agentName":"bedrock-cost-interview-sheet","sessionId":"s1"}
{"type":"ai-title","aiTitle":"Bedrock 日次コストの棚卸し","sessionId":"s1"}
{"type":"user","timestamp":"2026-08-22T10:01:00Z","isSidechain":true,"slug":"a-subagent-slug","message":{"content":"sub"}}
`

func TestParseReaderLabel(t *testing.T) {
	t.Parallel()
	s := ParseReader("6f1d-uuid.jsonl", strings.NewReader(labelFixture))

	if s.AITitle != "Bedrock 日次コストの棚卸し" || s.AgentName != "bedrock-cost-interview-sheet" {
		t.Errorf("title/agent = %q/%q", s.AITitle, s.AgentName)
	}
	// A subagent's slug describes the subagent, not the session it ran under.
	if s.Slug != "84-pr-terraform-plan-ci" {
		t.Errorf("Slug = %q, want the main-loop slug", s.Slug)
	}
	if got := s.Label(); got != "Bedrock 日次コストの棚卸し" {
		t.Errorf("Label = %q, want the ai-title", got)
	}
}

func TestSessionLabelFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    Session
		want string
	}{
		{name: "ai title wins", want: "t", s: Session{File: "f", Slug: "s", AgentName: "a", AITitle: "t"}},
		{name: "then agent name", want: "a", s: Session{File: "f", Slug: "s", AgentName: "a"}},
		{name: "then slug", want: "s", s: Session{File: "f", Slug: "s"}},
		{name: "filename last", want: "f", s: Session{File: "f"}},
	}
	for _, tc := range cases {
		if got := tc.s.Label(); got != tc.want {
			t.Errorf("%s: Label = %q, want %q", tc.name, got, tc.want)
		}
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
	src.BashCounts["grep"] = 3
	src.BashWithCd = 2
	src.ToolResults = 5
	src.ToolErrors[ErrFailure] = 1

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
	if dst.BashCounts["grep"] != 6 || dst.BashWithCd != 4 {
		t.Errorf("merged bash = %v / withCd %d, want grep:6 / 4", dst.BashCounts, dst.BashWithCd)
	}
	if dst.ToolResults != 10 || dst.ToolErrors[ErrFailure] != 2 {
		t.Errorf("merged results/errors = %d/%v, want 10/failure:2", dst.ToolResults, dst.ToolErrors)
	}
}
