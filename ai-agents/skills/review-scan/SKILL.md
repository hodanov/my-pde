---
name: review-scan
description: >-
  code-review-scanner と code-review-critic による2フェーズレビュー。
  大きなPRや複雑な変更を丁寧にレビューしたいときに使う。
  scanner がトリアージして重要な指摘だけ critic が深掘りする。
  軽いチェックには /review（4並列専門レビュー）を使う。
disable-model-invocation: true
argument-hint: "[PR番号 | ブランチ名 | staged]"
metadata:
  version: 1
---

# /review-scan スキル

> 軽い差分チェックは組み込みの `/code-review`、4観点の並列レビューは `/review` を使う。本スキルは大きなPR・複雑な変更を scanner→critic で丁寧に掘りたいときに使う。

## Goal

code-review-scanner（Phase 1）で変更を広くスキャンし、
code-review-critic（Phase 2）で高優先の指摘を深掘りする2段階レビュー。

## Workflow

### Step 1: diff 取得

引数に応じて diff を取得する:

- **PR番号** (例: `123`, `#123`): `gh pr diff <番号>` を実行
- **ブランチ名** (例: `feature/xxx`): `git diff main...<ブランチ名>` を実行
- **`staged`**: `git diff --staged` を実行
- **引数なし**: `git diff --staged` を実行（デフォルト）

diff が空の場合はユーザーに通知して終了する。

### Step 2: Phase 1 — code-review-scanner

Agent tool で `code-review-scanner` を起動する。

- `subagent_type`: `code-review-scanner`
- プロンプト:

```text
以下の diff をレビューしてください。

<diff>
{diff内容}
</diff>
```

scanner から **Scan Report**（優先度付きの指摘リスト）を受け取る。

### Step 3: Scan Report 提示

scanner の生出力（Scan Report）をそのままユーザーに表示する。再フォーマットは行わない（トークン節約 + critic への情報ロスを防ぐため）。

### Step 4: Phase 2 — code-review-critic

scanner の Scan Report を受け取ったら、Critical または Warning レベルの指摘がある場合、 Agent tool で `code-review-critic` を起動する。

- `subagent_type`: `code-review-critic`
- プロンプトには Scan Report 生出力の全文と Step 1 で取得した diff 全文の両方を含める:

```text
以下の Scan Report と diff を基に深掘りレビューしてください。

<scan-report>
{scanner の生出力全文}
</scan-report>

<diff>
{diff内容}
</diff>
```

critic から受け取った結果を出力する。

Critical/Warning がない場合は Phase 1 の結果のみで完了とする。

## Notes

- 日常的な軽いチェックには `/review`（4並列専門レビュー）が適している
- `/review` との使い分け: 大きなPR・複雑な変更 → `/review-scan`、小〜中規模の変更 → `/review`
