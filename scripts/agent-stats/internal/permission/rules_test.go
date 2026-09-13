package permission

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"agent-stats/internal/parser"
)

func TestNormalizeRule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rule string
		want string
	}{
		{name: "trailing colon wildcard becomes a spaced wildcard", rule: "Bash(git:*)", want: "Bash(git *)"},
		{name: "spaced wildcard is unchanged", rule: "Bash(git *)", want: "Bash(git *)"},
		{name: "colon before a mid-pattern wildcard is literal", rule: "Bash(git:* push)", want: "Bash(git:* push)"},
		{name: "domain rule keeps its colon", rule: "WebFetch(domain:example.com)", want: "WebFetch(domain:example.com)"},
		{name: "bare tool name is unchanged", rule: "mcp__cal__list_events", want: "mcp__cal__list_events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeRule(tt.rule); got != tt.want {
				t.Errorf("normalizeRule(%q) = %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

func TestSettingsRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings string
		want     []string
		wantErr  bool
	}{
		{
			name:     "allow, ask and deny are all configured rules",
			settings: `{"permissions":{"allow":["Bash(git *)"],"ask":["Bash(git push *)"],"deny":["Bash(rm -rf *)"]},"model":"x"}`,
			want:     []string{"Bash(git *)", "Bash(git push *)", "Bash(rm -rf *)"},
		},
		{name: "no permissions block", settings: `{"model":"x"}`, want: nil},
		{name: "malformed settings", settings: `{"permissions":`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SettingsRules(strings.NewReader(tt.settings))
			if (err != nil) != tt.wantErr {
				t.Fatalf("SettingsRules err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SettingsRules = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLocalAllowRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings string
		want     []string
		wantErr  bool
	}{
		{
			name:     "only allow rules are local grants",
			settings: `{"permissions":{"allow":["Bash(uv run *)"],"deny":["Bash(rm *)"]},"enabledPlugins":{}}`,
			want:     []string{"Bash(uv run *)"},
		},
		{name: "malformed settings", settings: `[`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LocalAllowRules(strings.NewReader(tt.settings))
			if (err != nil) != tt.wantErr {
				t.Fatalf("LocalAllowRules err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LocalAllowRules = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadDeclined(t *testing.T) {
	t.Parallel()
	got, err := ReadDeclined(strings.NewReader("Bash(curl *)\n\n  WebFetch(domain:example.com)  \n"))
	if err != nil {
		t.Fatalf("ReadDeclined err = %v", err)
	}
	want := []string{"Bash(curl *)", "WebFetch(domain:example.com)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadDeclined = %q, want %q", got, want)
	}
}

func TestRuleSetContains(t *testing.T) {
	t.Parallel()
	set := NewRuleSet([]string{"Bash(go:*)"}, []string{"WebFetch(domain:example.com)"})
	tests := []struct {
		name string
		rule string
		want bool
	}{
		{name: "colon form matches the spaced form", rule: "Bash(go *)", want: true},
		{name: "rule from the second list", rule: "WebFetch(domain:example.com)", want: true},
		{name: "unrelated rule", rule: "Bash(go env *)", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := set.Contains(tt.rule); got != tt.want {
				t.Errorf("Contains(%q) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}

func TestRuleFamily(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rule string
		want string
	}{
		{name: "bash rule is keyed by its leading command", rule: "Bash(go env *)", want: "Bash:go"},
		{name: "bash rule with an absolute command path", rule: "Bash(/usr/bin/make build)", want: "Bash:make"},
		{name: "webfetch rule is keyed by its domain", rule: "WebFetch(domain:example.com)", want: "WebFetch:example.com"},
		{name: "bare tool rule is keyed by the tool", rule: "mcp__gmail__search_threads", want: "mcp__gmail__search_threads"},
		{name: "path rule has no family", rule: "Read(//etc/**)", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ruleFamily(tt.rule); got != tt.want {
				t.Errorf("ruleFamily(%q) = %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

func TestCallFamily(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		call parser.ToolCall
		want string
	}{
		{name: "bash call steps over a cd prefix", call: parser.ToolCall{Name: "Bash", Input: json.RawMessage(`{"command":"cd /x && go env GOOS"}`)}, want: "Bash:go"},
		{name: "webfetch call is keyed by host", call: parser.ToolCall{Name: "WebFetch", Input: json.RawMessage(`{"url":"https://example.com/a?b=1"}`)}, want: "WebFetch:example.com"},
		{name: "mcp call is keyed by the tool", call: parser.ToolCall{Name: "mcp__gmail__search_threads", Input: json.RawMessage(`{}`)}, want: "mcp__gmail__search_threads"},
		{name: "file tool has no family", call: parser.ToolCall{Name: "Read", Input: json.RawMessage(`{"file_path":"/etc/hosts"}`)}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := callFamily(&tt.call); got != tt.want {
				t.Errorf("callFamily = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDraftRule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		call parser.ToolCall
		want string
	}{
		{name: "bash call drafts its leading command", call: parser.ToolCall{Name: "Bash", Input: json.RawMessage(`{"command":"docker compose up"}`)}, want: "Bash(docker *)"},
		{name: "webfetch call drafts its domain", call: parser.ToolCall{Name: "WebFetch", Input: json.RawMessage(`{"url":"https://example.com/x"}`)}, want: "WebFetch(domain:example.com)"},
		{name: "file tool drafts its absolute path", call: parser.ToolCall{Name: "Read", Input: json.RawMessage(`{"file_path":"/etc/hosts"}`)}, want: "Read(//etc/hosts)"},
		{name: "anything else drafts the bare tool", call: parser.ToolCall{Name: "mcp__gmail__search_threads"}, want: "mcp__gmail__search_threads"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := draftRule(&tt.call); got != tt.want {
				t.Errorf("draftRule = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeCall(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 130)
	tests := []struct {
		name string
		call parser.ToolCall
		want string
	}{
		{name: "bash keeps only the first line", call: parser.ToolCall{Name: "Bash", Input: json.RawMessage(`{"command":"cat <<EOF\nbody\nEOF"}`)}, want: "cat <<EOF"},
		{name: "long text is truncated", call: parser.ToolCall{Name: "Bash", Input: json.RawMessage(`{"command":"` + long + `"}`)}, want: strings.Repeat("a", 120) + "…"},
		{name: "webfetch shows the url", call: parser.ToolCall{Name: "WebFetch", Input: json.RawMessage(`{"url":"https://example.com","prompt":"p"}`)}, want: "https://example.com"},
		{name: "file tool shows the path", call: parser.ToolCall{Name: "Edit", Input: json.RawMessage(`{"file_path":"/x/y.go","old_string":"a"}`)}, want: "/x/y.go"},
		{name: "other tools show their input", call: parser.ToolCall{Name: "mcp__x", Input: json.RawMessage(`{"q":"z"}`)}, want: `{"q":"z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := summarizeCall(&tt.call); got != tt.want {
				t.Errorf("summarizeCall = %q, want %q", got, tt.want)
			}
		})
	}
}
