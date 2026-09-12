package permission

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"agent-stats/internal/parser"
)

// LocalRules is the allow list one repository's .claude/settings.local.json
// holds.
type LocalRules struct {
	Repo  string
	Allow []string
}

// Candidate is a rule that could be promoted into the global allow list,
// together with the evidence for and against it.
type Candidate struct {
	Rule     string `json:"rule"`
	Approved int    `json:"approved"`
	Rejected int    `json:"rejected"`
	Pending  int    `json:"pending"`
	// FamilyRejections counts every call of the same command family (Bash
	// leading command, WebFetch domain or MCP tool) rejected anywhere in the
	// scanned transcripts, including before the ledger existed.
	FamilyRejections int       `json:"family_rejections"`
	Sessions         int       `json:"sessions"`
	Repos            []string  `json:"repos,omitempty"`
	LocalRepos       []string  `json:"local_repos,omitempty"`
	LastSeen         time.Time `json:"last_seen,omitzero"`
	Examples         []string  `json:"examples,omitempty"`
}

// Report is the review input: rules Claude Code suggested, and rules drafted
// for prompts that came without a suggestion.
type Report struct {
	Candidates  []Candidate `json:"candidates"`
	Unsuggested []Candidate `json:"unsuggested"`
}

// Audit joins ledger entries with the transcript calls that resolve them.
type Audit struct {
	entries          []Entry
	byFile           map[string][]int
	matched          map[int]parser.ToolCall
	familyRejections map[string]int
	cwds             map[string]struct{}
}

// NewAudit starts an audit of the given ledger entries.
func NewAudit(entries []Entry) *Audit {
	a := &Audit{
		entries:          entries,
		byFile:           map[string][]int{},
		matched:          map[int]parser.ToolCall{},
		familyRejections: map[string]int{},
		cwds:             map[string]struct{}{},
	}
	for i := range entries {
		if file := entries[i].transcriptFile(); file != "" {
			a.byFile[file] = append(a.byFile[file], i)
		}
		if cwd := entries[i].Cwd; cwd != "" {
			a.cwds[cwd] = struct{}{}
		}
	}
	for _, indices := range a.byFile {
		slices.SortStableFunc(indices, func(x, y int) int {
			return entries[x].Timestamp.Compare(entries[y].Timestamp)
		})
	}
	return a
}

// AddTranscript takes what the audit needs from the transcript file at path:
// the calls that resolve ledger entries recorded against that file, rejected
// calls, and the directories sessions ran in.
func (a *Audit) AddTranscript(path string, idx parser.ToolCallIndex) {
	a.matchEntries(filepath.Clean(path), idx)
	for _, call := range idx {
		if call.Outcome == parser.OutcomeUserRejected {
			if family := callFamily(&call); family != "" {
				a.familyRejections[family]++
			}
		}
		if call.Cwd != "" {
			a.cwds[call.Cwd] = struct{}{}
		}
	}
}

// matchEntries pairs each entry with the latest unclaimed call of the same tool
// and input made before the prompt. The hook stamps entries to the second, so a
// call a little after the stamp still counts as before it.
func (a *Audit) matchEntries(path string, idx parser.ToolCallIndex) {
	const clockSlack = 5 * time.Second
	claimed := map[string]struct{}{}
	for _, i := range a.byFile[path] {
		e := &a.entries[i]
		bestID := ""
		for id, call := range idx {
			if _, taken := claimed[id]; taken || call.Name != e.ToolName {
				continue
			}
			if call.Timestamp.After(e.Timestamp.Add(clockSlack)) || !inputMatches(call.Input, e.ToolInput) {
				continue
			}
			if bestID == "" || call.Timestamp.After(idx[bestID].Timestamp) ||
				(call.Timestamp.Equal(idx[bestID].Timestamp) && id > bestID) {
				bestID = id
			}
		}
		if bestID != "" {
			claimed[bestID] = struct{}{}
			a.matched[i] = idx[bestID]
		}
	}
}

// Cwds returns every directory seen in the ledger or the transcripts, sorted.
func (a *Audit) Cwds() []string {
	return slices.Sorted(maps.Keys(a.cwds))
}

// Report aggregates the evidence per rule, leaving out rules in known. repoOf
// maps a working directory to its repository root, or "" when it has none.
func (a *Audit) Report(known RuleSet, locals []LocalRules, repoOf func(cwd string) string) Report {
	suggested := map[string]*tally{}
	unsuggested := map[string]*tally{}
	for i := range a.entries {
		e := &a.entries[i]
		call, found := a.matched[i]
		if !found {
			call = parser.ToolCall{Name: e.ToolName, Input: e.ToolInput, Outcome: parser.OutcomeNoResult}
		}
		rules, target := e.allowRules(), suggested
		if len(rules) == 0 {
			rules, target = []string{draftRule(&call)}, unsuggested
		}
		for _, rule := range rules {
			if known.Contains(rule) {
				continue
			}
			t := tallyFor(target, rule)
			t.count(call.Outcome)
			t.sessions[e.SessionID] = struct{}{}
			if repo := repoOf(e.Cwd); repo != "" {
				t.repos[repo] = struct{}{}
			}
			if e.Timestamp.After(t.c.LastSeen) {
				t.c.LastSeen = e.Timestamp
			}
			t.addExample(summarizeCall(&call))
		}
	}
	for _, l := range locals {
		for _, rule := range l.Allow {
			if known.Contains(rule) {
				continue
			}
			tallyFor(suggested, normalizeRule(rule)).locals[l.Repo] = struct{}{}
		}
	}
	return Report{
		Candidates:  a.candidates(suggested),
		Unsuggested: a.candidates(unsuggested),
	}
}

func (a *Audit) candidates(tallies map[string]*tally) []Candidate {
	out := make([]Candidate, 0, len(tallies))
	for _, t := range tallies {
		c := t.c
		c.Sessions = len(t.sessions)
		c.Repos = slices.Sorted(maps.Keys(t.repos))
		c.LocalRepos = slices.Sorted(maps.Keys(t.locals))
		if family := ruleFamily(c.Rule); family != "" {
			c.FamilyRejections = a.familyRejections[family]
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(x, y Candidate) int {
		return cmp.Or(
			cmp.Compare(y.Approved, x.Approved),
			cmp.Compare(len(y.LocalRepos), len(x.LocalRepos)),
			strings.Compare(x.Rule, y.Rule),
		)
	})
	return out
}

type tally struct {
	c        Candidate
	sessions map[string]struct{}
	repos    map[string]struct{}
	locals   map[string]struct{}
}

func tallyFor(tallies map[string]*tally, rule string) *tally {
	t, ok := tallies[rule]
	if !ok {
		t = &tally{
			c:        Candidate{Rule: rule},
			sessions: map[string]struct{}{},
			repos:    map[string]struct{}{},
			locals:   map[string]struct{}{},
		}
		tallies[rule] = t
	}
	return t
}

func (t *tally) count(outcome parser.Outcome) {
	switch outcome {
	case parser.OutcomeExecuted:
		t.c.Approved++
	case parser.OutcomeUserRejected, parser.OutcomeDenied:
		t.c.Rejected++
	default:
		t.c.Pending++
	}
}

func (t *tally) addExample(example string) {
	const maxExamples = 3
	if example == "" || len(t.c.Examples) >= maxExamples || slices.Contains(t.c.Examples, example) {
		return
	}
	t.c.Examples = append(t.c.Examples, example)
}
