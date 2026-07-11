package lint

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path (and parents) with content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
		t.Fatalf("write %s: %v", path, writeErr)
	}
}

// validSkill returns a SKILL.md that passes every rule.
func validSkill(name string) string {
	return "---\nname: " + name + "\ndescription: A valid skill description.\n---\n# body\n"
}

// validAgent returns an agent md that passes every rule.
func validAgent(name string) string {
	return "---\nname: " + name + "\ndescription: A valid agent.\ntools: Read\nmodel: sonnet\n---\nbody\n"
}

// tree describes a fixture ai-agents root.
type tree struct {
	skills map[string]string // dir name -> SKILL.md content
	agents map[string]string // base name -> <base>.md content
}

// build materializes tr under a fresh temp root, always creating skills/ and
// agents/ so dir-existence rules do not fire unless a test omits them.
func (tr tree) build(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if mkErr := os.MkdirAll(filepath.Join(root, "skills"), 0o755); mkErr != nil {
		t.Fatalf("mkdir skills: %v", mkErr)
	}
	if mkErr := os.MkdirAll(filepath.Join(root, "agents"), 0o755); mkErr != nil {
		t.Fatalf("mkdir agents: %v", mkErr)
	}
	for dir, content := range tr.skills {
		writeFile(t, filepath.Join(root, "skills", dir, "SKILL.md"), content)
	}
	for base, content := range tr.agents {
		writeFile(t, filepath.Join(root, "agents", base+".md"), content)
	}
	return root
}

// findRule returns the first finding for rule, if any.
func findRule(findings []Finding, rule string) (Finding, bool) {
	for _, f := range findings {
		if f.Rule == rule {
			return f, true
		}
	}
	return Finding{}, false
}

func TestRunCleanTree(t *testing.T) {
	t.Parallel()
	root := tree{
		skills: map[string]string{
			"alpha": validSkill("alpha"),
			"beta":  "---\nname: beta\ndescription: uses a subagent.\n---\n- `subagent_type`: `worker-one`\n",
		},
		agents: map[string]string{"worker-one": validAgent("worker-one")},
	}.build(t)

	report, runErr := Run(root)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %v, want none", report.Findings)
	}
	if report.SkillCount != 2 || report.AgentCount != 1 {
		t.Fatalf("counts = %d skills / %d agents, want 2 / 1", report.SkillCount, report.AgentCount)
	}
}

func TestRunSkillRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		dir      string
		content  string
		wantRule string
		wantSev  Severity
	}{
		{name: "valid", dir: "foo", content: validSkill("foo"), wantRule: ""},
		{name: "missing frontmatter", dir: "foo", content: "# no frontmatter here\n", wantRule: "frontmatter-present", wantSev: SeverityError},
		{name: "unterminated frontmatter", dir: "foo", content: "---\nname: foo\ndescription: d\n", wantRule: "frontmatter-closed", wantSev: SeverityError},
		{name: "empty name", dir: "foo", content: "---\nname:\ndescription: d\n---\n", wantRule: "name-required", wantSev: SeverityError},
		{name: "bad name format", dir: "foo_bar", content: "---\nname: foo_bar\ndescription: d\n---\n", wantRule: "name-format", wantSev: SeverityError},
		{name: "reserved word", dir: "claude-helper", content: "---\nname: claude-helper\ndescription: d\n---\n", wantRule: "name-reserved", wantSev: SeverityError},
		{name: "name mismatches dir", dir: "foo", content: "---\nname: bar\ndescription: d\n---\n", wantRule: "name-matches-dir", wantSev: SeverityError},
		{name: "empty description", dir: "foo", content: "---\nname: foo\ndescription:\n---\n", wantRule: "description-required", wantSev: SeverityError},
		{name: "unknown key", dir: "foo", content: "---\nname: foo\ndescription: d\nbogus: x\n---\n", wantRule: "frontmatter-unknown-key", wantSev: SeverityWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := tree{skills: map[string]string{tt.dir: tt.content}}.build(t)
			report, runErr := Run(root)
			if runErr != nil {
				t.Fatalf("Run err = %v", runErr)
			}
			if tt.wantRule == "" {
				if len(report.Findings) != 0 {
					t.Fatalf("findings = %v, want none", report.Findings)
				}
				return
			}
			f, ok := findRule(report.Findings, tt.wantRule)
			if !ok {
				t.Fatalf("rule %q not found in %v", tt.wantRule, report.Findings)
			}
			if f.Sev != tt.wantSev {
				t.Fatalf("rule %q sev = %s, want %s", tt.wantRule, f.Sev, tt.wantSev)
			}
		})
	}
}

func TestRunAgentRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		base     string
		content  string
		wantRule string
		wantSev  Severity
	}{
		{name: "valid", base: "foo", content: validAgent("foo"), wantRule: ""},
		{name: "missing frontmatter", base: "foo", content: "just body\n", wantRule: "frontmatter-present", wantSev: SeverityError},
		{name: "name mismatches file", base: "foo", content: "---\nname: bar\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-matches-file", wantSev: SeverityError},
		{name: "bad name format", base: "foo_bar", content: "---\nname: foo_bar\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-format", wantSev: SeverityError},
		{name: "reserved word", base: "claude-helper", content: "---\nname: claude-helper\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-reserved", wantSev: SeverityError},
		{name: "empty description", base: "foo", content: "---\nname: foo\ndescription:\ntools: Read\nmodel: x\n---\n", wantRule: "description-required", wantSev: SeverityError},
		{name: "tools absent", base: "foo", content: "---\nname: foo\ndescription: d\nmodel: x\n---\n", wantRule: "tools-present", wantSev: SeverityWarn},
		{name: "model absent", base: "foo", content: "---\nname: foo\ndescription: d\ntools: Read\n---\n", wantRule: "model-present", wantSev: SeverityWarn},
		{name: "unknown key", base: "foo", content: "---\nname: foo\ndescription: d\ntools: Read\nmodel: x\nbogus: y\n---\n", wantRule: "frontmatter-unknown-key", wantSev: SeverityWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := tree{agents: map[string]string{tt.base: tt.content}}.build(t)
			report, runErr := Run(root)
			if runErr != nil {
				t.Fatalf("Run err = %v", runErr)
			}
			if tt.wantRule == "" {
				if len(report.Findings) != 0 {
					t.Fatalf("findings = %v, want none", report.Findings)
				}
				return
			}
			f, ok := findRule(report.Findings, tt.wantRule)
			if !ok {
				t.Fatalf("rule %q not found in %v", tt.wantRule, report.Findings)
			}
			if f.Sev != tt.wantSev {
				t.Fatalf("rule %q sev = %s, want %s", tt.wantRule, f.Sev, tt.wantSev)
			}
		})
	}
}

func TestRunReferenceIntegrity(t *testing.T) {
	t.Parallel()
	t.Run("dangling reference errors", func(t *testing.T) {
		t.Parallel()
		root := tree{
			skills: map[string]string{
				"caller": "---\nname: caller\ndescription: d\n---\n- `subagent_type`: `ghost`\n",
			},
		}.build(t)
		report, runErr := Run(root)
		if runErr != nil {
			t.Fatalf("Run err = %v", runErr)
		}
		if _, ok := findRule(report.Findings, "ref-agent-exists"); !ok {
			t.Fatalf("expected ref-agent-exists, got %v", report.Findings)
		}
	})

	t.Run("resolved reference passes", func(t *testing.T) {
		t.Parallel()
		root := tree{
			skills: map[string]string{
				"caller": "---\nname: caller\ndescription: d\n---\n- `subagent_type`: `real`\n",
			},
			agents: map[string]string{"real": validAgent("real")},
		}.build(t)
		report, runErr := Run(root)
		if runErr != nil {
			t.Fatalf("Run err = %v", runErr)
		}
		if _, ok := findRule(report.Findings, "ref-agent-exists"); ok {
			t.Fatalf("unexpected ref-agent-exists in %v", report.Findings)
		}
	})
}

func TestRunNameUnique(t *testing.T) {
	t.Parallel()
	root := tree{
		skills: map[string]string{
			"one": "---\nname: shared\ndescription: d\n---\n",
			"two": "---\nname: shared\ndescription: d\n---\n",
		},
	}.build(t)
	report, runErr := Run(root)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.Findings, "name-unique"); !ok {
		t.Fatalf("expected name-unique, got %v", report.Findings)
	}
}

func TestRunMissingSkillMd(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if mkErr := os.MkdirAll(filepath.Join(root, "skills", "empty"), 0o755); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	if mkErr := os.MkdirAll(filepath.Join(root, "agents"), 0o755); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	report, runErr := Run(root)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.Findings, "skill-md-present"); !ok {
		t.Fatalf("expected skill-md-present, got %v", report.Findings)
	}
}

func TestRunMissingDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir() // neither skills/ nor agents/
	report, runErr := Run(root)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.Findings, "skills-dir-exists"); !ok {
		t.Fatalf("expected skills-dir-exists, got %v", report.Findings)
	}
	if _, ok := findRule(report.Findings, "agents-dir-exists"); !ok {
		t.Fatalf("expected agents-dir-exists, got %v", report.Findings)
	}
}

func TestHasErrorHasWarn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		findings []Finding
		wantErr  bool
		wantWarn bool
	}{
		{name: "empty", findings: nil, wantErr: false, wantWarn: false},
		{name: "warn only", findings: []Finding{{Sev: SeverityWarn}}, wantErr: false, wantWarn: true},
		{name: "error only", findings: []Finding{{Sev: SeverityError}}, wantErr: true, wantWarn: false},
		{name: "both", findings: []Finding{{Sev: SeverityWarn}, {Sev: SeverityError}}, wantErr: true, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HasError(tt.findings); got != tt.wantErr {
				t.Fatalf("HasError = %v, want %v", got, tt.wantErr)
			}
			if got := HasWarn(tt.findings); got != tt.wantWarn {
				t.Fatalf("HasWarn = %v, want %v", got, tt.wantWarn)
			}
		})
	}
}
