// Package model normalises the JSON that `gh issue list` / `gh pr list` emit
// into the pipeline's own view: scan issues carrying exactly one triage state,
// and auto PRs joined back to the issue they implement.
//
// Parsing is lenient. Unknown fields are ignored and records that cannot be
// normalised are dropped with a warning instead of failing the run, so a schema
// change on the gh side never blanks out the whole digest. Keep this the only
// package that knows the shape of the gh JSON.
package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TriageState is the human decision recorded on a scan issue through labels.
type TriageState string

const (
	// TriageAdopted marks a proposal accepted for implementation.
	TriageAdopted TriageState = "adopted"
	// TriageRejected marks a proposal dropped. It wins over TriageAdopted when
	// both labels are present: "adopted, implemented, then thrown away" is a
	// rejection, and the most expensive kind.
	TriageRejected TriageState = "rejected"
	// TriageUntriaged is an open scan issue with no decision yet.
	TriageUntriaged TriageState = "untriaged"
	// TriageUntrackedClose is a scan issue closed without any triage label,
	// i.e. the label discipline the metrics depend on broke down.
	TriageUntrackedClose TriageState = "untracked_close"
)

// Labels the pipeline assigns. ScanPrefix marks which scan opened the issue.
const (
	ScanPrefix     = "scan:"
	AdoptedLabel   = "adopted"
	RejectedLabel  = "rejected"
	PRCreatedLabel = "pr-created"
)

// autoBranchPrefix is the branch namespace the PR Bot works in; issueBranch
// additionally carries the issue number the branch implements.
const autoBranchPrefix = "auto/"

var issueBranch = regexp.MustCompile(`^auto/issue-(\d+)-`)

// Issue is a normalised GitHub issue. Build one with NewIssue: the canonical
// scan label and the triage state are derived there and never re-derived.
type Issue struct {
	number    int
	title     string
	scans     []string
	triage    TriageState
	adopted   bool
	rejected  bool
	prLabel   bool
	createdAt time.Time
	closedAt  time.Time
}

// NewIssue normalises one issue. labels may arrive in any order; the canonical
// scan label is the lexicographically first `scan:` label so that an issue
// carrying several of them still lands in exactly one bucket. A zero closedAt
// means the issue is still open.
func NewIssue(number int, title string, labels []string, createdAt, closedAt time.Time) (Issue, error) {
	if number <= 0 {
		return Issue{}, fmt.Errorf("issue number must be positive, got %d", number)
	}
	if createdAt.IsZero() {
		return Issue{}, fmt.Errorf("issue #%d has no createdAt", number)
	}
	if !closedAt.IsZero() && closedAt.Before(createdAt) {
		return Issue{}, fmt.Errorf("issue #%d was closed before it was created", number)
	}

	i := Issue{
		number:    number,
		title:     title,
		createdAt: createdAt.UTC(),
	}
	if !closedAt.IsZero() {
		i.closedAt = closedAt.UTC()
	}
	for _, label := range labels {
		switch {
		case strings.HasPrefix(label, ScanPrefix):
			i.scans = append(i.scans, label)
		case label == AdoptedLabel:
			i.adopted = true
		case label == RejectedLabel:
			i.rejected = true
		case label == PRCreatedLabel:
			i.prLabel = true
		}
	}
	slices.Sort(i.scans)
	i.triage = triageOf(i.adopted, i.rejected, !i.closedAt.IsZero())
	return i, nil
}

func triageOf(adopted, rejected, closed bool) TriageState {
	switch {
	case rejected:
		return TriageRejected
	case adopted:
		return TriageAdopted
	case closed:
		return TriageUntrackedClose
	default:
		return TriageUntriaged
	}
}

// Number returns the issue number.
func (i *Issue) Number() int { return i.number }

// Title returns the issue title.
func (i *Issue) Title() string { return i.title }

// Scan returns the canonical `scan:` label, or "" for a non-scan issue.
func (i *Issue) Scan() string {
	if len(i.scans) == 0 {
		return ""
	}
	return i.scans[0]
}

// IsScan reports whether the issue was opened by a scan routine.
func (i *Issue) IsScan() bool { return len(i.scans) > 0 }

// HasDuplicateScanLabels reports whether more than one `scan:` label is set,
// which means the issue is attributed to a single scan only by convention.
func (i *Issue) HasDuplicateScanLabels() bool { return len(i.scans) > 1 }

// Triage returns the collapsed triage state.
func (i *Issue) Triage() TriageState { return i.triage }

// HasBothTriageLabels reports whether adopted and rejected are both present.
func (i *Issue) HasBothTriageLabels() bool { return i.adopted && i.rejected }

// HasPRLabel reports whether the PR Bot stamped `pr-created`.
func (i *Issue) HasPRLabel() bool { return i.prLabel }

// CreatedAt returns the creation time in UTC.
func (i *Issue) CreatedAt() time.Time { return i.createdAt }

// ClosedAt returns the close time in UTC, or the zero time while open.
func (i *Issue) ClosedAt() time.Time { return i.closedAt }

// IsClosed reports whether the issue is closed.
func (i *Issue) IsClosed() bool { return !i.closedAt.IsZero() }

// PullRequest is a normalised GitHub pull request. Build one with
// NewPullRequest, which derives the issue join from the head branch name.
type PullRequest struct {
	number      int
	title       string
	headRef     string
	issueNumber int
	createdAt   time.Time
	closedAt    time.Time
	mergedAt    time.Time
}

// NewPullRequest normalises one PR. Zero closedAt / mergedAt mean "still open"
// and "never merged". The issue join comes from `auto/issue-<n>-…` head
// branches; any other branch yields IssueNumber() == 0.
func NewPullRequest(number int, title, headRef string, createdAt, closedAt, mergedAt time.Time) (PullRequest, error) {
	if number <= 0 {
		return PullRequest{}, fmt.Errorf("pr number must be positive, got %d", number)
	}
	if createdAt.IsZero() {
		return PullRequest{}, fmt.Errorf("pr #%d has no createdAt", number)
	}
	if !mergedAt.IsZero() && mergedAt.Before(createdAt) {
		return PullRequest{}, fmt.Errorf("pr #%d was merged before it was created", number)
	}

	p := PullRequest{
		number:    number,
		title:     title,
		headRef:   headRef,
		createdAt: createdAt.UTC(),
	}
	if !closedAt.IsZero() {
		p.closedAt = closedAt.UTC()
	}
	if !mergedAt.IsZero() {
		p.mergedAt = mergedAt.UTC()
	}
	if m := issueBranch.FindStringSubmatch(headRef); m != nil {
		// The regexp already constrained the group to digits, so the only way
		// this can fail is an overflowing number; treat that as "no join".
		if n, atoiErr := strconv.Atoi(m[1]); atoiErr == nil {
			p.issueNumber = n
		}
	}
	return p, nil
}

// Number returns the PR number.
func (p *PullRequest) Number() int { return p.number }

// Title returns the PR title.
func (p *PullRequest) Title() string { return p.title }

// HeadRef returns the head branch name.
func (p *PullRequest) HeadRef() string { return p.headRef }

// IsAuto reports whether the PR came from the pipeline's `auto/` namespace.
func (p *PullRequest) IsAuto() bool { return strings.HasPrefix(p.headRef, autoBranchPrefix) }

// IssueNumber returns the joined issue number, or 0 when the branch carries none.
func (p *PullRequest) IssueNumber() int { return p.issueNumber }

// IsMeta reports whether this is an `auto/` PR that implements no issue (the
// meta loop's own PRs); those are accounted separately from the scan flow.
func (p *PullRequest) IsMeta() bool { return p.IsAuto() && p.issueNumber == 0 }

// CreatedAt returns the creation time in UTC.
func (p *PullRequest) CreatedAt() time.Time { return p.createdAt }

// MergedAt returns the merge time in UTC, or the zero time when never merged.
func (p *PullRequest) MergedAt() time.Time { return p.mergedAt }

// IsMerged reports whether the PR was merged.
func (p *PullRequest) IsMerged() bool { return !p.mergedAt.IsZero() }

// IsClosedUnmerged reports whether the PR was closed without being merged.
func (p *PullRequest) IsClosedUnmerged() bool { return p.mergedAt.IsZero() && !p.closedAt.IsZero() }

// IsOpen reports whether the PR is still open.
func (p *PullRequest) IsOpen() bool { return p.mergedAt.IsZero() && p.closedAt.IsZero() }

// Dataset is the normalised pair of exports. Parse is its only constructor, so
// every Issue and PullRequest inside has already passed NewIssue / NewPullRequest.
type Dataset struct {
	issues   []Issue
	prs      []PullRequest
	warnings []string
}

// Issues returns a copy of the normalised issues.
func (d *Dataset) Issues() []Issue { return slices.Clone(d.issues) }

// PullRequests returns a copy of the normalised pull requests.
func (d *Dataset) PullRequests() []PullRequest { return slices.Clone(d.prs) }

// Warnings returns the records that were skipped during normalisation.
func (d *Dataset) Warnings() []string { return slices.Clone(d.warnings) }

type rawLabel struct {
	Name string `json:"name"`
}

type rawIssue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Labels    []rawLabel `json:"labels"`
	CreatedAt string     `json:"createdAt"`
	ClosedAt  string     `json:"closedAt"`
}

type rawPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	CreatedAt   string `json:"createdAt"`
	ClosedAt    string `json:"closedAt"`
	MergedAt    string `json:"mergedAt"`
}

// Parse normalises both gh exports. It fails only when the input is not a JSON
// array; individual records that cannot be normalised are skipped and reported
// through Dataset.Warnings. Empty input is treated as an empty array so that a
// failed `gh` call degrades to "no data" rather than to a crash.
func Parse(issuesJSON, prsJSON []byte) (Dataset, error) {
	var rawIssues []rawIssue
	if unmarshalErr := unmarshalArray(issuesJSON, &rawIssues); unmarshalErr != nil {
		return Dataset{}, fmt.Errorf("parse issues: %w", unmarshalErr)
	}
	var rawPRs []rawPR
	if unmarshalErr := unmarshalArray(prsJSON, &rawPRs); unmarshalErr != nil {
		return Dataset{}, fmt.Errorf("parse prs: %w", unmarshalErr)
	}

	d := Dataset{}
	for _, r := range rawIssues {
		createdAt, closedAt, timeErr := parseTimes(r.CreatedAt, r.ClosedAt)
		if timeErr != nil {
			d.warnings = append(d.warnings, fmt.Sprintf("issue #%d: %v", r.Number, timeErr))
			continue
		}
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		issue, newErr := NewIssue(r.Number, r.Title, labels, createdAt, closedAt)
		if newErr != nil {
			d.warnings = append(d.warnings, newErr.Error())
			continue
		}
		d.issues = append(d.issues, issue)
	}
	for _, r := range rawPRs {
		createdAt, closedAt, timeErr := parseTimes(r.CreatedAt, r.ClosedAt)
		if timeErr != nil {
			d.warnings = append(d.warnings, fmt.Sprintf("pr #%d: %v", r.Number, timeErr))
			continue
		}
		mergedAt, mergedErr := parseTime(r.MergedAt)
		if mergedErr != nil {
			d.warnings = append(d.warnings, fmt.Sprintf("pr #%d: %v", r.Number, mergedErr))
			continue
		}
		pr, newErr := NewPullRequest(r.Number, r.Title, r.HeadRefName, createdAt, closedAt, mergedAt)
		if newErr != nil {
			d.warnings = append(d.warnings, newErr.Error())
			continue
		}
		d.prs = append(d.prs, pr)
	}
	slices.SortFunc(d.issues, func(a, b Issue) int { return a.number - b.number })
	slices.SortFunc(d.prs, func(a, b PullRequest) int { return a.number - b.number })
	return d, nil
}

func unmarshalArray(data []byte, target any) error {
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	return json.Unmarshal(data, target)
}

func parseTimes(created, closed string) (createdAt, closedAt time.Time, err error) {
	createdAt, createdErr := parseTime(created)
	if createdErr != nil {
		return time.Time{}, time.Time{}, createdErr
	}
	closedAt, closedErr := parseTime(closed)
	if closedErr != nil {
		return time.Time{}, time.Time{}, closedErr
	}
	return createdAt, closedAt, nil
}

// parseTime accepts RFC3339, the empty string (gh emits null for absent
// timestamps) and Go's zero time, which some gh versions emit instead of null.
func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, parseErr := time.Parse(time.RFC3339, value)
	if parseErr != nil {
		return time.Time{}, fmt.Errorf("unparsable timestamp %q: %w", value, parseErr)
	}
	if t.IsZero() {
		return time.Time{}, nil
	}
	return t.UTC(), nil
}
