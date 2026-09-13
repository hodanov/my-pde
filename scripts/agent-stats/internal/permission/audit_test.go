package permission

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"agent-stats/internal/parser"
)

func TestAuditReport(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	entry := func(session, cwd string, ts time.Time, tool, input string, rules ...SuggestedRule) Entry {
		e := Entry{
			Timestamp:      ts,
			SessionID:      session,
			ToolName:       tool,
			Cwd:            cwd,
			TranscriptPath: "/p/" + session + ".jsonl",
			ToolInput:      json.RawMessage(input),
		}
		if len(rules) > 0 {
			e.Suggestions = []Suggestion{{Type: "addRules", Behavior: "allow", Destination: "localSettings", Rules: rules}}
		}
		return e
	}
	goEnv := SuggestedRule{ToolName: "Bash", RuleContent: "go env *"}
	call := func(id, tool, input string, ts time.Time, outcome parser.Outcome) parser.ToolCall {
		return parser.ToolCall{ID: id, Name: tool, Input: json.RawMessage(input), Timestamp: ts, Outcome: outcome}
	}
	repoOf := func(cwd string) string {
		if cwd == "/gone" {
			return ""
		}
		return cwd
	}
	type transcript struct {
		path string
		idx  parser.ToolCallIndex
	}
	subagentEntry := entry("s1", "/w/a", t1, "Bash", `{"command":"go env GOARCH"}`, goEnv)
	subagentEntry.AgentID = "ag1"

	tests := []struct {
		name        string
		entries     []Entry
		transcripts []transcript
		known       RuleSet
		locals      []LocalRules
		want        Report
	}{
		{
			name:    "approved prompt becomes a candidate",
			entries: []Entry{entry("s1", "/w/a", t1, "Bash", `{"command":"go env GOPATH"}`, goEnv)},
			transcripts: []transcript{{"/p/s1.jsonl", parser.ToolCallIndex{
				"toolu_1": call("toolu_1", "Bash", `{"command":"go env GOPATH","description":"d"}`, t1.Add(-time.Second), parser.OutcomeExecuted),
			}}},
			want: Report{
				Candidates: []Candidate{{
					Rule: "Bash(go env *)", Approved: 1, Sessions: 1,
					Repos: []string{"/w/a"}, LastSeen: t1, Examples: []string{"go env GOPATH"},
				}},
				Unsuggested: []Candidate{},
			},
		},
		{
			name: "rejected and pending prompts are counted per rule and past rejections per family",
			entries: []Entry{
				entry("s1", "/w/a", t1, "Bash", `{"command":"go env GOPATH"}`, goEnv),
				entry("s2", "/w/b", t2, "Bash", `{"command":"go env GOOS"}`, goEnv),
				entry("s2", "/w/b", t1, "Bash", `{"command":"go env GOARCH"}`, goEnv),
			},
			transcripts: []transcript{
				{"/p/s1.jsonl", parser.ToolCallIndex{
					"toolu_1": call("toolu_1", "Bash", `{"command":"go env GOPATH"}`, t1, parser.OutcomeExecuted),
				}},
				{"/p/s2.jsonl", parser.ToolCallIndex{
					"toolu_2": call("toolu_2", "Bash", `{"command":"go env GOOS"}`, t2, parser.OutcomeUserRejected),
					"toolu_9": call("toolu_9", "Bash", `{"command":"go build ./..."}`, t1, parser.OutcomeUserRejected),
				}},
			},
			want: Report{
				Candidates: []Candidate{{
					Rule: "Bash(go env *)", Approved: 1, Rejected: 1, Pending: 1, FamilyRejections: 2, Sessions: 2,
					Repos: []string{"/w/a", "/w/b"}, LastSeen: t2,
					Examples: []string{"go env GOPATH", "go env GOOS", "go env GOARCH"},
				}},
				Unsuggested: []Candidate{},
			},
		},
		{
			name:    "subagent call is matched in its agent transcript, not the session's",
			entries: []Entry{subagentEntry},
			transcripts: []transcript{
				{"/p/s1.jsonl", parser.ToolCallIndex{
					"toolu_m": call("toolu_m", "Bash", `{"command":"go env GOARCH"}`, t1, parser.OutcomeUserRejected),
				}},
				{"/p/s1/subagents/agent-ag1.jsonl", parser.ToolCallIndex{
					"toolu_s": call("toolu_s", "Bash", `{"command":"go env GOARCH"}`, t1, parser.OutcomeExecuted),
				}},
			},
			want: Report{
				Candidates: []Candidate{{
					Rule: "Bash(go env *)", Approved: 1, FamilyRejections: 1, Sessions: 1,
					Repos: []string{"/w/a"}, LastSeen: t1, Examples: []string{"go env GOARCH"},
				}},
				Unsuggested: []Candidate{},
			},
		},
		{
			name: "identical inputs resolve to the latest call before each prompt",
			entries: []Entry{
				entry("s1", "/w/a", t2, "Bash", `{"command":"go env GOPATH"}`, goEnv),
				entry("s1", "/w/a", t1, "Bash", `{"command":"go env GOPATH"}`, goEnv),
			},
			transcripts: []transcript{{"/p/s1.jsonl", parser.ToolCallIndex{
				"toolu_1": call("toolu_1", "Bash", `{"command":"go env GOPATH"}`, t1.Add(-time.Second), parser.OutcomeUserRejected),
				"toolu_2": call("toolu_2", "Bash", `{"command":"go env GOPATH"}`, t2.Add(-time.Second), parser.OutcomeExecuted),
				"toolu_3": call("toolu_3", "Bash", `{"command":"go env GOPATH"}`, t2.Add(time.Hour), parser.OutcomeExecuted),
			}}},
			want: Report{
				Candidates: []Candidate{{
					Rule: "Bash(go env *)", Approved: 1, Rejected: 1, FamilyRejections: 1, Sessions: 1,
					Repos: []string{"/w/a"}, LastSeen: t2, Examples: []string{"go env GOPATH"},
				}},
				Unsuggested: []Candidate{},
			},
		},
		{
			name:    "call stamped within the hook's second-level precision still matches",
			entries: []Entry{entry("s1", "/w/a", t1, "Bash", `{"command":"go env GOPATH"}`, goEnv)},
			transcripts: []transcript{{"/p/s1.jsonl", parser.ToolCallIndex{
				"toolu_1": call("toolu_1", "Bash", `{"command":"go env GOPATH"}`, t1.Add(800*time.Millisecond), parser.OutcomeExecuted),
			}}},
			want: Report{
				Candidates: []Candidate{{
					Rule: "Bash(go env *)", Approved: 1, Sessions: 1,
					Repos: []string{"/w/a"}, LastSeen: t1, Examples: []string{"go env GOPATH"},
				}},
				Unsuggested: []Candidate{},
			},
		},
		{
			name: "rules already configured or declined are left out",
			entries: []Entry{
				entry("s1", "/w/a", t1, "Bash", `{"command":"git status"}`, SuggestedRule{ToolName: "Bash", RuleContent: "git status *"}),
				entry("s1", "/w/a", t1, "WebFetch", `{"url":"https://example.com"}`, SuggestedRule{ToolName: "WebFetch", RuleContent: "domain:example.com"}),
			},
			known: NewRuleSet([]string{"Bash(git status:*)"}, []string{"WebFetch(domain:example.com)"}),
			want:  Report{Candidates: []Candidate{}, Unsuggested: []Candidate{}},
		},
		{
			name:    "local settings merge with prompted rules by normalized form",
			entries: []Entry{entry("s1", "/w/a", t1, "Bash", `{"command":"npm test"}`, SuggestedRule{ToolName: "Bash", RuleContent: "npm test *"})},
			transcripts: []transcript{{"/p/s1.jsonl", parser.ToolCallIndex{
				"toolu_1": call("toolu_1", "Bash", `{"command":"npm test"}`, t1, parser.OutcomeExecuted),
			}}},
			locals: []LocalRules{
				{Repo: "/w/b", Allow: []string{"Bash(npm test:*)", "Bash(uv run *)"}},
				{Repo: "/w/c", Allow: []string{"Bash(uv run *)"}},
			},
			want: Report{
				Candidates: []Candidate{
					{
						Rule: "Bash(npm test *)", Approved: 1, Sessions: 1, Repos: []string{"/w/a"},
						LocalRepos: []string{"/w/b"}, LastSeen: t1, Examples: []string{"npm test"},
					},
					{Rule: "Bash(uv run *)", LocalRepos: []string{"/w/b", "/w/c"}},
				},
				Unsuggested: []Candidate{},
			},
		},
		{
			name: "prompts without suggestions are drafted into unsuggested",
			entries: []Entry{
				entry("s1", "/gone", t1, "Read", `{"file_path":"/etc/hosts"}`),
				entry("s1", "/w/a", t2, "Bash", `{"command":"docker compose up"}`),
				entry("s2", "/w/a", t1, "mcp__gmail__search_threads", `{"q":"x"}`),
			},
			transcripts: []transcript{{"/p/s1.jsonl", parser.ToolCallIndex{
				"toolu_5": call("toolu_5", "Read", `{"file_path":"/etc/hosts"}`, t1, parser.OutcomeExecuted),
				"toolu_6": call("toolu_6", "Bash", `{"command":"docker compose up"}`, t2, parser.OutcomeExecuted),
			}}},
			want: Report{
				Candidates: []Candidate{},
				Unsuggested: []Candidate{
					{Rule: "Bash(docker *)", Approved: 1, Sessions: 1, Repos: []string{"/w/a"}, LastSeen: t2, Examples: []string{"docker compose up"}},
					{Rule: "Read(//etc/hosts)", Approved: 1, Sessions: 1, LastSeen: t1, Examples: []string{"/etc/hosts"}},
					{Rule: "mcp__gmail__search_threads", Pending: 1, Sessions: 1, Repos: []string{"/w/a"}, LastSeen: t1, Examples: []string{`{"q":"x"}`}},
				},
			},
		},
		{
			name:    "unmatched prompt is drafted and exemplified from its recorded input",
			entries: []Entry{entry("s9", "/w/a", t1, "Write", `{"file_path":"/x/y"}`)},
			want: Report{
				Candidates: []Candidate{},
				Unsuggested: []Candidate{
					{Rule: "Write(//x/y)", Pending: 1, Sessions: 1, Repos: []string{"/w/a"}, LastSeen: t1, Examples: []string{"/x/y"}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := NewAudit(tt.entries)
			for _, tr := range tt.transcripts {
				a.AddTranscript(tr.path, tr.idx)
			}
			got := a.Report(tt.known, tt.locals, repoOf)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Report =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestInputMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		call     string
		recorded string
		want     bool
	}{
		{name: "identical input", call: `{"command":"ls","timeout":120000}`, recorded: `{"timeout":120000,"command":"ls"}`, want: true},
		{name: "recorded input without the file body still matches", call: `{"file_path":"/x","content":"body"}`, recorded: `{"file_path":"/x"}`, want: true},
		{name: "a differing value does not match", call: `{"command":"ls -a"}`, recorded: `{"command":"ls"}`, want: false},
		{name: "a recorded key missing from the call does not match", call: `{"command":"ls"}`, recorded: `{"command":"ls","description":"d"}`, want: false},
		{name: "nothing recorded matches nothing", call: `{"command":"ls"}`, recorded: ``, want: false},
		{name: "malformed call input does not match", call: `{`, recorded: `{"command":"ls"}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inputMatches(json.RawMessage(tt.call), json.RawMessage(tt.recorded)); got != tt.want {
				t.Errorf("inputMatches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuditCwds(t *testing.T) {
	t.Parallel()
	a := NewAudit([]Entry{{SessionID: "s1", ToolName: "Bash", Cwd: "/w/a"}})
	a.AddTranscript("/p/x.jsonl", parser.ToolCallIndex{
		"toolu_8": {ID: "toolu_8", Cwd: "/w/b"},
		"toolu_9": {ID: "toolu_9"},
		"toolu_1": {ID: "toolu_1", Cwd: "/w/a"},
	})
	want := []string{"/w/a", "/w/b"}
	if got := a.Cwds(); !reflect.DeepEqual(got, want) {
		t.Errorf("Cwds = %q, want %q", got, want)
	}
}
