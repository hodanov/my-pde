// Package diff compares the AI CLI configuration sources under ai-agents/ with
// their deployed copies (~/.claude, ~/.cursor, ~/.codex, ~/.copilot) read-only,
// classifying each entry as ok, drift or missing.
//
// It takes the same (mode, src, dest) contract as ai-agents/scripts/copy-entries.sh
// and enumerates entries with the same rules, so "what copy touches" and "what
// diff inspects" stay in lockstep. It never writes anything.
package diff

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// State is the classification of a single entry.
type State int

const (
	// StateOK means the source entry matches its deployed copy.
	StateOK State = iota
	// StateDrift means the deployed copy exists but its content differs.
	StateDrift
	// StateMissing means the entry has not been deployed yet.
	StateMissing
)

// String returns the lowercase label used in the summary output.
func (s State) String() string {
	switch s {
	case StateOK:
		return "ok"
	case StateDrift:
		return "drift"
	case StateMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// Entry is the comparison result for one source entry.
type Entry struct {
	// Label is the entry name relative to src, matching copy-entries.sh.
	Label string
	// State is ok, drift or missing.
	State State
	// Note carries drift detail (per-file differences) when State is StateDrift.
	Note string
}

// item is an enumerated source entry before classification.
type item struct {
	path  string // absolute or src-relative source path
	label string // name under dest, per copy-entries.sh rules
	isDir bool   // true for skills entries (directory trees)
}

// Classify enumerates the src entries for mode and compares each with dest,
// returning results sorted by label. mode is skills, agents or settings.
func Classify(mode, src, dest string) ([]Entry, error) {
	if _, statErr := os.Stat(src); statErr != nil {
		return nil, fmt.Errorf("source directory not found: %s", src)
	}
	items, enumErr := enumerate(mode, src)
	if enumErr != nil {
		return nil, enumErr
	}
	entries := make([]Entry, 0, len(items))
	for _, it := range items {
		entry, classErr := classify(it, dest)
		if classErr != nil {
			return nil, classErr
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Label < entries[j].Label })
	return entries, nil
}

// AnyDivergent reports whether any entry is drift or missing.
func AnyDivergent(entries []Entry) bool {
	for _, e := range entries {
		if e.State != StateOK {
			return true
		}
	}
	return false
}

// enumerate lists the source entries for mode, mirroring copy-entries.sh:
//   - skills:   top-level directories (label = basename)
//   - agents:   top-level *.md files (label = basename)
//   - settings: every file, recursively (label = path relative to src)
func enumerate(mode, src string) ([]item, error) {
	switch mode {
	case "skills":
		return topLevel(src, func(d fs.DirEntry) bool { return d.IsDir() }, true)
	case "agents":
		return topLevel(src, func(d fs.DirEntry) bool {
			// find -type f matches regular files only, so symlinked *.md are skipped.
			return d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".md")
		}, false)
	case "settings":
		return settingsFiles(src)
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}
}

// topLevel returns the direct children of src that satisfy keep, labelled by
// basename. isDir records whether the entries are directory trees.
func topLevel(src string, keep func(fs.DirEntry) bool, isDir bool) ([]item, error) {
	dirEntries, readErr := os.ReadDir(src)
	if readErr != nil {
		return nil, readErr
	}
	var items []item
	for _, d := range dirEntries {
		if keep(d) {
			items = append(items, item{
				path:  filepath.Join(src, d.Name()),
				label: d.Name(),
				isDir: isDir,
			})
		}
	}
	return items, nil
}

// settingsFiles returns every file under src labelled by its path relative to src.
func settingsFiles(src string) ([]item, error) {
	var items []item
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Mirror copy-entries.sh `find -type f`: descend into directories but only
		// enumerate regular files, skipping symlinks and other special entries.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		items = append(items, item{path: path, label: rel, isDir: false})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return items, nil
}

// classify compares one source item with dest/<label>.
func classify(it item, dest string) (Entry, error) {
	target := filepath.Join(dest, it.label)
	if it.isDir {
		return classifyDir(it, target)
	}
	return classifyFile(it, target)
}

// classifyFile compares a single source file with its deployed target.
func classifyFile(it item, target string) (Entry, error) {
	if !exists(target) {
		return Entry{Label: it.label, State: StateMissing}, nil
	}
	same, cmpErr := sameFile(it.path, target)
	if cmpErr != nil {
		return Entry{}, cmpErr
	}
	if !same {
		return Entry{Label: it.label, State: StateDrift, Note: "content differs"}, nil
	}
	return Entry{Label: it.label, State: StateOK}, nil
}

// classifyDir compares a source directory tree with its deployed target,
// reporting per-file drift/missing/extra as the note.
func classifyDir(it item, target string) (Entry, error) {
	if !isDir(target) {
		return Entry{Label: it.label, State: StateMissing}, nil
	}
	notes, cmpErr := compareDir(it.path, target)
	if cmpErr != nil {
		return Entry{}, cmpErr
	}
	if len(notes) > 0 {
		return Entry{Label: it.label, State: StateDrift, Note: strings.Join(notes, ", ")}, nil
	}
	return Entry{Label: it.label, State: StateOK}, nil
}

// compareDir returns per-file differences between srcDir and destDir:
//   - "missing: r" a file present in src but not in dest
//   - "drift: r"   a file whose content differs
//   - "extra: r"   a file present in dest but not in src (would be removed on copy)
func compareDir(srcDir, destDir string) ([]string, error) {
	srcFiles, srcErr := relFiles(srcDir)
	if srcErr != nil {
		return nil, srcErr
	}
	destFiles, destErr := relFiles(destDir)
	if destErr != nil {
		return nil, destErr
	}
	inSrc := make(map[string]bool, len(srcFiles))
	var notes []string
	for _, rel := range srcFiles {
		inSrc[rel] = true
		destPath := filepath.Join(destDir, rel)
		if !exists(destPath) {
			notes = append(notes, "missing: "+rel)
			continue
		}
		same, cmpErr := sameFile(filepath.Join(srcDir, rel), destPath)
		if cmpErr != nil {
			return nil, cmpErr
		}
		if !same {
			notes = append(notes, "drift: "+rel)
		}
	}
	for _, rel := range destFiles {
		if !inSrc[rel] {
			notes = append(notes, "extra: "+rel)
		}
	}
	sort.Strings(notes)
	return notes, nil
}

// relFiles returns the sorted paths of every file under dir, relative to dir.
func relFiles(dir string) ([]string, error) {
	var files []string
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Regular files only, matching the copy enumeration; skip symlinks.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(files)
	return files, nil
}

// exists reports whether path exists.
func exists(path string) bool {
	_, statErr := os.Stat(path)
	return statErr == nil
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, statErr := os.Stat(path)
	return statErr == nil && info.IsDir()
}

// sameFile reports whether two files have identical content (sha256).
func sameFile(a, b string) (bool, error) {
	sumA, sumErrA := hashFile(a)
	if sumErrA != nil {
		return false, sumErrA
	}
	sumB, sumErrB := hashFile(b)
	if sumErrB != nil {
		return false, sumErrB
	}
	return bytes.Equal(sumA, sumB), nil
}

// hashFile returns the sha256 digest of the file at path.
func hashFile(path string) ([]byte, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}
