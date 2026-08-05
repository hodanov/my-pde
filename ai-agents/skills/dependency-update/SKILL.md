---
name: dependency-update
description: >-
  依存パッケージ（Go modules / npm / Terraform provider / GitHub Actions / pre-commit 等）の
  アップグレードを、outdated 検出 → CHANGELOG・破壊的変更の確認 → 更新 → lint/test 検証 →
  コミットまで一連の手順で安全に適用する。「依存更新」「パッケージを上げて」「dependency update」
  「go get -u」「npm update」「provider を上げる」等に言及されたときに使用する。
disable-model-invocation: true
argument-hint: "[対象: all / go / npm / terraform / gha / pre-commit] [--major]"
metadata:
  version: 1
---

# /dependency-update スキル

## Goal

依存パッケージのアップグレードを「検出 → 判断 → 適用 → 検証 → 記録」の型で安全に適用する。破壊的変更の評価とテストでの回帰確認という人間の判断が要る部分を飛ばさず、CHANGELOG を確認できない major 更新は保留する（検証先行）。

## 適用範囲・住み分け

- 本スキルは **更新そのものの適用と検証** を担う。GitHub Actions の SHA ピン留めは対象外（版数タグの更新のみ扱う）。
- Dependabot / Renovate が回っているリポジトリでは、本スキルは **major 更新・Bot PR のまとめレビュー・手動追随** を主対象とする。二重運用を避けるため、単一パッケージの patch/minor 追随は Bot に任せてよい。
- `$ARGUMENTS` で対象エコシステムを絞れる（`all` / `go` / `npm` / `terraform` / `gha` / `pre-commit`）。`--major` 指定時のみ major バージョンを対象に含める。

## Workflow

### 1. 検出

作業ツリーがクリーンであることを確認してから、対象エコシステムの outdated を洗い出す。

- Go: `go list -u -m all`（更新可能な module に `[x.y.z]` が付く）
- npm: `npm outdated`
- Terraform: `terraform init -upgrade` の差分（`.terraform.lock.hcl`）
- GitHub Actions: `.github/workflows/**` の `uses:` 版数の棚卸し
- pre-commit: `.pre-commit-config.yaml` の `rev` 棚卸し（`pre-commit autoupdate` は適用時に）

### 2. 判断

- semver で patch / minor / major に仕分ける。
- patch / minor は原則そのまま対象化。
- **major と `--major` 指定分は、CHANGELOG・release note・破壊的変更を確認してから対象化する**。確認できない、または影響が読めない場合は適用を保留し、その旨を報告する。

### 3. 適用

**1 グループ（エコシステム／関連 module 単位）ずつ** 小さく更新する。

- Go: `go get -u ./... && go mod tidy`（major は個別に `go get module@vX`）
- npm: `npm update`、または個別に `npm i pkg@x`
- Terraform: provider の版数制約（`required_providers`）を更新し `terraform init -upgrade`
- GitHub Actions: 版数タグを更新（SHA ピン留めは対象外）
- pre-commit: `pre-commit autoupdate`

### 4. 検証

変更ファイルの種別に応じて既存の検証資産を使い、通るまで修正する。CI（`.github/workflows/` の lint / test）で確認される内容に合わせる。

- Go: `golangci-lint run ./...` と `go test ./...`
- npm: `npm run lint` / `npm test`（定義があるもの）
- Terraform: `terraform fmt -check` / `terraform validate` / `tflint`
- 共通: `markdownlint` / secret-scan（該当時）

回帰が出たら **そのグループをロールバックし、より細かい単位で段階適用** に切り替える。

### 5. 記録

- 変更を意味単位で、命令形メッセージでコミットする（`commit-and-draft-pr` に接続可）。
- 保留した更新（CHANGELOG 未確認の major 等）は最後に明示的に報告する（silent に「全部上げた」風にしない）。

## Notes

- **検証先行**: major は必ず CHANGELOG / breaking change 確認を必須ステップにし、確認できなければ適用しない。
- **段階適用**: lint / test が薄いプロジェクトでは回帰を捕捉しきれない。1 グループずつ適用し、各段でコミットして切り戻し可能にする。
- **暴発防止**: `disable-model-invocation: true` により明示呼び出し前提。自動発火はしない。
- **配布経路**: 本スキルは skill 追加のみのため `ai-agents/Makefile` / `scripts/copy-entries.sh` の変更は不要（既存の `skills/` 配布経路にそのまま乗る）。hook ではないので 3 エディタ分の配線も不要。
