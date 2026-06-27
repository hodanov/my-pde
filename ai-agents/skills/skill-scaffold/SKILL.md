---
name: skill-scaffold
description: >-
  新しいスキルの雛形（ディレクトリ + SKILL.md）を本リポジトリの規約どおりに生成する。
  frontmatter（name / description / 任意の argument-hint・disable-model-invocation・metadata.version）と
  本文（# /SKILL_NAME スキル → Goal / Workflow / Notes）を対話的に埋め、
  ai-agents/skills/SKILL_NAME/SKILL.md に配置する。
  新しいスキルを新規作成・立ち上げたいときに `/skill-scaffold <スキル名> [用途の一言]` で呼び出す。
disable-model-invocation: true
argument-hint: "<スキル名> [用途の一言]"
metadata:
  version: 1
---

# /skill-scaffold スキル

## Goal

規約準拠のスキル雛形（ディレクトリ + SKILL.md）を最小手数で生成し、`skill-improve` が扱いやすい綺麗な初期状態を作る。
改善サイクル（Observe → Inspect → Amend → Evaluate）の手前にある「Create（新規スキルの立ち上げ）」フェーズを担う。

## Workflow

### Step 1: 引数パース

`$ARGUMENTS` を以下のように解釈する:

- **第1引数**: 生成するスキル名（以降 `SKILL_NAME`）
- **第2引数以降**: そのスキルの用途の一言（自由記述、省略可）

引数なしの場合は対話でスキル名と用途を尋ねる。

### Step 2: スキル名のバリデーション

`SKILL_NAME` が以下を満たすか検証する。満たさない場合は修正案を提示して確認する。

- lowercase + ハイフン（`a-z`, `0-9`, `-`）のみ
- 64 文字以内
- 予約語（`claude` / `anthropic`）を含まない

### Step 3: 衝突チェック（最重要）

`ai-agents/skills/SKILL_NAME/` が **存在しないこと** を必ず確認する。

- 既存の場合は **作成せず中断** し、既存スキルへの追記または `/skill-improve` を案内する（既存 SKILL.md を絶対に上書きしない）。

### Step 4: description のドラフト

ベストプラクティスに沿った `description` をドラフトしてユーザー確認を取る。

- **「何をするか」＋「いつ使うか（トリガー）」** を必ず両方含める（モデルの自動起動判断に必要）。
- 1024 文字以内、簡潔に。

### Step 5: frontmatter の確定

- `argument-hint`（引数を取るスキルのみ）と `disable-model-invocation`（明示呼び出し専用にするか）の要否を確認する。
- `metadata.version: 1` を付与する。

### Step 6: 本文の生成

`# /SKILL_NAME スキル` を見出しに、`## Goal` / `## Workflow` / `## Notes` の骨組みを生成する。

- 各セクションはプレースホルダ＋最小限の骨組みに留め、中身の作り込みはユーザーに委ねる（SKILL.md は簡潔さが重要）。

### Step 7: 書き出しとデプロイ案内

`ai-agents/skills/SKILL_NAME/SKILL.md` に書き出し、内容をユーザーに表示して確認する。

- デプロイは自動実行せず、`make skills-copy` で 4 エージェント（codex / claude / cursor / copilot）へ配布される旨を案内するに留める。

## Notes

- **既存スキルの上書き防止が最重要**。Step 3 の非存在確認を必ず先に行い、衝突時は作成せず中断する。
- 本スキルは「新規作成（Create）」専任。既存スキルの改善（Inspect / Amend）には踏み込まず、`/skill-improve` と役割を分担する。
- skills は 4 エージェントへ配布されるため、特定エディタ依存の機能やスクリプトは使わず、モデル駆動の純 Markdown 手順に留める（移植性確保）。
- 新規 skill のため hook 配線（settings.json / 3 エディタ分の `.sh`）は不要。`scripts/copy-entries.sh` の skills モードが `skills/` 直下を総当りでコピーするため、ディレクトリを置くだけで `make skills-copy` にそのまま乗る。
- 雛形は最小限に留め、過剰生成を避ける。スコープを「スキル作成」に絞り、hook 雛形生成などへ広げない。
