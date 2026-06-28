// Package history persists ai-bridge requests and supports replaying them.
//
// The daemon appends every successfully parsed request to an append-only JSONL
// file. The history and replay subcommands read that file to list past prompts
// or re-inject one as a new request, reusing the existing watcher/launcher path.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const fileName = "history.jsonl"

// Record is one persisted request. Fields mirror the daemon request format so a
// record can be written back out as request.json without transformation.
type Record struct {
	Prompt    string `json:"prompt"`
	CWD       string `json:"cwd"`
	Timestamp int64  `json:"timestamp"`
}

// Path returns the history file path inside bridgeDir.
func Path(bridgeDir string) string {
	return filepath.Join(bridgeDir, fileName)
}

// Append writes one record as a JSON line to the history file at path.
// The file is created if it does not exist. A single O_APPEND write keeps
// concurrent lines from interleaving.
func Append(path string, rec Record) error {
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		return fmt.Errorf("open history: %w", openErr)
	}
	defer func() { _ = f.Close() }()

	data, marshalErr := json.Marshal(rec)
	if marshalErr != nil {
		return fmt.Errorf("marshal record: %w", marshalErr)
	}
	if _, writeErr := f.Write(append(data, '\n')); writeErr != nil {
		return fmt.Errorf("write history: %w", writeErr)
	}
	return nil
}

// Load reads all records newest-first. A missing file yields an empty slice and
// no error. Corrupt (unparseable) lines are skipped so one bad row cannot block
// the whole history.
func Load(path string) ([]Record, error) {
	f, openErr := os.Open(path)
	if openErr != nil {
		if os.IsNotExist(openErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history: %w", openErr)
	}
	defer func() { _ = f.Close() }()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if unmarshalErr := json.Unmarshal(line, &rec); unmarshalErr != nil {
			continue // skip corrupt line
		}
		records = append(records, rec)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("read history: %w", scanErr)
	}

	// Reverse to newest-first.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

// WriteRequest atomically writes rec as a request file at path. It writes to a
// temp file in the same directory and renames it into place so the daemon never
// observes a partially written request.json.
func WriteRequest(path string, rec Record) (retErr error) {
	data, marshalErr := json.Marshal(rec)
	if marshalErr != nil {
		return fmt.Errorf("marshal request: %w", marshalErr)
	}

	dir := filepath.Dir(path)
	tmp, createTempErr := os.CreateTemp(dir, ".request-*.tmp")
	if createTempErr != nil {
		return fmt.Errorf("create temp request: %w", createTempErr)
	}
	tmpName := tmp.Name()
	defer func() {
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp request: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("close temp request: %w", closeErr)
	}
	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		return fmt.Errorf("rename temp request: %w", renameErr)
	}
	return nil
}

// FormatList renders records (already newest-first) as a human-readable list.
// limit > 0 caps the number of rows shown; limit <= 0 shows all.
func FormatList(records []Record, limit int) string {
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
