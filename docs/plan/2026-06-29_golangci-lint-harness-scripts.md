# Plan: scripts/ 配下の golangci-lint ハーネス強化

AI駆動開発における「ハーネス（検証ループ）」の質を高めるため、Go コードが集中する `scripts/`（`nvim-sync` / `ai-bridge` の2モジュール）に明示的な golangci-lint 設定を導入し、バグ検出と規約の機械的担保を内側ループ（Stop フック）と CI の双方で効かせる。

## Background

- 現状、両モジュールの golangci-lint は **設定ファイルなし＝GitHub Action のデフォルト**でしか回っておらず、AIへのフィードバックが弱い。
- `lint-changed.sh` Stop フックは変更ディレクトリに対し `golangci-lint run` を実行するため、設定を強化すると AI の内側ループの解像度がそのまま上がる。
- `scripts/ai-bridge/AGENTS.md` は既に規約（table-driven test + `t.Parallel()`、`err` の使い回し禁止、不要な export 回避）を定めており、いくつかは特定リンターに対応づけられる。
- 参考: OpenAI「Harness Engineering」、Anthropic「Steering Claude Code」。

## Current structure

- 2モジュールとも **Go 1.26**、それぞれ `go.mod` を持つ別モジュール。
  - `scripts/nvim-sync`（~1,032 LOC, pkg: main/syncer/watcher）
  - `scripts/ai-bridge`（~1,990 LOC, pkg: main/daemon/doctor/launchd/launcher/testutil/watcher）
- `.golangci.{yml,yaml,toml}` はリポジトリ内に存在しなかった。
- CI（`.github/workflows/lint_{nvim_sync,ai_bridge}.yml`）は `golangci/golangci-lint-action@v9.2.0`（= golangci-lint v2 系）を `working-directory` 指定で実行。→ 設定は v2 フォーマット必須。

## 採用方針

- リンター範囲: バグ検出 + 規約 + 品質（推奨ティア）。
- 既存違反: 同じ変更で全部修正（CIを最初から緑に）。
- 設定配置: リポジトリ直下に `.golangci.yml` を1つ。golangci-lint は実行ディレクトリから上位を探索するため、両モジュール・CI・Stop フックすべてがこの1ファイルを参照する（DRY）。

## 変更内容

### 1. `.golangci.yml`（リポジトリ直下・新規, v2 フォーマット）

標準セット（errcheck / govet / ineffassign / staticcheck / unused）を維持して追加有効化:

- バグ検出: `errorlint` `nilerr` `nilnil` `bodyclose` `contextcheck` `makezero` `wastedassign`
- 規約・品質: `gocritic` `revive` `unconvert` `unparam` `misspell`
- テスト品質: `tparallel` `thelper`
- settings: `govet` の `shadow`（`err` の使い回しを部分的に担保）、`errcheck.check-type-assertions`、`gocritic` tags（diagnostic/performance/style）
- exclusions: `_test.go` では `errcheck` / `unparam` / `revive` を緩和（ドレインループ等の慣用句を許容）
- formatters: `gofmt` + `goimports`

> 見送り: `gosec`（docker/tmux を exec するため G204 多発）と `paralleltest`（`t.Setenv` 例外と衝突）はノイズが大きいため今回は対象外。`err` の同一スコープ再代入禁止は対応リンターが無いため規約として残す。

### 2. 既存違反の修正

- `thelper`: ヘルパークロージャに `t.Helper()` を追加（nvim-sync 2, ai-bridge 4）。
- `revive package-comments`: 6パッケージにパッケージコメントを追加（launchd/launcher/testutil/main/daemon/watcher）。

### 3. CI

CI は `working-directory` をモジュール配下に指定しているが、golangci-lint が上位のルート設定（`../../.golangci.yml`）を探索・適用することをサブディレクトリ実行で確認済みのため、ワークフローの変更は不要。

## 検証結果

| モジュール | golangci-lint | go vet | go test   |
| ---------- | ------------- | ------ | --------- |
| nvim-sync  | 0 issues      | ✅     | ✅ 全パス |
| ai-bridge  | 0 issues      | ✅     | ✅ 全パス |

## 備考

golangci-lint はデフォルトで `max-same-issues=3` のため、初回実行では同種違反が3件しか表示されず見落としやすい。全件確認は `--max-same-issues=0 --max-issues-per-linter=0` を付ける。
