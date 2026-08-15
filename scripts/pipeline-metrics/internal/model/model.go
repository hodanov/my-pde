// Package model decodes `gh issue list` / `gh pr list` JSON payloads into the
// normalised shapes the metrics layer aggregates over.
//
// Decoding ignores fields it does not know (gh gains columns over time) but is
// strict about malformed JSON: the payload comes from a single gh call, so a
// parse error means the fetch itself went wrong and silently reporting zeros
// would hide that.
package model

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Label names and branch prefixes that encode the pipeline's conventions.
const (
	ScanLabelPrefix  = "scan:"
	AdoptedLabel     = "adopted"
	RejectedLabel    = "rejected"
	PRCreatedLabel   = "pr-created"
	AutoBranchPrefix = "auto/"
)

// TriageState is the human decision recorded on a scan issue through labels.
type TriageState string

// The triage states a scan issue can be in.
const (
	// Untriaged is open with neither adopted nor rejected.
	Untriaged TriageState = "untriaged"
	// Adopted carries the adopted label and no rejected label.
	Adopted TriageState = "adopted"
	// Rejected carries the rejected label, which wins over adopted: an issue
	// labelled both was adopted first and thrown away later.
	Rejected TriageState = "rejected"
	// UntrackedClose is closed without either label, i.e. the label convention
	// was not followed. Counting these keeps the metrics honest about drift.
	UntrackedClose TriageState = "untracked_close"
)

// Issue is a normalised GitHub issue.
type Issue struct {
	Number         int
	Title          string
	ScanLabel      string // primary scan:* label; empty when the issue is not a scan issue
	ScanLabelCount int    // how many scan:* labels the issue carried
	Triage         TriageState
	PRCreated      bool
	Closed         bool
	CreatedAt      time.Time
	ClosedAt       time.Time // zero when still open
}

// PullRequest is a normalised GitHub pull request.
type PullRequest struct {
	Number      int
	Title       string
	Branch      string
	IssueNumber int // issue encoded in an auto/issue-<N>-<slug> branch; 0 when absent
	CreatedAt   time.Time
	ClosedAt    time.Time // zero when still open
	MergedAt    time.Time // zero when never merged
	Merged      bool
	Open        bool
}

// IsAuto reports whether the PR came from the pipeline's auto/* branch space.
func (p *PullRequest) IsAuto() bool {
	return strings.HasPrefix(p.Branch, AutoBranchPrefix)
}

type rawLabel struct {
	Name string `json:"name"`
}

type rawIssue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Labels    []rawLabel `json:"labels"`
	CreatedAt *time.Time `json:"createdAt"`
	ClosedAt  *time.Time `json:"closedAt"`
}

type rawPullRequest struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	HeadRefName string     `json:"headRefName"`
	CreatedAt   *time.Time `json:"createdAt"`
	ClosedAt    *time.Time `json:"closedAt"`
	MergedAt    *time.Time `json:"mergedAt"`
}

// autoIssueRe extracts the issue number the PR Bot encodes into its branch
// names (auto/issue-<N>-<slug>), which is the only deterministic link between a
// scan issue and the PR that implements it.
var autoIssueRe = regexp.MustCompile(`^auto/issue-(\d+)(?:-|$)`)

// DecodeIssues reads a `gh issue list --json number,title,state,labels,createdAt,closedAt`
// payload.
func DecodeIssues(r io.Reader) ([]Issue, error) {
	var raws []rawIssue
	if decodeErr := json.NewDecoder(r).Decode(&raws); decodeErr != nil {
		return nil, fmt.Errorf("decode issues: %w", decodeErr)
	}
	issues := make([]Issue, 0, len(raws))
	for i := range raws {
		issues = append(issues, normaliseIssue(&raws[i]))
	}
	return issues, nil
}

// DecodePullRequests reads a `gh pr list --json number,title,state,headRefName,createdAt,closedAt,mergedAt`
// payload.
func DecodePullRequests(r io.Reader) ([]PullRequest, error) {
	var raws []rawPullRequest
	if decodeErr := json.NewDecoder(r).Decode(&raws); decodeErr != nil {
		return nil, fmt.Errorf("decode pull requests: %w", decodeErr)
	}
	prs := make([]PullRequest, 0, len(raws))
	for i := range raws {
		prs = append(prs, normalisePullRequest(&raws[i]))
	}
	return prs, nil
}

func normaliseIssue(raw *rawIssue) Issue {
	iss := Issue{
		Number:    raw.Number,
		Title:     raw.Title,
		Closed:    strings.EqualFold(raw.State, "closed"),
		CreatedAt: at(raw.CreatedAt),
		ClosedAt:  at(raw.ClosedAt),
	}

	var scanLabels []string
	hasAdopted, hasRejected := false, false
	for _, label := range raw.Labels {
		switch {
		case strings.HasPrefix(label.Name, ScanLabelPrefix):
			scanLabels = append(scanLabels, label.Name)
		case label.Name == AdoptedLabel:
			hasAdopted = true
		case label.Name == RejectedLabel:
			hasRejected = true
		case label.Name == PRCreatedLabel:
			iss.PRCreated = true
		}
	}
	// Multiple scan labels are a convention violation; pick the lexicographically
	// first one so the choice is stable, and let the metrics layer report it.
	slices.Sort(scanLabels)
	iss.ScanLabelCount = len(scanLabels)
	if len(scanLabels) > 0 {
		iss.ScanLabel = scanLabels[0]
	}

	switch {
	case hasRejected:
		iss.Triage = Rejected
	case hasAdopted:
		iss.Triage = Adopted
	case iss.Closed:
		iss.Triage = UntrackedClose
	default:
		iss.Triage = Untriaged
	}
	return iss
}

func normalisePullRequest(raw *rawPullRequest) PullRequest {
	pr := PullRequest{
		Number:    raw.Number,
		Title:     raw.Title,
		Branch:    raw.HeadRefName,
		CreatedAt: at(raw.CreatedAt),
		ClosedAt:  at(raw.ClosedAt),
		MergedAt:  at(raw.MergedAt),
		Open:      strings.EqualFold(raw.State, "open"),
	}
	pr.Merged = !pr.MergedAt.IsZero() || strings.EqualFold(raw.State, "merged")
	if m := autoIssueRe.FindStringSubmatch(pr.Branch); m != nil {
		// The regexp guarantees digits, so the only way this parse fails is an
		// issue number wider than int, which GitHub will not produce.
		var n int
		if _, scanErr := fmt.Sscanf(m[1], "%d", &n); scanErr == nil {
			pr.IssueNumber = n
		}
	}
	return pr
}

// at unwraps an optional timestamp. gh emits either null or the zero time for
// "not yet", and both must collapse to the same absent value.
func at(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}
