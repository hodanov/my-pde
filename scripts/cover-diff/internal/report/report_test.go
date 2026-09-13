package report

import (
	"strings"
	"testing"

	"cover-diff/internal/patch"
)

func threshold(v float64) *float64 { return new(v) }

func TestRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		lines []int
		want  string
	}{
		{name: "empty", lines: []int{}, want: ""},
		{name: "single line", lines: []int{7}, want: "7"},
		{name: "one run", lines: []int{77, 78, 79}, want: "77-79"},
		{name: "runs and singles", lines: []int{48, 49, 50, 55}, want: "48-50,55"},
		{name: "pair is a run", lines: []int{10, 11}, want: "10-11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ranges(tt.lines); got != tt.want {
				t.Errorf("ranges(%v) = %q, want %q", tt.lines, got, tt.want)
			}
		})
	}
}

func TestBelow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		report patch.Report
		want   bool
	}{
		{
			name:   "no threshold never fails",
			report: patch.Report{Total: patch.Totals{Statements: 4, Covered: 0}},
		},
		{
			name: "under the threshold fails",
			report: patch.Report{
				Total:     patch.Totals{Statements: 4, Covered: 3, Coverage: 0.75},
				Threshold: threshold(80),
			},
			want: true,
		},
		{
			name: "exactly at the threshold passes",
			report: patch.Report{
				Total:     patch.Totals{Statements: 5, Covered: 4, Coverage: 0.8},
				Threshold: threshold(80),
			},
		},
		{
			name:   "a patch with no statements clears any threshold",
			report: patch.Report{Total: patch.Totals{}, Threshold: threshold(100)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Below(&tt.report); got != tt.want {
				t.Errorf("Below() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		report patch.Report
		want   string
	}{
		{
			name: "modules, uncovered ranges and the patch total",
			report: patch.Report{
				Modules: []patch.Module{
					{
						Module: "ai-bridge",
						Total:  patch.Totals{Statements: 34, Covered: 21, Coverage: 0.6176},
						Files: []patch.File{
							{Path: "scripts/ai-bridge/internal/usecase/replay.go", Statements: 20, Covered: 16, UncoveredLines: []int{48, 49, 50, 55}},
							{Path: "scripts/ai-bridge/internal/infra/tmux.go", Statements: 14, Covered: 5, UncoveredLines: []int{77, 78, 79, 80, 81, 82}},
						},
					},
					{
						Module: "pipeline-metrics",
						Total:  patch.Totals{Statements: 12, Covered: 12, Coverage: 1},
						Files:  []patch.File{{Path: "scripts/pipeline-metrics/a.go", Statements: 12, Covered: 12, UncoveredLines: []int{}}},
					},
				},
				Total:      patch.Totals{Statements: 46, Covered: 33, Coverage: 0.7174},
				Unmeasured: []string{},
				Warnings:   []string{},
				Threshold:  threshold(80),
			},
			want: "scripts/ai-bridge         changed 34 / covered 21 / uncovered 13  (61.8%)\n" +
				"  internal/usecase/replay.go:48-50,55\n" +
				"  internal/infra/tmux.go:77-82\n" +
				"scripts/pipeline-metrics  changed 12 / covered 12 / uncovered 0  (100.0%)\n" +
				"----\n" +
				"patch coverage: 33/46 (71.7%)  threshold 80% -> FAIL\n",
		},
		{
			name: "nothing to measure",
			report: patch.Report{
				Modules: []patch.Module{}, Unmeasured: []string{}, Warnings: []string{},
			},
			want: "no changed statements under scripts/\n",
		},
		{
			name: "unmeasured modules and warnings are surfaced",
			report: patch.Report{
				Modules:    []patch.Module{},
				Unmeasured: []string{"scaffold"},
				Warnings:   []string{`a.go: malformed hunk header "@@ broken @@"`},
			},
			want: "----\n" +
				"patch coverage: 0/0 (0.0%)\n" +
				"unmeasured modules (no coverage profile): scaffold\n" +
				"warning: a.go: malformed hunk header \"@@ broken @@\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Text(&tt.report); got != tt.want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestTextPassingThresholdSaysPass(t *testing.T) {
	t.Parallel()
	r := patch.Report{
		Modules:    []patch.Module{},
		Total:      patch.Totals{Statements: 5, Covered: 5, Coverage: 1},
		Unmeasured: []string{},
		Warnings:   []string{},
		Threshold:  threshold(80),
	}
	if got := Text(&r); !strings.Contains(got, "threshold 80% -> PASS") {
		t.Errorf("Text() = %q, want a passing verdict", got)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	r := patch.Report{
		Modules: []patch.Module{{
			Module: "alpha",
			Total:  patch.Totals{Statements: 2, Covered: 1, Coverage: 0.5},
			Files: []patch.File{{
				Path: "scripts/alpha/a.go", Statements: 2, Covered: 1, UncoveredLines: []int{7},
			}},
		}},
		Total:      patch.Totals{Statements: 2, Covered: 1, Coverage: 0.5},
		Unmeasured: []string{},
		Warnings:   []string{},
		Threshold:  threshold(50),
	}
	got, jsonErr := JSON(&r)
	if jsonErr != nil {
		t.Fatalf("JSON returned error: %v", jsonErr)
	}
	want := `{
  "modules": [
    {
      "module": "alpha",
      "total": {
        "statements": 2,
        "covered": 1,
        "coverage": 0.5
      },
      "files": [
        {
          "path": "scripts/alpha/a.go",
          "statements": 2,
          "covered": 1,
          "uncovered_lines": [
            7
          ]
        }
      ]
    }
  ],
  "total": {
    "statements": 2,
    "covered": 1,
    "coverage": 0.5
  },
  "unmeasured_modules": [],
  "warnings": [],
  "threshold": 50
}`
	if string(got) != want {
		t.Errorf("JSON() =\n%s\nwant\n%s", got, want)
	}
}

// TestJSONRendersEmptyCollectionsAsArrays keeps `null` out of the collections a
// consumer iterates. Only threshold is nullable, and there it means "unset".
func TestJSONRendersEmptyCollectionsAsArrays(t *testing.T) {
	t.Parallel()
	r := patch.Report{Modules: []patch.Module{}, Unmeasured: []string{}, Warnings: []string{}}
	got, jsonErr := JSON(&r)
	if jsonErr != nil {
		t.Fatalf("JSON returned error: %v", jsonErr)
	}
	for _, key := range []string{"modules", "unmeasured_modules", "warnings"} {
		if !strings.Contains(string(got), `"`+key+`": []`) {
			t.Errorf("JSON() = %s, want %q rendered as an empty array", got, key)
		}
	}
	if !strings.Contains(string(got), `"threshold": null`) {
		t.Errorf("JSON() = %s, want an unset threshold rendered as null", got)
	}
}
