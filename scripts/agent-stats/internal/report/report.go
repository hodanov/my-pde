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

// CompactionStats aggregates every compaction that shared one trigger. An
// "auto" trigger means the session ran into the context limit rather than being
// compacted deliberately, so its count is the signal worth watching.
type CompactionStats struct {
	Trigger      string `json:"trigger"`
	Count        int    `json:"count"`
	AvgPreTokens int    `json:"avg_pre_tokens"`
	Dropped      int    `json:"dropped_tokens"`
}

// bashRedundant maps a shell command to the dedicated tool that should have run
// instead. Whether a shell command is redundant is a tool-selection judgement,
// not part of the transcript schema, so it lives here rather than in the parser.
//
// Only the leading command counts: `rg foo` is a Grep call written by hand, but
// the grep in `git log | grep foo` is filtering another command's output and has
// no tool equivalent. Counting the latter would overstate the problem.
var bashRedundant = map[string]string{
	"cat":  "Read",
	"head": "Read",
	"tail": "Read",
	"grep": "Grep",
	"rg":   "Grep",
	"find": "Glob",
	"ls":   "Glob",
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

	ModelTokens []ModelUsage `json:"model_tokens"`
	Tools       []Count      `json:"tools"`
	Files       []Count      `json:"files"`
	Skills      []Count      `json:"skills"`

	// BashCalls is every Bash call, and Bash breaks them down by leading
	// command. The breakdown can total less than BashCalls when a command string
	// has no recognisable leading command, so both are reported.
	BashCalls  int     `json:"bash_calls"`
	Bash       []Count `json:"bash"`
	BashWithCd int     `json:"bash_with_cd"`

	// RedundantBash counts, per replacing tool, the Bash calls a dedicated tool
	// should have handled. This is the before/after measure for tool-selection
	// guidance.
	RedundantBash      []Count `json:"redundant_bash"`
	RedundantBashTotal int     `json:"redundant_bash_total"`

	// ToolResults is every tool result seen — the denominator for the error
	// rate — and ToolErrors the failures by kind.
	ToolResults    int     `json:"tool_results"`
	ToolErrors     []Count `json:"tool_errors"`
	ToolErrorTotal int     `json:"tool_error_total"`

	Compactions []CompactionStats `json:"compactions"`

	List []parser.Session `json:"list"`
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
	compactions := map[string]CompactionStats{}
	for i := range sessions {
		s := &sessions[i]
		sum.Main.Merge(&s.Main)
		sum.Sub.Merge(&s.Sub)
		sum.ActiveDuration += s.ActiveDuration
		sum.ActiveTurns += s.ActiveTurns
		sum.Span += s.Span()
		sum.SyntheticTurns += s.SyntheticTurns
		for _, c := range s.Compactions {
			addCompaction(compactions, c)
		}
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

	sum.BashCalls = total.ToolCounts["Bash"]
	sum.Bash = ranked(total.BashCounts, 0)
	sum.BashWithCd = total.BashWithCd
	sum.RedundantBash, sum.RedundantBashTotal = redundantBash(total.BashCounts)

	sum.ToolResults = total.ToolResults
	sum.ToolErrors = ranked(total.ToolErrors, 0)
	for _, c := range sum.ToolErrors {
		sum.ToolErrorTotal += c.Count
	}

	sum.Compactions = rankedCompactions(compactions)
	return sum
}

// addCompaction folds one event into the per-trigger aggregate. Dropped tokens
// come from each event's own pre/post sizes, never from the transcript's
// cumulative counter, which is a running per-session total and would
// multiply-count if summed.
func addCompaction(into map[string]CompactionStats, c parser.Compaction) {
	trigger := c.Trigger
	if trigger == "" {
		trigger = "unknown"
	}
	cur := into[trigger]
	cur.Trigger = trigger
	cur.Count++
	// AvgPreTokens holds the running sum until rankedCompactions divides it.
	cur.AvgPreTokens += c.PreTokens
	cur.Dropped += c.Dropped()
	into[trigger] = cur
}

// rankedCompactions orders the aggregates by count desc then trigger asc, and
// turns the accumulated pre-token sums into averages.
func rankedCompactions(m map[string]CompactionStats) []CompactionStats {
	out := make([]CompactionStats, 0, len(m))
	for _, c := range m {
		if c.Count > 0 {
			c.AvgPreTokens /= c.Count
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b CompactionStats) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return strings.Compare(a.Trigger, b.Trigger)
	})
	return out
}

// redundantBash groups the Bash calls that a dedicated tool should have handled
// by the tool that replaces them, and returns their total.
func redundantBash(counts map[string]int) (byTool []Count, total int) {
	grouped := map[string]int{}
	for cmd, n := range counts {
		tool, ok := bashRedundant[cmd]
		if !ok {
			continue
		}
		grouped[tool] += n
		total += n
	}
	return ranked(grouped, 0), total
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

// How many entries the longer-tailed table sections show.
const (
	topFiles = 10
	topBash  = 15
)

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
	writeBash(&b, s)
	writeToolResults(&b, s)
	writeCounts(&b, "Skills", s.Skills, 0)
	writeCounts(&b, "Top files", s.Files, topFiles)
	writeCompactions(&b, s.Compactions)

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
	hidden := 0
	if limit > 0 && len(counts) > limit {
		hidden = len(counts) - limit
		counts = counts[:limit]
	}
	width := 0
	for _, c := range counts {
		width = max(width, len(c.Name))
	}
	for _, c := range counts {
		fmt.Fprintf(b, "  %-*s  %d\n", width, c.Name, c.Count)
	}
	// Say what was cut rather than let a truncated list read as the whole set.
	if hidden > 0 {
		fmt.Fprintf(b, "  (%d more not shown)\n", hidden)
	}
}

// writeBash opens up the single largest tool bucket. Bash dwarfs every other
// tool, so "Bash: 5145" on its own says nothing about what the time went on.
func writeBash(b *strings.Builder, s *Summary) {
	writeCounts(b, fmt.Sprintf("Bash breakdown (%d calls)", s.BashCalls), s.Bash, topBash)
	if s.BashCalls == 0 {
		return
	}
	if s.RedundantBashTotal > 0 {
		parts := make([]string, 0, len(s.RedundantBash))
		for _, c := range s.RedundantBash {
			parts = append(parts, fmt.Sprintf("%s %d", c.Name, c.Count))
		}
		fmt.Fprintf(b, "  -> %d replaceable by a dedicated tool (%s)\n",
			s.RedundantBashTotal, strings.Join(parts, ", "))
	}
	if s.BashWithCd > 0 {
		fmt.Fprintf(b, "  -> %d prefixed with cd (risks a permission prompt; pass absolute paths)\n", s.BashWithCd)
	}
}

// writeToolResults reports how often tool calls failed and why. Failures only
// appear in the results fed back to the assistant, never in its own turns.
func writeToolResults(b *strings.Builder, s *Summary) {
	b.WriteString("\nTool results\n")
	if s.ToolResults == 0 {
		b.WriteString("  (none)\n")
		return
	}
	fmt.Fprintf(b, "  results  %d\n", s.ToolResults)
	fmt.Fprintf(b, "  errors   %d (%.1f%%)\n",
		s.ToolErrorTotal, 100*float64(s.ToolErrorTotal)/float64(s.ToolResults))
	for _, c := range s.ToolErrors {
		fmt.Fprintf(b, "    %-11s%d\n", c.Name, c.Count)
	}
}

// writeCompactions shows the context pressure each session ran under. An auto
// trigger is direct evidence of hitting the context limit.
func writeCompactions(b *strings.Builder, stats []CompactionStats) {
	b.WriteString("\nCompactions\n")
	if len(stats) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	width := 0
	for _, c := range stats {
		width = max(width, len(c.Trigger))
	}
	for _, c := range stats {
		fmt.Fprintf(b, "  %-*s  %-5d avg pre-tokens %-8d dropped %d\n",
			width, c.Trigger, c.Count, c.AvgPreTokens, c.Dropped)
	}
}
