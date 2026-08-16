package metrics

import (
	"encoding/json"
	"testing"
	"time"

	"pipeline-metrics/internal/model"
)

type issueSpec struct {
	Number    int      `json:"number"`
	Labels    []string `json:"-"`
	CreatedAt string   `json:"createdAt"`
	ClosedAt  string   `json:"closedAt,omitempty"`
}

type prSpec struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	CreatedAt   string `json:"createdAt"`
	ClosedAt    string `json:"closedAt,omitempty"`
	MergedAt    string `json:"mergedAt,omitempty"`
}

// newDataset renders the specs into the JSON shape gh emits and parses it back,
// so the tests exercise the same normalisation the CLI does.
func newDataset(t *testing.T, issues []issueSpec, prs []prSpec) model.Dataset {
	t.Helper()
	type label struct {
		Name string `json:"name"`
	}
	type rawIssue struct {
		issueSpec
		Labels []label `json:"labels"`
	}
	raw := make([]rawIssue, 0, len(issues))
	for _, spec := range issues {
		labels := make([]label, 0, len(spec.Labels))
		for _, name := range spec.Labels {
			labels = append(labels, label{Name: name})
		}
		raw = append(raw, rawIssue{issueSpec: spec, Labels: labels})
	}
	issuesJSON, issuesErr := json.Marshal(raw)
	if issuesErr != nil {
		t.Fatalf("marshal issues: %v", issuesErr)
	}
	prsJSON, prsErr := json.Marshal(prs)
	if prsErr != nil {
		t.Fatalf("marshal prs: %v", prsErr)
	}
	dataset, parseErr := model.Parse(issuesJSON, prsJSON)
	if parseErr != nil {
		t.Fatalf("parse dataset: %v", parseErr)
	}
	return dataset
}

func testOptions(t *testing.T, now, since string) Options {
	t.Helper()
	nowTime, nowErr := time.Parse(time.RFC3339, now)
	if nowErr != nil {
		t.Fatalf("parse now: %v", nowErr)
	}
	sinceTime, sinceErr := time.Parse(time.DateOnly, since)
	if sinceErr != nil {
		t.Fatalf("parse since: %v", sinceErr)
	}
	return DefaultOptions(nowTime, sinceTime)
}

func scanRow(t *testing.T, r *Report, name string) *ScanMetrics {
	t.Helper()
	for i := range r.Scans {
		if r.Scans[i].Scan == name {
			return &r.Scans[i]
		}
	}
	t.Fatalf("no row for %q", name)
	return nil
}

func wantRate(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("rate is nil, want %v", want)
	}
	if *got != want {
		t.Errorf("rate = %v, want %v", *got, want)
	}
}

const (
	testNow   = "2026-08-16T00:00:00Z"
	testSince = "2026-06-28"
)

func TestComputeEmptyDataset(t *testing.T) {
	t.Parallel()
	dataset := newDataset(t, nil, nil)
	opt := testOptions(t, testNow, testSince)
	report := Compute(&dataset, &opt)

	if len(report.Scans) != 0 {
		t.Errorf("scans = %d, want 0", len(report.Scans))
	}
	if report.Total.Opened != 0 {
		t.Errorf("total opened = %d, want 0", report.Total.Opened)
	}
	if report.Total.AdoptedRate != nil {
		t.Errorf("adopted rate = %v, want nil for an empty sample", *report.Total.AdoptedRate)
	}
	if len(report.Alerts) != 0 {
		t.Errorf("alerts = %d, want none", len(report.Alerts))
	}
	if report.Backlog.OldestUntriagedDays != nil {
		t.Error("backlog reports an age with nothing untriaged")
	}
	// Empty slices, not nulls: the meta loop parses this document.
	if report.Scans == nil || report.Months == nil || report.Alerts == nil ||
		report.Anomalies.UntrackedClose == nil {
		t.Error("empty collections must serialise as [] rather than null")
	}
}

func TestComputeTriageCounts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		issues          []issueSpec
		wantOpened      int
		wantAdopted     int
		wantRejected    int
		wantUntriaged   int
		wantUntracked   int
		wantRejectedPR  int
		wantAdoptedRate *float64
	}{
		{
			name: "every issue still untriaged",
			issues: []issueSpec{
				{Number: 1, Labels: []string{"scan:a"}, CreatedAt: "2026-07-01T00:00:00Z"},
				{Number: 2, Labels: []string{"scan:a"}, CreatedAt: "2026-07-02T00:00:00Z"},
				{Number: 3, Labels: []string{"scan:a"}, CreatedAt: "2026-07-03T00:00:00Z"},
				{Number: 4, Labels: []string{"scan:a"}, CreatedAt: "2026-07-04T00:00:00Z"},
				{Number: 5, Labels: []string{"scan:a"}, CreatedAt: "2026-07-05T00:00:00Z"},
			},
			wantOpened:      5,
			wantUntriaged:   5,
			wantAdoptedRate: new(0.0),
		},
		{
			name: "single issue leaves rates unreported",
			issues: []issueSpec{
				{Number: 1, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-07-01T00:00:00Z"},
			},
			wantOpened:  1,
			wantAdopted: 1,
		},
		{
			name: "adopted then rejected counts once, as rejected after a PR",
			issues: []issueSpec{
				{Number: 1, Labels: []string{"scan:a", "adopted", "rejected", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-05T00:00:00Z"},
			},
			wantOpened:     1,
			wantRejected:   1,
			wantRejectedPR: 1,
		},
		{
			name: "closed without a triage label",
			issues: []issueSpec{
				{Number: 1, Labels: []string{"scan:a"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-02T00:00:00Z"},
			},
			wantOpened:    1,
			wantUntracked: 1,
		},
		{
			name: "issues without a scan label are out of scope",
			issues: []issueSpec{
				{Number: 1, Labels: []string{"enhancement", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-02T00:00:00Z"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dataset := newDataset(t, tt.issues, nil)
			opt := testOptions(t, testNow, testSince)
			report := Compute(&dataset, &opt)

			total := report.Total
			if total.Opened != tt.wantOpened {
				t.Errorf("opened = %d, want %d", total.Opened, tt.wantOpened)
			}
			if total.Adopted != tt.wantAdopted {
				t.Errorf("adopted = %d, want %d", total.Adopted, tt.wantAdopted)
			}
			if total.Rejected != tt.wantRejected {
				t.Errorf("rejected = %d, want %d", total.Rejected, tt.wantRejected)
			}
			if total.Untriaged != tt.wantUntriaged {
				t.Errorf("untriaged = %d, want %d", total.Untriaged, tt.wantUntriaged)
			}
			if total.UntrackedClose != tt.wantUntracked {
				t.Errorf("untracked_close = %d, want %d", total.UntrackedClose, tt.wantUntracked)
			}
			if total.RejectedAfterPR != tt.wantRejectedPR {
				t.Errorf("rejected_after_pr = %d, want %d", total.RejectedAfterPR, tt.wantRejectedPR)
			}
			switch {
			case tt.wantAdoptedRate == nil && total.AdoptedRate != nil:
				t.Errorf("adopted_rate = %v, want nil", *total.AdoptedRate)
			case tt.wantAdoptedRate != nil:
				wantRate(t, total.AdoptedRate, *tt.wantAdoptedRate)
			}
		})
	}
}

// TestComputeWindow covers the two-sided boundary: issues before --since are
// counted but excluded from every rate, and the rolling window trims the front
// once it is narrower than --since.
func TestComputeWindow(t *testing.T) {
	t.Parallel()
	issues := []issueSpec{
		{Number: 1, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-06-20T00:00:00Z"}, // before since
		{Number: 2, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-06-27T23:59:59Z"}, // one second early
		{Number: 3, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-06-28T00:00:00Z"}, // exactly at since
		{Number: 4, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-08-10T00:00:00Z", ClosedAt: "2026-08-11T00:00:00Z"},
	}
	dataset := newDataset(t, issues, nil)

	opt := testOptions(t, testNow, testSince)
	report := Compute(&dataset, &opt)
	if report.Total.Opened != 2 {
		t.Errorf("opened = %d, want 2 (issues before --since must not count)", report.Total.Opened)
	}
	if report.ExcludedBeforeSince != 2 {
		t.Errorf("excluded_before_since = %d, want 2", report.ExcludedBeforeSince)
	}
	if report.WindowStart != testSince {
		t.Errorf("window_start = %s, want %s", report.WindowStart, testSince)
	}
	// The monthly cohorts start at --since too, so June keeps only issue #3.
	if len(report.Months) != 2 || report.Months[0].Month != "2026-06" || report.Months[0].Opened != 1 {
		t.Errorf("months = %+v, want June with 1 issue then August", report.Months)
	}

	// A window narrower than --since wins: only the August issue survives.
	narrow := testOptions(t, testNow, testSince)
	narrow.WindowDays = 30
	narrowed := Compute(&dataset, &narrow)
	if narrowed.Total.Opened != 1 {
		t.Errorf("opened = %d, want 1 inside a 30 day window", narrowed.Total.Opened)
	}
	if narrowed.WindowStart != "2026-07-17" {
		t.Errorf("window_start = %s, want 2026-07-17", narrowed.WindowStart)
	}

	// Disabling the window falls back to --since alone.
	unbounded := testOptions(t, testNow, testSince)
	unbounded.WindowDays = 0
	if got := Compute(&dataset, &unbounded).WindowStart; got != testSince {
		t.Errorf("window_start = %s, want %s with the window disabled", got, testSince)
	}
}

// TestComputeLatencies pins the quantile behaviour, including the even-sized
// median and the interpolated p90.
func TestComputeLatencies(t *testing.T) {
	t.Parallel()
	issues := []issueSpec{
		// Four rejections lasting 1, 2, 3 and 4 days: an even sample whose
		// median falls between the two middle values.
		{Number: 1, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-02T00:00:00Z"},
		{Number: 2, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-03T00:00:00Z"},
		{Number: 3, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-04T00:00:00Z"},
		{Number: 4, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z", ClosedAt: "2026-07-05T00:00:00Z"},
		// A rejection that was never closed contributes no latency at all.
		{Number: 5, Labels: []string{"scan:a", "rejected"}, CreatedAt: "2026-07-01T00:00:00Z"},
		// Merge leads of 1, 2, 3, 4 and 10 days.
		{Number: 11, Labels: []string{"scan:b", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{Number: 12, Labels: []string{"scan:b", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{Number: 13, Labels: []string{"scan:b", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{Number: 14, Labels: []string{"scan:b", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{Number: 15, Labels: []string{"scan:b", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
	}
	prs := []prSpec{
		{Number: 111, HeadRefName: "auto/issue-11-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-03T00:00:00Z"},
		{Number: 112, HeadRefName: "auto/issue-12-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-04T00:00:00Z"},
		{Number: 113, HeadRefName: "auto/issue-13-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-05T00:00:00Z"},
		{Number: 114, HeadRefName: "auto/issue-14-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-06T00:00:00Z"},
		{Number: 115, HeadRefName: "auto/issue-15-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-12T00:00:00Z"},
	}
	dataset := newDataset(t, issues, prs)
	opt := testOptions(t, testNow, testSince)
	report := Compute(&dataset, &opt)

	a := scanRow(t, &report, "scan:a")
	if a.RejectLatencyP50 == nil || *a.RejectLatencyP50 != 2.5 {
		t.Errorf("reject latency p50 = %v, want 2.5 (mean of the two middle values)", a.RejectLatencyP50)
	}

	b := scanRow(t, &report, "scan:b")
	if b.MergeLeadP50 == nil || *b.MergeLeadP50 != 3 {
		t.Errorf("merge lead p50 = %v, want 3", b.MergeLeadP50)
	}
	// p90 sits at index 0.9*(5-1) = 3.6, i.e. 60% of the way from 4 to 10.
	if b.MergeLeadP90 == nil || *b.MergeLeadP90 != 7.6 {
		t.Errorf("merge lead p90 = %v, want 7.6", b.MergeLeadP90)
	}
	if b.PRLagP50 == nil || *b.PRLagP50 != 1 {
		t.Errorf("pr lag p50 = %v, want 1", b.PRLagP50)
	}
	if b.E2ELeadP50 == nil || *b.E2ELeadP50 != 4 {
		t.Errorf("e2e lead p50 = %v, want 4", b.E2ELeadP50)
	}
}

func TestComputePRJoin(t *testing.T) {
	t.Parallel()
	issues := []issueSpec{
		{Number: 1, Labels: []string{"scan:a", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{Number: 2, Labels: []string{"scan:a", "adopted", "pr-created"}, CreatedAt: "2026-07-01T00:00:00Z"}, // label lies: no PR
		{Number: 3, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-07-01T00:00:00Z"},               // PR without the label
	}
	prs := []prSpec{
		{Number: 101, HeadRefName: "auto/issue-1-a", CreatedAt: "2026-07-02T00:00:00Z", MergedAt: "2026-07-03T00:00:00Z"},
		{Number: 103, HeadRefName: "auto/issue-3-a", CreatedAt: "2026-07-02T00:00:00Z", ClosedAt: "2026-07-03T00:00:00Z"},
		{Number: 104, HeadRefName: "auto/routine-improve-20260801", CreatedAt: "2026-08-01T00:00:00Z", MergedAt: "2026-08-01T12:00:00Z"},
		{Number: 105, HeadRefName: "auto/issue-999-ghost", CreatedAt: "2026-08-01T00:00:00Z"},
		{Number: 106, HeadRefName: "feature/by-hand", CreatedAt: "2026-08-01T00:00:00Z", MergedAt: "2026-08-02T00:00:00Z"},
	}
	dataset := newDataset(t, issues, prs)
	opt := testOptions(t, testNow, testSince)
	report := Compute(&dataset, &opt)

	a := scanRow(t, &report, "scan:a")
	if a.PRs != 2 || a.Merged != 1 || a.ClosedUnmerged != 1 {
		t.Errorf("prs/merged/closed = %d/%d/%d, want 2/1/1", a.PRs, a.Merged, a.ClosedUnmerged)
	}
	if a.AdoptedWithPR != 2 {
		t.Errorf("adopted_with_pr = %d, want 2 (the join decides, not the label)", a.AdoptedWithPR)
	}
	if a.PRPending != 1 {
		t.Errorf("pr_pending = %d, want 1", a.PRPending)
	}
	if got := report.Anomalies.MetaBranchPRs; len(got) != 1 || got[0] != 104 {
		t.Errorf("meta branch prs = %v, want [104]", got)
	}
	if got := report.Anomalies.OrphanPRs; len(got) != 1 || got[0] != 105 {
		t.Errorf("orphan prs = %v, want [105] (hand written branches are not anomalies)", got)
	}
	if got := report.Anomalies.PRLabelWithoutPR; len(got) != 1 || got[0] != 2 {
		t.Errorf("pr label without pr = %v, want [2]", got)
	}
	if got := report.Anomalies.PRWithoutLabel; len(got) != 1 || got[0] != 3 {
		t.Errorf("pr without label = %v, want [3]", got)
	}
}

func TestComputeBacklogIgnoresTheWindow(t *testing.T) {
	t.Parallel()
	issues := []issueSpec{
		// Untriaged for months: aged out of the rate window, still the backlog.
		{Number: 1, Labels: []string{"scan:a"}, CreatedAt: "2026-01-05T00:00:00Z"},
		{Number: 2, Labels: []string{"scan:a", "adopted"}, CreatedAt: "2026-08-01T00:00:00Z"},
	}
	dataset := newDataset(t, issues, nil)
	opt := testOptions(t, testNow, testSince)
	report := Compute(&dataset, &opt)

	if report.Total.Untriaged != 0 {
		t.Errorf("windowed untriaged = %d, want 0", report.Total.Untriaged)
	}
	if report.Backlog.Untriaged != 1 || report.Backlog.OldestUntriagedIssue != 1 {
		t.Errorf("backlog = %+v, want issue #1 pending", report.Backlog)
	}
	if report.Backlog.PRPending != 1 {
		t.Errorf("pr_pending = %d, want 1", report.Backlog.PRPending)
	}
}

func TestEvaluateAlerts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		scan      ScanMetrics
		backlog   Backlog
		wantKinds []string
	}{
		{
			name: "healthy scan is quiet",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 12, OpenedLast28d: 4, Adopted: 11,
				AdoptedWithPR: 11, Merged: 10, ClosedUnmerged: 1,
			},
		},
		{
			name: "low adoption",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 5,
			},
			wantKinds: []string{AlertAdoptedRate},
		},
		{
			name: "adoption exactly at the threshold stays quiet",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 6,
			},
		},
		{
			name: "small sample never fires",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 7, OpenedLast28d: 0, Adopted: 0,
			},
		},
		{
			name: "silent routine",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 8, OpenedLast28d: 0, Adopted: 8, AdoptedWithPR: 8,
			},
			wantKinds: []string{AlertLiveness},
		},
		{
			name: "rejections after a PR",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 8, AdoptedWithPR: 8, RejectedAfterPR: 2,
			},
			wantKinds: []string{AlertRejectedAfterPRRate},
		},
		{
			name: "rejections after a PR exactly at the threshold stay quiet",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 9, AdoptedWithPR: 9, RejectedAfterPR: 1,
			},
		},
		{
			name: "adopted issues not reaching PRs",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 10, AdoptedWithPR: 7, PRPending: 3,
			},
			wantKinds: []string{AlertPRCreatedRate},
		},
		{
			name: "low PR rate without a queue stays quiet",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 10, AdoptedWithPR: 7, PRPending: 2,
			},
		},
		{
			name: "PRs not landing",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 12, OpenedLast28d: 4, Adopted: 12, AdoptedWithPR: 12,
				Merged: 6, ClosedUnmerged: 3,
			},
			wantKinds: []string{AlertMergeRate},
		},
		{
			name: "stale triage backlog",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 10, AdoptedWithPR: 10,
			},
			backlog:   Backlog{Untriaged: 2, OldestUntriagedDays: new(21.4), OldestUntriagedIssue: 512},
			wantKinds: []string{AlertTriageBacklog},
		},
		{
			name: "backlog exactly at the threshold stays quiet",
			scan: ScanMetrics{
				Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 10, AdoptedWithPR: 10,
			},
			backlog: Backlog{Untriaged: 1, OldestUntriagedDays: new(14.0), OldestUntriagedIssue: 512},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := Report{Scans: []ScanMetrics{tt.scan}, Backlog: tt.backlog}
			opt := testOptions(t, testNow, testSince)
			alerts := EvaluateAlerts(&report, &opt)

			if len(alerts) != len(tt.wantKinds) {
				t.Fatalf("alerts = %+v, want %v", alerts, tt.wantKinds)
			}
			for i, want := range tt.wantKinds {
				if alerts[i].Kind != want {
					t.Errorf("alert[%d].Kind = %q, want %q", i, alerts[i].Kind, want)
				}
			}
		})
	}
}

// TestEvaluateAlertsNamesTheResponsiblePrompt keeps the warning actionable: the
// reader must be told which file to open.
func TestEvaluateAlertsNamesTheResponsiblePrompt(t *testing.T) {
	t.Parallel()
	report := Report{Scans: []ScanMetrics{
		{Scan: "scan:nvim", Opened: 10, OpenedLast28d: 4, Adopted: 3},
		{Scan: "scan:unknown", Opened: 10, OpenedLast28d: 4, Adopted: 3},
	}}
	opt := testOptions(t, testNow, testSince)
	alerts := EvaluateAlerts(&report, &opt)
	if len(alerts) != 2 {
		t.Fatalf("alerts = %d, want 2", len(alerts))
	}
	if alerts[0].OwnerPrompt != "routines/prompts/daily-neovim-trend-scan.md" {
		t.Errorf("owner prompt = %q, want the nvim scan prompt", alerts[0].OwnerPrompt)
	}
	if alerts[1].OwnerPrompt != "" {
		t.Errorf("owner prompt = %q, want empty for an unmapped scan", alerts[1].OwnerPrompt)
	}
}

// TestEvaluateAlertsIgnoresMinSampleForRates makes sure a generous --min-sample
// suppresses table cells only, never the alerting itself.
func TestEvaluateAlertsIgnoresMinSampleForRates(t *testing.T) {
	t.Parallel()
	report := Report{Scans: []ScanMetrics{{Scan: "scan:a", Opened: 10, OpenedLast28d: 4, Adopted: 2}}}
	opt := testOptions(t, testNow, testSince)
	opt.MinSample = 100
	if alerts := EvaluateAlerts(&report, &opt); len(alerts) != 1 || alerts[0].Kind != AlertAdoptedRate {
		t.Errorf("alerts = %+v, want one adopted_rate alert", alerts)
	}
}
