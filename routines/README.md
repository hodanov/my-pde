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

このほか、LLM を使わない定型処理として `.github/workflows/pipeline-digest.yml`（毎週土曜 7:00 JST）が、`digest` ラベルの付いた単一 Issue の body を上書き更新する。内容は 2 段構え。

- **滞留（ストック）**: triage 待ち Issue・PR 化待ちの adopted・Open な `auto/*` PR・14 日以上 Open の scan Issue。
- **フロー指標**: [`scripts/pipeline-metrics`](../scripts/pipeline-metrics/README.md) がスキャン別に集計した採用率・PR 化率・マージ率・リードタイム・月次トレンドと、ラベル運用の異常。閾値を超えると body 冒頭に警告ブロックが出て Issue に `alert` ラベルが付く（回復すると自動で外れる）。末尾には機械可読な JSON ブロックが埋め込まれ、`Monthly Routine Improve` がそれを読んで改善テーマを絞る。

## プロンプトの間接参照

各 Routine の指示本文は `prompts/<routine-name>.md` に置き、JSON 側の `prompt` は「チェックアウト済みリポジトリのそのファイルを読んで従え（無ければ何もせず終了）」という薄いポインタにしている。

- **プロンプト本文の変更は、PR をマージするだけで次回実行から有効**（Routine は実行時に repo を checkout してファイルを読むため）。手動 apply は不要。
- 手動 apply が必要なのは `cron_expression` / `model` / `allowed_tools` / `prompt`（ポインタ文言自体）など **JSON 側フィールドの変更のみ**。

## クラウド実行で効く設定・効かない設定

Routine は Anthropic クラウド上の使い捨てセッションで、リポジトリを fresh clone して動く。したがって **リポジトリにコミットされたものは効き、`~/.claude` 配下は一切効かない**（[Configure cloud environments](https://code.claude.com/docs/en/cloud-environments) の "What carries over from your setup"）。

| 対象                                                                    | クラウドで有効                                                                   |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| repo の `CLAUDE.md` / `AGENTS.md`                                       | 有効                                                                             |
| repo の `.claude/rules/`（`paths:` にマッチしたときロード）             | 有効                                                                             |
| repo の `.claude/skills/` / `.claude/agents/` / `.claude/commands/`     | 有効                                                                             |
| repo の `.claude/settings.json` の hooks                                | 有効（`guard-version-pins.sh` と `test-changed.sh` は Routine 実行中も発火する） |
| repo の `.mcp.json`、`.claude/settings.json` で宣言した plugins         | 有効                                                                             |
| `~/.claude/` 配下（`agents.xml`、汎用 rules、skills、subagents、hooks） | 無効。`ai-agents/` から配っているものはクラウドに届かない                        |

この前提から、プロンプト本文には次を書かない。

- 「`~/.claude` はクラウドに無い」といった**仕様として自明なこと**。
- `AGENTS.md` / `.claude/rules/` / `ai-agents/scripts/verify-changed.sh` に**既に書いてある規約の写し**（lint コマンド一覧、SKILL.md の書式規約など）。二重管理になり、必ず片方がドリフトする。

検証は `ai-agents/scripts/verify-changed.sh` に寄せる。変更ファイルの種別から lint / test を自動で選ぶので、プロンプト側でコマンドを列挙する必要がない。ツールが無い場合は `[SKIP]` になる（PASS ではない）ため、残った SKIP は CI の判定に委ねる。

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
- スキャンが起票する Issue は**タイトル 32 字以内（`<prefix>:` を除く）・本文 1,500 字程度**に保つ。網羅的な証明ではなく主張と出典ポインタに留めるのが方針で、レビュー側が足りない分を会話で掘る前提に立っている。分量を緩める変更を入れるときは、この前提ごと見直すこと。

## ラベル運用の約束（効果測定の前提）

digest のフロー指標はラベルとタイムスタンプだけを入力にしている。**ラベル運用が崩れると数値が崩れる**ので、次を守る。破れたものは digest の「ラベル運用の異常」節に出る。

- **`rejected` は Close と同時に付ける。** 却下までのリードタイムは `closedAt` を判定時刻として使う。後から付けると実際より長く見える。
- **`adopted` は後から外さない。** 実装まで進めた後に捨てる場合は `adopted` を残したまま `rejected` を足して Close する。集計は rejected に倒し、「PR まで作ってから捨てた最もコストの高い失敗」として `rejected_after_pr_rate` に数える。
- **`scan:*` は 1 Issue に 1 つ。** 複数付いた場合は辞書順先頭のスキャンに寄せて集計する（異常として記録される）。
- **scan Issue をラベル無しで Close しない。** 採用でも却下でもない Close は `untracked_close` として運用崩れの指標になる。
- **PR のブランチ名は `auto/issue-<番号>-<slug>`。** PR と Issue の join はこの命名だけが根拠。`pr-created` ラベルは人間向けの目印で、集計は実際の PR の有無で判定する。
- **集計起点は 2026-06-28**（`rejected` 運用の定着後）。**遡及ラベル付けはしない**。それ以前の Issue は件数だけ数え、率の分母には入れない。
- 手動で起票した Issue には `scan:*` を付けない。パイプラインの効果測定という趣旨から外れるため集計対象外にしている。

## 既知の制約 / TODO

- **CI からの自動 apply は未対応**: スタンドアロン環境（GitHub Actions 等）から Routine API を叩くための認証手段が未確認。確認できたら `routines/*.json` を読んで create/update を呼ぶ reconcile スクリプト化を検討する。プロンプト本文の間接参照化（`prompts/*.md`）により、apply が必要な変更は JSON 側フィールドのみに縮小済み。
