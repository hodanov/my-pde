# routines/

claude.ai のスケジュール Routine（クラウドエージェント / CCR）の定義を、リポジトリで一元管理するためのディレクトリ。**ここを「正（source of truth）」**として扱い、変更は PR レビューを経て反映する。

設計の背景・全体像は [`docs/plan/2026-06-15_autonomous-improvement-scan.md`](../docs/plan/2026-06-15_autonomous-improvement-scan.md) を参照。

## 現在の定義

| ファイル                             | 名前                            | スケジュール                     | 役割                                                     |
| ------------------------------------ | ------------------------------- | -------------------------------- | -------------------------------------------------------- |
| `daily-neovim-trend-scan.json`       | `Daily Neovim Trend Scan`       | 毎日 8:00 JST (`0 23 * * *`)     | Neovim 動向を調べ改善を Issue 起票（最大1件）            |
| `weekly-devx-skills-hooks-scan.json` | `Weekly DevX Skills/Hooks Scan` | 毎週木 7:00 JST (`0 22 * * 3`)   | ai-agents 向け汎用 hooks/skills を Issue 起票（最大1件） |
| `weekly-adopted-issue-pr-bot.json`   | `Weekly Adopted-Issue PR Bot`   | 毎週日曜 8:00 JST (`0 23 * * 6`) | `adopted` Issue を実装しドラフト PR を作成               |

## スキーマ

各 JSON は 1 Routine を表す。

| フィールド                  | 説明                                                                                                           |
| --------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `name`                      | Routine 名                                                                                                     |
| `trigger_id`                | 稼働中 Routine の ID。**update の宛先（state 参照）。手で書き換えない**                                        |
| `enabled`                   | 有効/無効                                                                                                      |
| `cron_expression`           | 5 フィールド cron（**UTC**）。最短間隔は 1 時間                                                                |
| `schedule_note`             | 人間向けの時刻メモ（UTC ↔ JST）。動作には影響しない                                                            |
| `job_config.model`          | 使用モデル（例 `claude-opus-4-8`）                                                                             |
| `job_config.repository`     | チェックアウト対象リポジトリ                                                                                   |
| `job_config.environment_id` | 実行環境 ID                                                                                                    |
| `job_config.allowed_tools`  | 許可ツール                                                                                                     |
| `prompt`                    | エージェントへの指示。**1 行 = 配列 1 要素**（差分レビューしやすくするため）。apply 時に改行（`\n`）で結合する |

## 反映（apply）方法

> 注: Routine の管理面は claude.ai の API（`/v1/code/triggers`）と Web UI。リポジトリと自動同期する公式機構は現状なく、ここは **「定義の正＝repo、反映は手動 apply」** という運用。

1. **このディレクトリの JSON を編集**して PR を出す（レビュー対象）。
2. マージ後、Claude Code の `/schedule`（Update）で対象 Routine を選び、JSON の内容に合わせて反映する。
   - `prompt` 配列は改行で結合した 1 つの文字列として渡す。
   - 宛先は `trigger_id`。
3. 新規 Routine を足す場合は `/schedule`（Create）で作成し、払い出された `trigger_id` をこのディレクトリの新しい JSON に記録する。

削除は API 非対応のため <https://claude.ai/code/routines> から行い、対応する JSON も削除する。

## 既知の制約 / TODO

- **CI からの自動 apply は未対応**: スタンドアロン環境（GitHub Actions 等）から Routine API を叩くための認証手段が未確認。確認できたら `routines/*.json` を読んで create/update を呼ぶ reconcile スクリプト化を検討する。
