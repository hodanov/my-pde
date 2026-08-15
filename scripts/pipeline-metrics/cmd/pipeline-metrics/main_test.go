package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden markdown file")

const fixedNow = "2026-08-15T00:00:00Z"

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

// fixtureArgs is the canonical invocation the golden output is pinned to. The
// clock is fixed so the report does not drift with wall time.
func fixtureArgs(format string) []string {
	return []string{
		"--issues", testdata("issues.json"),
		"--prs", testdata("prs.json"),
		"--now", fixedNow,
		"--format", format,
	}
}

func TestRunMarkdownMatchesGolden(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if runErr := run(fixtureArgs("markdown"), strings.NewReader(""), &out); runErr != nil {
		t.Fatalf("run() error = %v", runErr)
	}

	// The golden file holds markdown but is stored as .txt: `prettier --check .`
	// runs over the whole repository and would reformat a .md file, which must
	// stay byte-identical to what the CLI emits.
	golden := testdata("golden.txt")
	if *update {
		if writeErr := os.WriteFile(golden, out.Bytes(), 0o644); writeErr != nil {
			t.Fatalf("write golden: %v", writeErr)
		}
		return
	}
	want, readErr := os.ReadFile(golden)
	if readErr != nil {
		t.Fatalf("read golden: %v (regenerate with: go test ./cmd/pipeline-metrics -update)", readErr)
	}
	if out.String() != string(want) {
		t.Errorf("markdown output drifted from %s; regenerate with: go test ./cmd/pipeline-metrics -update", golden)
	}
}

func TestRunIsDeterministic(t *testing.T) {
	t.Parallel()

	var first, second bytes.Buffer
	if runErr := run(fixtureArgs("markdown"), strings.NewReader(""), &first); runErr != nil {
		t.Fatalf("run() error = %v", runErr)
	}
	if runErr := run(fixtureArgs("markdown"), strings.NewReader(""), &second); runErr != nil {
		t.Fatalf("run() error = %v", runErr)
	}
	if first.String() != second.String() {
		t.Error("two runs over the same fixtures disagree")
	}
}

func TestRunFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		format  string
		wants   []string
		wantErr bool
	}{
		{
			name:   "markdown carries the flow table and the embedded json",
			format: "markdown",
			wants:  []string{"## 直近 90 日の成績（フロー）", "`scan:scripts`", "```json"},
		},
		{
			name:   "alerts names the failing scan and the prompt to edit",
			format: "alerts",
			wants:  []string{"> [!WARNING]", "scan:scripts", "routines/prompts/weekly-scripts-tooling-scan.md"},
		},
		{
			name:   "json is the machine-readable form",
			format: "json",
			wants:  []string{`"adopted_rate"`, `"alerts"`},
		},
		{
			name:    "an unknown format is rejected rather than guessed at",
			format:  "csv",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			runErr := run(fixtureArgs(tt.format), strings.NewReader(""), &out)
			if tt.wantErr {
				if runErr == nil {
					t.Fatal("run() error = nil, want an error")
				}
				return
			}
			if runErr != nil {
				t.Fatalf("run() error = %v", runErr)
			}
			for _, want := range tt.wants {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output is missing %q", want)
				}
			}
		})
	}
}

func TestRunJSONDecodesToTheDigestSchema(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if runErr := run(fixtureArgs("json"), strings.NewReader(""), &out); runErr != nil {
		t.Fatalf("run() error = %v", runErr)
	}

	// The meta loop reads this shape out of the digest issue, so the field names
	// it selects on are part of the contract.
	var decoded struct {
		MinSample int `json:"min_sample"`
		Scans     []struct {
			Label  string `json:"label"`
			Sample int    `json:"sample"`
		} `json:"scans"`
		Alerts []struct {
			Metric  string   `json:"metric"`
			Scope   string   `json:"scope"`
			Prompts []string `json:"prompts"`
		} `json:"alerts"`
	}
	if decodeErr := json.Unmarshal(out.Bytes(), &decoded); decodeErr != nil {
		t.Fatalf("unmarshal: %v", decodeErr)
	}
	if decoded.MinSample != 5 {
		t.Errorf("min_sample = %d, want 5", decoded.MinSample)
	}

	worst := ""
	for _, scan := range decoded.Scans {
		if scan.Label == "scan:scripts" {
			worst = scan.Label
			if scan.Sample != 8 {
				t.Errorf("scan:scripts sample = %d, want 8", scan.Sample)
			}
		}
	}
	if worst == "" {
		t.Fatal("scan:scripts is missing from the report")
	}
	for _, alert := range decoded.Alerts {
		if alert.Metric == "adopted_rate" && alert.Scope == "scan:scripts" {
			if len(alert.Prompts) == 0 {
				t.Error("the adopted_rate alert does not name a prompt file")
			}
			return
		}
	}
	t.Error("no adopted_rate alert for scan:scripts, which the fixture is built to trip")
}

func TestRunInputHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		stdin   string
		wantErr bool
	}{
		{
			name:    "both inputs are required",
			args:    []string{"--issues", testdata("issues.json")},
			wantErr: true,
		},
		{
			name:    "only one input can read stdin",
			args:    []string{"--issues", "-", "--prs", "-"},
			wantErr: true,
		},
		{
			name:    "a missing file is reported",
			args:    []string{"--issues", testdata("does-not-exist.json"), "--prs", testdata("prs.json")},
			wantErr: true,
		},
		{
			name:    "an unparseable --now is reported",
			args:    []string{"--issues", testdata("issues.json"), "--prs", testdata("prs.json"), "--now", "yesterday"},
			wantErr: true,
		},
		{
			name:    "an unparseable --since is reported",
			args:    []string{"--issues", testdata("issues.json"), "--prs", testdata("prs.json"), "--since", "2026/06/28"},
			wantErr: true,
		},
		{
			name:  "issues can arrive on stdin",
			args:  []string{"--issues", "-", "--prs", testdata("prs.json"), "--now", fixedNow},
			stdin: `[{"number":1,"state":"OPEN","labels":[{"name":"scan:nvim"}],"createdAt":"2026-08-01T00:00:00Z"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			runErr := run(tt.args, strings.NewReader(tt.stdin), &out)
			if tt.wantErr && runErr == nil {
				t.Fatal("run() error = nil, want an error")
			}
			if !tt.wantErr && runErr != nil {
				t.Fatalf("run() error = %v", runErr)
			}
		})
	}
}
