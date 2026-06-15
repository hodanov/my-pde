# Plan: 自律的プロダクト改善のための定期スキャンワークフロー

`anthropics/claude-code-action@v1` を cron で定期実行し、リポジトリ全体（コード・Skills・Workflows）を横断的に分析して改善提案を Issue として自動起票する。まずは「Issue 起票のみ」で 1〜2 週間試運転し、提案の質を見てから低リスク改善の自動 PR 化へ拡張する段階的アプローチを取る。

## Background

- このリポジトリ（`my-pde`）は個人開発環境であり、Go モジュール（`scripts/ai-bridge/`）、Skill 定義（`.claude/skills/`・`ai-agents/skills/`）、複数の lint/test ワークフロー（`.github/workflows/`）を横断的に持つ。
- 手動レビューに頼らず、定期的にコード品質・Skill 定義・CI/CD の改善余地を洗い出す仕組みを持ちたい。
- `claude-code-action` は GitHub Actions ランナー上で Claude Code ランタイムをフル実行し、リポジトリ全体・ファイル構造・git 履歴・diff を読み、ツール実行までできる。単なる API 呼び出しのラッパーではないため、`golangci-lint ./...` の実行結果や Skills ディレクトリ構造もそのまま読ませて分析させられる。
- 公式アクションは定期メンテナンスタスク向けに cron スケジュールで使うパターンを提供しており、これがそのまま土台になる。

## Current structure

- `scripts/ai-bridge/` — Go モジュール（`go.mod`）。`.github/workflows/lint_ai_bridge.yml` で golangci-lint 対象。
- `.claude/skills/`、`ai-agents/skills/` — Skill 定義が配置されている。
- `.github/workflows/` — `lint_ai_bridge.yml` / `lint_dockerfile.yml` / `lint_format.yml` / `lint_shell.yml` / `lint_stylua.yml` / `test_ai_bridge.yml` / `pr-docker-build.yml` / `auto-merge-deps.yml` / `bump-tool-versions.yml` / `update-go-tools.yml` などが稼働中。
- リポジトリ直下に Go コードは無く、Go は `scripts/ai-bridge/` 配下に閉じている。

## Design policy

- ベースは `anthropics/claude-code-action@v1` + cron スケジュール（`workflow_dispatch` も併設して手動実行可能にする）。
- 出力先はまず Issue 起票に限定する。自動 PR より先に Issue で提案を出し、人間がレビューしてから着手する形を安全側のデフォルトとする。`pull-requests: write` 権限は付けず、Issue 起票が安定してから追加する。
- 一つのプロンプトに全部を詰め込まず、対象別（コード改善 / Skills 改善 / CI 改善）にプロンプトを分割する。**分割方式は matrix ジョブを採用する**。理由は (1) パフォーマンス: 3 系統が並列実行されスキャン全体の所要時間が短い、(2) 監理のしやすさ: 系統ごとにログ・失敗・リトライが独立し、対象パスや権限もジョブ単位で最小化できるため。以前検討したマルチエージェント構成（security / performance / quality / test coverage）の考え方をここに応用する。
- Skills のスキャン対象は `ai-agents/skills/` のみとする（`.claude/skills/` は対象外）。
- **Issue の重複排除**: 新規起票の前に、各ジョブで既存の Open Issue（同系統のラベルで絞り込み）を `gh issue list` で取得して Claude に読ませる。同等の提案が既に存在する場合は新規作成せず、既存 Issue を更新するか起票をスキップする。
- 認証は Claude サブスク（Pro）の OAuth トークン（`claude_code_oauth_token`）を採用する。理由は Pro 契約済みのため**従量課金を発生させずサブスク枠内でスキャンを賄える**から。注意点として (1) Pro の利用枠は小さく週次スキャンが対話用クォータを食い合う、(2) `claude setup-token` で発行するトークンは有効期限がありローテーションが要る、の 2 点を許容する。スキャン範囲とターン数を絞って消費を抑え、対話作業のクォータを圧迫する場合は API キー（`anthropic_api_key`、従量課金）へフォールバックする。Bedrock / Vertex AI / Foundry も選択肢としては存在する。
- コスト管理を前提に組む。`--max-turns` や `max_tokens` の制限、対象パスのフィルタ（`ai-agents/skills/**`、`.github/workflows/**`、`scripts/ai-bridge/**` など）でスキャン範囲を絞り、API 消費を抑える。
- 試運転で提案の質を確認できたら、機械的・低リスクな改善（フォーマット系・lint エラー修正など）に限ってドラフト PR の自動作成へ拡張する。
- **採用判定**: 提案を採用・不採用で扱うための専用ラベル（`adopted` / `rejected`）を Issue に付与して記録する。
- **PR 自動化への移行基準**: 直近で作成された Issue が **10 回連続で採用された**（`adopted` ラベルが連続して付き、間に `rejected` が挟まらない）ことを基準とする。これを満たしたら `pull-requests: write` を追加して自動 PR 化フェーズに移行する。判定は試運転段階では手動チェックとし、`gh issue list --state closed --json number,labels,closedAt` を新しい順に走査して連続採用数を数える。

## Implementation steps

1. **週次スキャンワークフローを追加** — cron トリガー（毎週日曜 `0 0 * * 0`）+ `workflow_dispatch` のワークフローを `.github/workflows/` に新規作成する。最初は Issue 起票のみに振る舞いを限定し、`pull-requests: write` は付与しない。
2. **対象別 matrix ジョブで分割** — コード改善（`scripts/ai-bridge/`）/ Skills 改善（`ai-agents/skills/`）/ CI 改善（`.github/workflows/`）の 3 系統を matrix で並列実行する。各ジョブはスキャン対象パスとプロンプトを系統ごとに分け、権限も最小化する。
3. **各ジョブで lint 結果を Claude に読ませる** — コード改善ジョブは `golangci-lint ./...`（`scripts/ai-bridge/`）、CI 改善ジョブは `actionlint`（後述）の実行結果を Claude に参照させる。
4. **Issue 起票を実装** — `issues: write` 権限を付与し、`gh issue create` 相当を Claude に実行させて提案を起票する。
5. **1〜2 週間の試運転** — Issue 起票のみで運用し、提案の質を評価する。
6. **低リスク改善の自動 PR 化** — Issue 起票が安定したら、フォーマット系・lint エラー修正など機械的なものに限り、`pull-requests: write` 権限を追加してドラフト PR 作成まで自動化する。
7. **コスト管理の調整** — `--max-turns` / `max_tokens` / 対象パスフィルタを実運用のログを見ながらチューニングする。

### ベースとなるワークフロー YAML（試運転フェーズ: Issue 起票のみ / matrix 分割）

```yaml
name: Weekly Improvement Scan
on:
  schedule:
    - cron: "0 0 * * 0" # 毎週日曜
  workflow_dispatch:
jobs:
  scan:
    runs-on: ubuntu-latest
    permissions:
      contents: read # 試運転は read のみ。PR 自動化フェーズで write に引き上げ
      issues: write # Issue 起票に必要
      id-token: write
      # pull-requests: write は Issue 起票が安定してから追加する
    strategy:
      fail-fast: false
      matrix:
        target:
          - name: code
            label: "scan:code"
            paths: "scripts/ai-bridge/**"
            prompt: "コード品質・リファクタリング箇所を分析(下記の golangci-lint 結果も参照)。"
          - name: skills
            label: "scan:skills"
            paths: "ai-agents/skills/**"
            prompt: "ai-agents/skills/ 配下の Skill 定義の改善余地を分析。"
          - name: ci
            label: "scan:ci"
            paths: ".github/workflows/**"
            prompt: ".github/workflows/ の CI/CD 改善を分析(下記の actionlint 結果も参照)。"
    steps:
      - uses: actions/checkout@v6
        with: { fetch-depth: 0 }

      # 系統ごとに必要な lint を事前実行し、結果を後続で Claude に読ませる
      - name: Run golangci-lint
        if: matrix.target.name == 'code'
        working-directory: scripts/ai-bridge
        run: golangci-lint run ./... | tee /tmp/lint.txt || true
      - name: Run actionlint
        if: matrix.target.name == 'ci'
        run: |
          bash <(curl -s https://raw.githubusercontent.com/rhysd/actionlint/main/scripts/download-actionlint.bash)
          ./actionlint -color | tee /tmp/lint.txt || true

      # 既存 Open Issue を取得し、重複排除の判断材料として Claude に渡す
      - name: List existing issues
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh issue list --state open --label "${{ matrix.target.label }}" \
            --json number,title,body --limit 50 > /tmp/existing_issues.json

      - uses: anthropics/claude-code-action@v1
        with:
          claude_code_oauth_token: ${{ secrets.CLAUDE_CODE_OAUTH_TOKEN }}
          prompt: |
            REPO: ${{ github.repository }}
            対象パス: ${{ matrix.target.paths }}
            ${{ matrix.target.prompt }}
            lint 結果がある場合は /tmp/lint.txt を読んで参照すること。
            起票前に /tmp/existing_issues.json で既存 Open Issue を確認し、
            同等の提案が既にある場合は新規作成せず、既存 Issue を更新するかスキップせよ。
            新規起票する場合は `gh issue create --label "${{ matrix.target.label }}"` を使うこと。
```

> SHA 固定: 既存ワークフローは `actions/*` をコミット SHA でピン留めしている。実装時は `actions/checkout` / `anthropics/claude-code-action` も同じ方針で SHA ピンに揃える。

### actionlint の実行・参照方法

- **方針**: 既存 CI に専用 actionlint ジョブは無く重複しないため、スキャンワークフローの CI 改善ジョブ内に actionlint を組み込む。
- **実行**: CI 改善ジョブのステップで公式の `download-actionlint.bash` を使ってバイナリを取得・実行し、結果を `/tmp/lint.txt` に書き出す。
- **参照**: `actionlint` は既に `ai-agents/settings/claude/settings.json` の allowlist（`Bash(actionlint:*)`）にあるため、Claude 自身に再実行・再確認させることも可能。基本は事前ステップの出力（`/tmp/lint.txt`）をプロンプトで読ませ、必要に応じて Claude が追加実行する。
- **既存 CI への展開（任意）**: 試運転で有用と分かれば、独立した `lint_actionlint.yml` を PR トリガーで追加する選択肢もある（スキャンとは別タスク）。

## File changes

| File                                                    | Change                                                                                                             |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `.github/workflows/weekly-improvement-scan.yml`（新規） | cron + `workflow_dispatch` トリガーの定期スキャンワークフローを追加。`claude-code-action@v1` を実行し Issue を起票 |
| `CLAUDE_CODE_OAUTH_TOKEN`（リポジトリ Secret）          | `claude setup-token` で発行した Pro サブスクの OAuth トークンを登録。期限切れ時は再発行・更新が必要                |
| GitHub ラベル `scan:code` / `scan:skills` / `scan:ci`   | 重複排除のための系統別ラベルを事前作成（`gh label create`）                                                        |
| GitHub ラベル `adopted` / `rejected`                    | 採用・不採用を記録する判定ラベルを事前作成（`gh label create`）                                                    |

## Risks and mitigations

| Risk                         | Mitigation                                                                                                                                                                                                    |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pro クォータの圧迫           | `--max-turns` / `max_tokens` 制限、対象パスフィルタ（`ai-agents/skills/**`、`.github/workflows/**`、`scripts/ai-bridge/**`）でスキャン範囲を限定。対話作業に支障が出たら API キー（従量課金）へフォールバック |
| OAuth トークンの期限切れ     | `claude setup-token` で再発行し Secret `CLAUDE_CODE_OAUTH_TOKEN` を更新。スキャン失敗時の確認項目に含める                                                                                                     |
| 低品質・ノイズの多い提案     | まず Issue 起票のみで 1〜2 週間試運転し、質を確認してから PR 自動化へ拡張                                                                                                                                     |
| 自動 PR による意図しない変更 | 初期は PR 自動化を行わない。拡張時もフォーマット系・lint 修正など機械的・低リスクなものに限定し、ドラフト PR とする                                                                                           |
| 権限の過剰付与               | 試運転フェーズは必要最小限の権限（Issue 起票なら `issues: write`）に絞り、PR 自動化フェーズで `pull-requests: write` を追加                                                                                   |

## Validation

- [ ] `workflow_dispatch` で手動実行し、ワークフローが正常完了する
- [ ] スキャン結果が想定どおり Issue として起票される
- [ ] Issue に対象別（コード / Skills / CI）の提案が分かれて含まれている
- [ ] golangci-lint の実行結果が分析に反映されている
- [ ] 既存 Open Issue を読んだうえで、重複した提案を新規起票しない
- [ ] レビュー時に各 Issue へ `adopted` / `rejected` ラベルを付与して採用判定を記録する
- [ ] 試運転中、スキャンが Pro の対話用クォータを圧迫していないかモニタする
- [ ] 1〜2 週間の試運転で提案の質・コストが許容範囲か評価する
- [ ] `adopted` が 10 回連続（間に `rejected` なし）になったら PR 自動化フェーズに移行する
- [ ] （PR 自動化フェーズ）低リスク改善がドラフト PR として作成される

## Decisions

以下は当初の Open questions に対する確定事項。

- **PR 権限**: `pull-requests: write` は最初から付けず、Issue 起票が安定してから追加する。
- **プロンプト分割**: matrix ジョブを採用（パフォーマンスと監理のしやすさを優先）。
- **Skills 対象**: `ai-agents/skills/` のみを対象とする（`.claude/skills/` は対象外）。
- **実行頻度**: 週次（`0 0 * * 0`）。
- **actionlint**: スキャンワークフローの CI 改善ジョブ内で公式スクリプトにより実行し、出力を Claude に読ませる（[actionlint の実行・参照方法](#actionlint-の実行参照方法) 参照）。既存 CI と重複しない。
- **認証**: Claude サブスク（Pro）の OAuth トークン（`claude_code_oauth_token`）を採用。Pro 契約済みで従量課金を避けられるため。`claude setup-token` で発行し Secret `CLAUDE_CODE_OAUTH_TOKEN` に登録する。Pro クォータを圧迫し対話作業に支障が出る場合は API キー（`anthropic_api_key`、従量課金）へフォールバックする。
- **採用判定**: 専用ラベル `adopted` / `rejected` を Issue に付与して記録する。
- **PR 自動化への移行基準**: `adopted` が 10 回連続（間に `rejected` を挟まない）になったら、PR 自動化フェーズへ移行する。試運転段階では手動チェックでカウントする。
- **Issue の重複排除**: 毎回新規作成せず、各ジョブで既存 Open Issue（系統別ラベルで絞り込み）を `gh issue list` で取得 → Claude が確認し、重複は更新またはスキップする。

## Open questions

- なし（主要な設計判断は確定済み）。実装時に細部を詰める。
