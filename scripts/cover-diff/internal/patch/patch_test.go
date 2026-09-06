package patch

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"cover-diff/internal/cover"
	"cover-diff/internal/udiff"
)

func TestSelect(t *testing.T) {
	t.Parallel()
	changed := []udiff.File{
		{Path: "scripts/beta/cmd/beta/main.go", Lines: []int{1}},
		{Path: "scripts/alpha/internal/a/a.go", Lines: []int{2}},
		{Path: "scripts/alpha/internal/a/a_test.go", Lines: []int{3}},
		{Path: "scripts/alpha/internal/port/mock/mock_port.go", Lines: []int{4}},
		{Path: "scripts/alpha/README.md", Lines: []int{5}},
		{Path: "nvim/config/init.lua", Lines: []int{6}},
		{Path: "scripts/loose.go", Lines: []int{7}},
	}
	tests := []struct {
		name string
		opt  Options
		want []Selected
	}{
		{
			name: "keeps module go sources and drops tests, docs and non module paths",
			want: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/internal/a/a.go", Lines: []int{2}}},
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/internal/port/mock/mock_port.go", Lines: []int{4}}},
				{Module: "beta", File: udiff.File{Path: "scripts/beta/cmd/beta/main.go", Lines: []int{1}}},
			},
		},
		{
			name: "exclude drops matching paths",
			opt:  Options{Exclude: regexp.MustCompile(`/mock/`)},
			want: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/internal/a/a.go", Lines: []int{2}}},
				{Module: "beta", File: udiff.File{Path: "scripts/beta/cmd/beta/main.go", Lines: []int{1}}},
			},
		},
		{
			name: "mod filter narrows to one module",
			opt:  Options{ModFilter: "beta"},
			want: []Selected{
				{Module: "beta", File: udiff.File{Path: "scripts/beta/cmd/beta/main.go", Lines: []int{1}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Select(changed, tt.opt)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Select() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestModules(t *testing.T) {
	t.Parallel()
	got := Modules([]Selected{
		{Module: "beta"}, {Module: "alpha"}, {Module: "beta"},
	})
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Modules() = %v, want %v", got, want)
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		selected []Selected
		profiles map[string]cover.Profile
		warnings []string
		want     Report
	}{
		{
			name: "changed lines without statements leave the denominator",
			selected: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/a.go", Lines: []int{1, 3, 8, 10, 11}}},
			},
			profiles: map[string]cover.Profile{
				"alpha": {Mode: "set", Files: map[string]map[int]bool{
					"alpha/a.go": {8: true, 10: false, 11: false},
				}},
			},
			want: Report{
				Modules: []Module{{
					Module: "alpha",
					Total:  Totals{Statements: 3, Covered: 1, Coverage: 0.3333},
					Files: []File{{
						Path: "scripts/alpha/a.go", Statements: 3, Covered: 1,
						UncoveredLines: []int{10, 11},
					}},
				}},
				Total:      Totals{Statements: 3, Covered: 1, Coverage: 0.3333},
				Unmeasured: []string{},
				Warnings:   []string{},
			},
		},
		{
			name: "files whose changed lines are all declarations are omitted",
			selected: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/doc.go", Lines: []int{1, 2}}},
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/a.go", Lines: []int{5}}},
			},
			profiles: map[string]cover.Profile{
				"alpha": {Mode: "set", Files: map[string]map[int]bool{"alpha/a.go": {5: true}}},
			},
			want: Report{
				Modules: []Module{{
					Module: "alpha",
					Total:  Totals{Statements: 1, Covered: 1, Coverage: 1},
					Files: []File{{
						Path: "scripts/alpha/a.go", Statements: 1, Covered: 1,
						UncoveredLines: []int{},
					}},
				}},
				Total:      Totals{Statements: 1, Covered: 1, Coverage: 1},
				Unmeasured: []string{},
				Warnings:   []string{},
			},
		},
		{
			name: "a module without a profile is unmeasured, not uncovered",
			selected: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/a.go", Lines: []int{5}}},
			},
			profiles: map[string]cover.Profile{},
			want: Report{
				Modules:    []Module{},
				Total:      Totals{},
				Unmeasured: []string{"alpha"},
				Warnings:   []string{},
			},
		},
		{
			name: "a profile path outside the module is reported",
			selected: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/a.go", Lines: []int{5}}},
			},
			profiles: map[string]cover.Profile{
				"alpha": {Mode: "set", Files: map[string]map[int]bool{"example.com/other/a.go": {5: true}}},
			},
			want: Report{
				Modules:    []Module{{Module: "alpha", Total: Totals{}, Files: []File{}}},
				Total:      Totals{},
				Unmeasured: []string{},
				Warnings:   []string{`profile path "example.com/other/a.go" does not belong to module "alpha"`},
			},
		},
		{
			name: "incoming warnings are carried through and deduplicated",
			selected: []Selected{
				{Module: "alpha", File: udiff.File{Path: "scripts/alpha/a.go", Lines: []int{5}}},
			},
			profiles: map[string]cover.Profile{
				"alpha": {Mode: "set", Files: map[string]map[int]bool{"alpha/a.go": {5: false}}},
			},
			warnings: []string{"malformed hunk", "malformed hunk"},
			want: Report{
				Modules: []Module{{
					Module: "alpha",
					Total:  Totals{Statements: 1, Covered: 0, Coverage: 0},
					Files: []File{{
						Path: "scripts/alpha/a.go", Statements: 1, Covered: 0,
						UncoveredLines: []int{5},
					}},
				}},
				Total:      Totals{Statements: 1, Covered: 0, Coverage: 0},
				Unmeasured: []string{},
				Warnings:   []string{"malformed hunk"},
			},
		},
		{
			name:     "no changed statements",
			selected: []Selected{},
			profiles: map[string]cover.Profile{},
			want: Report{
				Modules:    []Module{},
				Total:      Totals{},
				Unmeasured: []string{},
				Warnings:   []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Join(tt.selected, tt.profiles, tt.warnings)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Join() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTotalsDerivations(t *testing.T) {
	t.Parallel()
	total := Totals{Statements: 8, Covered: 3, Coverage: 0.375}
	if got := total.Uncovered(); got != 5 {
		t.Errorf("Uncovered() = %d, want 5", got)
	}
	if got := total.Percent(); got != 37.5 {
		t.Errorf("Percent() = %v, want 37.5", got)
	}
}

// fakeRunner records the calls the collectors make so the command wiring can be
// asserted without a real git or go toolchain.
type fakeRunner struct {
	calls  []string
	out    []byte
	runErr error
}

func (f *fakeRunner) run(dir, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(append([]string{dir, name}, args...), " "))
	return f.out, f.runErr
}

func TestRepoRoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		out      string
		runErr   error
		wantRoot string
		wantErr  string
	}{
		{name: "trims the reported path", out: "/repo\n", wantRoot: "/repo"},
		{name: "git failure is reported", out: "not a git repository", runErr: errors.New("exit 128"), wantErr: "locate repository root"},
		{name: "empty output is reported", out: "\n", wantErr: "git reported no top level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeRunner{out: []byte(tt.out), runErr: tt.runErr}
			root, rootErr := RepoRoot(fake.run)
			if tt.wantErr != "" {
				if rootErr == nil {
					t.Fatalf("expected an error, got root %q", root)
				}
				if !strings.Contains(rootErr.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", rootErr.Error(), tt.wantErr)
				}
				return
			}
			if rootErr != nil {
				t.Fatalf("RepoRoot returned error: %v", rootErr)
			}
			if root != tt.wantRoot {
				t.Errorf("RepoRoot() = %q, want %q", root, tt.wantRoot)
			}
			if want := ". git rev-parse --show-toplevel"; fake.calls[0] != want {
				t.Errorf("call = %q, want %q", fake.calls[0], want)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	fake := &fakeRunner{out: []byte("diff --git a/a b/a\n")}
	got, diffErr := Diff(fake.run, "/repo", "origin/main")
	if diffErr != nil {
		t.Fatalf("Diff returned error: %v", diffErr)
	}
	if string(got) != "diff --git a/a b/a\n" {
		t.Errorf("Diff() = %q", got)
	}
	want := "/repo git -c core.quotePath=false diff --unified=0 origin/main...HEAD"
	if fake.calls[0] != want {
		t.Errorf("call = %q, want %q", fake.calls[0], want)
	}
}

func TestDiffReportsMissingBaseWithAHint(t *testing.T) {
	t.Parallel()
	fake := &fakeRunner{out: []byte("fatal: bad revision"), runErr: errors.New("exit 128")}
	_, diffErr := Diff(fake.run, "/repo", "origin/main")
	if diffErr == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(diffErr.Error(), "git fetch origin") {
		t.Errorf("error = %q, want it to hint at fetching", diffErr.Error())
	}
}

func TestWriteProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		coverpkg bool
		want     string
	}{
		{
			name: "default matches the CI invocation",
			want: "/repo/scripts/alpha go test ./... -count=1 -coverprofile=/tmp/alpha.out",
		},
		{
			name:     "coverpkg widens the measured set",
			coverpkg: true,
			want:     "/repo/scripts/alpha go test ./... -count=1 -coverprofile=/tmp/alpha.out -coverpkg=./...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeRunner{}
			if writeErr := WriteProfile(fake.run, "/repo", "alpha", "/tmp/alpha.out", tt.coverpkg); writeErr != nil {
				t.Fatalf("WriteProfile returned error: %v", writeErr)
			}
			if fake.calls[0] != tt.want {
				t.Errorf("call = %q, want %q", fake.calls[0], tt.want)
			}
		})
	}
}

func TestWriteProfileReportsFailure(t *testing.T) {
	t.Parallel()
	fake := &fakeRunner{out: []byte("FAIL\tcover-diff"), runErr: errors.New("exit 1")}
	writeErr := WriteProfile(fake.run, "/repo", "alpha", "/tmp/alpha.out", false)
	if writeErr == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(writeErr.Error(), "go test in scripts/alpha failed") {
		t.Errorf("error = %q", writeErr.Error())
	}
}
