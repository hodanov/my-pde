// Package event reads the Neovim usage log written by
// nvim/config/lua/usage_recorder.lua: the key chunks, Ex command names and
// search counts under events/*.jsonl, and the keymaps.json snapshot.
//
// Parsing is lenient: malformed lines and unknown kinds are skipped so a single
// bad line never hides the rest of the log.
package event

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Mode families shared with usage_recorder.lua. A chunk holds the keys typed in
// one family, and a keymap is matched against the chunks of its own family.
const (
	FamilyNormal = "normal"
	FamilyVisual = "visual"
)

// Chunk is a run of keys typed in one mode family. Keys holds what Neovim
// reported as typed, in keytrans notation: one entry per key, except that keys
// resolved into a mapping arrive as a single entry equal to the mapping's LHS.
type Chunk struct {
	At       time.Time
	Family   string
	Filetype string
	Keys     []string
}

// Log is every event recorded at or after the cutoff passed to ReadDir.
type Log struct {
	Chunks   []Chunk
	Commands map[string]int
	Searches int
	First    time.Time
	Last     time.Time
	days     map[string]struct{}
}

// Days reports how many distinct UTC dates have at least one event.
func (l *Log) Days() int { return len(l.days) }

// ReadDir reads every events/*.jsonl file under dir, keeping the events
// recorded at or after since. A missing directory yields an empty Log.
func ReadDir(dir string, since time.Time) (*Log, error) {
	l := &Log{Commands: map[string]int{}, days: map[string]struct{}{}}
	paths, err := filepath.Glob(filepath.Join(dir, "events", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if appendErr := l.appendFile(path, since); appendErr != nil {
			return nil, appendErr
		}
	}
	return l, nil
}

type record struct {
	Kind   string   `json:"kind"`
	TS     int64    `json:"ts"`
	Family string   `json:"family"`
	FT     string   `json:"ft"`
	Keys   []string `json:"keys"`
	Name   string   `json:"name"`
}

func (l *Log) appendFile(path string, since time.Time) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		at := time.Unix(r.TS, 0).UTC()
		if at.Before(since) {
			continue
		}
		l.add(&r, at)
	}
	return sc.Err()
}

func (l *Log) add(r *record, at time.Time) {
	switch r.Kind {
	case "keys":
		if r.Family != FamilyNormal && r.Family != FamilyVisual || len(r.Keys) == 0 {
			return
		}
		l.Chunks = append(l.Chunks, Chunk{At: at, Family: r.Family, Filetype: r.FT, Keys: r.Keys})
	case "cmd":
		if r.Name == "" {
			return
		}
		l.Commands[r.Name]++
	case "search":
		l.Searches++
	default:
		return
	}
	l.observe(at)
}

func (l *Log) observe(at time.Time) {
	if l.First.IsZero() || at.Before(l.First) {
		l.First = at
	}
	if at.After(l.Last) {
		l.Last = at
	}
	l.days[at.Format(time.DateOnly)] = struct{}{}
}

// Keymap is one mapping from the keymaps.json snapshot. LHS is in the same
// keytrans notation as Chunk.Keys, so each time the mapping is typed it shows
// up as one chunk entry equal to LHS.
type Keymap struct {
	Mode        string `json:"mode"`
	LHS         string `json:"lhs"`
	Desc        string `json:"desc,omitempty"`
	RHS         string `json:"rhs,omitempty"`
	BufferLocal bool   `json:"buffer_local"`
}

// Family returns the mode family whose chunks the mapping is typed in.
// Operator-pending mappings are typed inside normal-mode chunks.
func (k *Keymap) Family() string {
	if k.Mode == "x" {
		return FamilyVisual
	}
	return FamilyNormal
}

// Snapshot is the keymaps.json that Neovim writes on exit.
type Snapshot struct {
	Nvim    string   `json:"nvim"`
	Keymaps []Keymap `json:"keymaps"`
}

// ReadKeymaps reads dir/keymaps.json. A missing file yields an empty Snapshot,
// since Neovim writes it only on exit.
func ReadKeymaps(dir string) (*Snapshot, error) {
	path := filepath.Join(dir, "keymaps.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if unmarshalErr := json.Unmarshal(data, &s); unmarshalErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, unmarshalErr)
	}
	return &s, nil
}
