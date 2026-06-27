---
name: review
description: >-
  コード変更をセキュリティ・パフォーマンス・正確性・変更容易性の4観点から並列レビューし、統合レポートを出力する。
  小〜中規模の変更を素早くチェックしたいときに使う。
  大きなPRや複雑な変更を丁寧にレビューしたい場合は /review-scan を使う。
disable-model-invocation: true
argument-hint: "[PR番号 | ブランチ名 | staged]"
metadata:
  version: 1
---

# /review スキル

> 組み込みの `/code-review`（高速・単発の差分レビュー）で足りる軽いチェックはそちらを優先する。本スキルは4観点の専門サブエージェントで構造化レポートが欲しいときに使う。

## Goal

コード変更を4つの専門レビューサブエージェントで並列レビューし、統合 Review Report を出力する。

## Workflow

### Step 1: diff 取得

引数に応じて diff を取得する:

- **PR番号** (例: `123`, `#123`): `gh pr diff <番号>` を実行
- **ブランチ名** (例: `feature/xxx`): `git diff main...<ブランチ名>` を実行
- **`staged`**: `git diff --staged` を実行
- **引数なし**: `git diff --staged` を実行（デフォルト）

diff が空の場合はユーザーに通知して終了する。

### Step 2: 4つのサブエージェントを並列起動

Agent tool を使い、**同一レスポンス内で4つ全てを並列起動**する。各エージェントには Step 1 で取得した diff 全文を渡す。

起動する4エージェント:

| Agent                | subagent_type        | 観点           |
| -------------------- | -------------------- | -------------- |
| review-security      | review-security      | セキュリティ   |
| review-performance   | review-performance   | パフォーマンス |
| review-correctness   | review-correctness   | 正確性         |
| review-changeability | review-changeability | 変更容易性     |

各エージェントへのプロンプトは以下の形式:

```text
以下の diff をレビューしてください。

<diff>
{diff内容}
</diff>
```

### Step 3: 結果収集と統合 Review Report 生成

4つのサブエージェントから Findings Report を受け取ったら、以下のフォーマットで統合 Review Report を出力する:

```markdown
## Review Report

### 総評

(全体評価 2-4文。4つの観点の所見を統合した総合判断。)

### セキュリティ

(review-security の結果サマリ + 指摘。指摘なしの場合は「指摘なし」と記載。)

### パフォーマンス

(review-performance の結果サマリ + 指摘。指摘なしの場合は「指摘なし」と記載。)

### 正確性

(review-correctness の結果サマリ + 指摘。指摘なしの場合は「指摘なし」と記載。)

### 変更容易性

(review-changeability の結果サマリ + 指摘。指摘なしの場合は「指摘なし」と記載。)

### Phase.2 推奨

(Critical/Warning があれば code-review-critic での深掘りを推奨。なければ「Phase.2 不要」と記載。)

### 判定

- [ ] Approve — 問題なし、マージ可能
- [ ] Approve with comments — 軽微な指摘あり、修正推奨だがマージ可能
- [ ] Request changes — 修正が必要、再レビュー推奨
```

### Step 4: Phase.2 ハンドオフ推奨

いずれかのエージェントが Critical または Warning レベルの指摘を出した場合、統合レポートの「Phase.2 推奨」セクションに以下を記載する:

- Phase.2 深掘りが推奨される具体的な指摘事項
- `code-review-critic` エージェントの起動方法の案内

注: Phase.2 の実行はユーザーの判断に委ねる。自動起動はしない。

## Notes

- `code-review-scanner` / `code-review-critic` パイプラインは `/review-scan` スキルで呼び出せる
- サブエージェントは全て `model: sonnet` で動作し、コスト効率を重視している
- 4エージェントは読み取り専用（`permissionMode: plan`）なので、コードを変更しない
