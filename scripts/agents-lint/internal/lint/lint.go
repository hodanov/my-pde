// Package lint statically validates the AI agent skill and subagent
// definitions under ai-agents/ read-only: frontmatter schema, naming
// conventions, name/dir agreement, uniqueness and referential integrity
// between skills and the subagents they launch. It never writes anything.
//
// Target selection mirrors ai-agents/scripts/copy-entries.sh so "what deploy
// copies" and "what lint checks" stay in lockstep:
//
//   - skills = skills/<name>/SKILL.md   (top-level directories)
//   - agents = agents/<name>.md         (top-level *.md files)
//
// The rule logic is pure: parseFrontmatter and the check* helpers take parsed
// data and never touch the filesystem. The filesystem itself is injected as an
// fs.FS rooted at the ai-agents tree, so tests exercise the rules against
// in-memory fstest.MapFS fixtures.
package lint

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// severity ranks a finding. Only warn and error are ever emitted; a target
// with no findings passed every rule.
type severity int

const (
	// severityWarn is advisory: it fails the run only under --strict.
	severityWarn severity = iota
	// severityError fails the run unconditionally.
	severityError
)

// String returns the lowercase label used in the summary output.
func (s severity) String() string {
	switch s {
	case severityWarn:
		return "warn"
	case severityError:
		return "error"
	default:
		return "unknown"
	}
}

// finding is one rule violation for one target: the entry path relative to
// root (e.g. "skills/investigate/SKILL.md"), the stable rule slug (e.g.
// "name-matches-dir") and a human-readable detail. errFinding and warnFinding
// are the only constructors, so every finding carries a valid severity.
type finding struct {
	target string
	rule   string
	sev    severity
	detail string
}

// errFinding builds a finding that fails the run unconditionally.
func errFinding(target, rule, detail string) finding {
	return finding{target: target, rule: rule, sev: severityError, detail: detail}
}

// warnFinding builds an advisory finding (fails the run only under --strict).
func warnFinding(target, rule, detail string) finding {
	return finding{target: target, rule: rule, sev: severityWarn, detail: detail}
}

// Report is the outcome of a lint run. newReport is the only constructor and
// establishes the invariant that findings are sorted by (target, rule).
type Report struct {
	findings   []finding
	skillCount int
	agentCount int
}

// newReport sorts findings by (target, rule) and wraps them with the counts of
// inspected targets.
func newReport(findings []finding, skillCount, agentCount int) Report {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].target != findings[j].target {
			return findings[i].target < findings[j].target
		}
		return findings[i].rule < findings[j].rule
	})
	return Report{findings: findings, skillCount: skillCount, agentCount: agentCount}
}

// HasError reports whether any finding is an error.
func (r Report) HasError() bool {
	for _, f := range r.findings {
		if f.sev == severityError {
			return true
		}
	}
	return false
}

// HasWarn reports whether any finding is a warn.
func (r Report) HasWarn() bool {
	for _, f := range r.findings {
		if f.sev == severityWarn {
			return true
		}
	}
	return false
}

// Render writes the findings and a summary line to w.
func (r Report) Render(w io.Writer) {
	errCount, warnCount := 0, 0
	for _, f := range r.findings {
		switch f.sev {
		case severityError:
			errCount++
		case severityWarn:
			warnCount++
		}
		_, _ = fmt.Fprintf(w, "%-5s %s\t%s: %s\n", f.sev, f.target, f.rule, f.detail)
	}
	_, _ = fmt.Fprintf(w, "checked %d skills, %d agents — %d errors, %d warnings\n",
		r.skillCount, r.agentCount, errCount, warnCount)
}

// nameRe constrains a skill/agent name to a lowercase kebab-case token. It is
// the same rule the skill-scaffold skill applies at generation time, promoted
// here to a permanent gate.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// reservedWords must not appear as substrings of a name (agent identity should
// not shadow the vendor namespace).
var reservedWords = []string{"claude", "anthropic"}

// Known top-level frontmatter keys. Unknown keys are reported as warn, not
// error, so a spec addition never breaks the deploy gate (fail-open): update
// these tables to follow the spec rather than changing rule logic.
// Source: https://code.claude.com/docs/en/skills
var (
	skillKnownKeys = map[string]bool{
		"name": true, "description": true,
		"disable-model-invocation": true, "argument-hint": true, "metadata": true,
	}
	agentKnownKeys = map[string]bool{
		"name": true, "description": true, "tools": true, "model": true,
		"permissionMode": true, "memory": true, "maxTurns": true, "color": true,
	}
)

// Run lints the ai-agents tree rooted at fsys, returning findings sorted by
// (target, rule). A missing skills/ or agents/ subtree is reported as a
// finding, not an error; an error is returned only for unexpected IO failures.
func Run(fsys fs.FS) (Report, error) {
	agentFindings, agentNames, agentErr := lintAgents(fsys)
	if agentErr != nil {
		return Report{}, agentErr
	}
	skillFindings, skillCount, skillErr := lintSkills(fsys, agentNames)
	if skillErr != nil {
		return Report{}, skillErr
	}

	findings := make([]finding, 0, len(agentFindings)+len(skillFindings))
	findings = append(findings, agentFindings...)
	findings = append(findings, skillFindings...)
	return newReport(findings, skillCount, len(agentNames)), nil
}

// lintAgents checks agents/*.md and returns the set of agent names (file base
// names) used to resolve skill references. Names are collected from every
// regular *.md file regardless of frontmatter validity, because a reference
// resolves against the deployed file's existence.
func lintAgents(fsys fs.FS) ([]finding, map[string]bool, error) {
	names := map[string]bool{}
	dirEntries, readErr := fs.ReadDir(fsys, "agents")
	if readErr != nil {
		if errors.Is(readErr, fs.ErrNotExist) {
			return []finding{errFinding("agents", "agents-dir-exists", "agents directory not found")}, names, nil
		}
		return nil, nil, readErr
	}

	var findings []finding
	for _, d := range dirEntries {
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		base := strings.TrimSuffix(d.Name(), ".md")
		names[base] = true

		target := "agents/" + d.Name()
		data, fileErr := fs.ReadFile(fsys, path.Join("agents", d.Name()))
		if fileErr != nil {
			return nil, nil, fileErr
		}
		fm, parseErr := parseFrontmatter(data)
		if parseErr != nil {
			findings = append(findings, shapeFinding(target, parseErr))
			continue
		}
		findings = append(findings, checkAgent(target, base, fm)...)
	}
	return findings, names, nil
}

// checkAgent applies the agent rules to one well-formed parsed file (pure).
func checkAgent(target, base string, fm frontmatter) []finding {
	var findings []finding
	name := fm.value("name")
	if name == "" {
		findings = append(findings, errFinding(target, "name-required", "name is missing or empty"))
	} else {
		findings = append(findings, checkNameFormat(target, name)...)
		if name != base {
			findings = append(findings, errFinding(target, "name-matches-file", "name "+q(name)+" does not match file name "+q(base)))
		}
	}
	if fm.value("description") == "" {
		findings = append(findings, errFinding(target, "description-required", "description is missing or empty"))
	}
	if !fm.has("tools") {
		findings = append(findings, warnFinding(target, "tools-present", "tools field is absent"))
	}
	if !fm.has("model") {
		findings = append(findings, warnFinding(target, "model-present", "model field is absent"))
	}
	findings = append(findings, unknownKeyFindings(target, fm, agentKnownKeys)...)
	return findings
}

// declaredName pairs a skill name with the target that declared it, feeding
// cross-skill collision detection.
type declaredName struct {
	target string
	name   string
}

// lintSkills checks skills/<name>/SKILL.md and returns the findings plus the
// number of skills inspected. agentNames resolves referential integrity.
func lintSkills(fsys fs.FS, agentNames map[string]bool) ([]finding, int, error) {
	dirEntries, readErr := fs.ReadDir(fsys, "skills")
	if readErr != nil {
		if errors.Is(readErr, fs.ErrNotExist) {
			return []finding{errFinding("skills", "skills-dir-exists", "skills directory not found")}, 0, nil
		}
		return nil, 0, readErr
	}

	var findings []finding
	var declared []declaredName
	count := 0
	for _, d := range dirEntries {
		if !d.IsDir() {
			continue
		}
		count++
		skillDir := d.Name()
		target := "skills/" + skillDir + "/SKILL.md"
		data, fileErr := fs.ReadFile(fsys, path.Join("skills", skillDir, "SKILL.md"))
		if fileErr != nil {
			if errors.Is(fileErr, fs.ErrNotExist) {
				findings = append(findings, errFinding(target, "skill-md-present", "SKILL.md not found in skill directory"))
				continue
			}
			return nil, 0, fileErr
		}
		fm, parseErr := parseFrontmatter(data)
		if parseErr != nil {
			findings = append(findings, shapeFinding(target, parseErr))
			continue
		}
		findings = append(findings, checkSkill(target, skillDir, fm, agentNames)...)
		if name := fm.value("name"); name != "" {
			declared = append(declared, declaredName{target: target, name: name})
		}
	}
	findings = append(findings, duplicateNameFindings(declared)...)
	return findings, count, nil
}

// checkSkill applies the skill rules to one well-formed parsed file (pure).
// Cross-skill name collisions are handled separately by duplicateNameFindings.
func checkSkill(target, skillDir string, fm frontmatter, agentNames map[string]bool) []finding {
	var findings []finding
	findings = append(findings, checkSkillName(target, skillDir, fm.value("name"))...)
	if fm.value("description") == "" {
		findings = append(findings, errFinding(target, "description-required", "description is missing or empty"))
	}
	findings = append(findings, unknownKeyFindings(target, fm, skillKnownKeys)...)
	for _, ref := range extractAgentRefs(fm.body) {
		if !agentNames[ref] {
			findings = append(findings, errFinding(target, "ref-agent-exists", "referenced subagent "+q(ref)+" has no agents/"+ref+".md"))
		}
	}
	return findings
}

// checkNameFormat validates the naming rules shared by skills and agents:
// kebab-case within 64 chars, no reserved vendor words.
func checkNameFormat(target, name string) []finding {
	var findings []finding
	if !nameRe.MatchString(name) || len(name) > 64 {
		findings = append(findings, errFinding(target, "name-format", "name "+q(name)+" must be lowercase kebab-case within 64 chars"))
	}
	if w := reservedHit(name); w != "" {
		findings = append(findings, errFinding(target, "name-reserved", "name "+q(name)+" contains reserved word "+q(w)))
	}
	return findings
}

// checkSkillName validates the name field against the directory it lives in.
func checkSkillName(target, skillDir, name string) []finding {
	if name == "" {
		return []finding{errFinding(target, "name-required", "name is missing or empty")}
	}
	findings := checkNameFormat(target, name)
	if name != skillDir {
		findings = append(findings, errFinding(target, "name-matches-dir", "name "+q(name)+" does not match directory "+q(skillDir)))
	}
	return findings
}

// duplicateNameFindings reports every skill whose name was already declared by
// an earlier target (pure; declared is in directory iteration order).
func duplicateNameFindings(declared []declaredName) []finding {
	first := map[string]string{}
	var findings []finding
	for _, d := range declared {
		if firstTarget, dup := first[d.name]; dup {
			findings = append(findings, errFinding(d.target, "name-unique", "name "+q(d.name)+" already declared by "+firstTarget))
			continue
		}
		first[d.name] = d.target
	}
	return findings
}

// shapeFinding maps a parseFrontmatter error to the single blocking finding
// for the target; no field rules can run on a malformed block.
func shapeFinding(target string, parseErr error) finding {
	if errors.Is(parseErr, errFrontmatterMissing) {
		return errFinding(target, "frontmatter-present", parseErr.Error())
	}
	return errFinding(target, "frontmatter-closed", parseErr.Error())
}

// unknownKeyFindings reports each top-level frontmatter key absent from known
// as a warn (fail-open on spec drift).
func unknownKeyFindings(target string, fm frontmatter, known map[string]bool) []finding {
	var findings []finding
	for _, k := range fm.unknownKeys(known) {
		findings = append(findings, warnFinding(target, "frontmatter-unknown-key", "unknown frontmatter key "+q(k)))
	}
	return findings
}

// reservedHit returns the first reserved word contained in name, or "".
func reservedHit(name string) string {
	for _, w := range reservedWords {
		if strings.Contains(name, w) {
			return w
		}
	}
	return ""
}

// q wraps s in double quotes for messages.
func q(s string) string { return "\"" + s + "\"" }
