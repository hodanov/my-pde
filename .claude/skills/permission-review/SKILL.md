---
name: permission-review
description: >-
  permission-ledger hook が記録した permission prompt と各リポの .claude/settings.local.json から
  allow 昇格候補を集計し、ユーザーが選んだものだけを ai-agents/settings/claude/settings.json に反映する。
  「許可ルールを見直したい」「permission prompt を減らしたい」「承認した操作を allow に入れたい」
  「permission review」などと言われたときに `/permission-review` で使う。
disable-model-invocation: true
metadata:
  version: 1
---

# /permission-review スキル

## Goal

承認してきた操作のうち、ユーザーが選んだものだけをグローバル allow に昇格させる。書き込み先は常にソースの
`ai-agents/settings/claude/settings.json`。`~/.claude/settings.json` は配布されたコピーなので直接編集しない。

## Workflow

### Step 1: 候補を集める

`mise run permission-audit -- --json` を実行する。既定は直近 30 日で、全期間は `--since 0`。
出力フィールドの読み方は [references/review-criteria.md](references/review-criteria.md) の「集計値の読み方」を参照する。

候補が 0 件なら、その旨を伝えて終了する。

### Step 2: 分類する

`candidates` と `unsuggested` の各ルールを references の基準で次の 4 区分に振り分ける。

- **昇格**: そのままグローバルへ
- **一般化して昇格**: 複数の具体ルールを 1 本の prefix ルールにまとめる。まとめた後のルール文字列を示す
- **local のまま**: リポ・パス依存
- **見送り**: 危険・広すぎる・拒否歴あり・二度と一致しない完全一致

`unsuggested` は Claude Code の提案が無かった呼び出しから機械的に起こした下書きなので、そのまま昇格させずに必ず絞り込む。

### Step 3: 選んでもらう

区分ごとの表（ルール / 承認・拒否件数 / リポ数 / 例）を提示し、「昇格」と「一般化して昇格」の候補を
AskUserQuestion の multiSelect で選んでもらう。1 問 4 択が上限なので、候補が多い区分は複数問に分ける。
「local のまま」「見送り」は表で示すだけにし、異論があれば Other で拾う。

### Step 4: 反映する

1. 選ばれたルールを `ai-agents/settings/claude/settings.json` の `permissions.allow` に、既存の並びに合わせて挿入し、`prettier --check` を通す。
2. 提示したが昇格しなかったルール（「local のまま」「見送り」、選ばれなかった候補）を
   `${XDG_STATE_HOME:-$HOME/.local/state}/claude-permission-ledger/declined.txt` に 1 行 1 ルールで追記する。次回から候補に出なくなる。一般化で置き換えた元の具体ルールも入れる。
3. 昇格したルール（一般化の元になったルールを含む）が各リポの `.claude/settings.local.json` に残っていれば、
   ファイルごとに確認を取ってから削除する。

### Step 5: 配布する

`deploy-ai-config` スキルで `~/.claude` に反映する。コミットはユーザーの指示を待つ。

## Notes

- 昇格の可否は必ずユーザーが決める。hook は記録するだけで、自動昇格の経路は持たない（auto mode 相当のリスクを無審査で再現しないため）。
- `declined.txt` はリポジトリ外にある。見送りを取り消すときは、そのファイルから該当行を消す。
- transcript は `cleanupPeriodDays`（既定 30 日）で消えるため、`pending` が多いときは実行間隔が空きすぎている。
