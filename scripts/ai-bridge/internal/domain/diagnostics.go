package domain

import (
	"fmt"
	"strings"
)

// Status is the outcome of a single diagnostic check.
type Status int

const (
	// StatusOK indicates the check passed.
	StatusOK Status = iota
	// StatusWarn indicates a non-fatal problem.
	StatusWarn
	// StatusFail indicates a fatal problem that breaks the bridge.
	StatusFail
)

// String returns the lowercase label for the status.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusWarn:
		return "warn"
	case StatusFail:
		return "fail"
	default:
		return "unknown"
	}
}

// Check is the result of one diagnostic.
type Check struct {
	Name   string
	Status Status
	Detail string
}

// HasFailure reports whether any check has StatusFail.
func HasFailure(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// FormatChecks renders checks as human-readable, aligned lines.
func FormatChecks(checks []Check) string {
	var b strings.Builder
	b.WriteString("ai-bridge doctor\n")
	for _, c := range checks {
		fmt.Fprintf(&b, "  [%-4s] %s: %s\n", c.Status, c.Name, c.Detail)
	}
	return b.String()
}
