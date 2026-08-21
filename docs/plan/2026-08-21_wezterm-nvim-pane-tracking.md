# Plan: ai-panes に nvim ペインの追跡を統合する

`dotfiles/wezterm/ai-panes.lua` と `bin/ai-panes.sh` の検知対象を「AI CLI」から「AI CLI + nvim」へ広げ、
`CMD+a` のピッカー・`CMD+SHIFT+A` のダッシュボード・ステータスバーのすべてで nvim の所在が分かるようにする。
ファイル名・キーバインド・デプロイ経路は現状のまま。

## Context

- 複数 workspace（blog / my-pde / new-project / stable-diffusion）でそれぞれ nvim を開いており、
  「いまどの workspace で nvim が起動しているか」「その nvim のペインへ飛びたい」が今は分からない。
- AI CLI についてはこの問題を `ai-panes.lua` で既に解いてある（走査 → ピッカー → ジャンプ → ステータスバー）。
  nvim も「ワークスペース横断でプロセスの所在を把握して飛ぶ」という**まったく同じ形**なので、
  新モジュールを作らず既存の仕組みに検知対象として足す。分離すると `collect` / `jump_to` /
  `update-status` が二重化し、`set_left_status` の書き込みが競合する。
- 決定事項（確認済み）: ①ai-panes に統合、ファイル名とキーは据え置き ②ステータスバーは件数だけ
  （`nvim:2 claude:1`）③タブバーへの印付けはやらない（`appearance.lua` は触らない）。

### 検知が素直にいかない理由（実測）

`dotfiles/.zshrc:71` の `nvim()` は関数で、実体は docker 越しに起動される。

```text
ttys005  docker  docker container exec -it -w /Users/hodanov/workspace/my-pde nvim-dev bash --login -c nvim "$@"
```

- ホスト側の foreground プロセス名は `docker` であり、`nvim` ではない。名前一致だけでは絶対に拾えない。
- ペインの cwd は OSC 7 でシェルが報告した値がそのまま残るので、`-w` に渡ったホスト側のパス
  （= プロジェクトディレクトリ）が `pane:get_current_working_dir()` から取れる。project 名の解決は既存のままで良い。
- `get_foreground_process_info()` の `argv` は前回実装時に実測済み（`argv[1]=claude` で判定できている）。
  よって argv を最後まで見れば docker 越しの nvim も判定できる。

## Design policy

### 判定は「argv のトークンに素の `nvim` が現れるか」

`nvim-dev`（コンテナ名）で判定しない。`docker container exec -it nvim-dev bash --login` は
「コンテナに入っただけ」でエディタは起動しておらず、これを nvim として並べると嘘になる。
トークン単位で見れば実 nvim 起動だけを拾える。

| 実際のコマンドライン                                     | 判定      |
| -------------------------------------------------------- | --------- |
| `... nvim-dev bash --login -c 'nvim "$@"'`               | nvim ✓    |
| `docker container exec -it nvim-dev bash --login`        | 非 nvim ✓ |
| ホストの `/opt/homebrew/bin/nvim`（直接起動）            | nvim ✓    |
| `docker compose -f .../nvim.dockerfile ...` のような紛れ | 非 nvim ✓ |

- Lua: `token == "nvim" or token:match("^nvim%s")`（`bash -c` の引数が `nvim "$@"` という 1 トークンになるため）。
- シェル: `ps` の command 文字列に対する `(^| )nvim( |$)`。
- Lua 側は「foreground が `docker` のときだけ argv を舐める」に限定して誤検知の面を狭める。

### 内部の識別子だけ「AI 専用」から改名する

ファイル名・キー・user var（`ai_panes_dashboard`）・環境変数（`AI_PANES_*`）は据え置き（デプロイと
既存ペインの互換のため）。一方でコード内の `AI` / `agent_of` / `row.agent` は nvim を含むと嘘になるので、
`TRACKED` / `process_of` / `row.proc` に改める。`--json` の出力キーも `agent` → `proc`。
このキーの外部消費者はリポジトリ内に存在しない（grep 確認済み）。

### 表示順は「nvim → AI」に固定する

`TRACKED_ORDER` の先頭に `nvim` を置き、ピッカーの並びとステータスバーの並びの両方でこの順を使う
（ピッカーのソートキーは workspace → 種別順 → pane_id）。ダッシュボードも同じ順になるよう、
jq の `sort_by` に種別インデックスを挟む。nvim は「その workspace の主ペイン」なので各グループの先頭が自然。

## Implementation steps

1. **`dotfiles/wezterm/ai-panes.lua`**

   - `AI` → `TRACKED` にリネームし `nvim = "#89b4fa"`（Catppuccin Mocha blue、`cursor-agent` の緑と衝突しない）を追加。
     `AI_ORDER` → `TRACKED_ORDER = { "nvim", "claude", "codex", "cursor-agent", "copilot" }`。
   - `agent_of()` → `process_of()`。既存の candidates ループ（`info.name` / `argv[1]` / `executable` /
     `get_foreground_process_name()`）はそのまま残し、その後ろに docker ラッパーの分岐を足す:

     ```lua
     -- .zshrc の nvim() は docker exec でコンテナ内の nvim を起動するので、ホスト側の
     -- foreground は docker になる。コンテナ名(nvim-dev)ではなく argv のトークンに素の
     -- nvim が現れるかで見る。コンテナに入っただけのシェルを nvim と呼ばないため。
     if basename(info.name) == "docker" and info.argv then
       for _, token in ipairs(info.argv) do
         if token == "nvim" or token:match("^nvim%s") then
           return "nvim"
         end
       end
     end
     ```

   - `collect()` の `row.agent` → `row.proc`。`table.sort` の第 2 キーに `TRACKED_ORDER` の
     インデックス（モジュール先頭で `name → index` の逆引きテーブルを 1 つ作る）を挟む。
   - ピッカーのラベル: タブ名が種別と同じとき（`my-pde / nvim / nvim` になるケース）は
     タブ名を出さない。既存の「空タイトルは出さない」分岐に条件を 1 つ足すだけ。
   - `status_text()` は `TRACKED_ORDER` を回すだけなので、順序リストの変更で自動的に `nvim:2 claude:1` になる。
     全ペイン走査は 1 回のままでコスト増なし。

2. **`dotfiles/wezterm/bin/ai-panes.sh`**

   - `agents="${AI_PANES_AGENTS:-...}"` → `targets="${AI_PANES_AGENTS:-nvim claude codex cursor-agent copilot}"`
     （環境変数名は互換のため据え置き）。`ai_by_tty()` → `procs_by_tty()`。
   - `ps -Ao tty=,comm=` → `ps -Ao tty=,command=`。awk 側で
     - 先頭で `$1 == "??"` の行（tty なし = Docker Desktop 等の巨大 argv）を捨てて出力量を抑える
     - `$2` の basename が `targets` にあれば従来どおり採用（ホスト直接起動の nvim もここで拾える）
     - basename が `docker` かつ `$0 ~ /(^| )nvim( |$)/` なら `nvim` として採用
   - `agent_color()` → `proc_color()` に nvim = blue（`38;2;137;180;250`）を追加。
   - jq: `agent` キー → `proc`、`sort_by(.ws, .pane)` → `--arg targets` を受けて
     `sort_by(.ws, ($targets | split(" ") | index(.proc)), .pane)`。
   - 見出し `AI` → `PANES`、空表示 `no AI running` → `nothing running`。
     ファイル冒頭のコメントも「AI CLI + nvim を横断表示する」に更新。

3. **`docs/plan/2026-08-21_wezterm-nvim-pane-tracking.md`** にこのプランを記録（リポジトリの慣習）。

`wezterm.lua` / `keybindings.lua` / `workspaces.lua` / `appearance.lua` / `nvim/` は変更しない。
デプロイは `mise run dotfiles-link` の symlink 済みディレクトリ配下なので追加作業なし。

## File changes

| File                               | Change                                                                    |
| ---------------------------------- | ------------------------------------------------------------------------- |
| `dotfiles/wezterm/ai-panes.lua`    | `TRACKED` に nvim 追加、docker ラッパー判定、種別順ソート、ラベル重複除去 |
| `dotfiles/wezterm/bin/ai-panes.sh` | `ps command=` ベースの検知、nvim 色、jq キー/ソート、見出し文言           |
| `docs/plan/2026-08-21_*.md`        | 新規（プラン記録）                                                        |

## Risks and mitigations

| Risk                                                                      | Mitigation                                                                                                                                                                                                                                     |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS の `get_foreground_process_info()` が docker のフル argv を返さない | 実装の**最初**にデバッグオーバーレイ（`CMD+SHIFT+L`）で全ペインの `argv` を dump して確認する。取れない場合の代替は nvim 側から OSC 1337 `SetUserVar` を出す方式（ダッシュボードペインと同じ手口）だが、コンテナへの設定配布が要るので第二候補 |
| ダッシュボードとステータスバーで検知経路が二重（`ps` と procinfo）        | 既存からの構造的制約（Lua から `wezterm cli` を呼ぶとデッドロックし得る）。判定ルールの文言を両ファイルのコメントで相互参照させ、ズレたときに気づけるようにする                                                                                |
| `ps -Ao command=` の出力量増                                              | tty なし行を awk の先頭で捨てる。残るのは端末に紐づく数十行                                                                                                                                                                                    |
| nvim をコンテナ内から別の起動経路（ラッパー変更）にすると壊れる           | 判定は「argv に素の nvim トークン」なので、docker でもホスト直でも ssh でも同じルールで通る。コンテナ名に依存しない                                                                                                                            |
| `--json` のキー改名で手元のワンライナーが壊れる                           | 消費者はリポジトリ内に無いことを grep 済み。`--json` は手動検証用                                                                                                                                                                              |

## Validation

コマンドで確認済み:

- [x] `mise run lint:shell` / `mise run lint:stylua` が通る。`docs/plan/` の Markdown が prettier と markdownlint-cli2 を通る
- [x] `wezterm show-keys` が Lua エラーなく設定を読み、`SUPER a` / `SUPER A` と既存キー（`SUPER s` / `SUPER S` / `SUPER n` / `SUPER R` / `CTRL [` / `SHIFT Enter`）が残っている
- [x] `bin/ai-panes.sh --json` が docker 越しの nvim を拾う（実測 `{"ws":"my-pde","pane":35,"proc":"nvim","project":"my-pde"}`）
- [x] `bin/ai-panes.sh --once` が nvim（blue）と claude を workspace ごとに描画して終了する
- [x] 並びが `$targets` の順に従う（`AI_PANES_AGENTS="claude ... nvim"` にすると claude が先頭に来る）
- [x] `AI_PANES_AGENTS="nvim"` でも docker 分岐が独立して効く
- [x] awk の判定ルールを合成 `ps` 行で検証。`-c 'nvim "$@"'` / `-c nvim` / ホスト直 nvim は拾い、`docker container exec -it nvim-dev bash --login`（コンテナに入るだけ）と `docker compose -f .../nvim.dockerfile` と tty なし行は拾わない
- [x] 環境を落としても動く（`env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin` で `--once` が正常描画）。GUI 起動時の PATH 欠落を再現するため
- [x] Lua 側は `wezterm` をスタブして luajit で実行し、ピッカーの行とステータス文字列を検証。docker 越し nvim / ホスト nvim / symlink 解決で name がバージョン番号になる claude を拾い、コンテナに入るだけの docker と `nvim.dockerfile` は拾わない。同一 workspace 内で pane_id によらず nvim が claude より先に並び、タブ名がプロセス名と同じ行はタブ名を出さない。ステータスは `nvim:2 claude:1`

キー押下が要る手動チェック（WezTerm は設定変更を自動リロードするので、そのまま押せば反映される）:

- [ ] `CMD+a` のピッカーに `<workspace> / nvim #<id>` が workspace ごとに並び、各グループの先頭に来る
- [ ] 別 workspace の nvim を選ぶと workspace 切り替え + そのペインがアクティブになる
- [ ] `CMD+SHIFT+A` のダッシュボードに nvim 行が workspace ごとに出る
- [ ] タブバー左端が `nvim:2 claude:1` のように出る（nvim が 0 本なら nvim は出ない）
