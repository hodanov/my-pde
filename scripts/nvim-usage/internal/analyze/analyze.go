// Package analyze turns a usage log and keymap snapshot into the counts the
// nvim-usage-review skill interprets: how often each keymap is typed, keys
// pressed over and over, frequent key sequences and Ex command frequency.
package analyze

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"nvim-usage/internal/event"
)

// Thresholds of the report. Renderers read them to label their sections.
const (
	MinRepeat    = 4
	MinSequence  = 2
	MaxSequence  = 4
	TopSequences = 20
)

// Report is the aggregate rendered as Markdown or JSON.
type Report struct {
	Nvim      string         `json:"nvim,omitempty"`
	Period    Period         `json:"period"`
	Totals    Totals         `json:"totals"`
	Keymaps   []KeymapUsage  `json:"keymaps"`
	Repeats   []Repeat       `json:"repeats"`
	Sequences []Sequence     `json:"sequences"`
	Commands  []CommandCount `json:"commands"`
}

// Period is the span the report covers, as UTC dates. From and To are empty
// when no event was recorded.
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}

// Totals counts the recorded input.
type Totals struct {
	Chunks     int `json:"chunks"`
	NormalKeys int `json:"normal_keys"`
	VisualKeys int `json:"visual_keys"`
	Commands   int `json:"commands"`
	Searches   int `json:"searches"`
}

// KeymapUsage is a keymap with how many times it was typed in its mode family.
// Only input Neovim resolved into the mapping counts: the same keys typed one by
// one without resolving it arrive as separate chunk entries.
type KeymapUsage struct {
	event.Keymap
	Count int `json:"count"`
}

// Repeat is a key pressed MinRepeat or more times in a row: Runs is how many
// such runs there were, Presses the keys they add up to.
type Repeat struct {
	Family  string `json:"family"`
	Key     string `json:"key"`
	Runs    int    `json:"runs"`
	Presses int    `json:"presses"`
	Longest int    `json:"longest"`
}

// Sequence is a run of MinSequence to MaxSequence keys, joined by spaces, and
// how often it was typed. A typed mapping counts as one key. Runs of a single
// repeated key are left to Repeat.
type Sequence struct {
	Family string `json:"family"`
	Keys   string `json:"keys"`
	Count  int    `json:"count"`
}

// CommandCount is how often an Ex command was run, by its full name.
type CommandCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Build aggregates l against the keymaps in snap.
func Build(l *event.Log, snap *event.Snapshot) Report {
	r := Report{
		Nvim:      snap.Nvim,
		Period:    Period{Days: l.Days()},
		Totals:    totals(l),
		Keymaps:   keymapUsage(l.Chunks, snap.Keymaps),
		Repeats:   repeats(l.Chunks),
		Sequences: sequences(l.Chunks),
		Commands:  commandCounts(l.Commands),
	}
	if !l.First.IsZero() {
		r.Period.From = l.First.Format(time.DateOnly)
		r.Period.To = l.Last.Format(time.DateOnly)
	}
	return r
}

func totals(l *event.Log) Totals {
	t := Totals{Chunks: len(l.Chunks), Searches: l.Searches}
	for i := range l.Chunks {
		if l.Chunks[i].Family == event.FamilyVisual {
			t.VisualKeys += len(l.Chunks[i].Keys)
		} else {
			t.NormalKeys += len(l.Chunks[i].Keys)
		}
	}
	for _, n := range l.Commands {
		t.Commands += n
	}
	return t
}

func keymapUsage(chunks []event.Chunk, keymaps []event.Keymap) []KeymapUsage {
	type typedKey struct{ family, key string }
	typed := map[typedKey]int{}
	for i := range chunks {
		for _, key := range chunks[i].Keys {
			typed[typedKey{chunks[i].Family, key}]++
		}
	}
	usage := make([]KeymapUsage, 0, len(keymaps))
	for i := range keymaps {
		k := &keymaps[i]
		usage = append(usage, KeymapUsage{Keymap: *k, Count: typed[typedKey{k.Family(), k.LHS}]})
	}
	slices.SortStableFunc(usage, func(a, b KeymapUsage) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Mode, b.Mode), cmp.Compare(a.LHS, b.LHS))
	})
	return usage
}

func repeats(chunks []event.Chunk) []Repeat {
	type repeatKey struct{ family, key string }
	found := map[repeatKey]*Repeat{}
	for i := range chunks {
		c := &chunks[i]
		for start := 0; start < len(c.Keys); {
			end := start + 1
			for end < len(c.Keys) && c.Keys[end] == c.Keys[start] {
				end++
			}
			if run := end - start; run >= MinRepeat {
				k := repeatKey{c.Family, c.Keys[start]}
				r, ok := found[k]
				if !ok {
					r = &Repeat{Family: c.Family, Key: c.Keys[start]}
					found[k] = r
				}
				r.Runs++
				r.Presses += run
				r.Longest = max(r.Longest, run)
			}
			start = end
		}
	}
	out := make([]Repeat, 0, len(found))
	for _, r := range found {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Repeat) int {
		return cmp.Or(cmp.Compare(b.Presses, a.Presses), cmp.Compare(a.Family, b.Family), cmp.Compare(a.Key, b.Key))
	})
	return out
}

func sequences(chunks []event.Chunk) []Sequence {
	type sequenceKey struct{ family, keys string }
	counts := map[sequenceKey]int{}
	for i := range chunks {
		c := &chunks[i]
		for n := MinSequence; n <= MaxSequence; n++ {
			for start := 0; start+n <= len(c.Keys); start++ {
				window := c.Keys[start : start+n]
				if uniform(window) {
					continue
				}
				counts[sequenceKey{c.Family, strings.Join(window, " ")}]++
			}
		}
	}
	all := make([]Sequence, 0, len(counts))
	for k, n := range counts {
		all = append(all, Sequence{Family: k.family, Keys: k.keys, Count: n})
	}
	slices.SortFunc(all, func(a, b Sequence) int {
		return cmp.Or(cmp.Compare(a.Family, b.Family), cmp.Compare(b.Count, a.Count), cmp.Compare(a.Keys, b.Keys))
	})
	top := make([]Sequence, 0, len(all))
	kept := map[string]int{}
	for _, s := range all {
		if kept[s.Family] < TopSequences {
			kept[s.Family]++
			top = append(top, s)
		}
	}
	return top
}

func uniform(keys []string) bool {
	for _, k := range keys[1:] {
		if k != keys[0] {
			return false
		}
	}
	return true
}

func commandCounts(commands map[string]int) []CommandCount {
	out := make([]CommandCount, 0, len(commands))
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		out = append(out, CommandCount{Name: name, Count: commands[name]})
	}
	slices.SortStableFunc(out, func(a, b CommandCount) int { return cmp.Compare(b.Count, a.Count) })
	return out
}
