---
name: trip-log-from-calendar
description: Google カレンダー primary の予定から旅記録 Markdown を生成し `docs/travel/YYYY-MM-DD_<slug>.md` に保存する。期間指定、任意のタイトルキーワードでフィルタ可能。出力前に抽出結果の承認ステップを必ず挟む。
disable-model-invocation: "true"
argument-hint: "[旅の情報 例: 2026-04-18 から 2026-04-19 の佐賀旅行をまとめて]"
---

# Trip Log From Calendar

旅行期間のカレンダー予定を読み込み、ブログ下書きとしてレビューしやすい旅記録 Markdown を出力する。

## 想定する使い方

- `/trip-log-from-calendar 2026-04-18 から 2026-04-19 の佐賀旅行をまとめて`
- `/trip-log-from-calendar 先週末の旅行を旅記録にして`

## 入力

- 対象期間: 開始日〜終了日（必須、`YYYY-MM-DD`）
- 任意のタイトルキーワード（例: 「佐賀」「旅行」）
- 任意の旅テーマ（例: 「SVリーグチャンピオンシップ観戦」）
- slug（未指定なら目的地やテーマから kebab-case で自動生成）

期間が不明な場合は推測せずユーザーに確認する。

## 前提

- Google Calendar MCP が利用可能（`mcp__claude_ai_Google_Calendar__*`）
- 参照カレンダーは primary のみ
- タイムゾーンは `Asia/Tokyo`

## 手順

1. 対象期間と slug を確定する。曖昧ならユーザーに確認する。
2. `mcp__claude_ai_Google_Calendar__list_events` で対象期間の予定を取得する（`calendarId: "primary"`、`timeZone: "Asia/Tokyo"`）。
3. 必要に応じて `mcp__claude_ai_Google_Calendar__get_event` で詳細を取得する。
4. 取得結果から以下を抽出する。
   - 日付、イベントタイトル、場所、開始/終了時刻、説明欄のメモ
   - 交通機関系（🚄 プリフィックスのイベント等）は「移動」としてグルーピング
   - 宿泊系（🏨 プリフィックスのイベント等）は「宿泊」としてグルーピング
   - その他のイベントは「訪問/アクティビティ」としてグルーピング
5. **抽出したイベント一覧をユーザーに表で提示**し、出力して良いか承認を得る。プライベート予定は除外指定を受け付ける。
6. [templates/travel-log.md](templates/travel-log.md) に沿って Markdown を組み立てる。
7. `docs/travel/` が無ければ作成する。
8. ローカル日付の `YYYY-MM-DD_<slug>.md`（開始日ベース）で保存する。既存ファイルがある場合は上書き前に確認する。
9. `markdownlint-cli2 --fix <file>` を実行する（利用可能な場合）。
10. 生成ファイルのパスを返す。

## 出力規約

- 出力先: `docs/travel/YYYY-MM-DD_<slug>.md`
- 見出しは Day 単位で切る（1 日 1 セクション）
- 時刻は `HH:MM`、タイムゾーンは冒頭に `Asia/Tokyo` と明示
- 原文の予定タイトルは改変せず保持、補足メモのみ整形

## 注意

- 抽出結果を勝手に脚色しない（訪問先の感想、推測は書かない）
- カレンダーに無い情報を作らない。ユーザーが補足したい箇所は `TODO:` として残す
- 個人情報（電話番号、予約番号、座席番号）が説明欄に含まれる場合、出力時はそのまま残さずマスクするかユーザーに確認する
