---
name: skill-observe
description: >-
  スキル使用後の observation（結果・問題・フィードバック）を記録する。
  スキルの改善サイクル（Observe → Inspect → Amend → Evaluate）の起点。
  使用後に `/skill-observe <スキル名> <結果> [問題/フィードバック]` で呼び出す。
argument-hint: "<スキル名> <success|partial|failure> [問題やフィードバック]"
metadata:
  version: 2
---

# /skill-observe スキル

## Goal

スキル使用後の結果を構造化された observation として記録し、将来の `/skill-improve` による分析・改善の材料を蓄積する。

## Workflow

### Step 1: 引数パース

`$ARGUMENTS` を以下のように解釈する:

- **第1引数**: スキル名（例: `review`, `commit-and-draft-pr`, `investigate`）
- **第2引数**: 結果（`success` / `partial` / `failure`）
- **第3引数以降**: 問題やフィードバック（自由記述、省略可）

引数なしの場合は **バッチモード** に入る（Step 5 参照）。

スキル名のみで結果が省略されている場合、ユーザーに結果を尋ねる。

### Step 2: スキル存在確認

`ai-agents/skills/<スキル名>/SKILL.md` が存在するか確認する。
存在しない場合、ユーザーにスキル名が正しいか確認する。

### Step 3: observation の構築

以下の 6 フィールドで observation エントリを構築する:

| フィールド     | 取得元                                                    |
| -------------- | --------------------------------------------------------- |
| タスク         | 会話コンテキストから推測、不明なら「（未記載）」          |
| スキル         | 第1引数                                                   |
| 結果           | 第2引数（success / partial / failure）                    |
| 問題           | 第3引数以降。なければ「なし」                             |
| フィードバック | 第3引数に含まれるユーザー所感。分離が難しければ問題と統合 |
| コンテキスト   | 会話から判断できる詳細（リポジトリ、言語、規模など）      |

エントリのフォーマット:

```markdown
---
skill: <スキル名>
date: YYYY-MM-DD
---

# Observation: <スキル名> — YYYY-MM-DD_NNN

- **タスク**: ...
- **スキル**: ...
- **結果**: success | partial | failure
- **問題**: ...
- **フィードバック**: ...
- **コンテキスト**: ...
```

### Step 4: ファイルへの書き込み

保存先: `ai-agents/skills/<スキル名>/observations/YYYY-MM-DD_NNN_obs.md`（NNN は同日の連番、001 から開始。既存ファイルと重複しないようインクリメントする）

- ディレクトリが存在しなければ作成する
- 1 observation につき 1 ファイルを新規作成する（追記はしない）

書き込み完了後、記録内容をユーザーに表示して確認する。

### Step 5: バッチモード（引数なしの場合）

引数なしで呼ばれた場合:

1. 現在の会話でスキルを使用した形跡を確認する
2. 使用したスキルごとに結果を尋ねる
3. 各スキルについて Step 3-4 を実行する
4. 使用スキルが見つからない場合、手動入力を促す

## Notes

- observation は Stop hook（`ai-agents/settings/claude/hooks/skill-observe-nudge.sh`）が**自動記録**する。リポジトリスキルを使ったセッション終了時に、Claude がその場の文脈から結果を判定して observation を書き出す。本スキルは手動・補足記録およびバッチモード（引数なし）用として残る
- 保存先は環境変数 `SKILL_OBSERVE_HOME`（my-pde のクローンパス。`settings.json` の `env` で `~/workspace/my-pde` を設定。先頭の `~` は hook 側で `$HOME` に展開するので別 PC でも動く）で決まる。これにより**どのリポジトリで作業していてもスキル使用が my-pde の git 管理下に集約**される。`SKILL_OBSERVE_HOME` 未設定時は現在の git リポジトリ直下の `ai-agents/skills` にフォールバックし、それも無ければ何もしない
- observation ファイルは git 管理下に置く。`mise run claude-skills-copy` でローカル環境にもコピーされる
- `/skill-improve` がこれらの observation を分析し、SKILL.md の改善提案を生成する
- 1 日に複数回同じスキルの observation を記録しても問題ない（連番で区別）
- observation は事実ベースで記録する。主観的評価は「フィードバック」フィールドに集約する
