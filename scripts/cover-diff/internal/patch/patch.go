// Package patch joins the lines a diff changed with the statements a coverage
// profile recorded, and owns the one exec boundary the tool has.
//
// The join is pure: Select and Join take parsed data and return a Report. The
// collection helpers take a Runner instead of calling os/exec themselves, so
// the command wiring can be exercised without a real git or go toolchain.
package patch

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"cover-diff/internal/cover"
	"cover-diff/internal/udiff"
)

// ScriptsDir is the directory every Go module of this repository lives under.
const ScriptsDir = "scripts"

// testSuffix names the files excluded from the denominator unconditionally:
// measuring how well a test file is covered says nothing about the change.
const testSuffix = "_test.go"

// Options narrows which changed files enter the report.
type Options struct {
	// Exclude drops changed files whose repository-relative path matches.
	Exclude *regexp.Regexp
	// ModFilter keeps only modules whose name contains this substring.
	ModFilter string
}

// Report is the machine-readable result. Every count is a statement count:
// changed lines that no profile block spans carry no statement and are left
// out of both the numerator and the denominator.
type Report struct {
	Modules []Module `json:"modules"`
	Total   Totals   `json:"total"`
	// Unmeasured lists modules that had changed statements but no profile.
	Unmeasured []string `json:"unmeasured_modules"`
	Warnings   []string `json:"warnings"`
	// Threshold is the minimum coverage the run demanded, if any.
	Threshold *float64 `json:"threshold"`
}

// Totals is the statement tally of a module or of the whole patch.
type Totals struct {
	Statements int `json:"statements"`
	Covered    int `json:"covered"`
	// Coverage is Covered/Statements rounded to four decimals, or 0 when the
	// patch changed no statement at all.
	Coverage float64 `json:"coverage"`
}

// Uncovered reports the statements the tests never reached.
func (t Totals) Uncovered() int { return t.Statements - t.Covered }

// Percent renders Coverage as a percentage.
func (t Totals) Percent() float64 { return t.Coverage * 100 }

// Module is one scripts/<name> module's share of the patch.
type Module struct {
	Module string `json:"module"`
	Total  Totals `json:"total"`
	Files  []File `json:"files"`
}

// File is one changed source file's share of the patch.
type File struct {
	// Path is repository-relative.
	Path       string `json:"path"`
	Statements int    `json:"statements"`
	Covered    int    `json:"covered"`
	// UncoveredLines are the changed statement lines no test executed, sorted.
	UncoveredLines []int `json:"uncovered_lines"`
}

// Selected is a changed file that belongs to a module and passed the filters.
type Selected struct {
	Module string
	File   udiff.File
}

// Select keeps the changed Go source files that belong to a scripts/ module and
// survive the filters, sorted by path. It is the single place the "what counts
// as a changed file" rule lives, so the module list Collect runs tests for and
// the files Join scores can never drift apart.
func Select(changed []udiff.File, opt Options) []Selected {
	out := make([]Selected, 0, len(changed))
	for _, file := range changed {
		module, ok := moduleOf(file.Path)
		if !ok {
			continue
		}
		if !strings.HasSuffix(file.Path, ".go") || strings.HasSuffix(file.Path, testSuffix) {
			continue
		}
		if opt.ModFilter != "" && !strings.Contains(module, opt.ModFilter) {
			continue
		}
		if opt.Exclude != nil && opt.Exclude.MatchString(file.Path) {
			continue
		}
		out = append(out, Selected{Module: module, File: file})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File.Path < out[j].File.Path })
	return out
}

// Modules returns the module names Select kept, in sorted order.
func Modules(selected []Selected) []string {
	var names []string
	for _, sel := range selected {
		if !slices.Contains(names, sel.Module) {
			names = append(names, sel.Module)
		}
	}
	slices.Sort(names)
	return names
}

// moduleOf maps "scripts/<module>/<rest>" to "<module>".
func moduleOf(repoPath string) (string, bool) {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 3 || parts[0] != ScriptsDir || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// Join scores the selected changed lines against the profile of each module.
// profiles is keyed by module name; a module missing from it is reported as
// unmeasured rather than as fully uncovered.
func Join(selected []Selected, profiles map[string]cover.Profile, warnings []string) Report {
	report := Report{
		Modules:    []Module{},
		Unmeasured: []string{},
		Warnings:   append([]string{}, warnings...),
	}

	for _, name := range Modules(selected) {
		profile, measured := profiles[name]
		if !measured {
			report.Unmeasured = append(report.Unmeasured, name)
			continue
		}
		statements, convWarnings := statementLines(name, profile)
		report.Warnings = append(report.Warnings, convWarnings...)

		module := Module{Module: name, Files: []File{}}
		for _, sel := range selected {
			if sel.Module != name {
				continue
			}
			file := scoreFile(sel.File, statements[sel.File.Path])
			if file.Statements == 0 {
				continue
			}
			module.Files = append(module.Files, file)
			module.Total.Statements += file.Statements
			module.Total.Covered += file.Covered
		}
		module.Total.Coverage = rate(module.Total)
		report.Modules = append(report.Modules, module)
		report.Total.Statements += module.Total.Statements
		report.Total.Covered += module.Total.Covered
	}

	report.Total.Coverage = rate(report.Total)
	slices.Sort(report.Warnings)
	report.Warnings = slices.Compact(report.Warnings)
	return report
}

// scoreFile classifies each changed line of one file. Lines absent from
// statements carry no statement and are counted on neither side.
func scoreFile(changed udiff.File, statements map[int]bool) File {
	file := File{Path: changed.Path, UncoveredLines: []int{}}
	for _, line := range changed.Lines {
		covered, isStatement := statements[line]
		if !isStatement {
			continue
		}
		file.Statements++
		if covered {
			file.Covered++
			continue
		}
		file.UncoveredLines = append(file.UncoveredLines, line)
	}
	return file
}

// statementLines re-keys a profile from import paths to repository-relative
// paths. A profile path is "<module path>/<rest>" while a diff path is
// "scripts/<module>/<rest>"; this repository names every module after its
// directory, so the first segment is the module name. A path that disagrees is
// reported rather than silently scored against nothing.
func statementLines(module string, profile cover.Profile) (lines map[string]map[int]bool, warnings []string) {
	out := make(map[string]map[int]bool, len(profile.Files))
	for profilePath, fileLines := range profile.Files {
		prefix, rest, ok := strings.Cut(profilePath, "/")
		if !ok || prefix != module {
			warnings = append(warnings, fmt.Sprintf("profile path %q does not belong to module %q", profilePath, module))
			continue
		}
		out[path.Join(ScriptsDir, module, rest)] = fileLines
	}
	return out, warnings
}

// rate returns covered/statements rounded to four decimals.
func rate(t Totals) float64 {
	if t.Statements == 0 {
		return 0
	}
	return math.Round(float64(t.Covered)/float64(t.Statements)*10000) / 10000
}
