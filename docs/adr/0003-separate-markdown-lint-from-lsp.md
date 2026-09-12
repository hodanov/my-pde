# ADR-0003: Separate Markdown linting from Markdown LSP

- Status: Accepted (2026-09-12)

## Context

`nvim/config/lua/lsp/remark.lua` が、形だけ整っていて無効な状態で残っていた。

Neovim 0.11 以降の `lsp/<name>.lua` 形式（`cmd` / `filetypes` / `root_markers` を返す）に見えるが、置き場所が runtimepath 直下の `lsp/` ではなく `lua/lsp/` なので自動探索の対象外になる。`nvim/config/lua/lsp/init.lua` は `vim.lsp.enable` を明示列挙する方式で、`remark` はその列挙に無い。設定を読んで「Markdown にも LSP がある」と判断すると外れる。中身も `on_attach` と `capabilities` を自前で組む 0.10 以前の書き方で、現行の `vim.lsp.config` 方式と食い違っていた。

削除するにあたって「そもそも remark を有効化すべきではないか」という問いが出た。Markdown の指摘は `nvim/config/lua/nvim_lint.lua` の markdownlint-cli2 と `nvim/config/lua/textlint_nvim.lua` の textlint が担っているが、どちらも linter であって、`<leader>fs`（`telescope_config.lua` の `lsp_document_symbols`）のような LSP の動線は Markdown で使えない。

## Options considered

| 案                                            | 評価                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `remark.lua` を現行方式に書き直して有効化する | remark-language-server は `unified-language-server` のラッパで、実装しているのは diagnostics（didOpen / didChange / didClose）、codeAction、documentFormatting、didChangeWatchedFiles のみ。documentSymbol・hover・definition・references・completion は持たない。つまり LSP の動線は埋まらず、markdownlint-cli2 と同じカテゴリの linter が 2 つ並ぶ。ルールセットも `.markdownlint-cli2.yaml` と `.textlintrc.json` に続く 3 つ目を `.remarkrc` で抱えることになる |
| `remark.lua` を削除し、lint は現状維持        | 無効な設定による誤読が消える。Markdown に LSP が無い状態は変わらないが、markdownlint-cli2 は conform 経由で保存時に `--fix` が走るため自動修正は既にある                                                                                                                                                                                                                                                                                                            |
| 削除したうえで marksman を導入する            | marksman は documentSymbol・definition・references・rename・completion・hover を持つ本物の Markdown LSP で、linter と役割が重ならない。ただし self-contained binary なので `environment/tools/node/package.json` ではなく `nvim.dockerfile` 側の導入になり、`mise.toml` の `[env]` と `pins:sync` にも乗せる必要がある                                                                                                                                              |

## Decision

`nvim/config/lua/lsp/remark.lua` を削除し、remark は採用しない。代替の追加もしない。

決め手は、remark-language-server が LSP プロトコルに包まれた linter でしかないこと。問いの出発点は「linter では足りない」だったが、remark はその不足を埋めない。埋まらないまま重複だけが増える。

ここから一般則を引く。**Markdown のツールは lint（品質の指摘）と navigation（構造の走査）を別物として扱い、どちらを足すのかを決めてから選ぶ。** remark は前者の席に後者の顔で座ろうとしていた。

navigation が必要になったときの候補は marksman だが、導入コストが Dockerfile と pins に及ぶため別途判断する。

## Consequences

Markdown に LSP の動線は当面無い。`<leader>fs` は Markdown で効かないままになる。見出し一覧だけであれば、`markdown` / `markdown_inline` の treesitter parser が `nvim/config/lua/nvim_treesitter.lua` で既に入っているので、`builtin.treesitter` を 1 行マップすればコストゼロで得られる。定義ジャンプやリネーム追従が要るかどうかが marksman を入れる分かれ目になる。

今後 Markdown 関連のツールを検討するときは、まず lint と navigation のどちらの席を埋めるのかを判定する。lint 側は markdownlint-cli2 と textlint で埋まっており、3 つ目のルールセットは増やさない。

`lua/lsp/` に置かれた LSP 設定は自動探索されない。今後 LSP を足すときは `nvim/config/lsp/<name>.lua` に置くか、`nvim/config/lua/lsp/init.lua` の `vim.lsp.enable` に明示列挙する。形だけ整った無効な設定を残さない。

関連: [#788](https://github.com/hodanov/my-pde/issues/788)
