# Plan: 自律的プロダクト改善のための定期スキャンワークフロー

`anthropics/claude-code-action@v1` を cron で定期実行し、リポジトリ全体（コード・Skills・Workflows）を横断的に分析して改善提案を Issue として自動起票する。まずは「Issue 起票のみ」で 1〜2 週間試運転し、提案の質を見てから低リスク改善の自動 PR 化へ拡張する段階的アプローチを取る。

> **方針転換（2026-06-18）**: 当初の GitHub Actions 版（`.github/workflows/weekly-improvement-scan.yml`）は**削除し、claude.ai の Routine に一本化した**。理由は (1) Actions 版で `--max-turns` 超過が発生した、(2) `CLAUDE_CODE_OAUTH_TOKEN` の発行・期限管理が手間、の 2 点。以降の「## Routine 版」セクションが現行の運用であり、本セクション以下の Actions 版の記述は設計経緯として残す。

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

## Routine 版（クラウドエージェント）: Neovim 動向スキャン

本プラン（GitHub Actions / `claude-code-action` 版）とは別に、**claude.ai のスケジュール Routine（クラウドエージェント / CCR）**として、対象を **Neovim 設定（`nvim/config/`）**に絞った派生版を稼働させた。Actions 版がリポジトリ内部（コード / Skills / CI）の改善を扱うのに対し、こちらは **Web 上の Neovim 最新動向・トレンドを調査し、このリポジトリで活用できる改善を Issue 起票する**ことに特化する。

> **定義の管理**: 以降で扱う Routine（Neovim 動向スキャン / adopted Issue の PR 化）の定義は [`routines/`](../../routines/) ディレクトリの JSON を**正（source of truth）**として一元管理する。`name` / `cron_expression` / `model` / `allowed_tools` / `prompt` などを宣言的に保持し、変更は PR レビューを経て `/schedule`（Update）で反映する。`trigger_id` / `environment_id` は稼働中 Routine を指す識別子（資格情報ではない）として各 JSON にピン留めしている。スキーマと apply 手順・制約は [`routines/README.md`](../../routines/README.md) を参照。

### 設計

- **実行基盤**: claude.ai の Routine（cron スケジュールで隔離されたクラウドセッションを起動）。GitHub Actions ランナーではなく、Anthropic クラウド上で git チェックアウト・ツール実行まで行う。ローカル環境には依存しない。
- **対象**: `nvim/config/` 配下のみ。`init.lua` をエントリに、lazy.nvim（`lazy_nvim.lua` / `plugins.lua`）でプラグイン管理、LSP は `nvim/config/lua/lsp/`。
- **調査範囲**: Web 検索（WebSearch / WebFetch）で Neovim の最新動向を調べつつ、`nvim/config/` の現状も読む。ネタは「最新リリースの新機能」だけに限定せず、**注目プラグイン・モダン代替プラグイン・現行設定のベストプラクティス化・古い書き方の刷新・未活用機能・LSP/treesitter/補完/フォーマッタ/キーマップ/UX 改善**まで広く対象にする。最新動向に変化が無くても素材が尽きないようにするため。
- **出力**: Issue 起票のみ（コード変更・PR 作成はしない）。系統別ラベルは `scan:nvim`。**1 回のスキャンで最大 1 件**に絞り、ターン・コストを抑える。
- **重複排除（Actions 版から強化）**: 起票前に以下の 2 系統を取得し、**いずれとも重複しない**提案を選ぶ。
  1. `gh issue list --state open --label "scan:nvim"` — 既に提案済み（Open）
  2. `gh issue list --state closed --label "scan:nvim" --label "rejected"` — 一度不採用になった（Close 済み rejected）
- **Skip ではなく「別角度」へ**: Actions 版は重複時に「更新またはスキップ」だったが、Routine 版では**有力候補が既存と被る場合はその候補を捨て、被らない別の改善提案を選び直して 1 件起票する**。理由は (1) Neovim 側に変化が無いと永遠に起票されなくなる、(2) 起票 Issue が全て採用されるとは限らず未採用が溜まる、ため。被らない提案がどうしても無い場合に限り起票せず終了する。
- **採用判定の運用**: 不採用にした Issue は Close し `rejected` ラベルを付与する運用が前提（次回スキャンの重複排除が機能する条件）。`adopted` / `rejected` ラベルの考え方は Actions 版と共通。

### Routine 設定値

| 項目         | 値                                                              |
| ------------ | --------------------------------------------------------------- |
| 名前         | `Weekly Neovim Trend Scan`                                      |
| Routine ID   | `trig_01UdqdcQV6FGQkkbTvp3BJ3S`                                 |
| 管理 URL     | <https://claude.ai/code/routines/trig_01UdqdcQV6FGQkkbTvp3BJ3S> |
| スケジュール | `0 23 * * 5`（金 23:00 UTC = **毎週土曜 8:00 JST**）            |
| モデル       | `claude-opus-4-8`（Opus 4.8）                                   |
| リポジトリ   | `https://github.com/hodanov/my-pde`                             |
| 許可ツール   | `Bash` / `Read` / `Grep` / `Glob` / `WebSearch` / `WebFetch`    |
| environment  | Default（`env_01H6ttcff6EJXggu8Xqmx5gN`）                       |

### Actions 版との違い

| 観点         | Actions 版（本体）                       | Routine 版（Neovim）                    |
| ------------ | ---------------------------------------- | --------------------------------------- |
| 実行基盤     | GitHub Actions + `claude-code-action@v1` | claude.ai Routine（クラウド CCR）       |
| 対象         | コード / Skills / CI（リポジトリ内部）   | Neovim 設定（`nvim/config/`）+ Web 動向 |
| 分割         | matrix（3 系統並列）                     | 単一ジョブ（1 系統・最大 1 件）         |
| 重複時の挙動 | 更新 or スキップ                         | 被らない別角度を選び直して起票          |
| 重複対象     | Open Issue（系統別ラベル）               | Open + Close 済み rejected の両方       |
| 認証         | `CLAUDE_CODE_OAUTH_TOKEN`(Secret)        | Routine が claude.ai 側で管理           |
| 頻度         | 週次（日曜 `0 0 * * 0`）                 | 週次（土曜 8:00 JST `0 23 * * 5`）      |

## Routine 版（クラウドエージェント）: adopted Issue の PR 化

スキャンで起票され、人手レビューで `adopted` ラベルが付いた採用済み Issue を、**実装してドラフト PR を作成する** Routine。本プランの「PR 自動化フェーズ」をクラウド Routine として実装したもので、Neovim 動向スキャンと組み合わせて**スキャン → Issue 起票 → 人が `adopted` 付与 → PR Bot がドラフト PR 作成**という一連の自律改善ループを閉じる。動作確認済みで稼働中。

### 設計

- **実行基盤**: claude.ai の Routine（クラウド CCR）。Neovim スキャンと同じ基盤。
- **対象**: `adopted` ラベルが付いた Open Issue **全般**（系統ラベルは問わない）。`scan:nvim` 由来に限定せず、将来 `scan:code` 等にも使い回せるようにした。そのぶん変更範囲は Issue 本文が指す箇所に限定し、最小差分を徹底する。
- **処理件数**: 未処理の adopted Issue を**全件**処理する。**1 Issue = 1 ブランチ = 1 ドラフト PR**。
- **処理済み判定（重複 PR 防止）**: 以下のいずれかに該当する Issue はスキップする。
  1. その Issue に `pr-created` ラベルが付いている
  2. その Issue を参照する Open PR が既に存在する（`gh pr list --state open` で本文に `Closes #<番号>` / `#<番号>` を含む PR があるか確認）
- **実装フロー**: 各 Issue ごとに ①`main` から作業ブランチ（例 `auto/issue-<番号>-<slug>`）を切る → ②提案を実装（最小差分）→ ③検証（Lua は stylua、Go は `golangci-lint run ./...` + `go test ./...`、`Makefile` の該当タスクがあれば活用）→ ④命令形コミット & push → ⑤`gh pr create --draft --assignee hodanov`（body に `Closes #<番号>` と何を・なぜ・検証結果）→ ⑥Issue に `pr-created` ラベル付与（無ければ `gh label create` で作成）。
- **安全策**: すべてドラフト PR で出し、最終マージ判断は手動。`main` への直接コミットはしない。実装が困難・曖昧で安全に進められない Issue はスキップし最後に報告する（`pr-created` は付けない）。

### Routine 設定値

| 項目         | 値                                                              |
| ------------ | --------------------------------------------------------------- |
| 名前         | `Weekly Adopted-Issue PR Bot`                                   |
| Routine ID   | `trig_01GMx9Ye659J9dSa7D9LZoMV`                                 |
| 管理 URL     | <https://claude.ai/code/routines/trig_01GMx9Ye659J9dSa7D9LZoMV> |
| スケジュール | `0 23 * * 6`（土 23:00 UTC = **毎週日曜 8:00 JST**）            |
| モデル       | `claude-opus-4-8`（Opus 4.8）                                   |
| リポジトリ   | `https://github.com/hodanov/my-pde`                             |
| 許可ツール   | `Bash` / `Read` / `Write` / `Edit` / `Grep` / `Glob`            |
| environment  | Default（`env_01H6ttcff6EJXggu8Xqmx5gN`）                       |

### 自律改善ループ全体像

| 段階         | 担い手                        | スケジュール      | 出力                           |
| ------------ | ----------------------------- | ----------------- | ------------------------------ |
| 動向スキャン | `Weekly Neovim Trend Scan`    | 毎週土曜 8:00 JST | `scan:nvim` Issue（最大 1 件） |
| 採用判定     | 人手レビュー                  | 任意              | `adopted` / `rejected` ラベル  |
| PR 化        | `Weekly Adopted-Issue PR Bot` | 毎週日曜 8:00 JST | ドラフト PR（`Closes #N`）     |
| マージ判断   | 人手レビュー                  | 任意              | マージ / クローズ              |

- 追加ラベル: `pr-created`（PR 化済みの adopted Issue に付与し、重複 PR を防止）。
- 前提運用: 実装させたい Issue には `adopted` を、見送る Issue には `rejected`（Close）を付ける。これでスキャン側の重複排除と PR 側の対象選別が両立する。

## Open questions

- なし（主要な設計判断は確定済み）。実装時に細部を詰める。
