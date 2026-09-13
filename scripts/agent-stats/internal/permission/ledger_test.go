package permission

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadLedger(t *testing.T) {
	t.Parallel()
	line := func(ts string) string {
		return `{"ts":"` + ts + `","session_id":"s1","prompt_id":"p1","agent_id":null,"tool_name":"Bash","cwd":"/w/a","transcript_path":"/p/s1.jsonl","permission_mode":"acceptEdits","tool_input":{"command":"go env GOPATH","description":"d"},"suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"go env *"}],"behavior":"allow","destination":"localSettings"}]}`
	}
	entry := func(ts time.Time) Entry {
		return Entry{
			Timestamp:      ts,
			SessionID:      "s1",
			PromptID:       "p1",
			ToolName:       "Bash",
			Cwd:            "/w/a",
			TranscriptPath: "/p/s1.jsonl",
			PermissionMode: "acceptEdits",
			ToolInput:      json.RawMessage(`{"command":"go env GOPATH","description":"d"}`),
			Suggestions: []Suggestion{{
				Type:        "addRules",
				Rules:       []SuggestedRule{{ToolName: "Bash", RuleContent: "go env *"}},
				Behavior:    "allow",
				Destination: "localSettings",
			}},
		}
	}
	t0 := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		ledger string
		want   []Entry
	}{
		{name: "single entry", ledger: line("2026-09-12T10:00:00Z"), want: []Entry{entry(t0)}},
		{
			name:   "double-wired duplicate seconds apart keeps the first",
			ledger: line("2026-09-12T10:00:00Z") + "\n" + line("2026-09-12T10:00:01Z"),
			want:   []Entry{entry(t0)},
		},
		{
			name:   "same call prompted again later is a separate entry",
			ledger: line("2026-09-12T10:00:00Z") + "\n" + line("2026-09-12T10:05:00Z"),
			want:   []Entry{entry(t0), entry(t0.Add(5 * time.Minute))},
		},
		{
			name:   "malformed lines and lines without a session or tool are skipped",
			ledger: "not json\n{\"tool_name\":\"Bash\"}\n{\"session_id\":\"s1\"}\n\n" + line("2026-09-12T10:00:00Z"),
			want:   []Entry{entry(t0)},
		},
		{name: "empty ledger", ledger: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReadLedger(strings.NewReader(tt.ledger))
			if err != nil {
				t.Fatalf("ReadLedger err = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadLedger = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEntryTranscriptFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{name: "main-loop call is in the session transcript", entry: Entry{TranscriptPath: "/p/s1.jsonl"}, want: "/p/s1.jsonl"},
		{
			name:  "subagent call is in the agent transcript",
			entry: Entry{TranscriptPath: "/p/s1.jsonl", AgentID: "af43"},
			want:  "/p/s1/subagents/agent-af43.jsonl",
		},
		{name: "no transcript path", entry: Entry{AgentID: "af43"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.entry.transcriptFile(); got != tt.want {
				t.Errorf("transcriptFile = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEntryAllowRules(t *testing.T) {
	t.Parallel()
	allow := func(rules ...SuggestedRule) Suggestion {
		return Suggestion{Type: "addRules", Behavior: "allow", Destination: "localSettings", Rules: rules}
	}

	tests := []struct {
		name        string
		suggestions []Suggestion
		want        []string
	}{
		{
			name:        "content rule is wrapped and the colon form normalized",
			suggestions: []Suggestion{allow(SuggestedRule{ToolName: "Bash", RuleContent: "npm test:*"})},
			want:        []string{"Bash(npm test *)"},
		},
		{
			name:        "rule without content is the bare tool name",
			suggestions: []Suggestion{allow(SuggestedRule{ToolName: "mcp__cal__list_events"})},
			want:        []string{"mcp__cal__list_events"},
		},
		{
			name: "compound command yields one rule per subcommand",
			suggestions: []Suggestion{allow(
				SuggestedRule{ToolName: "Bash", RuleContent: "go env *"},
				SuggestedRule{ToolName: "Bash", RuleContent: "go list *"},
			)},
			want: []string{"Bash(go env *)", "Bash(go list *)"},
		},
		{
			name: "duplicate rules collapse",
			suggestions: []Suggestion{
				allow(SuggestedRule{ToolName: "Bash", RuleContent: "go env *"}),
				allow(SuggestedRule{ToolName: "Bash", RuleContent: "go env:*"}),
			},
			want: []string{"Bash(go env *)"},
		},
		{
			name: "deny suggestions and other update types are ignored",
			suggestions: []Suggestion{
				{Type: "addRules", Behavior: "deny", Rules: []SuggestedRule{{ToolName: "Bash", RuleContent: "rm *"}}},
				{Type: "addDirectories", Behavior: "allow"},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := Entry{Suggestions: tt.suggestions}
			if got := e.allowRules(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("allowRules = %q, want %q", got, tt.want)
			}
		})
	}
}
