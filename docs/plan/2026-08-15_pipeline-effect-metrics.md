# Plan: 自律改善パイプラインの効果測定ループ

自律改善ループ（スキャン → triage → PR Bot → PR Care Bot → マージ）の成果を決定論的に集計し、週次 digest に出し、月次メタループの改善テーマ選定に還す。観測して終わりのダッシュボードではなく、測定を次の改善判断に接続する閉ループを作る。

## Background

ループは回っているが、**効果が数値として残っていない**。

- `docs/slides/2026-07-08_claude-routine-loop-engineering.md` の「採用率 約 93%」「auto PR 25 件中 23 件マージ」は、人が一度手で数えた値。
- `.github/workflows/pipeline-digest.yml` が出すのは「今の滞留（ストック）」だけで、採用率・リードタイム・マージ率といった**フロー指標**が無い。
- メタループ（`routines/prompts/monthly-routine-improve.md`）は rejected コメントを定性的に読むだけで、「どのスキャンが効いていないか」を数値で特定できない。

実データ（2026-08-15 時点、`scan:*` Issue 91 件）を手で集計すると、この不在が実害であることが分かる。

| スキャン           | 採用率         |
| ------------------ | -------------- |
| `scan:ci`          | 100% (8/8)     |
| `scan:environment` | 100% (8/8)     |
| `scan:ai-agents`   | 89% (8/9)      |
| `scan:nvim`        | 81% (43/53)    |
| `scan:scripts`     | **62% (8/13)** |

全体では 82% でも、`scan:scripts` だけが明確に落ちている。この差は誰も気づいていなかった。さらに `rejected` かつ `pr-created`（PR まで作ってから捨てた最もコストの高い失敗）が 2 件（#518 / #446）、`adopted` なのに `pr-created` が付かないまま Close された Issue が 4 件（#528 / #509 / #508 / #435）実在する。

意図する成果は「digest を見れば弱っているスキャンが分かり、メタループがそこを起点に改善し、翌月に効果を数値で答え合わせできる」状態。

## Current structure

- `.github/workflows/pipeline-digest.yml` — 毎週土曜 7:00 JST。checkout なし、`gh` + `jq` のみのインライン bash 1 step。`digest` ラベルの単一 Issue の body を上書きする（Issue を増やさない）。
- `routines/prompts/monthly-routine-improve.md` — 毎月 2 日。定性材料から `routines/prompts/*.md` の改善を draft PR で 1 テーマだけ提案する。プロンプト本文の変更はマージだけで次回実行に反映される。
- `scripts/` の Go モジュール 6 本。`module <dir名>` / `go 1.26` / 標準ライブラリのみ。`agent-stats` が「寛容パース層 → 純粋な集計関数 → 複数レンダラ」+ 時刻の引数注入という同性質の先行事例。
- `.github/workflows/go_module_ci.yml` を `ci_<name>.yml` が `with: module: <name>` で呼ぶだけ。`mise.toml` に `<name>:{build,test,lint,clean}`。

## Design policy

**状態ファイルを持たず、毎回全再計算する。** `pins:sync` / `pins:check` のイディオムは「repo 内の入力から derive される成果物」専用で、外部の可変状態を入力にするメトリクスには使えない（毎回 diff が出て `--exit-code` が常に赤くなる）。月次トレンドも `createdAt` の年月でコホート分けして全期間を毎回計算すれば履歴ファイルは要らない。タイムスタンプは不変なので過去月の値は自然に安定する。ラベルを後から編集すれば過去の数値も動くが、それは source of truth に忠実であるという意味で正しい挙動。

**集計は新規 Go CLI `scripts/pipeline-metrics` に置く。** 中央値・ラベル別クロス集計・月次コホート・閾値評価を jq で書くとテストできない 200 行になり、必ず腐る。`scaffold` と `go_module_ci.yml` のレールがあるためモジュール追加コストは構造的に低い。

**CLI は `gh` を呼ばない。** JSON をファイル / stdin で受け取る純粋な集計器にする。テストがネットワーク・認証・レート制限から切り離され、`testdata/` を置くだけでゴールデンテストが書ける。取得は既に digest ワークフローが `gh` でやっている。CLI は一切書き込まない。

**出力は既存 digest Issue に統合する。** Issue を増やさない方針を維持し、人が既に見る場所にストックとフローを並べる。末尾に JSON ブロックを埋め込み、メタループが `gh issue list --label digest --json body` で取って決定論的にパースできるようにする（Actions artifact は保持期限とダウンロード手順があり、クラウド Routine から扱いにくい）。

**受動的なダッシュボードにしない。** 閾値アラート（body 冒頭の警告ブロック + `alert` ラベルの自動付け外し）と、メタループへの「テーマ優先付け + 予告と答え合わせ」で閉ループにする。

**人間の triage ゲートは維持する。** メトリクスに基づく自動 `adopted` 付与・自動マージ・自動 close は一切しない（`docs/plan/2026-07-04_routines-pipeline-expansion.md` で見送り済み）。出力先は「人が読む digest」と「draft PR を出すメタループ」に限る。

## 指標定義

入力は 2 つの JSON のみ。Search API（1000 件上限・インデックス遅延）は使わず、`auto/*` の絞り込みはローカルで前方一致させる。

```sh
gh issue list --state all --limit 1000 --json number,title,state,labels,createdAt,closedAt > issues.json
gh pr list   --state all --limit 1000 --json number,title,state,headRefName,createdAt,closedAt,mergedAt > prs.json
```

正規化ルール（`internal/model`、テストで固定）:

- スキャン Issue = `scan:` 前置ラベルを持つもの。複数あれば辞書順先頭に寄せ、異常として記録する。
- `createdAt >= --since`（既定 `2026-06-28`）のみ率の分母に入れる。それ以前は件数だけ数える。
- triage 状態は adopted / rejected / untriaged / `untracked_close`（ラベル無しで Close = 運用崩れ）の 4 値。
- **`adopted` と `rejected` が併存する Issue は rejected に倒す。** 「採用して実装まで進めたが最終的に捨てた」ケース。
- PR ↔ Issue の join は `^auto/issue-(\d+)-` から。マッチしない `auto/*` は meta ブランチとして別勘定。

指標はスキャン別に分解する（どのスキャンが効いていないかの特定が目的のため）。段階ごとに、スキャン品質（`opened` / `opened_last_28d` / `adopted_rate` / `rejected_after_pr_rate` / `untracked_close`）、triage（`oldest_untriaged_days` / `reject_latency_days_p50`）、PR 化（`pr_created_rate` / `pr_lag_days_p50`）、マージ（`merge_rate` / `merge_lead_days_p50` / `p90` / `e2e_lead_days_p50`）。定義と計算式は `scripts/pipeline-metrics/README.md` に置く。

小標本対策として、母数 5 件未満（`--min-sample`）のセルは率を出さず `— (n=3)` と表示し、閾値アラートは母数 8 件以上でのみ発火させる。既定窓は 90 日なので、週次で再計算しても率はほとんど動かない。

閾値は `adopted_rate < 0.6` / `rejected_after_pr_rate > 0.10` / `merge_rate < 0.8` / `pr_created_rate < 0.8` かつ PR 化待ち 3 件以上 / 直近 28 日の起票が 0 件 / 最古の未 triage が 14 日超。**「直近 28 日の起票が 0 件」は既存 digest では検知できない**（Routine が静かに死んでも滞留は増えないため）視点で、単独でも導入価値がある。警告文は責任範囲のプロンプトファイルを名指しする。

## Implementation steps

### Phase 0 — ラベル運用の明文化

1. `routines/README.md` に「ラベル運用の約束（効果測定の前提）」を追加する。`rejected` は Close と同時に付ける（`closedAt` を判定時刻として使うため）、`adopted` は後から外さず実装後に捨てる場合は `rejected` を足す、`scan:*` は 1 Issue に 1 つ、集計起点は 2026-06-28 で遡及ラベル付けはしない。

### Phase 1 — 測る + 警告 + メタループに還す

1. `scripts/pipeline-metrics` を追加する。`cmd/pipeline-metrics`（フラグ解釈・入出力）、`internal/model`（正規化）、`internal/metrics`（集計・分位点・月次コホート・閾値評価）、`internal/render`（警告ブロック / フロー節 / JSON）。`--now` を注入可能にし、`testdata/` のフィクスチャに対するゴールデンテストで出力を固定する。フィクスチャは実データの各パターン（実装後却下・untracked close・scan ラベル重複・meta ブランチ・集計起点より前の Issue）を再現する。
2. `.github/workflows/ci_pipeline_metrics.yml` を追加し、`go_module_ci.yml` を呼ぶ。
3. `mise.toml` に `pipeline-metrics:{build,run,test,lint,clean}` を追加し、`go:test` / `go:lint` の `depends` を更新する。
4. `.github/workflows/pipeline-digest.yml` を拡張する。checkout + mise を足し、`gh` で JSON を取得 → CLI を 2 回呼んでフロー節と警告ブロックを作り → 既存のストック節と連結して upsert → 警告の有無で `alert` ラベルを付け外しする。**CLI が失敗しても digest の公開自体は成立させる**（`continue-on-error` + 生成物の存在チェック）。ストックの jq は Phase 1 では触らない。
5. `routines/prompts/monthly-routine-improve.md` を、メトリクスで対象を絞ってから定性で裏を取る手順に書き換える。PR body に「根拠にしたメトリクス」と「検証予告」を必須項目として足し、翌月の実行が冒頭でその答え合わせをする。

### Phase 2 — 精度と単純化

1. ストック集計を `internal/render` に移し、`pipeline-digest.yml` の jq を削除する。**untriaged の定義が bash と Go に二重実装されている Phase 1 の負債を解消する。**
2. GraphQL の `LABELED_EVENT` を取得し、`adopt_latency_days`（起票 → `adopted` 付与）を出す。Issue は Open のままなので `closedAt` が使えず、`updatedAt` は無関係なイベントでも動くため代理にしない。
3. `first_pass_yield`（PR の `commits.totalCount == 1` かつ `reviews.totalCount == 0` の率）を追加し、PR Care Bot の負荷を測る。

### Phase 3 — 発展（Phase 1〜2 の実績を見てから判断）

1. メタループの「予告 vs 実測」の結果を digest にも 1 行残す。
2. スキャン側プロンプトへのフィードバック。**LLM が数値を追いかけて提案を歪めるリスクがあるため保留。**
3. 月確定時のスナップショットを `docs/metrics/YYYY-MM.json` にアーカイブ。state を再導入するので既定ではやらない。

## File changes

| File                                             | Change                                                                                                |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| `scripts/pipeline-metrics/go.mod`                | 新規（`module pipeline-metrics` / `go 1.26` / 標準ライブラリのみ）                                    |
| `scripts/pipeline-metrics/cmd/pipeline-metrics/` | 新規（`main.go` + ゴールデンテストを含む `main_test.go`）                                             |
| `scripts/pipeline-metrics/internal/model/`       | 新規（`gh` JSON の正規化と triage 判定）                                                              |
| `scripts/pipeline-metrics/internal/metrics/`     | 新規（集計・分位点・月次コホート・`EvaluateAlerts`）                                                  |
| `scripts/pipeline-metrics/internal/render/`      | 新規（警告ブロック / フロー節 Markdown / JSON）                                                       |
| `scripts/pipeline-metrics/testdata/`             | 新規（`issues.json` / `prs.json` / `*.golden`）                                                       |
| `scripts/pipeline-metrics/README.md`             | 新規（指標定義・閾値・制約・read-only 宣言）                                                          |
| `.github/workflows/ci_pipeline_metrics.yml`      | 新規（`go_module_ci.yml` 呼び出し）                                                                   |
| `.github/workflows/pipeline-digest.yml`          | checkout + mise + CLI 実行、警告ブロック / フロー節 / JSON、`alert` ラベル制御、`pull-requests: read` |
| `mise.toml`                                      | `pipeline-metrics:*` タスク追加、`go:test` / `go:lint` の `depends` 更新                              |
| `routines/prompts/monthly-routine-improve.md`    | メトリクス起点の分析手順、予告と答え合わせ、数値に振り回されないための制約                            |
| `routines/README.md`                             | digest の説明更新、ラベル運用の約束を明文化                                                           |

`routines/*.json` は変更しない（プロンプト本文の変更のみなので手動 apply は不要で、マージすれば次回実行から反映される）。

## Risks and mitigations

| Risk                                                   | Mitigation                                                                                                                                                                |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 個人リポの小標本で比率がノイズになる                   | 母数 5 件未満は率を出さない。アラートは母数 8 件以上。既定窓 90 日で週次でも率はほぼ動かない                                                                              |
| ラベル運用の歴史的な揺れ（`rejected` 運用前の期間）    | `--since 2026-06-28` を既定に。それ以前は件数のみ出し、率の分母に入れない                                                                                                 |
| 将来また運用が崩れる                                   | `untracked_close` を常時表示し、崩れを数値で検知する。前提を `routines/README.md` に明文化                                                                                |
| `adopted` と `rejected` の併存（#518 / #446 実在）     | rejected に倒すルールをテストで固定し、`rejected_after_pr_rate` として積極的に指標化する                                                                                  |
| scan 以外の手動 Issue の混入（#523 / #524）            | `scan:*` を持つ Issue のみ分母にする。混入は `anomalies.triaged_non_scan` に出す                                                                                          |
| GitHub API のページング上限                            | `gh --limit 1000`（内部で自動ページング）。1000 件を超えたら `--since` で窓を切る                                                                                         |
| メタループが数値に振り回されて的外れな改善をする       | 「数値は絞り込み用、変更理由は定性から導く」「食い違えば変更なしで終える」を明記。出力は draft PR 止まり・月 1 件・最小差分。予告と答え合わせで外れた改善が翌月に露見する |
| 過去の数値が後から変わる（ラベル編集）                 | 「毎回再計算・スナップショット無し」を設計として明示し、digest に生成時刻と集計起点を出す                                                                                 |
| digest workflow が checkout + mise で重くなる / 壊れる | `timeout-minutes: 10` 維持。CLI 失敗時もストック節だけで digest を公開し、本文に生成失敗を明記する                                                                        |
| ストック定義の二重実装（bash jq と Go）                | Phase 1 の既知の負債として記録し、Phase 2 で jq を削除して一本化する                                                                                                      |
| アラート疲れ（常時点灯して無視される）                 | 閾値は緩めから始める。常時点灯するものは緩めるか指標ごと落とす。`alert` ラベルは回復時に自動で外れる                                                                      |

## Validation

- [x] `go test ./...`（`mise run pipeline-metrics:test` 相当）が通る
- [x] フィクスチャ + `--now` 固定でゴールデン Markdown が一致し、2 回実行して同一出力になる
- [x] 境界ケース（空入力 / 全 untriaged / 母数 1 / join 不能ブランチ / `adopted`+`rejected` 併存 / 偶数長の中央値 / 集計起点より前のデータ）がテーブル駆動テストで固定されている
- [x] digest に埋め込む JSON が単体で `json.Unmarshal` でき、メタループが選ぶフィールド名が契約としてテストされている
- [x] フィクスチャが `adopted_rate` / `liveness` / `pr_created_rate` / `triage_backlog` の 4 種のアラートを実際に発火させる
- [ ] `workflow_dispatch` で digest Issue が 1 件のまま更新され、警告ブロック + ストック節 + フロー節 + 月次 + JSON が出る（マージ後に実行して確認）
- [ ] 閾値超過時に `alert` ラベルが付き、回復すると外れる（マージ後に確認）
- [x] 実データでの手計算と CLI 出力が一致する: スキャン別採用率（nvim 43/53・scripts 8/13・ai-agents 8/9・ci 8/8・environment 8/8）、`auto/*` merged 62 / 未マージ close 4、`rejected ∧ pr-created` 2 件（#518 / #446）

手計算は集計起点を切らずに数えていたため、照合は `--since 2026-01-01 --window 0` で実行した。2026-08-16 時点のデータ（手計算の翌日で nvim / ai-agents が 1 件ずつ増えている）に対し nvim 43/54・scripts 8/13・ai-agents 8/10・ci 8/8・environment 8/8、合計 75/93、`rejected ∧ pr-created` 2 件、未マージ close 4 で一致した。既定（`--since 2026-06-28`）では分母から起点前の 6 件が落ちる。

## Decisions

- 実装形態は新規 Go CLI（jq 拡張・`agent-stats` 拡張は不採用）。`agent-stats` はローカル transcript しか見えず、クラウド Routine のトークン・コストは原理的に測れないため流用しない。踏襲するのは設計イディオムのみ。
- 出力先は既存 digest Issue に統合（独立 Issue・リポジトリへのコミットは不採用）。
- メタループへの還元は「閾値アラート + テーマ優先付け」。数値でテーマを強制はしない。
- 集計起点は `2026-06-28`（`rejected` 運用の定着後。最古の rejected #446 の週は過渡期のため含めない）。
- 閾値超過で Actions job を失敗させる通知は入れない（CI バッジが赤くなる）。`alert` ラベルで気づけないと分かった時点で再検討する。
- `scan:*` 以外の手動 Issue は集計対象外（パイプラインの効果測定という趣旨に忠実に）。

## Open questions

- 「人が再スコープして実装した」ケース（#517 / #521）は現状 `adopted` に埋もれる。`reworked` ラベルを足せばスキャン品質の解像度が上がるが、triage の手間が 1 手増える。Phase 2 以降で導入するか。
