---
name: scan-issue-triage
description: >-
  スキャン Routine が起票した `scan:*` Issue の採否を、前提の裏取りと壁打ちで決め、決定を記録する
  （不採用は理由コメント + `rejected` + Close、採用は `adopted`）。
  Issue の URL や番号とともに「採否を判断したい」「この issue どう思う？」「rejected にして閉じて」
  「adopted にして」などと言われたときに `/scan-issue-triage <Issue URL | 番号>` で使う。
argument-hint: "<Issue URL | 番号>"
metadata:
  version: 1
---

# /scan-issue-triage スキル

## Goal

`scan:*` Issue 1 件の採否を、数字で裏取りした根拠をもとにユーザーと決める。決定は、次のスキャンと `Monthly Routine Improve` が読める形でラベルとコメントに残す。

ラベルの意味と運用ルールは `routines/README.md` の「採用判定」に従う。

## Workflow

### Step 1: Issue を読んで説明する

- `gh issue view <番号> --json title,body,labels,comments,state` で読む
- 「何が困っていて、何を変える提案か」を、背景から平易に説明する。Issue の構成（課題 / 変更 / リスク）をそのままなぞらない

### Step 2: 前提を裏取りする

スキャンの Issue は、件数や頻度を誇張していることがある（#831）。推奨を出す前に確かめる。

- Issue が挙げる件数・頻度・「既存の手段では足りない」という主張を、`git log` / `gh run` / `gh api` で確かめる。合っていた点と、誇張または誤りだった点を分けて示す
- 問題が実際に起きた件数を数える（例: main への直コミット、PR 外で失敗した run の中身）
- 既存の防御で足りていないかを確かめる（ブランチ保護、permission、既存の hook / skill）
- 過去の決定と照らし合わせる: `docs/adr/`、memory、`gh issue list --state closed --label rejected` で見つかる同じ系統の不採用 Issue

### Step 3: 推奨して壁打ちする

- 入れる理由と見送る理由を短く並べ、推奨を 1 つ出す。採用を勧めるなら、実装時の注意点（誤爆、fail open など）も添える
- **ユーザーが採否を明言するまで、ラベル・コメント・Close のどれもしない**

### Step 4: 決定を記録する

**不採用**: コメントを付け、`rejected` ラベルを付けて、`--reason "not planned"` で Close する。ここまでを続けて行う。`rejected` を後から付けると、却下までのリードタイムが実際より長く見えるため。

コメントの型:

- 冒頭 1 行に結論（「見送り。」）
- 前提の確認: 合っていた点と、誇張または誤りだった点
- 見送る理由: 箇条書き。同じ判断をした過去の Issue があれば番号を添える
- 再検討の条件: どんな実害が出たら考え直すか

**採用**: `adopted` ラベルを付け、Open のまま残す。壁打ちで実装方針を直したなら、直した点をコメントに残す。PR Bot は Issue 本文とコメントを読んで実装する。

コメント本文は `$TMPDIR` のファイルに書き、`--comment "$(cat <file>)"` で渡す。

### Step 5: memory に残す

- 次回の判断に効く事実を project memory に 1 件書き、`MEMORY.md` に 1 行の索引を足す。本文には、決め手になった数字（Why）と、同じ種類の提案が来たら最初に何を見るか（How to apply）を書く
- 同じ話題の memory が既にあれば、新しく作らずにそちらを更新する

## Notes

- 不採用のコメントは `Monthly Routine Improve` がスキャンのプロンプトを直す材料として読む。「不要」だけで終わらせず、次のスキャンが同じ角度の提案を避けられる粒度で書く
- 1 回の起動で扱う Issue は 1 件。続けて別の Issue を渡されたら、Step 1 からやり直す
