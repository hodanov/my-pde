---
name: plan-issue-export
description: AIとの壁打ちで整理したプラン、または指定ファイルの内容を GitHub Issue として作成する。セッション内のプランの Issue 化、既存プランファイルからの Issue 作成、config によるデフォルト値設定が必要なときに使う。
argument-hint: [source-file-or-request]
disable-model-invocation: true
metadata:
  version: 1
---

# Plan Issue Export

AIとの壁打ちで整理したプラン、または既存のプランファイルを、GitHub Issue として作成する。

## 想定する使い方

- `/plan-issue-export 提案してくれたプランを Issue にして`
- `/plan-issue-export さっき整理した実装案を Issue にして`
- `/plan-issue-export docs/plan/2026-03-10_plan-issue-export.md を Issue にして`

## 入力の扱い

- 引数にファイルパスや明示的な入力元が含まれる場合は、その内容を優先して使う
- 引数に具体的な入力元が無い場合は、このセッション内で直近に提案・整理・合意したプラン内容を入力として使う
- セッション内に十分なプラン内容が無い場合は、推測で埋めず、不足情報を確認する

## 原則

- 元プランの意図・判断・前提・リスク・検証項目を落とさない
- 断片的な議論を、そのまま起票できるレビュー可能な粒度の Issue に再構成する
- 表現だけ整え、要件や設計判断を勝手に追加しない（詳細は [../plan-markdown-export/normalize-rules.md](../plan-markdown-export/normalize-rules.md)）
- 必要に応じて GitHub Flavored Markdown の表、箇条書き、チェックリスト、コードブロックに変換する

## Config

`${CLAUDE_SKILL_DIR}/config.yaml` でデフォルト値を設定できる。雛形は [config.example.yaml](config.example.yaml) を参照。

config.yaml が存在する場合はデフォルト値として読み込み、引数やユーザー指示で上書きできる。config.yaml が存在しない場合は、必須項目（repository）をユーザーに確認する。

### Config フィールド

| フィールド   | 必須 | 説明                                                                    |
| ------------ | ---- | ----------------------------------------------------------------------- |
| `repository` | Yes  | Issue 作成先リポジトリ（`owner/repo` 形式）                             |
| `project`    | No   | GitHub Projects のプロジェクト番号（Sprint/Status/Estimate 設定に必要） |
| `sprint`     | No   | Sprint 値。`current` を指定した場合は現在の iteration を自動取得する    |
| `status`     | No   | Issue の Status フィールドに設定する値                                  |
| `estimate`   | No   | Issue の Estimate フィールドに設定する値                                |

## 正規化ルール

[../plan-markdown-export/normalize-rules.md](../plan-markdown-export/normalize-rules.md) を参照。

## 手順

1. 入力元を特定する
2. 会話中のプラン、または指定ファイルから、タイトル・背景・方針・実装手順・リスク・検証項目を抽出する
3. `${CLAUDE_SKILL_DIR}/config.yaml` が存在すれば読み込み、デフォルト値を取得する
4. `repository` が未設定の場合はユーザーに確認する
5. Title を決定する — プラン内容の h1 見出し（`# Plan: ...` の `...` 部分）を使う
6. Description を [template.md](template.md) ベースで整形する
7. `gh auth status` で認証状態を確認する（未認証なら案内して中断）
8. `gh issue create --repo <repository> --title <title> --body <body>` で Issue を作成する
9. config に `project` が設定されている場合、以下の Project フィールドを設定する:
   - Sprint: `sprint: current` の場合は `gh project item-list` から現在の iteration を取得して設定する。固定値の場合はそのまま設定する
   - Status: 値が指定されていれば設定する
   - Estimate: 値が指定されていれば設定する
   - Project フィールドの設定には `gh project item-edit` を使う
10. 作成した Issue の URL を返す

## テンプレート

出力形式は [template.md](template.md) を参照。

- 入力に `Current structure` や `Design policy` が無い場合は、該当セクションを省略してよい
- `File changes` と `Risks and mitigations` は、情報が表形式に向いている場合は表を優先する
- 会話ベースのプラン出力では、文脈上明らかな内容だけを補完してよいが、未合意事項は `Open questions` に残す

## 注意

- `gh` CLI が未認証の場合は `gh auth login` を案内して中断する
- リポジトリが存在しない場合はエラーメッセージを返す
- config.yaml が存在しない場合でも、必須項目をユーザーに確認すれば動作する
- Issue body が極端に長い場合は、要約版を body に入れ、詳細はコメントとして追加することを検討する
- Sprint の自動取得（`sprint: current`）が失敗した場合はスキップし、ユーザーに通知する
- 入力元が曖昧な場合は、どのプランを Issue にするか確認する
