---
name: routine-create-clean-connectors
description: >-
  クラウドルーチン（CCR / スケジュールエージェント）を作成し、作成直後に claude.ai が
  自動付与する不要な MCP connector を判定して除去する一連の処理。
  「ルーチンを作って」「スケジュールエージェントを作成」「routine 作成して connector を整理」等で使う。
  明らかに不要な connector は確認なしで外す。作成は /schedule のスキーマに従う。
disable-model-invocation: true
argument-hint: "[作りたいルーチンの説明 | 既存 trigger_id（cleanup のみ）]"
metadata:
  version: 1
---

# /routine-create-clean-connectors スキル

> ルーチンの「作成」だけなら `/schedule` で足りる。本スキルは **作成 → 自動付与された不要 connector の除去** までを一気通貫で行い、ルーチンに過剰な MCP 権限を残さないことを目的とする。

## Goal

クラウドルーチンを作成し、claude.ai が作成時にデフォルトで紐づける MCP connector のうち、そのルーチンのタスクに**明らかに不要**なものを確認なしで除去する。必要・曖昧なものだけ残す（曖昧時のみユーザーに確認）。

## 背景（なぜ必要か）

- claude.ai でルーチンを作成すると、アカウントに接続済みの connector（Gmail / Google Calendar / Google Drive 等）が**全件デフォルトで紐づく**。
- スキャン系や PR 作成系のように `gh` / WebSearch / WebFetch / Bash だけで完結するルーチンには、これらは不要な権限。作成しっぱなしだと過剰権限が残る。
- そこで「作成直後に connector を棚卸しして外す」までをスキル化する。

## Workflow

### Step 0: ツール読み込み

`ToolSearch` で `select:RemoteTrigger` を読み込む（auth はツール内で処理されるため curl は使わない）。

### Step 1: ルーチン作成

- 引数が **既存 trigger_id** の場合は作成をスキップし Step 2 へ（cleanup のみモード）。
- それ以外は `/schedule` の Create フローに従ってルーチンを作る。
  - リポジトリ内の `routines/*.json` を雛形にする場合、その定義（cron / model / allowed_tools / prompt 配列）を Create body へ変換する。`prompt` 配列は改行で結合して 1 つの文字列にする。
  - Create body の正確なスキーマ（`job_config.ccr.session_context` / `events[].data` / UUID 生成等）は `/schedule` スキルを参照する。重複定義しない。
- `RemoteTrigger {action: "create", body: {...}}` を実行し、レスポンスの `id`（= 採番された trigger_id）と `mcp_connections` を控える。

### Step 2: connector の棚卸し

作成（または get）レスポンスの `mcp_connections` を確認する。

- `[]`（空）なら除去対象なし。Step 4 へ。
- cleanup のみモードのときは `RemoteTrigger {action: "get", trigger_id}` で現在の `mcp_connections` を取得する。

各 connector が**必要か**を、ルーチンの `prompt` と `allowed_tools` から推定する:

- **不要の典型**: prompt にも allowed_tools にもその外部サービスへの言及が無い。
  例: タスクが `gh`(GitHub) / WebSearch / WebFetch / Bash / Read / Grep / Glob だけで完結する → Gmail / Calendar / Drive 等は不要。
- **必要の典型**: prompt がそのサービスの操作を明示している（「カレンダーを確認」「メールを下書き」「Drive のファイルを読む」等）、または対応する MCP ツールを使う前提になっている。

### Step 3: 除去（判定して実行）

- **全 connector が明らかに不要** → 確認なしで全除去:
  `RemoteTrigger {action: "update", trigger_id, body: {"clear_mcp_connections": true}}`
- **一部だけ残す** → 残したいものだけを渡して置換（`mcp_connections` は**置換**セマンティクス。消したいものを省く形で全量を渡す）:
  `RemoteTrigger {action: "update", trigger_id, body: {"mcp_connections": [<残す connector のみ>]}}`
  - 各要素は `{"connector_uuid": "...", "name": "...", "url": "..."}`。`name` は `[a-zA-Z0-9_-]` のみ（ドット・空白不可）。
- **必要 / 不要が曖昧な connector がある** → その connector については外さず、ユーザーに残すか確認する。明らかに不要なものは確認を待たずに先に外してよい。

### Step 4: 確認と報告

- update レスポンスの `mcp_connections` が期待どおり（残すもののみ／空）かを確認する。
- 残した connector・外した connector を一覧で報告する。
- 管理 URL を提示する: `https://claude.ai/code/routines/{trigger_id}`。
- リポジトリの `routines/*.json` 由来で作成した場合は、採番された `trigger_id` を該当 JSON の `PENDING_CREATE` 等のプレースホルダと差し替える（`routines/README.md` の運用に従う）。

## Notes

- **判定の安全側**: 「明らかに不要」= prompt にも tools にも痕跡が無いもの、に限る。少しでも使う可能性があれば残す or 確認する。誤って必要な connector を外すとルーチンが失敗するため。
- **connector が未接続のとき**: ユーザーが connector を追加したい場合は <https://claude.ai/customize/connectors> へ誘導する（このスキルは付与ではなく除去が主目的）。
- **有効な connector の確認方法**: `RemoteTrigger {action: "get", trigger_id}` または `{action: "list"}` のレスポンス `mcp_connections` を見る。Web UI でも確認できる。
- **削除（ルーチン自体）は非対応**: API では消せない。<https://claude.ai/code/routines> から行う。
- Create の詳細スキーマ・cron 変換・環境選択は `/schedule` を正とする。本スキルはそこに connector cleanup を上乗せするもの。
