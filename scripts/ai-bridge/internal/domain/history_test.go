package domain

import (
	"strings"
	"testing"
)

func TestFormatHistory(t *testing.T) {
	t.Parallel()
	records := []*Request{
		{Prompt: "newest", CWD: "/c", Timestamp: 1700000000},
		{Prompt: "middle\nsecond line", CWD: "/b", Timestamp: 1700000001},
		{Prompt: strings.Repeat("x", 150), CWD: "/a", Timestamp: 1700000002},
	}

	tests := []struct {
		name        string
		records     []*Request
		limit       int
		wantRows    int      // number of "index" rows (records shown)
		wantContain []string // substrings expected in output
	}{
		{
			name:        "empty yields placeholder",
			records:     nil,
			limit:       20,
			wantRows:    0,
			wantContain: []string{"(no history)"},
		},
		{
			name:        "limit caps rows",
			records:     records,
			limit:       2,
			wantRows:    2,
			wantContain: []string{"newest", "/c"},
		},
		{
			name:        "non-positive limit shows all",
			records:     records,
			limit:       0,
			wantRows:    3,
			wantContain: []string{"newest", "middle", "/a"},
		},
		{
			name:        "multiline prompt shows only first line",
			records:     records[1:2],
			limit:       20,
			wantRows:    1,
			wantContain: []string{"middle"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatHistory(tt.records, tt.limit)
			if tt.wantRows == 0 {
				if !strings.Contains(got, "(no history)") {
					t.Errorf("empty output = %q, want placeholder", got)
				}
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("output %q does not contain %q", got, want)
				}
			}
			if strings.Contains(got, "second line") {
				t.Errorf("output leaked second line of multiline prompt: %q", got)
			}
		})
	}
}
