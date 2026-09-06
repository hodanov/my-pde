// Package udiff parses a unified diff into the set of line numbers each file
// gained on the new side.
//
// It is a pure parser: it never runs git and never touches the filesystem, so
// the whole extraction can be pinned with fixture text. Hunks it cannot
// interpret are reported as warnings instead of being silently dropped.
package udiff

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// devNull is the path git uses for the missing side of an add or a delete.
const devNull = "/dev/null"

// File holds the new-side line numbers a diff added or rewrote in one file.
type File struct {
	// Path is the repository-relative path of the new side.
	Path string
	// Lines are the added line numbers, sorted and deduplicated.
	Lines []int
}

// Result is the outcome of parsing one diff.
type Result struct {
	// Files are the changed files, sorted by path. Files whose hunks added no
	// line (rename-only, mode-only, pure deletions) are omitted.
	Files []File
	// Warnings names the hunks that could not be interpreted.
	Warnings []string
}

// Parse reads a unified diff and returns the new-side lines each file gained.
// A diff produced with --unified=0 carries no context lines, but context is
// tolerated so a diff taken with the default -U3 parses the same way.
func Parse(diff []byte) Result {
	p := parser{lines: map[string]map[int]bool{}}
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		p.step(scanner.Text())
	}
	if scanErr := scanner.Err(); scanErr != nil {
		p.warnings = append(p.warnings, fmt.Sprintf("read diff: %v", scanErr))
	}
	return p.result()
}

// parser carries the file and hunk currently being read. remOld and remNew are
// the body lines the current hunk header still promises; while either is left,
// a line is hunk content and never a header, which keeps added source lines
// that happen to start with "+++" or "@@" from being mistaken for one.
type parser struct {
	path     string
	next     int
	remOld   int
	remNew   int
	lines    map[string]map[int]bool
	warnings []string
}

func (p *parser) step(line string) {
	if p.inHunk() {
		p.body(line)
		return
	}
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.path = ""
	case strings.HasPrefix(line, "+++ "):
		p.startFile(strings.TrimPrefix(line, "+++ "))
	case strings.HasPrefix(line, "@@"):
		p.startHunk(line)
	}
}

func (p *parser) inHunk() bool { return p.remOld > 0 || p.remNew > 0 }

// startFile records the new-side path of the file block that follows. A
// /dev/null target means the file was deleted, so nothing is collected for it.
func (p *parser) startFile(target string) {
	if idx := strings.Index(target, "\t"); idx >= 0 {
		target = target[:idx]
	}
	if target == devNull {
		p.path = ""
		return
	}
	path, unquoteErr := unquotePath(target)
	if unquoteErr != nil {
		p.warnings = append(p.warnings, fmt.Sprintf("unreadable path %s: %v", target, unquoteErr))
		p.path = ""
		return
	}
	p.path = strings.TrimPrefix(path, "b/")
	if _, seen := p.lines[p.path]; !seen {
		p.lines[p.path] = map[int]bool{}
	}
}

// startHunk reads the new-side start line and both body lengths out of an
// "@@ -a,b +c,d @@" header.
func (p *parser) startHunk(header string) {
	if p.path == "" {
		return
	}
	h, parseErr := parseHunkHeader(header)
	if parseErr != nil {
		p.warnings = append(p.warnings, fmt.Sprintf("%s: %v", p.path, parseErr))
		return
	}
	p.next = h.newStart
	p.remOld = h.oldCount
	p.remNew = h.newCount
}

// body consumes one hunk body line and advances the new-side line counter.
func (p *parser) body(line string) {
	switch {
	case strings.HasPrefix(line, `\`):
	case strings.HasPrefix(line, "+"):
		p.lines[p.path][p.next] = true
		p.next++
		p.remNew--
	case strings.HasPrefix(line, "-"):
		p.remOld--
	default:
		p.next++
		p.remNew--
		p.remOld--
	}
	p.remOld = max(p.remOld, 0)
	p.remNew = max(p.remNew, 0)
}

func (p *parser) result() Result {
	files := make([]File, 0, len(p.lines))
	for path, set := range p.lines {
		if len(set) == 0 {
			continue
		}
		nums := make([]int, 0, len(set))
		for n := range set {
			nums = append(nums, n)
		}
		slices.Sort(nums)
		files = append(files, File{Path: path, Lines: nums})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	warnings := p.warnings
	if warnings == nil {
		warnings = []string{}
	}
	return Result{Files: files, Warnings: warnings}
}

// hunk is the new- and old-side geometry declared by one hunk header.
type hunk struct {
	newStart int
	newCount int
	oldCount int
}

func parseHunkHeader(header string) (hunk, error) {
	rest, _, ok := strings.Cut(strings.TrimPrefix(header, "@@"), "@@")
	if !ok {
		return hunk{}, fmt.Errorf("malformed hunk header %q", header)
	}
	var h hunk
	var sawOld, sawNew bool
	for field := range strings.FieldsSeq(rest) {
		switch {
		case strings.HasPrefix(field, "-") && !sawOld:
			_, count, rangeErr := parseRange(field[1:])
			if rangeErr != nil {
				return hunk{}, fmt.Errorf("malformed hunk header %q", header)
			}
			h.oldCount = count
			sawOld = true
		case strings.HasPrefix(field, "+") && !sawNew:
			start, count, rangeErr := parseRange(field[1:])
			if rangeErr != nil {
				return hunk{}, fmt.Errorf("malformed hunk header %q", header)
			}
			h.newStart = start
			h.newCount = count
			sawNew = true
		}
	}
	if !sawOld || !sawNew {
		return hunk{}, fmt.Errorf("malformed hunk header %q", header)
	}
	return h, nil
}

// parseRange reads a "start,count" range, where an absent count means one line.
func parseRange(text string) (start, count int, err error) {
	startText, countText, hasCount := strings.Cut(text, ",")
	first, startErr := strconv.Atoi(startText)
	if startErr != nil {
		return 0, 0, startErr
	}
	if !hasCount {
		return first, 1, nil
	}
	length, countErr := strconv.Atoi(countText)
	if countErr != nil {
		return 0, 0, countErr
	}
	return first, length, nil
}

// unquotePath undoes the C-style quoting git applies to paths with special
// characters when core.quotePath is left on.
func unquotePath(path string) (string, error) {
	if !strings.HasPrefix(path, `"`) {
		return path, nil
	}
	return strconv.Unquote(path)
}
