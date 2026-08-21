// Package metrics turns a normalised dataset into the flow metrics of the
// autonomous improvement pipeline: how many proposals each scan produced, how
// many were adopted, how fast they reached a PR and how many merged.
//
// Everything here is a pure function of (dataset, options). There is no state
// file and no snapshot: every run recomputes the full history, so editing a
// label retroactively changes past numbers on purpose — the labels are the
// source of truth.
package metrics

import (
	"math"
	"slices"
	"strings"
	"time"

	"pipeline-metrics/internal/model"
)

// Defaults for the sampling guards. Rates over a denominator smaller than
// DefaultMinSample are not reported at all, and alerts need an even larger
// denominator: on a personal repository a 2/3 ratio is noise, not a signal.
const (
	DefaultMinSample      = 5
	DefaultAlertMinSample = 8
	DefaultWindowDays     = 90
	livenessDays          = 28
)

// Thresholds are the alert trip points. They start deliberately loose: an alert
// that is always on gets ignored, which is worse than not having it.
type Thresholds struct {
	AdoptedRate         float64 // below this, the scan proposes the wrong things
	RejectedAfterPRRate float64 // above this, too much work is thrown away late
	PRCreatedRate       float64 // below this, adopted issues are not reaching PRs
	PRPending           int     // ... and this many are actually queued up
	MergeRate           float64 // below this, auto PRs are not landing
	UntriagedDays       float64 // above this, triage itself is the bottleneck
}

// DefaultThresholds returns the trip points documented in README.md.
func DefaultThresholds() Thresholds {
	return Thresholds{
		AdoptedRate:         0.6,
		RejectedAfterPRRate: 0.10,
		PRCreatedRate:       0.8,
		PRPending:           3,
		MergeRate:           0.8,
		UntriagedDays:       14,
	}
}

// Options configures a computation. Now is injected so that every output is
// reproducible from a fixture.
type Options struct {
	Now            time.Time
	Since          time.Time
	WindowDays     int
	MinSample      int
	AlertMinSample int
	Thresholds     Thresholds
}

// DefaultOptions returns the options used by the weekly digest.
func DefaultOptions(now, since time.Time) Options {
	return Options{
		Now:            now.UTC(),
		Since:          since.UTC(),
		WindowDays:     DefaultWindowDays,
		MinSample:      DefaultMinSample,
		AlertMinSample: DefaultAlertMinSample,
		Thresholds:     DefaultThresholds(),
	}
}

// WindowStart is the lower bound of the rate window: the later of Since and
// Now-WindowDays. Issues created before it are still counted in the monthly
// cohorts, but never enter a rate denominator.
func (o *Options) WindowStart() time.Time {
	if o.WindowDays <= 0 {
		return o.Since
	}
	rolling := o.Now.AddDate(0, 0, -o.WindowDays)
	if rolling.After(o.Since) {
		return rolling
	}
	return o.Since
}

func (o *Options) normalized() Options {
	out := *o
	out.Now = out.Now.UTC()
	out.Since = out.Since.UTC()
	if out.MinSample < 1 {
		out.MinSample = 1
	}
	if out.AlertMinSample < 1 {
		out.AlertMinSample = 1
	}
	return out
}

// ScanMetrics is one row of the per-scan breakdown; the same shape is reused
// for the "all scans" total row. A nil rate means "sample too small to report".
type ScanMetrics struct {
	Scan string `json:"scan"`

	// Scan quality: what the routine proposed and how it was judged.
	Opened              int      `json:"opened"`
	OpenedLast28d       int      `json:"opened_last_28d"`
	Adopted             int      `json:"adopted"`
	Rejected            int      `json:"rejected"`
	Untriaged           int      `json:"untriaged"`
	UntrackedClose      int      `json:"untracked_close"`
	RejectedAfterPR     int      `json:"rejected_after_pr"`
	AdoptedRate         *float64 `json:"adopted_rate"`
	RejectedAfterPRRate *float64 `json:"rejected_after_pr_rate"`

	// Triage stage.
	OldestUntriagedDays  *float64 `json:"oldest_untriaged_days"`
	OldestUntriagedIssue int      `json:"oldest_untriaged_issue"`
	RejectLatencyP50     *float64 `json:"reject_latency_days_p50"`

	// PR stage.
	AdoptedWithPR int      `json:"adopted_with_pr"`
	PRCreatedRate *float64 `json:"pr_created_rate"`
	PRPending     int      `json:"pr_pending"`
	PRLagP50      *float64 `json:"pr_lag_days_p50"`

	// Merge stage.
	PRs            int      `json:"prs"`
	Merged         int      `json:"merged"`
	ClosedUnmerged int      `json:"closed_unmerged"`
	OpenPRs        int      `json:"open_prs"`
	MergeRate      *float64 `json:"merge_rate"`
	MergeLeadP50   *float64 `json:"merge_lead_days_p50"`
	MergeLeadP90   *float64 `json:"merge_lead_days_p90"`
	E2ELeadP50     *float64 `json:"e2e_lead_days_p50"`
}

// MonthMetrics is one cohort of the monthly trend, keyed by the month an issue
// was opened in. Cohorts cover everything since Options.Since, not just the
// rate window, so the trend keeps its history.
type MonthMetrics struct {
	Month       string   `json:"month"`
	Opened      int      `json:"opened"`
	Adopted     int      `json:"adopted"`
	Rejected    int      `json:"rejected"`
	Untriaged   int      `json:"untriaged"`
	AdoptedRate *float64 `json:"adopted_rate"`
	Merged      int      `json:"merged"`
	E2ELeadP50  *float64 `json:"e2e_lead_days_p50"`
}

// Backlog is the repo-wide queue state. Unlike the counts above it ignores the
// rate window: an item that ages out of the window is exactly the one the
// backlog alert has to keep pointing at.
type Backlog struct {
	Untriaged            int      `json:"untriaged"`
	OldestUntriagedDays  *float64 `json:"oldest_untriaged_days"`
	OldestUntriagedIssue int      `json:"oldest_untriaged_issue"`
	OldestUntriagedScan  string   `json:"oldest_untriaged_scan"`
	PRPending            int      `json:"pr_pending"`
}

// Alert is one threshold breach. It carries numbers only; the wording lives in
// the render package, and OwnerPrompt names the prompt file responsible for the
// stage that broke so the meta loop knows where to look.
type Alert struct {
	Kind        string  `json:"kind"`
	Scope       string  `json:"scope"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Observed    int     `json:"observed"`
	Sample      int     `json:"sample"`
	OwnerPrompt string  `json:"owner_prompt"`
}

// Alert kinds. These names are part of the JSON contract the meta loop reads.
const (
	AlertAdoptedRate         = "adopted_rate"
	AlertRejectedAfterPRRate = "rejected_after_pr_rate"
	AlertPRCreatedRate       = "pr_created_rate"
	AlertMergeRate           = "merge_rate"
	AlertLiveness            = "liveness"
	AlertTriageBacklog       = "triage_backlog"
)

// Anomalies collects issue and PR numbers whose bookkeeping contradicts the
// label conventions the metrics rely on. Every field is non-nil so the JSON
// contract stays stable.
type Anomalies struct {
	DuplicateScanLabels []int    `json:"duplicate_scan_labels"`
	TriagedNonScan      []int    `json:"triaged_non_scan"`
	AdoptedAndRejected  []int    `json:"adopted_and_rejected"`
	UntrackedClose      []int    `json:"untracked_close"`
	RejectedStillOpen   []int    `json:"rejected_still_open"`
	PRLabelWithoutPR    []int    `json:"pr_label_without_pr"`
	PRWithoutLabel      []int    `json:"pr_without_label"`
	MetaBranchPRs       []int    `json:"meta_branch_prs"`
	OrphanPRs           []int    `json:"orphan_prs"`
	ParseWarnings       []string `json:"parse_warnings"`
}

// Report is the whole computation, and the JSON document embedded in the digest.
type Report struct {
	GeneratedAt string `json:"generated_at"`
	Since       string `json:"since"`
	WindowStart string `json:"window_start"`
	WindowDays  int    `json:"window_days"`
	// ExcludedBeforeSince counts scan issues opened before Since. They are
	// reported as a bare count and never enter a rate denominator, because the
	// label conventions the rates rely on were not yet in place back then.
	ExcludedBeforeSince int            `json:"excluded_before_since"`
	MinSample           int            `json:"min_sample"`
	AlertMinSample      int            `json:"alert_min_sample"`
	Scans               []ScanMetrics  `json:"scans"`
	Total               ScanMetrics    `json:"total"`
	Months              []MonthMetrics `json:"months"`
	Backlog             Backlog        `json:"backlog"`
	Alerts              []Alert        `json:"alerts"`
	Anomalies           Anomalies      `json:"anomalies"`
}

// totalScope is the Scan value of the aggregate row.
const totalScope = "all"

// Compute folds the dataset into a Report. It never mutates the dataset and is
// deterministic: same input plus same Options.Now always yields the same bytes.
func Compute(d *model.Dataset, opt *Options) Report {
	o := opt.normalized()
	issues := d.Issues()
	prs := d.PullRequests()

	byNumber := make(map[int]*model.Issue, len(issues))
	for i := range issues {
		byNumber[issues[i].Number()] = &issues[i]
	}

	anomalies := newAnomalies()
	anomalies.ParseWarnings = d.Warnings()

	// PRs indexed by the issue they implement, oldest first.
	prsByIssue := map[int][]*model.PullRequest{}
	for i := range prs {
		pr := &prs[i]
		if !pr.IsAuto() {
			continue
		}
		if pr.IsMeta() {
			anomalies.MetaBranchPRs = append(anomalies.MetaBranchPRs, pr.Number())
			continue
		}
		issue, ok := byNumber[pr.IssueNumber()]
		if !ok || !issue.IsScan() {
			anomalies.OrphanPRs = append(anomalies.OrphanPRs, pr.Number())
			continue
		}
		prsByIssue[pr.IssueNumber()] = append(prsByIssue[pr.IssueNumber()], pr)
	}
	for _, list := range prsByIssue {
		slices.SortFunc(list, func(a, b *model.PullRequest) int {
			return a.CreatedAt().Compare(b.CreatedAt())
		})
	}

	windowStart := o.WindowStart()
	liveSince := o.Now.AddDate(0, 0, -livenessDays)

	acc := map[string]*accumulator{}
	backlog := Backlog{}
	excludedBeforeSince := 0
	var oldestUntriaged *model.Issue

	for i := range issues {
		issue := &issues[i]
		if !issue.IsScan() {
			if issue.Triage() == model.TriageAdopted || issue.Triage() == model.TriageRejected {
				anomalies.TriagedNonScan = append(anomalies.TriagedNonScan, issue.Number())
			}
			continue
		}
		if issue.HasDuplicateScanLabels() {
			anomalies.DuplicateScanLabels = append(anomalies.DuplicateScanLabels, issue.Number())
		}
		if issue.HasBothTriageLabels() {
			anomalies.AdoptedAndRejected = append(anomalies.AdoptedAndRejected, issue.Number())
		}
		if issue.Triage() == model.TriageUntrackedClose {
			anomalies.UntrackedClose = append(anomalies.UntrackedClose, issue.Number())
		}
		if issue.Triage() == model.TriageRejected && !issue.IsClosed() {
			anomalies.RejectedStillOpen = append(anomalies.RejectedStillOpen, issue.Number())
		}
		linked := prsByIssue[issue.Number()]
		if issue.HasPRLabel() && len(linked) == 0 {
			anomalies.PRLabelWithoutPR = append(anomalies.PRLabelWithoutPR, issue.Number())
		}
		if !issue.HasPRLabel() && len(linked) > 0 {
			anomalies.PRWithoutLabel = append(anomalies.PRWithoutLabel, issue.Number())
		}

		// Backlog ignores the window on purpose (see Backlog).
		if issue.Triage() == model.TriageUntriaged {
			backlog.Untriaged++
			if oldestUntriaged == nil || issue.CreatedAt().Before(oldestUntriaged.CreatedAt()) {
				oldestUntriaged = issue
			}
		}
		if issue.Triage() == model.TriageAdopted && !issue.IsClosed() && len(linked) == 0 {
			backlog.PRPending++
		}

		if issue.CreatedAt().Before(o.Since) {
			excludedBeforeSince++
		}
		if issue.CreatedAt().Before(windowStart) || issue.CreatedAt().After(o.Now) {
			continue
		}
		a := ensure(acc, issue.Scan())
		a.addIssue(issue, linked, liveSince)
	}

	if oldestUntriaged != nil {
		days := round1(daysBetween(oldestUntriaged.CreatedAt(), o.Now))
		backlog.OldestUntriagedDays = &days
		backlog.OldestUntriagedIssue = oldestUntriaged.Number()
		backlog.OldestUntriagedScan = oldestUntriaged.Scan()
	}

	// PR-stage counters are keyed off the PR's own creation time so that a PR
	// opened inside the window still counts when its issue predates it.
	for issueNumber, list := range prsByIssue {
		issue := byNumber[issueNumber]
		for _, pr := range list {
			if pr.CreatedAt().Before(windowStart) || pr.CreatedAt().After(o.Now) {
				continue
			}
			ensure(acc, issue.Scan()).addPR(issue, pr)
		}
	}

	anomalies.sort()

	report := Report{
		GeneratedAt:         o.Now.Format(time.RFC3339),
		Since:               o.Since.Format(time.DateOnly),
		WindowStart:         windowStart.Format(time.DateOnly),
		WindowDays:          o.WindowDays,
		ExcludedBeforeSince: excludedBeforeSince,
		MinSample:           o.MinSample,
		AlertMinSample:      o.AlertMinSample,
		Backlog:             backlog,
		Anomalies:           anomalies,
	}

	names := make([]string, 0, len(acc))
	for name := range acc {
		names = append(names, name)
	}
	slices.Sort(names)

	total := newAccumulator()
	for _, name := range names {
		report.Scans = append(report.Scans, acc[name].finish(name, &o))
		total.merge(acc[name])
	}
	if report.Scans == nil {
		report.Scans = []ScanMetrics{}
	}
	report.Total = total.finish(totalScope, &o)
	report.Months = monthlyCohorts(issues, prsByIssue, &o)
	report.Alerts = EvaluateAlerts(&report, &o)
	return report
}

// accumulator collects the raw samples of one scan before they are folded into
// a ScanMetrics row.
type accumulator struct {
	opened          int
	openedLast28d   int
	adopted         int
	rejected        int
	untriaged       int
	untrackedClose  int
	rejectedAfterPR int
	adoptedWithPR   int
	prPending       int

	oldestUntriagedAt    time.Time
	oldestUntriagedIssue int

	rejectLatency []float64
	prLag         []float64

	prs            int
	merged         int
	closedUnmerged int
	openPRs        int
	mergeLead      []float64
	e2eLead        []float64
}

func newAccumulator() *accumulator { return &accumulator{} }

func ensure(acc map[string]*accumulator, name string) *accumulator {
	if a, ok := acc[name]; ok {
		return a
	}
	a := newAccumulator()
	acc[name] = a
	return a
}

func (a *accumulator) addIssue(issue *model.Issue, linked []*model.PullRequest, liveSince time.Time) {
	a.opened++
	if !issue.CreatedAt().Before(liveSince) {
		a.openedLast28d++
	}
	switch issue.Triage() {
	case model.TriageAdopted:
		a.adopted++
		if len(linked) > 0 {
			a.adoptedWithPR++
			a.prLag = append(a.prLag, daysBetween(issue.CreatedAt(), linked[0].CreatedAt()))
		} else if !issue.IsClosed() {
			a.prPending++
		}
	case model.TriageRejected:
		a.rejected++
		if issue.HasPRLabel() || len(linked) > 0 {
			a.rejectedAfterPR++
		}
		if issue.IsClosed() {
			a.rejectLatency = append(a.rejectLatency, daysBetween(issue.CreatedAt(), issue.ClosedAt()))
		}
	case model.TriageUntriaged:
		a.untriaged++
		if a.oldestUntriagedAt.IsZero() || issue.CreatedAt().Before(a.oldestUntriagedAt) {
			a.oldestUntriagedAt = issue.CreatedAt()
			a.oldestUntriagedIssue = issue.Number()
		}
	case model.TriageUntrackedClose:
		a.untrackedClose++
	}
}

func (a *accumulator) addPR(issue *model.Issue, pr *model.PullRequest) {
	a.prs++
	switch {
	case pr.IsMerged():
		a.merged++
		a.mergeLead = append(a.mergeLead, daysBetween(pr.CreatedAt(), pr.MergedAt()))
		a.e2eLead = append(a.e2eLead, daysBetween(issue.CreatedAt(), pr.MergedAt()))
	case pr.IsClosedUnmerged():
		a.closedUnmerged++
	default:
		a.openPRs++
	}
}

func (a *accumulator) merge(other *accumulator) {
	a.opened += other.opened
	a.openedLast28d += other.openedLast28d
	a.adopted += other.adopted
	a.rejected += other.rejected
	a.untriaged += other.untriaged
	a.untrackedClose += other.untrackedClose
	a.rejectedAfterPR += other.rejectedAfterPR
	a.adoptedWithPR += other.adoptedWithPR
	a.prPending += other.prPending
	if !other.oldestUntriagedAt.IsZero() &&
		(a.oldestUntriagedAt.IsZero() || other.oldestUntriagedAt.Before(a.oldestUntriagedAt)) {
		a.oldestUntriagedAt = other.oldestUntriagedAt
		a.oldestUntriagedIssue = other.oldestUntriagedIssue
	}
	a.rejectLatency = append(a.rejectLatency, other.rejectLatency...)
	a.prLag = append(a.prLag, other.prLag...)
	a.prs += other.prs
	a.merged += other.merged
	a.closedUnmerged += other.closedUnmerged
	a.openPRs += other.openPRs
	a.mergeLead = append(a.mergeLead, other.mergeLead...)
	a.e2eLead = append(a.e2eLead, other.e2eLead...)
}

func (a *accumulator) finish(name string, o *Options) ScanMetrics {
	m := ScanMetrics{
		Scan:                 name,
		Opened:               a.opened,
		OpenedLast28d:        a.openedLast28d,
		Adopted:              a.adopted,
		Rejected:             a.rejected,
		Untriaged:            a.untriaged,
		UntrackedClose:       a.untrackedClose,
		RejectedAfterPR:      a.rejectedAfterPR,
		AdoptedRate:          rate(a.adopted, a.opened, o.MinSample),
		RejectedAfterPRRate:  rate(a.rejectedAfterPR, a.opened, o.MinSample),
		OldestUntriagedIssue: a.oldestUntriagedIssue,
		RejectLatencyP50:     quantile(a.rejectLatency, 0.5),
		AdoptedWithPR:        a.adoptedWithPR,
		PRCreatedRate:        rate(a.adoptedWithPR, a.adopted, o.MinSample),
		PRPending:            a.prPending,
		PRLagP50:             quantile(a.prLag, 0.5),
		PRs:                  a.prs,
		Merged:               a.merged,
		ClosedUnmerged:       a.closedUnmerged,
		OpenPRs:              a.openPRs,
		MergeRate:            rate(a.merged, a.merged+a.closedUnmerged, o.MinSample),
		MergeLeadP50:         quantile(a.mergeLead, 0.5),
		MergeLeadP90:         quantile(a.mergeLead, 0.9),
		E2ELeadP50:           quantile(a.e2eLead, 0.5),
	}
	if !a.oldestUntriagedAt.IsZero() {
		days := round1(daysBetween(a.oldestUntriagedAt, o.Now))
		m.OldestUntriagedDays = &days
	}
	return m
}

// monthlyCohorts buckets issues by the month they were opened. The cohort's
// merge numbers follow the issues, not the PRs: "of what was proposed in July,
// this much has shipped".
func monthlyCohorts(issues []model.Issue, prsByIssue map[int][]*model.PullRequest, o *Options) []MonthMetrics {
	type cohort struct {
		opened    int
		adopted   int
		rejected  int
		untriaged int
		merged    int
		e2e       []float64
	}
	buckets := map[string]*cohort{}
	for i := range issues {
		issue := &issues[i]
		if !issue.IsScan() || issue.CreatedAt().Before(o.Since) || issue.CreatedAt().After(o.Now) {
			continue
		}
		key := issue.CreatedAt().Format("2006-01")
		c, ok := buckets[key]
		if !ok {
			c = &cohort{}
			buckets[key] = c
		}
		c.opened++
		switch issue.Triage() {
		case model.TriageAdopted:
			c.adopted++
		case model.TriageRejected:
			c.rejected++
		case model.TriageUntriaged:
			c.untriaged++
		}
		for _, pr := range prsByIssue[issue.Number()] {
			if pr.IsMerged() {
				c.merged++
				c.e2e = append(c.e2e, daysBetween(issue.CreatedAt(), pr.MergedAt()))
			}
		}
	}

	months := make([]string, 0, len(buckets))
	for key := range buckets {
		months = append(months, key)
	}
	slices.Sort(months)

	out := make([]MonthMetrics, 0, len(months))
	for _, key := range months {
		c := buckets[key]
		out = append(out, MonthMetrics{
			Month:       key,
			Opened:      c.opened,
			Adopted:     c.adopted,
			Rejected:    c.rejected,
			Untriaged:   c.untriaged,
			AdoptedRate: rate(c.adopted, c.opened, o.MinSample),
			Merged:      c.merged,
			E2ELeadP50:  quantile(c.e2e, 0.5),
		})
	}
	return out
}

// EvaluateAlerts applies the thresholds to a computed report. Rates are
// recomputed from the raw counts here rather than read off the report, so that
// a large --min-sample cannot silently disable alerting.
func EvaluateAlerts(r *Report, opt *Options) []Alert {
	o := opt.normalized()
	t := o.Thresholds
	var alerts []Alert

	add := func(a Alert) { alerts = append(alerts, a) }

	for i := range r.Scans {
		s := &r.Scans[i]
		owner := OwnerPromptFor(s.Scan)

		// A routine that stopped opening issues raises no backlog anywhere, so
		// silence is the only symptom of a dead scan.
		if s.Opened >= o.AlertMinSample && s.OpenedLast28d == 0 {
			add(Alert{
				Kind: AlertLiveness, Scope: s.Scan,
				Value: 0, Threshold: 1, Observed: 0, Sample: s.Opened,
				OwnerPrompt: owner,
			})
		}
		if s.Opened >= o.AlertMinSample {
			if v := ratio(s.Adopted, s.Opened); v < t.AdoptedRate {
				add(Alert{
					Kind: AlertAdoptedRate, Scope: s.Scan,
					Value: round4(v), Threshold: t.AdoptedRate, Observed: s.Adopted, Sample: s.Opened,
					OwnerPrompt: owner,
				})
			}
			if v := ratio(s.RejectedAfterPR, s.Opened); v > t.RejectedAfterPRRate {
				add(Alert{
					Kind: AlertRejectedAfterPRRate, Scope: s.Scan,
					Value: round4(v), Threshold: t.RejectedAfterPRRate, Observed: s.RejectedAfterPR, Sample: s.Opened,
					OwnerPrompt: owner,
				})
			}
		}
		if s.Adopted >= o.AlertMinSample && s.PRPending >= t.PRPending {
			if v := ratio(s.AdoptedWithPR, s.Adopted); v < t.PRCreatedRate {
				add(Alert{
					Kind: AlertPRCreatedRate, Scope: s.Scan,
					Value: round4(v), Threshold: t.PRCreatedRate, Observed: s.AdoptedWithPR, Sample: s.Adopted,
					OwnerPrompt: PRBotPrompt,
				})
			}
		}
		if decided := s.Merged + s.ClosedUnmerged; decided >= o.AlertMinSample {
			if v := ratio(s.Merged, decided); v < t.MergeRate {
				add(Alert{
					Kind: AlertMergeRate, Scope: s.Scan,
					Value: round4(v), Threshold: t.MergeRate, Observed: s.Merged, Sample: decided,
					OwnerPrompt: PRCarePrompt,
				})
			}
		}
	}

	// Triage is a human step, so its backlog is reported repo-wide and needs no
	// minimum sample: one issue stuck for three weeks is already the problem.
	if d := r.Backlog.OldestUntriagedDays; d != nil && *d > t.UntriagedDays {
		add(Alert{
			Kind: AlertTriageBacklog, Scope: "",
			Value: *d, Threshold: t.UntriagedDays,
			Observed: r.Backlog.OldestUntriagedIssue, Sample: r.Backlog.Untriaged,
			OwnerPrompt: "",
		})
	}

	slices.SortFunc(alerts, func(a, b Alert) int {
		if c := kindOrder(a.Kind) - kindOrder(b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Scope, b.Scope)
	})
	if alerts == nil {
		return []Alert{}
	}
	return alerts
}

// Prompt files that own a pipeline stage, named in alerts so the reader knows
// which file to open. Scans map to the routine that opens their issues.
const (
	PRBotPrompt  = "routines/prompts/weekly-adopted-issue-pr-bot.md"
	PRCarePrompt = "routines/prompts/weekly-pr-care-bot.md"
)

var scanPrompts = map[string]string{
	"scan:nvim":        "routines/prompts/daily-neovim-trend-scan.md",
	"scan:scripts":     "routines/prompts/weekly-scripts-tooling-scan.md",
	"scan:environment": "routines/prompts/weekly-environment-scan.md",
	"scan:ai-agents":   "routines/prompts/weekly-devx-skills-hooks-scan.md",
	"scan:ci":          "routines/prompts/weekly-ci-workflows-scan.md",
}

// OwnerPromptFor returns the prompt file of the routine behind a scan label, or
// "" for a scan this build does not know about (add it to scanPrompts).
func OwnerPromptFor(scan string) string { return scanPrompts[scan] }

func kindOrder(kind string) int {
	order := []string{
		AlertLiveness,
		AlertTriageBacklog,
		AlertAdoptedRate,
		AlertRejectedAfterPRRate,
		AlertPRCreatedRate,
		AlertMergeRate,
	}
	if i := slices.Index(order, kind); i >= 0 {
		return i
	}
	return len(order)
}

func newAnomalies() Anomalies {
	return Anomalies{
		DuplicateScanLabels: []int{},
		TriagedNonScan:      []int{},
		AdoptedAndRejected:  []int{},
		UntrackedClose:      []int{},
		RejectedStillOpen:   []int{},
		PRLabelWithoutPR:    []int{},
		PRWithoutLabel:      []int{},
		MetaBranchPRs:       []int{},
		OrphanPRs:           []int{},
		ParseWarnings:       []string{},
	}
}

func (a *Anomalies) sort() {
	for _, list := range [][]int{
		a.DuplicateScanLabels, a.TriagedNonScan, a.AdoptedAndRejected, a.UntrackedClose,
		a.RejectedStillOpen, a.PRLabelWithoutPR, a.PRWithoutLabel, a.MetaBranchPRs, a.OrphanPRs,
	} {
		slices.Sort(list)
	}
	if a.ParseWarnings == nil {
		a.ParseWarnings = []string{}
	}
}

// rate returns num/den rounded to 4 decimals, or nil when the denominator is
// too small for the ratio to mean anything.
func rate(num, den, minSample int) *float64 {
	if den <= 0 || den < minSample {
		return nil
	}
	v := round4(float64(num) / float64(den))
	return &v
}

func ratio(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// quantile returns the p-quantile in days, rounded to one decimal, using linear
// interpolation between order statistics. For p=0.5 over an even sample that is
// the mean of the two middle values, i.e. the usual median.
func quantile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	if len(sorted) == 1 {
		v := round1(sorted[0])
		return &v
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		v := round1(sorted[lo])
		return &v
	}
	v := round1(sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo)))
	return &v
}

func daysBetween(from, to time.Time) float64 {
	return to.Sub(from).Hours() / 24
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
