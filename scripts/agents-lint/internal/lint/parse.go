package lint

import (
	"regexp"
	"strings"
)

// frontmatter is the parsed leading --- ... --- block of a markdown file.
// Values collapse block scalars (">-", "|") and nested maps into a single
// string; the rules only need presence and emptiness of top-level fields, so
// this deliberately does not implement full YAML (dependency-free).
type frontmatter struct {
	present bool              // a leading --- delimiter was found on line 1
	closed  bool              // a closing --- delimiter was found
	fields  map[string]string // top-level key -> value (block scalars joined)
	keys    []string          // top-level keys in document order
	body    string            // content after the closing delimiter
}

// value returns the value for key, or "" if absent.
func (f frontmatter) value(key string) string { return f.fields[key] }

// has reports whether key was present in the frontmatter.
func (f frontmatter) has(key string) bool {
	_, ok := f.fields[key]
	return ok
}

// parseFrontmatter extracts the frontmatter block and the body that follows.
// It is pure: no filesystem access.
func parseFrontmatter(data []byte) frontmatter {
	fm := frontmatter{fields: map[string]string{}}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || trimCR(lines[0]) != "---" {
		return fm // present == false
	}
	fm.present = true

	end := len(lines) // index of the closing delimiter, or EOF if unterminated
	for i := 1; i < len(lines); i++ {
		if trimCR(lines[i]) == "---" {
			fm.closed = true
			end = i
			break
		}
	}

	block := make([]string, 0, end-1)
	for i := 1; i < end; i++ {
		block = append(block, trimCR(lines[i]))
	}
	parseBlock(&fm, block)

	if fm.closed && end < len(lines) {
		bodyLines := make([]string, 0, len(lines)-end-1)
		for i := end + 1; i < len(lines); i++ {
			bodyLines = append(bodyLines, trimCR(lines[i]))
		}
		fm.body = strings.Join(bodyLines, "\n")
	}
	return fm
}

// parseBlock fills fm.fields/keys from the frontmatter body lines. Top-level
// keys are unindented "key: value" lines; a key whose value is empty or a
// block-scalar indicator absorbs the following indented/blank lines.
func parseBlock(fm *frontmatter, block []string) {
	j := 0
	for j < len(block) {
		line := block[j]
		if strings.TrimSpace(line) == "" || indented(line) {
			j++
			continue
		}
		rawKey, rawRest, found := strings.Cut(line, ":")
		if !found {
			j++
			continue
		}
		key := strings.TrimSpace(rawKey)
		rest := strings.TrimSpace(rawRest)
		if _, seen := fm.fields[key]; !seen {
			fm.keys = append(fm.keys, key)
		}
		if rest != "" && !isBlockScalar(rest) {
			fm.fields[key] = unquote(rest)
			j++
			continue
		}
		// Absorb continuation lines (indented or blank) until the next top-level key.
		var buf []string
		k := j + 1
		for k < len(block) {
			nxt := block[k]
			if strings.TrimSpace(nxt) == "" {
				buf = append(buf, "")
				k++
				continue
			}
			if indented(nxt) {
				buf = append(buf, strings.TrimSpace(nxt))
				k++
				continue
			}
			break
		}
		fm.fields[key] = strings.TrimSpace(strings.Join(buf, " "))
		j = k
	}
}

// indented reports whether line begins with a space or tab.
func indented(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t')
}

// isBlockScalar reports whether a value is a YAML block-scalar indicator
// (">", ">-", "|", "|+", ...).
func isBlockScalar(v string) bool {
	return v != "" && (v[0] == '>' || v[0] == '|')
}

// unquote strips a single pair of matching surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// trimCR removes a trailing carriage return (CRLF tolerance).
func trimCR(s string) string { return strings.TrimSuffix(s, "\r") }

// backtickRe captures the content of a `...` span.
var backtickRe = regexp.MustCompile("`([^`]+)`")

// extractAgentRefs returns the subagent names a skill body launches. It anchors
// on the explicit "subagent_type" signal to avoid false positives from prose,
// covering the three forms used in this repo:
//
//   - `subagent_type`: `name`            (backtick name after the key)
//   - `subagent_type: name`              (key and name in one backtick span)
//   - a markdown table with a subagent_type column
//
// Illustrative file paths like `agents/foo.md` are not subagent_type signals
// and are deliberately ignored.
func extractAgentRefs(body string) []string {
	lines := strings.Split(body, "\n")
	seen := map[string]bool{}
	var refs []string
	add := func(name string) {
		name = stripTicks(name)
		if name != "" && name != "subagent_type" && nameRe.MatchString(name) && !seen[name] {
			seen[name] = true
			refs = append(refs, name)
		}
	}

	for i, line := range lines {
		if !strings.Contains(line, "subagent_type") {
			continue
		}
		// Inline backtick forms, anchored on a subagent_type span: the span
		// either carries the name itself (`subagent_type: name`) or the name
		// is the next span on the line (`subagent_type`: `name`). Other spans
		// on the line are prose, not references.
		spans := backtickRe.FindAllStringSubmatch(line, -1)
		for si, m := range spans {
			after, ok := strings.CutPrefix(strings.TrimSpace(m[1]), "subagent_type")
			if !ok {
				continue
			}
			if name := strings.TrimSpace(strings.TrimLeft(after, ": ")); name != "" {
				add(name)
			} else if si+1 < len(spans) {
				add(spans[si+1][1])
			}
		}
		// Markdown table form: find the subagent_type column, read it in rows below.
		if col := tableColumnIndex(line, "subagent_type"); col >= 0 {
			for _, row := range lines[i+1:] {
				if !isTableRow(row) {
					break
				}
				if isTableSeparator(row) {
					continue
				}
				add(strings.TrimSpace(tableCell(row, col)))
			}
		}
	}
	return refs
}

// stripTicks trims surrounding whitespace and backticks, so a backticked
// table header or cell (`subagent_type`, `review-security`) compares and
// resolves the same as its bare form.
func stripTicks(s string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "`"))
}

// tableColumnIndex returns the zero-based cell index whose header equals title
// in a markdown table row, or -1.
func tableColumnIndex(row, title string) int {
	if !isTableRow(row) {
		return -1
	}
	for i, cell := range splitRow(row) {
		if stripTicks(cell) == title {
			return i
		}
	}
	return -1
}

// tableCell returns the col-th cell of a markdown table row, or "".
func tableCell(row string, col int) string {
	cells := splitRow(row)
	if col < 0 || col >= len(cells) {
		return ""
	}
	return cells[col]
}

// isTableRow reports whether line looks like a markdown table row.
func isTableRow(line string) bool {
	return strings.Contains(strings.TrimSpace(line), "|")
}

// isTableSeparator reports whether line is a markdown table separator (---|---).
func isTableSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.Contains(t, "-") {
		return false
	}
	return strings.Trim(t, "|-: \t") == ""
}

// splitRow splits a markdown table row into its cells, dropping the empty
// leading/trailing fields produced by the outer pipes.
func splitRow(row string) []string {
	parts := strings.Split(strings.TrimSpace(row), "|")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
