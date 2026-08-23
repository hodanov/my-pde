package report

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"agent-stats/internal/parser"
)

func sessions() []parser.Session {
	return []parser.Session{
		{
			File: "s1.jsonl",
			Main: parser.Scope{
				Tokens:         parser.Tokens{Input: 100, Output: 40, CacheRead: 10},
				AssistantTurns: 2,
				ByModel: map[string]parser.Usage{
					"claude-opus-4-8": {Turns: 2, Tokens: parser.Tokens{Input: 100, Output: 40, CacheRead: 10}},
				},
				ToolCounts:  map[string]int{"Edit": 3, "Bash": 2, "Skill": 2},
				FileCounts:  map[string]int{"a.go": 2, "b.go": 1},
				SkillCounts: map[string]int{"dev-workflow": 2},
				BashCounts:  map[string]int{"git": 1, "grep": 1},
				BashWithCd:  1,
				ToolResults: 10,
				ToolErrors:  map[string]int{parser.ErrFailure: 2, parser.ErrPermission: 1},
			},
			Sub: parser.Scope{
				Tokens:         parser.Tokens{Input: 8, Output: 4},
				AssistantTurns: 1,
				ByModel: map[string]parser.Usage{
					"claude-sonnet-5": {Turns: 1, Tokens: parser.Tokens{Input: 8, Output: 4}},
				},
				ToolCounts:  map[string]int{"Grep": 2, "Bash": 1},
				BashCounts:  map[string]int{"ls": 1},
				ToolResults: 3,
				ToolErrors:  map[string]int{parser.ErrHook: 1},
			},
			TurnDurations:  []time.Duration{60 * time.Second, 30 * time.Second},
			SyntheticTurns: 1,
			Start:          time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC),
			End:            time.Date(2026, 7, 30, 10, 5, 0, 0, time.UTC),
			Compactions: []parser.Compaction{
				{Trigger: "manual", PreTokens: 200000, PostTokens: 50000},
				{Trigger: "auto", PreTokens: 180000, PostTokens: 60000},
			},
		},
		{
			File: "s2.jsonl",
			Main: parser.Scope{
				Tokens:         parser.Tokens{Input: 50, Output: 20},
				AssistantTurns: 1,
				ByModel: map[string]parser.Usage{
					"claude-sonnet-5": {Turns: 1, Tokens: parser.Tokens{Input: 50, Output: 20}},
				},
				ToolCounts:  map[string]int{"Edit": 1, "Read": 4, "Skill": 1},
				FileCounts:  map[string]int{"a.go": 1},
				SkillCounts: map[string]int{"review": 1},
				ToolResults: 6,
			},
			TurnDurations: []time.Duration{30 * time.Second},
			Start:         time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC),
			End:           time.Date(2026, 7, 30, 11, 10, 0, 0, time.UTC),
			Compactions: []parser.Compaction{
				{Trigger: "manual", PreTokens: 220000, PostTokens: 40000},
			},
		},
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	s := Summarize(sessions())

	if s.Sessions != 2 {
		t.Errorf("Sessions = %d, want 2", s.Sessions)
	}
	// Headline tokens are both scopes of both sessions combined.
	want := parser.Tokens{Input: 158, Output: 64, CacheRead: 10}
	if s.Tokens != want {
		t.Errorf("Tokens = %+v, want %+v", s.Tokens, want)
	}
	if s.AssistantTurns != 4 {
		t.Errorf("AssistantTurns = %d, want 4", s.AssistantTurns)
	}
	if s.Main.AssistantTurns != 3 || s.Sub.AssistantTurns != 1 {
		t.Errorf("turns main/sub = %d/%d, want 3/1", s.Main.AssistantTurns, s.Sub.AssistantTurns)
	}
	if s.Sub.Tokens.Output != 4 {
		t.Errorf("Sub.Tokens.Output = %d, want 4", s.Sub.Tokens.Output)
	}
	if s.Span != 15*time.Minute {
		t.Errorf("Span = %s, want 15m", s.Span)
	}
	if s.ActiveDuration != 2*time.Minute || s.ActiveTurns != 3 {
		t.Errorf("Active = %s over %d turns, want 2m over 3", s.ActiveDuration, s.ActiveTurns)
	}
	// [30s 30s 60s] pooled across both sessions.
	if s.MedianTurn != 30*time.Second || s.LongestTurn != time.Minute {
		t.Errorf("median/longest = %s/%s, want 30s/1m", s.MedianTurn, s.LongestTurn)
	}
	if s.SyntheticTurns != 1 {
		t.Errorf("SyntheticTurns = %d, want 1", s.SyntheticTurns)
	}
	// Edit: 3+1=4 leads; a.go: 2+1=3 leads. Subagent tool use is included.
	if s.Tools[0].Name != "Edit" || s.Tools[0].Count != 4 {
		t.Errorf("Tools[0] = %+v, want Edit:4", s.Tools[0])
	}
	if grep := indexOf(s.Tools, "Grep"); grep < 0 || s.Tools[grep].Count != 2 {
		t.Errorf("Tools = %+v, want subagent's Grep:2 included", s.Tools)
	}
	if s.Files[0].Name != "a.go" || s.Files[0].Count != 3 {
		t.Errorf("Files[0] = %+v, want a.go:3", s.Files[0])
	}
	if len(s.Skills) != 2 || s.Skills[0].Name != "dev-workflow" || s.Skills[0].Count != 2 {
		t.Errorf("Skills = %+v, want [dev-workflow:2 review:1]", s.Skills)
	}
	// sonnet-5 appears in two scopes across two sessions and must be pooled.
	wantModels := []ModelUsage{
		{Name: "claude-opus-4-8", Turns: 2, Tokens: parser.Tokens{Input: 100, Output: 40, CacheRead: 10}},
		{Name: "claude-sonnet-5", Turns: 2, Tokens: parser.Tokens{Input: 58, Output: 24}},
	}
	if len(s.ModelTokens) != len(wantModels) {
		t.Fatalf("ModelTokens = %+v, want %+v", s.ModelTokens, wantModels)
	}
	for i := range wantModels {
		if s.ModelTokens[i] != wantModels[i] {
			t.Errorf("ModelTokens[%d] = %+v, want %+v", i, s.ModelTokens[i], wantModels[i])
		}
	}
}

func TestSummarizeBash(t *testing.T) {
	t.Parallel()
	s := Summarize(sessions())

	// Every Bash call counts, main loop and subagent alike.
	if s.BashCalls != 3 {
		t.Errorf("BashCalls = %d, want 3", s.BashCalls)
	}
	if len(s.Bash) != 3 || s.Bash[0].Name != "git" {
		t.Errorf("Bash = %+v, want [git grep ls]", s.Bash)
	}
	if s.BashWithCd != 1 {
		t.Errorf("BashWithCd = %d, want 1", s.BashWithCd)
	}
}

func TestSummarizeToolResults(t *testing.T) {
	t.Parallel()
	s := Summarize(sessions())

	if s.ToolResults != 19 {
		t.Errorf("ToolResults = %d, want 19", s.ToolResults)
	}
	if s.ToolErrorTotal != 4 {
		t.Errorf("ToolErrorTotal = %d, want 4", s.ToolErrorTotal)
	}
	want := []Count{
		{Name: parser.ErrFailure, Count: 2},
		{Name: parser.ErrHook, Count: 1},
		{Name: parser.ErrPermission, Count: 1},
	}
	if len(s.ToolErrors) != len(want) {
		t.Fatalf("ToolErrors = %+v, want %+v", s.ToolErrors, want)
	}
	for i := range want {
		if s.ToolErrors[i] != want[i] {
			t.Errorf("ToolErrors[%d] = %+v, want %+v", i, s.ToolErrors[i], want[i])
		}
	}
}

func TestSummarizeCompactions(t *testing.T) {
	t.Parallel()
	s := Summarize(sessions())

	// Grouped by trigger, count desc: manual twice across both sessions, auto once.
	want := []CompactionStats{
		{Trigger: "manual", Count: 2, AvgPreTokens: 210000, Dropped: 330000},
		{Trigger: "auto", Count: 1, AvgPreTokens: 180000, Dropped: 120000},
	}
	if len(s.Compactions) != len(want) {
		t.Fatalf("Compactions = %+v, want %+v", s.Compactions, want)
	}
	for i := range want {
		if s.Compactions[i] != want[i] {
			t.Errorf("Compactions[%d] = %+v, want %+v", i, s.Compactions[i], want[i])
		}
	}
}

func TestSummarizeUnlabelledCompaction(t *testing.T) {
	t.Parallel()
	// A compaction with no trigger still happened; it must not be filed under an
	// empty label.
	s := Summarize([]parser.Session{{
		File:        "x.jsonl",
		Compactions: []parser.Compaction{{PreTokens: 100, PostTokens: 40}},
	}})
	if len(s.Compactions) != 1 || s.Compactions[0].Trigger != "unknown" {
		t.Fatalf("Compactions = %+v, want one entry triggered \"unknown\"", s.Compactions)
	}
	if s.Compactions[0].Dropped != 60 {
		t.Errorf("Dropped = %d, want 60", s.Compactions[0].Dropped)
	}
}

func indexOf(counts []Count, name string) int {
	for i, c := range counts {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func TestSummarizeEmptyScopes(t *testing.T) {
	t.Parallel()
	// Sessions whose scopes were never written to (nil maps, e.g. restored from
	// JSON) must fold in without panicking.
	s := Summarize([]parser.Session{{File: "bare.jsonl"}})
	if s.Sessions != 1 || s.AssistantTurns != 0 || len(s.Tools) != 0 {
		t.Errorf("Summarize of a bare session = %+v", s)
	}
}

func TestRankedOrdering(t *testing.T) {
	t.Parallel()
	// Equal counts must break ties by name ascending, deterministically.
	got := ranked(map[string]int{"b": 2, "a": 2, "c": 1}, 0)
	want := []Count{{Name: "a", Count: 2}, {Name: "b", Count: 2}, {Name: "c", Count: 1}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ranked[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if lim := ranked(map[string]int{"b": 2, "a": 2, "c": 1}, 2); len(lim) != 2 {
		t.Errorf("limit=2 returned %d entries", len(lim))
	}
}

func TestRankedModelsOrdering(t *testing.T) {
	t.Parallel()
	// Ranked by output tokens desc, so a model with fewer turns but more output
	// outranks a chattier one; equal output breaks ties by name ascending.
	got := rankedModels(map[string]parser.Usage{
		"b": {Turns: 1, Tokens: parser.Tokens{Output: 5}},
		"a": {Turns: 9, Tokens: parser.Tokens{Output: 5}},
		"c": {Turns: 1, Tokens: parser.Tokens{Output: 50}},
	})
	wantOrder := []string{"c", "a", "b"}
	if len(got) != len(wantOrder) {
		t.Fatalf("rankedModels = %+v", got)
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("rankedModels[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestRenderJSONRoundTrip(t *testing.T) {
	t.Parallel()
	summary := Summarize(sessions())
	out, err := RenderJSON(&summary, true)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var back Summary
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Sessions != 2 || back.Tokens.Input != 158 {
		t.Errorf("round-trip mismatch: %+v", back)
	}
	if back.Sub.AssistantTurns != 1 || back.ActiveDuration != 2*time.Minute {
		t.Errorf("round-trip lost the scope split or active time: %+v", back)
	}
	if len(back.List) != 2 {
		t.Errorf("detail should carry the per-session list, got %d entries", len(back.List))
	}
}

func TestRenderJSONOmitsListWithoutDetail(t *testing.T) {
	t.Parallel()
	summary := Summarize(sessions())
	out, err := RenderJSON(&summary, false)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if strings.Contains(string(out), `"list"`) {
		t.Errorf("the per-session list must be opt-in:\n%s", out)
	}
	// Dropping it from the output must not clear it on the caller's summary.
	if len(summary.List) != 2 {
		t.Errorf("RenderJSON must not mutate the summary, List = %d", len(summary.List))
	}
	// The aggregate is still there, and so is the per-session top-N.
	var back Summary
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Sessions != 2 || len(back.TopSessions) != 2 {
		t.Errorf("aggregate lost without detail: %+v", back)
	}
}

func TestSummarizeTurnStatsDoesNotReorderInput(t *testing.T) {
	t.Parallel()
	// Summarize is documented as pure; computing the median must not sort the
	// caller's own slices.
	ss := sessions()
	ss[0].TurnDurations = []time.Duration{60 * time.Second, 30 * time.Second}
	before := slices.Clone(ss[0].TurnDurations)
	s := Summarize(ss)
	if !slices.Equal(ss[0].TurnDurations, before) {
		t.Errorf("Summarize reordered its input: %v, was %v", ss[0].TurnDurations, before)
	}
	if s.MedianTurn != 30*time.Second {
		t.Errorf("MedianTurn = %s, want 30s", s.MedianTurn)
	}
}

func TestSummarizeNoTurnTimings(t *testing.T) {
	t.Parallel()
	// Older CLI versions record no turn timings at all; the summary must say
	// zero rather than divide by it.
	s := Summarize([]parser.Session{{File: "x.jsonl"}})
	if s.ActiveTurns != 0 || s.ActiveDuration != 0 || s.MedianTurn != 0 || s.LongestTurn != 0 {
		t.Errorf("timings without data = %+v", s)
	}
}

func TestSummarizeProjects(t *testing.T) {
	t.Parallel()
	ss := sessions()
	ss[0].Cwd = "/w/proj-a"
	ss[1].Cwd = "/w/proj-b"
	extra := ss[1]
	extra.File = "s3.jsonl"
	extra.Cwd = "/w/proj-a"
	s := Summarize(append(ss, extra))

	// proj-a: s1 (output 44) + s3 (output 20) = 64; proj-b: 20.
	want := []ProjectStats{
		{Cwd: "/w/proj-a", Sessions: 2, Turns: 4, Output: 64},
		{Cwd: "/w/proj-b", Sessions: 1, Turns: 1, Output: 20},
	}
	if len(s.Projects) != len(want) {
		t.Fatalf("Projects = %+v, want %+v", s.Projects, want)
	}
	for i := range want {
		if s.Projects[i] != want[i] {
			t.Errorf("Projects[%d] = %+v, want %+v", i, s.Projects[i], want[i])
		}
	}
}

func TestSummarizeProjectsWithoutCwd(t *testing.T) {
	t.Parallel()
	// A transcript that never recorded a cwd still ran somewhere; it must be
	// grouped under a visible label rather than an empty string.
	s := Summarize([]parser.Session{{File: "x.jsonl"}})
	if len(s.Projects) != 1 || s.Projects[0].Cwd != "(unknown)" {
		t.Fatalf("Projects = %+v, want one entry labelled (unknown)", s.Projects)
	}
}

func TestSummarizeTopSessions(t *testing.T) {
	t.Parallel()
	ss := sessions()
	ss[0].AITitle = "the heavy one"
	s := Summarize(ss)

	// s1 output 40+4=44 outranks s2's 20.
	want := []SessionStats{
		{Title: "the heavy one", File: "s1.jsonl", Turns: 3, Output: 44, ActiveDuration: 90 * time.Second},
		{Title: "s2.jsonl", File: "s2.jsonl", Turns: 1, Output: 20, ActiveDuration: 30 * time.Second},
	}
	if len(s.TopSessions) != len(want) {
		t.Fatalf("TopSessions = %+v, want %+v", s.TopSessions, want)
	}
	for i := range want {
		if s.TopSessions[i] != want[i] {
			t.Errorf("TopSessions[%d] = %+v, want %+v", i, s.TopSessions[i], want[i])
		}
	}
}

func TestSummarizeTopSessionsCaps(t *testing.T) {
	t.Parallel()
	all := make([]parser.Session, 0, topSessions+5)
	for i := range topSessions + 5 {
		all = append(all, parser.Session{
			File: fmt.Sprintf("s%02d.jsonl", i),
			Main: parser.Scope{Tokens: parser.Tokens{Output: i}},
		})
	}
	s := Summarize(all)
	if len(s.TopSessions) != topSessions {
		t.Fatalf("TopSessions = %d entries, want %d", len(s.TopSessions), topSessions)
	}
	// Heaviest first, and the count printed alongside says how many were left out.
	if s.TopSessions[0].Output != topSessions+4 {
		t.Errorf("TopSessions[0] = %+v, want the heaviest session", s.TopSessions[0])
	}
	if out := RenderTable(&s); !strings.Contains(out, fmt.Sprintf("(%d of %d)", topSessions, topSessions+5)) {
		t.Errorf("table must say how many sessions the top-N covers:\n%s", out)
	}
}

func TestRenderTable(t *testing.T) {
	t.Parallel()
	summary := Summarize(sessions())
	out := RenderTable(&summary)
	for _, want := range []string{
		"Sessions:", "Active time:", "Session span:", "Tokens",
		"Origin", "subagent", "Model tokens", "claude-opus-4-8",
		"Edit", "a.go", "Skills", "dev-workflow", "Claude Code",
		"1 turns excluded",
		"Bash breakdown (3 calls)", "prefixed with cd",
		"Tool results", "errors   4 (21.1%)", "permission",
		"Compactions", "manual", "avg pre-tokens 210000", "dropped 330000",
		"By project (session cwd)", "Top sessions by output tokens (2 of 2)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
}

func TestRenderTableSaysWhatItTruncated(t *testing.T) {
	t.Parallel()
	// A cut-off list must say so; silently showing the top N reads as the whole
	// set.
	counts := make([]Count, 0, topFiles+3)
	for i := range topFiles + 3 {
		counts = append(counts, Count{Name: fmt.Sprintf("f%02d.go", i), Count: 1})
	}
	var b strings.Builder
	writeCounts(&b, "Top files", counts, topFiles)
	if !strings.Contains(b.String(), "(3 more not shown)") {
		t.Errorf("truncated list must report what it hid:\n%s", b.String())
	}
}

func TestRenderTableEmpty(t *testing.T) {
	t.Parallel()
	summary := Summarize(nil)
	out := RenderTable(&summary)
	if !strings.Contains(out, "(none)") {
		t.Errorf("empty table should show (none):\n%s", out)
	}
	// With nothing excluded there is no exclusion note to print.
	if strings.Contains(out, "turns excluded") {
		t.Errorf("empty table should not mention exclusions:\n%s", out)
	}
}
