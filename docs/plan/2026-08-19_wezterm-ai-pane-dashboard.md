# Plan: WezTerm に AI CLI ダッシュボードペインを追加する

全ワークスペース横断で「いまどのプロジェクトでどの AI CLI が動いているか」を表示する細い左ペインと、そこへ飛ぶ fuzzy ピッカー、そしてどのタブからでも総数が見えるステータスバーのサマリを WezTerm の Lua 設定に足す。Herdr のような AI agent manager は導入せず、既存のワークスペース運用を一切変えずに「AI の所在を把握する」UI だけを自前で持つ。

## Background

- 複数プロジェクトで claude / codex を並行して動かすようになり、どのワークスペースのどのタブで何が走っているか把握できなくなった。
- Herdr 等の AI agent manager が解くのは「AI を起動・監視・detach/attach して**管理する**」問題で、欲しいのは「**所在を把握する**」ことだけ。前者を入れると現在の workspace + tab 構成（nvim / ai-cli / ops）ごと運用が置き換わる。
- WezTerm は Lua から mux 配下の全 window / tab / pane を走査でき、各ペインの foreground プロセス名と cwd も取れる。必要な情報はすべて WezTerm 自身が持っているので、外部ツールではなく欲しい UI だけを足す。
- 状態表示（working / waiting）は初回スコープに含めない。そこまでやると Herdr の再実装になる。
- workspace は今後もプロダクト単位のまま維持し、左ペインだけが全 workspace を横断して見る。

## Current structure

- `dotfiles/wezterm/wezterm.lua` — エントリポイント。`config_builder()` を作り各モジュールを `require("mod")(config)` で順に適用する。
- `dotfiles/wezterm/keybindings.lua` — `config.keys` を**新規代入**する唯一のモジュール。workspace セレクタ（fuzzy `InputSelector` / `ShowLauncherArgs`）もここ。
- `dotfiles/wezterm/workspaces.lua` — データ駆動の `workspace_defs`、`gui-startup` でのレイアウト生成、`update-right-status`、`table.insert(config.keys, ...)`。
- `dotfiles/wezterm/appearance.lua` — Catppuccin Mocha。色は hex 直書きでパレット変数は無い。
- 既存キー: `CTRL+[` / `SHIFT+Enter` / `CMD+f`(無効化) / `CMD+s` / `CMD+SHIFT+S` / `CTRL+SHIFT+W` / `CMD+n` / `CMD+SHIFT+R`。leader key と key_tables は未使用。
- デプロイは `mise run dotfiles-link` が `dotfiles/wezterm` を**ディレクトリごと** `~/.config/wezterm` へ symlink する。配下にファイルを増やしてもタスクの変更は不要で、`lint:shell` / `lint:stylua` はリポジトリ全体を走査するので lint 設定も不要。

### 実測で確定した環境の事実

- `wezterm 20240203-110809-5046fc22`。
- `wezterm cli list --format json` は全 workspace の全ペインを `workspace` / `pane_id` / `tab_title` / `cwd`（`file://` URL）/ `tty_name` 付きで返すが、**foreground プロセス名は持たない**。`title` も当てにならない（zsh のままだったり空だったり）。
- `ps -Ao tty=,comm=` を 1 回叩けば全プロセスが tty 付きで取れる。`wezterm cli list` は `/dev/ttys008`、`ps` は `ttys008` を返すので join 前に `/dev/` の有無を揃える必要がある。
- workspace を切り替える `wezterm cli` サブコマンドは**存在しない**。ジャンプは Lua 側でやるしかない。
- 使う Lua API はインストール済み `wezterm-gui` バイナリのシンボルで存在を確認済み（`get_foreground_process_name` / `get_user_vars` / `activate` / `config_dir` / `SplitPane` / `InputSelector` / `CloseCurrentPane` / `toast_notification` / Url の `file_path`）。一方 **`wezterm.mux.get_pane()` はこのバージョンには無い**。
- ホストの `/bin/bash` は 3.2（連想配列なし）。`jq` はあり、`fzf` は無い。
- **`wezterm cli spawn` / `split-pane` は呼び出したクライアントの環境を子プロセスに渡すが、Lua の `act.SplitPane` は WezTerm GUI プロセスの環境で起動する。** GUI の PATH にはログインシェルの初期化が反映されておらず、Homebrew の `/opt/homebrew/bin`（`jq` / `wezterm` 本体）も mise の shim も入っていない。CLI 経由の検証だけでは絶対に再現しない差分。
- `act.CloseCurrentPane` は `window:perform_action(action, pane)` の第 2 引数ではなく「いまアクティブなペイン」を閉じる。任意のペインを閉じる用途には使えない。
- **`get_foreground_process_name()` は解決後の実パスを返す。** `claude` は `~/.local/bin/claude` が `~/.local/share/claude/versions/<version>` への symlink なので、basename がバージョン番号（実測で `2.1.235`）になり名前一致に失敗する。`get_foreground_process_info()` の実測値は `name=2.1.235` / `argv[1]=claude` / `executable=~/.local/bin/claude`（symlink のまま）で、**判定に使えるのは `argv[0]` と `executable`**。一方シェル側の `ps -o comm=` は `claude` を返すため、この問題はシェル側には出ない。
- AI CLI はホスト上で直接起動される（docker 経由なのは nvim だけ）ので、`ps` の `comm` に素のコマンド名で出る。

## Design policy

- 新機能は `dotfiles/wezterm/ai-panes.lua` 1 ファイルに閉じる。`keybindings.lua` / `workspaces.lua` / `appearance.lua` は変更しない。
- **役割分担**: 「一覧の描画」はシェル（左ペインで動き続ける別プロセス）、「ジャンプ」は Lua（workspace 切替が CLI から出来ないため）。ダッシュボードペインは読み取り専用の掲示板に徹し、対話機能は持たせない。
- Lua 側は mux API のみを使い、サブプロセスを一切起動しない。`wezterm.run_child_process` は GUI スレッドを同期ブロックし、`wezterm cli` は同一プロセスの mux へ問い合わせるため、Lua から `wezterm cli` を呼ぶとデッドロックの危険がある。
- 副作用として AI CLI 名のリストが Lua とシェルの 2 箇所に重複する。名前は 4 つなので許容し、両方にクロスリファレンスのコメントを置く。3 箇所目が必要になったら単一ソースのファイルに切り出す。
- ダッシュボードスクリプトは `dotfiles/wezterm/bin/ai-panes.sh` に置く。symlink デプロイに便乗でき、`lint:shell` にも自動で乗る。`--once` / `--json` サブモードを持たせて手動検証と将来の作り替えに備える。
- ペインは**キーでトグル**。起動時の自動配置はやらない（4 ワークスペース分の描画ループ常駐と、nvim の横幅が常に削られるのを避ける）。
- ダッシュボードペインの識別は、スクリプトが起動時に立てる **user var** を単一の真実とする。`wezterm.GLOBAL` に pane_id を持つ方式はペインを手動で閉じたときに stale な id が残り、`mux.get_pane()` が無いこのバージョンでは生死確認も難しい。user var ならペインが消えれば目印も消える。

### AI CLI の検知はプロセス名のみで行う

`ps` の STAT に付く `+`（フォアグラウンドプロセスグループ）で絞る案もあるが、採用しない。`claude` が Bash ツールで子プロセスを走らせている間に一覧から消えるリスクがあり、逆に「フォアグラウンドでない claude をどう扱うか」は本機能の目的（所在の把握）にとって区別する意味がない。tty が一致する既知の AI CLI 名を拾うだけにする。

### プロジェクト名は git を叩かずワークスペース名を主にする

この環境では workspace 名がそのままプロダクト名（`blog` / `my-pde` / `new-project`）で、ジャンプ先の指定にも workspace 名を使う。よって一覧の主キーは workspace 名とし、cwd の basename が workspace 名と食い違うペイン（実測で `new-project` workspace に cwd が `qbittorrent` のペインが存在した）でだけ `↳` の 2 行目で補足する。`git rev-parse --show-toplevel` はペイン数ぶんのコストに対して得られる情報が「サブディレクトリにいるときにリポジトリ名を出せる」だけで、bash 3.2 にはキャッシュ用の連想配列も無い。必要になったらオプトインで足す。

### スクリプトは PATH を自前で用意し、依存欠落を可視化する

`ai-panes.lua` はスクリプトをペインのプログラムとして直接起動する（間にログインシェルを挟まない。挟むと Ctrl-C でペインが閉じなくなる）。そのため PATH は GUI プロセスのものになり、`jq` も `wezterm` も見つからない。スクリプト冒頭で mise の shim、`/opt/homebrew/bin`、`/usr/local/bin`、そして `$WEZTERM_EXECUTABLE` のあるディレクトリを明示的に足す。

あわせて `wezterm` / `jq` / `ps` / `awk` の存在を毎描画チェックし、欠けていればペインに名指しで出す。依存が無いまま `ps` が失敗すると `no AI running` という**もっともらしい嘘**が表示されてしまい、原因にたどり着けない。

### 空のときも黙らない

AI が 1 本も動いていないとき、`toast_notification` で知らせる実装は macOS の通知設定次第で無音になり「キーが効いていない」ように見える（実際にそう報告された）。代わりに `InputSelector` 自体は開き、`no AI panes` の 1 行だけを候補として出す。センチネル id を用意し、選ばれても何もせずセレクタを閉じる。トーストは「選んだペインが既に閉じられていた」ケースにだけ残す。

### 常時可視性はステータスバーで補う

左ペインはタブ内の分割なので、別タブや別 workspace へ移ると見えない。「常時見える化」という当初の要求はこれだけでは半分しか満たせない。そこでタブバー左端に `cla:1 cdx:2` 形式のサマリを出し、どこにいても総数だけは分かるようにする。左ペインは「作業中の workspace で詳細を見る」、ステータスバーは「どこにいても総数を把握する」で役割が違い、補完関係にある。

`workspaces.lua` が `update-right-status` + `set_right_status` で workspace 名を出しているが、こちらは `update-status` + `set_left_status` を使う。両イベントは併存して発火し、書き込むスロットも別なので `workspaces.lua` を一切変更せずに共存できる。`update-status` は毎秒発火するため全ペイン走査は 3 秒でスロットルし、キャッシュは `wezterm.GLOBAL` にスカラー 2 本（時刻と文字列）で持つ。`wezterm.GLOBAL` に入れたテーブルはネストした書き換えが保持されないことがあるためテーブルは持たせない。表示順は `pairs()` の走査順が不定なので固定の順序リストで決める。AI が 0 本のときは何も出さない。

### キーバインド

| キー          | 動作                                       | 衝突                                            |
| ------------- | ------------------------------------------ | ----------------------------------------------- |
| `CMD+a`       | AI ペインピッカー（fuzzy `InputSelector`） | なし。`CMD+s` の workspace セレクタと対の操作感 |
| `CMD+SHIFT+A` | 左ダッシュボードペインのトグル             | なし                                            |

`CTRL|SHIFT` 系は `macos_forward_to_ime_modifier_mask = "SHIFT|CTRL"` の対象なので避ける。

## Implementation steps

1. `dotfiles/wezterm/bin/ai-panes.sh` を作る（mode 755）。`wezterm cli list --format json` と `ps -Ao tty=,comm=` を jq で `tty_name` をキーに join し、workspace ごとにグルーピングして描画する。自分のペイン（`$WEZTERM_PANE`）は除外し、自分と同じ workspace の行は `●`(green)、他は `○`(overlay0) で描き分ける。alternate screen（`ESC[?1049h`）を使い、再描画は `ESC[H` + `ESC[0J`（`ESC[2J` はちらつく）。INT / TERM トラップは alternate screen を戻して `exit 0` する。
2. `dotfiles/wezterm/ai-panes.lua` を作る。`mux.all_windows()` → `win:tabs()` → `tab:panes()` を走査し `pane:get_foreground_process_name()` で AI CLI を判定する。`mux.get_pane()` が無いので `MuxPane` オブジェクトそのものを行に保持し、ピッカーで選ばれたら `row.pane:activate()` → 必要なら `SwitchToWorkspace` の順で飛ぶ（逆順だと切替完了前に activate してレースする）。トグルは user var でダッシュボードペインを探し、あれば `CloseCurrentPane`、無ければ `SplitPane`（`direction = "Left"`, `size = { Percent = 18 }`）してから元のペインへフォーカスを戻す。あわせて `update-status` に `set_left_status` のサマリを登録する。
3. `dotfiles/wezterm/wezterm.lua` に `require("ai-panes")(config)` を `keybindings` より後に追加し、順序依存をコメントで明示する。

## File changes

| File                               | Change                                                                              |
| ---------------------------------- | ----------------------------------------------------------------------------------- |
| `dotfiles/wezterm/ai-panes.lua`    | 新規。AI ペインの走査、`CMD+a` ピッカー、`CMD+SHIFT+A` トグル、左ステータスのサマリ |
| `dotfiles/wezterm/bin/ai-panes.sh` | 新規（mode 755）。描画ループ本体。`--once` / `--json` サブモード                    |
| `dotfiles/wezterm/wezterm.lua`     | `require("ai-panes")(config)` を追加し、require 順序の理由をコメントで明示          |

## Risks and mitigations

| Risk                                                                                                         | Mitigation                                                                                                                                                                                                                         |
| ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pane:activate()` → `SwitchToWorkspace` の順でフォーカスが狙ったペインに乗らない                             | API の存在は確認済みなので残るのは挙動の問題。駄目なら逆順、それでも駄目なら `wezterm cli activate-pane` + `mux.set_active_workspace()`                                                                                            |
| ~~`CloseCurrentPane` で任意のペインを閉じる~~ **実機で発生**。渡したペインではなくアクティブなペインが閉じた | `existing:send_text("\003")` に変更。スクリプトはペイン直下のプロセスなので INT トラップ → `exit 0` → WezTerm がそのペインだけを閉じる。応答不能になった場合はトグル off が効かないが `CMD+w` で閉じられる                         |
| `require` 順を誤ると `config.keys` が nil で落ちる                                                           | `wezterm.lua` にコメントで明示。リロード直後にキーが効くか毎回確認する                                                                                                                                                             |
| ダッシュボードが「常時」見えるのはそのタブの中だけ                                                           | タブバー左端のサマリ（`update-status` + `set_left_status`）で補完済み                                                                                                                                                              |
| `get_foreground_process_name()` は Lua 側にしか無く、シェル側は tty 経由の `ps`                              | 検出ロジックが 2 系統になるのは意図的（Lua から `wezterm cli` を呼ぶとデッドロックし得る）。AI 名リストは両方にコメントで相互参照                                                                                                  |
| `copilot` / `cursor-agent` の実プロセス名が未確認                                                            | `claude` / `codex` は実測済み。残り 2 つは実際に起動して `ps -Ao tty=,comm=`（シェル側）と `get_foreground_process_info()` の `name` / `argv[0]` / `executable`（Lua 側）の両方を確認する。node 等のラッパー経由だと別名になりうる |
| CLI から検証すると Lua の起動環境を再現できず、PATH 依存のバグを見逃す                                       | 検証は `wezterm cli split-pane -- /usr/bin/env -i HOME=... PATH=/usr/bin:/bin:/usr/sbin:/sbin <script>` のように**環境を落として**行う。素の `split-pane` はクライアント環境を渡すので通ってしまう                                 |
| 2 秒ごとの `wezterm cli list` + `ps -A` が重い                                                               | git 呼び出しを排したので 1 リフレッシュあたり 2 プロセス。`AI_PANES_INTERVAL` で間隔を変えられる                                                                                                                                   |
| cwd の URL がパーセントエンコードされている                                                                  | basename の表示のみなので実害は小さい。パスに空白を含むリポジトリを扱うようになったらデコードを足す                                                                                                                                |

## Validation

自動テストの足場が無いため、CLI で確認できるものは実機で確認し、キー押下が要るものはチェックリストとして残す。

コマンドで確認済み:

- [x] `shellcheck` / `shfmt -d` / `stylua --check` が通る（`mise run lint:shell`, `mise run lint:stylua`）
- [x] `docs/plan/` の Markdown が `markdownlint-cli2` と `prettier --check` を通る
- [x] `wezterm show-keys` が Lua エラーなく設定を読み込み、`SUPER a` / `SUPER A` が登録されている
- [x] 既存キー（`SUPER s`, `SUPER S`, `SUPER n`, `SUPER R`, `CTRL W`, `SHIFT Enter`, `CTRL [`）が `show-keys` に残っている
- [x] `ai-panes.sh --json` が有効な JSON を返し、AI が 1 本も無いときは `[]` になる
- [x] `ai-panes.sh --once` が 1 フレーム描画して終了する
- [x] `WEZTERM_PANE` に自分のペインを指定すると、その行が結果から除外される
- [x] 自分と同じ workspace の行だけが `●`、他 workspace が `○` になる
- [x] cwd の basename が workspace 名と食い違うペインを正しく取り出せる
- [x] `wezterm cli split-pane --left --percent 18` で実際に立てたペイン（37 桁）の中で描画が折り返さない
- [x] 実際に走っている CLI が、そのペインの workspace 配下にグルーピングされて出る
- [x] Ctrl-C でペインが残骸を残さず閉じ、**そのペインだけ**が閉じる（トグル off の実体）
- [x] `env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin` の実ペイン（= Lua が作る環境と同じ）で一覧が正しく描画される
- [x] `wezterm` / `jq` / `ps` / `awk` が PATH に無いとき、`no AI running` ではなく `not on PATH: ...` と名指しで出る
- [x] 制御端末が無い文脈で実行しても `/dev/tty` の診断メッセージが漏れない
- [x] `collect()` / `status_text()` / `agent_of()` / `select_ai_pane()` の純ロジックを luajit + スタブでユニットテスト（30 ケース。AI 以外のペインの除外、workspace→pane_id 順、Url オブジェクトと `file://` 文字列の両対応、例外を投げるペインの握り潰し、symlink 経由で実パスがバージョン番号になる claude の検知、`AI_ORDER` による表示順の固定、スロットルのキャッシュ返却と明けの再計算、0 件時のプレースホルダ、選択時の `activate()` → `SwitchToWorkspace` の順序と同一 workspace での省略）
- [x] スタブの前提を実機で裏取り（`wezterm.log_info` で `get_foreground_process_info()` の実値を GUI ログへ 1 回だけ出して確認し、確認後に除去）。当初スタブは `name="claude"` と推測していたが実際は `2.1.235` で、**推測のままなら誤った前提のテストが緑になっていた**
- [x] 設定リロード後に WezTerm の GUI ログへ Lua エラーが 1 件も出ない（`update-status` は毎秒発火するので、ハンドラが壊れていれば必ずログに出る）

キー押下が要るので手で確認する:

- [ ] `CMD+SHIFT+A` で左に細いペインが開き、フォーカスは元のペインに残る
- [ ] もう一度 `CMD+SHIFT+A` で**ダッシュボードだけ**が閉じる（作業中のペインが巻き添えにならないこと）
- [ ] ダッシュボードを `CMD+w` で手動で閉じてから `CMD+SHIFT+A` を押すと正しく開き直す
- [ ] 設定リロード後も `CMD+SHIFT+A` が既存のダッシュボードを検出して閉じられる（user var による識別）
- [ ] 別ペインで `claude` を起動すると 2 秒以内に一覧へ現れ、終了すると消える
- [ ] `CMD+a` でピッカーが開き候補が出る
- [ ] AI が 1 本も動いていないときに `CMD+a` を押すと `no AI panes` の 1 行が出て、選んでも何も壊れない
- [ ] `CMD+a` でピッカーが開き、**同一 workspace** の別タブの AI を選ぶとそのタブ・ペインがアクティブになる
- [ ] `CMD+a` で**別 workspace** の AI を選ぶと workspace が切り替わり、かつ目的のタブ・ペインがアクティブになっている
- [ ] タブバー左端に `cla:1 cdx:2` 形式のサマリが出る（AI が 0 本なら何も出ない）
- [ ] 右ステータスバーの workspace 名表示が消えていない（左右のサマリが**同時に**出ていること）
- [ ] AI を 1 本起動 / 終了させて 3〜4 秒以内にサマリのカウントが追随する

## Open questions

- 状態表示（working / waiting）を Phase 2 でやるか。やるなら Claude Code の hook（`ai-agents/settings/claude/hooks/`）が `$WEZTERM_PANE` をキーに状態ファイルを書き、スクリプトがそれを読む方式が本命。codex / copilot には同等の hook が無いので claude だけ状態が出る非対称になる。
- 常設化（`workspace_defs` に `ai_pane` を足して `gui-startup` で自動配置）はいつやるか。数日使ってどのタブで開くことが多いかを見てから決める。
- ペインのクリックでジャンプは WezTerm の Lua から取れない。`CMD+a` ピッカーで代替する前提でよいか。
- `.claude/rules/lua-nvim.md` の `paths` が `nvim/**/*.lua` 限定で、`dotfiles/wezterm/**/*.lua` を触っても Lua ルールが自動ロードされない。別途 `paths` を広げるか。
