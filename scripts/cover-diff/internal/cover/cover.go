// Package cover parses a Go coverage profile into per-file statement lines.
//
// The parser is deliberately strict about the header and deliberately narrow
// about what it records: only lines a profile block actually spans become map
// keys. Every other line of the source -- imports, declarations, comments,
// closing braces -- is absent, which is how callers tell "not covered" apart
// from "carries no statement at all".
package cover

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Modes are the covermode values `go test` writes on the profile's first line.
var Modes = []string{"set", "count", "atomic"}

// Profile is one parsed coverage profile.
type Profile struct {
	// Mode is the covermode the profile was produced with.
	Mode string
	// Files maps a profile path (an import path, not a repository path) to its
	// statement lines. A line is present only when a block spans it; the value
	// reports whether any block covering it ran.
	Files map[string]map[int]bool
}

// Parse reads a coverage profile. A missing or unknown "mode:" header is an
// error: silently treating an unrecognised format as empty would report every
// changed line as uncovered.
func Parse(profile []byte) (Profile, error) {
	scanner := bufio.NewScanner(bytes.NewReader(profile))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	p := Profile{Files: map[string]map[int]bool{}}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if p.Mode == "" {
			mode, modeErr := parseMode(line)
			if modeErr != nil {
				return Profile{}, modeErr
			}
			p.Mode = mode
			continue
		}
		if blockErr := p.addBlock(line); blockErr != nil {
			return Profile{}, blockErr
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return Profile{}, fmt.Errorf("read profile: %w", scanErr)
	}
	if p.Mode == "" {
		return Profile{}, fmt.Errorf("empty profile: want a first line of %q", modeHeader(Modes[0]))
	}
	return p, nil
}

func parseMode(line string) (string, error) {
	for _, mode := range Modes {
		if line == modeHeader(mode) {
			return mode, nil
		}
	}
	return "", fmt.Errorf("unknown profile header %q (want one of %s)", line, strings.Join(headers(), ", "))
}

func modeHeader(mode string) string { return "mode: " + mode }

func headers() []string {
	out := make([]string, 0, len(Modes))
	for _, mode := range Modes {
		out = append(out, strconv.Quote(modeHeader(mode)))
	}
	return out
}

// addBlock records one "path:sL.sC,eL.eC numStmt count" block. The end position
// is exclusive, so a block ending at column 1 stops before that line and the
// closing brace it usually sits on is left out of the statement set.
func (p Profile) addBlock(line string) error {
	sep := strings.LastIndex(line, ":")
	if sep < 1 {
		return fmt.Errorf("malformed profile block %q", line)
	}
	path := line[:sep]

	fields := strings.Fields(line[sep+1:])
	if len(fields) != 3 {
		return fmt.Errorf("malformed profile block %q", line)
	}
	span, spanErr := parseSpan(fields[0])
	if spanErr != nil {
		return fmt.Errorf("malformed profile block %q: %w", line, spanErr)
	}
	count, countErr := strconv.Atoi(fields[2])
	if countErr != nil {
		return fmt.Errorf("malformed profile block %q: %w", line, countErr)
	}

	lines, seen := p.Files[path]
	if !seen {
		lines = map[int]bool{}
		p.Files[path] = lines
	}
	for n := span.start; n <= span.end; n++ {
		lines[n] = lines[n] || count > 0
	}
	return nil
}

// span is the inclusive range of source lines one profile block covers.
type span struct {
	start int
	end   int
}

func parseSpan(text string) (span, error) {
	startText, endText, ok := strings.Cut(text, ",")
	if !ok {
		return span{}, fmt.Errorf("missing block end in %q", text)
	}
	startLine, _, startErr := parsePosition(startText)
	if startErr != nil {
		return span{}, startErr
	}
	endLine, endCol, endErr := parsePosition(endText)
	if endErr != nil {
		return span{}, endErr
	}
	if endCol <= 1 {
		endLine--
	}
	return span{start: startLine, end: endLine}, nil
}

func parsePosition(text string) (line, col int, err error) {
	lineText, colText, ok := strings.Cut(text, ".")
	if !ok {
		return 0, 0, fmt.Errorf("missing column in %q", text)
	}
	line, lineErr := strconv.Atoi(lineText)
	if lineErr != nil {
		return 0, 0, lineErr
	}
	col, colErr := strconv.Atoi(colText)
	if colErr != nil {
		return 0, 0, colErr
	}
	return line, col, nil
}
