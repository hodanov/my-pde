---
name: permission-prompt-tuner
description: >-
  permission prompt が出た（自動許可されなかった）Bash コマンドを記録し、
  settings.json をどう調整すればプロンプトを減らせるかを提案する。
  Stop hook（permission-prompt-nudge.sh）が検出バッファ非空時に自動起動する。
metadata:
  version: 1
---

# /permission-prompt-tuner スキル

## Goal

permission prompt が出たコマンドの原因を分類し、`settings.json` への追記案（または
コマンド書き換え案）を提示して記録に蓄積する。設定の適用はしない（提案のみ）。

## Workflow

### Step 1: 検出バッファを読む

Stop hook の reason に記載された検出バッファ（JSONL、1 行 1 コマンド）を読む。各行は
`{ts, command, cwd, causes:[{segment, kind}]}`。バッファが無い/空なら何もせず終了する。

手動起動でバッファパスが不明な場合は
`${TMPDIR:-/tmp}/claude-permission-prompt-tuner/<session_id>.jsonl` を探す。

### Step 2: 原因を分類する

各コマンドの `causes[].kind` を `references/prompt-causes.md` に照らして解釈する。1 コマンドが
複数 kind を持つことがある（例: 複合コマンドの未許可セグメント＋cd+redirection）。

- `missing-allow` — allow にマッチしないセグメント
- `prefix-break` — `terraform -chdir=...` 等で前方一致が外れる
- `builtin-cd-redirect` — `cd`＋出力 redirection の組み込みガード（allow では解決不可）
- `deny-hit` — deny にマッチ（意図的遮断の可能性）

### Step 3: 推奨アクションを決める

kind ごとに `references/prompt-causes.md` の対応表に従って決める。要点:

- `missing-allow` → `Bash(<tool> *)` を allow に追加。**スコープ判断**: 汎用的で無害なコマンドは
  グローバル（`~/.claude/settings.json`）、リポ固有・パス依存は project（`.claude/settings.local.json`）。
  `cat` は Read ツール推奨のため allow 追加を勧めない（コマンド側を Read に変える提案）。
- `prefix-break` → 具体パターン（例 `Bash(terraform -chdir=* show *)`）追加、または `-chdir` を
  使わない書き換え、の二択を提示。
- `builtin-cd-redirect` → allow では消せない。**コマンド書き換え**（`cd` を使わず絶対パスで
  redirect する）を提示。
- `deny-hit` → 意図的 deny の可能性が高い。allow 追加は勧めず、なぜ deny されているかを確認するよう促す。

### Step 4: records に蓄積する

Stop hook の reason に記載された記録先ディレクトリ（絶対パス）に、Write ツールで
`YYYY-MM-DD_NNN.md` を新規作成する（NNN は当日連番、001 開始、既存と重複しないよう採番）。
ディレクトリが無ければ作成する。1 バッチ（今回の検出分）につき 1 ファイル。形式:

```markdown
---
skill: permission-prompt-tuner
date: YYYY-MM-DD
---

# Permission Prompt Record — YYYY-MM-DD_NNN

## <コマンドの1行要約>

- **コマンド**: `<full command>`
- **原因**: <kind と、なぜプロンプトが出たかの説明>
- **推奨**: <allow 追記 JSON / 書き換え案>
- **対象**: <追記先ファイルとスコープ、または「コマンド書き換え」>
- **コンテキスト**: cwd: <cwd>
```

### Step 5: 提案を提示してバッファを空にする

- 記録した内容の提案サマリをチャットに簡潔に提示する（settings.json は編集しない）。
- 検出バッファを空にする（同じプロンプトの再通知を防ぐため）。Write ツールで空文字列を書き込む
  （Bash を使うと新たなプロンプトを誘発しうるため避ける）。
- 作成したファイルを 1 行で報告して終了する。ユーザーへの確認は不要。

## Notes

- 検出は Claude Code の許可判定の**近似**（quoting / `$(...)` の厳密解析はしない）。誤検出は
  「余計な提案が 1 件出る」程度で無害。判断に迷うケースは records に事実だけ残す。
- 記録先は環境変数 `SKILL_OBSERVE_HOME`（`~` は hook 側で `$HOME` に展開）で決まる。未設定時は
  現在の git リポジトリ直下の `ai-agents/skills` にフォールバックする（[[skill-observe]] と同じ機構）。
- 設定の適用（settings.json 追記）はユーザーが行う。必要なら [[update-config]] を案内する。
- markdownlint を通すため、既存 records ファイルの体裁に合わせる。
