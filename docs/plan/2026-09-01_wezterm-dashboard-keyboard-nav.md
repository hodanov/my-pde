# Plan: ダッシュボードペインをキーボードで操作する（j/k/l）

## Context

`CMD+SHIFT+A` で開く AI ペインダッシュボード（`dotfiles/wezterm/bin/ai-panes.sh` を左 18% のペインで走らせる）は、いまジャンプ手段がマウスクリック（OSC 8 ハイパーリンク、`bc447b4a`）と `CMD+a` のファジーピッカーの 2 つしかない。ダッシュボードを見ながら手を離さず選んで飛びたい。

欲しいのは:

- ダッシュボードペインでの文字入力を禁止（打鍵しても表示が乱れない）
- `j` で下、`k` で上に選択を移動
- `l`（および Enter）で選択行のペインへジャンプ

前提として、いまの `toggle_dashboard()` は分割直後に元の作業ペインへフォーカスを戻している。キー操作を効かせるにはフォーカスがダッシュボードに乗っている必要があるので、未実装プラン `docs/plan/2026-09-01_wezterm-dashboard-focus-on-open.md` の内容（`pane:activate()` 削除）を本プランに統合する。

決定事項:

- `l` でジャンプしてもダッシュボードは閉じない（クリックジャンプと同じ挙動）
- `j`/`k`/`l`/Enter 以外のキーは全部無視。`Ctrl-C` だけは従来どおり閉じる

## 設計判断: 選択状態はシェル側に持つ

キー処理を Lua の `key_table` でやると、選択インデックスを `wezterm.GLOBAL` に持ち、それを描画側（別プロセスのシェルスクリプト）へ渡す経路を新設することになる。状態と描画が別プロセスに割れるので採らない。

代わりに **スクリプト自身を TUI にする**。`sleep` を `read -rsn1 -t "$interval"` に置き換えれば、リフレッシュ待ちとキー待ちが同じ 1 本のループになる。選択状態は描画するプロセスの中に閉じる。文字入力の禁止も、スクリプトが全バイトを読み捨てる構造から自動的に得られる。

ジャンプだけはワークスペース切り替えを伴うので Lua に投げ返す必要がある。`wezterm cli activate-pane` はペイン/タブをアクティブにするだけでワークスペースは切り替えないため使えない。**user var を通知チャネルにする**: スクリプトが `SetUserVar` の OSC を吐き、Lua の `user-var-changed` が既存の `find_pane()` / `jump_to()` を呼ぶ。ダッシュボードのマーカー（`ai_panes_dashboard`）で既に実績のある経路。

## Implementation steps

### 1. `dotfiles/wezterm/ai-panes.lua` — 開いたときにダッシュボードへフォーカスを乗せる

`toggle_dashboard()`（299-321 行）の `SplitPane` 後にある `pane:activate()` とその直上のコメント（318-319 行）を削除する。`act.SplitPane` は新しいペインをアクティブにするので、削除すればフォーカスがダッシュボードに残る。

`window:perform_action()` はキュー経由で実行され mux API は即時に効くため、318-319 行はもともと無効だった可能性がある（`docs/plan/2026-08-21_wezterm-shared-dashboard-pane.md` の実測）。削除してもフォーカスが移らなかった場合のみ、mux API での生成に置き換える:

```lua
local dashboard = pane:split({
 direction = "Left",
 size = DASHBOARD_PERCENT / 100,
 command = { args = { DASHBOARD_CMD } },
})
dashboard:activate()
```

`MuxPane:split{}` の `size` は 0〜1 の割合なので `{ Percent = 18 }` ではなく `0.18`。

### 2. `dotfiles/wezterm/ai-panes.lua` — user var 経由のジャンプ受け口

既存の `open-uri` ハンドラ（331-343 行）と対になるものを足す。`find_pane()` / `jump_to()` はそのまま再利用する。

```lua
local JUMP_VAR = "ai_panes_jump"
local JUMP_VAR_PATTERN = "^(%d+):"
```

```lua
wezterm.on("user-var-changed", function(window, _, name, value)
 if name ~= JUMP_VAR then
  return
 end
 local id = value:match(JUMP_VAR_PATTERN)
 if not id then
  return
 end
 local row = find_pane(tonumber(id))
 if row then
  jump_to(window, row)
 else
  window:toast_notification("WezTerm", "Pane #" .. id .. " is gone", nil, 2000)
 end
end)
```

値が `<pane_id>:<連番>` なのは、同じペインへ連続でジャンプしたときに値が変わらず `user-var-changed` が発火しない可能性を潰すため。連番はスクリプト側で単調増加させる。

### 3. `dotfiles/wezterm/bin/ai-panes.sh` — `render()` を「引数で受けた行を描く」関数にする

いま `render()` は内部で `collect` を呼んでいる。選択位置の解決とキー処理に行データが要るので、収集と描画を分離する。

- `render <rows_json> <selected_index>` に変更する。`missing_deps` / `collect` 失敗のメッセージ表示は呼び出し側（`loop`）へ移し、`render` は正常系だけを描く
- 行の描画ループの gutter に選択カーソルを出す。いまの `printf '   %s...'`（空白 3 つ）を「空白 + カーソル 1 文字 + 空白」に変える。選択行は mauve の `▸`、非選択行は空白
- `↳ project` の補足行は親行の選択状態に追従させる（インデントのみ、カーソルは出さない）
- フッタのヒントを差し替える:

  ```text
   j/k move  l/Enter jump
   click / CMD+a  jump
  ```

- `--once` は `render "$(collect)" 0` を呼ぶ

### 4. `dotfiles/wezterm/bin/ai-panes.sh` — `loop()` をキー入力ループにする

```text
起動時:
  saved_stty=$(stty -g </dev/tty)
  stty -echo -icanon min 1 time 0 </dev/tty     # isig は残す = Ctrl-C は効く
  alternate screen + カーソル非表示 + ai_panes_dashboard の SetUserVar（現状どおり）

ループ:
  rows/count が未取得なら collect し直す
  選択の解決: selected_pane が rows に居ればその index、居なければ旧 index を [0, count-1] にクランプ
  frame=$(render "$rows" "$index");  prev と違えば ESC[H + ESC[0J で再描画
  read -rsn1 -t "$interval" key
    タイムアウト（読めなかった）→ rows を再収集して次周
    j → index+1（末尾で止める。ラップしない）、rows は再収集しない
    k → index-1（先頭で止める）、rows は再収集しない
    l / Enter → count>0 なら SetUserVar=ai_panes_jump=<pane_id>:<seq> を吐く（base64）、seq++
    それ以外 → 無視
```

キー押下時に `collect` を回さないのがポイント。`wezterm cli list` + `ps` は 100ms 級かかるので、これをスキップすることで j/k が即応する。行の更新はタイムアウト側（既定 2 秒）だけが担う。

選択は index ではなく **pane_id で覚える**。ペインの増減で行が入れ替わっても選択が別の行へずれない。

`l` の送信は既存のマーカー送出と同じ形:

```bash
printf '\033]1337;SetUserVar=ai_panes_jump=%s\a' "$(printf '%s:%s' "$pane_id" "$seq" | base64)"
```

`cleanup()` に `stty "$saved_stty" </dev/tty` を足す（既存の `ESC[?25h` / `ESC[?1049l` に加えて）。INT / TERM / EXIT トラップは現状の 3 つのままでよい。

### 5. `docs/plan/` の追従

- `docs/plan/2026-09-01_wezterm-dashboard-focus-on-open.md`（未実装）は本プランに統合したので削除する
- `docs/plan/2026-08-21_wezterm-shared-dashboard-pane.md`（未実装）は「分割元へフォーカスを戻す」前提で書かれているので、その記述と Validation 項目を「フォーカスはダッシュボード」に直す

## File changes

| File                                                    | Change                                                                                        |
| ------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `dotfiles/wezterm/ai-panes.lua`                         | `toggle_dashboard()` の `pane:activate()` 削除、`user-var-changed` ハンドラと `JUMP_VAR` 追加 |
| `dotfiles/wezterm/bin/ai-panes.sh`                      | `render()` の引数化と選択カーソル、`loop()` のキー入力化、`stty` の設定と復帰                 |
| `docs/plan/2026-08-21_wezterm-shared-dashboard-pane.md` | フォーカス前提の記述を修正                                                                    |

`~/.config/wezterm` は `dotfiles/wezterm` への symlink（`mise run dotfiles-link`）なので、編集は設定リロードでそのまま効く。配布手順は不要。

コメントは足さない（`.claude/rules/code-comments.md`）。判断の理由はコミットメッセージと本文書に残す。

## 実測メモ

### `user-var-changed` は発火する。ただし起動直後の OSC は取りこぼす

wezterm 20240203 で隔離インスタンス（`wezterm --config-file <probe> start --class <name> --always-new-process`）を立てて確認した:

- ペイン起動から 3 秒後に `SetUserVar` を吐くと `user-var-changed` が発火し、`pane:get_user_vars()` にも載る
- ペイン起動と同時に吐くと**どちらも取りこぼす**。Lua のイベント配線が済む前に OSC が流れるため

ダッシュボードが `ai_panes_jump` を吐くのは `l` の打鍵時、つまり起動から十分後なのでこの競合には当たらない。起動直後に吐く `ai_panes_dashboard` のマーカーは `get_user_vars()` で読む方式なので、イベントを取りこぼしても影響しない。

### Enter は「rc=0 かつ空文字」で届く

`stty -echo -icanon` 下で `IFS= read -rsn1` を使うと、Enter（CR）も LF も **1 バイトとしては
返らず、`read` が改行を行区切りとして食うため rc=0 かつ空文字**になる（疑似 tty で実測）。
`icrnl` は落としていないので CR は LF に変換されるが、どちらでも結果は同じ。

タイムアウトは rc≠0 なので空文字と区別できる。よって case 節は `l | "")` でよく、
リフレッシュのたびにジャンプが暴発することはない（3 秒アイドルさせても追加のジャンプが
出ないことを確認済み）。

### 稼働中の GUI は `require` 先の編集を自動リロードしない

`wezterm.lua` からの `require("ai-panes")` 先を書き換えても、起動しっぱなしの GUI は古い `ai-panes.lua` を持ち続ける（`~/.config/wezterm` が symlink であることも効いている可能性がある）。`wezterm.lua` を `touch` しても拾わなかった。

実機確認は**設定をリロードしてから**行うこと。リロード後に読み込まれた設定では、`SetUserVar=ai_panes_jump=<pane_id>:<seq>` を投げると対象ペインが `is_active=true` になり、投げた側が `false` に落ちることを確認済み。

## Validation

静的検証・自動検証（実施済み）:

- [x] `mise run verify:changed`（`stylua --check` / `shfmt -d` / `shellcheck` / `markdownlint` / `prettier`）
- [x] `wezterm show-keys` — `SUPER a` / `SUPER A` が両方定義され、設定が構文エラー無く読める
- [x] `--json` / `--once` が従来どおり出る
- [x] 疑似 tty（Python `pty`）で `loop()` を駆動:
      `j`/`k` でカーソルが上下し先頭・末尾で止まる、`X` / 日本語 / 矢印キーはエコーも再描画もされない、
      `l` と Enter（CR / LF どちらでも）で `ai_panes_jump=<pane>:<seq>` が連番付きで出る、
      アイドル時のリフレッシュではジャンプしない、`Ctrl-C` で終了し `ECHO` が復帰する
- [x] 行の入れ替わり（3 行 → 2 行 → 0 行 → 1 行）で選択が pane_id を追い、空リストでも壊れない
- [x] 実機で `SetUserVar=ai_panes_jump` → 対象ペインが active になる（設定リロード済みインスタンス）

手動確認（設定リロード後にユーザーが実施）:

- [ ] `CMD+SHIFT+A` → 左 18% にダッシュボードが出て、そのペインにフォーカスが乗っている
- [ ] `j` / `k` で `▸` が動き、`l` または Enter で別ワークスペースの行へワークスペースごと飛ぶ
- [ ] ジャンプ後もダッシュボードは元のタブに残っている
- [ ] `Ctrl-C` / `CMD+SHIFT+A` 再押下で閉じ、閉じたあと他のペインで通常どおり文字入力できる
- [ ] `CMD+a` のジャンプピッカーとクリックジャンプ（`bc447b4a`）が壊れていない

## Risks and mitigations

| Risk                                                                         | Mitigation                                                                                                            |
| ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `stty -echo -icanon` にしたことで `Ctrl-C` が届かず閉じられなくなる          | `isig` は落とさない。pty テストで `Ctrl-C` 終了と `ECHO` 復帰を確認済み                                               |
| ペインが異常終了して `stty` が復帰せず、その tty が壊れたまま残る            | `cleanup` は EXIT トラップにも掛かる。ダッシュボードはペイン直下プロセスなので、閉じればその tty ごと消える           |
| `▸` カーソルが 18% 幅では見つけにくい                                        | 見にくければ選択行を反転（`ESC[7m`）に変更する。ただし反転は 24bit 前景色が背景に回るため、その際は選択行の色を落とす |
| キー押下時に `collect` を回さないので、押した瞬間の行が最大 `$interval` 古い | ジャンプ先が消えていた場合は既存の `jump_to()` / `find_pane()` が toast を出す。クリックジャンプと同じ挙動            |
