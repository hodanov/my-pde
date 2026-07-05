package gen

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeCITemplate is a minimal stand-in for .github/workflows/ci_config_diff.yml.
const fakeCITemplate = `name: CI config-diff
on:
  pull_request:
    paths:
      - "scripts/config-diff/**"
      - mise.toml
jobs:
  ci:
    uses: ./.github/workflows/go_module_ci.yml
    with:
      module: config-diff
`

// fakeMise is a minimal stand-in for mise.toml with two module sections.
const fakeMise = `# ---- config-diff (Go) ----

[tasks."config-diff:build"]
dir = "scripts/config-diff"
run = "go build -o config-diff ./cmd/config-diff"

[tasks."config-diff:clean"]
run = "rm -f config-diff coverage.out"

# ---- go-verify (Go) ----

[tasks."go-verify:build"]
dir = "scripts/go-verify"
`

// fakeRead returns the fake templates keyed by repository-relative path.
func fakeRead(rel string) ([]byte, error) {
	switch rel {
	case ".github/workflows/ci_config_diff.yml":
		return []byte(fakeCITemplate), nil
	case "mise.toml":
		return []byte(fakeMise), nil
	default:
		return nil, errors.New("no such file: " + rel)
	}
}

// noneExist reports every path as absent.
func noneExist(string) bool { return false }

// fromOnlyExists reports only the template module directory as present, the
// valid state for NewSpec: --from exists, the new module does not.
func fromOnlyExists(rel string) bool { return rel == "scripts/config-diff" }

// mustSpec builds a valid spec through the factory, failing the test on error.
func mustSpec(t *testing.T, name, from string) spec {
	t.Helper()
	s, newErr := NewSpec(name, from, fromOnlyExists)
	if newErr != nil {
		t.Fatalf("NewSpec(%q, %q) returned error: %v", name, from, newErr)
	}
	return s
}

func TestNewSpecErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		modName string
		from    string
		exists  ExistsFunc
		wantSub string
	}{
		{
			name:    "invalid name",
			modName: "Bad_Name",
			from:    "config-diff",
			exists:  fromOnlyExists,
			wantSub: "invalid module name",
		},
		{
			name:    "invalid from",
			modName: "log-tail",
			from:    "Bad",
			exists:  fromOnlyExists,
			wantSub: "invalid --from",
		},
		{
			name:    "same name and from",
			modName: "config-diff",
			from:    "config-diff",
			exists:  fromOnlyExists,
			wantSub: "must differ",
		},
		{
			name:    "from module missing",
			modName: "log-tail",
			from:    "ghost",
			exists:  fromOnlyExists,
			wantSub: "not found",
		},
		{
			name:    "new module already exists",
			modName: "log-tail",
			from:    "config-diff",
			exists:  func(string) bool { return true },
			wantSub: "already exists",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, newErr := NewSpec(tt.modName, tt.from, tt.exists)
			if newErr == nil {
				t.Fatalf("NewSpec returned nil error, want error containing %q", tt.wantSub)
			}
			if !strings.Contains(newErr.Error(), tt.wantSub) {
				t.Fatalf("NewSpec error = %q, want it to contain %q", newErr.Error(), tt.wantSub)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()
	res, planErr := mustSpec(t, "log-tail", "config-diff").Plan(fakeRead, noneExist)
	if planErr != nil {
		t.Fatalf("Plan returned error: %v", planErr)
	}

	// Observe the planned files through Write with a recording callback, the
	// same way main consumes the plan.
	written := map[string]string{}
	if writeErr := res.Write(func(rel string, content []byte) error {
		written[rel] = string(content)
		return nil
	}); writeErr != nil {
		t.Fatalf("Write returned error: %v", writeErr)
	}

	wantPaths := map[string]bool{
		"scripts/log-tail/go.mod":                    false,
		"scripts/log-tail/cmd/log-tail/main.go":      false,
		"scripts/log-tail/cmd/log-tail/main_test.go": false,
		"scripts/log-tail/README.md":                 false,
		".github/workflows/ci_log_tail.yml":          false,
	}
	for p, content := range written {
		if _, ok := wantPaths[p]; !ok {
			t.Errorf("unexpected file %q", p)
			continue
		}
		wantPaths[p] = true
		if strings.Contains(content, "config-diff") {
			t.Errorf("file %q still contains template token config-diff", p)
		}
	}
	for p, seen := range wantPaths {
		if !seen {
			t.Errorf("missing expected file %q", p)
		}
	}

	var report bytes.Buffer
	res.Report(&report)
	got := report.String()
	if !strings.Contains(got, `[tasks."log-tail:build"]`) {
		t.Errorf("report missing renamed mise task: %q", got)
	}
	if strings.Contains(got, "go-verify") {
		t.Errorf("report leaked the next mise section: %q", got)
	}
	if strings.Contains(got, "config-diff") {
		t.Errorf("report still contains template token: %q", got)
	}
	if !strings.Contains(got, "Created module log-tail:") {
		t.Errorf("report missing header: %q", got)
	}
}

func TestWritePropagatesError(t *testing.T) {
	t.Parallel()
	res, planErr := mustSpec(t, "log-tail", "config-diff").Plan(fakeRead, noneExist)
	if planErr != nil {
		t.Fatalf("Plan returned error: %v", planErr)
	}

	writeErr := res.Write(func(string, []byte) error { return errors.New("disk full") })
	if writeErr == nil {
		t.Fatal("Write returned nil error, want error")
	}
	if !strings.Contains(writeErr.Error(), "write scripts/log-tail/go.mod") || !strings.Contains(writeErr.Error(), "disk full") {
		t.Fatalf("Write error = %q, want it to name the file and wrap the cause", writeErr.Error())
	}
}

func TestPlanErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spec    spec
		read    ReadFunc
		exists  ExistsFunc
		wantSub string
	}{
		{
			name: "missing template workflow",
			spec: func() spec {
				s, newErr := NewSpec("log-tail", "ghost", func(rel string) bool { return rel == "scripts/ghost" })
				if newErr != nil {
					panic(newErr)
				}
				return s
			}(),
			read:    fakeRead,
			exists:  noneExist,
			wantSub: "read template workflow",
		},
		{
			name: "missing mise section",
			read: func(rel string) ([]byte, error) {
				if rel == "mise.toml" {
					return []byte("# ---- other (Go) ----\n"), nil
				}
				return fakeRead(rel)
			},
			exists:  noneExist,
			wantSub: "no",
		},
		{
			name:    "collision refuses overwrite",
			read:    fakeRead,
			exists:  func(rel string) bool { return rel == "scripts/log-tail/go.mod" },
			wantSub: "already exists",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.spec
			if s == (spec{}) {
				s = mustSpec(t, "log-tail", "config-diff")
			}
			_, planErr := s.Plan(tt.read, tt.exists)
			if planErr == nil {
				t.Fatalf("Plan returned nil error, want error containing %q", tt.wantSub)
			}
			if !strings.Contains(planErr.Error(), tt.wantSub) {
				t.Fatalf("Plan error = %q, want it to contain %q", planErr.Error(), tt.wantSub)
			}
		})
	}
}

func TestExtractMiseSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mise     string
		from     string
		wantSub  string
		wantErr  bool
		wantTail string
	}{
		{
			name:     "extracts bounded section",
			mise:     fakeMise,
			from:     "config-diff",
			wantSub:  `[tasks."config-diff:build"]`,
			wantTail: "\n",
		},
		{
			name:    "missing section errors",
			mise:    "# ---- other (Go) ----\n",
			from:    "config-diff",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, sectionErr := extractMiseSection(tt.mise, tt.from)
			if tt.wantErr {
				if sectionErr == nil {
					t.Fatalf("extractMiseSection error = nil, want error")
				}
				return
			}
			if sectionErr != nil {
				t.Fatalf("extractMiseSection error = %v", sectionErr)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("section = %q, want it to contain %q", got, tt.wantSub)
			}
			if strings.Contains(got, "go-verify") {
				t.Errorf("section leaked next block: %q", got)
			}
			if !strings.HasSuffix(got, tt.wantTail) {
				t.Errorf("section = %q, want suffix %q", got, tt.wantTail)
			}
		})
	}
}

func TestWorkflowPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		module string
		want   string
	}{
		{name: "kebab becomes underscore", module: "config-diff", want: ".github/workflows/ci_config_diff.yml"},
		{name: "single word", module: "doctor", want: ".github/workflows/ci_doctor.yml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := workflowPath(tt.module); got != tt.want {
				t.Fatalf("workflowPath(%q) = %q, want %q", tt.module, got, tt.want)
			}
		})
	}
}
