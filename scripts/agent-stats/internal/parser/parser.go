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
	"path/filepath"
	"time"
)

// Tokens holds a session's token usage broken down by kind.
type Tokens struct {
	Input         int `json:"input"`
	Output        int `json:"output"`
	CacheRead     int `json:"cache_read"`
	CacheCreation int `json:"cache_creation"`
}

// Session is the aggregated view of a single transcript file.
type Session struct {
	File           string         `json:"file"`
	Model          string         `json:"model"`
	Cwd            string         `json:"cwd"`
	GitBranch      string         `json:"git_branch"`
	Tokens         Tokens         `json:"tokens"`
	AssistantTurns int            `json:"assistant_turns"`
	ToolCounts     map[string]int `json:"tool_counts"`
	FileCounts     map[string]int `json:"file_counts"`
	Start          time.Time      `json:"start"`
	End            time.Time      `json:"end"`
}

// Duration returns the wall-clock span between the first and last timestamped
// entry, or zero when it cannot be determined.
func (s *Session) Duration() time.Duration {
	if s.Start.IsZero() || s.End.IsZero() || s.End.Before(s.Start) {
		return 0
	}
	return s.End.Sub(s.Start)
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
	Type      string  `json:"type"`
	Timestamp string  `json:"timestamp"`
	Cwd       string  `json:"cwd"`
	GitBranch string  `json:"gitBranch"`
	Message   *rawMsg `json:"message"`
}

type rawMsg struct {
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

// ParseFile parses a single transcript file. It returns an error only when the
// file cannot be opened; malformed content within is tolerated.
func ParseFile(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = f.Close() }()
	return ParseReader(filepath.Base(path), f), nil
}

// ParseReader parses a transcript stream, labelling the result with name.
// It never returns an error: unparseable lines are skipped.
func ParseReader(name string, r io.Reader) Session {
	s := Session{
		File:       name,
		ToolCounts: map[string]int{},
		FileCounts: map[string]int{},
	}
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
		applyLine(&s, &raw)
	}
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
	if raw.Type != "assistant" || raw.Message == nil {
		return
	}
	s.AssistantTurns++
	if raw.Message.Model != "" {
		s.Model = raw.Message.Model
	}
	if u := raw.Message.Usage; u != nil {
		s.Tokens.Input += u.InputTokens
		s.Tokens.Output += u.OutputTokens
		s.Tokens.CacheRead += u.CacheReadInputTokens
		s.Tokens.CacheCreation += u.CacheCreationInputTokens
	}
	for _, c := range decodeContent(raw.Message.Content) {
		if c.Type != "tool_use" || c.Name == "" {
			continue
		}
		s.ToolCounts[c.Name]++
		if fileEditTools[c.Name] {
			if fp := filePathOf(c.Input); fp != "" {
				s.FileCounts[fp]++
			}
		}
	}
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
