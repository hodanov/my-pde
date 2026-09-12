// Package permission turns recorded permission prompts, their outcomes in the
// transcripts, and repositories' local allow lists into allow-rule candidates
// for review.
package permission

import (
	"bufio"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Entry is one PermissionRequest the permission-ledger hook recorded. The hook
// input carries no tool_use_id, so an entry is tied to its transcript call by
// the transcript file, the tool name and the recorded input.
type Entry struct {
	Timestamp      time.Time       `json:"ts"`
	SessionID      string          `json:"session_id"`
	PromptID       string          `json:"prompt_id"`
	AgentID        string          `json:"agent_id"`
	ToolName       string          `json:"tool_name"`
	Cwd            string          `json:"cwd"`
	TranscriptPath string          `json:"transcript_path"`
	PermissionMode string          `json:"permission_mode"`
	ToolInput      json.RawMessage `json:"tool_input"`
	Suggestions    []Suggestion    `json:"suggestions"`
}

// Suggestion is one permission update Claude Code offered with the prompt.
type Suggestion struct {
	Type        string          `json:"type"`
	Rules       []SuggestedRule `json:"rules"`
	Behavior    string          `json:"behavior"`
	Destination string          `json:"destination"`
}

// SuggestedRule is a rule in the shape the permission dialog would save it.
type SuggestedRule struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent"`
}

// ReadLedger parses the ledger, skipping unparseable lines. The hook is wired
// through both the plugin and the settings copy, so one prompt can be recorded
// twice within moments; such repeats are dropped.
func ReadLedger(r io.Reader) ([]Entry, error) {
	const duplicateWindow = 10 * time.Second
	var entries []Entry
	lastSeen := map[string]time.Time{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || e.SessionID == "" || e.ToolName == "" {
			continue
		}
		key := strings.Join([]string{e.SessionID, e.AgentID, e.PromptID, e.ToolName, canonicalInput(e.ToolInput)}, "\x00")
		if prev, ok := lastSeen[key]; ok && e.Timestamp.Sub(prev).Abs() < duplicateWindow {
			continue
		}
		lastSeen[key] = e.Timestamp
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

func (e *Entry) transcriptFile() string {
	if e.TranscriptPath == "" {
		return ""
	}
	if e.AgentID == "" {
		return filepath.Clean(e.TranscriptPath)
	}
	return filepath.Join(strings.TrimSuffix(e.TranscriptPath, ".jsonl"), "subagents", "agent-"+e.AgentID+".jsonl")
}

func (e *Entry) allowRules() []string {
	var rules []string
	for _, s := range e.Suggestions {
		if s.Type != "addRules" || s.Behavior != "allow" {
			continue
		}
		for _, r := range s.Rules {
			rule := r.ToolName
			if r.RuleContent != "" {
				rule += "(" + r.RuleContent + ")"
			}
			rule = normalizeRule(rule)
			if !slices.Contains(rules, rule) {
				rules = append(rules, rule)
			}
		}
	}
	return rules
}
