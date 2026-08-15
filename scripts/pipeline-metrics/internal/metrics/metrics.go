// Package metrics folds normalised issues and pull requests into the pipeline's
// effectiveness figures and evaluates the alert thresholds.
//
// Every function here is pure: identical input plus an injected clock always
// yields identical, deterministically ordered output. There is no state file —
// GitHub is the source of truth and the whole history is recomputed on each run,
// so month cohorts stay stable without anything being persisted.
package metrics

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"pipeline-metrics/internal/model"
)

// Defaults for the aggregation windows and sample guards.
const (
	// DefaultWindow is the flow window. 90 days keeps per-scan denominators
	// large enough to be worth reading in a personal repository.
	DefaultWindow = 90 * 24 * time.Hour
	// DefaultMinSample is the smallest denominator that still gets a rate.
	DefaultMinSample = 5
	// DefaultMonths is how many month cohorts the trend table covers.
	DefaultMonths = 12

	// alertMinSample is deliberately stricter than DefaultMinSample: showing a
	// rate is cheap, raising an alert on a handful of issues is not.
	alertMinSample = 8
	// livenessWindow is how long a scan may stay silent before it is treated as
	// dead rather than merely quiet.
	livenessWindow = 28 * 24 * time.Hour
	// staleTriageDays is how long an untriaged issue may wait on the human gate.
	staleTriageDays = 14

	adoptedRateFloor       = 0.6
	rejectedAfterPRCeiling = 0.10
	mergeRateFloor         = 0.8
	prCreatedRateFloor     = 0.8
	prPendingFloor         = 3
)

// promptForScan maps a scan label to the routine prompt that governs it, so an
// alert can name the file to edit instead of just the symptom.
var promptForScan = map[string]string{
	"scan:nvim":        "routines/prompts/daily-neovim-trend-scan.md",
	"scan:scripts":     "routines/prompts/weekly-scripts-tooling-scan.md",
	"scan:environment": "routines/prompts/weekly-environment-scan.md",
	"scan:ai-agents":   "routines/prompts/weekly-devx-skills-hooks-scan.md",
	"scan:ci":          "routines/prompts/weekly-ci-workflows-scan.md",
}

const (
	prBotPrompt   = "routines/prompts/weekly-adopted-issue-pr-bot.md"
	prCarePrompt  = "routines/prompts/weekly-pr-care-bot.md"
	scopeOverall  = "overall"
	oneDay        = 24 * time.Hour
	percentileP50 = 0.5
	percentileP90 = 0.9
)

// Options tunes the aggregation. Now is injected so output is reproducible.
type Options struct {
	Now       time.Time
	Since     time.Time
	Window    time.Duration
	Months    int
	MinSample int
}

// Ratio is a rate with its denominator kept alongside, because a rate without a
// sample size invites exactly the over-reading this loop is meant to avoid.
// Rate is nil when the denominator is empty or below the sample guard.
type Ratio struct {
	Rate  *float64 `json:"rate"`
	Num   int      `json:"num"`
	Denom int      `json:"denom"`
}

// ScanStats is one scan routine's record over the flow window.
type ScanStats struct {
	Label               string   `json:"label"`
	Prompt              string   `json:"prompt"`
	Opened              int      `json:"opened"`
	OpenedLast28d       int      `json:"opened_last_28d"`
	Adopted             int      `json:"adopted"`
	Rejected            int      `json:"rejected"`
	Untriaged           int      `json:"untriaged"`
	UntrackedClose      int      `json:"untracked_close"`
	OldestUntriagedDays int      `json:"oldest_untriaged_days"`
	AdoptedRate         Ratio    `json:"adopted_rate"`
	RejectedAfterPRRate Ratio    `json:"rejected_after_pr_rate"`
	PRCreatedRate       Ratio    `json:"pr_created_rate"`
	MergeRate           Ratio    `json:"merge_rate"`
	PRLagDaysP50        *float64 `json:"pr_lag_days_p50"`
	E2ELeadDaysP50      *float64 `json:"e2e_lead_days_p50"`
	Sample              int      `json:"sample"`
}

// OverallStats aggregates every scan plus the auto/* pull request space.
type OverallStats struct {
	Opened               int      `json:"opened"`
	Adopted              int      `json:"adopted"`
	Rejected             int      `json:"rejected"`
	Untriaged            int      `json:"untriaged"`
	UntrackedClose       int      `json:"untracked_close"`
	OldestUntriagedDays  int      `json:"oldest_untriaged_days"`
	PRPending            int      `json:"pr_pending"`
	AdoptedRate          Ratio    `json:"adopted_rate"`
	RejectedAfterPRRate  Ratio    `json:"rejected_after_pr_rate"`
	PRCreatedRate        Ratio    `json:"pr_created_rate"`
	MergeRate            Ratio    `json:"merge_rate"`
	MergedPRs            int      `json:"merged_prs"`
	UnmergedClosedPRs    int      `json:"unmerged_closed_prs"`
	OpenPRs              int      `json:"open_prs"`
	UnmergedPRNumbers    []int    `json:"unmerged_pr_numbers"`
	MetaBranchPRs        int      `json:"meta_branch_prs"`
	RejectLatencyDaysP50 *float64 `json:"reject_latency_days_p50"`
	PRLagDaysP50         *float64 `json:"pr_lag_days_p50"`
	MergeLeadDaysP50     *float64 `json:"merge_lead_days_p50"`
	MergeLeadDaysP90     *float64 `json:"merge_lead_days_p90"`
	E2ELeadDaysP50       *float64 `json:"e2e_lead_days_p50"`
	PrePolicyIssues      int      `json:"pre_policy_issues"`
}

// MonthStats is one month cohort. Issue-side figures are bucketed by the issue's
// createdAt and PR-side figures by the PR's createdAt; the two are deliberately
// not mixed.
type MonthStats struct {
	Month          string   `json:"month"`
	Opened         int      `json:"opened"`
	AdoptedRate    Ratio    `json:"adopted_rate"`
	PRCreatedRate  Ratio    `json:"pr_created_rate"`
	PRsCreated     int      `json:"prs_created"`
	MergeRate      Ratio    `json:"merge_rate"`
	E2ELeadDaysP50 *float64 `json:"e2e_lead_days_p50"`
	Partial        bool     `json:"partial"`
}

// Alert is a threshold breach, carrying the prompt files responsible for it so
// the monthly meta loop has somewhere to go.
type Alert struct {
	Metric  string   `json:"metric"`
	Scope   string   `json:"scope"`
	Message string   `json:"message"`
	Prompts []string `json:"prompts"`
}

// Anomalies records inputs that break the label conventions the metrics assume.
type Anomalies struct {
	MultiScanLabel []int `json:"multi_scan_label"`
	TriagedNonScan []int `json:"triaged_non_scan"`
}

// Summary is the whole report and the schema embedded in the digest issue.
type Summary struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Since       time.Time    `json:"since"`
	WindowDays  int          `json:"window_days"`
	MinSample   int          `json:"min_sample"`
	Overall     OverallStats `json:"overall"`
	Scans       []ScanStats  `json:"scans"`
	Months      []MonthStats `json:"months"`
	Alerts      []Alert      `json:"alerts"`
	Anomalies   Anomalies    `json:"anomalies"`
}

// scanBucket accumulates one scan label's issues before they become ScanStats.
type scanBucket struct {
	issues        []*model.Issue
	openedLast28d int
	prLags        []float64
	e2eLeads      []float64
	mergeLeads    []float64
	merged        int
	unmergedClose int
}

// Summarize folds issues and pull requests into the report.
func Summarize(issues []model.Issue, prs []model.PullRequest, opt Options) Summary {
	opt = opt.withDefaults()
	windowStart := opt.Now.Add(-opt.Window)
	livenessStart := opt.Now.Add(-livenessWindow)

	sum := Summary{
		GeneratedAt: opt.Now.UTC(),
		Since:       opt.Since.UTC(),
		WindowDays:  int(opt.Window / oneDay),
		MinSample:   opt.MinSample,
		Scans:       []ScanStats{},
		Months:      []MonthStats{},
		Alerts:      []Alert{},
		Anomalies:   Anomalies{MultiScanLabel: []int{}, TriagedNonScan: []int{}},
	}

	issueByNumber := make(map[int]*model.Issue, len(issues))
	// buckets is keyed off every scan label seen since the policy start, not just
	// the ones active in the window, so a scan that went silent still shows up.
	buckets := map[string]*scanBucket{}
	for i := range issues {
		iss := &issues[i]
		if iss.ScanLabelCount > 1 {
			sum.Anomalies.MultiScanLabel = append(sum.Anomalies.MultiScanLabel, iss.Number)
		}
		if iss.ScanLabel == "" {
			if iss.Triage == model.Adopted || iss.Triage == model.Rejected {
				sum.Anomalies.TriagedNonScan = append(sum.Anomalies.TriagedNonScan, iss.Number)
			}
			continue
		}
		if iss.CreatedAt.Before(opt.Since) {
			sum.Overall.PrePolicyIssues++
			continue
		}
		issueByNumber[iss.Number] = iss
		if _, ok := buckets[iss.ScanLabel]; !ok {
			buckets[iss.ScanLabel] = &scanBucket{}
		}
		bucket := buckets[iss.ScanLabel]
		if !iss.CreatedAt.Before(livenessStart) {
			bucket.openedLast28d++
		}
		if iss.CreatedAt.Before(windowStart) {
			continue
		}
		bucket.issues = append(bucket.issues, iss)
	}

	autoPRs := collectAutoPRs(prs, windowStart)
	joinPRs(autoPRs, issueByNumber, buckets, &sum.Overall)

	for label, bucket := range buckets {
		sum.Scans = append(sum.Scans, bucket.stats(label, opt))
	}
	slices.SortFunc(sum.Scans, func(a, b ScanStats) int { return cmp.Compare(a.Label, b.Label) })

	sum.Overall = overallStats(sum.Overall, sum.Scans, buckets, opt)
	sum.Months = monthlyStats(issueByNumber, autoPRs, opt)
	sum.Alerts = EvaluateAlerts(&sum)
	return sum
}

func (o Options) withDefaults() Options {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	o.Now = o.Now.UTC()
	if o.Window <= 0 {
		o.Window = DefaultWindow
	}
	if o.Months <= 0 {
		o.Months = DefaultMonths
	}
	if o.MinSample <= 0 {
		o.MinSample = DefaultMinSample
	}
	return o
}

// collectAutoPRs keeps the auto/* pull requests created inside the window.
func collectAutoPRs(prs []model.PullRequest, windowStart time.Time) []*model.PullRequest {
	out := make([]*model.PullRequest, 0, len(prs))
	for i := range prs {
		pr := &prs[i]
		if !pr.IsAuto() || pr.CreatedAt.Before(windowStart) {
			continue
		}
		out = append(out, pr)
	}
	return out
}

// joinPRs attributes each auto PR to the scan that produced its issue, using the
// issue number the PR Bot encodes in the branch name.
func joinPRs(prs []*model.PullRequest, issueByNumber map[int]*model.Issue, buckets map[string]*scanBucket, overall *OverallStats) {
	for _, pr := range prs {
		switch {
		case pr.Open:
			overall.OpenPRs++
		case pr.Merged:
			overall.MergedPRs++
		default:
			overall.UnmergedClosedPRs++
			overall.UnmergedPRNumbers = append(overall.UnmergedPRNumbers, pr.Number)
		}

		iss, ok := issueByNumber[pr.IssueNumber]
		if !ok {
			// auto/routine-improve-* and friends carry no issue number; they are
			// part of the pipeline but not attributable to a scan.
			overall.MetaBranchPRs++
			continue
		}
		bucket, ok := buckets[iss.ScanLabel]
		if !ok {
			continue
		}
		bucket.prLags = append(bucket.prLags, days(iss.CreatedAt, pr.CreatedAt))
		switch {
		case pr.Merged:
			bucket.merged++
			bucket.e2eLeads = append(bucket.e2eLeads, days(iss.CreatedAt, pr.MergedAt))
			bucket.mergeLeads = append(bucket.mergeLeads, days(pr.CreatedAt, pr.MergedAt))
		case !pr.Open:
			bucket.unmergedClose++
		}
	}
	slices.Sort(overall.UnmergedPRNumbers)
}

func (b *scanBucket) stats(label string, opt Options) ScanStats {
	st := ScanStats{
		Label:         label,
		Prompt:        promptForScan[label],
		Opened:        len(b.issues),
		OpenedLast28d: b.openedLast28d,
	}
	prCreated, rejectedAfterPR := 0, 0
	for _, iss := range b.issues {
		switch iss.Triage {
		case model.Adopted:
			st.Adopted++
		case model.Rejected:
			st.Rejected++
		case model.Untriaged:
			st.Untriaged++
			st.OldestUntriagedDays = max(st.OldestUntriagedDays, int(days(iss.CreatedAt, opt.Now)))
		case model.UntrackedClose:
			st.UntrackedClose++
		}
		if iss.PRCreated {
			prCreated++
			if iss.Triage == model.Rejected {
				rejectedAfterPR++
			}
		}
	}
	st.Sample = st.Adopted + st.Rejected
	st.AdoptedRate = newRatio(st.Adopted, st.Sample, opt.MinSample)
	st.RejectedAfterPRRate = newRatio(rejectedAfterPR, prCreated, opt.MinSample)
	st.PRCreatedRate = newRatio(prCreatedAmongAdopted(b.issues), st.Adopted, opt.MinSample)
	st.MergeRate = newRatio(b.merged, b.merged+b.unmergedClose, opt.MinSample)
	st.PRLagDaysP50 = percentile(b.prLags, percentileP50)
	st.E2ELeadDaysP50 = percentile(b.e2eLeads, percentileP50)
	return st
}

func prCreatedAmongAdopted(issues []*model.Issue) int {
	n := 0
	for _, iss := range issues {
		if iss.Triage == model.Adopted && iss.PRCreated {
			n++
		}
	}
	return n
}

func overallStats(base OverallStats, scans []ScanStats, buckets map[string]*scanBucket, opt Options) OverallStats {
	out := base
	adoptedWithPR, rejectedAfterPR, prCreated := 0, 0, 0
	var rejectLatencies, prLags, e2eLeads, mergeLeads []float64
	for _, st := range scans {
		out.Opened += st.Opened
		out.Adopted += st.Adopted
		out.Rejected += st.Rejected
		out.Untriaged += st.Untriaged
		out.UntrackedClose += st.UntrackedClose
		out.OldestUntriagedDays = max(out.OldestUntriagedDays, st.OldestUntriagedDays)
		adoptedWithPR += st.PRCreatedRate.Num
		rejectedAfterPR += st.RejectedAfterPRRate.Num
		prCreated += st.RejectedAfterPRRate.Denom
	}
	for _, bucket := range buckets {
		prLags = append(prLags, bucket.prLags...)
		e2eLeads = append(e2eLeads, bucket.e2eLeads...)
		for _, iss := range bucket.issues {
			if iss.Triage == model.Adopted && !iss.PRCreated && !iss.Closed {
				out.PRPending++
			}
			if iss.Triage == model.Rejected && !iss.ClosedAt.IsZero() {
				rejectLatencies = append(rejectLatencies, days(iss.CreatedAt, iss.ClosedAt))
			}
		}
	}
	mergeLeads = mergeLeadDays(buckets)

	out.AdoptedRate = newRatio(out.Adopted, out.Adopted+out.Rejected, opt.MinSample)
	out.PRCreatedRate = newRatio(adoptedWithPR, out.Adopted, opt.MinSample)
	out.RejectedAfterPRRate = newRatio(rejectedAfterPR, prCreated, opt.MinSample)
	out.MergeRate = newRatio(out.MergedPRs, out.MergedPRs+out.UnmergedClosedPRs, opt.MinSample)
	out.RejectLatencyDaysP50 = percentile(rejectLatencies, percentileP50)
	out.PRLagDaysP50 = percentile(prLags, percentileP50)
	out.E2ELeadDaysP50 = percentile(e2eLeads, percentileP50)
	out.MergeLeadDaysP50 = percentile(mergeLeads, percentileP50)
	out.MergeLeadDaysP90 = percentile(mergeLeads, percentileP90)
	if out.UnmergedPRNumbers == nil {
		out.UnmergedPRNumbers = []int{}
	}
	return out
}

// mergeLeadDays is collected separately from the scan buckets because a merge
// lead time is a property of the PR, not of the scan that seeded it.
func mergeLeadDays(buckets map[string]*scanBucket) []float64 {
	var out []float64
	for _, bucket := range buckets {
		out = append(out, bucket.mergeLeads...)
	}
	return out
}

func monthlyStats(issueByNumber map[int]*model.Issue, prs []*model.PullRequest, opt Options) []MonthStats {
	type monthBucket struct {
		opened        int
		adopted       int
		rejected      int
		adoptedWithPR int
		prsCreated    int
		merged        int
		unmergedClose int
		e2eLeads      []float64
	}
	months := map[string]*monthBucket{}
	get := func(key string) *monthBucket {
		if _, ok := months[key]; !ok {
			months[key] = &monthBucket{}
		}
		return months[key]
	}

	for _, iss := range issueByNumber {
		bucket := get(iss.CreatedAt.Format("2006-01"))
		bucket.opened++
		switch iss.Triage {
		case model.Adopted:
			bucket.adopted++
			if iss.PRCreated {
				bucket.adoptedWithPR++
			}
		case model.Rejected:
			bucket.rejected++
		case model.Untriaged, model.UntrackedClose:
		}
	}
	for _, pr := range prs {
		bucket := get(pr.CreatedAt.Format("2006-01"))
		bucket.prsCreated++
		switch {
		case pr.Merged:
			bucket.merged++
			if iss, ok := issueByNumber[pr.IssueNumber]; ok {
				bucket.e2eLeads = append(bucket.e2eLeads, days(iss.CreatedAt, pr.MergedAt))
			}
		case !pr.Open:
			bucket.unmergedClose++
		}
	}

	currentMonth := opt.Now.Format("2006-01")
	out := make([]MonthStats, 0, len(months))
	for key, bucket := range months {
		out = append(out, MonthStats{
			Month:          key,
			Opened:         bucket.opened,
			AdoptedRate:    newRatio(bucket.adopted, bucket.adopted+bucket.rejected, opt.MinSample),
			PRCreatedRate:  newRatio(bucket.adoptedWithPR, bucket.adopted, opt.MinSample),
			PRsCreated:     bucket.prsCreated,
			MergeRate:      newRatio(bucket.merged, bucket.merged+bucket.unmergedClose, opt.MinSample),
			E2ELeadDaysP50: percentile(bucket.e2eLeads, percentileP50),
			Partial:        key == currentMonth,
		})
	}
	slices.SortFunc(out, func(a, b MonthStats) int { return cmp.Compare(a.Month, b.Month) })
	if len(out) > opt.Months {
		out = out[len(out)-opt.Months:]
	}
	return out
}

// EvaluateAlerts applies the thresholds. Rates only fire above alertMinSample;
// liveness and the human-gate backlog have no sample guard because a zero is a
// zero however small the history is.
func EvaluateAlerts(sum *Summary) []Alert {
	alerts := []Alert{}
	for i := range sum.Scans {
		st := &sum.Scans[i]
		prompts := []string{}
		if st.Prompt != "" {
			prompts = append(prompts, st.Prompt)
		}
		if fires(st.AdoptedRate, func(rate float64) bool { return rate < adoptedRateFloor }) {
			alerts = append(alerts, Alert{
				Metric:  "adopted_rate",
				Scope:   st.Label,
				Message: fmt.Sprintf("%s の採用率が %s（n=%d）で閾値 %.2f を下回っている。提案の選定基準が広すぎる可能性がある。", st.Label, formatRate(st.AdoptedRate), st.AdoptedRate.Denom, adoptedRateFloor),
				Prompts: prompts,
			})
		}
		if fires(st.RejectedAfterPRRate, func(rate float64) bool { return rate > rejectedAfterPRCeiling }) {
			alerts = append(alerts, Alert{
				Metric:  "rejected_after_pr_rate",
				Scope:   st.Label,
				Message: fmt.Sprintf("%s は PR 化した提案の %s（n=%d）が最終的に不採用になっている。実装コストを払ってから捨てている。", st.Label, formatRate(st.RejectedAfterPRRate), st.RejectedAfterPRRate.Denom),
				Prompts: append(slices.Clone(prompts), prBotPrompt),
			})
		}
		if st.OpenedLast28d == 0 {
			alerts = append(alerts, Alert{
				Metric:  "liveness",
				Scope:   st.Label,
				Message: fmt.Sprintf("%s は直近 28 日で 1 件も起票していない。ネタ切れか Routine の実行失敗を疑う。", st.Label),
				Prompts: prompts,
			})
		}
	}

	if fires(sum.Overall.MergeRate, func(rate float64) bool { return rate < mergeRateFloor }) {
		alerts = append(alerts, Alert{
			Metric:  "merge_rate",
			Scope:   scopeOverall,
			Message: fmt.Sprintf("auto/* PR のマージ率が %s（n=%d）で閾値 %.2f を下回っている。", formatRate(sum.Overall.MergeRate), sum.Overall.MergeRate.Denom, mergeRateFloor),
			Prompts: []string{prBotPrompt, prCarePrompt},
		})
	}
	if sum.Overall.PRPending >= prPendingFloor &&
		fires(sum.Overall.PRCreatedRate, func(rate float64) bool { return rate < prCreatedRateFloor }) {
		alerts = append(alerts, Alert{
			Metric:  "pr_created_rate",
			Scope:   scopeOverall,
			Message: fmt.Sprintf("adopted の PR 化率が %s（n=%d）で、PR 化待ちが %d 件滞留している。PR Bot の実行失敗を疑う。", formatRate(sum.Overall.PRCreatedRate), sum.Overall.PRCreatedRate.Denom, sum.Overall.PRPending),
			Prompts: []string{prBotPrompt},
		})
	}
	if sum.Overall.OldestUntriagedDays > staleTriageDays {
		alerts = append(alerts, Alert{
			Metric:  "triage_backlog",
			Scope:   scopeOverall,
			Message: fmt.Sprintf("最も古い未 triage の scan Issue が %d 日待っている（閾値 %d 日）。判定は人間側の担当。", sum.Overall.OldestUntriagedDays, staleTriageDays),
			Prompts: []string{},
		})
	}

	slices.SortFunc(alerts, func(a, b Alert) int {
		if c := cmp.Compare(a.Metric, b.Metric); c != 0 {
			return c
		}
		return cmp.Compare(a.Scope, b.Scope)
	})
	return alerts
}

// fires reports whether a rate exists, clears the alert sample guard, and
// satisfies the breach predicate.
func fires(r Ratio, breached func(rate float64) bool) bool {
	return r.Rate != nil && r.Denom >= alertMinSample && breached(*r.Rate)
}

func formatRate(r Ratio) string {
	if r.Rate == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *r.Rate*100)
}

// newRatio builds a Ratio, withholding the rate when the denominator is too
// small to mean anything.
func newRatio(num, denom, minSample int) Ratio {
	r := Ratio{Num: num, Denom: denom}
	if denom <= 0 || denom < minSample {
		return r
	}
	rate := roundTo(float64(num)/float64(denom), 3)
	r.Rate = &rate
	return r
}

// percentile interpolates linearly between the two neighbouring samples, so a
// p50 over an even-length input is the mean of the middle pair. It returns nil
// for an empty input rather than a misleading zero.
func percentile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	if len(sorted) == 1 {
		out := roundTo(sorted[0], 1)
		return &out
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	value := sorted[lo]
	if hi != lo {
		value = sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
	}
	out := roundTo(value, 1)
	return &out
}

func roundTo(v float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return math.Round(v*scale) / scale
}

// days is the elapsed time from a to b expressed in fractional days.
func days(a, b time.Time) float64 {
	return b.Sub(a).Hours() / 24
}
