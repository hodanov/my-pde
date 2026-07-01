# Plan: my-pde リポジトリ固有の Claude 制御層の効率化

## Background

このリポジトリで Claude を動かすときの効率を上げたい。狙いは **このリポジトリ専用の制御層**
（リポジトリ直下の `AGENTS.md` と `.claude/`）であり、他リポジトリへ配布する再利用層
（`ai-agents/` → `~/.claude` へデプロイ）ではない。Anthropic のブログ
[Steering Claude Code](https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more)
の指針（CLAUDE.md は薄く / 手続きは skill / 決定的強制は hook / path-scoped rule で文脈限定）に照らして、
溜まった残骸を掃除し、指針に沿った穴を埋める。

## Current structure

- **再利用層（対象外）**: `ai-agents/` → `~/.claude/settings.json` 等へデプロイ。clean な allow/deny、
  PostToolUse フォーマッタ、Stop linter、グローバル rule、サブエージェント、スキル。
- **このリポジトリ層（対象）**:
  - `AGENTS.md`（`.claude/CLAUDE.md` は `@AGENTS.md` の 1 行参照＝単一ソース、73 行）
  - `.claude/rules/`（path-scoped rule、tracked）
  - `.claude/settings.local.json`（個人用 allowlist、untracked、デバッグ残骸・重複・古いパスが堆積）
  - `.claude/skills/skill-creator/`（untracked のはぐれコピー）

ブログの指針に照らすと本リポジトリは既に良い形（CLAUDE.md が薄い / rule が path-scoped / 手続きは skill /
隔離は subagent / 決定的処理は hook）。よって再設計は不要で、掃除＋的を絞った穴埋めが本題。

## Design policy

- 既存の良い構造は壊さない。クロスツール共有ソース（`AGENTS.md`）と Claude 専用制御（`.claude/`）の
  役割分担を保つ。
- ブログのアンチパターン「never do X をプロンプトで書く」→ 決定的な PreToolUse hook へ。
- 未 rule 化の頻出領域は path-scoped rule で穴埋め（普段はトークンゼロ、触れた時だけロード）。
- 冗長・重複・古い設定は削り、context と permission の無駄を減らす。

## Implementation steps

1. **クリーンアップ**: `.claude/settings.local.json` からデバッグ残骸・古いパス（`my_dotfiles_nvim`）・
   グローバル allowlist と重複するエントリを削除（60+ → 25）。はぐれ `.claude/skills/skill-creator/` を削除。
2. **決定的フック**: `.claude/hooks/guard-version-pins.sh` を新設し、`.claude/settings.json`（tracked、
   PreToolUse のみ、グローバルとマージ）で登録。Dockerfile の `ARG *_VERSION=`/`*_TOOLCHAIN=` 行、および
   ピンマニフェスト `environment/tools/go/go-tools.txt`・`environment/tools/node/package.json` の手動編集を
   exit 2 でブロックし、更新経路（`update-go-tools.sh` / `bump-tool-versions.yml`）を案内する。
3. **未カバー rule 追加**: `plan-docs.md`（`docs/plan/**`）、`agent-authoring.md`（`ai-agents/agents/**`）、
   `terraform.md`（`**/*.tf`）を既存ファイルから規約抽出して新設。
4. **デプロイのスキル化**: `.claude/skills/deploy-ai-config/SKILL.md` を新設し、`make claude-link` /
   `skills-copy` / `agents-copy` / `settings-copy` / `dotfiles-link` の「どの編集にどのターゲットか」＋検証を手続き化。
5. **ai-bridge 詳細規約の確実ロード**: Claude Code は `AGENTS.md` を自動ロードしない（読むのは `CLAUDE.md`
   のみ）。`scripts/ai-bridge/CLAUDE.md`（中身は `@AGENTS.md`）を新設し、配下に触れると nested CLAUDE.md 経由で
   詳細規約が決定的にロードされるようにする。`AGENTS.md` はクロスツール共有ソースのまま維持。
   `.claude/rules/go-ai-bridge.md` はポインタを外し Claude 固有の追記だけにスリム化。

## File changes

| 種別 | パス                                       | 内容                                                                  |
| ---- | ------------------------------------------ | --------------------------------------------------------------------- |
| 変更 | `.claude/settings.local.json`              | 残骸/重複/古いパス削除（untracked、コミット対象外）                   |
| 削除 | `.claude/skills/skill-creator/`            | はぐれコピー除去                                                      |
| 変更 | `AGENTS.md`                                | Coding/Testing 節をポインタ化、hook/skill/nested CLAUDE.md を相互参照 |
| 新規 | `.claude/settings.json`                    | PreToolUse hook 登録（tracked、追加マージ）                           |
| 新規 | `.claude/hooks/guard-version-pins.sh`      | 版数ピン編集ブロック hook                                             |
| 変更 | `.claude/rules/dockerfile-versions.md`     | hook 強制の旨を相互参照                                               |
| 新規 | `.claude/rules/plan-docs.md`               | `docs/plan/**` 規約                                                   |
| 新規 | `.claude/rules/agent-authoring.md`         | `ai-agents/agents/**` 規約                                            |
| 新規 | `.claude/rules/terraform.md`               | `**/*.tf` 規約                                                        |
| 新規 | `.claude/skills/deploy-ai-config/SKILL.md` | デプロイ手続きスキル                                                  |
| 新規 | `scripts/ai-bridge/CLAUDE.md`              | `@AGENTS.md` import（詳細規約を決定的ロード）                         |
| 変更 | `.claude/rules/go-ai-bridge.md`            | ポインタ削除し Claude 固有の追記だけにスリム化                        |

## Risks and mitigations

- **guard hook の過剰ブロック**: バージョン行に触れない Dockerfile 編集や `update-go-tools.sh`・
  `pyproject.toml` 等は通す設計。7 ケースの手動テストで確認済み。
- **重複 permission 削除でプロンプト増**: グローバル `~/.claude/settings.json` がデプロイ済みの前提。
  重複分はグローバルが引き続き許可するため増えない。
- **nested CLAUDE.md の初回 import 承認ダイアログ**: ルートの `@AGENTS.md` import と同じ挙動、想定内。

## Validation

- guard hook: 7 シナリオ（ARG 版数変更 / 非版数編集 / go-tools.txt / 無関係ファイル / update-go-tools.sh /
  MultiEdit / package.json）で期待 exit を確認。`shellcheck`・`shfmt` クリーン。
- `.claude/settings.json` / `.claude/settings.local.json` を `jq` で JSON 検証。
- 追加 md をリポジトリルートから `markdownlint-cli2` でチェック（0 エラー）。
- `make -n skills-copy` / `make -n claude-link` でデプロイターゲットのドライラン確認。
- `deploy-ai-config` がスキル一覧に登録されることを確認。

## Open questions

- なし（プラン承認済み・実装済み。本ドキュメントは設計記録）。
