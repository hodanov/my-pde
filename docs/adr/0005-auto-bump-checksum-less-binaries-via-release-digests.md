# ADR-0005: Auto-bump checksum-less release binaries via GitHub asset digests

- Status: Accepted (2026-09-12)

## Context

lua-language-server と marksman は、上流がリリースに集約チェックサム（hadolint の `checksums.sha256` や terraform の `SHA256SUMS` に相当するもの）を公開していない。そのため arch ごとの SHA256 を `nvim.dockerfile` の ARG に直書きしている。バージョンは `mise.toml` の `[env]` にあって `pins:sync` で ARG に書き写されるが、SHA256 は `sync-pins.sh` の対象外で、`automation-tools-bump.yml` も両者を bump しない。結果として、この 2 つだけ bump のたびに version と SHA256×2 を手で更新している（[ADR-0004](0004-adopt-marksman-for-markdown-navigation.md) の Decision）。手で回す以上、上流が新しいリリースを出しても誰かが気付くまでピンは古いまま残る。

ADR-0004 は「上流にチェックサムが無いツールが 3 件目になったら見直す」とし、見直す際の懸念として、`pins:check` が PR ごとにバイナリをダウンロードするコストを挙げていた。

GitHub の release API は asset ごとに `digest`（`sha256:<hex>`）を返す。現行ピン（lua-language-server 3.19.1 / marksman 2026-02-08）の 4 asset の `digest` は、ARG に直書きしてある SHA256 と一致する。

## Options considered

| 案                                                               | 評価                                                                                                                                                                                                                                                         |
| ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 手動 bump を続ける（ADR-0004 のまま）                            | 検証の規律は保てるが、2 ツールぶんの version + SHA256×2 の手更新が残り、週次 bump の外に置かれ続ける                                                                                                                                                         |
| `sync-pins.sh` でバイナリを DL して SHA256 を計算する            | 正本に SHA256 を書かずに済む。ただし `pins:check` が CI で毎回 `pins:sync` を走らせるので、`mise.toml` や Dockerfile を触る PR のたびに数十 MB（marksman だけで arch あたり約 22MB）を DL することになる。ネットワーク障害で同期チェックが落ちるようにもなる |
| bump ワークフローでバイナリを DL して `sha256sum` する           | `digest` API に依存しないのが利点。しかし得られる値は `digest` と同じで、代わりに週 1 回の数十 MB の DL と一時ファイルの後始末を抱える。`digest` が欠けても fail-fast で気付けるので、DL を常用するほどの差は無い                                            |
| `[tools]`（aqua backend）に移して `mise upgrade --bump` に乗せる | 週次 bump には乗る。しかしイメージ側は各 stage が自分で `curl` する構成なので、mise の checksum 検証がビルドに効かない。ADR-0004 でこの案を却下した理由がそのまま残る                                                                                        |

## Decision

per-asset の SHA256 も `mise.toml` の `[env]` を正本にし（`LUA_LS_SHA256_AMD64` / `LUA_LS_SHA256_ARM64` / `MARKSMAN_SHA256_AMD64` / `MARKSMAN_SHA256_ARM64`）、`pins:sync` で Dockerfile の ARG に書き写す。`automation-tools-bump.yml` は両ツールの `releases/latest` を取得し、`tag_name` を version として、Dockerfile と同じ規則で組み立てた asset 名の `digest` を SHA256 として `mise set` する。

決め手は、SHA256 の解決を bump 時の 1 回に閉じ込められること。正本に値を書いておけば `pins:sync` / `pins:check` はテキストのコピーのままで、ADR-0004 が懸念した PR ごとの DL は発生しない。値の出どころは手動 bump のときと同じ GitHub release なので、信頼の根拠も変わらない。

3 件目を待たずに 2 件目で見直すのは、ADR-0004 が 3 件目まで待つとした理由（PR ごとの DL コスト）が、この方式では生じないため。

marksman の導入そのもの（ADR-0004 の他の決定）は変えない。

## Consequences

**lua-language-server と marksman が週次 bump に乗る。** bump PR は `automation-deps-auto-merge.yml` で auto-merge の対象になる。

**CI が実ビルドで検証するのは amd64 の SHA256 だけ。** `ci-docker-build.yml` は `ubuntu-latest` で走るため、arm64 の値はビルドで確かめられない。amd64 と同じ API 応答から取っているので取り違えは起きにくいが、arm64 の不一致はローカルビルドで初めて表面化する。表面化の仕方は checksum mismatch でビルドが落ちる方向なので、黙って壊れることは無い。

**`digest` が欠けた release では bump 全体が止まる。** asset 名の規則が上流で変わった場合も同様。resolve step の検証で落ちるので壊れた値の PR は作られないが、go / node など他ツールの bump も巻き添えで止まる。

**信頼モデルは TOFU のまま。** 上流がチェックサムを別経路で公開していない以上、bump 時点の release asset を信じる点は手動 bump と変わらない。

**Dockerfile の SHA256 ARG は生成物になる。** `guard-version-pins.sh` のブロック対象に `*_SHA256_AMD64` / `*_SHA256_ARM64` を加える。手で更新する場合も `mise set <KEY>=<value>` → `mise run pins:sync` を通す。

関連: [ADR-0004](0004-adopt-marksman-for-markdown-navigation.md)
