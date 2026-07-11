// Package lint statically validates the AI agent skill and subagent
// definitions under ai-agents/ read-only: frontmatter schema, naming
// conventions, name/dir agreement, uniqueness and referential integrity
// between skills and the subagents they launch. It never writes anything.
//
// Target selection mirrors ai-agents/scripts/copy-entries.sh so "what deploy
// copies" and "what lint checks" stay in lockstep:
//
//   - skills = <root>/skills/<name>/SKILL.md   (top-level directories)
//   - agents = <root>/agents/<name>.md         (top-level *.md files)
//
// The rule logic is pure: parseFrontmatter and the check* helpers take parsed
// data and never touch the filesystem. Only Run and the lint<Kind> functions
// read files, so tests exercise the rules against fixture trees built with
// t.TempDir().
package lint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Severity ranks a finding. Only warn and error are ever emitted; a target
// with no findings passed every rule.
type Severity int

const (
	// SeverityWarn is advisory: it fails the run only under --strict.
	SeverityWarn Severity = iota
	// SeverityError fails the run unconditionally.
	SeverityError
)

// String returns the lowercase label used in the summary output.
func (s Severity) String() string {
	switch s {
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// Finding is one rule violation for one target.
type Finding struct {
	// Target is the entry path relative to root, e.g. "skills/investigate/SKILL.md".
	Target string
	// Rule is the stable rule slug, e.g. "name-matches-dir".
	Rule string
	// Sev is warn or error.
	Sev Severity
	// Detail is a human-readable explanation.
	Detail string
}

// Report is the outcome of a lint run.
type Report struct {
	// Findings are every warn/error, sorted by (target, rule).
	Findings []Finding
	// SkillCount is the number of skills inspected.
	SkillCount int
	// AgentCount is the number of agents inspected.
	AgentCount int
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

// Run lints the ai-agents tree rooted at root, returning findings sorted by
// (target, rule). A missing skills/ or agents/ subtree is reported as a
// finding, not an error; an error is returned only for unexpected IO failures.
func Run(root string) (Report, error) {
	agentFindings, agentNames, agentErr := lintAgents(filepath.Join(root, "agents"))
	if agentErr != nil {
		return Report{}, agentErr
	}
	skillFindings, skillCount, skillErr := lintSkills(filepath.Join(root, "skills"), agentNames)
	if skillErr != nil {
		return Report{}, skillErr
	}

	findings := make([]Finding, 0, len(agentFindings)+len(skillFindings))
	findings = append(findings, agentFindings...)
	findings = append(findings, skillFindings...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Target != findings[j].Target {
			return findings[i].Target < findings[j].Target
		}
		return findings[i].Rule < findings[j].Rule
	})
	return Report{Findings: findings, SkillCount: skillCount, AgentCount: len(agentNames)}, nil
}

// HasError reports whether any finding is an error.
func HasError(findings []Finding) bool {
	for _, f := range findings {
		if f.Sev == SeverityError {
			return true
		}
	}
	return false
}

// HasWarn reports whether any finding is a warn.
func HasWarn(findings []Finding) bool {
	for _, f := range findings {
		if f.Sev == SeverityWarn {
			return true
		}
	}
	return false
}

// lintAgents checks agents/*.md and returns the set of agent names (file base
// names) used to resolve skill references. Names are collected from every
// regular *.md file regardless of frontmatter validity, because a reference
// resolves against the deployed file's existence.
func lintAgents(dir string) ([]Finding, map[string]bool, error) {
	names := map[string]bool{}
	dirEntries, readErr := os.ReadDir(dir)
	if readErr != nil {
		if errors.Is(readErr, fs.ErrNotExist) {
			return []Finding{{Target: "agents", Rule: "agents-dir-exists", Sev: SeverityError, Detail: "agents directory not found: " + dir}}, names, nil
		}
		return nil, nil, readErr
	}

	var findings []Finding
	for _, d := range dirEntries {
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		base := strings.TrimSuffix(d.Name(), ".md")
		names[base] = true

		target := "agents/" + d.Name()
		data, fileErr := os.ReadFile(filepath.Join(dir, d.Name()))
		if fileErr != nil {
			return nil, nil, fileErr
		}
		findings = append(findings, checkAgent(target, base, parseFrontmatter(data))...)
	}
	return findings, names, nil
}

// checkAgent applies the agent rules to one parsed file (pure).
func checkAgent(target, base string, fm frontmatter) []Finding {
	var findings []Finding
	if bad := checkFrontmatterShape(target, fm); bad != nil {
		return bad
	}
	name := fm.value("name")
	if name == "" {
		findings = append(findings, Finding{target, "name-required", SeverityError, "name is missing or empty"})
	} else {
		findings = append(findings, checkNameFormat(target, name)...)
		if name != base {
			findings = append(findings, Finding{target, "name-matches-file", SeverityError, "name " + q(name) + " does not match file name " + q(base)})
		}
	}
	if fm.value("description") == "" {
		findings = append(findings, Finding{target, "description-required", SeverityError, "description is missing or empty"})
	}
	if !fm.has("tools") {
		findings = append(findings, Finding{target, "tools-present", SeverityWarn, "tools field is absent"})
	}
	if !fm.has("model") {
		findings = append(findings, Finding{target, "model-present", SeverityWarn, "model field is absent"})
	}
	findings = append(findings, unknownKeyFindings(target, fm, agentKnownKeys)...)
	return findings
}

// lintSkills checks skills/<name>/SKILL.md and returns the findings plus the
// number of skills inspected. agentNames resolves referential integrity.
func lintSkills(dir string, agentNames map[string]bool) ([]Finding, int, error) {
	dirEntries, readErr := os.ReadDir(dir)
	if readErr != nil {
		if errors.Is(readErr, fs.ErrNotExist) {
			return []Finding{{Target: "skills", Rule: "skills-dir-exists", Sev: SeverityError, Detail: "skills directory not found: " + dir}}, 0, nil
		}
		return nil, 0, readErr
	}

	var findings []Finding
	count := 0
	seen := map[string]string{} // name -> first target that declared it
	for _, d := range dirEntries {
		if !d.IsDir() {
			continue
		}
		count++
		skillDir := d.Name()
		target := "skills/" + skillDir + "/SKILL.md"
		data, fileErr := os.ReadFile(filepath.Join(dir, skillDir, "SKILL.md"))
		if fileErr != nil {
			if errors.Is(fileErr, fs.ErrNotExist) {
				findings = append(findings, Finding{target, "skill-md-present", SeverityError, "SKILL.md not found in skill directory"})
				continue
			}
			return nil, 0, fileErr
		}
		findings = append(findings, checkSkill(target, skillDir, parseFrontmatter(data), agentNames, seen)...)
	}
	return findings, count, nil
}

// checkSkill applies the skill rules to one parsed file (pure apart from the
// shared seen map, which detects cross-skill name collisions).
func checkSkill(target, skillDir string, fm frontmatter, agentNames map[string]bool, seen map[string]string) []Finding {
	if bad := checkFrontmatterShape(target, fm); bad != nil {
		return bad
	}
	var findings []Finding
	findings = append(findings, checkSkillName(target, skillDir, fm.value("name"), seen)...)
	if fm.value("description") == "" {
		findings = append(findings, Finding{target, "description-required", SeverityError, "description is missing or empty"})
	}
	findings = append(findings, unknownKeyFindings(target, fm, skillKnownKeys)...)
	for _, ref := range extractAgentRefs(fm.body) {
		if !agentNames[ref] {
			findings = append(findings, Finding{target, "ref-agent-exists", SeverityError, "referenced subagent " + q(ref) + " has no agents/" + ref + ".md"})
		}
	}
	return findings
}

// checkNameFormat validates the naming rules shared by skills and agents:
// kebab-case within 64 chars, no reserved vendor words.
func checkNameFormat(target, name string) []Finding {
	var findings []Finding
	if !nameRe.MatchString(name) || len(name) > 64 {
		findings = append(findings, Finding{target, "name-format", SeverityError, "name " + q(name) + " must be lowercase kebab-case within 64 chars"})
	}
	if w := reservedHit(name); w != "" {
		findings = append(findings, Finding{target, "name-reserved", SeverityError, "name " + q(name) + " contains reserved word " + q(w)})
	}
	return findings
}

// checkSkillName validates the name field and records it for collision
// detection.
func checkSkillName(target, skillDir, name string, seen map[string]string) []Finding {
	if name == "" {
		return []Finding{{target, "name-required", SeverityError, "name is missing or empty"}}
	}
	findings := checkNameFormat(target, name)
	if name != skillDir {
		findings = append(findings, Finding{target, "name-matches-dir", SeverityError, "name " + q(name) + " does not match directory " + q(skillDir)})
	}
	if first, dup := seen[name]; dup {
		findings = append(findings, Finding{target, "name-unique", SeverityError, "name " + q(name) + " already declared by " + first})
	} else {
		seen[name] = target
	}
	return findings
}

// checkFrontmatterShape returns a single blocking finding when the frontmatter
// block is absent or unterminated, in which case no field rules can run. It
// returns nil when the block is well-formed.
func checkFrontmatterShape(target string, fm frontmatter) []Finding {
	if !fm.present {
		return []Finding{{target, "frontmatter-present", SeverityError, "missing leading --- frontmatter block"}}
	}
	if !fm.closed {
		return []Finding{{target, "frontmatter-closed", SeverityError, "frontmatter block is not terminated by ---"}}
	}
	return nil
}

// unknownKeyFindings reports each top-level frontmatter key absent from known
// as a warn (fail-open on spec drift).
func unknownKeyFindings(target string, fm frontmatter, known map[string]bool) []Finding {
	var findings []Finding
	for _, k := range fm.keys {
		if !known[k] {
			findings = append(findings, Finding{target, "frontmatter-unknown-key", SeverityWarn, "unknown frontmatter key " + q(k)})
		}
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
