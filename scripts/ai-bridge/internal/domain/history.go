package domain

import (
	"fmt"
	"strings"
	"time"
)

// FormatHistory renders records (already newest-first) as a human-readable,
// indexed list. limit > 0 caps the number of rows shown; limit <= 0 shows all.
// An empty result yields a single "(no history)" line.
func FormatHistory(records []*Request, limit int) string {
	if limit > 0 && limit < len(records) {
		records = records[:limit]
	}
	if len(records) == 0 {
		return "(no history)\n"
	}
	var b strings.Builder
	for i, r := range records {
		ts := time.Unix(r.Timestamp, 0).Format("2006-01-02 15:04:05")
		fmt.Fprintf(&b, "%3d  %s  %s\n     %s\n", i, ts, r.CWD, firstLine(r.Prompt))
	}
	return b.String()
}

// firstLine returns the first line of s, truncated to keep list rows compact.
func firstLine(s string) string {
	const maxLen = 100
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}
