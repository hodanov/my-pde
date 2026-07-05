// Package gen builds the file set for a new scripts/ Go module from live
// templates in the repository, so a new module inherits the current CI wiring
// and mise task conventions instead of a hand-maintained, drift-prone copy.
//
// The Go skeleton (go.mod / cmd / README) is a minimal fixed template — it
// carries no pinned action SHAs and a blank starting point is the point. The
// CI workflow and mise task block are rendered from the live files of an
// existing "template" module (config-diff by default) by replacing the module
// name token, so pinned SHAs and workflow structure follow the repository.
//
// NewSpec and the Plan method are pure: they take reader/exists callbacks and
// never touch the filesystem themselves, so they are fully exercised from
// t.TempDir()-independent table-driven tests.
package gen

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// nameRe constrains a module name to a lowercase kebab-case token so it is a
// safe path segment, Go module path, and replacement token.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// spec describes a validated scaffold request. The type is unexported so the
// only way for callers to obtain one is NewSpec, which means every spec in
// existence satisfies the preconditions (valid names, --from present, new
// module absent) — including that a zero-value bypass is impossible outside
// this package.
type spec struct {
	// name is the new module name (e.g. "log-tail").
	name string
	// from is the existing module whose CI/mise are used as the template.
	from string
}

// NewSpec validates a scaffold request and returns a spec that is guaranteed
// to satisfy the generation preconditions: both names are lowercase
// kebab-case, the --from module exists under scripts/ (it is the template to
// read), and the new module does not exist yet (generation never overwrites).
// the only way to obtain a spec, so the preconditions above always hold.
//
//nolint:revive // Returning the unexported type is deliberate: it makes NewSpec
func NewSpec(name, from string, exists ExistsFunc) (spec, error) {
	if !nameRe.MatchString(name) {
		return spec{}, fmt.Errorf("invalid module name %q: want lowercase kebab-case (e.g. log-tail)", name)
	}
	if !nameRe.MatchString(from) {
		return spec{}, fmt.Errorf("invalid --from module %q: want lowercase kebab-case", from)
	}
	if name == from {
		return spec{}, errors.New("module name and --from must differ")
	}
	if !exists(modulePath(from, "")) {
		return spec{}, fmt.Errorf("--from module %q not found under scripts/", from)
	}
	if exists(modulePath(name, "")) {
		return spec{}, fmt.Errorf("%s already exists, refusing to overwrite", modulePath(name, ""))
	}
	return spec{name: name, from: from}, nil
}

// File is one file to be written, with a repository-relative path.
type File struct {
	Path    string
	Content string
}

// Result is the full plan for a scaffold request.
type Result struct {
	// Files is the set of files to create (never mutating existing ones).
	Files []File
	// MiseBlock is the mise task block to append to mise.toml by hand.
	MiseBlock string
}

// ReadFunc reads a repository-relative file, returning its bytes.
type ReadFunc func(relPath string) ([]byte, error)

// ExistsFunc reports whether a repository-relative path already exists.
type ExistsFunc func(relPath string) bool

// Plan assembles the files and mise block for the spec, reading the template
// module's live CI workflow and mise.toml through read. It returns an error if
// a template cannot be read or any target path already exists (generation is
// additive and never overwrites). Name validity is already guaranteed by
// NewSpec.
func (s spec) Plan(read ReadFunc, exists ExistsFunc) (Result, error) {
	ciSrcPath := workflowPath(s.from)
	ciSrc, readCIErr := read(ciSrcPath)
	if readCIErr != nil {
		return Result{}, fmt.Errorf("read template workflow %s: %w", ciSrcPath, readCIErr)
	}

	miseSrc, readMiseErr := read("mise.toml")
	if readMiseErr != nil {
		return Result{}, fmt.Errorf("read mise.toml: %w", readMiseErr)
	}
	miseSection, sectionErr := extractMiseSection(string(miseSrc), s.from)
	if sectionErr != nil {
		return Result{}, sectionErr
	}

	files := []File{
		{Path: modulePath(s.name, "go.mod"), Content: goModContent(s.name)},
		{Path: modulePath(s.name, path.Join("cmd", s.name, "main.go")), Content: mainContent(s.name)},
		{Path: modulePath(s.name, path.Join("cmd", s.name, "main_test.go")), Content: mainTestContent(s.name)},
		{Path: modulePath(s.name, "README.md"), Content: readmeContent(s.name)},
		{Path: workflowPath(s.name), Content: renderToken(string(ciSrc), s.from, s.name)},
	}

	if collideErr := ensureNoCollisions(files, exists); collideErr != nil {
		return Result{}, collideErr
	}

	return Result{
		Files:     files,
		MiseBlock: renderToken(miseSection, s.from, s.name),
	}, nil
}

// ensureNoCollisions fails if any planned file already exists.
func ensureNoCollisions(files []File, exists ExistsFunc) error {
	for _, f := range files {
		if exists(f.Path) {
			return fmt.Errorf("target already exists, refusing to overwrite: %s", f.Path)
		}
	}
	return nil
}

// workflowPath returns the CI workflow path for a module. The filename uses
// underscores (ci_config_diff.yml) while the in-file token stays kebab-case.
func workflowPath(module string) string {
	return path.Join(".github", "workflows", "ci_"+strings.ReplaceAll(module, "-", "_")+".yml")
}

// modulePath joins a repository-relative path under scripts/<module>/.
func modulePath(module, rel string) string {
	return path.Join("scripts", module, rel)
}

// renderToken replaces every occurrence of the from module token with to.
func renderToken(src, from, to string) string {
	return strings.ReplaceAll(src, from, to)
}

// extractMiseSection returns the "# ---- <from> (Go) ----" marker block from
// mise.toml, up to (but excluding) the next "# ---- " marker.
func extractMiseSection(mise, from string) (string, error) {
	marker := "# ---- " + from + " (Go) ----"
	start := strings.Index(mise, marker)
	if start < 0 {
		return "", fmt.Errorf("mise.toml has no %q section to use as a template", marker)
	}
	rest := mise[start+len(marker):]
	next := strings.Index(rest, "# ---- ")
	section := rest
	if next >= 0 {
		section = rest[:next]
	}
	return marker + strings.TrimRight(section, "\n") + "\n", nil
}

// goModContent renders the module's go.mod, matching the Go version other
// scripts/ modules pin.
func goModContent(name string) string {
	return fmt.Sprintf("module %s\n\ngo 1.26\n", name)
}

// mainContent renders a minimal, testable command skeleton.
func mainContent(name string) string {
	return fmt.Sprintf(`// Command %[1]s is a scaffold-generated scripts/ tool. Replace this
// description and the execute body with the real behaviour.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout))
}

// execute is the testable entry point: it returns the process exit code so
// tests can drive the command without spawning a process.
func execute(args []string, out io.Writer) int {
	_, _ = fmt.Fprintf(out, "%[1]s: %%d args\n", len(args))
	return 0
}
`, name)
}

// mainTestContent renders a table-driven test for the skeleton, following the
// scripts/ai-bridge/AGENTS.md conventions (t.Parallel on function and subtests,
// no err reuse).
func mainTestContent(name string) string {
	return fmt.Sprintf(`package main

import (
	"bytes"
	"testing"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{name: "no args", args: nil, wantCode: 0, wantOut: "%[1]s: 0 args\n"},
		{name: "two args", args: []string{"a", "b"}, wantCode: 0, wantOut: "%[1]s: 2 args\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := execute(tt.args, &out)
			if code != tt.wantCode {
				t.Fatalf("execute code = %%d, want %%d", code, tt.wantCode)
			}
			if out.String() != tt.wantOut {
				t.Fatalf("execute out = %%q, want %%q", out.String(), tt.wantOut)
			}
		})
	}
}
`, name)
}

// readmeContent renders a starter README for the generated module.
func readmeContent(name string) string {
	return fmt.Sprintf(`# %[1]s

`+"`scripts/%[1]s`"+` の Go ツール雛形。scaffold で生成された。CI ワークフロー
（`+"`.github/workflows/ci_%[2]s.yml`"+`）と mise タスク（`+"`%[1]s:build|test|lint|clean`"+`）が
生成時に配線されている。

## 次の手順

1. `+"`cmd/%[1]s/main.go`"+` の `+"`execute`"+` に実装を書く。
2. mise タスクブロックを `+"`mise.toml`"+` 末尾へ貼り、`+"`go:test`"+` / `+"`go:lint`"+` の
   `+"`depends`"+` に `+"`%[1]s:test`"+` / `+"`%[1]s:lint`"+` を追加する。
3. `+"`mise run %[1]s:test`"+` と `+"`mise run %[1]s:lint`"+` で検証する。
`, name, strings.ReplaceAll(name, "-", "_"))
}
