# Plan: WezTerm を nightly チャンネルへ移行する

WezTerm の安定版は `20240203-110809-5046fc22` から 2 年半以上更新されていない。macOS 周りの修正・新オプション・`tmux -CC` 対応などはすべて nightly にしか入らないため、常用チャンネルを nightly へ移す。

`dotfiles/wezterm/` は mux 全走査・`user_var`・`open-uri` の独自スキーム・`wezterm cli list --format json` の JSON 契約と、公開 API の細かい挙動に深く依存している。切り替えの主眼は「新機能を取り込むこと」ではなく「足元が動いても今の設定が壊れないことを確認すること」に置いた。

## 決定

- チャンネル: brew cask を `wezterm` から `wezterm@nightly` へ差し替える
- 互換分岐は入れない。nightly 前提で書き、ロールバックは cask を戻して `git revert` する
- 新機能の取り込みと未実装プラン（`2026-04-04` / `2026-08-21`）の消化はスコープ外

`wezterm` と `wezterm@nightly` は互いに `conflicts_with` で、どちらも `/Applications/WezTerm.app` に入る。したがって brew は最後まで触らず、`~/Applications/WezTerm-nightly.app` に隔離インストールした nightly ですべての検証を通してから昇格させる。

## 検証手順

再実行できるよう手順を残す。`NIGHTLY=~/Applications/WezTerm-nightly.app/Contents/MacOS/wezterm` とする。

1. **退避** — `/Applications/WezTerm.app` を `~/Applications/WezTerm-stable-20240203.app` へコピー。cask が将来 Homebrew から消えても戻せるようにする
2. **隔離インストール** — `WezTerm-macos-nightly.zip` を取得し、同梱の `.sha256` と突き合わせてから `~/Applications` へ展開。`/Applications` には置かない
3. **静的検証** — `$NIGHTLY --config-file ~/.config/wezterm/wezterm.lua show-keys`。GUI とは別プロセスで設定を読むので、構文エラー・未知アクション・未知設定キーがここで落ちる。stable の出力と diff を取ると、デフォルトキーの変更と自前キーの登録状況が一度に見える
4. **隔離 GUI** — `$NIGHTLY --config-file ... start --always-new-process`。常用インスタンスを汚さずに実機挙動を見る
5. **CLI 契約** — 隔離インスタンスのソケットを `WEZTERM_UNIX_SOCKET` に指定すれば、GUI を触らずに `cli list` / `ai-panes.sh --json` を叩ける

検証対象のバージョンは `20260904-143531-d4f5c487`。

## 実測で確定した事実

### 非互換は 1 件も無かった

| 項目                             | 結果                                                                                                                                                          |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `show-keys` の設定読み込み       | exit 0、警告なし。削除・リネームされた設定キーもアクションも無い                                                                                              |
| キー割り当て 277 行の diff       | 実効差分なし。`NotNan(1.0)` → `1.0` の Debug 表記、`PromptInputLine` へのフィールド追加、CopyMode `Close` の `Multiple([ScrollToBottom, Close])` 包み込みのみ |
| `ls-fonts` のフォント解決        | stable と完全一致                                                                                                                                             |
| `gui-startup` のレイアウト生成   | 4 workspace・タブ名・split すべて意図どおり                                                                                                                   |
| `wezterm cli list --format json` | スカラーキー 21 個が完全一致。`ai-panes.sh` の jq join は無改修で通る                                                                                         |
| `get_foreground_process_info()`  | `name` / `argv` / `executable` 健在。`process_of()` の 3 段フォールバックが argv 経由で正しく命中                                                             |
| `front_end = "WebGpu"`           | 起動・描画・長時間稼働で panic なし                                                                                                                           |

changelog 上の `tab:panes()` の zoom 挙動変更は、`tab:set_zoomed(true)` 下で両バージョンとも全ペインを返し、差は観測できなかった。`find_dashboard()` / `count_tracked()` に影響しない。

### `user-var-changed` は発火元ペインが表示されていないと配送されない

ジャンプ機構を調べる過程で確定した、nightly とは無関係の既存の性質。

- ジャンプ発火元のペインが、どの GUI ウィンドウにも表示されていない mux ウィンドウに属していると、`user-var-changed` ハンドラは呼ばれない
- 1 回目のジャンプで `SwitchToWorkspace` が GUI ウィンドウを別 workspace へ移すため、発火元のダッシュボードは「見えない側」に取り残される。結果として **workspace をまたぐジャンプは 1 回しか効かない**
- `20240203` と `20260904` で挙動は完全に同一。nightly の回帰ではない

`2026-08-21_wezterm-shared-dashboard-pane.md` が扱う「ダッシュボードをアクティブタブへ追従させる」設計は、この制約の直接の帰結でもある。追従が実装されればダッシュボードは常に表示側に居続けるので、この問題も同時に解ける。

計測は「同じ刺激を両バージョンへ与える」形にしないと結論を誤る。最初の比較では stable 側で 1 回目・nightly 側で 3 回目のジャンプを見ており、nightly の回帰だと誤認した。

### `--config-file` を指定してもモジュール解決は `~/.config/wezterm` が優先される

設定一式を別ディレクトリへ複製し `--config-file <copy>/wezterm.lua` で起動しても、`require("ai-panes")` は `~/.config/wezterm/ai-panes.lua`（実物）を読む。複製にログを仕込んで実設定を計測する手は使えない。モジュール名を変えた自己完結の probe を書くこと。

## 運用

- `wezterm@nightly` は `version :latest` の cask なので、`brew upgrade --greedy` を打たない限り勝手には上がらない。更新タイミングは手元で握れる
- 更新後は `mise run wezterm-verify` を打つ。`show-keys` で設定の読み込みを、`ai-panes.sh --json` で `cli list` のスキーマを見る
- `2026-08-19` / `2026-08-21` に埋まっている `20240203-110809-5046fc22` の記述は、当時の実測事実としてそのまま残す

## ロールバック

```sh
brew uninstall --cask wezterm@nightly
brew install --cask wezterm
# cask が失われていたら ~/Applications/WezTerm-stable-20240203.app を /Applications へ戻す
```

## 積み残し

- **Phase 5（brew の差し替え）は未実行。** 検証はすべて通っているが、`/Applications/WezTerm.app` の置き換えは実行中のセッションを巻き込むため手動で行う。差し替え後は `WEZTERM_EXECUTABLE` が指す CLI と稼働中の mux の版が食い違うので、WezTerm を一度再起動する
- `ai-panes.lua` の `find_pane()` に付いた「この wezterm には `mux.get_pane()` が無い」というコメントは nightly では成り立たない（`mux.get_pane` は存在する）。全走査を残すか置き換えるかは別途判断する
