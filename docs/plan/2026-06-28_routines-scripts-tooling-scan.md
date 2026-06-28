# Plan: `scripts/` 向け新アプリ/スクリプト提案ルーチンの追加

## Context

`my-pde` には既に「動向を調べて改善を Issue 起票する」自律スキャン系ルーチンが2本（`daily-neovim-trend-scan.json` / `weekly-devx-skills-hooks-scan.json`）と、`adopted` Issue を実装する PR Bot（`weekly-adopted-issue-pr-bot.json`）が稼働しており、`scan → Issue → 人が adopted → PR Bot` の自律改善ループを形成している。

しかし現状このループの「動向スキャン」は **Neovim 設定** と **ai-agents の hooks/skills** しかカバーしておらず、`scripts/`（現状 `ai-bridge` の Go daemon のみ）を対象に開発効率を高める **新しいアプリ/スクリプトの実装** を提案する担い手が無い。

そこで、最近のソフトウェアエンジニアリング動向を踏まえて `scripts/` 配下に新規ツール（または既存 ai-bridge の拡張）を提案する週次スキャンルーチンを1本追加する。出力は既存スキャン同様 **Issue 起票のみ・最大1件** とし、`adopted` を付ければ既存 PR Bot がそのまま実装フェーズに引き継ぐ。

## 決定事項

- **実行頻度**: 週次。`0 22 * * 1`（月22:00 UTC = **毎週火 7:00 JST**）。既存スロット（毎日23:00 UTC / 水22:00 UTC / 土23:00 UTC）と衝突しない。
- **スコープ**: net-new ツールに加え、既存 `ai-bridge` への新サブコマンド追加等の拡張も提案可。
- **ラベル**: `scan:scripts`（既存の `scan:*` 命名規約に準拠）。重複排除は Open `scan:scripts` ＋ Close 済み `scan:scripts`+`rejected` の両系統。

## 追加するファイル

### `routines/weekly-scripts-tooling-scan.json`（新規）

既存スキャンルーチン（`daily-neovim-trend-scan.json`）を雛形に、read-only ツール構成・最大1件・重複排除・「被ったら別角度を選び直す」方針を踏襲する。

- `name`: `Weekly Scripts Tooling Scan`
- `trigger_id`: 新規作成前は払い出されていないため `"PENDING_CREATE"` のプレースホルダ。`/schedule`（Create）実行後に実 ID へ差し替える（`routines/README.md` の「新規 Routine を足す場合」に従う）。
- `cron_expression`: `0 22 * * 1`、`schedule_note`: 月 22:00 UTC = 毎週火曜 7:00 JST
- `environment_id`: 既存と共通の Default `env_01H6ttcff6EJXggu8Xqmx5gN`
- `allowed_tools`: `Bash` / `Read` / `Grep` / `Glob` / `WebSearch` / `WebFetch`（read-only）
- `prompt`: 1行=配列1要素（差分レビューのため）

#### prompt の骨子（既存スキャンの構成を踏襲）

1. **役割定義**: 「`my-pde` の `scripts/` 配下を対象に、開発効率を高める新しいアプリ/スクリプトの実装を自律的に提案するエージェント」。PDE 全体像（Neovim + Docker + AI agents 連携）も踏まえる。
2. **リポジトリ構成の提示**: `scripts/ai-bridge/`（Neovim(Docker内) ↔ ホスト側 AI CLI を仲介する Go daemon。`cmd/ai-bridge/`、`internal/{daemon,launcher,watcher,launchd}`、Go 1.26 + fsnotify、`AI_BRIDGE_*` 環境変数）。検証は `lint_ai_bridge.yml`(goimports + golangci-lint) / `test_ai_bridge.yml`(go test) が PR で走る点も明記。
3. **重複排除（起票前に両系統を取得）**:
   - `gh issue list --state open --label "scan:scripts" --json number,title,body --limit 50`（提案済み Open）
   - `gh issue list --state closed --label "scan:scripts" --label "rejected" --json number,title,body --limit 50`（不採用 Close 済み）
   - どちらとも重複しないものを選ぶ。特に rejected は同じ角度で出し直さない。
4. **調査**: WebSearch / WebFetch で最近のソフトウェアエンジニアリング動向（開発者向けツール・CLI/TUI・タスク自動化・AI 支援開発・DX/生産性ツール 等）を調べつつ、`scripts/ai-bridge/` の現状を読む。ネタは「新規ツール」「既存 ai-bridge への機能拡張」の両方を広く対象にする。
5. **1つだけ選ぶ**: この PDE で実際に開発効率を高められる提案を1件。手順3の Open / rejected と被るものは選び直す。net-new は `scripts/<name>/`、拡張は ai-bridge 内に閉じる前提で、ai-bridge の既存目的と無駄に重複しないこと。
6. **起票**: ラベルが無ければ `gh label create scan:scripts`。`gh issue create --label "scan:scripts"` で1件だけ。body に含める項目:
   - (a) 何を・なぜ（どんな開発効率の課題を解くか）
   - (b) 根拠とした最近の SWE 動向・出典URL（あれば）
   - (c) 配置先と技術選定: net-new なら `scripts/<name>/`（ai-bridge に倣い Go、用途次第でシェル）/ 拡張なら ai-bridge のどこに。PDE(Neovim/Docker/AI agents) とどう連携するか
   - (d) 実装スケッチ・スコープ感（最小構成）
   - (e) リスク・留意点（CI 検証 `lint_ai_bridge` / `test_ai_bridge` / `lint_shell` への影響を含む）
7. **制約**: コード変更・PR 作成はしない（Issue 起票のみ）。探索を広げすぎない。最大1件。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。被らない新提案がどうしても無い場合のみ起票せず終了。

## 更新するファイル

### `routines/README.md`

「現在の定義」テーブルに新ルーチンの1行を追記する:

| ファイル                           | 名前                          | スケジュール                   | 役割                                                       |
| ---------------------------------- | ----------------------------- | ------------------------------ | ---------------------------------------------------------- |
| `weekly-scripts-tooling-scan.json` | `Weekly Scripts Tooling Scan` | 毎週火 7:00 JST (`0 22 * * 1`) | `scripts/` 向け新アプリ/スクリプトを Issue 起票（最大1件） |

## なぜ既存パターンを再利用するか

- スキャン系の prompt 構成（役割→構成提示→重複排除2系統→調査→1件選定→起票項目→制約）は `daily-neovim-trend-scan.json` / `weekly-devx-skills-hooks-scan.json` で確立済み。新ルーチンはこれを `scripts/` 文脈に差し替えるだけで、運用・レビューの一貫性を保てる。
- `adopted` / `rejected` / `pr-created` のラベル運用と PR Bot 連携は既存のまま流用でき、新たな配線は不要（PR Bot は系統ラベルを問わず `adopted` 全件を処理する）。

## 検証

ルーチン定義の追加なので、検証は主に **静的チェック** と **手動 apply** になる。

1. **JSON 妥当性**: `jq . routines/weekly-scripts-tooling-scan.json` でパースが通ること。
2. **prettier**: `prettier --check routines/weekly-scripts-tooling-scan.json`。README 変更は `markdownlint-cli2` + `prettier --check`。CI の `lint_format.yml` 相当。
3. **スキーマ整合**: 既存3本の JSON とキー構成が一致していること。`cron_expression` が UTC 5フィールドで `schedule_note` に JST 併記があること。
4. **prompt の自己レビュー**: 重複排除の2コマンド・`gh label create` / `gh issue create` の系統ラベルが `scan:scripts` で揃っているか確認。
5. **反映（手動・PR マージ後）**: `routines/README.md` の手順に従い `/schedule`（Create）で新ルーチンを作成 → 払い出された `trigger_id` を JSON の `"PENDING_CREATE"` と差し替える追従コミット。`.claude/rules/routines.md` の通り CI 自動 apply は無く手動。
6. **動作確認（任意）**: `/schedule` から手動トリガーで1回走らせ、`scan:scripts` ラベル付き Issue が1件だけ起票され、body に上記 (a)〜(e) が含まれることを確認する。
