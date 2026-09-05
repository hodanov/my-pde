# Plan: AI エージェントの稼働状態をタブバーとダッシュボードに出す

## Background

`2026-09-05_wezterm-nightly-api-cleanup.md` の積み残しにあった新機能 3 件のうち、`pane:get_progress()` を採る。3 件の中でこれだけ「実弾がある」ことを裏取りできた。

Claude Code のバイナリには `terminalProgressBarEnabled` という設定があり、説明文が literally `Emit OSC 9;4 progress sequences during long operations`、既定値は `true`（`/config` の "Terminal progress bar"）。`indeterminate` / `completed` の状態遷移も持っている。codex / cursor-agent / copilot には該当機能が見当たらないので、当面これは **claude 専用の指標**になる。

しかも今の設定はその恩恵を取り逃していた。nightly は既定のタブバー描画で Indeterminate 時にスピナーを出すようになったが、`appearance.lua` が `format-tab-title` を自前定義しているため既定描画ごと置き換わっている。

併せて `macos_fullscreen_extend_behind_notch` を入れる。ハードは MacBook Air M2（Mac14,2）で notch があり、フルスクリーンを常用しているため。

## Current structure

`appearance.lua` の `format-tab-title` は、パスを剥いだタイトルを固定の空白 1 文字と一緒に出し、非アクティブなら灰色にするだけだった（`icon` という変数名だったが中身は `" "` で、実質は余白）。

ダッシュボードは `2026-09-05_wezterm-resident-dashboard-pane.md` で全タブ常駐になり、収集も描画も `ai-panes.lua` に寄った。`collect()` が mux を走査して行データを組み、`render()` がフレーム文字列を作り、`pane:inject_output()` で sink（`bin/ai-panes.sh`）へ流す。シェルはペインを開いたまま保持してキーを中継するだけで、`wezterm cli list` も `jq` も使わない。

## Design policy

- **progress は行データの一部として扱う。** 収集も描画も Lua 側にあるので、`collect()` が組む行に `progress` を 1 フィールド足せば `render()` まで素通しできる。プロセス間の配管は要らない
- トークンは `busy` / `NN%` / `err` の 3 形に正規化する。表示側に OSC 9;4 の値の形を持ち込まない
- `pane:get_progress()` を持たない wezterm でも行が組めること。`pcall` で包み、失敗は「進捗なし」に落とす
- 幅が足りない行はトークンを落とす。ダッシュボードは画面幅 18% の細いペインなので、折り返しは行の対応関係ごと壊す
- 色は既存の Catppuccin パレットから採り、新しい色は error の red だけに留める

## Implementation steps

### 1. タブバー（`appearance.lua`）

`format-tab-title` で `tab.active_pane.progress`（`PaneInformation.progress`、nightly 追加）を読む。値は `"None"` / `"Indeterminate"` / `{ Percentage = n }` / `{ Error = n }`。docs の例が `or "None"` と書いているので `nil` も来うる前提で扱う。

進捗があるときだけ、先頭の余白を nerdfont のグリフに差し替える。Indeterminate は `md_circle_medium`、Percentage は `md_circle_slice_1..8`（`floor(pct / 12.5) + 1`）、Error は `md_alert_circle_outline`。

色は error のときだけ red `#f38ba8` を当て、直後に `"ResetAttributes"` を挟む。**それ以外では Foreground を一切指定しない。** アクティブタブの背景は mauve `#cba6f7` で、進捗色に mauve を使うとアクティブタブでは見えなくなる。グリフの形だけで状態を区別し、色に頼らない。

### 2. ダッシュボード（`ai-panes.lua`）

`progress_token()` が OSC 9;4 の値を `busy` / `NN%` / `err` に正規化し、`progress_of()` が `pcall` 越しに `pane:get_progress()` を呼ぶ。`collect()` は追跡対象のペインだけを見るので、進捗を引くのも同じ範囲で済む。

`render()` は pane id の直後、OSC 8 ハイパーリンクの内側にトークンを置く。行全体がクリックでジャンプできる性質を保つため。色は `err` だけ red で、それ以外は green。

幅は `render()` が持つ計算にトークン分を足すだけで足りる。`room` からトークン幅（`#token + 1`）を引いた残りをプロセス名のパディングに渡し、引いた結果が 1 桁未満になる幅ではトークン自体を落とす。

収集は `COLLECT_THROTTLE_SECONDS`（2 秒）に間引かれる。`get_progress()` は mux 内のメモリ参照なので毎秒でも回せるが、`get_foreground_process_info()` と同じ走査に相乗りしているので独立したスロットルは持たせない。`state.painted` がフレーム文字列で差分を見るので、進捗が変われば再描画は自動的に走る。

### 3. notch（`appearance.lua`）

`macos_fullscreen_extend_behind_notch = true`。docs が要求する `native_macos_fullscreen_mode = false` は既定値だが、将来 native 側を変えたときに黙って無効化されるので明示的に併記した。

## File changes

- `dotfiles/wezterm/appearance.lua` — `progress_mark()` と `format-tab-title` の書き換え、notch 2 行
- `dotfiles/wezterm/ai-panes.lua` — `progress_token()` / `progress_of()`、`collect()` の行への `progress`、`render()` のトークン列と幅計算
- `dotfiles/wezterm/tests/ai-panes_test.lua` — `progress_token()` の 6 ケース、`collect()` が行に載せること、`render()` の色とリンク内側の位置、幅 10 でトークンを落とすこと

## Risks and mitigations

**タブバーは各タブのアクティブペインしか見ない。** claude が非アクティブペインで回っているタブには出ない。ダッシュボードは全ペインを見るのでそちらが埋める。実測でも、split の非アクティブ側（pane 3）に進捗を立ててもタブバーには出なかった。

**行が長くなる。** 最悪ケース（`cursor-agent` + 3 桁 pane id + `busy`）で 28 桁。ダッシュボードは画面幅 18% なので通常は足りる。足りない幅ではトークンを落として折り返しを避け、テストが幅 10 / 14 / 20 / 26 / 37 で全行が収まることを見張る。

**`get_progress()` は nightly の API。** 安定版へ戻したり nightly が壊したりすると呼び出しが失敗する。`pcall` に包んであるので進捗が消えるだけで、ダッシュボードの他の列は従来どおり出る。

## Validation

タブバーと OSC 9;4 の状態遷移は、隔離インスタンス（`wezterm --config-file ~/.config/wezterm/wezterm.lua start --always-new-process`）で実測した。

**`wezterm cli send-text` は既定でブラケットペーストとして送る。** 改行が Enter にならずプロンプトに積まれるだけで、コマンドが実行されない。`--no-paste` が要る。最初これに気づかず「進捗が立たない」と誤読した。

| 観測点                               | 結果                                                     |
| ------------------------------------ | -------------------------------------------------------- |
| ペイン内で `printf "\033]9;4;3;0\a"` | タブバーのグリフが `md_circle_medium`                    |
| `\033]9;4;1;42\a`                    | グリフは slice 4（`floor(42/12.5)+1`）                   |
| `\033]9;4;2;7\a`                     | グリフは `md_alert_circle_outline`、`is_error=true`      |
| `\033]9;4;0\a`                       | グリフが消える                                           |
| 進捗なし                             | `raw=None`、グリフ nil。タブバーの出力は従来と同一       |
| split の非アクティブ側に進捗         | タブバーには出ない（設計どおり）                         |
| GUI ログ                             | Lua エラー・panic なし。`"ResetAttributes"` は受理される |

ダッシュボード側は常駐化（`2026-09-05_wezterm-resident-dashboard-pane.md`）との統合で描画経路ごと書き直したため、`tests/ai-panes_test.lua` に移した。`mise run wezterm-verify` が `show-keys` と合わせて回す。

静的検証は `mise run wezterm-verify`、`stylua --check`、`markdownlint-cli2` すべて通過。

## Open questions

**Claude Code が実際に OSC 9;4 を流すかは未実測。** 設定の存在と既定 on までは確認したが、稼働中の claude で観測はしていない。実装は OSC を出すどのプログラムにも効くので、出さないと分かっても無駄にはならない。日常利用で出なければ `/config` の "Terminal progress bar" を確認する。

**常駐ダッシュボードでのトークン表示は GUI 実機で未確認。** 統合後の描画はテストで押さえたが、実際に進捗を立てた状態のスクリーンショットは取っていない。反映後の最初の claude 実行で確認する。

codex / cursor-agent / copilot が将来 OSC 9;4 を出すようになれば、設定側は無改修で対応できる。
