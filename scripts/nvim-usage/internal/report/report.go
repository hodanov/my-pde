// Package report renders an analyze.Report as the Markdown the
// nvim-usage-review skill reads.
package report

import (
	"fmt"
	"strconv"
	"strings"

	"nvim-usage/internal/analyze"
	"nvim-usage/internal/event"
)

// RenderMarkdown renders r with one table per metric.
func RenderMarkdown(r *analyze.Report) string {
	var b strings.Builder
	b.WriteString("# Neovim 使い方レポート\n\n")
	writeSummary(&b, r)
	writeKeymaps(&b, r.Keymaps)
	writeRepeats(&b, r.Repeats)
	writeSequences(&b, r.Sequences)
	writeCommands(&b, r.Commands)
	return b.String()
}

func writeSummary(b *strings.Builder, r *analyze.Report) {
	if r.Period.From == "" {
		b.WriteString("- 期間: 記録なし\n")
	} else {
		fmt.Fprintf(b, "- 期間: %s 〜 %s（記録のある日 %d 日）\n", r.Period.From, r.Period.To, r.Period.Days)
	}
	if r.Nvim != "" {
		fmt.Fprintf(b, "- Neovim: %s\n", r.Nvim)
	}
	fmt.Fprintf(b, "- キー入力: ノーマル %d / ビジュアル %d（チャンク %d）\n",
		r.Totals.NormalKeys, r.Totals.VisualKeys, r.Totals.Chunks)
	fmt.Fprintf(b, "- Ex コマンド: %d 回 / 検索: %d 回\n\n", r.Totals.Commands, r.Totals.Searches)
}

func writeKeymaps(b *strings.Builder, usage []analyze.KeymapUsage) {
	var unused, used [][]string
	for i := range usage {
		u := &usage[i]
		if u.Count == 0 {
			unused = append(unused, []string{u.Mode, code(u.LHS), text(u.Desc), yesNo(u.BufferLocal)})
		} else {
			used = append(used, []string{u.Mode, code(u.LHS), text(u.Desc), strconv.Itoa(u.Count)})
		}
	}
	b.WriteString("## 未使用のキーマップ\n\n")
	b.WriteString("回数は、入力がそのマッピングとして解決された回数。" +
		"同じキーを 1 つずつ打ってマッピングにならなかった分は数えない。\n\n")
	writeTable(b, []string{"モード", "lhs", "desc", "バッファローカル"}, unused)
	b.WriteString("## キーマップの使用回数\n\n")
	writeTable(b, []string{"モード", "lhs", "desc", "回数"}, used)
}

func writeRepeats(b *strings.Builder, repeats []analyze.Repeat) {
	rows := make([][]string, 0, len(repeats))
	for _, r := range repeats {
		rows = append(rows, []string{
			familyLabel(r.Family), code(r.Key), strconv.Itoa(r.Runs), strconv.Itoa(r.Presses), strconv.Itoa(r.Longest),
		})
	}
	fmt.Fprintf(b, "## 同じキーの連打（%d 回以上）\n\n", analyze.MinRepeat)
	writeTable(b, []string{"系列", "キー", "連打の回数", "押下数", "最長"}, rows)
}

func writeSequences(b *strings.Builder, sequences []analyze.Sequence) {
	rows := make([][]string, 0, len(sequences))
	for _, s := range sequences {
		rows = append(rows, []string{familyLabel(s.Family), code(s.Keys), strconv.Itoa(s.Count)})
	}
	fmt.Fprintf(b, "## 頻出シーケンス（%d〜%d キー、系列ごとに上位 %d 件）\n\n",
		analyze.MinSequence, analyze.MaxSequence, analyze.TopSequences)
	b.WriteString("キー列はスペース区切りで、打ったマッピングは lhs 全体で 1 キーとして数える。" +
		"同じキーだけが続く並びは、連打の表で数えるので除く。\n\n")
	writeTable(b, []string{"系列", "キー列", "回数"}, rows)
}

func writeCommands(b *strings.Builder, commands []analyze.CommandCount) {
	rows := make([][]string, 0, len(commands))
	for _, c := range commands {
		rows = append(rows, []string{code(c.Name), strconv.Itoa(c.Count)})
	}
	b.WriteString("## Ex コマンド\n\n")
	writeTable(b, []string{"コマンド", "回数"}, rows)
}

func writeTable(b *strings.Builder, header []string, rows [][]string) {
	if len(rows) == 0 {
		b.WriteString("なし\n\n")
		return
	}
	writeRow(b, header)
	separator := make([]string, len(header))
	for i := range separator {
		separator[i] = "---"
	}
	writeRow(b, separator)
	for _, row := range rows {
		writeRow(b, row)
	}
	b.WriteString("\n")
}

func writeRow(b *strings.Builder, cells []string) {
	b.WriteString("| ")
	b.WriteString(strings.Join(cells, " | "))
	b.WriteString(" |\n")
}

func familyLabel(family string) string {
	switch family {
	case event.FamilyNormal:
		return "ノーマル"
	case event.FamilyVisual:
		return "ビジュアル"
	default:
		return family
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func code(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

func text(s string) string {
	return strings.NewReplacer("|", `\|`, "<", `\<`, "\n", " ").Replace(s)
}
