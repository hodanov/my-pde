package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pipeline-metrics/internal/metrics"
	"pipeline-metrics/internal/model"
)

func testSummary(t *testing.T, issues []model.Issue, prs []model.PullRequest) metrics.Summary {
	t.Helper()
	now, _ := time.Parse(time.RFC3339, "2026-08-15T00:00:00Z")
	since, _ := time.Parse(time.RFC3339, "2026-06-28T00:00:00Z")
	return metrics.Summarize(issues, prs, metrics.Options{Now: now, Since: since})
}

func adoptedIssue(number int, label string) model.Issue {
	return adoptedIssueAt(number, label, "2026-08-01T00:00:00Z")
}

func adoptedIssueAt(number int, label, created string) model.Issue {
	at, _ := time.Parse(time.RFC3339, created)
	return model.Issue{Number: number, ScanLabel: label, ScanLabelCount: 1, Triage: model.Adopted, CreatedAt: at}
}

func TestRatio(t *testing.T) {
	t.Parallel()

	rate := 0.812
	tests := []struct {
		name  string
		input metrics.Ratio
		want  string
	}{
		{name: "withheld rate shows an em dash with its denominator", input: metrics.Ratio{Num: 1, Denom: 3}, want: "— (n=3)"},
		{name: "empty denominator", input: metrics.Ratio{}, want: "— (n=0)"},
		{name: "rate rounds to a whole percent", input: metrics.Ratio{Rate: &rate, Num: 43, Denom: 53}, want: "81% (n=53)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ratio(tt.input); got != tt.want {
				t.Errorf("ratio() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDaysAndNumbers(t *testing.T) {
	t.Parallel()

	value := 1.25
	if got := days(nil); got != "—" {
		t.Errorf("days(nil) = %q, want an em dash", got)
	}
	if got := days(&value); got != "1.2 日" {
		t.Errorf("days() = %q, want %q", got, "1.2 日")
	}
	if got := numbers(nil); got != "なし" {
		t.Errorf("numbers(nil) = %q, want なし", got)
	}
	if got := numbers([]int{640, 623}); got != "#640 #623" {
		t.Errorf("numbers() = %q, want %q", got, "#640 #623")
	}
}

func TestAlertsIsEmptyWhenNothingBreached(t *testing.T) {
	t.Parallel()

	sum := testSummary(t, []model.Issue{adoptedIssue(1, "scan:nvim")}, nil)
	sum.Alerts = nil
	if got := Alerts(&sum); got != "" {
		t.Errorf("Alerts() = %q, want an empty string so the caller can test for emptiness", got)
	}
}

func TestAlertsRendersACalloutWithPromptPaths(t *testing.T) {
	t.Parallel()

	// A scan that has not raised anything for 28 days trips the liveness alert.
	sum := testSummary(t, []model.Issue{adoptedIssueAt(1, "scan:environment", "2026-06-30T00:00:00Z")}, nil)
	got := Alerts(&sum)
	if !strings.HasPrefix(got, "> [!WARNING]\n") {
		t.Fatalf("Alerts() = %q, want a GitHub warning callout", got)
	}
	if !strings.Contains(got, "`routines/prompts/weekly-environment-scan.md`") {
		t.Errorf("Alerts() does not name the prompt to edit:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if !strings.HasPrefix(line, ">") {
			t.Errorf("line escapes the callout: %q", line)
		}
	}
}

func TestMarkdownIsDeterministic(t *testing.T) {
	t.Parallel()

	issues := []model.Issue{adoptedIssue(1, "scan:nvim"), adoptedIssue(2, "scan:ci")}
	sum := testSummary(t, issues, nil)
	first, firstErr := Markdown(&sum)
	if firstErr != nil {
		t.Fatalf("Markdown() error = %v", firstErr)
	}
	second, secondErr := Markdown(&sum)
	if secondErr != nil {
		t.Fatalf("Markdown() error = %v", secondErr)
	}
	if first != second {
		t.Error("Markdown() is not stable across calls")
	}
	for _, want := range []string{"## 直近 90 日の成績（フロー）", "<summary>月次トレンド</summary>", "```json"} {
		if !strings.Contains(first, want) {
			t.Errorf("Markdown() is missing %q", want)
		}
	}
}

func TestJSONEmbeddedInMarkdownStaysParseable(t *testing.T) {
	t.Parallel()

	sum := testSummary(t, []model.Issue{adoptedIssue(1, "scan:nvim")}, nil)
	out, renderErr := Markdown(&sum)
	if renderErr != nil {
		t.Fatalf("Markdown() error = %v", renderErr)
	}
	_, block, found := strings.Cut(out, "```json\n")
	if !found {
		t.Fatal("Markdown() has no json block for the meta loop to parse")
	}
	block, _, found = strings.Cut(block, "\n```")
	if !found {
		t.Fatal("json block is not terminated")
	}
	var decoded metrics.Summary
	if decodeErr := json.Unmarshal([]byte(block), &decoded); decodeErr != nil {
		t.Fatalf("embedded json does not round-trip: %v", decodeErr)
	}
	if len(decoded.Scans) != 1 || decoded.Scans[0].Label != "scan:nvim" {
		t.Errorf("decoded scans = %+v, want the scan:nvim row", decoded.Scans)
	}
}
