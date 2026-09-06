package udiff

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		diff         string
		wantFiles    []File
		wantWarnings []string
	}{
		{
			name: "unified=0 addition and rewrite",
			diff: "diff --git a/scripts/x/a.go b/scripts/x/a.go\n" +
				"--- a/scripts/x/a.go\n" +
				"+++ b/scripts/x/a.go\n" +
				"@@ -10,0 +11,2 @@ func f() {\n" +
				"+\tone()\n" +
				"+\ttwo()\n" +
				"@@ -20,1 +22,1 @@\n" +
				"-\told()\n" +
				"+\tnew()\n",
			wantFiles:    []File{{Path: "scripts/x/a.go", Lines: []int{11, 12, 22}}},
			wantWarnings: []string{},
		},
		{
			name: "context lines advance the counter",
			diff: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1,4 +1,5 @@\n" +
				" package main\n" +
				" \n" +
				"+var added = 1\n" +
				" \n" +
				" func main() {}\n",
			wantFiles:    []File{{Path: "a.go", Lines: []int{3}}},
			wantWarnings: []string{},
		},
		{
			name: "deleted file contributes nothing",
			diff: "diff --git a/gone.go b/gone.go\n" +
				"deleted file mode 100644\n" +
				"--- a/gone.go\n" +
				"+++ /dev/null\n" +
				"@@ -1,2 +0,0 @@\n" +
				"-package main\n" +
				"-\n",
			wantFiles:    []File{},
			wantWarnings: []string{},
		},
		{
			name: "rename and mode only blocks contribute nothing",
			diff: "diff --git a/old.go b/new.go\n" +
				"similarity index 100%\n" +
				"rename from old.go\n" +
				"rename to new.go\n" +
				"diff --git a/exec.go b/exec.go\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantFiles:    []File{},
			wantWarnings: []string{},
		},
		{
			name: "no newline marker is ignored",
			diff: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"\\ No newline at end of file\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			wantFiles:    []File{{Path: "a.go", Lines: []int{1}}},
			wantWarnings: []string{},
		},
		{
			name: "added lines that look like headers stay content",
			diff: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -0,0 +1,2 @@\n" +
				"+++ b/not-a-header.go\n" +
				"+@@ -1 +1 @@\n",
			wantFiles:    []File{{Path: "a.go", Lines: []int{1, 2}}},
			wantWarnings: []string{},
		},
		{
			name: "quoted path is unquoted",
			diff: "diff --git \"a/scripts/x/\\346\\227\\245.go\" \"b/scripts/x/\\346\\227\\245.go\"\n" +
				"--- \"a/scripts/x/\\346\\227\\245.go\"\n" +
				"+++ \"b/scripts/x/\\346\\227\\245.go\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+package x\n",
			wantFiles:    []File{{Path: "scripts/x/日.go", Lines: []int{1}}},
			wantWarnings: []string{},
		},
		{
			name: "tab suffixed path keeps only the path",
			diff: "--- a/a.go\t2026-09-06 00:00:00\n" +
				"+++ b/a.go\t2026-09-06 00:00:01\n" +
				"@@ -0,0 +1 @@\n" +
				"+package main\n",
			wantFiles:    []File{{Path: "a.go", Lines: []int{1}}},
			wantWarnings: []string{},
		},
		{
			name: "malformed hunk header warns instead of dropping silently",
			diff: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ broken @@\n" +
				"@@ -0,0 +5 @@\n" +
				"+ok()\n",
			wantFiles:    []File{{Path: "a.go", Lines: []int{5}}},
			wantWarnings: []string{`a.go: malformed hunk header "@@ broken @@"`},
		},
		{
			name:         "empty diff",
			diff:         "",
			wantFiles:    []File{},
			wantWarnings: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Parse([]byte(tt.diff))
			want := Result{Files: tt.wantFiles, Warnings: tt.wantWarnings}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Parse() = %+v, want %+v", got, want)
			}
		})
	}
}
