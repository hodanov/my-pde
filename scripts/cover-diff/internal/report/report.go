// Package report renders a patch report as text for a reader or as JSON for a
// later consumer.
//
// Both renderers are pure functions over a patch.Report and read no clock, so
// the same report always renders byte for byte the same output.
package report

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"cover-diff/internal/patch"
)

// Text renders the human-facing report: one line per module, the uncovered
// statement lines beneath it, and the patch total last.
func Text(r *patch.Report) string {
	b := &strings.Builder{}
	if r.Total.Statements == 0 && len(r.Unmeasured) == 0 {
		fmt.Fprintf(b, "no changed statements under %s/\n", patch.ScriptsDir)
		writeNotes(b, r)
		return b.String()
	}

	width := moduleWidth(r.Modules)
	for _, module := range r.Modules {
		fmt.Fprintf(b, "%-*s  changed %d / covered %d / uncovered %d  (%s)\n",
			width, path.Join(patch.ScriptsDir, module.Module),
			module.Total.Statements, module.Total.Covered, module.Total.Uncovered(),
			percent(module.Total))
		for _, file := range module.Files {
			if len(file.UncoveredLines) == 0 {
				continue
			}
			fmt.Fprintf(b, "  %s:%s\n",
				strings.TrimPrefix(file.Path, path.Join(patch.ScriptsDir, module.Module)+"/"),
				ranges(file.UncoveredLines))
		}
	}

	fmt.Fprintln(b, "----")
	fmt.Fprintf(b, "patch coverage: %d/%d (%s)%s\n",
		r.Total.Covered, r.Total.Statements, percent(r.Total), verdict(r))
	writeNotes(b, r)
	return b.String()
}

// JSON renders the whole report for a later consumer.
func JSON(r *patch.Report) ([]byte, error) {
	out, marshalErr := json.MarshalIndent(r, "", "  ")
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal report: %w", marshalErr)
	}
	return out, nil
}

func writeNotes(b *strings.Builder, r *patch.Report) {
	if len(r.Unmeasured) > 0 {
		fmt.Fprintf(b, "unmeasured modules (no coverage profile): %s\n", strings.Join(r.Unmeasured, ", "))
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(b, "warning: %s\n", warning)
	}
}

// verdict states the threshold outcome, and is empty when none was demanded.
func verdict(r *patch.Report) string {
	if r.Threshold == nil {
		return ""
	}
	outcome := "PASS"
	if Below(r) {
		outcome = "FAIL"
	}
	return fmt.Sprintf("  threshold %s%% -> %s", trim(*r.Threshold), outcome)
}

// Below reports whether the patch total misses the demanded threshold. A patch
// with no changed statement clears any threshold: there is nothing to cover.
func Below(r *patch.Report) bool {
	if r.Threshold == nil || r.Total.Statements == 0 {
		return false
	}
	return r.Total.Percent() < *r.Threshold
}

func moduleWidth(modules []patch.Module) int {
	width := 0
	for _, module := range modules {
		width = max(width, len(path.Join(patch.ScriptsDir, module.Module)))
	}
	return width
}

func percent(t patch.Totals) string {
	return fmt.Sprintf("%.1f%%", t.Percent())
}

// trim renders a threshold without trailing zeros so "80" stays "80".
func trim(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// ranges collapses sorted line numbers into "1-3,7" notation.
func ranges(lines []int) string {
	if len(lines) == 0 {
		return ""
	}
	parts := make([]string, 0, len(lines))
	start, prev := lines[0], lines[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
			return
		}
		parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
	}
	for _, line := range lines[1:] {
		if line == prev+1 {
			prev = line
			continue
		}
		flush()
		start, prev = line, line
	}
	flush()
	return strings.Join(parts, ",")
}
