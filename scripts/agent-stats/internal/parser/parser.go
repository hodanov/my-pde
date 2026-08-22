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
	"errors"
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

	// BashCounts breaks the single largest tool bucket down by the shell command
	// each call actually runs, keyed by LeadingCommand. BashWithCd counts the
	// calls that prefix a cd, which the CLI warns triggers permission prompts.
	BashCounts map[string]int `json:"bash_counts"`
	BashWithCd int            `json:"bash_with_cd"`

	// ToolResults is every tool_result seen, the denominator for ToolErrors,
	// which buckets the failed ones by ErrPermission / ErrHook / ErrFailure.
	ToolResults int            `json:"tool_results"`
	ToolErrors  map[string]int `json:"tool_errors"`
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
	if s.BashCounts == nil {
		s.BashCounts = map[string]int{}
	}
	if s.ToolErrors == nil {
		s.ToolErrors = map[string]int{}
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
	mergeCounts(s.BashCounts, src.BashCounts)
	mergeCounts(s.ToolErrors, src.ToolErrors)
	s.BashWithCd += src.BashWithCd
	s.ToolResults += src.ToolResults
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

// Compaction is one context-compaction event. Trigger is "auto" when the CLI
// compacted on its own because the context filled up, or "manual" when it was
// asked to.
type Compaction struct {
	Trigger string `json:"trigger"`
	// PreTokens and PostTokens are the context size either side of the
	// compaction. Dropped is derived from them rather than from the
	// transcript's cumulativeDroppedTokens, which is a running total for the
	// whole session and would multiply-count if summed per event.
	PreTokens  int `json:"pre_tokens"`
	PostTokens int `json:"post_tokens"`
}

// Dropped returns how many tokens this single compaction discarded.
func (c Compaction) Dropped() int {
	if c.PreTokens <= c.PostTokens {
		return 0
	}
	return c.PreTokens - c.PostTokens
}

// Session is the aggregated view of one logical session, assembled from its own
// transcript file plus any subagent transcripts belonging to it.
type Session struct {
	File      string `json:"file"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"git_branch"`

	// Label candidates, most descriptive first. A transcript's filename is a
	// UUID, so without these a session cannot be told apart by eye. Each holds
	// the most recently recorded value: the CLI rewrites the line as the label
	// changes, and the latest one describes the session as it ended up.
	AITitle   string `json:"ai_title"`
	AgentName string `json:"agent_name"`
	Slug      string `json:"slug"`

	Main Scope `json:"main"`
	Sub  Scope `json:"sub"`

	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	// TurnDurations holds every turn Claude Code timed itself (system lines with
	// subtype turn_duration), in transcript order.
	//
	// The whole distribution is kept, not just the sum, for two reasons. Only
	// newer CLI versions write these lines, so the count says how much of the
	// session the figure covers at all; and the values are extremely skewed — a
	// session left open records one turn spanning hours — so a sum presented on
	// its own reads as working time when it is nothing of the kind.
	TurnDurations []time.Duration `json:"turn_durations_ns"`

	// SyntheticTurns counts assistant turns whose model is a placeholder such
	// as "<synthetic>" (CLI-generated, not a real inference). They are excluded
	// from per-model attribution but counted here so the omission is visible.
	SyntheticTurns int `json:"synthetic_turns"`

	// Compactions records each time the session's context was compacted, in
	// transcript order. An automatic one means the session ran into the context
	// limit rather than being wound down deliberately.
	Compactions []Compaction `json:"compactions"`

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

// ActiveDuration sums the turns the CLI timed. Read it together with
// ActiveTurns and the spread of TurnDurations: it is a sum over a skewed
// distribution, not a session's working time.
func (s *Session) ActiveDuration() time.Duration {
	var total time.Duration
	for _, d := range s.TurnDurations {
		total += d
	}
	return total
}

// ActiveTurns returns how many turns ActiveDuration actually covers.
func (s *Session) ActiveTurns() int {
	return len(s.TurnDurations)
}

// Label returns the most descriptive name available for the session, falling
// back to the transcript filename when the CLI recorded no label at all.
func (s *Session) Label() string {
	for _, c := range []string{s.AITitle, s.AgentName, s.Slug} {
		if c != "" {
			return c
		}
	}
	return s.File
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
	Type            string      `json:"type"`
	Subtype         string      `json:"subtype"`
	Timestamp       string      `json:"timestamp"`
	Cwd             string      `json:"cwd"`
	GitBranch       string      `json:"gitBranch"`
	Slug            string      `json:"slug"`
	AITitle         string      `json:"aiTitle"`
	AgentName       string      `json:"agentName"`
	IsSidechain     bool        `json:"isSidechain"`
	DurationMs      int64       `json:"durationMs"`
	CompactMetadata *rawCompact `json:"compactMetadata"`
	Message         *rawMsg     `json:"message"`
}

type rawCompact struct {
	Trigger    string `json:"trigger"`
	PreTokens  int    `json:"preTokens"`
	PostTokens int    `json:"postTokens"`
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
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	IsError bool            `json:"is_error"`
	Content json.RawMessage `json:"content"`
}

// maxLineBytes caps a single JSONL line so an unexpectedly huge record cannot
// exhaust memory. A longer line is skipped; the lines after it are not.
const maxLineBytes = 16 * 1024 * 1024

// initialLineBytes is where the line buffer starts. It grows on demand up to
// maxLineBytes, so the common case of short lines costs one small allocation.
const initialLineBytes = 64 * 1024

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
	appendLines(s, r, maxLineBytes)
}

// appendLines folds every parseable line of r into s, tolerating lines longer
// than maxLine.
//
// bufio.Scanner stops for good at a token it cannot fit, so one oversized record
// would otherwise hide every record after it — a silent truncation, which is
// worse than a skipped line. Restarting the scanner on the same reader resumes
// past that record: everything buffered when the scanner gave up belongs to the
// oversized line, and its tail comes back as one unparseable line that is
// skipped like any other.
func appendLines(s *Session, r io.Reader, maxLine int) {
	for {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, min(initialLineBytes, maxLine)), maxLine)
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
		if !errors.Is(sc.Err(), bufio.ErrTooLong) {
			return
		}
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
	// Labels are taken from main-loop lines only. A session is assembled from its
	// own transcript plus its subagents', and a subagent's label names the
	// subagent, not the session it was spawned from.
	if !raw.IsSidechain {
		if raw.AITitle != "" {
			s.AITitle = raw.AITitle
		}
		if raw.AgentName != "" {
			s.AgentName = raw.AgentName
		}
		if raw.Slug != "" {
			s.Slug = raw.Slug
		}
	}
	switch raw.Type {
	case "assistant":
		applyAssistant(s, raw)
	case "user":
		applyUser(s, raw)
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
		if c.Name == "Bash" {
			addBashCall(sc, commandOf(c.Input))
		}
	}
}

func addBashCall(sc *Scope, cmd string) {
	if cmd == "" {
		return
	}
	name, withCd := LeadingCommand(cmd)
	if name != "" {
		sc.BashCounts[name]++
	}
	if withCd {
		sc.BashWithCd++
	}
}

// applyUser reads the results the CLI feeds back to the assistant. Tool
// outcomes only appear here, so this is the only place failures, permission
// denials and hook blocks can be counted.
func applyUser(s *Session, raw *rawLine) {
	if raw.Message == nil {
		return
	}
	sc := s.scope(raw.IsSidechain)
	for _, c := range decodeContent(raw.Message.Content) {
		if c.Type != "tool_result" {
			continue
		}
		sc.ToolResults++
		if c.IsError {
			sc.ToolErrors[classifyToolError(toolResultText(c.Content))]++
		}
	}
}

// applySystem reads the CLI's own bookkeeping lines. They carry measurements
// the assistant lines cannot supply, such as how long a turn actually took.
func applySystem(s *Session, raw *rawLine) {
	switch raw.Subtype {
	case "turn_duration":
		// A zero duration is an absent measurement, not a turn that took no time.
		if raw.DurationMs > 0 {
			s.TurnDurations = append(s.TurnDurations, time.Duration(raw.DurationMs)*time.Millisecond)
		}
	case "compact_boundary":
		if m := raw.CompactMetadata; m != nil {
			s.Compactions = append(s.Compactions, Compaction{
				Trigger:    m.Trigger,
				PreTokens:  m.PreTokens,
				PostTokens: m.PostTokens,
			})
		}
	}
}

// Error kinds recorded in Scope.ToolErrors.
const (
	// ErrPermission is a tool call the user declined.
	ErrPermission = "permission"
	// ErrHook is a tool call a PreToolUse or PostToolUse hook blocked.
	ErrHook = "hook"
	// ErrFailure is everything else: a non-zero exit, a missing file, an MCP
	// error, and anything the patterns below no longer recognise.
	ErrFailure = "failure"
)

// classifyToolError buckets a failed tool_result by the prose the CLI puts in
// it. There is no machine-readable field for the reason, so this matches on
// wording and will drift as the CLI is reworded; unrecognised text falls back to
// ErrFailure so the total stays exact even when the breakdown degrades.
func classifyToolError(text string) string {
	switch {
	case strings.Contains(text, "PreToolUse:"), strings.Contains(text, "PostToolUse:"):
		return ErrHook
	case strings.HasPrefix(text, "The user doesn't want to"),
		strings.Contains(text, "tool use was rejected"):
		return ErrPermission
	default:
		return ErrFailure
	}
}

// toolResultText flattens a tool_result's content for classification. It is
// either a plain string or an array of blocks, and only the leading text
// matters, so decoding stops at the first usable piece.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Text != "" {
			return b.Text
		}
	}
	return ""
}

// maxBashPrefixHops bounds how many wrapper tokens LeadingCommand steps over,
// so a pathological command string cannot spin.
const maxBashPrefixHops = 8

// LeadingCommand returns the shell command a Bash call actually runs, plus
// whether the call prefixed a cd.
//
// Only the first line is examined. Multi-line calls are typically a command
// followed by a heredoc body, and tokenising the whole string counts heredoc
// content — EOF markers, comments, prose — as if it were commands.
//
// Wrapper tokens are stepped over so a call is attributed to the command doing
// the work: `cd /x && rg foo` is an rg call, not a cd call. The cd itself is
// still reported, because prefixing one is what triggers permission prompts.
func LeadingCommand(cmd string) (name string, withCd bool) {
	rest, _, _ := strings.Cut(cmd, "\n")
	for range maxBashPrefixHops {
		rest = strings.TrimLeft(rest, "( \t")
		if rest == "" {
			return "", withCd
		}
		tok, tail := splitToken(rest)
		switch {
		case tok == "cd":
			withCd = true
			next, ok := afterSeparator(tail)
			if !ok {
				// Nothing but the cd itself.
				return "cd", true
			}
			rest = next
		case tok == "env", isEnvAssignment(tok):
			rest = tail
		default:
			return normalizeCommand(tok), withCd
		}
	}
	return "", withCd
}

// splitToken returns the first whitespace-delimited token of s and the remainder.
func splitToken(s string) (tok, tail string) {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// afterSeparator returns what follows the first shell command separator in s,
// so a leading cd can be stepped over.
func afterSeparator(s string) (tail string, ok bool) {
	at, width := -1, 0
	for _, sep := range []string{"&&", "||", ";", "|"} {
		if i := strings.Index(s, sep); i >= 0 && (at < 0 || i < at) {
			at, width = i, len(sep)
		}
	}
	if at < 0 {
		return "", false
	}
	return s[at+width:], true
}

// isEnvAssignment reports whether tok is a VAR=value prefix rather than a
// command. A path or a substitution containing "=" is not one.
func isEnvAssignment(tok string) bool {
	i := strings.Index(tok, "=")
	return i > 0 && !strings.ContainsAny(tok[:i], `/.$'"`)
}

// normalizeCommand reduces a command token to its bare name, so /usr/bin/grep
// and grep land in the same bucket. Quoting and grouping punctuation is
// stripped: no command name legitimately begins or ends with it.
func normalizeCommand(tok string) string {
	tok = strings.Trim(tok, "'\"`();")
	if tok == "" || strings.HasPrefix(tok, "$") {
		return ""
	}
	if i := strings.LastIndexByte(tok, '/'); i >= 0 {
		tok = tok[i+1:]
	}
	return tok
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

func commandOf(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	return fields.Command
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
