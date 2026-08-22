# agent-stats

ホスト側 AI CLI（Claude Code）がローカルに残すセッション transcript を **read-only** で
走査し、トークン消費・ツール呼び出し・触れたファイルをセッション単位で集計するローカル観測
CLI。SaaS を挟まず、transcript を一切書き換えない。

## 何を集計するか

Claude Code の `~/.claude/projects/**/*.jsonl` を 1 行ずつ寛容にパースし（未知フィールド・
壊れ行は skip）、次を集計する。

- トークン内訳（input / output / cache read / cache creation）
- アシスタントターン数と、それを **main loop / subagent** に分けた内訳（`Origin`）
- **モデル別**の turn 数と output / cache read トークン（`Model tokens`）
- tool 別呼び出し回数（Edit / Write / Bash / 各 MCP / skill など）
- **Bash の内訳**（先頭コマンド別）と、専用ツールで代替可能な件数（`Bash breakdown`）
- **tool_result のエラー率**と内訳（permission 拒否 / hook ブロック / それ以外）
- **コンテキスト compaction** の trigger 別回数・平均 preTokens・drop 量
- Edit / Write / MultiEdit / NotebookEdit の `file_path` から頻出編集ファイル top-N
- `Skill` tool_use の `input.skill` から、どの skill が何回呼ばれたか（skill 内訳）
- `cwd` 別の内訳（`By project`）と、output トークンが重いセッション top-N（`Top sessions`）
- CLI 自身が記録したターン所要時間（`Active time`）と、先頭・末尾 timestamp の差（`Session span`）

### 1 セッション = 複数ファイル

Claude Code はセッション本体を `<project>/<uuid>.jsonl` に、そのセッションが起動した subagent を
`<project>/<uuid>/subagents/agent-*.jsonl` に書く。1 ファイル = 1 セッションとして数えると
セッション数がおよそ 2 倍になり、subagent は親の時間内で動くため wall-clock も二重計上になる。
このツールは同一セッションに属する全ファイルを 1 セッションに畳み込む。

subagent の行には `isSidechain` が立つため、main loop と subagent は別スコープに集計される。
「どれだけ委譲できているか」はここで読む。

### 二重カウントの回避

Claude Code は1回のアシスタント応答（thinking → text → tool_use）を、同じ `message.id` を
持つ複数の JSONL 行に分割して書き、各行に同一の usage を repeat する。トークン数・ターン数は
`message.id` ごとに1回だけ集計し、この分割による二重カウントを避けている（`id` を持たない行は
dedupe できないため、そのまま1行1カウントとして扱う）。

### 時間の 2 つの指標

- `Active time` は `system` / `turn_duration` 行（CLI 自身が計測したターン所要時間）の合算。
  ただし**分布が極端に歪む**。セッションを開いたまま放置すると 1 ターンが数十時間として記録される
  ため、合算値だけを「実作業時間」と読むと大きく誤る。median と最長ターンを併記しているので、
  必ずそちらと一緒に読むこと。カバーしたターン数も出す（`turn_duration` を書くのは新しめの CLI
  バージョンのみ）。
- `Session span` は各セッションの「先頭 timestamp 〜 末尾 timestamp」の差分の合算。resume で日を
  またいだセッションのアイドル時間も含むため、実作業時間の上限にすぎない。

### 集計の読み方の注意

- `Bash breakdown` は `input.command` の**1 行目の先頭コマンド**で分類する。heredoc の本文を
  コマンドと誤認しないため、また `cmd | grep` のようなパイプ途中の `grep`（専用ツールに代替手段が
  ない）を「代替可能」に数えないため。`cd` や `VAR=value` の前置は読み飛ばして実コマンドに帰属させ、
  `cd` 前置自体は permission prompt を誘発するパターンとして別途カウントする。
- tool_result のエラー分類は CLI の**文言マッチによるヒューリスティック**（機械可読な理由フィールド
  が無い）。文言が変わると内訳は劣化するが、分類不能は `failure` に落ちるので総数は常に正確。
- compaction の drop 量は各イベントの `preTokens - postTokens` から求める。transcript の
  `cumulativeDroppedTokens` はセッション内の累計であり、イベント単位で合算すると多重計上になる。
- `<synthetic>` のような CLI 生成の擬似モデルはモデル別集計から除外し、除外件数だけを注記する。
- 打ち切られた一覧には `(N more not shown)` を出す。上位 N 件を全件に見せない。
- skill 内訳（`Skills`）は `Tool calls` の `Skill` 行とは別集計。`Tool calls` の `Skill` は
  `input.skill` の有無によらず全 Skill 呼び出しをカウントするのに対し、`Skills` は
  `input.skill` を持つ呼び出しだけをその skill 名で集計する。

## 使い方

```sh
# 既定ディレクトリ（~/.claude/projects）を集計してテーブル表示
go run ./cmd/agent-stats

# 対象ディレクトリ・期間・プロジェクトを絞る／JSON 出力
go run ./cmd/agent-stats summary --dir ~/.claude/projects --since 24h --project my-pde
go run ./cmd/agent-stats --json | jq '.tokens, .model_tokens, .redundant_bash'

# セッション 1 件ずつの明細まで含める（出力サイズは約 3 倍）
go run ./cmd/agent-stats --json --detail | jq '.list[0]'
```

| フラグ      | 既定                 | 意味                                                                      |
| ----------- | -------------------- | ------------------------------------------------------------------------- |
| `--dir`     | `~/.claude/projects` | 走査する transcript ディレクトリ                                          |
| `--since`   | `0`（全件）          | 末尾 timestamp がこの期間内のセッションだけを対象にする（例 `24h`）       |
| `--project` | 空                   | ファイルパスまたは `cwd` にこの部分文字列を含むセッションだけを対象にする |
| `--json`    | `false`              | テーブルの代わりに機械可読な JSON を出力する                              |
| `--detail`  | `false`              | `--json` と併用時、全セッションの明細（`list`）を含める                   |

先頭に任意の `summary` サブコマンドを付けても同じ挙動（将来のサブコマンド拡張のための予約）。

`--detail` を分けているのは、`list` が他の全項目を合わせたより大きく、集計値だけを見たい用途に
そのコストを払わせないため。`list` 抜きでも `top_sessions` / `projects` は出る。

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
