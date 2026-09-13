---
name: nvim-usage-review
description: >-
  usage_recorder.lua が ~/.nvim-usage に記録した Neovim の操作ログを nvim-usage CLI で集計し、
  使っていないキーマップの削除候補と、標準のキー・コマンドで置き換えられる操作を
  docs/nvim-usage-review/YYYY-MM-DD.md にレポートする。
  「Neovim の使い方をレビューして」「使ってないキーマップを洗い出して」「nvim usage review」
  などと言われたときに使う。
metadata:
  version: 1
---

# /nvim-usage-review スキル

## Goal

実際の操作ログを根拠に、Neovim の設定と操作の改善提案をまとめる。提案は「削除候補」「標準機能での置き換え」「使えていない既定キー」の 3 種類で、どれにも件数の根拠を添える。設定の変更はしない。

## Workflow

### Step 1: 集計する

`mise run nvim-usage:report -- --format json` を実行する。既定は直近 30 日で、全期間は `--since 0`。

- `period.from` が空なら記録が動いていない。`~/.nvim-usage` のマウントと、イメージの再ビルド・コンテナの再作成を案内して終了する。
- `keymaps` が空なら、Neovim をまだ一度も終了していない（`keymaps.json` は終了時に書かれる）。その旨を伝えて終了する。
- `period.days` が 7 未満なら、データ不足としてレポートの冒頭に書く。

### Step 2: 未使用キーマップを振り分ける

`count` が 0 のキーマップについて、`lhs` と `desc` で `nvim/config/` を検索する。lhs は `<Space>` に展開済みなので、`<Leader>` / `<leader>` 表記でも探す。

- **自作**（`nvim/config/` に定義がある）→ 削除候補。定義箇所を `path:line` で添える。
- **既定・プラグイン由来**（定義が無い）→ 使えていない既定キーの候補。

使用頻度の低い用途のキー（デバッガなど）は、期間が短いと 0 回になる。削除候補には、それがいつ使うものかを添えて、判断をユーザーに委ねる。

### Step 3: 操作パターンを置き換え候補にする

`repeats` と `sequences` を、同じ結果を少ない打鍵で得られる標準のキー・コマンドに対応付ける。

- 提案する前に、コンテナ内の Neovim のヘルプで存在と挙動を裏取りする。コンテナのシェルには `$VIMRUNTIME` が無いので、`docker exec nvim-dev` からは `/usr/local/share/nvim/runtime/doc` を直接読む。Neovim の版は JSON の `nvim` に出ている。
- 同じ操作に既に自作キーマップがあるなら、新しいキーではなくそれを案内する。

### Step 4: 利用スタイルで絞る

`routines/prompts/daily-neovim-trend-scan.md` の「役割」節にある、オーナーの利用スタイルと設定方針に照らして、合わない提案を落とす。プラグインの追加は提案しない。それは Daily Neovim Trend Scan の役割。

### Step 5: レポートを書く

`docs/nvim-usage-review/YYYY-MM-DD.md` に、次の節で書く。

1. 集計の概要（期間・記録のある日数・キー入力数）
2. 削除候補（キーマップ / desc / 定義箇所 / いつ使うものか）
3. 標準機能の活用提案（観測したパターンと件数 → 代替の操作 → 効く場面）
4. 使えていない既定キー（主用途に効くものだけ）

リポジトリは PUBLIC なので、載せるのは集計値と短いキー列だけにする。ログの行は貼らない。

### Step 6: 伝える

レポートのパスと、効果が大きそうな提案を 3 件まで返す。設定の変更やコミットは、ユーザーの指示を待つ。

## Notes

- `count` は、入力がそのマッピングとして解決された回数。数え方は `scripts/nvim-usage/README.md` を参照する。
- 記録は、コンテナの nvim で `NVIM_USAGE_DIR` が設定されているときだけ動く（`nvim/config/lua/usage_recorder.lua`）。インサート・コマンドライン・ターミナルで打った文字は記録していない。
