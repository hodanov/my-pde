# routines/

claude.ai のスケジュール Routine（クラウドエージェント / CCR）の定義を、リポジトリで一元管理するためのディレクトリ。**ここを「正（source of truth）」**として扱い、変更は PR レビューを経て反映する。

設計の背景・全体像は [`docs/plan/2026-06-15_autonomous-improvement-scan.md`](../docs/plan/2026-06-15_autonomous-improvement-scan.md) と [`docs/plan/2026-07-04_routines-pipeline-expansion.md`](../docs/plan/2026-07-04_routines-pipeline-expansion.md) を参照。

## 現在の定義

| ファイル                             | 名前                            | スケジュール                     | 役割                                                         |
| ------------------------------------ | ------------------------------- | -------------------------------- | ------------------------------------------------------------ |
| `daily-neovim-trend-scan.json`       | `Daily Neovim Trend Scan`       | 毎日 8:00 JST (`0 23 * * *`)     | Neovim 動向を調べ改善を Issue 起票（最大1件）                |
| `weekly-adopted-issue-pr-bot.json`   | `Weekly Adopted-Issue PR Bot`   | 毎週日曜 8:00 JST (`0 23 * * 6`) | `adopted` Issue を実装しドラフト PR を作成                   |
| `weekly-pr-care-bot.json`            | `Weekly PR Care Bot`            | 毎週月曜 7:00 JST (`0 22 * * 0`) | Open な `auto/*` PR の CI 失敗・コンフリクト・レビュー対応   |
| `weekly-scripts-tooling-scan.json`   | `Weekly Scripts Tooling Scan`   | 毎週火曜 7:00 JST (`0 22 * * 1`) | `scripts/` 向け新アプリ/スクリプトを Issue 起票（最大1件）   |
| `weekly-environment-scan.json`       | `Weekly Environment Scan`       | 毎週水曜 7:00 JST (`0 22 * * 2`) | `environment/`・`dotfiles/`・`mise.toml` の改善を Issue 起票 |
| `weekly-devx-skills-hooks-scan.json` | `Weekly DevX Skills/Hooks Scan` | 毎週木曜 7:00 JST (`0 22 * * 3`) | ai-agents 向け汎用 hooks/skills を Issue 起票（最大1件）     |
| `weekly-ci-workflows-scan.json`      | `Weekly CI Workflows Scan`      | 毎週金曜 7:00 JST (`0 22 * * 4`) | `.github/workflows/` の CI 改善を Issue 起票（最大1件）      |
| `monthly-routine-improve.json`       | `Monthly Routine Improve`       | 毎月2日 7:00 JST (`0 22 1 * *`)  | 運用実績からプロンプト改善を draft PR で提案（メタループ）   |

このほか、LLM を使わない定型処理として `.github/workflows/pipeline-digest.yml`（毎週土曜 7:00 JST）が、triage 待ち Issue・滞留 adopted・Open な `auto/*` PR をまとめた digest Issue を更新する。

## プロンプトの間接参照

各 Routine の指示本文は `prompts/<routine-name>.md` に置き、JSON 側の `prompt` は「チェックアウト済みリポジトリのそのファイルを読んで従え（無ければ何もせず終了）」という薄いポインタにしている。

- **プロンプト本文の変更は、PR をマージするだけで次回実行から有効**（Routine は実行時に repo を checkout してファイルを読むため）。手動 apply は不要。
- 手動 apply が必要なのは `cron_expression` / `model` / `allowed_tools` / `prompt`（ポインタ文言自体）など **JSON 側フィールドの変更のみ**。

## スキーマ

各 JSON は 1 Routine を表す。

| フィールド                  | 説明                                                                                    |
| --------------------------- | --------------------------------------------------------------------------------------- |
| `name`                      | Routine 名                                                                              |
| `trigger_id`                | 稼働中 Routine の ID。**update の宛先（state 参照）。手で書き換えない**。新規作成前は空 |
| `enabled`                   | 有効/無効                                                                               |
| `cron_expression`           | 5 フィールド cron（**UTC**）。最短間隔は 1 時間                                         |
| `schedule_note`             | 人間向けの時刻メモ（UTC ↔ JST）。動作には影響しない                                     |
| `job_config.model`          | 使用モデル（例 `claude-opus-4-8`）                                                      |
| `job_config.repository`     | チェックアウト対象リポジトリ                                                            |
| `job_config.environment_id` | 実行環境 ID                                                                             |
| `job_config.allowed_tools`  | 許可ツール                                                                              |
| `prompt`                    | `prompts/<name>.md` への薄いポインタ。**1 行 = 配列 1 要素**。apply 時に改行で結合する  |

## 反映（apply）方法

> 注: Routine の管理面は claude.ai の API（`/v1/code/triggers`）と Web UI。リポジトリと自動同期する公式機構は現状なく、ここは **「定義の正＝repo、反映は手動 apply」** という運用。ただしプロンプト本文（`prompts/*.md`）は間接参照のためマージだけで反映される。

1. **このディレクトリの JSON / `prompts/*.md` を編集**して PR を出す（レビュー対象）。
2. `prompts/*.md` だけの変更なら、マージで完了（apply 不要）。
3. JSON 側フィールドを変えた場合はマージ後、Claude Code の `/schedule`（Update）で対象 Routine を選び、JSON の内容に合わせて反映する。
   - `prompt` 配列は改行で結合した 1 つの文字列として渡す。
   - 宛先は `trigger_id`。
4. 新規 Routine を足す場合は `/schedule`（Create）で作成し、払い出された `trigger_id` をこのディレクトリの JSON に記録する。

削除は API 非対応のため <https://claude.ai/code/routines> から行い、対応する JSON も削除する。

## 運用ノート

- スキャンが起票した Issue の採用判定はラベルで記録する: 採用は `adopted`、不採用は **`rejected` を付けて Close**。
- **rejected で Close するときは、不採用の理由を一言コメントに残す**。`Monthly Routine Improve`（メタループ）がこのコメントを読んでスキャンのプロンプト改善に使う。
- `adopted` Issue は日曜朝の PR Bot がドラフト PR 化し、`pr-created` ラベルを付ける。月曜朝の PR Care Bot が CI 失敗・コンフリクト・レビュー指摘をケアする。

## 既知の制約 / TODO

- **CI からの自動 apply は未対応**: スタンドアロン環境（GitHub Actions 等）から Routine API を叩くための認証手段が未確認。確認できたら `routines/*.json` を読んで create/update を呼ぶ reconcile スクリプト化を検討する。プロンプト本文の間接参照化（`prompts/*.md`）により、apply が必要な変更は JSON 側フィールドのみに縮小済み。
