package model

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		payload            string
		wantScanLabel      string
		wantScanLabelCount int
		wantTriage         TriageState
		wantPRCreated      bool
		wantClosed         bool
		wantClosedAtZero   bool
	}{
		{
			name:               "adopted scan issue with a PR",
			payload:            `[{"number":1,"state":"CLOSED","labels":[{"name":"scan:nvim"},{"name":"adopted"},{"name":"pr-created"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":"2026-08-02T00:00:00Z"}]`,
			wantScanLabel:      "scan:nvim",
			wantScanLabelCount: 1,
			wantTriage:         Adopted,
			wantPRCreated:      true,
			wantClosed:         true,
		},
		{
			name:               "rejected wins over adopted",
			payload:            `[{"number":2,"state":"CLOSED","labels":[{"name":"scan:scripts"},{"name":"adopted"},{"name":"pr-created"},{"name":"rejected"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":"2026-08-02T00:00:00Z"}]`,
			wantScanLabel:      "scan:scripts",
			wantScanLabelCount: 1,
			wantTriage:         Rejected,
			wantPRCreated:      true,
			wantClosed:         true,
		},
		{
			name:               "open with neither label is untriaged",
			payload:            `[{"number":3,"state":"OPEN","labels":[{"name":"scan:ci"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":null}]`,
			wantScanLabel:      "scan:ci",
			wantScanLabelCount: 1,
			wantTriage:         Untriaged,
			wantClosedAtZero:   true,
		},
		{
			name:               "closed with neither label is an untracked close",
			payload:            `[{"number":4,"state":"CLOSED","labels":[{"name":"scan:ci"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":"2026-08-02T00:00:00Z"}]`,
			wantScanLabel:      "scan:ci",
			wantScanLabelCount: 1,
			wantTriage:         UntrackedClose,
			wantClosed:         true,
		},
		{
			name:               "two scan labels pick the lexicographically first",
			payload:            `[{"number":5,"state":"OPEN","labels":[{"name":"scan:nvim"},{"name":"scan:ci"},{"name":"adopted"}],"createdAt":"2026-08-01T00:00:00Z"}]`,
			wantScanLabel:      "scan:ci",
			wantScanLabelCount: 2,
			wantTriage:         Adopted,
			wantClosedAtZero:   true,
		},
		{
			name:               "no scan label leaves the label empty",
			payload:            `[{"number":6,"state":"CLOSED","labels":[{"name":"enhancement"},{"name":"adopted"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":"2026-08-02T00:00:00Z"}]`,
			wantScanLabel:      "",
			wantScanLabelCount: 0,
			wantTriage:         Adopted,
			wantClosed:         true,
		},
		{
			name:               "the zero timestamp gh emits counts as absent",
			payload:            `[{"number":7,"state":"OPEN","labels":[{"name":"scan:nvim"}],"createdAt":"2026-08-01T00:00:00Z","closedAt":"0001-01-01T00:00:00Z"}]`,
			wantScanLabel:      "scan:nvim",
			wantScanLabelCount: 1,
			wantTriage:         Untriaged,
			wantClosedAtZero:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			issues, decodeErr := DecodeIssues(strings.NewReader(tt.payload))
			if decodeErr != nil {
				t.Fatalf("DecodeIssues() error = %v", decodeErr)
			}
			if len(issues) != 1 {
				t.Fatalf("DecodeIssues() returned %d issues, want 1", len(issues))
			}
			got := issues[0]
			if got.ScanLabel != tt.wantScanLabel {
				t.Errorf("ScanLabel = %q, want %q", got.ScanLabel, tt.wantScanLabel)
			}
			if got.ScanLabelCount != tt.wantScanLabelCount {
				t.Errorf("ScanLabelCount = %d, want %d", got.ScanLabelCount, tt.wantScanLabelCount)
			}
			if got.Triage != tt.wantTriage {
				t.Errorf("Triage = %q, want %q", got.Triage, tt.wantTriage)
			}
			if got.PRCreated != tt.wantPRCreated {
				t.Errorf("PRCreated = %v, want %v", got.PRCreated, tt.wantPRCreated)
			}
			if got.Closed != tt.wantClosed {
				t.Errorf("Closed = %v, want %v", got.Closed, tt.wantClosed)
			}
			if got.ClosedAt.IsZero() != tt.wantClosedAtZero {
				t.Errorf("ClosedAt.IsZero() = %v, want %v", got.ClosedAt.IsZero(), tt.wantClosedAtZero)
			}
		})
	}
}

func TestDecodeIssuesRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	if _, decodeErr := DecodeIssues(strings.NewReader(`{"not":"an array"}`)); decodeErr == nil {
		t.Fatal("DecodeIssues() error = nil, want an error for a malformed payload")
	}
}

func TestDecodePullRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		payload         string
		wantIssueNumber int
		wantMerged      bool
		wantOpen        bool
		wantAuto        bool
	}{
		{
			name:            "merged auto PR joins its issue",
			payload:         `[{"number":10,"state":"MERGED","headRefName":"auto/issue-201-ship","createdAt":"2026-08-02T00:00:00Z","closedAt":"2026-08-03T00:00:00Z","mergedAt":"2026-08-03T00:00:00Z"}]`,
			wantIssueNumber: 201,
			wantMerged:      true,
			wantAuto:        true,
		},
		{
			name:            "closed without a merge",
			payload:         `[{"number":11,"state":"CLOSED","headRefName":"auto/issue-304-tool","createdAt":"2026-07-26T00:00:00Z","closedAt":"2026-07-30T00:00:00Z","mergedAt":null}]`,
			wantIssueNumber: 304,
			wantAuto:        true,
		},
		{
			name:     "meta branch carries no issue number",
			payload:  `[{"number":12,"state":"MERGED","headRefName":"auto/routine-improve-20260802","createdAt":"2026-08-02T00:00:00Z","mergedAt":"2026-08-02T00:00:00Z"}]`,
			wantAuto: true, wantMerged: true,
		},
		{
			name:    "non-auto branch is not part of the pipeline",
			payload: `[{"number":13,"state":"MERGED","headRefName":"chore/bump-tool-versions","createdAt":"2026-08-03T00:00:00Z","mergedAt":"2026-08-03T00:00:00Z"}]`,
			// A dependency bump merges like any other PR but must not be counted.
			wantMerged: true,
		},
		{
			name:     "open PR",
			payload:  `[{"number":14,"state":"OPEN","headRefName":"auto/issue-405-workflow","createdAt":"2026-08-09T00:00:00Z","closedAt":null,"mergedAt":null}]`,
			wantOpen: true, wantIssueNumber: 405, wantAuto: true,
		},
		{
			name:            "branch with no slug still joins",
			payload:         `[{"number":15,"state":"MERGED","headRefName":"auto/issue-77","createdAt":"2026-08-09T00:00:00Z","mergedAt":"2026-08-10T00:00:00Z"}]`,
			wantIssueNumber: 77,
			wantMerged:      true,
			wantAuto:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prs, decodeErr := DecodePullRequests(strings.NewReader(tt.payload))
			if decodeErr != nil {
				t.Fatalf("DecodePullRequests() error = %v", decodeErr)
			}
			if len(prs) != 1 {
				t.Fatalf("DecodePullRequests() returned %d PRs, want 1", len(prs))
			}
			got := prs[0]
			if got.IssueNumber != tt.wantIssueNumber {
				t.Errorf("IssueNumber = %d, want %d", got.IssueNumber, tt.wantIssueNumber)
			}
			if got.Merged != tt.wantMerged {
				t.Errorf("Merged = %v, want %v", got.Merged, tt.wantMerged)
			}
			if got.Open != tt.wantOpen {
				t.Errorf("Open = %v, want %v", got.Open, tt.wantOpen)
			}
			if got.IsAuto() != tt.wantAuto {
				t.Errorf("IsAuto() = %v, want %v", got.IsAuto(), tt.wantAuto)
			}
		})
	}
}

func TestDecodeTimestampsAreUTC(t *testing.T) {
	t.Parallel()

	issues, decodeErr := DecodeIssues(strings.NewReader(`[{"number":1,"state":"OPEN","labels":[],"createdAt":"2026-08-01T09:00:00+09:00"}]`))
	if decodeErr != nil {
		t.Fatalf("DecodeIssues() error = %v", decodeErr)
	}
	want := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if !issues[0].CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", issues[0].CreatedAt, want)
	}
	if issues[0].CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", issues[0].CreatedAt.Location())
	}
}
