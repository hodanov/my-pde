---
name: plan-markdown-export
description: AIとの壁打ちで整理したプラン、または別ファイルの設計メモや実装メモを、読みやすいMarkdown形式の実装プランに整形して `docs/plan/YYYY-MM-DD_<plan-name>.md` に保存する作業で使う。会話中に提案したプランの書き出し、テキストメモの構造化、見出し整理、表への変換、チェックリスト化、保存が必要なときに使う。
argument-hint: [source-file-or-request]
disable-model-invocation: true
metadata:
  version: 1
---

# Plan Markdown Export

AIとの壁打ちで整理したプラン、または既存のメモを、レビューしやすい Markdown の実装プランに整形して保存する。

## 想定する使い方

- `/plan-markdown-export 提案してくれたプランを出力して`
- `/plan-markdown-export さっき整理した実装案を markdown にして`
- `/plan-markdown-export /path/to/memo.txt をベースにプラン化して`

## 入力の扱い

- 引数にファイルパスや明示的な入力元が含まれる場合は、その内容を優先して使う
- 引数に具体的な入力元が無い場合は、このセッション内で直近に提案・整理・合意したプラン内容を入力として使う
- セッション内に十分なプラン内容が無い場合は、推測で埋めず、不足情報を確認する

## 原則

- 元プランの意図・判断・前提・リスク・検証項目を落とさない
- 断片的な議論を、レビュー可能な粒度の実装プランに再構成する
- 表現だけ整え、要件や設計判断を勝手に追加しない（詳細は [normalize-rules.md](normalize-rules.md)）
- 必要に応じて GitHub Flavored Markdown の表、箇条書き、チェックリスト、コードブロックに変換する

## 手順

1. 入力元を特定する
2. 会話中のプラン、または指定メモから、タイトル・背景・方針・実装手順・リスク・検証項目を抽出する
3. プラン名を決める。タイトルから短い ASCII の kebab-case を作る（指定がある場合はそれを使う）
   - 例: "WezTerm CLI の操作性改善" → `wezterm-cli-comfort`
4. `docs/plan/` が無ければ作成する
5. 生成先パス `docs/plan/YYYY-MM-DD_<plan-name>.md` を決める（日付はローカル日付）。既存ファイルがあれば上書き前に確認する
6. [template.md](template.md) をベースに Markdown を出力する
7. 生成ファイルのパスを返す

## 正規化ルール

[normalize-rules.md](normalize-rules.md) を参照。

## テンプレート

出力形式は [template.md](template.md) を参照。

- 入力に `Current structure` や `Design policy` が無い場合は、該当セクションを省略してよい
- `File changes` と `Risks and mitigations` は、情報が表形式に向いている場合は表を優先する
- 会話ベースのプラン出力では、文脈上明らかな内容だけを補完してよいが、未合意事項は `Open questions` に残す

## 注意

- 入力元が曖昧な場合は、どのプランを出力するか確認する
