# pipeline-metrics

`routines/` の自律改善パイプライン（スキャン → triage → PR Bot → マージ）が**どれだけ効いているか**を、
GitHub 上の Issue / PR から決定論的に集計する **read-only** の CLI。

`.github/workflows/pipeline-digest.yml` から呼ばれ、週次 digest Issue に「成果（フロー）指標」と
閾値警告を出す。その出力（末尾の JSON ブロック）を月次のメタループ
（`routines/prompts/monthly-routine-improve.md`）が読み、どのスキャンを改善すべきかの起点にする。

## 設計

- **`gh` を呼ばない。** JSON をファイル or stdin で受け取るだけの純粋な集計器。取得はワークフロー側の
  責務にしてあり、テストがネットワーク・認証・レート制限から完全に切り離される。
- **一切書き込まない。** GitHub にも、ローカルにも。出力は stdout のみ。
- **状態ファイルを持たない。** GitHub が source of truth なので、実行のたびに全履歴を再計算する。
  タイムスタンプは不変なので過去月の値は自然に安定する（当月だけが暫定値）。
  ラベルを後から編集すれば過去の数値も変わるが、それは source of truth に忠実であるという意味で正しい。
- **時刻は注入する。** `--now` を渡せば出力が固定されるので、ゴールデンテストが書ける。
- 標準ライブラリのみに依存する。

## 使い方

```sh
gh issue list --state all --limit 1000 \
  --json number,title,state,labels,createdAt,closedAt > issues.json
gh pr list --state all --limit 1000 \
  --json number,title,state,headRefName,createdAt,closedAt,mergedAt > prs.json

go run ./cmd/pipeline-metrics --issues issues.json --prs prs.json --format markdown
```

| フラグ          | 既定         | 意味                                                     |
| --------------- | ------------ | -------------------------------------------------------- |
| `--issues`      | （必須）     | `gh issue list --json ...` の出力。`-` で stdin          |
| `--prs`         | （必須）     | `gh pr list --json ...` の出力。`-` で stdin             |
| `--since`       | `2026-06-28` | この日より前に起票された Issue を率の分母から外す        |
| `--now`         | 現在時刻     | 評価時刻（RFC3339）。テスト・再現用                      |
| `--window-days` | `90`         | フロー指標の集計窓                                       |
| `--months`      | `12`         | 月次トレンド表の月数                                     |
| `--min-sample`  | `5`          | 率を表示する最小の母数。これ未満は `—` と出す            |
| `--format`      | `markdown`   | `markdown`（フロー節）/ `alerts`（警告ブロック）/ `json` |

`--format alerts` は閾値超過が無ければ**空文字列**を返す。ワークフローはこの空判定で
digest Issue の `alert` ラベルを付け外しする。終了コードは警告の有無で変わらない
（警告は失敗ではない）。

## 集計対象と正規化

対象は `scan:*` ラベルを持つ Issue と、head が `auto/` の PR。

- **triage 状態**: `adopted` あり → adopted / `rejected` あり → rejected / どちらも無く Open → untriaged /
  どちらも無く Closed → `untracked_close`（ラベル運用が崩れたシグナル）。
- **`adopted` と `rejected` が併存する Issue は rejected に倒す。** 「一度採用して実装まで進めたが、
  最終的に捨てた」ケースであり、`rejected_after_pr_rate` の入力になる。
- **PR ↔ Issue の join** は PR Bot がブランチ名に埋める `auto/issue-<N>-<slug>` から行う。
  マッチしない `auto/*`（`auto/routine-improve-*` 等）は meta ブランチとして別勘定。
- `scan:*` を複数持つ Issue は辞書順で先頭のラベルに寄せ、`anomalies.multi_scan_label` に記録する。
- `--since` より前の Issue は件数（`pre_policy_issues`）だけ出し、率の分母には入れない。

## 指標

| 段階         | 指標                           | 計算                                                                        |
| ------------ | ------------------------------ | --------------------------------------------------------------------------- |
| スキャン品質 | `opened` / `opened_last_28d`   | 窓内の起票数と、直近 28 日の起票数（スキャンが生きているかの一次指標）      |
|              | `adopted_rate`                 | `adopted / (adopted + rejected)`。untriaged は分母から外す                  |
|              | `rejected_after_pr_rate`       | `rejected ∧ pr-created / pr-created`。最もコストの高い失敗                  |
|              | `untracked_close`              | ラベル無しで Close された件数（運用崩れの検知）                             |
| triage       | `oldest_untriaged_days`        | 未 triage で最も長く待っている Issue の日数                                 |
|              | `reject_latency_days_p50`      | `closedAt - createdAt`。`rejected` は close 時付与なので close が判定時刻   |
| PR 化        | `pr_created_rate`              | `adopted ∧ pr-created / adopted`                                            |
|              | `pr_lag_days_p50`              | Issue 起票 → PR 作成。PR Bot は日曜バッチなので 0〜7 日が理論値             |
| マージ       | `merge_rate`                   | `merged / (merged + 未マージ close)`。Open な PR は未確定なので分母から外す |
|              | `merge_lead_days_p50` / `_p90` | PR 作成 → マージ                                                            |
|              | `e2e_lead_days_p50`            | Issue 起票 → PR マージ。ループ全体の速度を表す単一の数字                    |

月次トレンドでは、Issue 側の指標は **Issue の起票月**、PR 側の指標は **PR の作成月**で束ねる
（両者は別のコホートなので混ぜない）。

## 閾値（アラート）

率の閾値は母数 8 件以上でのみ発火する。率を表示するのは 5 件以上（`--min-sample`）だが、
警告を出すのはより慎重にしている。

| 指標                     | 条件                            | 名指しするプロンプト                                           |
| ------------------------ | ------------------------------- | -------------------------------------------------------------- |
| `adopted_rate`           | `< 0.6`                         | 該当スキャンのプロンプト（選定基準が広すぎる）                 |
| `rejected_after_pr_rate` | `> 0.10`                        | 該当スキャン + `weekly-adopted-issue-pr-bot.md`                |
| `merge_rate`             | `< 0.8`（全体）                 | `weekly-adopted-issue-pr-bot.md` / `weekly-pr-care-bot.md`     |
| `pr_created_rate`        | `< 0.8` かつ PR 化待ち 3 件以上 | `weekly-adopted-issue-pr-bot.md`（PR Bot の実行失敗を疑う）    |
| `liveness`               | 直近 28 日の起票が 0 件         | 該当スキャン（ネタ切れ or Routine の静かな死。母数ガード無し） |
| `triage_backlog`         | 最古の未 triage が 14 日超      | なし（人間側のボトルネック）                                   |

## 制約

- **母数が小さい。** 個人リポの率は数件で大きく動く。率は必ず母数 `(n=..)` とセットで表示し、
  母数不足のセルは率を出さない。メタループ側にも「数値は絞り込み用、変更理由は定性分析から導く」
  と明記してある。
- **`gh --limit 1000` を前提にしている。** Search API（1000 件上限・インデックス遅延）は使わず、
  `auto/*` の絞り込みはローカルで前方一致させている。Issue が 1000 件を超えたら `--since` で窓を切る。
- **`adopted` ラベルが付いた時刻は取れない。** Issue は Open のままなので `closedAt` が使えず、
  `updatedAt` は無関係なイベントでも動くため代理にしていない。正確な triage リードタイムには
  `LABELED_EVENT` のタイムラインが要る（今後の拡張）。

## 構成

```text
scripts/pipeline-metrics/
  cmd/pipeline-metrics/main.go   # フラグ解釈 → デコード → 集計 → 出力
  internal/model/                # gh JSON → 正規化した Issue / PullRequest
  internal/metrics/              # 集計・分位点・月次コホート・閾値評価（純粋関数）
  internal/render/               # 警告ブロック / フロー節 Markdown / JSON
  testdata/                      # フィクスチャとゴールデン出力
```

ゴールデン出力を更新するには `go test ./cmd/pipeline-metrics -update`。
