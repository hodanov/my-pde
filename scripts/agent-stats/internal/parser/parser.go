// Package parser reads AI CLI session transcripts (JSONL) into structured
// per-session summaries.
//
// It targets Claude Code's ~/.claude/projects/**/*.jsonl transcripts and parses
// leniently: unknown fields are ignored and malformed lines are skipped so a
// schema change never crashes the tool. The transcript schema is unofficial and
// may change; keep this the only place that knows its shape.
package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// Tokens holds a session's token usage broken down by kind.
type Tokens struct {
	Input         int `json:"input"`
	Output        int `json:"output"`
	CacheRead     int `json:"cache_read"`
	CacheCreation int `json:"cache_creation"`
}

// Add accumulates o into t.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheCreation += o.CacheCreation
}

// Usage is a turn count paired with the tokens those turns consumed.
type Usage struct {
	Turns  int    `json:"turns"`
	Tokens Tokens `json:"tokens"`
}

// Scope is the aggregate for one origin of work. Claude Code flags every
// subagent line with isSidechain, so main-loop and delegated work can be told
// apart; keeping them in separate scopes is what makes "how much was
// delegated, and at what cost" answerable.
type Scope struct {
	Tokens         Tokens           `json:"tokens"`
	AssistantTurns int              `json:"assistant_turns"`
	ByModel        map[string]Usage `json:"by_model"`
	ToolCounts     map[string]int   `json:"tool_counts"`
	FileCounts     map[string]int   `json:"file_counts"`
	SkillCounts    map[string]int   `json:"skill_counts"`
}

// NewScope returns a Scope whose maps are ready to be written to.
func NewScope() Scope {
	s := Scope{}
	s.ensure()
	return s
}

// ensure initialises the maps a zero-value or JSON-restored Scope lacks, so
// callers can accumulate into one without constructing it first.
func (s *Scope) ensure() {
	if s.ByModel == nil {
		s.ByModel = map[string]Usage{}
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	if s.FileCounts == nil {
		s.FileCounts = map[string]int{}
	}
	if s.SkillCounts == nil {
		s.SkillCounts = map[string]int{}
	}
}

// Merge accumulates src into s, used to fold per-session scopes into totals.
func (s *Scope) Merge(src *Scope) {
	s.ensure()
	s.Tokens.Add(src.Tokens)
	s.AssistantTurns += src.AssistantTurns
	for model, u := range src.ByModel {
		cur := s.ByModel[model]
		cur.Turns += u.Turns
		cur.Tokens.Add(u.Tokens)
		s.ByModel[model] = cur
	}
	mergeCounts(s.ToolCounts, src.ToolCounts)
	mergeCounts(s.FileCounts, src.FileCounts)
	mergeCounts(s.SkillCounts, src.SkillCounts)
}

func mergeCounts(dst, src map[string]int) {
	for name, n := range src {
		dst[name] += n
	}
}

// addModelUsage credits one turn and its tokens to model.
func (s *Scope) addModelUsage(model string, t Tokens) {
	s.ensure()
	u := s.ByModel[model]
	u.Turns++
	u.Tokens.Add(t)
	s.ByModel[model] = u
}

// Session is the aggregated view of a single transcript file.
type Session struct {
	File      string `json:"file"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"git_branch"`

	Main Scope `json:"main"`
	Sub  Scope `json:"sub"`

	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	// ActiveDuration sums the turns Claude Code timed itself (system lines with
	// subtype turn_duration). Only newer CLI versions write them, so
	// ActiveTurns records how many turns the figure actually covers; report it
	// alongside the duration rather than passing it off as the whole session's
	// working time.
	ActiveDuration time.Duration `json:"active_duration_ns"`
	ActiveTurns    int           `json:"active_turns"`

	// SyntheticTurns counts assistant turns whose model is a placeholder such
	// as "<synthetic>" (CLI-generated, not a real inference). They are excluded
	// from per-model attribution but counted here so the omission is visible.
	SyntheticTurns int `json:"synthetic_turns"`

	// seenMessageIDs dedupes assistant turns and their token usage. Claude Code
	// writes one logical turn (thinking -> text -> tool_use) as multiple JSONL
	// lines that share the same message.id and each repeat the whole turn's
	// usage; without this, turns and tokens are counted once per line instead
	// of once per turn.
	seenMessageIDs map[string]struct{}
}

// Span returns the wall-clock distance between the first and last timestamped
// entry, or zero when it cannot be determined. A resumed session's span
// includes the idle gaps, so it is an upper bound on working time rather than a
// measure of it; ActiveDuration is the measure.
func (s *Session) Span() time.Duration {
	if s.Start.IsZero() || s.End.IsZero() || s.End.Before(s.Start) {
		return 0
	}
	return s.End.Sub(s.Start)
}

// AssistantTurns returns the session's main-loop and subagent turns combined.
func (s *Session) AssistantTurns() int {
	return s.Main.AssistantTurns + s.Sub.AssistantTurns
}

// Tokens returns the session's main-loop and subagent token usage combined.
func (s *Session) Tokens() Tokens {
	t := s.Main.Tokens
	t.Add(s.Sub.Tokens)
	return t
}

// scope selects which origin's aggregate a line belongs to. A line without the
// isSidechain flag is treated as main-loop work (the lenient default; in
// practice only a handful of lines omit it).
func (s *Session) scope(isSidechain bool) *Scope {
	if isSidechain {
		return &s.Sub
	}
	return &s.Main
}

// fileEditTools are the tool names whose input carries a file_path we count as
// a touched file.
var fileEditTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

type rawLine struct {
	Type        string  `json:"type"`
	Subtype     string  `json:"subtype"`
	Timestamp   string  `json:"timestamp"`
	Cwd         string  `json:"cwd"`
	GitBranch   string  `json:"gitBranch"`
	IsSidechain bool    `json:"isSidechain"`
	DurationMs  int64   `json:"durationMs"`
	Message     *rawMsg `json:"message"`
}

type rawMsg struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type rawUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type rawContent struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// maxLineBytes caps a single JSONL line so an unexpectedly huge record cannot
// exhaust memory; longer lines are skipped by the scanner.
const maxLineBytes = 16 * 1024 * 1024

// NewSession returns an empty session labelled name, ready to be filled by
// AppendReader or AppendFile.
func NewSession(name string) Session {
	return Session{
		File:           name,
		Main:           NewScope(),
		Sub:            NewScope(),
		seenMessageIDs: map[string]struct{}{},
	}
}

// AppendFile folds a transcript file into s. It returns an error only when the
// file cannot be opened; malformed content within is tolerated.
func AppendFile(s *Session, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	AppendReader(s, f)
	return nil
}

// AppendReader folds a transcript stream into s. Claude Code splits one logical
// session across several files — the session's own transcript plus one per
// subagent it spawned — so a session is assembled from all of them, sharing one
// message.id dedupe set.
//
// It never returns an error: unparseable lines are skipped.
func AppendReader(s *Session, r io.Reader) {
	if s.seenMessageIDs == nil {
		s.seenMessageIDs = map[string]struct{}{}
	}
	s.Main.ensure()
	s.Sub.ensure()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw rawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		applyLine(s, &raw)
	}
}

// ParseReader parses a whole transcript stream into its own session, labelling
// the result with name.
func ParseReader(name string, r io.Reader) Session {
	s := NewSession(name)
	AppendReader(&s, r)
	return s
}

func applyLine(s *Session, raw *rawLine) {
	if ts := parseTime(raw.Timestamp); !ts.IsZero() {
		if s.Start.IsZero() || ts.Before(s.Start) {
			s.Start = ts
		}
		if ts.After(s.End) {
			s.End = ts
		}
	}
	if raw.Cwd != "" {
		s.Cwd = raw.Cwd
	}
	if raw.GitBranch != "" {
		s.GitBranch = raw.GitBranch
	}
	switch raw.Type {
	case "assistant":
		applyAssistant(s, raw)
	case "system":
		applySystem(s, raw)
	}
}

func applyAssistant(s *Session, raw *rawLine) {
	if raw.Message == nil {
		return
	}
	sc := s.scope(raw.IsSidechain)
	// A turn split across multiple lines shares one message.id and repeats the
	// same usage on every line; count the turn and its tokens once per id. A
	// line without an id can't be deduped, so it is always counted (lenient
	// fallback for older or unrecognised transcript shapes).
	id := raw.Message.ID
	alreadyCounted := false
	if id != "" {
		if _, ok := s.seenMessageIDs[id]; ok {
			alreadyCounted = true
		} else {
			s.seenMessageIDs[id] = struct{}{}
		}
	}
	if !alreadyCounted {
		sc.AssistantTurns++
		var t Tokens
		if u := raw.Message.Usage; u != nil {
			t = Tokens{
				Input:         u.InputTokens,
				Output:        u.OutputTokens,
				CacheRead:     u.CacheReadInputTokens,
				CacheCreation: u.CacheCreationInputTokens,
			}
		}
		sc.Tokens.Add(t)
		switch model := raw.Message.Model; {
		case isRealModel(model):
			sc.addModelUsage(model, t)
		case model != "":
			s.SyntheticTurns++
		}
	}
	for _, c := range decodeContent(raw.Message.Content) {
		if c.Type != "tool_use" || c.Name == "" {
			continue
		}
		sc.ToolCounts[c.Name]++
		if fileEditTools[c.Name] {
			if fp := filePathOf(c.Input); fp != "" {
				sc.FileCounts[fp]++
			}
		}
		if c.Name == "Skill" {
			if skill := skillNameOf(c.Input); skill != "" {
				sc.SkillCounts[skill]++
			}
		}
	}
}

// applySystem reads the CLI's own bookkeeping lines. They carry measurements
// the assistant lines cannot supply, such as how long a turn actually took.
func applySystem(s *Session, raw *rawLine) {
	if raw.Subtype == "turn_duration" && raw.DurationMs > 0 {
		s.ActiveDuration += time.Duration(raw.DurationMs) * time.Millisecond
		s.ActiveTurns++
	}
}

// isRealModel reports whether a message.model value names an actual model.
// Claude Code uses angle-bracketed placeholders (notably "<synthetic>") for
// turns it generated itself, which carry no real inference cost.
func isRealModel(model string) bool {
	return model != "" && !strings.HasPrefix(model, "<")
}

// decodeContent extracts tool_use blocks from a message's content. Content may
// be a JSON array of blocks or a plain string (user turns); only the array form
// carries tool_use, so a decode failure yields no blocks.
func decodeContent(raw json.RawMessage) []rawContent {
	if len(raw) == 0 {
		return nil
	}
	var blocks []rawContent
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

func filePathOf(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	return fields.FilePath
}

func skillNameOf(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	return fields.Skill
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
