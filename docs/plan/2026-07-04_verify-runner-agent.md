# Plan: verify-runner サブエージェントの追加

テスト・lint 実行を隔離コンテキストで引き受け、合否と失敗箇所の要約だけを返す検証実行サブエージェント `verify-runner` を追加する。

## Background

- 既存の 8 サブエージェント（scanner/critic、scout/diver、観点別 review-\*）はすべて読み取り分析型で、検証コマンドを実行する実行系エージェントが存在しない
- テスト・linter の長い出力はメインエージェントのコンテキストを汚染する最大要因（Context Rot 対策の一環）
- `lint-changed.sh` Stop hook は事後検出かつ lint のみ（go/lua/md/sh の 4 種）で、テストは実行しない。作業中に能動的に呼べるテスト込みのフル検証手段が欲しい

## Current structure

- `ai-agents/agents/`: 分析型サブエージェント 8 定義。`mise run agents-copy` で `~/.claude`・`~/.cursor`・`~/.copilot` の `agents/` へ配布
- `mise.toml`: 検証タスク（`<app>:test` / `<app>:lint` / `go:test` / `go:lint` / `lint:*`）
- `ai-agents/settings/claude/hooks/lint-changed.sh`: ターン終了時に変更ファイルへ lint を実行する非ブロッキング Stop hook
- `ai-agents/skills/dev-workflow/SKILL.md`: verify ステップはインライン実行として記述されていた

## Design policy

- **修正はしない**: verify-runner は実行と報告のみ。修正権限を持たせると自己採点ループになり、実装意図を知らないまま「テストを黙らせる修正」をするリスクがあるため、修正は実装コンテキストを持つメインエージェントの責務とする
- **既存規約に準拠**: frontmatter・ボディ構成は `code-review-scanner.md`（Bash 持ち・`permissionMode: default`）をモデルにし、`.claude/rules/agent-authoring.md` に従う。モデルは既存エージェントと同じ sonnet
- **マッピングは lint-changed.sh / mise.toml と整合**: 変更検出は同じ union（unstaged + staged + untracked）、コマンドは mise タスクと CI 相当の linter 群
- **出力は要約強制**: 成功は 1 行、失敗は file:line + エラー抜粋 10 行程度まで
- **hook の置き換えではなく補完**: Stop hook は決定的で安い最後の砦として残し、verify-runner はオンデマンドの深い検証を担う

## Implementation steps

1. `ai-agents/agents/verify-runner.md` を作成
2. `ai-agents/skills/dev-workflow/SKILL.md` の verify ステップと検証ルールに委譲の記述を追加（version 2）
3. `mise run agents-copy` と `mise run skills-copy` で配布

## File changes

- `ai-agents/agents/verify-runner.md`（新規）: エージェント定義。変更ファイル検出 → コマンドマッピング表 → リポジトリルートから実行 → 日本語見出しの Verification Report（判定/実行結果/失敗詳細/スキップ）
- `ai-agents/skills/dev-workflow/SKILL.md`（編集）: 新規/既存両フローの verify ステップと検証ルール（共通）に verify-runner への委譲を追記

## Risks and mitigations

- **マッピングの陳腐化**: mise タスク追加時に表が古くなる。表にないファイル種別は `mise tasks ls` で探すフォールバック指示をボディに記載済み
- **サブエージェントの permission**: `permissionMode: default` + Bash。settings.json の allowlist（`Bash(mise:*)` 等）とサンドボックスで無人実行できる想定。不足コマンドは都度 allowlist に追加する

## Validation

- 新規/編集した Markdown に対しリポジトリルートから `markdownlint-cli2` を実行して pass を確認
- `ls ~/.claude/agents/verify-runner.md` で配布を確認
- スモークテスト: 新セッションで変更ファイルがある状態の verify-runner を起動し、レポート形式どおりの要約が返ること・修正を行わないことを確認（エージェント定義は次セッションから有効）
