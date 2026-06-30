package domain

import (
	"strings"
	"testing"
)

func TestStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "ok", status: StatusOK, want: "ok"},
		{name: "warn", status: StatusWarn, want: "warn"},
		{name: "fail", status: StatusFail, want: "fail"},
		{name: "unknown", status: Status(99), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		checks []Check
		want   bool
	}{
		{
			name:   "all ok",
			checks: []Check{{Status: StatusOK}, {Status: StatusWarn}},
			want:   false,
		},
		{
			name:   "one failure",
			checks: []Check{{Status: StatusOK}, {Status: StatusFail}},
			want:   true,
		},
		{
			name:   "empty",
			checks: nil,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HasFailure(tt.checks); got != tt.want {
				t.Errorf("HasFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatChecks(t *testing.T) {
	t.Parallel()
	checks := []Check{
		{Name: "BridgeDir", Status: StatusOK, Detail: "/home/x/.ai-bridge"},
		{Name: "CLI", Status: StatusFail, Detail: `"claude" not found`},
	}
	out := FormatChecks(checks)
	for _, want := range []string{"ai-bridge doctor", "BridgeDir", "[ok", "CLI", "[fail"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatChecks() output missing %q:\n%s", want, out)
		}
	}
}
