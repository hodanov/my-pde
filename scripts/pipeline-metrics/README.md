# pipeline-metrics

自律改善パイプライン（スキャン → triage → PR Bot → PR Care Bot → マージ）の**フロー指標**を、
`gh` が吐いた JSON から決定論的に集計する **read-only** な CLI。週次 digest
（`.github/workflows/pipeline-digest.yml`）が警告ブロック・フロー節・機械可読 JSON を作るのに使い、
月次メタループ（`routines/prompts/monthly-routine-improve.md`）がその JSON を読んで改善テーマを絞る。

## 設計の前提

- **`gh` を呼ばない。** 入力は JSON ファイル / stdin のみ。ネットワーク・認証・レート制限から
  切り離されているため、`testdata/` のフィクスチャに対するゴールデンテストで出力を固定できる。
- **一切書き込まない。** Issue / PR / ラベルの更新はしない。ラベル付けは人間の triage と
  各 Routine の責任範囲に残す。
- **状態ファイルを持たない。** 毎回全期間を再計算する。スナップショットも履歴ファイルも無いので、
  ラベルを後から編集すれば過去の数値も動く。それは「ラベルが source of truth」という意味で正しい挙動。
- **`--now` を注入できる。** 出力は同じ入力・同じ `--now` に対して常にバイト一致する。

## 使い方

```sh
gh issue list --state all --limit 1000 \
  --json number,title,state,labels,createdAt,closedAt > issues.json
gh pr list --state all --limit 1000 \
  --json number,title,state,headRefName,createdAt,closedAt,mergedAt > prs.json

# digest のフロー節（末尾に機械可読 JSON を埋め込む）
go run ./cmd/pipeline-metrics --issues issues.json --prs prs.json

# 警告ブロックだけ（閾値超過が無ければ 0 バイト）
go run ./cmd/pipeline-metrics --issues issues.json --prs prs.json --format alerts

# 機械可読 JSON だけ
go run ./cmd/pipeline-metrics --issues issues.json --prs prs.json --format json
```

| フラグ               | 既定         | 意味                                                               |
| -------------------- | ------------ | ------------------------------------------------------------------ |
| `--issues`           | （必須）     | `gh issue list --json …` の出力パス（`-` で stdin）                |
| `--prs`              | （必須）     | `gh pr list --json …` の出力パス（`-` で stdin）                   |
| `--since`            | `2026-06-28` | 集計起点。これより前の Issue は件数のみ数え、率の分母に入れない    |
| `--window`           | `90`         | 率の窓（日）。`--now` から遡る。`0` で無効（`--since` 以降すべて） |
| `--min-sample`       | `5`          | 率を表示する最小の母数。下回るセルは `— (n=3)` と出す              |
| `--alert-min-sample` | `8`          | 閾値アラートが発火しうる最小の母数                                 |
| `--format`           | `markdown`   | `markdown`（フロー節）/ `alerts`（警告ブロック）/ `json`           |
| `--now`              | 現在時刻     | 評価時刻（RFC3339）。テストと再現のための注入口                    |

率の窓の下限は `max(--since, --now - --window)`。既定では `--since` が効いており、
時間が経って 90 日窓のほうが新しくなると窓が優先される。月次トレンドだけは窓を使わず、
`--since` 以降の全期間をコホート分けする。

## 正規化ルール

- **スキャン Issue** = `scan:` 前置ラベルを持つ Issue。複数あれば辞書順先頭に寄せ、
  `anomalies.duplicate_scan_labels` に記録する。`scan:` を持たない手動 Issue は集計対象外
  （triage ラベルが付いていれば `anomalies.triaged_non_scan` に出す）。
- **triage 状態**は adopted / rejected / untriaged / untracked_close（ラベル無しで Close = 運用崩れ）の 4 値。
  **`adopted` と `rejected` が併存する Issue は rejected に倒す**（採用して実装まで進めたが最終的に捨てた、
  最もコストの高い失敗）。
- **PR ↔ Issue の join** は head ブランチ `^auto/issue-(\d+)-` から。マッチしない `auto/*` は
  meta ブランチとして別勘定（`anomalies.meta_branch_prs`）、`auto/` 以外の手書き PR は完全に対象外。
- PR 段階の指標は **PR 自身の作成日**で窓を切る。Issue 段階の指標は Issue の作成日で切る。
- パース不能なレコードは落として `anomalies.parse_warnings` に残す。`gh` 側のスキーマが変わっても
  digest 全体は落ちない。

## 指標

スキャン別（+ 合計行）に、パイプラインの段階ごとへ分解する。どのスキャンが効いていないかの
特定が目的なので、全体値だけを見ない。

| 段階         | 指標                          | 定義                                                                      |
| ------------ | ----------------------------- | ------------------------------------------------------------------------- |
| スキャン品質 | `opened`                      | 窓内に起票された scan Issue 数                                            |
|              | `opened_last_28d`             | うち直近 28 日の起票数（Routine の生存確認）                              |
|              | `adopted_rate`                | `adopted` / `opened`（未 triage も分母に含む）                            |
|              | `rejected_after_pr_rate`      | PR まで作ってから却下 / `opened`                                          |
|              | `untracked_close`             | ラベル無しで Close された数                                               |
| triage       | `oldest_untriaged_days`       | 最古の未 triage Issue の経過日数                                          |
|              | `reject_latency_days_p50`     | 起票 → `rejected` で Close の中央値                                       |
| PR 化        | `pr_created_rate`             | PR が存在する adopted Issue / adopted Issue（ラベルではなく join で判定） |
|              | `pr_pending`                  | adopted かつ Open かつ PR 無し（PR 化待ち）                               |
|              | `pr_lag_days_p50`             | 起票 → 最初の PR 作成の中央値                                             |
| マージ       | `merge_rate`                  | merged / (merged + 未マージ Close)。Open な PR は分母に入れない           |
|              | `merge_lead_days_p50` / `p90` | PR 作成 → マージ                                                          |
|              | `e2e_lead_days_p50`           | 起票 → マージ                                                             |

- 分位点は order statistics の線形補間（R type 7）。母数が偶数のときの p50 は中央 2 値の平均。
- 日数は小数第 1 位、率は小数第 4 位で丸める。
- `oldest_untriaged_days` と `pr_pending` だけは**窓を無視して全期間**で数える。窓から外れて
  見えなくなる滞留こそがアラートの対象だから。

## 閾値アラート

母数 `--alert-min-sample`（既定 8）以上のセルでのみ発火する。緩めから始め、常時点灯するものは
緩めるか指標ごと落とす。

| kind                     | 条件                                                  | 名指しするプロンプト             |
| ------------------------ | ----------------------------------------------------- | -------------------------------- |
| `liveness`               | 直近 28 日の起票が 0 件                               | 該当スキャンのプロンプト         |
| `triage_backlog`         | 最古の未 triage が 14 日超（母数条件なし・repo 全体） | なし（人間の triage）            |
| `adopted_rate`           | 採用率 < 0.6                                          | 該当スキャンのプロンプト         |
| `rejected_after_pr_rate` | PR 後却下率 > 0.10                                    | 該当スキャンのプロンプト         |
| `pr_created_rate`        | PR 化率 < 0.8 かつ PR 化待ち 3 件以上                 | `weekly-adopted-issue-pr-bot.md` |
| `merge_rate`             | マージ率 < 0.8                                        | `weekly-pr-care-bot.md`          |

`liveness` は既存 digest では検知できない視点（Routine が静かに死んでも滞留は増えない）。
アラートの母数判定は `--min-sample` に影響されない（表示用の閾値でアラートが黙らないように、
生カウントから率を再計算している）。

## 出力

- `markdown` — digest 本文のフロー節。スキャン品質 / triage と PR 化 / マージ / 月次トレンド /
  ラベル運用の異常、末尾に `<!-- pipeline-metrics:json -->` マーカー付きの JSON ブロック。
- `alerts` — GitHub の `> [!WARNING]` ブロック。閾値超過が無ければ **0 バイト**を出す
  （digest workflow はこの空判定で `alert` ラベルを付け外しする）。
- `json` — `Report` 全体。トップレベルは `scans` / `total` / `months` / `backlog` / `alerts` /
  `anomalies`。フィールド名はメタループとの契約で、`cmd/pipeline-metrics/main_test.go` の
  `TestJSONContract` が固定している。

## 構成

```text
scripts/pipeline-metrics/
  cmd/pipeline-metrics/main.go  # フラグ解釈 → 入出力
  internal/model/               # gh JSON の正規化（triage 判定・PR join）
  internal/metrics/             # 集計・分位点・月次コホート・閾値評価（純粋関数）
  internal/render/              # 警告ブロック / フロー節 Markdown / JSON
  testdata/                     # フィクスチャとゴールデン
```

標準ライブラリのみに依存する。

## 開発

```sh
mise run pipeline-metrics:test
mise run pipeline-metrics:lint
mise run pipeline-metrics:run -- --issues issues.json --prs prs.json

# 出力を変えたときはゴールデンを更新する（差分をレビューすること）
cd scripts/pipeline-metrics && go test ./... -update
```

`testdata/issues.json` / `prs.json` は実データのパターン（実装後却下・追跡外 Close・
`scan:` ラベル重複・meta ブランチ・集計起点より前の Issue・ラベルと PR の食い違い）を再現した
合成データで、4 種のアラート（`liveness` / `triage_backlog` / `adopted_rate` / `pr_created_rate`）を
実際に発火させる。

## 制約

- **スキャン名 → プロンプトファイルの対応は `internal/metrics` にハードコード**している
  （`scanPrompts`）。スキャンを増やしたらここも足す。未知のスキャンはアラート文からファイル名が
  落ちるだけで、集計は壊れない。
- `adopted` が付いた**時刻**は取れない（Issue は Open のままなので `closedAt` が使えず、
  `updatedAt` は無関係なイベントでも動く）。`adopt_latency` は GraphQL の `LABELED_EVENT` が要る。
- 1 回の `gh --limit 1000` を超える規模になったら `--since` で窓を切る必要がある。
