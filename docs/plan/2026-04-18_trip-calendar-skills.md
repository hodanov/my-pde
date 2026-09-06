# Plan: trip-\* カレンダー連携 Skill 群

Google カレンダーと連携する旅行用 Agent Skill を 3 つ新設する。カレンダーから旅記録・時刻表を生成する 2 本と、旅行会社メールを解析して旅程をカレンダーに登録する 1 本を、`trip-` プリフィックスで揃えて実装する。

## Background

- 旅の予定や乗り換え情報を Google Calendar に登録している
- 旅行後にブログ記事として残したい、旅行前に 1 日単位の時刻表を手元に置きたい、というニーズがある
- 旅行会社からの予約メールを手動でカレンダーに転記する作業が面倒
- MCP の Google Calendar ツールは揃っているが、毎回手で叩くのは煩雑なので Skill 化する

## Current structure

- 利用可能な MCP ツール
  - `mcp__claude_ai_Google_Calendar__list_calendars` / `list_events` / `get_event` / `search_events`
  - `mcp__claude_ai_Google_Calendar__create_event` / `update_event` / `delete_event`
- Skill 置き場: リポ側ソースは `ai-agents/personal/skills/<skill-name>/`、デプロイ先は `~/.claude/skills/<skill-name>/`（`SKILL.md` 必須）
- 類似 Skill: `plan-markdown-export`, `plan-issue-export`, `blog-idea-draft-export`
  - いずれも Markdown 出力の流儀（日付プリフィックス付きファイル名、kebab-case スラッグ）が揃っている
- 旅記録ブログ出力との橋渡しは今回のスコープ外

## Design policy

- 命名は `trip-` プリフィックスで統一し、カレンダーとの向きを語尾で示す（`-from-calendar` / `-to-calendar`）
- 参照・書き込み対象は primary カレンダーのみ
- タイムゾーンは `Asia/Tokyo` を常に明示
- Skill は MCP 呼び出しをラップし、テンプレート整形・差分確認・ユーザー承認を担当
- 認証はコネクタ側に委ねる。Skill では MCP 呼び出しが失敗したときに再接続を案内するだけにする
- プライベート予定を意図せず出力・登録しないよう、出力/登録前に抽出結果をユーザーに提示して承認を得るステップを必ず入れる
- Skill ソースは個人用途なので `ai-agents/personal/skills/` 配下に配置し、既存仕組み `mise run claude-skills-copy` で `~/.claude/skills/` にコピーされる前提
- 旅程登録時のリマインダーは **開始 30 分前のポップアップ通知** をデフォルトとする

## Skill 一覧

| Skill                          | 役割                                                              | 方向                | 出力先                                |
| ------------------------------ | ----------------------------------------------------------------- | ------------------- | ------------------------------------- |
| `trip-log-from-calendar`       | 旅記録 Markdown を生成                                            | Calendar → Markdown | `docs/travel/YYYY-MM-DD_<slug>.md`    |
| `trip-timetable-from-calendar` | 1 日単位の時刻表 Markdown を生成（複数路線を 1 ファイルに束ねる） | Calendar → Markdown | `docs/timetable/YYYY-MM-DD_<slug>.md` |
| `trip-register-to-calendar`    | 旅行会社メールなどから情報を抽出して Calendar に登録              | Text → Calendar     | Google Calendar (primary)             |

## Skill 詳細

### trip-log-from-calendar

- 入力: 期間（開始日〜終了日）、任意のタイトルキーワードやタグ
- 処理
  1. `list_events` で期間内のイベント取得
  2. 必要に応じて `get_event` で詳細取得
  3. 旅記録テンプレに流し込む（日付、場所、移動、滞在、メモ）
  4. ユーザーに抽出結果を提示して承認
- 出力: `docs/travel/YYYY-MM-DD_<slug>.md`（開始日ベース）

### trip-timetable-from-calendar

- 入力: 対象日（単一日）、任意の路線フィルタ
- 処理
  1. `list_events` で当日分を取得
  2. 交通機関イベントを抽出（タイトル・説明から出発/到着駅、列車名、座席を解釈）
  3. 時系列で並べ、複数路線でも 1 ファイルにまとめる
- 出力: `docs/timetable/YYYY-MM-DD_<slug>.md`（slug 例: `saga-trip`）

### trip-register-to-calendar

- 入力: 旅の目的/テーマ + 旅行会社メール等の生テキスト（フォーマットはバラバラ前提、テンプレ無し）
- 抽出対象: 日時、列車名/便名、出発駅/到着駅、座席、宿泊施設名、チェックイン日、プラン名、施設電話番号、往復区分
- 処理
  1. 生テキストから必要情報を LLM で抽出（定型構造に寄せて内部表現化）
  2. 交通機関 1 区間 = 1 イベント、宿泊 = 1 イベントに正規化
  3. タイトル命名規則
     - 交通: `🚄 列車名 列車番号 | 出発駅→到着駅`（例: `🚄 つるぎ1号 | 金沢→敦賀`）
     - 宿泊: `🏨 施設名`（例: `🏨 コンフォートホテル佐賀`）
  4. 説明欄に「座席」「プラン名」「電話番号」等を保持
     - 交通:

       ```text
       新大阪発 08:41 > 博多着 11:09
       列車: のぞみ3号
       座席: 4号車14A
       ```

     - 宿泊:

       ```text
       TEL: xxxx-xx-xxxx
       部屋: 1ベッド ダブルエコノミー（禁煙・バス付き）
       プラン: コンフォートRパッケージ 朝食付き
       ```

  5. リマインダーは全イベント一律で「開始 30 分前のポップアップ通知」をデフォルトで付与
  6. ユーザーに登録予定イベント一覧を表で提示し、承認後に `create_event` 実行
  7. 登録後、作成された各イベント ID を一覧で返す

- 出力: Google Calendar (primary) に複数イベント

#### 入力サンプル（動作確認用）

2026/04/18 の SV リーグチャンピオンシップ観戦（佐賀）旅程メール:

- 宿泊: コンフォートホテル佐賀 1 泊
- 往路: 金沢 06:00 → 敦賀 → 新大阪 → 博多 → 佐賀 12:16（つるぎ 1 号 / サンダーバード 2 号 / のぞみ 3 号 / みどり 23 号）
- 復路 (2026/04/19): 佐賀 17:08 → 博多 → 新大阪 → 敦賀 → 金沢 23:25（みどり 44 号 / のぞみ 58 号 / サンダーバード 49 号 / つるぎ 50 号）
- 合計 9 イベント（宿泊 1 + 交通 8）が登録される想定

## Implementation steps

1. `ai-agents/personal/skills/trip-log-from-calendar/` 作成
   1. `SKILL.md` 記述（目的 / 使い方 / 入力 / 処理 / 出力 / 承認ステップ）
   2. `templates/travel-log.md` 作成
2. `ai-agents/personal/skills/trip-timetable-from-calendar/` 作成
   1. `SKILL.md` 記述
   2. `templates/timetable.md` 作成（1 日 1 ファイル、路線セクション分割）
3. `ai-agents/personal/skills/trip-register-to-calendar/` 作成
   1. `SKILL.md` 記述
   2. 抽出ルール（交通機関 / 宿泊）とタイトル命名規則を明文化
   3. `examples/` に佐賀旅行サンプルを配置（動作確認用）
4. `mise run claude-skills-copy` で `~/.claude/skills/` に反映
5. 3 Skill それぞれをドライランし、期待する出力/登録内容を確認
6. 必要に応じて `~/.claude/settings.json` の MCP 権限を調整（`create_event` などの許可）
7. 出力先ディレクトリ `docs/travel/` と `docs/timetable/` は初回実行時に必要なら自動生成（事前作成しない）

## File changes

| File                                                                              | Change               |
| --------------------------------------------------------------------------------- | -------------------- |
| `ai-agents/personal/skills/trip-log-from-calendar/SKILL.md`                       | 新規作成             |
| `ai-agents/personal/skills/trip-log-from-calendar/templates/travel-log.md`        | 新規作成             |
| `ai-agents/personal/skills/trip-timetable-from-calendar/SKILL.md`                 | 新規作成             |
| `ai-agents/personal/skills/trip-timetable-from-calendar/templates/timetable.md`   | 新規作成             |
| `ai-agents/personal/skills/trip-register-to-calendar/SKILL.md`                    | 新規作成             |
| `ai-agents/personal/skills/trip-register-to-calendar/examples/saga-2026-04-18.md` | 新規作成（サンプル） |

## Risks and mitigations

| Risk                                     | Mitigation                                                                                         |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------- |
| MCP 認証未完了で失敗                     | 認証はコネクタ側の責務。SKILL.md には呼び出し失敗時に再接続を案内する手順だけ書く                  |
| 予約メールのフォーマットが多様で抽出漏れ | 抽出対象（時間/列車/座席/宿泊先/プラン名/電話番号/往復）を列挙し、見つからない項目は空欄として扱う |
| 誤情報のままカレンダーに登録される       | `create_event` 前に抽出結果を表で提示し、ユーザー承認を必須化                                      |
| タイムゾーンずれ                         | 内部は ISO8601、出力/登録時に `Asia/Tokyo` を明示                                                  |
| プライベート予定の意図しない出力         | 出力前にイベントタイトル一覧を提示し、除外指定を受け付ける                                         |
| 大量イベントで API レート制限            | 時刻表は 1 日単位、旅記録は期間必須にして取得件数を抑える                                          |
| 命名がカレンダー操作汎用 Skill と衝突    | `trip-` プリフィックス固定でスコープを「旅行用途」に限定                                           |

## Validation

- [ ] `trip-log-from-calendar` で直近の旅行期間を指定して旅記録 Markdown が `docs/travel/` に生成される
- [ ] `trip-timetable-from-calendar` で 2026/04/18 を指定したとき、往路 4 区間が 1 ファイルにまとめて出力される
- [ ] `trip-register-to-calendar` に佐賀旅行メールを渡すと、宿泊 1 + 交通 8 の計 9 イベントが抽出され、承認後に primary カレンダーへ登録される
- [ ] 3 Skill いずれもタイムゾーン `Asia/Tokyo` が出力/登録に反映される
- [ ] 認証未済のとき親切なエラー/誘導が出る
- [ ] プライベート予定がある場合、出力前の承認ステップが機能する

## Open questions

- 時刻表 slug の命名規則（例: 目的地ベース `saga-trip` か、路線列挙 `tsurugi-thunderbird-nozomi-midori` か）— とりあえず目的地ベースで進め、運用で見直す想定
