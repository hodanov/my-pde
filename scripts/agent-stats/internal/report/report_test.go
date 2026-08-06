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
			File:           "s1.jsonl",
			Model:          "claude-opus-4-8",
			Tokens:         parser.Tokens{Input: 100, Output: 40, CacheRead: 10},
			AssistantTurns: 2,
			ToolCounts:     map[string]int{"Edit": 3, "Bash": 1, "Skill": 2},
			FileCounts:     map[string]int{"a.go": 2, "b.go": 1},
			SkillCounts:    map[string]int{"dev-workflow": 2},
			Start:          time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC),
			End:            time.Date(2026, 7, 30, 10, 5, 0, 0, time.UTC),
		},
		{
			File:           "s2.jsonl",
			Model:          "claude-sonnet-5",
			Tokens:         parser.Tokens{Input: 50, Output: 20},
			AssistantTurns: 1,
			ToolCounts:     map[string]int{"Edit": 1, "Read": 4, "Skill": 1},
			FileCounts:     map[string]int{"a.go": 1},
			SkillCounts:    map[string]int{"review": 1},
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
	if s.Tokens.Input != 150 || s.Tokens.Output != 60 || s.Tokens.CacheRead != 10 {
		t.Errorf("Tokens = %+v", s.Tokens)
	}
	if s.AssistantTurns != 3 {
		t.Errorf("AssistantTurns = %d, want 3", s.AssistantTurns)
	}
	if s.Duration != 15*time.Minute {
		t.Errorf("Duration = %s, want 15m", s.Duration)
	}
	// Edit: 3+1=4 leads; a.go: 2+1=3 leads.
	if s.Tools[0].Name != "Edit" || s.Tools[0].Count != 4 {
		t.Errorf("Tools[0] = %+v, want Edit:4", s.Tools[0])
	}
	if s.Files[0].Name != "a.go" || s.Files[0].Count != 3 {
		t.Errorf("Files[0] = %+v, want a.go:3", s.Files[0])
	}
	if len(s.Skills) != 2 || s.Skills[0].Name != "dev-workflow" || s.Skills[0].Count != 2 {
		t.Errorf("Skills = %+v, want [dev-workflow:2 review:1]", s.Skills)
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
	if back.Sessions != 2 || back.Tokens.Input != 150 {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestRenderTable(t *testing.T) {
	t.Parallel()
	summary := Summarize(sessions())
	out := RenderTable(&summary)
	for _, want := range []string{"Sessions:", "Tokens", "Edit", "a.go", "Skills", "dev-workflow", "Claude Code"} {
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
}
