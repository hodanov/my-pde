# permission prompt の原因と対処

Claude Code の Bash 許可判定と、permission prompt が出る主な原因・推奨対処の対応表。
検出フック（`settings/claude/hooks/permission-prompt-detect.sh`）が付ける `kind` に対応する。

## 前提: 許可判定の仕組み（近似）

- コマンドは `&&` / `||` / `|` / `;` / 改行で**セグメントに分割**され、**全セグメント**が
  `permissions.allow` のいずれかにマッチして初めて自動承認される。1 つでも未マッチだとプロンプトが出る。
- allow パターン `Bash(<pat>)` は前方一致。`*` はワイルドカード。マッチは先頭アンカー。
- deny は allow より優先。deny にマッチすると allow があっても実行不可。
- 上記とは別に、Claude Code 組み込みの安全ガードが手動承認を要求するケースがある（下記
  `builtin-cd-redirect`）。これは allow-list では解除できない。

## kind 別の対処

### `missing-allow`

- **症状**: セグメントの実コマンド（先頭の `VAR=...` 除去後）が allow のどのパターンにも当たらない。
  複合コマンドで `cd` / `echo` / `tail` / `head` / `wc` / `cat` などの無害コマンドが未登録なとき頻発。
- **推奨**: `Bash(<tool> *)` を allow に追加。
  - **スコープ**: 汎用的で無害なコマンド（`echo` / `tail` / `head` / `wc` / `cd` 等）は
    グローバル `~/.claude/settings.json`。特定リポ・特定パスに依存するものは project の
    `.claude/settings.local.json`。
  - **例外**: `cat` は `Read` ツールで代替できる（プロジェクト方針で推奨）。allow 追加ではなく
    コマンド側を Read に変える提案をする。

### `prefix-break`

- **症状**: コマンド名は allow にあるが、フラグが差し込まれて前方一致が外れる。代表例:
  `terraform -chdir=/path show ...` は `Bash(terraform show *)` に当たらない（`-chdir` が
  `terraform` と `show` の間に入るため）。
- **推奨**: 次の二択を提示。
  - パターン追加: `Bash(terraform -chdir=* show *)` のように `-chdir` 込みで登録。
  - 書き換え: `-chdir` を使わず素直な形にする（サブコマンドを先頭付近に置く）。

### `builtin-cd-redirect`

- **症状**: 複合コマンド内に `cd`（特に変数ディレクトリ）と出力 redirection（`>` / `>>`）が共存。
  「移動後に相対パスへ書き込むと、書き込み先が静的に確定できずパス制限をすり抜けうる」ため、
  Claude Code 組み込みガードが手動承認を要求する。
- **重要**: これは **allow-list では解除できない**。
- **推奨**: コマンドを書き換える。`cd` を使わず、redirection 先を絶対パスにする。
  - 例: `cd "$SP" && terraform show > out.json`
    → `terraform -chdir="$SP" show > "$SP/out.json"`（ただし前者/後者とも他の kind に注意）。

### `deny-hit`

- **症状**: セグメントが `permissions.deny` にマッチ（例 `rm -rf *` / `wget *`）。
- **推奨**: 意図的な遮断である可能性が高い。allow 追加は勧めない。なぜ deny されているかを
  確認し、本当に必要なら deny 側の見直し要否をユーザーに委ねる。

## 保守メモ

- 検出はあくまで近似。行継続（`\`＋改行）の連結、ヒアドキュメント本文の除外、引用符・
  `$(...)`・バッククォートのマスクは行うが、ネスト引用や複雑なシェル構文は厳密には解釈しない。
- Claude Code 側のガードが増減したら、この対応表と検出フックの判定を合わせて更新する。
