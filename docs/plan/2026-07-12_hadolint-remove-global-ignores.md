# Plan: .hadolint.yml のグローバル ignore を全撤廃し nvim.dockerfile を堅牢化

## Background

`.hadolint.yml` で 6 ルール（DL3003 / DL3008 / DL3062 / DL4001 / SC1091 / SC2086）を
グローバルに ignore していた。グローバル ignore はスコープが広すぎて、新しく追加する
RUN 命令の問題も握りつぶしてしまう。ignore を撤廃し、hadolint デフォルトルールに
耐える Dockerfile にすることで、リンタを「常時効いている状態」に戻すのが目的。

ignore なしで hadolint 2.14.0 を実行して実態を確認した結果:

- 実際に発火するのは DL3003（4箇所）/ DL3008（6箇所）/ DL3062（1箇所）のみ。
- DL4001 / SC1091 / SC2086 は現行の Dockerfile では発火しない stale な ignore。
- 対象 Dockerfile はリポジトリ内で `environment/docker/nvim.dockerfile` の 1 つだけ。

## Current structure

- `.hadolint.yml` — リポジトリルートのグローバル ignore 設定（撤廃対象）。
- `environment/docker/nvim.dockerfile` — マルチステージビルド。lint 対象はこれのみ。
- lint 経路: `mise run lint:docker`（mise.toml）、CI は `.github/workflows/lint_dockerfile.yml`。
- `ARG *_VERSION=` 行と `environment/tools/go/go-tools.txt` は `mise run pins:sync` の生成物で、
  `guard-version-pins.sh` フックにより手動編集がブロックされる。

## Design policy

- DL3003 / DL3062 は構造修正で解消し、ignore 自体を不要にする。
- DL3008（apt パッケージのバージョンピン）は `pkg=version` ピンにしない。Ubuntu は
  セキュリティ更新で旧バージョンをアーカイブから削除するため、ピンはビルドを
  非決定的に壊し、かえって堅牢性を下げる。代わりに理由コメント付きの行単位
  inline ignore でスコープを最小化する。
- チェックサム検証は go-builder / hadolint-builder で使っている手動ハッシュ比較
  パターンに統一する。

## Implementation steps

1. `.hadolint.yml` を削除（全 ignore 撤廃で中身が空になるため）。
2. DL3003（`cd` 禁止）を 4 箇所で構造解消:
   - nvim-builder: `git clone` + `cd` + `git checkout` を
     `git clone --depth 1 --branch "v$NEOVIM_VERSION"` + `make -C /neovim` に変更。
     shallow clone でネットワーク面も堅牢化。
   - node-builder: `cd /tmp` を `WORKDIR /tmp/node` に置き換え、パス参照を相対化。
   - npm-tools: `cd /opt/npm-tools && npm install` を `WORKDIR` + `RUN npm install` に分離。
   - terraform-builder: サブシェル `(cd "$TMPDIR" && ... | sha256sum -c -)` を
     手動ハッシュ比較（`grep` + `awk` で期待値抽出 → `sha256sum` 実測値と比較）に統一。
3. DL3062（`go install "$pkg"` のピン警告）: `go-tools.txt` は全行ピン済みだが
   hadolint は変数の中身を検査できない構造的 false positive。while ループに
   「`@` を含まない行が来たら fail」する実行時ガードを追加した上で行単位 inline ignore。
4. DL3008: 6 箇所の apt-get RUN に行単位 `# hadolint ignore=DL3008`。
   理由コメントは base ステージの 1 箇所に集約。

## File changes

- `.hadolint.yml` — 削除。
- `environment/docker/nvim.dockerfile` — 上記の構造修正と inline ignore。
- `.github/workflows/lint_dockerfile.yml` — 変更なし（`paths` の `.hadolint.yml` は
  将来再追加された場合のトリガーとして残置）。

## Risks and mitigations

- `pins:sync` は Dockerfile の `ARG` 行を sed パターンで書き換えるため、編集が
  パターンを壊すと生成フローが破綻する → 編集後に `pins:sync` を実行し、
  ファイルハッシュが変化しないこと（冪等性）を確認した。
- shallow clone への変更で Neovim ビルドが壊れるリスク → `docker:build` の完走と
  イメージ内での `nvim --version` で確認した。
- terraform のチェックサム検証方式の変更 → go-builder と同一パターンのため
  挙動は既知。ビルド完走で確認した。

## Validation

1. `hadolint environment/docker/nvim.dockerfile`（設定ファイルなし・デフォルトルール）が exit 0。
2. `mise run lint:docker` が pass。
3. `mise run pins:sync` 実行後にファイルが変化しない（sha 一致）。
4. `mise run docker:build` が完走。
5. イメージ内スモーク: `nvim --version` / `terraform version` / `gopls version` /
   `node --version` / `stylua --version` / `tree-sitter --version` / `hadolint --version` /
   `uv --version` がすべて動作。
