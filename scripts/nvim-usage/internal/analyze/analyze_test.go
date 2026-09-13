package analyze

import (
	"fmt"
	"reflect"
	"testing"

	"nvim-usage/internal/event"
)

func TestKeymapUsage(t *testing.T) {
	t.Parallel()
	normalY := event.Keymap{Mode: "n", LHS: "<Space>y"}
	visualY := event.Keymap{Mode: "x", LHS: "<Space>y"}
	textObject := event.Keymap{Mode: "o", LHS: "ih"}
	unused := event.Keymap{Mode: "n", LHS: "<Space>q"}
	tests := []struct {
		name    string
		chunks  []event.Chunk
		keymaps []event.Keymap
		want    []KeymapUsage
	}{
		{
			name: "typed mappings count only in their own family and keys typed apart do not count",
			chunks: []event.Chunk{
				{Family: event.FamilyNormal, Keys: []string{"<Space>y", "d", "ih", "<Space>y", "<Space>", "q"}},
				{Family: event.FamilyVisual, Keys: []string{"j", "<Space>y"}},
			},
			keymaps: []event.Keymap{unused, visualY, textObject, normalY},
			want: []KeymapUsage{
				{Keymap: normalY, Count: 2},
				{Keymap: textObject, Count: 1},
				{Keymap: visualY, Count: 1},
				{Keymap: unused, Count: 0},
			},
		},
		{
			name:    "no chunks leaves every keymap unused",
			keymaps: []event.Keymap{normalY},
			want:    []KeymapUsage{{Keymap: normalY, Count: 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keymapUsage(tt.chunks, tt.keymaps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("keymapUsage =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestRepeats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		chunks []event.Chunk
		want   []Repeat
	}{
		{
			name: "runs at the threshold are counted per family and key",
			chunks: []event.Chunk{
				{Family: event.FamilyNormal, Keys: []string{"j", "j", "j", "j", "d", "j", "j", "j", "j", "j", "j"}},
				{Family: event.FamilyNormal, Keys: []string{"x", "x", "x", "x"}},
				{Family: event.FamilyVisual, Keys: []string{"j", "j", "j", "j", "y"}},
			},
			want: []Repeat{
				{Family: event.FamilyNormal, Key: "j", Runs: 2, Presses: 10, Longest: 6},
				{Family: event.FamilyNormal, Key: "x", Runs: 1, Presses: 4, Longest: 4},
				{Family: event.FamilyVisual, Key: "j", Runs: 1, Presses: 4, Longest: 4},
			},
		},
		{
			name:   "runs below the threshold are ignored",
			chunks: []event.Chunk{{Family: event.FamilyNormal, Keys: []string{"j", "j", "j", "k"}}},
			want:   []Repeat{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := repeats(tt.chunks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("repeats =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestSequences(t *testing.T) {
	t.Parallel()
	manyKeys := make([]string, 0, TopSequences+2)
	for i := range TopSequences + 2 {
		manyKeys = append(manyKeys, fmt.Sprintf("k%d", i))
	}
	tests := []struct {
		name       string
		chunks     []event.Chunk
		want       []Sequence
		wantLength int
	}{
		{
			name: "windows of every length are counted and uniform ones are left out",
			chunks: []event.Chunk{
				{Family: event.FamilyNormal, Keys: []string{"d", "w", "d", "w"}},
				{Family: event.FamilyVisual, Keys: []string{"j", "j", "<Space>y"}},
			},
			want: []Sequence{
				{Family: event.FamilyNormal, Keys: "d w", Count: 2},
				{Family: event.FamilyNormal, Keys: "d w d", Count: 1},
				{Family: event.FamilyNormal, Keys: "d w d w", Count: 1},
				{Family: event.FamilyNormal, Keys: "w d", Count: 1},
				{Family: event.FamilyNormal, Keys: "w d w", Count: 1},
				{Family: event.FamilyVisual, Keys: "j <Space>y", Count: 1},
				{Family: event.FamilyVisual, Keys: "j j <Space>y", Count: 1},
			},
			wantLength: 7,
		},
		{
			name:       "each family keeps only the top sequences",
			chunks:     []event.Chunk{{Family: event.FamilyNormal, Keys: manyKeys}},
			wantLength: TopSequences,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sequences(tt.chunks)
			if len(got) != tt.wantLength {
				t.Fatalf("len(sequences) = %d, want %d", len(got), tt.wantLength)
			}
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sequences =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestCommandCounts(t *testing.T) {
	t.Parallel()
	got := commandCounts(map[string]int{"write": 2, "vsplit": 3, "edit": 2})
	want := []CommandCount{{Name: "vsplit", Count: 3}, {Name: "edit", Count: 2}, {Name: "write", Count: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commandCounts = %+v, want %+v", got, want)
	}
}
