package parser

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

// Outcome is how the CLI resolved a tool call, as recorded in the transcript.
type Outcome string

const (
	// OutcomeNoResult is a call with no tool_result yet: the session is still
	// running, or its transcript was cut short.
	OutcomeNoResult Outcome = "no-result"
	// OutcomeExecuted is a call that was allowed to run, whether or not the
	// tool itself then failed.
	OutcomeExecuted Outcome = "executed"
	// OutcomeUserRejected is a call the user declined in a permission dialog.
	OutcomeUserRejected Outcome = "user-rejected"
	// OutcomeDenied is a call a deny rule, hook or classifier stopped before it
	// ran.
	OutcomeDenied Outcome = "denied"
)

// ToolCall is one tool invocation and how it was resolved.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Cwd       string          `json:"cwd"`
	Timestamp time.Time       `json:"timestamp"`
	Outcome   Outcome         `json:"outcome"`
}

// ToolCallIndex maps tool_use ids to their calls. A session spreads its calls
// over its own transcript and its subagents', so one index is filled from all
// of them.
type ToolCallIndex map[string]ToolCall

// AppendFile folds a transcript file into the index. It returns an error only
// when the file cannot be opened; malformed content within is tolerated.
func (idx ToolCallIndex) AppendFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	idx.AppendReader(f)
	return nil
}

// AppendReader folds a transcript stream into the index, skipping unparseable
// lines.
func (idx ToolCallIndex) AppendReader(r io.Reader) {
	scanLines(r, maxLineBytes, idx.apply)
}

func (idx ToolCallIndex) apply(raw *rawLine) {
	if raw.Message == nil {
		return
	}
	switch raw.Type {
	case "assistant":
		for _, c := range decodeContent(raw.Message.Content) {
			if c.Type != "tool_use" || c.ID == "" {
				continue
			}
			call := idx[c.ID]
			call.ID = c.ID
			call.Name = c.Name
			call.Input = c.Input
			call.Cwd = raw.Cwd
			call.Timestamp = parseTime(raw.Timestamp)
			if call.Outcome == "" {
				call.Outcome = OutcomeNoResult
			}
			idx[c.ID] = call
		}
	case "user":
		for _, c := range decodeContent(raw.Message.Content) {
			if c.Type != "tool_result" || c.ToolUseID == "" {
				continue
			}
			call := idx[c.ToolUseID]
			call.ID = c.ToolUseID
			call.Outcome = resultOutcome(raw.ToolDenialKind, &c)
			idx[c.ToolUseID] = call
		}
	}
}

func resultOutcome(denialKind string, c *rawContent) Outcome {
	switch {
	case denialKind == "user-rejected":
		return OutcomeUserRejected
	case denialKind != "":
		return OutcomeDenied
	case !c.IsError:
		return OutcomeExecuted
	}
	switch classifyToolError(toolResultText(c.Content)) {
	case ErrPermission:
		return OutcomeUserRejected
	case ErrHook:
		return OutcomeDenied
	default:
		return OutcomeExecuted
	}
}
