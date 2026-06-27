# AI ステアリング改善プラン — steering Claude Code 適用

> **ステータス（2026-06-27）**: PR #441 で実装・マージ済み。本ファイルは設計の記録として取り込む。

## Context

Anthropic のブログ [Steering Claude Code: skills, hooks, rules, subagents and more](https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more) を基準に、本 PDE リポジトリの「7 つのステアリング機構」の使い分けを見直す。

ブログの意思決定フレーム:

1. **事実**(ビルドコマンド・構成) → CLAUDE.md
2. **制約**(API バリデーション等) → path-scoped Rules
3. **手順**(リリース手順) → Skills
4. **並列作業** → Subagents
5. **自動化**(編集後フォーマット) → Hooks
6. **役割変換** → Output Styles（慎重に）

現状の評価:

- **強い**: PostToolUse フォーマッタ hook 群、二相 review/investigate サブエージェント、observe→improve スキルループ、deny 権限ガード、cloud routines。
- **未活用 / 改善余地**:
  - **Rules を全く使っていない**（最大の空白）。言語別の lint/format 制約や Dockerfile ARG・routines の制約が、常時ロードの `AGENTS.md` / `agents.xml` に混在。
  - `AGENTS.md` が「9 agents / 12 skills」等の**件数をハードコード**しており実体（8 agents / 15 skills）とドリフト。ブログの「index するが duplicate しない」に反する。
  - `agents.xml`（= 配布先 `~/.claude/CLAUDE.md`）が**ペルソナ + 60 行超のワークフロー手順 + 命令形の検証ルール**を一枚に詰め込み。手順は Skill、検証は hook が適所。
  - skills の frontmatter が不揃いで、改善ループ用の version 管理が一部欠落。
  - hook はフォーマットのみ。「ALWAYS run linters」を**決定論的に担保する lint hook がない**。

**決定事項**:

1. ペルソナは **agents.xml 維持 + Rules で Claude 固有を上乗せ**（複数 CLI 配布のポータビリティ優先。Output Styles は採用しない）。
2. 検証は **軽量 lint hook + ガイダンス併用**。
3. 着手範囲: **4 領域すべて**（Rules 導入 / CLAUDE.md スリム化 / skills ブラッシュアップ / hooks 強化・権限整理）。

ゴール: 常時ロードのコンテキストを削り、制約は触れたときだけ載る Rules へ、手順は Skill へ、強制は hook へ寄せて、本リポジトリ開発と汎用 ai-agents 資産の両方の効率を上げる。

## 領域 1: path-scoped Rules の導入

`.claude/rules/<topic>.md` に YAML frontmatter `paths:` でスコープ。該当ファイルに触れたときだけロードされる（docs-only セッションでは載らない）。

### 1-1. リポジトリ内 Rules（`.claude/rules/`）

各ファイルは 1 トピック。`AGENTS.md` の常時ロード内容のうち「特定パスでしか効かない制約」を移す。

| ファイル                 | `paths`                                                      | 内容（移設元）                                                                                                                 |
| ------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| `dockerfile-versions.md` | `environment/docker/nvim.dockerfile`、`environment/tools/**` | ARG 行は無インデント単一行・手動でバージョン bump しない（CI 担当）・変更後はイメージ再ビルド                                  |
| `go-ai-bridge.md`        | `scripts/ai-bridge/**`                                       | バグ修正は test-first、`go vet`/`golangci-lint run`/`goimports`、`make ai-bridge-test`（`scripts/ai-bridge/AGENTS.md` と整合） |
| `lua-nvim.md`            | `nvim/**/*.lua`                                              | `stylua --check .` / `stylua .`                                                                                                |
| `routines.md`            | `routines/*.json`                                            | 定義が source of truth、変更は PR レビュー → 手動 `/schedule` update、CI 自動反映なし                                          |
| `skill-authoring.md`     | `ai-agents/skills/**`                                        | SKILL.md frontmatter 規約・progressive disclosure・observe→improve ループ                                                      |

### 1-2. 汎用（cross-repo）Rules の配布

言語非依存で全リポジトリ共通の lint/format 規約（Markdown/JSON-YAML/Shell/TOML）は `ai-agents/settings/claude/rules/` をソースに置き、既存の `claude-settings-copy`（settings モードの再帰コピー）で `~/.claude/rules/` へ配布。user-level rules は公式サポート済みで全プロジェクトに適用される。Makefile 変更は不要。

## 領域 2: CLAUDE.md / AGENTS.md スリム化

ブログ: root は < 200 行・index するが duplicate しない・件数等のドリフトしやすい値を持たない。

### 2-1. `AGENTS.md`

- lint/format 表を削除し領域 1 の Rules へ移設。AGENTS.md は「触れたファイルの Rule が載る」ポインタのみ残す。
- ハードコードされた件数（9 agents / 12 skills / 9 CI）を撤去し、数を持たない表現に。
- Dockerfile 制約は Rule 化しポインタに圧縮。残すのは構成・主要 make コマンド・コミット/PR 規約・プラットフォーム。

### 2-2. `ai-agents/agents.xml`（= `~/.claude/CLAUDE.md`）

- ワークフロー手順（new_product / existing_product）を新規 `ai-agents/skills/dev-workflow/SKILL.md` へ移設。
- 検証ルールを圧縮し、決定論的担保は領域 4 の lint hook に委ねる。
- ペルソナは維持（複数 CLI 配布のため）。結果として「ペルソナ + 検証 principle + dev-workflow へのポインタ」に縮小。

## 領域 3: ai-agents skills ブラッシュアップ

- frontmatter 統一監査: `name` / `description`（発火条件含む） / `argument-hint` / `disable-model-invocation`（boolean） / `metadata.version` を揃える。全 skill に `metadata.version` を付与。
- 肥大 SKILL.md は手順を SKILL.md に、詳細を `references/` へ（progressive disclosure）。※残タスク。
- observe→improve ループの version bump 接続を `skill-authoring.md` Rule に集約。
- built-in（`/code-review`・`/security-review`）との使い分けを `review` / `review-scan` の冒頭に 1 行明記。

## 領域 4: hooks 強化・権限整理

### 4-1. 軽量 lint feedback hook

- `ai-agents/settings/claude/hooks/lint-changed.sh` を **Stop hook** に追加。`git diff` で変更ファイルを拾い、拡張子別に linter を実行（go/lua/md/shell）。エラーは要約表示（非ブロッキング）。macOS の bash 3.2 互換。
- 「ALWAYS run linters」を命令文に頼らず編集完了時に自動可視化。フォーマットは既存 PostToolUse、lint は Stop と役割分離。

### 4-2. 権限 allowlist 拡充

`settings.json` の `allow` に `Bash(make:*)` / `Bash(hadolint:*)` / `Bash(tflint:*)` / `Bash(terraform fmt:*)` / `Bash(goimports:*)` / `Bash(docker compose:*)` / `WebFetch(domain:code.claude.com)` を追加。

### 4-3. PreToolUse ガード

Dockerfile ARG 行の手動編集ブロックは検出が不安定なため見送り、Rule（領域 1）+ 既存 deny 権限で代替。

## 検証

1. 追加 md は `markdownlint-cli2`、`settings.json` は `jq` + `prettier --check`、hook は `shellcheck` + `shfmt`。
2. 配布の dry-run: `find` で `lint-changed.sh` と generic rules が `~/.claude/` に乗ることを確認。反映は `make claude-settings-copy` / `make skills-copy` / `make claude-link`。
3. Rules ロード確認: スコープ対象ファイルを開くセッションで載り、無関係セッションで載らないことを実機確認。
4. lint hook 動作確認: lint エラーのあるファイルを編集 → Stop で指摘が出ること。
5. 回帰なし: Go（ai-bridge）は無変更。
6. ドリフト確認: AGENTS.md からハードコード件数が消え、エージェント/スキル追加時に編集不要であること。

## Sources

- [Steering Claude Code](https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more)
- [How Claude remembers your project](https://code.claude.com/docs/en/memory)（Rules: `.claude/rules/` + `paths:` frontmatter、user-level `~/.claude/rules/`）
- [Skill authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
