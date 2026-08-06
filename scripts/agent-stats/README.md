# agent-stats

ホスト側 AI CLI（Claude Code）がローカルに残すセッション transcript を **read-only** で
走査し、トークン消費・ツール呼び出し・触れたファイルをセッション単位で集計するローカル観測
CLI。SaaS を挟まず、transcript を一切書き換えない。

## 何を集計するか

Claude Code の `~/.claude/projects/**/*.jsonl` を 1 行ずつ寛容にパースし（未知フィールド・
壊れ行は skip）、1 ファイル = 1 セッションとして次を集計する。

- トークン内訳（input / output / cache read / cache creation）と model
- アシスタントターン数
- tool 別呼び出し回数（Edit / Write / Bash / 各 MCP / skill など）
- Edit / Write / MultiEdit / NotebookEdit の `file_path` から頻出編集ファイル top-N
- `Skill` tool_use の `input.skill` から、どの skill が何回呼ばれたか（skill 内訳）
- 先頭・末尾 timestamp から算出したセッション時間、`cwd` / `gitBranch`

Claude Code は1回のアシスタント応答（thinking → text → tool_use）を、同じ `message.id` を
持つ複数の JSONL 行に分割して書き、各行に同一の usage を repeat する。トークン数・ターン数は
`message.id` ごとに1回だけ集計し、この分割による二重カウントを避けている（`id` を持たない行は
dedupe できないため、そのまま1行1カウントとして扱う）。

集計テーブルの `Duration` は、各セッションの「先頭 timestamp 〜 末尾 timestamp」の差分を全セッ
ション分単純合算した値であり、実作業時間ではない。resume で日をまたいだセッションはアイドル時
間も含むため、値が大きく見えることがある。

skill 内訳（`Skills`）は `Tool calls` の `Skill` 行とは別集計。`Tool calls` の `Skill` は
`input.skill` の有無によらず全 Skill 呼び出しをカウントするのに対し、`Skills` は
`input.skill` を持つ呼び出しだけをその skill 名で集計する。

## 使い方

```sh
# 既定ディレクトリ（~/.claude/projects）を集計してテーブル表示
go run ./cmd/agent-stats

# 対象ディレクトリ・期間・プロジェクトを絞る／JSON 出力
go run ./cmd/agent-stats summary --dir ~/.claude/projects --since 24h --project my-pde
go run ./cmd/agent-stats --json
```

| フラグ      | 既定                 | 意味                                                                      |
| ----------- | -------------------- | ------------------------------------------------------------------------- |
| `--dir`     | `~/.claude/projects` | 走査する transcript ディレクトリ                                          |
| `--since`   | `0`（全件）          | 末尾 timestamp がこの期間内のセッションだけを対象にする（例 `24h`）       |
| `--project` | 空                   | ファイルパスまたは `cwd` にこの部分文字列を含むセッションだけを対象にする |
| `--json`    | `false`              | テーブルの代わりに機械可読な JSON を出力する                              |

先頭に任意の `summary` サブコマンドを付けても同じ挙動（将来のサブコマンド拡張のための予約）。

## 設計・制約

- **read-only**: transcript を open して読むだけ。書き込み・変更は一切しない。
- **寛容パース**: transcript のスキーマは非公式で将来変わり得る。未知フィールドは無視、壊れ行は
  skip し、パーサ（`internal/parser`）に隔離してレポート層へ波及させない。
- **MVP は Claude Code のみ**: 他 CLI（Cursor / Codex / Copilot）は transcript 形式が異なる
  ため未対応。出力にその旨を明示する（silent に「全部見た」風にしない）。
- **プライバシー**: 既定サマリは集計値のみで、プロンプト本文は出さない。

## 構成

```text
scripts/agent-stats/
  cmd/agent-stats/main.go     # フラグ解釈 → 走査 → 集計 → 出力
  internal/parser/            # JSONL → Session（CLI 別実装を差し替えられるよう分離）
  internal/report/            # 集計 + table / json 整形（純粋関数中心でテスト容易）
```

標準ライブラリのみに依存する。
