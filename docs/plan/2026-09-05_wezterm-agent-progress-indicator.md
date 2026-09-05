# Plan: AI エージェントの稼働状態をタブバーとダッシュボードに出す

## Background

`2026-09-05_wezterm-nightly-api-cleanup.md` の積み残しにあった新機能 3 件のうち、`pane:get_progress()` を採る。3 件の中でこれだけ「実弾がある」ことを裏取りできた。

Claude Code のバイナリには `terminalProgressBarEnabled` という設定があり、説明文が literally `Emit OSC 9;4 progress sequences during long operations`、既定値は `true`（`/config` の "Terminal progress bar"）。`indeterminate` / `completed` の状態遷移も持っている。codex / cursor-agent / copilot には該当機能が見当たらないので、当面これは **claude 専用の指標**になる。

しかも今の設定はその恩恵を取り逃していた。nightly は既定のタブバー描画で Indeterminate 時にスピナーを出すようになったが、`appearance.lua` が `format-tab-title` を自前定義しているため既定描画ごと置き換わっている。

ただしこの設定だけでは足りない。Claude Code の送信ゲートはターミナルの許可リスト方式で、ConEmu / ghostty≥1.2.0 / iTerm.app≥3.6.6 しか通さず、`TERM_PROGRAM=WezTerm` は素通りして `false` に落ちる。初版はここを見落として実装したため、タブバーもダッシュボードも `"None"` しか受け取れず丸ごと無効だった。`ConEmuANSI` が立っていれば無条件で通るので、WezTerm 側からこれを渡してゲートを開ける。

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

### 3. 送信ゲート（`ai-panes.lua`）

`config.set_environment_variables` で `ConEmuANSI = "ON"` を渡す。Claude Code の `progressReporting` 判定は `ConEmuANSI` / `ConEmuPID` / `ConEmuTask` のいずれかがあれば無条件で真を返す。

WezTerm は元々 ConEmu 形式の `ESC ] 9 ; 4 ; st ; pr ST` を実装している（`pane:get_progress()` の docs がそう書いている）ので、これは能力の誤申告ではない。ターミナル名の判定は `TERM_PROGRAM` を ConEmu チェックより先に見るため、Claude Code から見た terminal は `"WezTerm"` のまま変わらず、ハイパーリンクや kitty keyboard protocol の能力判定には影響しない。

置き場所を `ai-panes.lua` にしたのは、この env var が稼働状態表示のためだけに存在し、同モジュールがその機能を端から端まで持っているため。

### 4. notch（`appearance.lua`）

`macos_fullscreen_extend_behind_notch = true`。docs が要求する `native_macos_fullscreen_mode = false` は既定値だが、将来 native 側を変えたときに黙って無効化されるので明示的に併記した。

## File changes

- `dotfiles/wezterm/appearance.lua` — `progress_mark()` と `format-tab-title` の書き換え、notch 2 行
- `dotfiles/wezterm/ai-panes.lua` — `progress_token()` / `progress_of()`、`collect()` の行への `progress`、`render()` のトークン列と幅計算、`config.set_environment_variables` の `ConEmuANSI`
- `dotfiles/wezterm/tests/ai-panes_test.lua` — `progress_token()` の 6 ケース、`collect()` が行に載せること、`render()` の色とリンク内側の位置、幅 10 でトークンを落とすこと

## Risks and mitigations

**タブバーは各タブのアクティブペインしか見ない。** claude が非アクティブペインで回っているタブには出ない。ダッシュボードは全ペインを見るのでそちらが埋める。実測でも、split の非アクティブ側（pane 3）に進捗を立ててもタブバーには出なかった。

**行が長くなる。** 最悪ケース（`cursor-agent` + 3 桁 pane id + `busy`）で 28 桁。ダッシュボードは画面幅 18% なので通常は足りる。足りない幅ではトークンを落として折り返しを避け、テストが幅 10 / 14 / 20 / 26 / 37 で全行が収まることを見張る。

**`get_progress()` は nightly の API。** 安定版へ戻したり nightly が壊したりすると呼び出しが失敗する。`pcall` に包んであるので進捗が消えるだけで、ダッシュボードの他の列は従来どおり出る。

**`ConEmuANSI` は wezterm が spawn する全プロセスに漏れる。** ConEmu 用の分岐を持つツールが誤動作しうる。macOS でこの変数を見るツールはほぼ無く、見る場合も「ANSI が使える」方向にしか倒れない。1 行削れば元に戻る。

**Claude Code 側のゲートは非公開の内部実装。** 将来のバージョンで判定が変われば再び沈黙する。沈黙しても `progress == "None"` の経路に落ちるだけで、ダッシュボードの他の列とタブタイトルは従来どおり出る。

**反映には再起動が 2 段要る。** `set_environment_variables` は新規 spawn にしか効かず、claude は起動時に env を読む。WezTerm を再起動し、さらに claude を起動し直さないと変化しない。

**claude から来るのは `Indeterminate` と `None` だけ。** Claude Code の分岐は `indeterminate` と `completed`（= clear）の 2 つしか通らない。よってダッシュボードのトークンは常に `busy`、タブグリフは常に `md_circle_medium` になる。`PROGRESS_SLICES` と `err` は OSC を出す他プログラム向けの汎用サポートとして残す。

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

`ConEmuANSI` の効きは、`script(1)` の pty 越しに claude を起動して生バイト列を捕まえて実測した。wezterm を介さないので受信側の実装から独立に、送信側だけを切り分けられる。

| 条件                   | 捕まえた OSC 9;4                                 |
| ---------------------- | ------------------------------------------------ |
| `ConEmuANSI` なし      | ゼロ。OSC は `0`（タイトル）と `8`（リンク）だけ |
| `ConEmuANSI=ON`        | `ESC]9;4;0;BEL`（起動直後 idle）                 |
| `ConEmuANSI=ON` で推論 | `9;4;0` → `9;4;3`（推論開始）→ `9;4;0`（完了）   |

同じキャプチャでタイトルの遷移も取れた: `✳ Claude Code` → `◐ Basic arithmetic` → `◑ …`（推論中はアニメーション）→ `✳ …`。**タブ先頭に見えていた「推論中のアイコン」はこれで、`progress_mark()` のグリフではなかった。** タイトルは高頻度に変わるのでタブバーの再計算頻度に表示が引きずられる。progress 由来のグリフは状態変化時のみのイベント駆動なので、この弱点を持たない。

隔離インスタンスで `ConEmuANSI=ON` / `TERM_PROGRAM=WezTerm` の併存も確認済み。

ダッシュボード側は常駐化（`2026-09-05_wezterm-resident-dashboard-pane.md`）との統合で描画経路ごと書き直したため、`tests/ai-panes_test.lua` に移した。`mise run wezterm-verify` が `show-keys` と合わせて回す。

静的検証は `mise run wezterm-verify`、`stylua --check`、`markdownlint-cli2` すべて通過。

## Open questions

**非アクティブタブでグリフが追随するかは実機確認待ち。** docs は「progress OSC を処理して状態が変化したら wezterm がタブバー更新をトリガーし `format-tab-title` が走る」と書いており、1 秒ポーリングではなくイベント駆動なので取りこぼさないはず。claude を回したまま別タブへ移り、busy の開始と終了にグリフが追随するかを見る。追随しなければ、常駐ダッシュボード側は `inject_output()` で自前に描くのでタブバーの再計算に依存せず、そちらだけが残る。

codex / cursor-agent / copilot が将来 OSC 9;4 を出すようになれば、設定側は無改修で対応できる。
