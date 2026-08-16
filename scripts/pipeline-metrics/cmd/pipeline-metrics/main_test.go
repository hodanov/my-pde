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

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

// fixtureNow pins the evaluation time so every golden is reproducible.
const fixtureNow = "2026-08-16T00:00:00Z"

func fixturePath(name string) string { return filepath.Join("..", "..", "testdata", name) }

func fixtureArgs(format string) []string {
	return []string{
		"--issues", fixturePath("issues.json"),
		"--prs", fixturePath("prs.json"),
		"--now", fixtureNow,
		"--format", format,
	}
}

func runFixture(t *testing.T, format string) string {
	t.Helper()
	var out bytes.Buffer
	if runErr := run(fixtureArgs(format), strings.NewReader(""), &out); runErr != nil {
		t.Fatalf("run(%s) returned error: %v", format, runErr)
	}
	return out.String()
}

func TestRunGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		golden string
	}{
		{name: "flow section", format: "markdown", golden: "digest.golden"},
		{name: "warning block", format: "alerts", golden: "alerts.golden"},
		{name: "machine readable report", format: "json", golden: "metrics.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := runFixture(t, tt.format)
			path := fixturePath(tt.golden)
			if *update {
				if writeErr := os.WriteFile(path, []byte(got), 0o644); writeErr != nil {
					t.Fatalf("update %s: %v", path, writeErr)
				}
				return
			}
			want, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			if got != string(want) {
				t.Errorf("output does not match %s (run `go test ./... -update` to refresh)\n--- got ---\n%s", tt.golden, got)
			}
		})
	}
}

// TestRunIsDeterministic guards the property the digest depends on: the same
// inputs and the same --now must produce byte-identical output, whatever order
// Go happens to walk the internal maps in.
func TestRunIsDeterministic(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"markdown", "alerts", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			first := runFixture(t, format)
			for i := range 5 {
				if got := runFixture(t, format); got != first {
					t.Fatalf("run %d differs from the first run", i+2)
				}
			}
		})
	}
}

// TestJSONContract pins the field names the monthly meta loop reads out of the
// digest. Renaming any of them is a breaking change for that prompt.
func TestJSONContract(t *testing.T) {
	t.Parallel()
	var doc map[string]any
	if unmarshalErr := json.Unmarshal([]byte(runFixture(t, "json")), &doc); unmarshalErr != nil {
		t.Fatalf("digest JSON does not unmarshal: %v", unmarshalErr)
	}

	for _, key := range []string{
		"generated_at", "since", "window_start", "window_days", "excluded_before_since",
		"min_sample", "alert_min_sample", "scans", "total", "months", "backlog", "alerts", "anomalies",
	} {
		if _, ok := doc[key]; !ok {
			t.Errorf("top-level field %q is missing", key)
		}
	}

	scans, ok := doc["scans"].([]any)
	if !ok || len(scans) == 0 {
		t.Fatalf("scans is not a non-empty array: %#v", doc["scans"])
	}
	scan, ok := scans[0].(map[string]any)
	if !ok {
		t.Fatalf("scan entry is not an object: %#v", scans[0])
	}
	for _, key := range []string{
		"scan", "opened", "opened_last_28d", "adopted", "rejected", "untriaged", "untracked_close",
		"adopted_rate", "rejected_after_pr_rate", "oldest_untriaged_days", "reject_latency_days_p50",
		"pr_created_rate", "pr_pending", "pr_lag_days_p50", "merge_rate", "merge_lead_days_p50",
		"merge_lead_days_p90", "e2e_lead_days_p50",
	} {
		if _, found := scan[key]; !found {
			t.Errorf("scan field %q is missing", key)
		}
	}

	alerts, ok := doc["alerts"].([]any)
	if !ok || len(alerts) == 0 {
		t.Fatalf("alerts is not a non-empty array: %#v", doc["alerts"])
	}
	alert, ok := alerts[0].(map[string]any)
	if !ok {
		t.Fatalf("alert entry is not an object: %#v", alerts[0])
	}
	for _, key := range []string{"kind", "scope", "value", "threshold", "observed", "sample", "owner_prompt"} {
		if _, found := alert[key]; !found {
			t.Errorf("alert field %q is missing", key)
		}
	}
}

// TestFixtureFiresEveryAlertKind keeps the fixture honest: if a threshold stops
// tripping, the golden alone would happily record the silence.
func TestFixtureFiresEveryAlertKind(t *testing.T) {
	t.Parallel()
	got := runFixture(t, "alerts")
	for _, want := range []string{
		"直近 28 日で 0 件", // liveness
		"最古の未 triage",  // triage_backlog
		"の採用率",         // adopted_rate
		"PR 化待ちが",      // pr_created_rate
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning block is missing %q:\n%s", want, got)
		}
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing inputs",
			args:    []string{"--format", "json"},
			wantErr: "--issues and --prs are required",
		},
		{
			name:    "both inputs on stdin",
			args:    []string{"--issues", "-", "--prs", "-"},
			wantErr: "only one of",
		},
		{
			name:    "unknown format",
			args:    fixtureArgs("html"),
			wantErr: `unknown --format "html"`,
		},
		{
			name:    "bad since",
			args:    append(fixtureArgs("json"), "--since", "yesterday"),
			wantErr: `invalid --since "yesterday"`,
		},
		{
			name:    "bad now",
			args:    append(fixtureArgs("json"), "--now", "2026-08-16"),
			wantErr: `invalid --now "2026-08-16"`,
		},
		{
			name:    "missing file",
			args:    []string{"--issues", fixturePath("nope.json"), "--prs", fixturePath("prs.json")},
			wantErr: "read issues",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			runErr := run(tt.args, strings.NewReader(""), &out)
			if runErr == nil {
				t.Fatalf("expected an error, got output: %s", out.String())
			}
			if !strings.Contains(runErr.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", runErr.Error(), tt.wantErr)
			}
		})
	}
}

// TestRunReadsStdin covers the "pipe one side in" path the workflow may use.
func TestRunReadsStdin(t *testing.T) {
	t.Parallel()
	issues, readErr := os.ReadFile(fixturePath("issues.json"))
	if readErr != nil {
		t.Fatalf("read fixture: %v", readErr)
	}
	var out bytes.Buffer
	args := []string{"--issues", "-", "--prs", fixturePath("prs.json"), "--now", fixtureNow, "--format", "json"}
	if runErr := run(args, bytes.NewReader(issues), &out); runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if !strings.Contains(out.String(), `"scan:alpha"`) {
		t.Errorf("stdin input produced no scan rows:\n%s", out.String())
	}
}

// TestRunHandlesEmptyExports keeps a failed `gh` call from taking the digest
// down with it: no data must still render a section.
func TestRunHandlesEmptyExports(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	if writeErr := os.WriteFile(empty, []byte("[]\n"), 0o600); writeErr != nil {
		t.Fatalf("write fixture: %v", writeErr)
	}
	var out bytes.Buffer
	args := []string{"--issues", empty, "--prs", empty, "--now", fixtureNow}
	if runErr := run(args, strings.NewReader(""), &out); runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if !strings.Contains(out.String(), "## フロー指標") {
		t.Errorf("empty input produced no flow section:\n%s", out.String())
	}
}
