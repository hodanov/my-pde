# Plan: WezTerm nightly 前提で設定を棚卸しする

## Background

PR #752 で常用チャンネルを nightly へ移したが、あのときの検証は「今の設定が壊れないこと」に主眼があった。壊れてはいないが古い書き方のまま残っているものは手つかずなので、ここで潰す。

nightly の changelog（`20240203-110809-5046fc22` 以降の全件）と現行 docs の deprecation マーカーを、`dotfiles/wezterm/**` の API 使用箇所すべてと突き合わせた。手元は `wezterm 20260905-055013-e019f1b1`。

結果、壊れている箇所は 1 件も無い。直すのは「公式に非推奨」か「コメントに書かれた前提が今の WezTerm では成り立っていない」ものだけで、挙動は変えない。

## Current structure

非推奨とされている項目のうち、現行設定が触れていたのは `update-right-status` だけだった。`show_update_window`、`font_antialias` / `font_hinting`、`kde_window_background_blur`、`Copy` / `Paste` / `PastePrimarySelection` アクション、`pane:mux_pane()` はいずれも未使用。

いっぽうで、コードに書かれた前提のほうが 2 件陳腐化していた。`ai-panes.lua` の `find_pane()` は「この wezterm には `mux.get_pane()` が無い」という誤った前提で mux を全走査しており、`status_text()` のキャッシュは「`wezterm.GLOBAL` のネストしたテーブル書き換えは保持されない」という既に解消済みの制約を理由にスカラー 2 本で持っていた。

## Design policy

- 挙動・見た目を変える新オプションは入れない。`text_min_contrast_ratio` や `command_palette_font` の類は別途判断する
- `show_close_tab_button_in_tabs` は既定（`true`）のまま。nightly でタブに `×` が出るようになったが、これは残す
- 陳腐化したコメントは、それが説明している当のコードを書き直すタイミングで落とす。理由はこの文書に移す

## Implementation steps

### 1. `update-right-status` は公式に非推奨

`workspaces.lua` のステータス表示が旧イベントに残っていた。docs（`config/lua/window-events/update-right-status.md`）に「deprecated、`update-status` へ移行せよ」と明記がある。非推奨化は `20220903-194523-3bb1ed61` なので nightly 移行で生じたものではないが、`ai-panes.lua` が既に `update-status` を使っている以上、片方だけ旧イベントに残す理由が無い。

イベント名だけ差し替え、`window:set_right_status(window:active_workspace())` はそのまま。同一イベントに複数ハンドラを登録しても全部が呼ばれ、書き込むスロットが `set_right_status` / `set_left_status` で別なので競合しない。`ai-panes.lua` にあった共存の説明コメントは、前提が消えたので削除する。

### 2. `mux.get_pane()` は存在する

`wezterm.mux.get_pane()` は `20220624-141144-bd1b7c5d` から、`pane:window()` は `20220807-113146-c2fee766` からある。移行前の stable にも入っていた。

`find_pane()` を pane_id から直接引く形にする。ジャンプ 1 回あたり「全 window × 全 tab × 全 pane」を舐めていたのが定数時間の解決になり、`open-uri`（クリック）と `user-var-changed`（`l` キー）の両経路が恩恵を受ける。

これで `each_pane()` の呼び出し元が `count_tracked()` だけになり、戻り値を見て早期 return する仕組みが誰にも使われなくなるので、`count_tracked()` の中に素の入れ子ループとして畳む。

### 3. `wezterm.GLOBAL` のネストしたテーブル書き換えは、もう保持される

`status_text()` のキャッシュに付いた制約の説明は `20230320-124340-559cb7b0` で解消済み（`config/lua/wezterm/GLOBAL.md`）。スカラー 2 本で持つこと自体は今も正しく動き、テーブル化しても得るものが無いので、コードは変えずコメントだけ落とす。

## File changes

- `dotfiles/wezterm/workspaces.lua` — `update-right-status` → `update-status`
- `dotfiles/wezterm/ai-panes.lua` — `find_pane()` の書き換え、`each_pane()` を `count_tracked()` へ畳み込み、陳腐化したコメント 2 件の削除
- `docs/plan/2026-09-05_wezterm-nightly-migration.md` — 積み残しに残っていた `mux.get_pane()` の判断を解決済みとして参照を張る

`appearance.lua` / `keybindings.lua` / `wezterm.lua` / `bin/ai-panes.sh` は変更しない。

## Risks and mitigations

**`mux.get_pane()` は不明な id に対して `nil` ではなく Lua エラーを送出する。** nightly の実装（`lua-api-crates/mux/src/lib.rs`）が内部で `pane.resolve(&mux)?` しているため。`pcall` を外すと「Pane #N is gone」トーストの経路がそのまま設定のクラッシュに化けるので、ここは外せない。いっぽう `pane:window()` は `Option` を返すので `nil` チェックで足りる。

`~/.config/wezterm` は `dotfiles/wezterm` へのディレクトリ symlink なので、リポジトリを編集した時点で実設定が変わる。稼働中のインスタンスは `require` 先の変更を自動リロードしないため実害は出ないが、実機確認は必ず隔離インスタンスで行う。

## Validation

隔離インスタンス（`wezterm --config-file ~/.config/wezterm/wezterm.lua start --always-new-process`）に一時的な `wezterm.log_error` を仕込んで測った。

| 観測点 | 結果 |
| --- | --- |
| `find_pane(6)`（実在） | `mux.get_pane` 成功、`pane:window():get_workspace()` が `my-pde` を返す。別 workspace のペインも 1 発で解決できる |
| `find_pane(99999)`（不在） | `pcall` が `ok=false, err="pane id 99999 not found in mux"` を受ける。`find_pane` は `nil` を返し、gone トーストの経路が保たれる |
| `update-status` ハンドラ 2 本 | `workspaces.lua` 側・`ai-panes.lua` 側とも毎秒発火する。片方だけ呼ばれることはない |
| `gui-startup` | 4 workspace・タブ名・`stable-diffusion` の `Bottom` split すべて従来どおり生成される |
| `mise run wezterm-verify` | `show-keys` exit 0、`ai-panes.sh --json` が配列を返す |

`user-var-changed` は背景ウィンドウには配送されないので、`wezterm cli send-text` で OSC 1337 を流し込んでもハンドラは呼ばれない。これは `2026-09-05_wezterm-nightly-migration.md` に記録済みの既存の性質で、今回の変更とは無関係。`find_pane()` を直接叩く probe に切り替えて測った。

## Open questions

nightly で挙動が変わったが対応不要と判断したもの:

- **nucleo fuzzy matcher への移行** — `CMD+s` の `InputSelector({ fuzzy = true })` のマッチングが fzf 寄りになる。改善なのでそのまま
- **DECRQCRA が既定で無効** — `enable_checksum_rectangular_area` は設定しない。セキュアな既定を維持する
- **dim/bold のコントラスト改善** — 既定の変更。設定側で打ち消さない
- **Copy Mode `Close` がスクロール位置を変えなくなった** — copy mode を自前定義していないので影響なし
- **`ShowTabNavigator` が既定でアクティブタブを選ぶ** — 未使用

見た目・挙動を変えるので今回は入れなかった新機能。採用するなら個別に判断する:

- `text_min_contrast_ratio` — 背景画像 `opacity = 0.13` と Catppuccin Mocha の組み合わせで効きそう
- `command_palette_font` / `char_select_font` / `pane_select_font` — 既定は `window_frame.font`（Roboto）なので日本語グリフを持たない
- `PromptInputLine` の `prompt` / `initial_value`、`launcher_alphabet`、`macos_fullscreen_extend_behind_notch`
- `pane:get_progress()` — ConEmu 形式の進捗。AI ペインダッシュボードに「動いているか」を出す材料になりうる。`2026-08-21_wezterm-shared-dashboard-pane.md` と一緒に検討する
