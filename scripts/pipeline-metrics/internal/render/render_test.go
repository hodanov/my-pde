package render

import (
	"encoding/json"
	"strings"
	"testing"

	"pipeline-metrics/internal/metrics"
)

func TestAlertsIsEmptyWhenNothingTripped(t *testing.T) {
	t.Parallel()
	report := metrics.Report{Alerts: []metrics.Alert{}}
	// The digest workflow decides whether to attach the `alert` label by
	// checking this output for content, so "quiet" must mean zero bytes.
	if got := Alerts(&report); got != "" {
		t.Errorf("Alerts() = %q, want an empty string", got)
	}
}

func TestAlertMessages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		alert    metrics.Alert
		scans    []metrics.ScanMetrics
		wantAll  []string
		wantNone []string
	}{
		{
			name: "adopted rate names the scan prompt",
			alert: metrics.Alert{
				Kind: metrics.AlertAdoptedRate, Scope: "scan:scripts", Value: 0.4615, Threshold: 0.6,
				Observed: 6, Sample: 13, OwnerPrompt: "routines/prompts/weekly-scripts-tooling-scan.md",
			},
			wantAll: []string{"scan:scripts", "46%", "（6/13）", "60%", "weekly-scripts-tooling-scan.md"},
		},
		{
			name: "rejected after pr keeps one decimal",
			alert: metrics.Alert{
				Kind: metrics.AlertRejectedAfterPRRate, Scope: "scan:nvim", Value: 0.1224, Threshold: 0.1,
				Observed: 6, Sample: 49, OwnerPrompt: "routines/prompts/daily-neovim-trend-scan.md",
			},
			wantAll: []string{"12.2%", "10.0%", "（6/49）"},
		},
		{
			name: "liveness explains the silence",
			alert: metrics.Alert{
				Kind: metrics.AlertLiveness, Scope: "scan:ci", Threshold: 1, Sample: 9,
				OwnerPrompt: "routines/prompts/weekly-ci-workflows-scan.md",
			},
			wantAll: []string{"scan:ci", "直近 28 日で 0 件", "weekly-ci-workflows-scan.md"},
		},
		{
			name: "pr creation reads the queue length off the report",
			alert: metrics.Alert{
				Kind: metrics.AlertPRCreatedRate, Scope: "scan:a", Value: 0.7, Threshold: 0.8,
				Observed: 7, Sample: 10, OwnerPrompt: metrics.PRBotPrompt,
			},
			scans:   []metrics.ScanMetrics{{Scan: "scan:a", PRPending: 4}},
			wantAll: []string{"70%", "4 件滞留", "weekly-adopted-issue-pr-bot.md"},
		},
		{
			name: "merge rate points at the PR care bot",
			alert: metrics.Alert{
				Kind: metrics.AlertMergeRate, Scope: "scan:a", Value: 0.7, Threshold: 0.8,
				Observed: 7, Sample: 10, OwnerPrompt: metrics.PRCarePrompt,
			},
			wantAll: []string{"マージ率 70%", "weekly-pr-care-bot.md"},
		},
		{
			name: "triage backlog is on the human, so it names no prompt",
			alert: metrics.Alert{
				Kind: metrics.AlertTriageBacklog, Value: 21.4, Threshold: 14, Observed: 512, Sample: 2,
			},
			wantAll:  []string{"21.4 日経過", "#512", "閾値 14 日"},
			wantNone: []string{"routines/prompts/"},
		},
		{
			name: "unmapped scan degrades to a bare sentence",
			alert: metrics.Alert{
				Kind: metrics.AlertAdoptedRate, Scope: "scan:unknown", Value: 0.1, Threshold: 0.6,
				Observed: 1, Sample: 10,
			},
			wantAll:  []string{"scan:unknown", "10%"},
			wantNone: []string{"→ ``"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := metrics.Report{Scans: tt.scans, Alerts: []metrics.Alert{tt.alert}}
			got := Alerts(&report)
			if !strings.HasPrefix(got, "> [!WARNING]\n") {
				t.Errorf("block does not open with a GitHub warning callout:\n%s", got)
			}
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("block is missing %q:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.wantNone {
				if strings.Contains(got, unwanted) {
					t.Errorf("block unexpectedly contains %q:\n%s", unwanted, got)
				}
			}
		})
	}
}

func TestRateCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rate *float64
		num  int
		den  int
		prec int
		want string
	}{
		{name: "whole percent", rate: new(0.8235), num: 75, den: 91, want: "82% (75/91)"},
		{name: "one decimal", rate: new(0.0769), num: 1, den: 13, prec: 1, want: "7.7% (1/13)"},
		{name: "sample too small", num: 3, den: 3, want: "— (n=3)"},
		{name: "no sample at all", want: "—"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rateCell(tt.rate, tt.num, tt.den, tt.prec); got != tt.want {
				t.Errorf("rateCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDaysCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days *float64
		want string
	}{
		{name: "absent", want: "—"},
		{name: "whole days drop the decimal", days: new(3.0), want: "3"},
		{name: "fractional days keep one decimal", days: new(26.6), want: "26.6"},
		{name: "zero", days: new(0.0), want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := daysCell(tt.days); got != tt.want {
				t.Errorf("daysCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFlowEmbedsParsableJSON(t *testing.T) {
	t.Parallel()
	report := metrics.Report{
		GeneratedAt: "2026-08-16T00:00:00Z",
		Since:       "2026-06-28",
		WindowStart: "2026-06-28",
		WindowDays:  90,
		MinSample:   5,
		Scans:       []metrics.ScanMetrics{{Scan: "scan:a", Opened: 9, Adopted: 8, AdoptedRate: new(0.8889)}},
		Months:      []metrics.MonthMetrics{{Month: "2026-07", Opened: 9, Adopted: 8}},
		Alerts:      []metrics.Alert{},
	}
	flow, flowErr := Flow(&report)
	if flowErr != nil {
		t.Fatalf("Flow: %v", flowErr)
	}
	if !strings.Contains(flow, JSONBlockMarker) {
		t.Errorf("flow section carries no JSON marker:\n%s", flow)
	}

	_, block, found := strings.Cut(flow, "```json\n")
	if !found {
		t.Fatalf("no fenced JSON block:\n%s", flow)
	}
	block, _, found = strings.Cut(block, "\n```")
	if !found {
		t.Fatalf("fenced JSON block is not closed:\n%s", flow)
	}
	var parsed metrics.Report
	if unmarshalErr := json.Unmarshal([]byte(block), &parsed); unmarshalErr != nil {
		t.Fatalf("embedded JSON does not round-trip: %v", unmarshalErr)
	}
	if len(parsed.Scans) != 1 || parsed.Scans[0].Scan != "scan:a" {
		t.Errorf("round-tripped scans = %+v, want scan:a", parsed.Scans)
	}
}

// TestFlowRendersWithoutData keeps the section printable when `gh` returned
// nothing at all, so the digest never loses a heading.
func TestFlowRendersWithoutData(t *testing.T) {
	t.Parallel()
	flow, flowErr := Flow(&metrics.Report{})
	if flowErr != nil {
		t.Fatalf("Flow: %v", flowErr)
	}
	for _, want := range []string{"## フロー指標", "### スキャン品質", "**合計**", "なし"} {
		if !strings.Contains(flow, want) {
			t.Errorf("empty report is missing %q:\n%s", want, flow)
		}
	}
}
