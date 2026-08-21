// Package render turns a metrics.Report into the three artefacts the weekly
// digest is built from: the warning block that goes on top of the issue body,
// the flow section that goes below the existing stock section, and the JSON
// document the monthly meta loop parses.
//
// All output is deterministic and free of wall-clock reads; the report carries
// its own generation time.
package render

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"pipeline-metrics/internal/metrics"
)

// JSONBlockMarker opens the fenced block that carries the machine-readable
// report inside the digest body. The meta loop looks for it.
const JSONBlockMarker = "<!-- pipeline-metrics:json -->"

// JSON serialises the report for embedding and for `--format json`.
func JSON(r *metrics.Report) ([]byte, error) {
	out, marshalErr := json.MarshalIndent(r, "", "  ")
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal report: %w", marshalErr)
	}
	return out, nil
}

// Alerts renders the warning block, or "" when nothing tripped. The digest
// workflow keys the `alert` label off this emptiness, so a quiet week must
// produce no bytes at all.
func Alerts(r *metrics.Report) string {
	if len(r.Alerts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "> [!WARNING]\n> **閾値超過 %d 件**（%s 時点 / 集計起点 %s）\n>\n",
		len(r.Alerts), r.GeneratedAt, r.Since)
	for i := range r.Alerts {
		fmt.Fprintf(&b, "> - %s\n", alertMessage(r, &r.Alerts[i]))
	}
	return b.String()
}

func alertMessage(r *metrics.Report, a *metrics.Alert) string {
	fix := func(hint string) string {
		if a.OwnerPrompt == "" {
			return hint
		}
		return fmt.Sprintf("%s → `%s`", hint, a.OwnerPrompt)
	}
	switch a.Kind {
	case metrics.AlertLiveness:
		return fix(fmt.Sprintf(
			"`%s` の起票が直近 28 日で 0 件（窓内 %d 件）。Routine が停止している疑い。claude.ai の Routine 稼働状況と併せて確認する",
			a.Scope, a.Sample))
	case metrics.AlertTriageBacklog:
		return fmt.Sprintf(
			"最古の未 triage が %s 日経過（#%d / 未 triage 計 %d 件、閾値 %s 日）。`adopted` か `rejected` を付けて滞留を解く",
			trimFloat(a.Value), a.Observed, a.Sample, trimFloat(a.Threshold))
	case metrics.AlertAdoptedRate:
		return fix(fmt.Sprintf(
			"`%s` の採用率 %s（%d/%d）が閾値 %s を下回る。提案の選定基準を見直す",
			a.Scope, pct(a.Value, 0), a.Observed, a.Sample, pct(a.Threshold, 0)))
	case metrics.AlertRejectedAfterPRRate:
		return fix(fmt.Sprintf(
			"`%s` は PR 化後の却下が %s（%d/%d）で閾値 %s を超える。最もコストの高い失敗なので、提案の具体性と triage の判断タイミングを見直す",
			a.Scope, pct(a.Value, 1), a.Observed, a.Sample, pct(a.Threshold, 1)))
	case metrics.AlertPRCreatedRate:
		pending := 0
		if s := findScan(r, a.Scope); s != nil {
			pending = s.PRPending
		}
		return fix(fmt.Sprintf(
			"`%s` の PR 化率 %s（%d/%d）が閾値 %s を下回り、PR 化待ちが %d 件滞留している",
			a.Scope, pct(a.Value, 0), a.Observed, a.Sample, pct(a.Threshold, 0), pending))
	case metrics.AlertMergeRate:
		return fix(fmt.Sprintf(
			"`%s` の auto PR マージ率 %s（%d/%d）が閾値 %s を下回る。CI 失敗・コンフリクトの解消が追いついていない",
			a.Scope, pct(a.Value, 0), a.Observed, a.Sample, pct(a.Threshold, 0)))
	default:
		return fix(fmt.Sprintf("`%s` の %s が閾値を超えた（%s / 閾値 %s）",
			a.Scope, a.Kind, trimFloat(a.Value), trimFloat(a.Threshold)))
	}
}

func findScan(r *metrics.Report, scan string) *metrics.ScanMetrics {
	for i := range r.Scans {
		if r.Scans[i].Scan == scan {
			return &r.Scans[i]
		}
	}
	return nil
}

// Flow renders the digest's flow section: scan quality, triage, PR creation,
// merge, the monthly trend, the bookkeeping anomalies and the embedded JSON.
func Flow(r *metrics.Report) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "## フロー指標（%s 時点）\n\n", r.GeneratedAt)
	fmt.Fprintf(&b,
		"集計起点 %s / 率の窓 直近 %d 日（起点 %s 以降）/ 母数 %d 件未満は率を出さない"+
			"（集計起点より前の scan Issue %d 件は件数のみで率の分母に入れない）。"+
			"状態ファイルは持たず毎回全再計算するため、ラベルを編集すると過去の数値も動く。\n\n",
		r.Since, r.WindowDays, r.WindowStart, r.MinSample, r.ExcludedBeforeSince)

	b.WriteString("### スキャン品質\n\n")
	writeTable(&b,
		[]string{"スキャン", "起票", "直近 28 日", "採用", "却下", "未 triage", "追跡外 Close", "採用率", "PR 後却下率"},
		scanRows(r, func(s *metrics.ScanMetrics) []string {
			return []string{
				itoa(s.Opened), itoa(s.OpenedLast28d), itoa(s.Adopted), itoa(s.Rejected),
				itoa(s.Untriaged), itoa(s.UntrackedClose),
				rateCell(s.AdoptedRate, s.Adopted, s.Opened, 0),
				rateCell(s.RejectedAfterPRRate, s.RejectedAfterPR, s.Opened, 1),
			}
		}))

	b.WriteString("\n### triage と PR 化\n\n")
	writeTable(&b,
		[]string{"スキャン", "最古の未 triage（日）", "却下まで p50（日）", "PR 化率", "PR 化待ち", "起票→PR p50（日）"},
		scanRows(r, func(s *metrics.ScanMetrics) []string {
			return []string{
				daysCell(s.OldestUntriagedDays), daysCell(s.RejectLatencyP50),
				rateCell(s.PRCreatedRate, s.AdoptedWithPR, s.Adopted, 0),
				itoa(s.PRPending), daysCell(s.PRLagP50),
			}
		}))

	b.WriteString("\n### マージ\n\n")
	writeTable(&b,
		[]string{"スキャン", "PR", "マージ", "未マージ Close", "Open", "マージ率", "PR→マージ p50", "p90", "起票→マージ p50"},
		scanRows(r, func(s *metrics.ScanMetrics) []string {
			return []string{
				itoa(s.PRs), itoa(s.Merged), itoa(s.ClosedUnmerged), itoa(s.OpenPRs),
				rateCell(s.MergeRate, s.Merged, s.Merged+s.ClosedUnmerged, 0),
				daysCell(s.MergeLeadP50), daysCell(s.MergeLeadP90), daysCell(s.E2ELeadP50),
			}
		}))

	b.WriteString("\n### 月次トレンド（起票月コホート）\n\n")
	monthRows := make([][]string, 0, len(r.Months))
	for i := range r.Months {
		m := &r.Months[i]
		monthRows = append(monthRows, []string{
			m.Month, itoa(m.Opened), itoa(m.Adopted), itoa(m.Rejected), itoa(m.Untriaged),
			rateCell(m.AdoptedRate, m.Adopted, m.Opened, 0), itoa(m.Merged), daysCell(m.E2ELeadP50),
		})
	}
	writeTable(&b,
		[]string{"月", "起票", "採用", "却下", "未 triage", "採用率", "マージ済", "起票→マージ p50"},
		monthRows)

	b.WriteString("\n### ラベル運用の異常\n\n")
	writeAnomalies(&b, &r.Anomalies)

	jsonBytes, jsonErr := JSON(r)
	if jsonErr != nil {
		return "", jsonErr
	}
	fmt.Fprintf(&b, "\n%s\n\n<details>\n<summary>machine-readable metrics (JSON)</summary>\n\n```json\n%s\n```\n\n</details>\n",
		JSONBlockMarker, jsonBytes)
	return b.String(), nil
}

// scanRows renders one row per scan plus a bold total row. cells receives each
// row's metrics and returns everything after the leading name column.
func scanRows(r *metrics.Report, cells func(s *metrics.ScanMetrics) []string) [][]string {
	rows := make([][]string, 0, len(r.Scans)+1)
	for i := range r.Scans {
		s := &r.Scans[i]
		rows = append(rows, append([]string{"`" + s.Scan + "`"}, cells(s)...))
	}
	total := cells(&r.Total)
	for i, c := range total {
		total[i] = "**" + c + "**"
	}
	rows = append(rows, append([]string{"**合計**"}, total...))
	return rows
}

func writeTable(b *strings.Builder, header []string, rows [][]string) {
	if len(rows) == 0 {
		b.WriteString("データなし\n")
		return
	}
	b.WriteString("| " + strings.Join(header, " | ") + " |\n")
	aligns := make([]string, len(header))
	for i := range aligns {
		aligns[i] = "---:"
	}
	aligns[0] = "---"
	b.WriteString("| " + strings.Join(aligns, " | ") + " |\n")
	for _, row := range rows {
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
}

func writeAnomalies(b *strings.Builder, a *metrics.Anomalies) {
	lines := []string{
		issueLine("`scan:` ラベル重複", a.DuplicateScanLabels),
		issueLine("`adopted` と `rejected` の併存（rejected に倒して集計）", a.AdoptedAndRejected),
		issueLine("ラベル無しで Close（追跡外）", a.UntrackedClose),
		issueLine("`rejected` なのに Open のまま", a.RejectedStillOpen),
		issueLine("`pr-created` があるが PR が見つからない", a.PRLabelWithoutPR),
		issueLine("PR はあるが `pr-created` が無い", a.PRWithoutLabel),
		issueLine("`scan:` 無しで triage された Issue（集計対象外）", a.TriagedNonScan),
		issueLine("issue に紐づかない `auto/*` PR（meta ブランチ）", a.MetaBranchPRs),
		issueLine("scan Issue に紐づかない `auto/issue-*` PR", a.OrphanPRs),
	}
	written := false
	for _, line := range lines {
		if line != "" {
			b.WriteString(line)
			written = true
		}
	}
	for _, w := range a.ParseWarnings {
		fmt.Fprintf(b, "- パース不能なレコード: %s\n", w)
		written = true
	}
	if !written {
		b.WriteString("なし\n")
	}
}

func issueLine(label string, numbers []int) string {
	if len(numbers) == 0 {
		return ""
	}
	refs := make([]string, 0, len(numbers))
	for _, n := range numbers {
		refs = append(refs, "#"+itoa(n))
	}
	return fmt.Sprintf("- %s: %s\n", label, strings.Join(refs, " "))
}

// rateCell renders a rate as "82% (75/91)", or "— (n=3)" when the sample was
// too small for the rate to be reported.
func rateCell(v *float64, num, den, prec int) string {
	if v == nil {
		if den == 0 {
			return "—"
		}
		return fmt.Sprintf("— (n=%d)", den)
	}
	return fmt.Sprintf("%s (%d/%d)", pct(*v, prec), num, den)
}

func daysCell(v *float64) string {
	if v == nil {
		return "—"
	}
	return trimFloat(*v)
}

func pct(v float64, prec int) string {
	return strconv.FormatFloat(v*100, 'f', prec, 64) + "%"
}

// trimFloat prints a float with at most one decimal, dropping a trailing ".0"
// so that whole numbers read as "14" rather than "14.0".
func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

func itoa(v int) string { return strconv.Itoa(v) }
