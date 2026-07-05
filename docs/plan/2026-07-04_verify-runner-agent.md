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
- **マッピングは決定的スクリプトに分離**: 変更検出（lint-changed.sh と同じ unstaged + staged + untracked の union）とファイル種別 → コマンドのマッピングは LLM の判断ではなく `ai-agents/scripts/verify-changed.sh` が決定的に行う。エージェントはスクリプトを起動して出力を要約するだけの存在にし、遅い・高い・マッピングを間違えるという LLM 実行のコストを排除する。エージェントは mise を経由せずスクリプトを直接実行する（ツール欠如は各チェックの `command -v` ガードにより SKIP へ縮退）。人間向けには `mise run verify:changed` を入り口として用意する
- **スクリプトは my-pde 専用、汎用性はエージェントの LLM フォールバックで担う**: 決定性が価値を持つのはマッピングが既知の範囲（= my-pde）だけ。汎用化（go.mod 探索・素の go ツールへの縮退・`~/.local/bin` への symlink 配布・3段フォールバック）をスクリプト側に背負わせると bash の列挙的な条件分岐が肥大化する一方、初見リポジトリの検証コマンド推論は LLM が最も得意な仕事。そこでスクリプトは Go = `scripts/<app>/` + mise タスク前提の my-pde 専用に絞り、スクリプトを持たないリポジトリではエージェントが設定ファイル（`mise tasks ls` / `package.json` / `Makefile` / `pyproject.toml` 等）から検証コマンドを探して実行する。「home 配布でどのリポでも使える」要件は、エージェント定義の repo 非依存性 + このフォールバックで満たす（確実性は落ちるが決定的である必要のない場所）。当初は repo 非依存スクリプト + symlink 配布で実装したが、この理由で 2026-07-05 に簡素化した
- **出力は要約強制**: 成功は 1 行、失敗は file:line + エラー抜粋 10 行程度まで
- **hook の置き換えではなく補完**: Stop hook は決定的で安い最後の砦として残し、verify-runner はオンデマンドの深い検証を担う

## Implementation steps

1. `ai-agents/scripts/verify-changed.sh` を作成し、`mise.toml` に `verify:changed` タスクを追加
2. `ai-agents/agents/verify-runner.md` を作成（`verify:changed` の起動と要約に専念）
3. `ai-agents/skills/dev-workflow/SKILL.md` の verify ステップと検証ルールに委譲の記述を追加（version 2）
4. `mise run agents-copy` と `mise run skills-copy` で配布

## File changes

- `ai-agents/scripts/verify-changed.sh`（新規）: 変更検出 → マッピング → 実行 → `[PASS]/[FAIL]/[SKIP]` レポートまでを決定的に行う my-pde 専用スクリプト。引数でファイル指定可（`mise run verify:changed -- <file>...`）。失敗が 1 つでもあれば exit 1。マッピング対象外のファイルは `no check mapped` として列挙
- `mise.toml`（編集）: `verify:changed` タスクを追加
- `ai-agents/agents/verify-runner.md`（新規）: エージェント定義。リポジトリルートの `ai-agents/scripts/verify-changed.sh` を直接実行 → 出力を日本語見出しの Verification Report（判定/実行結果/失敗詳細/スキップ）に要約。スクリプトを持たないリポジトリでは設定ファイルから検証コマンドを探すフォールバックで全フェーズを代替
- `ai-agents/skills/dev-workflow/SKILL.md`（編集）: 新規/既存両フローの verify ステップと検証ルール（共通）に verify-runner への委譲を追記

## Risks and mitigations

- **マッピングの陳腐化**: mise タスク追加時にスクリプトのマッピングが古くなる。マッピングは verify-changed.sh の 1 箇所に集約されており、対象外ファイルは `no check mapped` として必ず可視化される（サイレントに漏れない）
- **サブエージェントの permission**: `permissionMode: default` + Bash。settings.json の allowlist（`Bash(mise:*)` 等）とサンドボックスで無人実行できる想定。不足コマンドは都度 allowlist に追加する

## Validation

- verify-changed.sh の挙動テスト: pass するファイル / lint 違反ファイル / マッピング対象外ファイル / `scripts/<app>` の Go ファイル → mise タスク / 存在しないパス / Dockerfile → hadolint の 6 ケースで exit code と出力を確認
- 新規/編集した Markdown に対しリポジトリルートから `markdownlint-cli2` を実行して pass を確認。verify-changed.sh は `shfmt -d` + `shellcheck`、mise.toml は `tombi lint` で確認
- `ls ~/.claude/agents/verify-runner.md` で配布を確認
- スモークテスト: 新セッションで変更ファイルがある状態の verify-runner を起動し、レポート形式どおりの要約が返ること・修正を行わないことを確認（エージェント定義は次セッションから有効）
