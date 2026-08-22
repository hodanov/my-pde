package report

import (
	"encoding/json"
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
				ToolCounts:  map[string]int{"Edit": 3, "Bash": 1, "Skill": 2},
				FileCounts:  map[string]int{"a.go": 2, "b.go": 1},
				SkillCounts: map[string]int{"dev-workflow": 2},
			},
			Sub: parser.Scope{
				Tokens:         parser.Tokens{Input: 8, Output: 4},
				AssistantTurns: 1,
				ByModel: map[string]parser.Usage{
					"claude-sonnet-5": {Turns: 1, Tokens: parser.Tokens{Input: 8, Output: 4}},
				},
				ToolCounts: map[string]int{"Grep": 2},
			},
			ActiveDuration: 90 * time.Second,
			ActiveTurns:    2,
			SyntheticTurns: 1,
			Start:          time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC),
			End:            time.Date(2026, 7, 30, 10, 5, 0, 0, time.UTC),
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
			},
			ActiveDuration: 30 * time.Second,
			ActiveTurns:    1,
			Start:          time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC),
			End:            time.Date(2026, 7, 30, 11, 10, 0, 0, time.UTC),
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
	out, err := RenderJSON(&summary)
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
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
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
