package lint

import (
	"sort"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		src             string
		wantPresent     bool
		wantClosed      bool
		wantName        string
		wantDescription string
		wantHasMetadata bool
		wantBody        string
	}{
		{
			name:            "simple scalars",
			src:             "---\nname: foo\ndescription: bar\n---\nbody text",
			wantPresent:     true,
			wantClosed:      true,
			wantName:        "foo",
			wantDescription: "bar",
			wantBody:        "body text",
		},
		{
			name:            "folded block scalar",
			src:             "---\nname: foo\ndescription: >-\n  line one\n  line two\n---\n# Heading",
			wantPresent:     true,
			wantClosed:      true,
			wantName:        "foo",
			wantDescription: "line one line two",
			wantBody:        "# Heading",
		},
		{
			name:            "nested metadata map",
			src:             "---\nname: foo\ndescription: bar\nmetadata:\n  version: 1\n---\n",
			wantPresent:     true,
			wantClosed:      true,
			wantName:        "foo",
			wantDescription: "bar",
			wantHasMetadata: true,
			wantBody:        "",
		},
		{
			name:            "double quoted value",
			src:             "---\nname: foo\ndescription: \"quoted val\"\n---\n",
			wantPresent:     true,
			wantClosed:      true,
			wantName:        "foo",
			wantDescription: "quoted val",
		},
		{
			name:        "no frontmatter",
			src:         "# just a heading\ntext",
			wantPresent: false,
			wantClosed:  false,
		},
		{
			name:            "unterminated frontmatter",
			src:             "---\nname: foo\ndescription: bar\n",
			wantPresent:     true,
			wantClosed:      false,
			wantName:        "foo",
			wantDescription: "bar",
		},
		{
			name:            "empty name value",
			src:             "---\nname:\ndescription: bar\n---\n",
			wantPresent:     true,
			wantClosed:      true,
			wantName:        "",
			wantDescription: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := parseFrontmatter([]byte(tt.src))
			if fm.present != tt.wantPresent {
				t.Fatalf("present = %v, want %v", fm.present, tt.wantPresent)
			}
			if fm.closed != tt.wantClosed {
				t.Fatalf("closed = %v, want %v", fm.closed, tt.wantClosed)
			}
			if got := fm.value("name"); got != tt.wantName {
				t.Fatalf("name = %q, want %q", got, tt.wantName)
			}
			if got := fm.value("description"); got != tt.wantDescription {
				t.Fatalf("description = %q, want %q", got, tt.wantDescription)
			}
			if got := fm.has("metadata"); got != tt.wantHasMetadata {
				t.Fatalf("has(metadata) = %v, want %v", got, tt.wantHasMetadata)
			}
			if tt.wantClosed && fm.body != tt.wantBody {
				t.Fatalf("body = %q, want %q", fm.body, tt.wantBody)
			}
		})
	}
}

func TestExtractAgentRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "backtick key then backtick name",
			body: "### Step 2\n- `subagent_type`: `investigation-scout`\n",
			want: []string{"investigation-scout"},
		},
		{
			name: "key and name in one backtick span",
			body: "Agent tool で `subagent_type: verify-runner` に委譲する。verify-runner が使えない環境では自分で実行する。",
			want: []string{"verify-runner"},
		},
		{
			name: "markdown table column",
			body: "起動する4エージェント:\n\n" +
				"| Agent            | subagent_type    | 観点   |\n" +
				"| ---------------- | ---------------- | ------ |\n" +
				"| review-security  | review-security  | sec    |\n" +
				"| review-performance | review-performance | perf |\n",
			want: []string{"review-performance", "review-security"},
		},
		{
			name: "unrelated backtick span on signal line is ignored",
			body: "- `subagent_type`: `real-agent` を `medium` の深さで起動する\n",
			want: []string{"real-agent"},
		},
		{
			name: "backticked table header and cells",
			body: "| Agent | `subagent_type` |\n" +
				"| ----- | --------------- |\n" +
				"| sec   | `review-security` |\n",
			want: []string{"review-security"},
		},
		{
			name: "prose mention without signal is ignored",
			body: "investigation-scout と investigation-diver による2フェーズ調査。",
			want: nil,
		},
		{
			name: "illustrative file path is ignored",
			body: "出力先: `agents/investigation-diver.md` に書き出す。",
			want: nil,
		},
		{
			name: "duplicate references collapse",
			body: "- `subagent_type`: `verify-runner`\n再度 `subagent_type: verify-runner` を起動する。",
			want: []string{"verify-runner"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAgentRefs(tt.body)
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("refs = %v, want %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("refs = %v, want %v", got, want)
				}
			}
		})
	}
}
