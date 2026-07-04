# Plan: 自律改善パイプライン（routines/）の拡張

自律改善ループ（スキャン → Issue 起票 → 人が adopted/rejected → PR Bot → 人がマージ）を 4 つの柱で拡張する: (1) プロンプトの間接参照化で手動 apply をほぼ撤廃、(2) auto PR をケアする Routine の追加、(3) 運用実績からプロンプトを改善するメタループ、(4) スキャン対象の追加（CI workflows / environment・dotfiles）。あわせて LLM 不要の定型処理（滞留の可視化）は素の GitHub Actions に切り出す。

## Background

- 既存ループは健全に回っている（直近 scan Issue の採用率 ~95%、`auto/*` PR 16 件中 14 件マージ）が、次のギャップがある。
  1. プロンプト変更のたびに手動 `/schedule` apply が必要（`routines/README.md` の既知 TODO）。
  2. PR 作成後のケア（CI 失敗・コンフリクト・レビューコメント対応）が全部手動。
  3. rejected や未マージ close の教訓がプロンプトに還元されない（skills の observe → improve に相当する仕組みが routines に無い）。
  4. `.github/workflows/` と `environment/`・`dotfiles/` がスキャンの空白地帯。
  5. triage 待ち・滞留 adopted・Open な auto PR が一覧できない。
- triage ゲート（人手の `adopted` 付与）は現状維持とする。採用率は高いが、無駄打ち実装（rejected 後 PR）は稀で許容範囲のため安全側を取る。

## Current structure

- `routines/*.json` — Routine 定義（source of truth）。`prompt` は配列で全文を持ち、変更のたびに `/schedule` Update で手動反映していた。
- スキャン 3 本（daily nvim / weekly scripts / weekly devx）+ `Weekly Adopted-Issue PR Bot`。
- ラベル運用: `scan:*` / `adopted` / `rejected` / `pr-created`。

## Design policy

- **プロンプト間接化**: 指示本文を `routines/prompts/<name>.md` へ移し、JSON の `prompt` は「checkout 済み repo のそのファイルを読んで従え。無ければ何もせず終了」という薄いポインタにする。Routine は実行時に repo を checkout するため、プロンプト変更はマージだけで次回実行から有効になる。手動 apply は JSON 側フィールド（cron / model / allowed_tools / ポインタ文言）の変更時のみに縮小。markdown 化により markdownlint / prettier の検証対象になる副次効果もある。
- **Weekly PR Care Bot**（月曜 7:00 JST）: Open な `auto/*` PR の CI 失敗修正・コンフリクト解消（merge のみ、rebase / force-push 禁止）・レビューコメント対応を行う。人手 commit が後に積まれた PR はスキップ。ready 化・マージはしない。
- **Monthly Routine Improve**（毎月 2 日 7:00 JST）: 直近 1 ヶ月の rejected Issue（close コメント＝不採用理由）・未マージ close の auto PR・レビュー指摘を分析し、`routines/prompts/*.md` への最小差分の改善を draft PR（最大 1 件、ラベル `meta:routines`）で提案する。間接化により「マージ＝反映」が成立する。前提として rejected close 時に理由コメントを残す運用を README に明記。
- **スキャン追加**: `Weekly Environment Scan`（水曜、`scan:environment`、対象 `environment/`・`dotfiles/`・`mise.toml`）と `Weekly CI Workflows Scan`（金曜、`scan:ci`、actionlint を自走して参照）。いずれも既存スキャンと同型（Open + closed rejected の重複排除、最大 1 件起票、コード変更なし）。
- **Pipeline digest（Routine 以外の方法）**: 決定論的な滞留可視化に LLM は不要なので、`gh` + `jq` だけの GitHub Actions cron（土曜 7:00 JST + `workflow_dispatch`）にする。`digest` ラベルの単一 Issue の body を上書き更新し、Issue を増殖させない。`scan:*` ラベルは付けず、スキャンの重複排除を汚染しない。
- 週次スケジュールは曜日で分散: 日=PR Bot、月=PR Care、火=scripts、水=environment、木=devx、金=ci、土=digest、毎日=nvim。

## Implementation steps

1. 既存 4 Routine の prompt を `routines/prompts/*.md` へ移設し、JSON をポインタ化（scripts scan の「scripts/ 配下は ai-bridge ただ 1 つ」等の古い記述は現状に合わせて一般化）。
2. 新規 4 Routine のプロンプト md と JSON（`trigger_id` は空）を追加。
3. `.github/workflows/pipeline-digest.yml` を追加。
4. `routines/README.md`（一覧・スキーマ・apply 手順・運用ノート）と `.claude/rules/routines.md` を更新。
5. マージ後の apply: 既存 4 本を `/schedule` Update でポインタ prompt へ切替（1 回きり）、新規 4 本を `/schedule` Create し、払い出された `trigger_id` を JSON に追記コミット。

## File changes

| File                                                                                                                                  | Change                                      |
| ------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `routines/prompts/{daily-neovim-trend-scan,weekly-scripts-tooling-scan,weekly-devx-skills-hooks-scan,weekly-adopted-issue-pr-bot}.md` | 既存 prompt 本文を移設（新規）              |
| `routines/prompts/{weekly-pr-care-bot,weekly-ci-workflows-scan,weekly-environment-scan,monthly-routine-improve}.md`                   | 新 Routine のプロンプト（新規）             |
| `routines/{daily-neovim-trend-scan,weekly-scripts-tooling-scan,weekly-devx-skills-hooks-scan,weekly-adopted-issue-pr-bot}.json`       | `prompt` をポインタ化                       |
| `routines/{weekly-pr-care-bot,weekly-ci-workflows-scan,weekly-environment-scan,monthly-routine-improve}.json`                         | 新規定義（`trigger_id` は Create 後に記録） |
| `routines/README.md`                                                                                                                  | 一覧・スキーマ・apply 手順・運用ノート更新  |
| `.claude/rules/routines.md`                                                                                                           | prompt 間接参照のルールを追記               |
| `.github/workflows/pipeline-digest.yml`                                                                                               | LLM 抜き週次ダイジェスト（新規）            |

## Risks and mitigations

| Risk                                                | Mitigation                                                                            |
| --------------------------------------------------- | ------------------------------------------------------------------------------------- |
| ポインタ prompt で md が読めない（checkout 失敗等） | 「ファイルが無ければ何もせず終了」を JSON 側に明記しフェイルセーフに                  |
| PR Care Bot が人間の作業中 PR を触る                | 人手 commit が bot より後にある PR はスキップ。rebase / force-push 禁止、merge で解消 |
| メタループが的外れなプロンプト変更を出す            | 出力は draft PR 止まり（マージ判断は人間）。月 1・最大 1 件・最小差分                 |
| スキャン増による triage 負荷増                      | digest Issue で滞留を可視化。過多なら cron を隔週化（JSON 編集 + apply のみ）         |
| digest Issue がスキャンの重複排除を汚染             | `digest` ラベルのみ付与し、`scan:*` は付けない                                        |

## Validation

- [ ] `markdownlint-cli2` / `prettier --check` / `jq` パース / `actionlint` が全て通る
- [ ] `pipeline-digest.yml` を `workflow_dispatch` で実行し、digest Issue が作成・更新される
- [ ] apply 後、既存 4 Routine がポインタ prompt で従来どおり動く（実行ログで確認）
- [ ] 新スキャン 2 本が最大 1 件だけ起票する
- [ ] PR Care Bot が Open auto PR を正しく処理/スキップする
- [ ] 月次メタループが根拠付きの draft PR（または「変更なし」報告）を出す

## Open questions

- なし（triage ゲート維持・housekeeping の Actions 化はユーザー決定済み）。
