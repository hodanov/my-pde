# ADR-0012: Pin Neovim plugins with lazy-lock.json and smoke-test the image

- Status: Accepted (2026-10-19, recorded 2026-11-22)

## Context

コンテナ内 Neovim のプラグインは、`lazy-lock.json` をリポジトリに持たず、イメージをビルドするたびに最新を入れていた。固定する提案は 3 回見送っている。

- #722（2026-08-30、rejected）: `vim.pack` への移行を検討していて、`lazy-lock.json` は lazy.nvim 専用の形式だから。
- #707（2026-10-03、not planned）と PR #857（同日、マージせずに Close）: 「破壊的変更に遭った記憶がない。壊れたら都度直す」が理由だった。再検討の条件は「実際に破壊的変更で困ったとき」とした。

その条件を満たす事例が PR #904 で 2 件起きた。

- blink.cmp が upstream main の v2 化で壊れた。
- nvim-treesitter の古いロックがイメージに残った。

どちらもコンテナを作り直すまで気付けなかった。すり抜けた理由は 3 つある。

- ロックが無いので、ビルドのたびに最新のプラグインが入る。
- treesitter のビルド失敗や lazy の config 失敗は通知に出るだけで、headless の nvim は exit 0 で終わる。
- `ci-docker-build.yml` の対象パスに `nvim/` が無く、HEALTHCHECK も起動直後の `v:errmsg` しか見ていない。

## Options considered

| 案                                                    | 評価                                                                                                             |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| 固定しない（従来どおり）                              | 再検討の条件を満たした。壊れたことに、作り直すまで気付けない                                                     |
| `lazy-lock.json` で固定し、ビルド時に `Lazy! restore` | 版が再現できる。ただし固定しただけでは、config の失敗が exit 0 ですり抜けるのは変わらない                        |
| 固定に加えて、ビルド時にスモークテストを流す          | ロックとの一致、ネイティブバイナリの有無、全プラグインをロードしたときのエラーを、ビルドの失敗として捕まえられる |

## Decision

`nvim/config/lazy-lock.json` で固定し、イメージのビルド時にスモークテストを流す（74dffb9b / PR #905）。

- Dockerfile は `Lazy! sync` ではなく `Lazy! restore` を使う。`sync` は update を含むので、ロックを無視して最新へ上げてしまう。
- `nvim/tests/smoke_test.lua` を、ビルド中に bind mount して実行する。全プラグインの HEAD がロックと一致すること、blink.cmp の Rust 製マッチャーがあること、全プラグインをロードしても ERROR の通知と `v:errmsg` が出ないことを確かめる。

## Consequences

「Neovim プラグインは固定しない」という方針（#707 / #722 / #857）は撤回する。スキャンの提案を「固定しない方針だから」という理由で却下しない。

プラグインは、ロックを更新しない限り上がらない。ロックを定期的に上げる仕組みは 74dffb9b に含めていない。必要になったら、同じスモークテストを門番にして別途決める。

mise.toml、Dockerfile の ARG、npm / pip / Go、Actions の pin は、この ADR の前から厳格に固定しており、変わらない。
