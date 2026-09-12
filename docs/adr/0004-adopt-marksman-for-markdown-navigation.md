# ADR-0004: Adopt marksman for Markdown navigation

- Status: Accepted (2026-09-12)

## Context

ADR-0003 で「Markdown のツールは lint（品質の指摘）と navigation（構造の走査）を別物として扱う」と定め、lint 側は markdownlint-cli2 と textlint で埋まっているとした一方、navigation の席は空けたまま「候補は marksman だが、導入コストが Dockerfile と pins に及ぶため別途判断する」と保留した。この ADR がその別途判断にあたる。

保留の実害は、`telescope_config.lua` の `<leader>fs` / `<leader>fS`（`lsp_document_symbols` / `lsp_dynamic_workspace_symbols`）が Markdown で無反応なこと。このリポジトリは `docs/plan/**` が 65 本、`docs/adr/` が増えつつあり、長い Markdown を跨いで見出しやリンクを辿る場面が実際にある。

導入にあたっての制約が 2 つある。marksman は .NET 製の self-contained binary なので、`terraform-ls` や `tflint` のように `environment/tools/go/go-tools.txt` 経由で `go install` する道が使えず、バイナリを取ってくる stage が要る。そして marksman はリリースに集約チェックサムを公開していない（アセットは裸のバイナリ 6 本のみで、hadolint の `checksums.sha256` や terraform の `SHA256SUMS` に相当するものが無い）。一方でこのリポジトリは、生バイナリを HTTP で取る stage（node / go / hadolint / terraform / lua-language-server）で例外なく SHA256 を検証している。

## Options considered

| 案                                           | 評価                                                                                                                                                                                                                                                                                                                         |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 導入しない                                   | ADR-0003 の状態を維持する。Markdown に navigation は無いまま。イメージサイズも運用も増えないが、`<leader>fs` が効かない状態が残る                                                                                                                                                                                            |
| treesitter で済ませる                        | `markdown` / `markdown_inline` の parser は `nvim_treesitter.lua` に既にあるので、`builtin.treesitter` を 1 行マップすれば見出し一覧はコストゼロで得られる。ただし得られるのは現在のバッファの構造だけで、リンク先へのジャンプ、参照の逆引き、見出しリネーム時のリンク追従は得られない                                       |
| marksman を `[tools]` に入れて自動 bump      | mise の registry に marksman がある（`aqua:artempyanykh/marksman`）ため、`mise upgrade --bump marksman` に 1 語足すだけで週次 bump に乗る。ただしイメージ側は mise を使わず各 stage が自分で `curl` する構成なので、mise 経由の checksum 検証はビルドに効かない。結果として、生バイナリを検証せずに入れる唯一の stage になる |
| marksman を lua-language-server と同形で導入 | `[env]` に `MARKSMAN_VERSION`、SHA256 は arch ごとに ARG 直書き、自動 bump の対象外。検証の規律を保てるが、bump のたびに version と SHA256×2 を手で更新する                                                                                                                                                                  |

## Decision

marksman を導入し、バージョン管理は lua-language-server と同形にする。`mise.toml` の `[env]` に `MARKSMAN_VERSION` を置き、SHA256 は arch ごとに `nvim.dockerfile` の ARG へ直書きし、`automation-tools-bump.yml` の自動 bump には乗せない。

決め手は、上流にチェックサムが無いツールの前例が lua-language-server ただ 1 つで、そこで既に「per-asset の sha256 を ARG に固定し、bump 時は両方更新する」という答えが出ていること。marksman だけ検証を外すと、「生バイナリを HTTP で取る stage は必ず SHA256 を検証する」という規律に例外が 1 つできる。規律に例外を作るより、既にある手動更新の形をもう 1 件ぶん引き受けるほうが安い。marksman のタグは日付形式（`2026-02-08`）でリリースは年 5 回程度なので、この頻度なら手で回る。

treesitter で済ませないのは、欲しかったものが見出し一覧ではなく navigation そのものだから。ADR-0003 で分けたのは「現在のバッファをどう表示するか」ではなく「文書間の参照を辿れるか」で、definition・references・rename の追従は treesitter では代替できない。

stage の形は lua-language-server ではなく **hadolint に倣う**。lua-language-server は tarball を展開してディレクトリごと持つため `/opt/lua-language-server/` と `ENV PATH` への追加が要るが、marksman は単一バイナリなので `install -m 0755 ... /usr/local/bin/marksman` で済み、`/usr/local/bin` は既に PATH にある。「SHA256 の扱いは lua_ls と同形、stage の形は hadolint と同形」を意図的に混ぜている。

有効化は `nvim/config/lua/lsp/init.lua` の `vim.lsp.enable("marksman")` で行う。ADR-0003 の Consequences は `nvim/config/lsp/<name>.lua` に置く道も許しているが、最終 stage の `COPY` が `init.lua` と `lua/` しか拾わないため、`nvim/config/lsp/` を作るとイメージに入らない。既存 11 個の LSP もすべて列挙方式で揃っている。nvim-lspconfig が `lsp/marksman.lua` を提供しているので `vim.lsp.config` による上書きは不要。

## Consequences

**bump は手動になる。** 手順は lua-language-server と同じで、`mise set MARKSMAN_VERSION=<新バージョン>` → 新しいバイナリの SHA256 を 2 つ計算して ARG を書き換え → `mise run pins:sync`。`sync-pins.sh` は SHA256 を扱わないので、バージョンだけ上げて SHA256 を忘れるとビルドが checksum mismatch で落ちる。落ちる方向に倒れるので黙って壊れることはない。

**上流にチェックサムが無いツールが 3 件目になったら、この方針を見直す。** 2 件までは手動更新の重複として許容するが、3 件目は `sync-pins.sh` 側で SHA256 を扱う判断に切り替える。その際は `pins:check` が CI で毎回 `pins:sync` を走らせる構造上、PR ごとにバイナリをダウンロードするコストが乗ることを勘案する。

**イメージが arch あたり約 22MB 増える。** 単一バイナリの `COPY` なので層は 1 つ。

**`ci-docker-build.yml` はこの変更を自動で検証しない。** job の `if` が `dependencies` ラベル / dependabot / `chore/bump-tool-versions` ブランチに限定されているため、Dockerfile を触る通常の PR ではビルドが走らない。marksman の bump や Dockerfile の変更を出すときは `dependencies` ラベルを付けて発火させる。なお `runs-on: ubuntu-latest` なので CI が検証するのは amd64 のみで、arm64 はローカルビルドで担保する。

**`<leader>fs` / `<leader>fS` が Markdown で効くようになる。** キーマップの追加は不要で、既存の LSP 動線がそのまま繋がる。`builtin.treesitter` のマップは追加しない（marksman が documentSymbol を返すため不要）。

関連: [#788](https://github.com/hodanov/my-pde/issues/788)、[ADR-0003](0003-separate-markdown-lint-from-lsp.md)
