# nvim-usage

`nvim/config/lua/usage_recorder.lua` が書き出す Neovim の操作ログを **read-only** で集計する CLI。
自作キーマップの使用回数、同じキーの連打、頻出するキー列、Ex コマンドの頻度を出す。
解釈（削除候補や標準機能での代替の提案）は `nvim-usage-review` スキルが担う。

## 入力

`--dir`（既定 `~/.nvim-usage`）配下の次の 2 つを読む。書式は `usage_recorder.lua` との契約。

| パス                      | 中身                                                                                    |
| ------------------------- | --------------------------------------------------------------------------------------- |
| `events/YYYY-MM-DD.jsonl` | 1 行 1 イベント。`kind` は `keys`（キー列のチャンク）/ `cmd`（Ex コマンド名）/ `search` |
| `keymaps.json`            | Neovim 終了時点のキーマップ一覧（n / x / o、バッファローカルを含む）                    |

壊れた行や未知の `kind` は読み飛ばす。ログが無ければ空のレポートを出す。

## 使い方

```sh
mise run nvim-usage:report                     # 直近 30 日を Markdown で
mise run nvim-usage:report -- --format json    # スキル向けの JSON
mise run nvim-usage:report -- --since 0        # 全期間
```

| フラグ     | 既定            | 説明                                                  |
| ---------- | --------------- | ----------------------------------------------------- |
| `--dir`    | `~/.nvim-usage` | ログのディレクトリ                                    |
| `--since`  | `30d`           | 集計期間。`<n>d` または Go の duration、`0` で全期間  |
| `--format` | `md`            | `md` / `json`                                         |
| `--now`    | 現在時刻        | 期間の基準時刻（RFC3339）。テストで固定するために使う |

## 集計の中身

- **キーマップの使用回数**: 入力がそのマッピングとして解決された回数。Neovim の `vim.on_key` は、
  解決されたマッピングを lhs 全体で 1 回の typed として渡し、マッピングにならなかったキーは
  1 つずつ渡す。そのため、同じモード系列（n / o はノーマル、x はビジュアル）のチャンクで
  lhs と一致した要素を数えれば、打ったキーの偶然の並びを拾わずに済む。
- **同じキーの連打**: 同じキーが 4 回以上続いた箇所をキー別に数える（`jjjj` → `4j` などの候補）。
- **頻出シーケンス**: 2〜4 キーの並びを系列ごとに上位 20 件、スペース区切りで出す。打ったマッピングは
  lhs 全体で 1 キーとして数える。同じキーだけの並びは連打側で数える。
- **Ex コマンド**: 正式名（`:vs` → `vsplit`）ごとの回数。引数は記録していない。

## テスト

```sh
mise run nvim-usage:test
go test ./... -update   # golden（testdata/*.golden）を更新する
```

`testdata/usage/` のフィクスチャから Markdown / JSON を生成し、golden と突き合わせる。
