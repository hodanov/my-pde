package model

import (
	"strings"
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	if value == "" {
		return time.Time{}
	}
	parsed, parseErr := time.Parse(time.RFC3339, value)
	if parseErr != nil {
		t.Fatalf("parse %q: %v", value, parseErr)
	}
	return parsed
}

func TestNewIssueDerivesTriage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		labels     []string
		closed     string
		wantTriage TriageState
		wantScan   string
		wantDupe   bool
		wantBoth   bool
	}{
		{
			name:       "adopted and open",
			labels:     []string{"scan:nvim", "adopted"},
			wantTriage: TriageAdopted,
			wantScan:   "scan:nvim",
		},
		{
			name:       "rejected on close",
			labels:     []string{"scan:scripts", "rejected"},
			closed:     "2026-07-02T00:00:00Z",
			wantTriage: TriageRejected,
			wantScan:   "scan:scripts",
		},
		{
			// Adopted, implemented, then thrown away: the expensive failure.
			name:       "adopted and rejected collapses to rejected",
			labels:     []string{"scan:nvim", "adopted", "rejected", "pr-created"},
			closed:     "2026-07-02T00:00:00Z",
			wantTriage: TriageRejected,
			wantScan:   "scan:nvim",
			wantBoth:   true,
		},
		{
			name:       "open without a decision",
			labels:     []string{"scan:ci"},
			wantTriage: TriageUntriaged,
			wantScan:   "scan:ci",
		},
		{
			name:       "closed without a decision",
			labels:     []string{"scan:ci"},
			closed:     "2026-07-02T00:00:00Z",
			wantTriage: TriageUntrackedClose,
			wantScan:   "scan:ci",
		},
		{
			name:       "several scan labels pick the first in lexicographic order",
			labels:     []string{"scan:nvim", "adopted", "scan:ci"},
			wantTriage: TriageAdopted,
			wantScan:   "scan:ci",
			wantDupe:   true,
		},
		{
			name:       "no scan label",
			labels:     []string{"enhancement", "rejected"},
			closed:     "2026-07-02T00:00:00Z",
			wantTriage: TriageRejected,
			wantScan:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issue, newErr := NewIssue(42, "title", tt.labels, mustTime(t, "2026-07-01T00:00:00Z"), mustTime(t, tt.closed))
			if newErr != nil {
				t.Fatalf("NewIssue: %v", newErr)
			}
			if got := issue.Triage(); got != tt.wantTriage {
				t.Errorf("Triage() = %q, want %q", got, tt.wantTriage)
			}
			if got := issue.Scan(); got != tt.wantScan {
				t.Errorf("Scan() = %q, want %q", got, tt.wantScan)
			}
			if got := issue.IsScan(); got != (tt.wantScan != "") {
				t.Errorf("IsScan() = %v, want %v", got, tt.wantScan != "")
			}
			if got := issue.HasDuplicateScanLabels(); got != tt.wantDupe {
				t.Errorf("HasDuplicateScanLabels() = %v, want %v", got, tt.wantDupe)
			}
			if got := issue.HasBothTriageLabels(); got != tt.wantBoth {
				t.Errorf("HasBothTriageLabels() = %v, want %v", got, tt.wantBoth)
			}
			if got := issue.IsClosed(); got != (tt.closed != "") {
				t.Errorf("IsClosed() = %v, want %v", got, tt.closed != "")
			}
		})
	}
}

func TestNewIssueRejectsImpossibleRecords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		number  int
		created string
		closed  string
	}{
		{name: "zero number", number: 0, created: "2026-07-01T00:00:00Z"},
		{name: "negative number", number: -1, created: "2026-07-01T00:00:00Z"},
		{name: "no creation time", number: 1, created: ""},
		{name: "closed before created", number: 1, created: "2026-07-02T00:00:00Z", closed: "2026-07-01T00:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, newErr := NewIssue(tt.number, "t", nil, mustTime(t, tt.created), mustTime(t, tt.closed)); newErr == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestNewPullRequestJoinsIssues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		headRef   string
		wantIssue int
		wantAuto  bool
		wantMeta  bool
	}{
		{name: "issue branch", headRef: "auto/issue-517-fix-toggle", wantIssue: 517, wantAuto: true},
		{name: "multi digit issue", headRef: "auto/issue-1234-something", wantIssue: 1234, wantAuto: true},
		{name: "meta branch", headRef: "auto/routine-improve-20260801", wantAuto: true, wantMeta: true},
		{name: "missing trailing slug", headRef: "auto/issue-517", wantAuto: true, wantMeta: true},
		{name: "hand written branch", headRef: "feature/manual"},
		{name: "issue prefix outside auto", headRef: "issue-517-nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pr, newErr := NewPullRequest(7, "title", tt.headRef, mustTime(t, "2026-07-01T00:00:00Z"), time.Time{}, time.Time{})
			if newErr != nil {
				t.Fatalf("NewPullRequest: %v", newErr)
			}
			if got := pr.IssueNumber(); got != tt.wantIssue {
				t.Errorf("IssueNumber() = %d, want %d", got, tt.wantIssue)
			}
			if got := pr.IsAuto(); got != tt.wantAuto {
				t.Errorf("IsAuto() = %v, want %v", got, tt.wantAuto)
			}
			if got := pr.IsMeta(); got != tt.wantMeta {
				t.Errorf("IsMeta() = %v, want %v", got, tt.wantMeta)
			}
		})
	}
}

func TestPullRequestState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		closed           string
		merged           string
		wantMerged       bool
		wantClosedUnmerg bool
		wantOpen         bool
	}{
		{name: "open", wantOpen: true},
		{
			name:       "merged",
			closed:     "2026-07-02T00:00:00Z",
			merged:     "2026-07-02T00:00:00Z",
			wantMerged: true,
		},
		{
			name:             "closed without merging",
			closed:           "2026-07-02T00:00:00Z",
			wantClosedUnmerg: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pr, newErr := NewPullRequest(7, "t", "auto/issue-1-x",
				mustTime(t, "2026-07-01T00:00:00Z"), mustTime(t, tt.closed), mustTime(t, tt.merged))
			if newErr != nil {
				t.Fatalf("NewPullRequest: %v", newErr)
			}
			if got := pr.IsMerged(); got != tt.wantMerged {
				t.Errorf("IsMerged() = %v, want %v", got, tt.wantMerged)
			}
			if got := pr.IsClosedUnmerged(); got != tt.wantClosedUnmerg {
				t.Errorf("IsClosedUnmerged() = %v, want %v", got, tt.wantClosedUnmerg)
			}
			if got := pr.IsOpen(); got != tt.wantOpen {
				t.Errorf("IsOpen() = %v, want %v", got, tt.wantOpen)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		issues       string
		prs          string
		wantIssues   int
		wantPRs      int
		wantWarnings int
		wantErr      bool
	}{
		{
			name:       "gh output",
			issues:     `[{"number":1,"title":"a","state":"OPEN","labels":[{"name":"scan:ci"}],"createdAt":"2026-07-01T00:00:00Z","closedAt":null}]`,
			prs:        `[{"number":2,"title":"b","state":"OPEN","headRefName":"auto/issue-1-a","createdAt":"2026-07-02T00:00:00Z","closedAt":null,"mergedAt":null}]`,
			wantIssues: 1,
			wantPRs:    1,
		},
		{
			name:       "empty arrays",
			issues:     `[]`,
			prs:        `[]`,
			wantIssues: 0,
			wantPRs:    0,
		},
		{
			name:       "empty input is treated as no data",
			issues:     "",
			prs:        "   ",
			wantIssues: 0,
			wantPRs:    0,
		},
		{
			name:       "unknown fields are ignored",
			issues:     `[{"number":1,"createdAt":"2026-07-01T00:00:00Z","author":{"login":"x"},"comments":7}]`,
			prs:        `[]`,
			wantIssues: 1,
		},
		{
			name:       "zero timestamps count as absent",
			issues:     `[{"number":1,"createdAt":"2026-07-01T00:00:00Z","closedAt":"0001-01-01T00:00:00Z"}]`,
			prs:        `[]`,
			wantIssues: 1,
		},
		{
			name:         "unparsable record is skipped, not fatal",
			issues:       `[{"number":1,"createdAt":"yesterday"},{"number":2,"createdAt":"2026-07-01T00:00:00Z"}]`,
			prs:          `[]`,
			wantIssues:   1,
			wantWarnings: 1,
		},
		{
			name:         "invalid record is skipped, not fatal",
			issues:       `[{"number":0,"createdAt":"2026-07-01T00:00:00Z"}]`,
			prs:          `[]`,
			wantIssues:   0,
			wantWarnings: 1,
		},
		{name: "not an array", issues: `{"number":1}`, prs: `[]`, wantErr: true},
		{name: "broken pr json", issues: `[]`, prs: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dataset, parseErr := Parse([]byte(tt.issues), []byte(tt.prs))
			if tt.wantErr {
				if parseErr == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("Parse: %v", parseErr)
			}
			if got := len(dataset.Issues()); got != tt.wantIssues {
				t.Errorf("issues = %d, want %d", got, tt.wantIssues)
			}
			if got := len(dataset.PullRequests()); got != tt.wantPRs {
				t.Errorf("prs = %d, want %d", got, tt.wantPRs)
			}
			if got := len(dataset.Warnings()); got != tt.wantWarnings {
				t.Errorf("warnings = %d (%s), want %d", got, strings.Join(dataset.Warnings(), "; "), tt.wantWarnings)
			}
		})
	}
}

// TestParseSortsByNumber keeps the aggregation independent of the order gh
// happens to return records in.
func TestParseSortsByNumber(t *testing.T) {
	t.Parallel()
	dataset, parseErr := Parse([]byte(`[
		{"number":9,"createdAt":"2026-07-03T00:00:00Z"},
		{"number":3,"createdAt":"2026-07-01T00:00:00Z"},
		{"number":5,"createdAt":"2026-07-02T00:00:00Z"}
	]`), []byte(`[]`))
	if parseErr != nil {
		t.Fatalf("Parse: %v", parseErr)
	}
	issues := dataset.Issues()
	want := []int{3, 5, 9}
	for i := range issues {
		if issues[i].Number() != want[i] {
			t.Fatalf("issue order = %d at %d, want %d", issues[i].Number(), i, want[i])
		}
	}
}

// TestIssuesReturnsACopy guards the read-only contract: callers must not be
// able to reach into the dataset and mutate it.
func TestIssuesReturnsACopy(t *testing.T) {
	t.Parallel()
	dataset, parseErr := Parse([]byte(`[{"number":1,"createdAt":"2026-07-01T00:00:00Z","labels":[{"name":"scan:ci"}]}]`), nil)
	if parseErr != nil {
		t.Fatalf("Parse: %v", parseErr)
	}
	copied := dataset.Issues()
	copied[0] = Issue{}
	if got := dataset.Issues()[0].Number(); got != 1 {
		t.Errorf("dataset was mutated through Issues(): number = %d, want 1", got)
	}
}
