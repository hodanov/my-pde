package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestToolCallIndexAppendReader(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	bash := func(id string, outcome Outcome) ToolCall {
		return ToolCall{
			ID:        id,
			Name:      "Bash",
			Input:     json.RawMessage(`{"command":"go env GOPATH"}`),
			Cwd:       "/w/repo",
			Timestamp: at,
			Outcome:   outcome,
		}
	}
	const use = `{"type":"assistant","timestamp":"2026-09-12T10:00:00Z","cwd":"/w/repo","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go env GOPATH"}}]}}`

	tests := []struct {
		name       string
		transcript string
		want       ToolCallIndex
	}{
		{
			name: "successful result is executed",
			transcript: use + `
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"/go"}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeExecuted)},
		},
		{
			name: "failed command still counts as executed",
			transcript: use + `
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"Exit code 1"}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeExecuted)},
		},
		{
			name: "toolDenialKind user-rejected is a dialog rejection",
			transcript: use + `
{"type":"user","toolDenialKind":"user-rejected","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeUserRejected)},
		},
		{
			name: "any other toolDenialKind is a rule denial",
			transcript: use + `
{"type":"user","toolDenialKind":"permission-rule","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"Permission to use Bash has been denied."}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeDenied)},
		},
		{
			name: "rejection prose without toolDenialKind is a dialog rejection",
			transcript: use + `
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":[{"type":"text","text":"The user doesn't want to proceed with this tool use."}]}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeUserRejected)},
		},
		{
			name: "hook block without toolDenialKind is a rule denial",
			transcript: use + `
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"PreToolUse:Bash hook error: blocked"}]}}`,
			want: ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeDenied)},
		},
		{
			name:       "call without a result line has no result",
			transcript: use,
			want:       ToolCallIndex{"toolu_1": bash("toolu_1", OutcomeNoResult)},
		},
		{
			name: "result for an unseen call keeps its outcome",
			transcript: `
{"type":"user","toolDenialKind":"user-rejected","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_9","is_error":true,"content":"rejected"}]}}`,
			want: ToolCallIndex{"toolu_9": {ID: "toolu_9", Outcome: OutcomeUserRejected}},
		},
		{
			name: "blocks without ids and malformed lines are skipped",
			transcript: `
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
not json
{"type":"user","message":{"content":"plain prompt"}}`,
			want: ToolCallIndex{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ToolCallIndex{}
			got.AppendReader(strings.NewReader(tt.transcript))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("index = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToolCallIndexAppendFileJoinsSubagentTranscripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "s.jsonl")
	subPath := filepath.Join(dir, "agent-a.jsonl")
	mainLines := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_main","name":"WebFetch","input":{"url":"https://example.com"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_main","content":"ok"}]}}
`
	subLines := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"tool_use","id":"toolu_sub","name":"Bash","input":{"command":"go env GOARCH"}}]}}
{"type":"user","isSidechain":true,"toolDenialKind":"user-rejected","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_sub","is_error":true,"content":"rejected"}]}}
`
	if err := os.WriteFile(mainPath, []byte(mainLines), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subPath, []byte(subLines), 0o600); err != nil {
		t.Fatal(err)
	}

	idx := ToolCallIndex{}
	for _, p := range []string{mainPath, subPath} {
		if err := idx.AppendFile(p); err != nil {
			t.Fatalf("AppendFile(%s) = %v", p, err)
		}
	}
	if err := idx.AppendFile(filepath.Join(dir, "missing.jsonl")); err == nil {
		t.Error("AppendFile(missing) = nil, want error")
	}

	wantOutcomes := map[string]Outcome{"toolu_main": OutcomeExecuted, "toolu_sub": OutcomeUserRejected}
	gotOutcomes := map[string]Outcome{}
	for id, c := range idx {
		gotOutcomes[id] = c.Outcome
	}
	if !reflect.DeepEqual(gotOutcomes, wantOutcomes) {
		t.Errorf("outcomes = %v, want %v", gotOutcomes, wantOutcomes)
	}
}
