// Package render turns a metrics summary into the three fragments the digest
// workflow concatenates: the warning block that goes above everything, the flow
// section, and the machine-readable JSON the monthly meta loop parses.
package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"pipeline-metrics/internal/metrics"
)

// Alerts renders the warning block for the top of the digest body. It returns
// an empty string when nothing breached, so the caller can test for emptiness
// instead of parsing the output.
func Alerts(sum *metrics.Summary) string {
	if len(sum.Alerts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("> [!WARNING]\n")
	b.WriteString("> **効果測定で閾値を超えた指標があります。**\n>\n")
	for i := range sum.Alerts {
		alert := &sum.Alerts[i]
		fmt.Fprintf(&b, "> - `%s` / `%s` — %s", alert.Metric, alert.Scope, alert.Message)
		if len(alert.Prompts) > 0 {
			quoted := make([]string, 0, len(alert.Prompts))
			for _, p := range alert.Prompts {
				quoted = append(quoted, "`"+p+"`")
			}
			fmt.Fprintf(&b, "（→ %s）", strings.Join(quoted, " / "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Markdown renders the flow section: the per-scan table, the overall figures,
// the month trend and the embedded JSON block.
func Markdown(sum *metrics.Summary) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "## 直近 %d 日の成績（フロー）\n\n", sum.WindowDays)
	fmt.Fprintf(&b,
		"集計対象は %s 以降に起票された `scan:*` Issue と `auto/*` PR。母数が %d 件未満の率は `—` と表示する（小標本の率は偶然で大きく振れるため）。\n\n",
		sum.Since.Format("2006-01-02"), sum.MinSample)

	b.WriteString("| スキャン | 起票 | 直近28日 | 採用率 | 実装後却下 | PR化率 | マージ率 | PR化 p50 | E2E p50 |\n")
	b.WriteString("| --- | --: | --: | --- | --- | --- | --- | --: | --: |\n")
	for i := range sum.Scans {
		st := &sum.Scans[i]
		fmt.Fprintf(&b, "| `%s` | %d | %d | %s | %s | %s | %s | %s | %s |\n",
			st.Label, st.Opened, st.OpenedLast28d,
			ratio(st.AdoptedRate), ratio(st.RejectedAfterPRRate), ratio(st.PRCreatedRate),
			ratio(st.MergeRate), days(st.PRLagDaysP50), days(st.E2ELeadDaysP50))
	}
	overall := &sum.Overall
	fmt.Fprintf(&b, "| **合計** | %d | — | %s | %s | %s | %s | %s | %s |\n\n",
		overall.Opened, ratio(overall.AdoptedRate), ratio(overall.RejectedAfterPRRate),
		ratio(overall.PRCreatedRate), ratio(overall.MergeRate),
		days(overall.PRLagDaysP50), days(overall.E2ELeadDaysP50))

	fmt.Fprintf(&b, "- `auto/*` PR: マージ %d / 未マージ close %d / Open %d（うち Issue に紐づかない meta ブランチ %d）\n",
		overall.MergedPRs, overall.UnmergedClosedPRs, overall.OpenPRs, overall.MetaBranchPRs)
	fmt.Fprintf(&b, "- マージまで p50 %s / p90 %s、不採用判定まで p50 %s\n",
		days(overall.MergeLeadDaysP50), days(overall.MergeLeadDaysP90), days(overall.RejectLatencyDaysP50))
	fmt.Fprintf(&b, "- 未 triage %d 件（最古 %d 日）/ PR 化待ち %d 件\n",
		overall.Untriaged, overall.OldestUntriagedDays, overall.PRPending)
	fmt.Fprintf(&b, "- 未マージ close の PR: %s\n", numbers(overall.UnmergedPRNumbers))
	fmt.Fprintf(&b, "- ラベル運用の崩れ: untracked close %d 件 / `scan:*` 重複 %s / scan 以外に triage ラベル %s\n",
		overall.UntrackedClose, numbers(sum.Anomalies.MultiScanLabel), numbers(sum.Anomalies.TriagedNonScan))
	if overall.PrePolicyIssues > 0 {
		fmt.Fprintf(&b, "- 集計起点より前の scan Issue %d 件は率の分母から除外している（`rejected` 運用の開始前）\n", overall.PrePolicyIssues)
	}
	b.WriteString("\n")

	b.WriteString("<details>\n<summary>月次トレンド</summary>\n\n")
	b.WriteString("Issue 側（起票・採用率・PR 化率）は Issue の起票月、PR 側（作成数・マージ率・E2E）は PR の作成月で束ねている。\n\n")
	b.WriteString("| 月 | 起票 | 採用率 | PR化率 | PR作成 | マージ率 | E2E p50 |\n")
	b.WriteString("| --- | --: | --- | --- | --: | --- | --: |\n")
	for i := range sum.Months {
		m := &sum.Months[i]
		label := m.Month
		if m.Partial {
			label += "（進行中）"
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %d | %s | %s |\n",
			label, m.Opened, ratio(m.AdoptedRate), ratio(m.PRCreatedRate),
			m.PRsCreated, ratio(m.MergeRate), days(m.E2ELeadDaysP50))
	}
	b.WriteString("\n</details>\n\n")

	payload, marshalErr := JSON(sum)
	if marshalErr != nil {
		return "", marshalErr
	}
	b.WriteString("<details>\n<summary>machine-readable metrics</summary>\n\n")
	b.WriteString("```json\n")
	b.Write(payload)
	b.WriteString("\n```\n\n</details>\n")
	return b.String(), nil
}

// JSON serialises the summary for tool-to-tool use.
func JSON(sum *metrics.Summary) ([]byte, error) {
	out, marshalErr := json.MarshalIndent(sum, "", "  ")
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal summary: %w", marshalErr)
	}
	return out, nil
}

// ratio prints a rate with its denominator, or an em dash when the sample was
// too small for the rate to be meaningful.
func ratio(r metrics.Ratio) string {
	if r.Rate == nil {
		return fmt.Sprintf("— (n=%d)", r.Denom)
	}
	return fmt.Sprintf("%.0f%% (n=%d)", *r.Rate*100, r.Denom)
}

func days(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f 日", *v)
}

func numbers(ns []int) string {
	if len(ns) == 0 {
		return "なし"
	}
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, fmt.Sprintf("#%d", n))
	}
	return strings.Join(parts, " ")
}
