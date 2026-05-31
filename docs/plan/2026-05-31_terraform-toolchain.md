# Plan: Terraform ツールチェーンの導入

この PDE (Neovim on Docker) で AWS を Terraform 管理するために、フォーマッタ・リンタ・LSP(定義ジャンプ)・構文ハイライトを既存パターンに沿って追加する。デファクトツール (`terraform fmt` / `terraform-ls` / `tflint` / treesitter) を選定し、Dockerfile・nvim 設定・CI 自動バンプへ組み込む。

## Background

- AWS アカウントのリソースを Terraform で管理することになり、この個人開発環境でコーディングとレビューを完結させたい。
- 既存の各言語 (Go / Python / Lua / Markdown 等) と同じ「Docker でツールを焼き込み + nvim で LSP/Format/Lint」の体験を Terraform でも揃える。
- 最低限フォーマッタとリンタは必須。加えて定義ジャンプ (LSP) を実現したい。
- 対象の Terraform コード本体は別リポジトリに置く想定。この PDE は「ツールチェーンの提供」が役割で、`.tf` ファイル自体はここには持ち込まない (CI lint 対象も別リポジトリ側)。

### 確定した方針 (2026-05-31)

- **CLI は terraform 本体を採用** (OpenTofu は見送り)。社内に terraform ベースの資産が多く、OpenTofu をメイン運用しているプロダクトが現状ない。今回の AWS 構成管理は急ぎのため、ノウハウにリーチしやすい terraform CLI を優先する。
- **初期スコープは `terraform` / `terraform-ls` / `tflint` / treesitter の 4 点に限定**。`trivy` / `checkov` / `terraform-docs` は今回入れない。
  - `trivy`/`checkov` は保存ごとのリアルタイム lint には不向きで、CI・git hooks・レビュー時の明示スキャン向き (`tflint` がリアルタイム lint、スキャナは随時実行という役割分担)。`trivy` は Dockerfile・コンテナイメージ CVE 等にも使える汎用性があるため将来の追加候補として残すが、今回は導入しない。
  - `terraform-docs` は人が読むには便利だが、解析は AI に任せる方針のため不要。
- **PDE はツールチェーン提供のみ**。このリポジトリに `.tf` サンプルや Terraform 用 CI (fmt -check / tflint) は置かない。
- **`tflint-ruleset-aws` のバージョン管理は各 Terraform プロジェクトに委ねる**。PDE 側は `tflint` 本体のバイナリ提供のみ。

## Current structure

- `environment/docker/nvim.dockerfile`: マルチステージビルド。ツール種別ごとに builder stage を分け、最終段で `COPY --from=...`。
  - Go ツール: `environment/tools/go/go-tools.txt` を `go install` でループ導入 (Stage 3)。
  - バイナリ配布物 (hadolint): 専用 builder stage で release バイナリを取得し、checksum 検証して `/usr/local/bin` へ配置 (Stage 6)。
- `nvim/config/lua/lsp/init.lua`: `vim.lsp.enable("...")` で LSP を有効化 (Neovim 0.12 ネイティブ style, nvim-lspconfig 併用)。`gd` で `vim.lsp.buf.definition` (定義ジャンプ) を割当済み。
- `nvim/config/lua/conform_nvim.lua`: `formatters_by_ft` でフォーマッタを定義 (format on save 有効)。
- `nvim/config/lua/nvim_lint.lua`: `linters_by_ft` でリンタを定義 (`BufReadPost`/`BufWritePost` で実行)。
- `nvim/config/lua/nvim_treesitter.lua`: `ts.install({...})` + `FileType` autocmd で対象言語のハイライトを有効化。
- `.github/workflows/bump-tool-versions.yml`: 毎週 Dockerfile の `ARG` バージョンを最新へ自動更新し PR を作成。
- `environment/tools/go/update-go-tools.sh`: `go-tools.txt` の各ツールを `@latest` に更新するスクリプト。
- `AGENTS.md`: ファイル種別ごとの Lint/Format 一覧表を保持。

## Design policy

- 既存パターンを踏襲する。新規ツールも「Dockerfile builder stage で焼き込み + nvim 設定 + CI 自動バンプ」の三点セットで導入する。
- ツールの入手経路で配置先を分ける。
  - `go install` 可能なもの (`terraform-ls`, `tflint`) → `go-tools.txt` に追記 (自動バンプ対象)。
  - `go install` できない `terraform` CLI 本体 → hadolint と同じく専用 builder stage で release zip を取得し checksum 検証。
- フォーマッタは `terraform` CLI 同梱の `terraform fmt` を使う (conform の `terraform_fmt` 経由)。専用フォーマッタは入れない。
- 定義ジャンプ・補完・hover・参照は公式 LSP `terraform-ls` に任せる。既存の `gd`/`K`/`grr` マッピングがそのまま効く。
- AWS 固有のベストプラクティス検査は `tflint` + `tflint-ruleset-aws` プラグインで行う。プラグインはプロジェクト側 `.tflint.hcl` + `tflint --init` で導入する (PDE 側はバイナリ提供のみ)。
- セキュリティ/誤設定スキャン (`trivy` 等) とドキュメント生成 (`terraform-docs`) は今回のスコープ外 (Background の確定方針参照)。`trivy` は将来 PDE か Mac ホストへ追加する余地を残す。

## Tool selection (デファクト)

| 役割                               | ツール                                | 選定理由                                                            | 導入方法                                                     |
| ---------------------------------- | ------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------ |
| フォーマッタ                       | `terraform fmt` (terraform CLI 同梱)  | 公式・標準。追加フォーマッタ不要                                    | release binary builder stage                                 |
| LSP (定義ジャンプ/補完/hover/参照) | `terraform-ls` (HashiCorp 公式)       | 公式 LSP。`gd` 等の既存マッピングで定義ジャンプ可能                 | `go-tools.txt`                                               |
| リンタ                             | `tflint` + `tflint-ruleset-aws`       | デファクトの Terraform linter。AWS ルールセットで provider 固有検査 | `go-tools.txt` (本体) + per-project `tflint --init` (plugin) |
| 構文ハイライト                     | treesitter `terraform` / `hcl` パーサ | 既存の treesitter 基盤に追加するだけ                                | `nvim_treesitter.lua`                                        |

- `terraform` CLI は BSL ライセンス。OSS 代替の OpenTofu (`tofu`) は MPL で `fmt`/`validate` 挙動は同等だが、今回は terraform 本体を採用 (Background 参照)。
- スコープ外 (将来候補): `trivy` (IaC 誤設定 + Dockerfile/コンテナ CVE スキャン、単一 Go バイナリ)、`terraform-docs` (module README 生成)。`checkov` は Python 依存で重く汎用性も `trivy` に劣るため不採用。

## Implementation steps

1. `environment/docker/nvim.dockerfile` に Terraform CLI の builder stage を追加する (hadolint stage を雛形にする)。
   - `ARG TERRAFORM_VERSION=<latest>` を unindented・single-line で追加 (CI sed の対象にするため)。
   - `https://releases.hashicorp.com/terraform/<ver>/terraform_<ver>_linux_<arch>.zip` を取得、`terraform_<ver>_SHA256SUMS` で checksum 検証、`/usr/local/bin/terraform` へ `install`。
   - 最終段に `COPY --from=terraform-builder /usr/local/bin/terraform /usr/local/bin/terraform` を追加。
2. `environment/tools/go/go-tools.txt` に LSP とリンタを追記する。
   - `github.com/hashicorp/terraform-ls@<ver>`
   - `github.com/terraform-linters/tflint@<ver>`
3. `nvim/config/lua/lsp/init.lua` に `vim.lsp.enable("terraformls")` を追加する (必要なら `vim.lsp.config("terraformls", {...})`)。
4. `nvim/config/lua/conform_nvim.lua` の `formatters_by_ft` に `terraform = { "terraform_fmt" }` と `["terraform-vars"] = { "terraform_fmt" }` を追加する。
5. `nvim/config/lua/nvim_lint.lua` の `linters_by_ft` に `terraform = { "tflint" }` を追加する。
6. `nvim/config/lua/nvim_treesitter.lua` の `ts.install({...})` と `FileType` autocmd の pattern に `terraform`, `hcl` を追加する。
7. `.github/workflows/bump-tool-versions.yml` に `TERRAFORM_VERSION` の解決と `sed` 置換ステップ、PR 本文への記載を追加する。
   - 解決元は HashiCorp Checkpoint API: `TERRAFORM_VERSION=$(curl -s https://checkpoint-api.hashicorp.com/v1/check/terraform | jq -r '.current_version')`
   - `sed -i.bak -E "s/^ARG TERRAFORM_VERSION(=.*)?$/ARG TERRAFORM_VERSION=${{ steps.resolve.outputs.terraform }}/"` を Dockerfile 更新ステップに追加。
8. `AGENTS.md` の Lint/Format 表に Terraform 行を追加し、`README.md` の Features の LSP 列挙に Terraform を追記する。`tflint-ruleset-aws` は各プロジェクトの `.tflint.hcl` + `tflint --init` で導入する旨も明記する。
9. イメージを再ビルドし、`.tf` ファイルで動作確認する (`checkhealth` / 定義ジャンプ / format on save / tflint diagnostics)。

## File changes

| File                                       | Change                                                                                                          |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| `environment/docker/nvim.dockerfile`       | `ARG TERRAFORM_VERSION` 追加、`terraform-builder` stage 追加、最終段に `COPY --from=terraform-builder ...` 追加 |
| `environment/tools/go/go-tools.txt`        | `terraform-ls`, `tflint` を追記                                                                                 |
| `nvim/config/lua/lsp/init.lua`             | `vim.lsp.enable("terraformls")` を追加                                                                          |
| `nvim/config/lua/conform_nvim.lua`         | `terraform` / `terraform-vars` に `terraform_fmt` を追加                                                        |
| `nvim/config/lua/nvim_lint.lua`            | `terraform = { "tflint" }` を追加                                                                               |
| `nvim/config/lua/nvim_treesitter.lua`      | `terraform`, `hcl` パーサを install / FileType に追加                                                           |
| `.github/workflows/bump-tool-versions.yml` | `TERRAFORM_VERSION` の解決と sed 置換、PR 記載を追加                                                            |
| `AGENTS.md`                                | Lint/Format 表に Terraform 行を追加                                                                             |
| `README.md`                                | Features の LSP 列に Terraform を追記                                                                           |

## Risks and mitigations

| Risk                                                                          | Mitigation                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `terraform` CLI は BUSL-1.1 (BSL) でこのリポジトリは Public                   | BSL の制限は「HashiCorp と競合する商用製品として Terraform を提供する」ケースのみ。本 PDE は (1) 自分のインフラ管理のための dev ツール用途で Additional Use Grant の許可範囲、(2) Dockerfile が公式バイナリをビルド時に取得するだけでソース/バイナリを再配布していない、(3) BSL が縛るのは Terraform 本体で本リポジトリ自身のライセンスには及ばない — の 3 点から Public でも問題なし。ポリシー変更時は MPL の OpenTofu (`tofu`) へ切替可能 (`fmt`/`validate` 挙動は同等) |
| `terraform` 本体は `go install` できず `go-tools.txt` に載せられない          | hadolint と同じ release バイナリ builder stage で取得し checksum 検証                                                                                                                                                                                                                                                                                                                                                                                                     |
| `tflint-ruleset-aws` は `tflint --init` でネットワーク経由 DL が必要          | PDE は `tflint` 本体のみ提供。プラグインはプロジェクト側 `.tflint.hcl` + `tflint --init` で導入する運用を AGENTS.md に明記                                                                                                                                                                                                                                                                                                                                                |
| `go install` した `tflint` の `--version` 表記が空になりうる (ldflags 未注入) | 動作には影響なし。気になる場合は release バイナリ stage に切替                                                                                                                                                                                                                                                                                                                                                                                                            |
| treesitter `terraform`/`hcl` パーサのコンパイルに tree-sitter CLI が必要      | 既存イメージに `tree-sitter` が同梱済みのため追加対応不要                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ツール追加でイメージ再ビルドが必須                                            | コミット/PR に「container rebuild required」を明記 (AGENTS.md の規約に従う)                                                                                                                                                                                                                                                                                                                                                                                               |
| `TERRAFORM_VERSION` の最新取得方法を bump スクリプトに正しく実装する必要      | HashiCorp Checkpoint API (`https://checkpoint-api.hashicorp.com/v1/check/terraform` の `.current_version`) で解決。ダウンロード元 `releases.hashicorp.com` と source of truth が一致し、公開済みバージョンのみ返るため skew を回避。常に最新 GA stable を返すので prerelease 除外ロジックも不要                                                                                                                                                                           |

## Validation

ホストで実施済み (2026-05-31):

- [x] `hadolint` / `stylua --check` / `prettier --check` / `markdownlint-cli2` が変更ファイルでクリーン
- [x] `terraform-builder` stage を `--target` ビルドし、checksum 検証・unzip・install が成功
- [x] ビルドしたイメージで `terraform version` → `Terraform v1.15.5 on linux_arm64` を確認

フルイメージ再ビルドが必要 (未実施):

- [ ] イメージを再ビルドして `nvim --headless -c 'checkhealth' -c 'qall'` がエラーなく完了する
- [ ] `terraform-ls --version` / `tflint --version` がコンテナ内で実行できる
- [ ] `.tf` を開いて `gd` でリソース/変数/モジュール参照の定義ジャンプができる
- [ ] `.tf` 保存時に `terraform fmt` 相当の format on save が走る
- [ ] 構文エラーや tflint 違反が diagnostics として表示される
- [ ] `.tf` / `.tfvars` の treesitter ハイライトが効く
- [ ] `bump-tool-versions` workflow を `workflow_dispatch` で実行し `TERRAFORM_VERSION` が更新された PR が作られる

## Resolved decisions

これまで Open questions だった項目は 2026-05-31 に確定済み (Background 参照)。

- CLI: **terraform 本体を採用** (OpenTofu 見送り)。
- セキュリティ/誤設定スキャナ: **今回は導入しない**。`trivy` は将来候補として保留、`checkov` は不採用。
- `terraform-docs`: **不要** (解析は AI に任せる)。
- PDE リポジトリへの `.tf` / Terraform 用 CI: **不要** (ツールチェーン提供のみ)。
- `tflint-ruleset-aws` のバージョン管理: **各 Terraform プロジェクトに委ねる**。
- `TERRAFORM_VERSION` の解決元: **HashiCorp Checkpoint API を採用**。ダウンロード元 `releases.hashicorp.com` と source of truth が一致し、公開済みバージョンのみ返るため GitHub API の skew を回避できる (`.current_version` を使用)。

## Open questions

- 将来 `trivy` を追加する場合、PDE コンテナ (ソーススキャン中心) と Mac ホスト (`trivy image` 中心) のどちらに置くか。
