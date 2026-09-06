package cover

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile string
		want    Profile
	}{
		{
			name:    "block ending at column one stops before that line",
			profile: "mode: set\nx/main.go:22.2,23.1 1 0\n",
			want: Profile{Mode: "set", Files: map[string]map[int]bool{
				"x/main.go": {22: false},
			}},
		},
		{
			name:    "block ending mid line includes that line",
			profile: "mode: set\nx/main.go:28.2,28.20 1 1\n",
			want: Profile{Mode: "set", Files: map[string]map[int]bool{
				"x/main.go": {28: true},
			}},
		},
		{
			name:    "multi line block marks every spanned line",
			profile: "mode: set\nx/main.go:29.3,32.1 3 1\n",
			want: Profile{Mode: "set", Files: map[string]map[int]bool{
				"x/main.go": {29: true, 30: true, 31: true},
			}},
		},
		{
			name:    "overlapping blocks are combined with or",
			profile: "mode: count\nx/a.go:5.2,7.1 2 0\nx/a.go:6.10,6.20 1 3\n",
			want: Profile{Mode: "count", Files: map[string]map[int]bool{
				"x/a.go": {5: false, 6: true},
			}},
		},
		{
			name:    "separate files stay separate",
			profile: "mode: atomic\nx/a.go:1.1,1.5 1 1\nx/b/c.go:9.1,9.5 1 0\n",
			want: Profile{Mode: "atomic", Files: map[string]map[int]bool{
				"x/a.go":   {1: true},
				"x/b/c.go": {9: false},
			}},
		},
		{
			name:    "profile with no blocks",
			profile: "mode: set\n",
			want:    Profile{Mode: "set", Files: map[string]map[int]bool{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, parseErr := Parse([]byte(tt.profile))
			if parseErr != nil {
				t.Fatalf("Parse() returned error: %v", parseErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseNeverRecordsLinesWithoutStatements pins the rule the whole report
// depends on: a line no block spans must not become a key, so callers can drop
// it from the denominator instead of reporting it as uncovered.
func TestParseNeverRecordsLinesWithoutStatements(t *testing.T) {
	t.Parallel()
	got, parseErr := Parse([]byte("mode: set\nx/main.go:10.2,10.9 1 0\n"))
	if parseErr != nil {
		t.Fatalf("Parse() returned error: %v", parseErr)
	}
	for _, line := range []int{1, 2, 9, 11, 12} {
		if _, present := got.Files["x/main.go"][line]; present {
			t.Errorf("line %d is recorded, want it absent from the statement set", line)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile string
		wantErr string
	}{
		{name: "empty input", profile: "", wantErr: "empty profile"},
		{name: "missing header", profile: "x/a.go:1.1,1.5 1 1\n", wantErr: "unknown profile header"},
		{name: "unknown mode", profile: "mode: lines\n", wantErr: "unknown profile header"},
		{name: "block without column", profile: "mode: set\nx/a.go:1,2 1 1\n", wantErr: "missing column"},
		{name: "block without end", profile: "mode: set\nx/a.go:1.1 1 1\n", wantErr: "malformed profile block"},
		{name: "block with too few fields", profile: "mode: set\nx/a.go:1.1,2.1 1\n", wantErr: "malformed profile block"},
		{name: "block without path", profile: "mode: set\n1.1,2.1 1 1\n", wantErr: "malformed profile block"},
		{name: "non numeric count", profile: "mode: set\nx/a.go:1.1,2.5 1 many\n", wantErr: "malformed profile block"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, parseErr := Parse([]byte(tt.profile))
			if parseErr == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(parseErr.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", parseErr.Error(), tt.wantErr)
			}
		})
	}
}
