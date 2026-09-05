# Plan: AI エージェントの稼働状態をタブバーとダッシュボードに出す

## Background

`2026-09-05_wezterm-nightly-api-cleanup.md` の積み残しにあった新機能 3 件のうち、`pane:get_progress()` を採る。3 件の中でこれだけ「実弾がある」ことを裏取りできた。

Claude Code のバイナリには `terminalProgressBarEnabled` という設定があり、説明文が literally `Emit OSC 9;4 progress sequences during long operations`、既定値は `true`（`/config` の "Terminal progress bar"）。`indeterminate` / `completed` の状態遷移も持っている。codex / cursor-agent / copilot には該当機能が見当たらないので、当面これは **claude 専用の指標**になる。

しかも今の設定はその恩恵を取り逃していた。nightly は既定のタブバー描画で Indeterminate 時にスピナーを出すようになったが、`appearance.lua` が `format-tab-title` を自前定義しているため既定描画ごと置き換わっている。

併せて `macos_fullscreen_extend_behind_notch` を入れる。ハードは MacBook Air M2（Mac14,2）で notch があり、フルスクリーンを常用しているため。

## Current structure

`appearance.lua` の `format-tab-title` は、パスを剥いだタイトルを固定の空白 1 文字と一緒に出し、非アクティブなら灰色にするだけだった（`icon` という変数名だったが中身は `" "` で、実質は余白）。

ダッシュボードは `bin/ai-panes.sh` が `wezterm cli list --format json` と `ps` を tty で join して行を組む。`ai-panes.lua` 側は mux を走査して左ステータスの `nvim:2 claude:1` を作る。

## Design policy

- **progress は `wezterm cli list --format json` に含まれない。** 移行時に数えたスカラーキー 21 個に無いので、ダッシュボードへ出すには Lua からシェルへ渡す配管が要る
- 配管はファイル 1 本。トークンの正規化（`busy` / `NN%` / `err`）は Lua 側で済ませ、シェルにパースを持ち込まない
- ファイルが無い・空・書き出し側が動いていない、のいずれでも既存の表示のまま壊れないこと
- 出力先は既存の環境変数（`AI_PANES_INTERVAL` / `AI_PANES_AGENTS`）と同じ流儀で `AI_PANES_PROGRESS_FILE` から差し替えられるようにする。隔離インスタンスで検証するとき、常用インスタンスと同じファイルを取り合わないため
- 色は既存の Catppuccin パレットから採り、新しい色は error の red だけに留める

## Implementation steps

### 1. タブバー（`appearance.lua`）

`format-tab-title` で `tab.active_pane.progress`（`PaneInformation.progress`、nightly 追加）を読む。値は `"None"` / `"Indeterminate"` / `{ Percentage = n }` / `{ Error = n }`。docs の例が `or "None"` と書いているので `nil` も来うる前提で扱う。

進捗があるときだけ、先頭の余白を nerdfont のグリフに差し替える。Indeterminate は `md_circle_medium`、Percentage は `md_circle_slice_1..8`（`floor(pct / 12.5) + 1`）、Error は `md_alert_circle_outline`。

色は error のときだけ red `#f38ba8` を当て、直後に `"ResetAttributes"` を挟む。**それ以外では Foreground を一切指定しない。** アクティブタブの背景は mauve `#cba6f7` で、進捗色に mauve を使うとアクティブタブでは見えなくなる。グリフの形だけで状態を区別し、色に頼らない。

### 2. ダッシュボード（`ai-panes.lua` + `bin/ai-panes.sh`）

#### Lua 側（書き出し）

既存の `update-status` ハンドラの先頭で `write_progress()` を呼ぶ。既存の `is_focused()` 早期 return より前に置く。背景ウィンドウでも進捗は更新したい。

全 window / tab / pane を走査するが、`count_tracked()` が 3 秒に間引かれている理由は `get_foreground_process_info()` のプロセステーブル参照であって走査そのものではない。`pane:get_progress()` は mux 内のメモリ参照なので毎秒でよい。GUI ウィンドウが複数あると `update-status` が窓の数だけ発火するので、既存の `ai_panes_status_at` と同じ形の 1 秒スロットルを `wezterm.GLOBAL` に置き、内容が前回と同じなら書き込み自体を省く。

書き込みは一時ファイル + `os.rename` で原子的に行う。シェルが毎秒読むので部分書き込みを踏ませない。ディレクトリは最初の `io.open` が失敗したときだけ `mkdir -p` する。

#### シェル側（表示）

`collect()` に `--arg progress "$(cat "$progress_file" 2>/dev/null)"` を足し、jq で `pane_id -> token` に畳んで各行に `busy` を付ける。TSV に列を 1 本増やし、`row_busy` 配列を経由して行末に色付きで出す。

`frame_key` が TSV 本体（`rows_serial`）を含んでいるので、**進捗が変われば再描画は自動的に走る**。追加の仕掛けは要らなかった。

### 3. notch（`appearance.lua`）

`macos_fullscreen_extend_behind_notch = true`。docs が要求する `native_macos_fullscreen_mode = false` は既定値だが、将来 native 側を変えたときに黙って無効化されるので明示的に併記した。

## File changes

- `dotfiles/wezterm/appearance.lua` — `progress_mark()` と `format-tab-title` の書き換え、notch 2 行
- `dotfiles/wezterm/ai-panes.lua` — `progress_token()` / `collect_progress()` / `write_progress()` と `update-status` からの呼び出し
- `dotfiles/wezterm/bin/ai-panes.sh` — `progress_file`、jq の join、`row_busy`、行末の表示

## Risks and mitigations

**タブバーは各タブのアクティブペインしか見ない。** claude が非アクティブペインで回っているタブには出ない。ダッシュボードは全ペインを見るのでそちらが埋める。実測でも、split の非アクティブ側（pane 3）に進捗を立てると進捗ファイルには載るがタブバーには出なかった。

**行が長くなる。** 最悪ケース（`cursor-agent` + 3 桁 pane id + `busy`）で 28 桁。ダッシュボードは画面幅 18% なので通常は足りるが、`stty` が失敗したときのフォールバック幅 26 桁では溢れる。フォールバックに落ちるのは制御端末が無いときだけなので実害はないと判断した。

**常用インスタンスとの取り合い。** 進捗ファイルの既定パスは 1 本なので、隔離インスタンスを同時に起動すると両者が同じファイルを書く。`AI_PANES_PROGRESS_FILE` で逃がす。

## Validation

隔離インスタンス（`AI_PANES_PROGRESS_FILE=<scratch> wezterm --config-file ~/.config/wezterm/wezterm.lua start --always-new-process`）で実測した。

**`wezterm cli send-text` は既定でブラケットペーストとして送る。** 改行が Enter にならずプロンプトに積まれるだけで、コマンドが実行されない。`--no-paste` が要る。最初これに気づかず「進捗が立たない」と誤読した。

| 観測点 | 結果 |
| --- | --- |
| ペイン内で `printf "\033]9;4;3;0\a"` | 進捗ファイルに `<pane>\tbusy`、タブバーのグリフが `md_circle_medium` |
| `\033]9;4;1;42\a` | `<pane>\t42%`、グリフは slice 4（`floor(42/12.5)+1`） |
| `\033]9;4;2;7\a` | `<pane>\terr`、グリフは `md_alert_circle_outline`、`is_error=true` |
| `\033]9;4;0\a` | 該当行が消え、全解除でファイルが空になる |
| 進捗なし | `raw=None`、グリフ nil。タブバーの出力は従来と同一 |
| split の非アクティブ側に進捗 | 進捗ファイルには載る。タブバーには出ない（設計どおり） |
| ダッシュボードの join | `busy` / `42%` / `err` / 空 のすべてで期待どおり。進捗ファイルが存在しないときも全行が空に落ちる |
| 描画バイト列 | `busy` / `42%` は green、`err` は red。トークンは OSC 8 ハイパーリンクの内側にあり、行全体がクリック可能なまま |
| GUI ログ | Lua エラー・panic なし。`"ResetAttributes"` は受理される |
| 静的 | `mise run wezterm-verify`、`stylua --check`、`shfmt -d`、`shellcheck` すべて通過 |

## Open questions

**Claude Code が実際に OSC 9;4 を流すかは未実測。** 設定の存在と既定 on までは確認したが、稼働中の claude で観測はしていない。実装は OSC を出すどのプログラムにも効くので、出さないと分かっても無駄にはならない。日常利用で出なければ `/config` の "Terminal progress bar" を確認する。

codex / cursor-agent / copilot が将来 OSC 9;4 を出すようになれば、設定側は無改修で対応できる。

ダッシュボードの `bin/ai-panes.sh` が Lua とファイルで結ばれたのは初めてで、契約が 1 つ増えた。`2026-08-21_wezterm-shared-dashboard-pane.md` を実装するときはこのファイルの扱いも一緒に見直す。
