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

// Summary is the aggregate over a set of sessions.
type Summary struct {
	Sessions       int              `json:"sessions"`
	Tokens         parser.Tokens    `json:"tokens"`
	AssistantTurns int              `json:"assistant_turns"`
	Duration       time.Duration    `json:"duration_ns"`
	Models         []Count          `json:"models"`
	Tools          []Count          `json:"tools"`
	Files          []Count          `json:"files"`
	List           []parser.Session `json:"list"`
}

// Summarize folds sessions into a Summary. It is a pure function: identical
// input always yields identical, deterministically ordered output.
func Summarize(sessions []parser.Session) Summary {
	models := map[string]int{}
	tools := map[string]int{}
	files := map[string]int{}
	sum := Summary{Sessions: len(sessions), List: sessions}
	for i := range sessions {
		s := &sessions[i]
		sum.Tokens.Input += s.Tokens.Input
		sum.Tokens.Output += s.Tokens.Output
		sum.Tokens.CacheRead += s.Tokens.CacheRead
		sum.Tokens.CacheCreation += s.Tokens.CacheCreation
		sum.AssistantTurns += s.AssistantTurns
		sum.Duration += s.Duration()
		if s.Model != "" {
			models[s.Model]++
		}
		for name, n := range s.ToolCounts {
			tools[name] += n
		}
		for path, n := range s.FileCounts {
			files[path] += n
		}
	}
	sum.Models = ranked(models, 0)
	sum.Tools = ranked(tools, 0)
	sum.Files = ranked(files, 0)
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
	fmt.Fprintf(&b, "Assistant turns: %d\n", s.AssistantTurns)
	fmt.Fprintf(&b, "Duration:        %s (sum of each session's first-to-last timestamp span, including idle gaps in resumed sessions)\n", s.Duration.Round(time.Second))
	b.WriteString("\nTokens\n")
	fmt.Fprintf(&b, "  input          %d\n", s.Tokens.Input)
	fmt.Fprintf(&b, "  output         %d\n", s.Tokens.Output)
	fmt.Fprintf(&b, "  cache read     %d\n", s.Tokens.CacheRead)
	fmt.Fprintf(&b, "  cache creation %d\n", s.Tokens.CacheCreation)

	writeCounts(&b, "Models", s.Models, 0)
	writeCounts(&b, "Tool calls", s.Tools, 0)
	writeCounts(&b, "Top files", s.Files, topFiles)

	b.WriteString("\nNote: only Claude Code transcripts are parsed; other AI CLIs are not yet supported.\n")
	return b.String()
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
