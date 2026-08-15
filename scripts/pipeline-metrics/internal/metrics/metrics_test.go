package metrics

import (
	"testing"
	"time"

	"pipeline-metrics/internal/model"
)

var (
	testNow   = ts("2026-08-15T00:00:00Z")
	testSince = ts("2026-06-28T00:00:00Z")
)

func ts(s string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, s)
	return parsed
}

func testOptions() Options {
	return Options{Now: testNow, Since: testSince, Window: DefaultWindow, Months: DefaultMonths, MinSample: DefaultMinSample}
}

// scanIssue builds a scan issue with the triage state already decided, which is
// what the model layer hands the metrics layer.
func scanIssue(number int, label string, triage model.TriageState, created string, opts ...func(*model.Issue)) model.Issue {
	iss := model.Issue{
		Number:         number,
		ScanLabel:      label,
		ScanLabelCount: 1,
		Triage:         triage,
		CreatedAt:      ts(created),
	}
	if triage == model.Rejected || triage == model.UntrackedClose {
		iss.Closed = true
		iss.ClosedAt = iss.CreatedAt.Add(48 * time.Hour)
	}
	for _, opt := range opts {
		opt(&iss)
	}
	return iss
}

func withPR(iss *model.Issue) { iss.PRCreated = true }

func autoPR(number, issueNumber int, branch, created, merged string) model.PullRequest {
	pr := model.PullRequest{
		Number:      number,
		Branch:      branch,
		IssueNumber: issueNumber,
		CreatedAt:   ts(created),
	}
	if merged == "" {
		pr.ClosedAt = pr.CreatedAt.Add(24 * time.Hour)
		return pr
	}
	pr.MergedAt = ts(merged)
	pr.ClosedAt = pr.MergedAt
	pr.Merged = true
	return pr
}

func TestNewRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		num       int
		denom     int
		minSample int
		wantRate  *float64
	}{
		{name: "empty denominator withholds the rate", num: 0, denom: 0, minSample: 5},
		{name: "below the sample guard withholds the rate", num: 3, denom: 4, minSample: 5},
		{name: "at the sample guard reports the rate", num: 3, denom: 5, minSample: 5, wantRate: ptr(0.6)},
		{name: "rounds to three digits", num: 1, denom: 3, minSample: 1, wantRate: ptr(0.333)},
		{name: "a guard of one still needs a denominator", num: 0, denom: 0, minSample: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := newRatio(tt.num, tt.denom, tt.minSample)
			switch {
			case tt.wantRate == nil && got.Rate != nil:
				t.Fatalf("Rate = %v, want nil", *got.Rate)
			case tt.wantRate != nil && got.Rate == nil:
				t.Fatalf("Rate = nil, want %v", *tt.wantRate)
			case tt.wantRate != nil && *got.Rate != *tt.wantRate:
				t.Fatalf("Rate = %v, want %v", *got.Rate, *tt.wantRate)
			}
			if got.Num != tt.num || got.Denom != tt.denom {
				t.Errorf("Num/Denom = %d/%d, want %d/%d", got.Num, got.Denom, tt.num, tt.denom)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []float64
		p      float64
		want   *float64
	}{
		{name: "empty input has no percentile", values: nil, p: percentileP50},
		{name: "single value", values: []float64{3.2}, p: percentileP50, want: ptr(3.2)},
		{name: "odd length takes the middle", values: []float64{5, 1, 3}, p: percentileP50, want: ptr(3.0)},
		{name: "even length averages the middle pair", values: []float64{4, 1, 3, 2}, p: percentileP50, want: ptr(2.5)},
		{name: "p90 interpolates near the top", values: []float64{1, 2, 3, 4, 5}, p: percentileP90, want: ptr(4.6)},
		{name: "input order does not matter", values: []float64{9, 0}, p: percentileP50, want: ptr(4.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := percentile(tt.values, tt.p)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("percentile() = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("percentile() = nil, want %v", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("percentile() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestPercentileDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	values := []float64{3, 1, 2}
	percentile(values, percentileP50)
	if values[0] != 3 {
		t.Errorf("input was reordered: %v", values)
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		issues []model.Issue
		prs    []model.PullRequest
		check  func(t *testing.T, sum *Summary)
	}{
		{
			name: "no input yields an empty but well-formed summary",
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if len(sum.Scans) != 0 || len(sum.Alerts) != 0 {
					t.Errorf("Scans/Alerts = %d/%d, want 0/0", len(sum.Scans), len(sum.Alerts))
				}
				if sum.Overall.AdoptedRate.Rate != nil {
					t.Errorf("AdoptedRate.Rate = %v, want nil", *sum.Overall.AdoptedRate.Rate)
				}
			},
		},
		{
			name: "issues created before the policy start stay out of the denominators",
			issues: []model.Issue{
				scanIssue(1, "scan:nvim", model.Adopted, "2026-05-01T00:00:00Z"),
				scanIssue(2, "scan:nvim", model.Adopted, "2026-08-01T00:00:00Z"),
				scanIssue(3, "scan:nvim", model.Rejected, "2026-08-02T00:00:00Z"),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if sum.Overall.PrePolicyIssues != 1 {
					t.Errorf("PrePolicyIssues = %d, want 1", sum.Overall.PrePolicyIssues)
				}
				if got := sum.Scans[0].Sample; got != 2 {
					t.Errorf("Sample = %d, want 2", got)
				}
			},
		},
		{
			name: "all untriaged reports the backlog and withholds every rate",
			issues: []model.Issue{
				scanIssue(1, "scan:ci", model.Untriaged, "2026-08-01T00:00:00Z"),
				scanIssue(2, "scan:ci", model.Untriaged, "2026-07-01T00:00:00Z"),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				st := sum.Scans[0]
				if st.Untriaged != 2 {
					t.Errorf("Untriaged = %d, want 2", st.Untriaged)
				}
				if st.OldestUntriagedDays != 45 {
					t.Errorf("OldestUntriagedDays = %d, want 45", st.OldestUntriagedDays)
				}
				if st.AdoptedRate.Rate != nil {
					t.Errorf("AdoptedRate.Rate = %v, want nil", *st.AdoptedRate.Rate)
				}
			},
		},
		{
			name: "an issue rejected after its PR is the expensive failure",
			issues: []model.Issue{
				scanIssue(1, "scan:scripts", model.Rejected, "2026-08-01T00:00:00Z", withPR),
				scanIssue(2, "scan:scripts", model.Adopted, "2026-08-02T00:00:00Z", withPR),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				got := sum.Scans[0].RejectedAfterPRRate
				if got.Num != 1 || got.Denom != 2 {
					t.Errorf("RejectedAfterPRRate = %d/%d, want 1/2", got.Num, got.Denom)
				}
			},
		},
		{
			name: "a PR whose branch names no known issue counts as a meta branch",
			issues: []model.Issue{
				scanIssue(1, "scan:nvim", model.Adopted, "2026-08-01T00:00:00Z", withPR),
			},
			prs: []model.PullRequest{
				autoPR(90, 1, "auto/issue-1-ship", "2026-08-02T00:00:00Z", "2026-08-03T00:00:00Z"),
				autoPR(91, 0, "auto/routine-improve-20260802", "2026-08-02T00:00:00Z", "2026-08-02T00:00:00Z"),
				autoPR(92, 999, "auto/issue-999-orphan", "2026-08-02T00:00:00Z", "2026-08-02T00:00:00Z"),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if sum.Overall.MetaBranchPRs != 2 {
					t.Errorf("MetaBranchPRs = %d, want 2", sum.Overall.MetaBranchPRs)
				}
				if sum.Overall.MergedPRs != 3 {
					t.Errorf("MergedPRs = %d, want 3", sum.Overall.MergedPRs)
				}
				if got := sum.Scans[0].E2ELeadDaysP50; got == nil || *got != 2 {
					t.Errorf("E2ELeadDaysP50 = %v, want 2", got)
				}
			},
		},
		{
			name: "an unmerged close is listed for the qualitative follow-up",
			issues: []model.Issue{
				scanIssue(1, "scan:nvim", model.Adopted, "2026-08-01T00:00:00Z", withPR),
			},
			prs: []model.PullRequest{
				autoPR(93, 1, "auto/issue-1-ship", "2026-08-02T00:00:00Z", ""),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if len(sum.Overall.UnmergedPRNumbers) != 1 || sum.Overall.UnmergedPRNumbers[0] != 93 {
					t.Errorf("UnmergedPRNumbers = %v, want [93]", sum.Overall.UnmergedPRNumbers)
				}
			},
		},
		{
			name: "label convention breaches are reported rather than silently dropped",
			issues: []model.Issue{
				{Number: 601, Triage: model.Adopted, CreatedAt: ts("2026-08-01T00:00:00Z")},
				{Number: 602, ScanLabel: "scan:ci", ScanLabelCount: 2, Triage: model.Adopted, CreatedAt: ts("2026-08-01T00:00:00Z")},
				scanIssue(603, "scan:ci", model.UntrackedClose, "2026-08-01T00:00:00Z"),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if len(sum.Anomalies.TriagedNonScan) != 1 || sum.Anomalies.TriagedNonScan[0] != 601 {
					t.Errorf("TriagedNonScan = %v, want [601]", sum.Anomalies.TriagedNonScan)
				}
				if len(sum.Anomalies.MultiScanLabel) != 1 || sum.Anomalies.MultiScanLabel[0] != 602 {
					t.Errorf("MultiScanLabel = %v, want [602]", sum.Anomalies.MultiScanLabel)
				}
				if sum.Overall.UntrackedClose != 1 {
					t.Errorf("UntrackedClose = %d, want 1", sum.Overall.UntrackedClose)
				}
			},
		},
		{
			name: "a scan that went quiet still appears so its silence is visible",
			issues: []model.Issue{
				scanIssue(1, "scan:environment", model.Adopted, "2026-06-30T00:00:00Z"),
			},
			check: func(t *testing.T, sum *Summary) {
				t.Helper()
				if len(sum.Scans) != 1 || sum.Scans[0].OpenedLast28d != 0 {
					t.Fatalf("Scans = %+v, want one scan with OpenedLast28d 0", sum.Scans)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sum := Summarize(tt.issues, tt.prs, testOptions())
			tt.check(t, &sum)
		})
	}
}

func TestEvaluateAlerts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		issues     []model.Issue
		wantMetric string
		wantScope  string
		wantNone   bool
	}{
		{
			name:       "a low adoption rate names the scan's prompt",
			issues:     repeatIssues("scan:scripts", 3, 5, "2026-08-01T00:00:00Z"),
			wantMetric: "adopted_rate",
			wantScope:  "scan:scripts",
		},
		{
			name:     "the same rate below the alert sample guard stays quiet",
			issues:   repeatIssues("scan:scripts", 2, 3, "2026-08-01T00:00:00Z"),
			wantNone: true,
		},
		{
			name:       "a scan silent for 28 days is flagged without any sample guard",
			issues:     []model.Issue{scanIssue(1, "scan:environment", model.Adopted, "2026-06-30T00:00:00Z")},
			wantMetric: "liveness",
			wantScope:  "scan:environment",
		},
		{
			name:       "an issue stuck at the human gate is flagged",
			issues:     []model.Issue{scanIssue(1, "scan:ci", model.Untriaged, "2026-07-01T00:00:00Z")},
			wantMetric: "triage_backlog",
			wantScope:  scopeOverall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sum := Summarize(tt.issues, nil, testOptions())
			if tt.wantNone {
				for _, alert := range sum.Alerts {
					if alert.Metric == "adopted_rate" {
						t.Fatalf("adopted_rate alert fired below the sample guard: %+v", alert)
					}
				}
				return
			}
			found := false
			for _, alert := range sum.Alerts {
				if alert.Metric == tt.wantMetric && alert.Scope == tt.wantScope {
					found = true
					if tt.wantScope != scopeOverall && len(alert.Prompts) == 0 {
						t.Errorf("alert %s/%s carries no prompt path", alert.Metric, alert.Scope)
					}
				}
			}
			if !found {
				t.Fatalf("no %s alert for %s; got %+v", tt.wantMetric, tt.wantScope, sum.Alerts)
			}
		})
	}
}

func TestSummarizeIsDeterministic(t *testing.T) {
	t.Parallel()

	issues := append(repeatIssues("scan:nvim", 4, 2, "2026-08-01T00:00:00Z"), repeatIssues("scan:ci", 3, 1, "2026-08-02T00:00:00Z")...)
	first := Summarize(issues, nil, testOptions())
	second := Summarize(issues, nil, testOptions())
	if len(first.Scans) != len(second.Scans) {
		t.Fatalf("scan count differs between runs: %d vs %d", len(first.Scans), len(second.Scans))
	}
	for i := range first.Scans {
		if first.Scans[i].Label != second.Scans[i].Label {
			t.Fatalf("scan order differs at %d: %s vs %s", i, first.Scans[i].Label, second.Scans[i].Label)
		}
	}
}

// repeatIssues builds adopted and rejected issues for one scan label, which is
// how the rate thresholds get their denominators.
func repeatIssues(label string, adopted, rejected int, created string) []model.Issue {
	out := make([]model.Issue, 0, adopted+rejected)
	number := 1
	for range adopted {
		out = append(out, scanIssue(number, label, model.Adopted, created, withPR))
		number++
	}
	for range rejected {
		out = append(out, scanIssue(number, label, model.Rejected, created))
		number++
	}
	return out
}

func ptr(v float64) *float64 { return &v }
