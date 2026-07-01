---
name: deploy-ai-config
description: ai-agents/ と dotfiles/ の編集内容を各 AI CLI（~/.claude, ~/.cursor, ~/.codex, ~/.copilot）と ~/.config へ反映するデプロイ手順。「設定を反映」「デプロイ」「~/.claude に配って」「スキル/エージェント/設定を更新したから配布」等を求められたときに使用する。
metadata:
  version: 1
---

# Deploy AI config

このリポジトリ（my-pde）が AI CLI 設定とドットファイルの source of truth。`ai-agents/` や `dotfiles/`
を編集したら、このスキルで配布物を各ツールへ反映する。全 make ターゲットはリポジトリルートから実行する
（ルート Makefile が `ai-agents/Makefile` へ委譲している）。

## 何をどのターゲットで配るか

| 編集した場所                                         | 反映ターゲット                                         | 配布先                                       |
| ---------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------- |
| `ai-agents/agents.xml`                               | `make claude-link`（Cursor/Codex/Copilot は `*-link`） | `~/.claude/CLAUDE.md`（symlink）             |
| `ai-agents/skills/**`                                | `make skills-copy`                                     | 各 CLI の `skills/`（全 CLI 一括）           |
| `ai-agents/agents/**`                                | `make agents-copy`                                     | 各 CLI の `agents/`（Claude/Cursor/Copilot） |
| `ai-agents/settings/**`（hooks/rules/settings.json） | `make settings-copy`                                   | 各 CLI のルート（Claude/Cursor/Copilot）     |
| `dotfiles/wezterm/**`                                | `make dotfiles-link`                                   | `~/.config/wezterm`（symlink）               |

`*-copy` は実体コピー（編集ごとに再実行が必要）。`*-link` / `dotfiles-link` は symlink（一度貼れば追従）。

## 手順

1. `git status` で何を編集したかを確認し、上表から必要なターゲットだけ選ぶ。
2. ドライランで配布先を確認: `make -n skills-copy` 等（特に初回や配布先を確認したいとき）。
3. 実行: 該当ターゲットを `make <target>` で実行。複数同時なら `make skills-copy agents-copy settings-copy`。
4. Claude 設定（hooks/rules/settings.json）を変えた場合は、反映を効かせるため Claude Code セッションの
   再読み込みが必要（`/memory` でルール、`/hooks` 相当でフック状態を確認）。

## 検証

- `*-copy` 後: 配布先（例 `ls ~/.claude/skills`, `ls ~/.claude/hooks`）に最新が入ったか確認。
- `*-link` 後: `ls -l ~/.claude/CLAUDE.md` / `ls -l ~/.config/wezterm` が本リポジトリを指す symlink か確認。
- hooks を変えたら、対象ファイルを 1 つ編集して PostToolUse フォーマッタ／Stop linter が想定通り動くか確認。

## 注意

- このリポジトリ直下の `.claude/`（rules・settings.json・guard hook）は **このリポジトリ専用** で、
  デプロイ対象ではない。配布されるのは `ai-agents/` と `dotfiles/` 配下のみ。
- バージョンピン（`environment/**`）はこのスキルの対象外。`guard-version-pins.sh` で保護されている。
