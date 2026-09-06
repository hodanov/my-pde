---
name: trip-timetable-from-calendar
description: Google カレンダー primary から指定 1 日の交通機関予定を抽出し、複数路線を束ねた時刻表 Markdown を `docs/timetable/YYYY-MM-DD_<slug>.md` として生成する。乗り換えを時系列で 1 ファイルにまとめる。
disable-model-invocation: "true"
argument-hint: "[抽出したい旅の情報 例: 2026-04-18 佐賀行き]"
---

# Trip Timetable From Calendar

旅行当日に手元で使える時刻表 Markdown を、カレンダーの交通機関予定から生成する。

## 想定する使い方

- `/trip-timetable-from-calendar 2026-04-18 佐賀行き`
- `/trip-timetable-from-calendar 明日の時刻表`

## 入力

- 対象日: 単一日（必須、`YYYY-MM-DD`）
- 任意の路線フィルタ（例: `新幹線`、`特急のみ`）
- slug（未指定なら目的地から kebab-case で自動生成。例: `saga-trip`）

## 前提

- Google Calendar MCP が利用可能
- 未認証の場合は `authenticate` → `complete_authentication` を先に案内する
- 参照カレンダーは primary のみ
- タイムゾーンは `Asia/Tokyo`

## 手順

1. 対象日と slug を確定する。曖昧ならユーザーに確認する。
2. `mcp__claude_ai_Google_Calendar__list_events` で対象日の予定を取得する（`calendarId: "primary"`、`timeZone: "Asia/Tokyo"`、`timeMin` と `timeMax` は対象日の 00:00〜23:59）。
3. 交通機関イベントを抽出する。判定ヒント:
   - タイトルの 🚄 プリフィックス
   - タイトル内の `<駅>→<駅>` パターン
   - タイトル末尾の列車名/便名
   - 説明欄の「列車:」「座席:」行
4. 抽出結果を開始時刻の昇順でソートし、乗り換え区間として束ねる。
5. **抽出結果をユーザーに表で提示**し、出力して良いか承認を得る。
6. [templates/timetable.md](templates/timetable.md) に沿って Markdown を組み立てる。
7. `docs/timetable/` が無ければ作成する。
8. ローカル日付の `YYYY-MM-DD_<slug>.md` で保存する。既存ファイルがある場合は上書き前に確認する。
9. `markdownlint-cli2 --fix <file>` を実行する（利用可能な場合）。
10. 生成ファイルのパスを返す。

## 出力規約

- 出力先: `docs/timetable/YYYY-MM-DD_<slug>.md`
- 1 日 1 ファイル（複数路線でも 1 ファイルに束ねる）
- 時刻は `HH:MM`、タイムゾーンは冒頭に `Asia/Tokyo` と明示
- 乗り換え時間（前の到着〜次の出発）を計算して併記する

## 注意

- イベントや交通機関イベントが無い場合は、その旨を返してファイルは作成しない
- 座席番号などの機微情報が含まれる場合、出力時はそのまま残さずユーザーに確認する
- 目的地が読み取れない時は slug をユーザーに確認する
