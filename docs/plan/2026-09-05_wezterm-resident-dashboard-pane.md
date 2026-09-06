# Plan: ダッシュボードを全タブ常駐にして描画を Lua に寄せる

## Background

`CMD+SHIFT+A` のペインダッシュボードは押したタブの中にしか出ないので、タブ切替（`CMD+1..9`）とワークスペース切替（`CMD+s` / `CMD+n`）で視界から消える。常に左端に居てほしい。

同じ課題を扱った未実装プラン（`2026-08-21_wezterm-shared-dashboard-pane.md`、コミットせず破棄）は「mux 全体で 1 本を保ち、切り替えの直前に行き先のタブへ引っ越させる」設計だった。一度実装したが体験が悪くオミットされている。悪かった原因は 3 つとも**引っ越しに由来**する。引っ越しがプロセスの kill + respawn なので毎回 0.1〜0.3 秒の空ペインが見える。訪れるタブが毎回 18% リサイズされて nvim / AI CLI が再描画する。そして行き先を先回りするために切替キー 17 個をコールバックへ差し替える必要があった。

### nightly に新しい API は無い

実バイナリ（`20260905-055013-e019f1b1`）から Lua バインディング名を抽出して API 全面を確認した。

| 対象        | 生えているメソッド                                                                                                                                                                                    |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MuxPane`   | `split` `activate` `window` `tab` `move_to_new_tab` `move_to_new_window` `inject_output` `get_progress` `get_user_vars` `get_foreground_process_info` `get_current_working_dir` `get_dimensions` ほか |
| `MuxTab`    | `panes` `panes_with_info` `active_pane` `activate` `set_zoomed` `get_size` `set_title` ほか                                                                                                           |
| `MuxWindow` | `tabs` `active_tab` `active_pane` `get_workspace` `set_workspace` `spawn_tab` `gui_window` ほか                                                                                                       |
| `GuiWindow` | `set_left_status` `set_right_status` `perform_action` `is_focused` `window_id` `active_key_table` `set_position` ほか                                                                                 |

- **既存タブへペインを移す Lua API は無い。** `move_to_new_*` は新しいタブ / ウィンドウを作る側だけで、実移動を持つのは CLI の `wezterm cli split-pane --move-pane-id` のみ。これは 20240203 の頃からあり nightly の追加ではない。Lua への露出を求める issue（wezterm#7916）が 2026-07 から開いたまま
- フローティングペイン / サイドバーに相当する概念は無い
- nightly の changelog にペイン・mux・イベント系の追加は 1 件も無い

つまり「nightly でスマートに解ける」は成り立たない。**代わりに、当時使っていなかった既存 API `pane:inject_output()` が旧プランの前提を崩す。**旧プランが「全タブ常駐」を却下した理由はワークスペース数に比例して描画ループ（`ai-panes.sh` プロセス）が増えることだったが、描画を Lua が担えばプロセスは増えない。常駐にすれば引っ越しが消え、悪かった 3 点が丸ごと消える。

## Current structure

- `ai-panes.lua` が `process_of()` で mux を走査して `nvim:2 claude:1` をタブバー左に出し、`CMD+SHIFT+A` でアクティブタブに `bin/ai-panes.sh` のペインを左 18% に分割していた
- `bin/ai-panes.sh` は 396 行の TUI。`wezterm cli list --format json` と `ps -ao tty=,command=` を `jq` で突き合わせて行を作り、alt screen に自前描画し、`read -rsn1` で j/k/l を捌いていた
- 検知ロジックが Lua（`argv_runs_nvim`）と awk に二重にあり、プラン群が繰り返しリスクに挙げていた
- ジャンプが `user-var-changed` 経由なので、ワークスペースを跨ぐと発火元ペインが不可視側に取り残され 2 回目以降が効かなかった（`2026-09-05_wezterm-nightly-migration.md`）

## Design policy

### 訪れたタブに 1 本ずつ常駐させる

`update-status`（毎秒）で、アクティブタブにダッシュボードが無ければその場に立てる。立てたら閉じない。引っ越しも先回りも無いので、切替キーの差し替え・`workspace-will-switch` イベント・引っ越しの順序芸はすべて不要になる。リサイズはタブごとに初回 1 回だけ。

### 描画とデータは Lua が持ち、ペインは inject 先に徹する

行データは mux の走査だけで揃う（workspace は `win:get_workspace()`、種別は `process_of()`、project は `pane:get_current_working_dir()`）。`wezterm cli list` も `ps` も `jq` も要らなくなり、**検知ロジックの二重化が消えて 1 本になる**。ステータスバーの件数も同じ走査結果から作る。

フレームは Lua が組み立て、アクティブタブのダッシュボードにだけ `inject_output` する。非アクティブタブのぶんは見えないので描かない。

### キーは sink プロセスからの中継にする

素の `j` / `k` / `l` を `config.keys` に取ると、nvim を含む全ペインの打鍵が毎回 Lua コールバックを通る。それは避け、`bin/ai-panes.sh` を**キーを読んで user var に中継するだけの sink** に縮める。選択位置と行データは Lua 側にあるので、旧プランが嫌った「状態と描画が別プロセスに割れる」問題は起きない。

`user-var-changed` が不可視ペインに配送されない件は常駐化そのものが解く。ジャンプ後の次の打鍵は移動先のワークスペースにある**そのタブ自身の**ダッシュボードから飛ぶ。

### on/off は `wezterm.GLOBAL` の 1 フラグ

常駐化により「ペインが無い = まだ立てていない」になるので、旧プランの「存在＝真実」だけでは足りない。on/off は `wezterm.GLOBAL.ai_panes_on`（設定リロードを跨いで保つ）に持ち、タブ内の有無は user var マーカーで判定する。手動で `Ctrl-C` すると次のティックで生え直すが、これは「常に左側に居る」の当然の帰結として受け入れる。off にする手段は `CMD+SHIFT+A` だけ。

### `is_focused()` のガードは外す

旧プランでは必須だった。非アクティブなワークスペースが自分のアクティブタブへダッシュボードを**引き寄せて** ping-pong するのを止めるためで、引っ越しが無い本方式には当てはまらない。GUI ウィンドウが複数あっても、各ウィンドウは自分のアクティブタブに自分のダッシュボードを持つだけで競合しない。外したことで、WezTerm が最前面でない間もダッシュボードが更新され続ける。タブ変化の検出はウィンドウ単位に持つ（`window:window_id()` をキーにする）。

## Implementation steps

1. **`dotfiles/wezterm/ai-panes.lua`** — `count_tracked()` を `collect()`（`{ ws, proc, pane_id, project }` の配列、`ws` → `TRACKED_ORDER` の順位 → `pane_id` でソート）に一般化し、`status_text(rows)` をその消費者にする。`render(rows, ctx)` で旧シェルのフレームを組み立てる。`ensure_dashboard(tab, focus)` / `paint(window, tab)` / `close_all()` を追加。`update-status` は「アクティブタブに立てる → 収集（2 秒スロットル）→ ステータス更新 → inject」。`user-var-changed` は `ai_panes_key` を受けて選択移動またはジャンプ。テスト用に `collect` / `status_text` / `render` / `move_selection` / `resolve_selection` を公開し、`__call` で従来どおり `require("ai-panes")(config)` を受ける。
2. **`dotfiles/wezterm/bin/ai-panes.sh`** — マーカー送出・`stty` の設定と復帰・キー中継・クリーンアップだけの sink に縮める（396 行 → 56 行）。`collect` / `render` / `ps` / `jq` / `wezterm cli` / `--json` / `--once` は削除する。検知が Lua に一本化された以上、第二の実装は嘘の温床になる。
3. **`dotfiles/wezterm/tests/ai-panes_test.lua`** — `wezterm` をスタブして luajit で走らせるハーネス。これまで各プランが使い捨てで書いていたものをリポジトリに残す。
4. **`mise.toml`** — `wezterm-verify` の `ai-panes.sh --json` を Lua テストに差し替える。この設定はもう `cli list` の JSON に依存せず、壊れうるのは Lua の mux API 側なので検知点をそちらへ移す。

`appearance.lua` / `keybindings.lua` / `workspaces.lua` / `wezterm.lua` は変更しない。切替キーの差し替えも `workspace-will-switch` も入れないのが本方式の要点。`~/.config/wezterm` は `dotfiles/wezterm` への symlink なので配布作業は無い。

## File changes

| File                                       | Change                                                                          |
| ------------------------------------------ | ------------------------------------------------------------------------------- |
| `dotfiles/wezterm/ai-panes.lua`            | 収集の行データ化・フレーム生成・常駐と inject・キー中継の受け口。大幅な書き換え |
| `dotfiles/wezterm/bin/ai-panes.sh`         | sink へ縮小。収集・描画・`--json` / `--once` を削除                             |
| `dotfiles/wezterm/tests/ai-panes_test.lua` | 新規。`wezterm` スタブ + 偽 mux ツリーで収集・整形・折り返し・選択を検証        |
| `mise.toml`                                | `wezterm-verify` の `--json` ステップを Lua テストへ差し替え                    |

## Risks and mitigations

| Risk                                                                      | Mitigation                                                                                                                                          |
| ------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| 訪れた全タブが恒久的に 18% 狭くなる                                       | 本方式が引き受ける唯一の代償で、「常に左側に表示される」の定義そのもの。リサイズはタブごとに初回 1 回だけ。狭すぎれば `DASHBOARD_FRACTION` を下げる |
| `update-status` の毎秒走査が GUI スレッドを重くする                       | 収集は 2 秒スロットル、inject はフレームに差分があるときだけ。走査そのものは従来の `count_tracked()` と同じコスト                                   |
| 幅の狭いペインで行が折り返して読めなくなる                                | 幅は `dash:get_dimensions().cols` から取り、プロセス名・project 名・見出し・ヒントをすべて幅に合わせて詰める。14〜37 桁でテストしている             |
| `Ctrl-C` でダッシュボードを閉じても次のティックで生え直す                 | on/off の真実はフラグ側にあるという設計上の帰結。off は `CMD+SHIFT+A` に一本化し、フッタのヒントにも出す                                            |
| `ActivatePaneDirection` がダッシュボードに入る                            | 常駐ペインである以上避けられない。キーボードでフォーカスを乗せる手段でもある                                                                        |
| `--json` を消すと nightly 更新時の `cli list` スキーマ検知が失われる      | この設定はもう `cli list` に依存しない。検知点を Lua テストへ移すのが正しい。`show-keys` による設定読み込み確認は残す                               |
| タブごとに sink プロセスが 1 本増える                                     | `read` で tty をブロックするだけなので CPU はゼロ。旧プランが却下した「タブ数ぶんの描画ループ」とは別物                                             |
| 分割直後は sink がまだマーカーを立てておらず、次のティックで 2 本目が出る | 生成時に pane_id をモジュール側に控え、マーカーと OR で判定する                                                                                     |

## Validation

### 実測で確定した事実

隔離インスタンスに probe を仕込んで測った。`--config-file` を指定しても `require` は `~/.config/wezterm` しか見ないので、probe 側で `package.path` を通してモジュール名を変えたコピーを読ませている。

| 観測点                                                      | 結果                                                                                            |
| ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `pane:inject_output()`                                      | 色・UTF-8 罫線・OSC 8・`ESC[H` + `ESC[0J` の再描画すべて通る。alt screen は使わないので触れない |
| 改行                                                        | tty の line discipline を通らないので `\r\n` が要る                                             |
| `pane:split{ direction="Left", size=0.18, top_level=true }` | 成功。新ペインがアクティブになるので、分割前のペインを `activate()` し直せばフォーカスは戻る    |
| `pane:get_lines_as_text(n)`                                 | 下から n 行を返す。上端を読むには `get_dimensions().viewport_rows` を渡す                       |
| `window:is_focused()`                                       | 最前面でない GUI ウィンドウでは false。ガードを残すと非フォーカス時に描画が止まる               |

### 自動検証（実施済み）

- [x] `dotfiles/wezterm/tests/ai-panes_test.lua`（luajit + `wezterm` スタブ、37 項目）: docker 越し nvim / ホスト nvim / basename がバージョン番号になる claude を拾い、コンテナに入るだけの docker・`nvim.dockerfile` の compose・sink ペインは拾わない。種別順が pane_id より優先される。`status_text()` が `nvim:2 claude:1 codex:1`。フレームのグループ見出し・`●`/`○`・`▸` の数・OSC 8・CRLF 終端・幅追従（14 / 20 / 26 / 37 桁でどの行も折り返さない）。j/k の端での停止と、選択していたペインが消えたときの先頭への復帰
- [x] `bin/ai-panes.sh` を疑似 tty（Python `pty`）で駆動: マーカーを `1` に立てる、`ECHO` と `ICANON` を落として `ISIG` は残す、`j`/`k`/`l`/Enter を `j:1` `k:2` `l:3` `l:4` と連番付きで中継する、`X` / 矢印キー / 日本語は中継もエコーもしない、`Ctrl-C` で exit 0 してマーカーを `0` に戻しカーソルを復帰する
- [x] `mise run wezterm-verify` — `show-keys` exit 0、`SUPER A` / `SUPER s` / `SUPER S` / `SUPER n` / `SUPER R` / `CTRL [` / `SHIFT Enter` が残っている
- [x] 隔離インスタンスでの実機 E2E（2 ワークスペース × 3 タブ、`exec -a nvim sleep` で検知対象を偽装）:
      タブを訪れるとダッシュボードが生え、フォーカスは作業ペインに残る。別タブへ移るとそのタブにも生える。
      同じタブへ戻っても 2 本目は生えない。ダッシュボードは訪れたタブの数だけ（未訪問のワークスペースには生えない）。
      `j` / `k` が sink → user var → Lua → 再 inject を通って `▸` を動かす。
      `l` でワークスペースが `alpha` → `beta` に切り替わり、移動先のタブにもダッシュボードが生えて `●` / `○` が反転する。
      **続けてもう一度 `l` を押すと `beta` → `alpha` に戻る**（旧実装で 1 回しか効かなかった経路の解消確認）

### 手動確認（設定を配ったあとにユーザーが実施）

- [ ] `CMD+SHIFT+A` → アクティブタブの左 18% に出て、フォーカスがダッシュボードに乗る
- [ ] `CMD+1..9` / `CMD+s` / `CMD+n` で移動しても左端に在り、2 回目以降の訪問ではリサイズが起きない
- [ ] 切替直後にそのまま nvim へタイプできる（自動生成でフォーカスを奪わない）
- [ ] クリックジャンプが従来どおり効く
- [ ] `CMD+SHIFT+A` 再押下で全タブから消え、切り替えても復活しない。`ps -Ao command= | grep -c '[a]i-panes.sh'` が `0`
- [ ] タブバー左端の `nvim:2 claude:1` が従来どおり出る

## Open questions

- 常駐の対象は「訪れたタブ」にした。`workspaces.lua` が作る全タブへ先回りして広げるかは、しばらく使ってから判断する
- `pane:get_progress()`（nightly で追加）で AI CLI が実際に動いているかを行に出す案はスコープ外とした。`2026-09-05_wezterm-agent-progress-indicator.md` が本方式の上で実装している
