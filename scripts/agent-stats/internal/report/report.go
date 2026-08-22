// Package report aggregates parsed sessions into an observability summary and
// renders it as a human-readable table or machine-readable JSON.
package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"agent-stats/internal/parser"
)

// Count pairs a name with its tally, used for ranked lists.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ModelUsage attributes turns and tokens to one model.
type ModelUsage struct {
	Name   string        `json:"name"`
	Turns  int           `json:"turns"`
	Tokens parser.Tokens `json:"tokens"`
}

// Summary is the aggregate over a set of sessions.
type Summary struct {
	Sessions       int           `json:"sessions"`
	Tokens         parser.Tokens `json:"tokens"`
	AssistantTurns int           `json:"assistant_turns"`

	// ActiveDuration is time the CLI measured itself, over ActiveTurns turns;
	// Span is the first-to-last timestamp distance, idle gaps included. The two
	// answer different questions and neither substitutes for the other.
	ActiveDuration time.Duration `json:"active_duration_ns"`
	ActiveTurns    int           `json:"active_turns"`
	Span           time.Duration `json:"span_ns"`

	// SyntheticTurns are turns left out of ModelTokens because their model is a
	// CLI-generated placeholder rather than a real one.
	SyntheticTurns int `json:"synthetic_turns"`

	Main parser.Scope `json:"main"`
	Sub  parser.Scope `json:"sub"`

	ModelTokens []ModelUsage     `json:"model_tokens"`
	Tools       []Count          `json:"tools"`
	Files       []Count          `json:"files"`
	Skills      []Count          `json:"skills"`
	List        []parser.Session `json:"list"`
}

// Summarize folds sessions into a Summary. It is a pure function: identical
// input always yields identical, deterministically ordered output.
func Summarize(sessions []parser.Session) Summary {
	sum := Summary{
		Sessions: len(sessions),
		Main:     parser.NewScope(),
		Sub:      parser.NewScope(),
		List:     sessions,
	}
	for i := range sessions {
		s := &sessions[i]
		sum.Main.Merge(&s.Main)
		sum.Sub.Merge(&s.Sub)
		sum.ActiveDuration += s.ActiveDuration
		sum.ActiveTurns += s.ActiveTurns
		sum.Span += s.Span()
		sum.SyntheticTurns += s.SyntheticTurns
	}

	// The headline figures and the ranked lists are the two scopes combined;
	// Main and Sub stay available for anyone asking how much was delegated.
	total := parser.NewScope()
	total.Merge(&sum.Main)
	total.Merge(&sum.Sub)
	sum.Tokens = total.Tokens
	sum.AssistantTurns = total.AssistantTurns
	sum.ModelTokens = rankedModels(total.ByModel)
	sum.Tools = ranked(total.ToolCounts, 0)
	sum.Files = ranked(total.FileCounts, 0)
	sum.Skills = ranked(total.SkillCounts, 0)
	return sum
}

// ranked turns a name->count map into a slice ordered by count desc then name
// asc. A limit <= 0 keeps every entry.
func ranked(m map[string]int, limit int) []Count {
	out := make([]Count, 0, len(m))
	for name, n := range m {
		out = append(out, Count{Name: name, Count: n})
	}
	slices.SortFunc(out, func(a, b Count) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// rankedModels orders per-model usage by output tokens desc then name asc.
// Output tokens rank it because they are what the model actually generated;
// input and cache volume mostly track context size rather than work done.
func rankedModels(m map[string]parser.Usage) []ModelUsage {
	out := make([]ModelUsage, 0, len(m))
	for name, u := range m {
		out = append(out, ModelUsage{Name: name, Turns: u.Turns, Tokens: u.Tokens})
	}
	slices.SortFunc(out, func(a, b ModelUsage) int {
		if c := cmp.Compare(b.Tokens.Output, a.Tokens.Output); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// RenderJSON serialises the summary as indented JSON for tool-to-tool use.
func RenderJSON(s *Summary) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// topFiles is how many touched files the table shows.
const topFiles = 10

// RenderTable renders the summary as a plain-text report.
func RenderTable(s *Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sessions:        %d\n", s.Sessions)
	fmt.Fprintf(&b, "Assistant turns: %d (main %d / subagent %d)\n",
		s.AssistantTurns, s.Main.AssistantTurns, s.Sub.AssistantTurns)
	fmt.Fprintf(&b, "Active time:     %s (sum of turn_duration over %d recorded turns)\n",
		s.ActiveDuration.Round(time.Second), s.ActiveTurns)
	fmt.Fprintf(&b, "Session span:    %s (first-to-last timestamp, includes idle gaps in resumed sessions)\n",
		s.Span.Round(time.Second))

	b.WriteString("\nTokens\n")
	fmt.Fprintf(&b, "  input          %d\n", s.Tokens.Input)
	fmt.Fprintf(&b, "  output         %d\n", s.Tokens.Output)
	fmt.Fprintf(&b, "  cache read     %d\n", s.Tokens.CacheRead)
	fmt.Fprintf(&b, "  cache creation %d\n", s.Tokens.CacheCreation)

	writeOrigin(&b, s)
	writeModels(&b, s.ModelTokens, s.SyntheticTurns)
	writeCounts(&b, "Tool calls", s.Tools, 0)
	writeCounts(&b, "Skills", s.Skills, 0)
	writeCounts(&b, "Top files", s.Files, topFiles)

	b.WriteString("\nNote: only Claude Code transcripts are parsed; other AI CLIs are not yet supported.\n")
	return b.String()
}

// writeOrigin shows how much of the work ran in the main loop versus in
// subagents, which is what makes under- or over-delegation visible.
func writeOrigin(b *strings.Builder, s *Summary) {
	b.WriteString("\nOrigin\n")
	writeUsageRow(b, 8, "main", s.Main.AssistantTurns, s.Main.Tokens)
	writeUsageRow(b, 8, "subagent", s.Sub.AssistantTurns, s.Sub.Tokens)
}

func writeModels(b *strings.Builder, models []ModelUsage, synthetic int) {
	b.WriteString("\nModel tokens\n")
	if len(models) == 0 {
		b.WriteString("  (none)\n")
	} else {
		width := 0
		for _, m := range models {
			width = max(width, len(m.Name))
		}
		for _, m := range models {
			writeUsageRow(b, width, m.Name, m.Turns, m.Tokens)
		}
	}
	if synthetic > 0 {
		fmt.Fprintf(b, "  (%d turns excluded: CLI-generated placeholder model)\n", synthetic)
	}
}

func writeUsageRow(b *strings.Builder, width int, name string, turns int, t parser.Tokens) {
	fmt.Fprintf(b, "  %-*s  turns %-7d output %-10d cache read %d\n", width, name, turns, t.Output, t.CacheRead)
}

func writeCounts(b *strings.Builder, title string, counts []Count, limit int) {
	fmt.Fprintf(b, "\n%s\n", title)
	if len(counts) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	if limit > 0 && len(counts) > limit {
		counts = counts[:limit]
	}
	width := 0
	for _, c := range counts {
		width = max(width, len(c.Name))
	}
	for _, c := range counts {
		fmt.Fprintf(b, "  %-*s  %d\n", width, c.Name, c.Count)
	}
}
