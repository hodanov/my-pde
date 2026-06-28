---
name: investigate
description: >-
  investigation-scout と investigation-diver による2フェーズ調査。
  不具合の原因調査やコード探索など、未知の領域を掘り下げたいときに使う。
  scout が候補を広く洗い出し、diver が高優先の候補を深掘りする。
disable-model-invocation: true
argument-hint: "<調査対象の説明>"
metadata:
  version: 1
---

# /investigate スキル

## Goal

investigation-scout（Phase 1）でコードベースを広くスキャンし候補を洗い出し、
investigation-diver（Phase 2）で高優先の候補を深掘りする2段階調査。

## Workflow

### Step 1: 引数バリデーション

`$ARGUMENTS` が空の場合、ユーザーに調査対象を尋ねて終了する。

### Step 2: Phase 1 — investigation-scout

Agent tool で `investigation-scout` を起動する。

- `subagent_type`: `investigation-scout`
- プロンプト:

```text
以下の内容を調査してください。

<investigation-target>
{$ARGUMENTS}
</investigation-target>
```

scout から **Scout Report**（優先度付きの候補リスト）を受け取る。

### Step 3: Scout Report 提示

scout の生出力（Scout Report）をそのままユーザーに表示する。
再フォーマットは行わない（トークン節約 + diver への情報ロスを防ぐため）。

### Step 4: Phase 2 判定 → investigation-diver

Scout Report に **High 優先度の候補がある場合**、
Agent tool で `investigation-diver` を起動する。

- `subagent_type`: `investigation-diver`
- プロンプトには **Scout Report 生出力の全文** と **元の調査対象** の両方を含める:

```text
以下の Scout Report と調査対象を基に深掘り調査してください。

<scout-report>
{scout の生出力全文}
</scout-report>

<investigation-target>
{$ARGUMENTS}
</investigation-target>
```

diver から受け取った Diver Report をそのままユーザーに表示する。

High 候補がない場合は Phase 1 の結果のみで完了とし、ユーザーに通知する。

## Notes

- 直接 `investigation-scout` / `investigation-diver` を指示しても同等の結果は得られる
- このスキルの価値は「調査パイプラインの標準化・ショートカット化」にある
