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

// ProjectStats aggregates the sessions that ran in one directory. Grouping is
// by the session's recorded cwd rather than a derived repository name: worktrees
// and subdirectories are separate working contexts, and inferring a shared
// project from the path would mislabel them.
type ProjectStats struct {
	Cwd      string `json:"cwd"`
	Sessions int    `json:"sessions"`
	Turns    int    `json:"turns"`
	Output   int    `json:"output_tokens"`
}

// SessionStats is one session reduced to what the table shows.
type SessionStats struct {
	Title          string        `json:"title"`
	File           string        `json:"file"`
	Turns          int           `json:"turns"`
	Output         int           `json:"output_tokens"`
	ActiveDuration time.Duration `json:"active_duration_ns"`
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

	// MedianTurn and LongestTurn describe the distribution ActiveDuration was
	// summed over. It is severely skewed in practice — a session left open
	// records a single turn spanning hours — so the sum on its own would be read
	// as working time. Reporting the two together makes the skew impossible to
	// miss.
	MedianTurn  time.Duration `json:"median_turn_ns"`
	LongestTurn time.Duration `json:"longest_turn_ns"`

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

	// ToolResults is every tool result seen — the denominator for the error
	// rate — and ToolErrors the failures by kind.
	ToolResults    int     `json:"tool_results"`
	ToolErrors     []Count `json:"tool_errors"`
	ToolErrorTotal int     `json:"tool_error_total"`

	Compactions []CompactionStats `json:"compactions"`

	Projects    []ProjectStats `json:"projects"`
	TopSessions []SessionStats `json:"top_sessions"`

	// List is every session in full. It dwarfs the rest of the output, so
	// RenderJSON only includes it when detail is asked for.
	List []parser.Session `json:"list,omitempty"`
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
	projects := map[string]ProjectStats{}
	var turns []time.Duration
	for i := range sessions {
		s := &sessions[i]
		sum.Main.Merge(&s.Main)
		sum.Sub.Merge(&s.Sub)
		sum.Span += s.Span()
		sum.SyntheticTurns += s.SyntheticTurns
		turns = append(turns, s.TurnDurations...)
		for _, c := range s.Compactions {
			addCompaction(compactions, c)
		}
		addProject(projects, s)
		sum.TopSessions = append(sum.TopSessions, SessionStats{
			Title:          s.Label(),
			File:           s.File,
			Turns:          s.AssistantTurns(),
			Output:         s.Tokens().Output,
			ActiveDuration: s.ActiveDuration(),
		})
	}
	sum.ActiveTurns = len(turns)
	sum.ActiveDuration, sum.MedianTurn, sum.LongestTurn = turnStats(turns)
	sum.Projects = rankedProjects(projects)
	sum.TopSessions = rankSessions(sum.TopSessions)

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

	sum.ToolResults = total.ToolResults
	sum.ToolErrors = ranked(total.ToolErrors, 0)
	for _, c := range sum.ToolErrors {
		sum.ToolErrorTotal += c.Count
	}

	sum.Compactions = rankedCompactions(compactions)
	return sum
}

// turnStats reduces the pooled turn timings to the three figures that describe
// them together. The median is the typical turn; the longest exposes the
// outliers that dominate the total. Sorting a copy keeps Summarize pure — it
// must not reorder its caller's session data.
func turnStats(turns []time.Duration) (total, median, longest time.Duration) {
	if len(turns) == 0 {
		return 0, 0, 0
	}
	sorted := slices.Clone(turns)
	slices.Sort(sorted)
	for _, d := range sorted {
		total += d
	}
	return total, sorted[len(sorted)/2], sorted[len(sorted)-1]
}

// addProject folds one session into its directory's aggregate.
func addProject(into map[string]ProjectStats, s *parser.Session) {
	cwd := s.Cwd
	if cwd == "" {
		cwd = "(unknown)"
	}
	cur := into[cwd]
	cur.Cwd = cwd
	cur.Sessions++
	cur.Turns += s.AssistantTurns()
	cur.Output += s.Tokens().Output
	into[cwd] = cur
}

// rankedProjects orders directories by output tokens desc then cwd asc. Output
// tokens rank them for the same reason they rank models: they track work done
// rather than context size.
func rankedProjects(m map[string]ProjectStats) []ProjectStats {
	out := make([]ProjectStats, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b ProjectStats) int {
		if c := cmp.Compare(b.Output, a.Output); c != 0 {
			return c
		}
		return strings.Compare(a.Cwd, b.Cwd)
	})
	return out
}

// rankSessions keeps the heaviest sessions by output tokens, so "which session
// cost that much" is answerable without dumping every session.
func rankSessions(all []SessionStats) []SessionStats {
	slices.SortFunc(all, func(a, b SessionStats) int {
		if c := cmp.Compare(b.Output, a.Output); c != 0 {
			return c
		}
		return strings.Compare(a.File, b.File)
	})
	if len(all) > topSessions {
		all = all[:topSessions]
	}
	return all
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
// detail adds the full per-session list, which is far larger than everything
// else combined, so callers that only want the aggregate are not made to pay
// for it.
func RenderJSON(s *Summary, detail bool) ([]byte, error) {
	out := *s
	if !detail {
		out.List = nil
	}
	return json.MarshalIndent(out, "", "  ")
}

// How many entries the longer-tailed sections keep.
const (
	topFiles    = 10
	topBash     = 15
	topProjects = 10
	topSessions = 10
)

// RenderTable renders the summary as a plain-text report.
func RenderTable(s *Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sessions:        %d\n", s.Sessions)
	fmt.Fprintf(&b, "Assistant turns: %d (main %d / subagent %d)\n",
		s.AssistantTurns, s.Main.AssistantTurns, s.Sub.AssistantTurns)
	fmt.Fprintf(&b, "Active time:     %s (sum of turn_duration over %d recorded turns; median %s, longest %s)\n",
		s.ActiveDuration.Round(time.Second), s.ActiveTurns,
		s.MedianTurn.Round(time.Second), s.LongestTurn.Round(time.Second))
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
	writeProjects(&b, s.Projects)
	writeTopSessions(&b, s.TopSessions, s.Sessions)

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
	if s.BashWithCd > 0 {
		fmt.Fprintf(b, "  -> %d prefixed with cd (resets the shell's working directory; pass absolute paths)\n", s.BashWithCd)
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

// writeProjects shows where the work went, grouped by the directory each
// session ran in.
func writeProjects(b *strings.Builder, projects []ProjectStats) {
	b.WriteString("\nBy project (session cwd)\n")
	if len(projects) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	hidden := 0
	if len(projects) > topProjects {
		hidden = len(projects) - topProjects
		projects = projects[:topProjects]
	}
	for _, p := range projects {
		fmt.Fprintf(b, "  sessions %-5d turns %-6d output %-10d %s\n", p.Sessions, p.Turns, p.Output, p.Cwd)
	}
	if hidden > 0 {
		fmt.Fprintf(b, "  (%d more not shown)\n", hidden)
	}
}

// writeTopSessions names the heaviest sessions. Transcript filenames are UUIDs,
// so the label is what makes a row actionable; it goes last because a title can
// be any width and must not push the figures out of alignment.
func writeTopSessions(b *strings.Builder, top []SessionStats, total int) {
	fmt.Fprintf(b, "\nTop sessions by output tokens (%d of %d)\n", len(top), total)
	if len(top) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, s := range top {
		fmt.Fprintf(b, "  output %-10d turns %-6d active %-10s %s\n",
			s.Output, s.Turns, s.ActiveDuration.Round(time.Second), s.Title)
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
