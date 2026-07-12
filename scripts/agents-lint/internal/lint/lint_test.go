package lint

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

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

// fsys materializes tr as an in-memory filesystem, always creating skills/ and
// agents/ so dir-existence rules do not fire unless a test omits them.
func (tr tree) fsys() fstest.MapFS {
	m := fstest.MapFS{
		"skills": &fstest.MapFile{Mode: fs.ModeDir},
		"agents": &fstest.MapFile{Mode: fs.ModeDir},
	}
	for dir, content := range tr.skills {
		m["skills/"+dir+"/SKILL.md"] = &fstest.MapFile{Data: []byte(content)}
	}
	for base, content := range tr.agents {
		m["agents/"+base+".md"] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// findRule returns the first finding for rule, if any.
func findRule(findings []finding, rule string) (finding, bool) {
	for _, f := range findings {
		if f.rule == rule {
			return f, true
		}
	}
	return finding{}, false
}

func TestRunCleanTree(t *testing.T) {
	t.Parallel()
	fsys := tree{
		skills: map[string]string{
			"alpha": validSkill("alpha"),
			"beta":  "---\nname: beta\ndescription: uses a subagent.\n---\n- `subagent_type`: `worker-one`\n",
		},
		agents: map[string]string{"worker-one": validAgent("worker-one")},
	}.fsys()

	report, runErr := Run(fsys)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if len(report.findings) != 0 {
		t.Fatalf("findings = %v, want none", report.findings)
	}
	if report.skillCount != 2 || report.agentCount != 1 {
		t.Fatalf("counts = %d skills / %d agents, want 2 / 1", report.skillCount, report.agentCount)
	}
}

func TestRunSkillRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		dir      string
		content  string
		wantRule string
		wantSev  severity
	}{
		{name: "valid", dir: "foo", content: validSkill("foo"), wantRule: ""},
		{name: "missing frontmatter", dir: "foo", content: "# no frontmatter here\n", wantRule: "frontmatter-present", wantSev: severityError},
		{name: "unterminated frontmatter", dir: "foo", content: "---\nname: foo\ndescription: d\n", wantRule: "frontmatter-closed", wantSev: severityError},
		{name: "empty name", dir: "foo", content: "---\nname:\ndescription: d\n---\n", wantRule: "name-required", wantSev: severityError},
		{name: "bad name format", dir: "foo_bar", content: "---\nname: foo_bar\ndescription: d\n---\n", wantRule: "name-format", wantSev: severityError},
		{name: "reserved word", dir: "claude-helper", content: "---\nname: claude-helper\ndescription: d\n---\n", wantRule: "name-reserved", wantSev: severityError},
		{name: "name mismatches dir", dir: "foo", content: "---\nname: bar\ndescription: d\n---\n", wantRule: "name-matches-dir", wantSev: severityError},
		{name: "empty description", dir: "foo", content: "---\nname: foo\ndescription:\n---\n", wantRule: "description-required", wantSev: severityError},
		{name: "unknown key", dir: "foo", content: "---\nname: foo\ndescription: d\nbogus: x\n---\n", wantRule: "frontmatter-unknown-key", wantSev: severityWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := tree{skills: map[string]string{tt.dir: tt.content}}.fsys()
			report, runErr := Run(fsys)
			if runErr != nil {
				t.Fatalf("Run err = %v", runErr)
			}
			if tt.wantRule == "" {
				if len(report.findings) != 0 {
					t.Fatalf("findings = %v, want none", report.findings)
				}
				return
			}
			f, ok := findRule(report.findings, tt.wantRule)
			if !ok {
				t.Fatalf("rule %q not found in %v", tt.wantRule, report.findings)
			}
			if f.sev != tt.wantSev {
				t.Fatalf("rule %q sev = %s, want %s", tt.wantRule, f.sev, tt.wantSev)
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
		wantSev  severity
	}{
		{name: "valid", base: "foo", content: validAgent("foo"), wantRule: ""},
		{name: "missing frontmatter", base: "foo", content: "just body\n", wantRule: "frontmatter-present", wantSev: severityError},
		{name: "name mismatches file", base: "foo", content: "---\nname: bar\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-matches-file", wantSev: severityError},
		{name: "bad name format", base: "foo_bar", content: "---\nname: foo_bar\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-format", wantSev: severityError},
		{name: "reserved word", base: "claude-helper", content: "---\nname: claude-helper\ndescription: d\ntools: Read\nmodel: x\n---\n", wantRule: "name-reserved", wantSev: severityError},
		{name: "empty description", base: "foo", content: "---\nname: foo\ndescription:\ntools: Read\nmodel: x\n---\n", wantRule: "description-required", wantSev: severityError},
		{name: "tools absent", base: "foo", content: "---\nname: foo\ndescription: d\nmodel: x\n---\n", wantRule: "tools-present", wantSev: severityWarn},
		{name: "model absent", base: "foo", content: "---\nname: foo\ndescription: d\ntools: Read\n---\n", wantRule: "model-present", wantSev: severityWarn},
		{name: "unknown key", base: "foo", content: "---\nname: foo\ndescription: d\ntools: Read\nmodel: x\nbogus: y\n---\n", wantRule: "frontmatter-unknown-key", wantSev: severityWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := tree{agents: map[string]string{tt.base: tt.content}}.fsys()
			report, runErr := Run(fsys)
			if runErr != nil {
				t.Fatalf("Run err = %v", runErr)
			}
			if tt.wantRule == "" {
				if len(report.findings) != 0 {
					t.Fatalf("findings = %v, want none", report.findings)
				}
				return
			}
			f, ok := findRule(report.findings, tt.wantRule)
			if !ok {
				t.Fatalf("rule %q not found in %v", tt.wantRule, report.findings)
			}
			if f.sev != tt.wantSev {
				t.Fatalf("rule %q sev = %s, want %s", tt.wantRule, f.sev, tt.wantSev)
			}
		})
	}
}

func TestRunReferenceIntegrity(t *testing.T) {
	t.Parallel()
	t.Run("dangling reference errors", func(t *testing.T) {
		t.Parallel()
		fsys := tree{
			skills: map[string]string{
				"caller": "---\nname: caller\ndescription: d\n---\n- `subagent_type`: `ghost`\n",
			},
		}.fsys()
		report, runErr := Run(fsys)
		if runErr != nil {
			t.Fatalf("Run err = %v", runErr)
		}
		if _, ok := findRule(report.findings, "ref-agent-exists"); !ok {
			t.Fatalf("expected ref-agent-exists, got %v", report.findings)
		}
	})

	t.Run("resolved reference passes", func(t *testing.T) {
		t.Parallel()
		fsys := tree{
			skills: map[string]string{
				"caller": "---\nname: caller\ndescription: d\n---\n- `subagent_type`: `real`\n",
			},
			agents: map[string]string{"real": validAgent("real")},
		}.fsys()
		report, runErr := Run(fsys)
		if runErr != nil {
			t.Fatalf("Run err = %v", runErr)
		}
		if _, ok := findRule(report.findings, "ref-agent-exists"); ok {
			t.Fatalf("unexpected ref-agent-exists in %v", report.findings)
		}
	})
}

func TestRunNameUnique(t *testing.T) {
	t.Parallel()
	fsys := tree{
		skills: map[string]string{
			"one": "---\nname: shared\ndescription: d\n---\n",
			"two": "---\nname: shared\ndescription: d\n---\n",
		},
	}.fsys()
	report, runErr := Run(fsys)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.findings, "name-unique"); !ok {
		t.Fatalf("expected name-unique, got %v", report.findings)
	}
}

func TestRunMissingSkillMd(t *testing.T) {
	t.Parallel()
	fsys := tree{}.fsys()
	fsys["skills/empty"] = &fstest.MapFile{Mode: fs.ModeDir}
	report, runErr := Run(fsys)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.findings, "skill-md-present"); !ok {
		t.Fatalf("expected skill-md-present, got %v", report.findings)
	}
}

func TestRunMissingDirs(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{} // neither skills/ nor agents/
	report, runErr := Run(fsys)
	if runErr != nil {
		t.Fatalf("Run err = %v", runErr)
	}
	if _, ok := findRule(report.findings, "skills-dir-exists"); !ok {
		t.Fatalf("expected skills-dir-exists, got %v", report.findings)
	}
	if _, ok := findRule(report.findings, "agents-dir-exists"); !ok {
		t.Fatalf("expected agents-dir-exists, got %v", report.findings)
	}
}

func TestReportHasErrorHasWarn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		findings []finding
		wantErr  bool
		wantWarn bool
	}{
		{name: "empty", findings: nil, wantErr: false, wantWarn: false},
		{name: "warn only", findings: []finding{warnFinding("t", "r", "d")}, wantErr: false, wantWarn: true},
		{name: "error only", findings: []finding{errFinding("t", "r", "d")}, wantErr: true, wantWarn: false},
		{name: "both", findings: []finding{warnFinding("t", "r", "d"), errFinding("t", "r", "d")}, wantErr: true, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := newReport(tt.findings, 0, 0)
			if got := report.HasError(); got != tt.wantErr {
				t.Fatalf("HasError = %v, want %v", got, tt.wantErr)
			}
			if got := report.HasWarn(); got != tt.wantWarn {
				t.Fatalf("HasWarn = %v, want %v", got, tt.wantWarn)
			}
		})
	}
}
