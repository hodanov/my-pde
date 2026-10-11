# Plan: Claude Code OTel 分析基盤 — セッション単位のドリルダウン

## Context

`Claude Code Usage` ダッシュボード（`environment/otel/dashboards/claude-code.json`）は期間全体の俯瞰しかできない。
1 セッションの中で「壁打ち → Plan 出力 → dev-workflow 実行 → 検証・hooks → 完了」がどう進み、どこに何分かかったかを、SessionID ごとのレイテンシーブレークダウンとして見たい。

### 実データで確認済みの前提（2026-10-11、Loki / Prometheus を直接クエリ）

- Loki のインデックスラベルは `service_name` だけ。他の属性はすべて structured metadata で、`| session_id="..."` のようにフィルタできる。
- 全イベントに `session_id`・`prompt_id`・`event_sequence`・`event_timestamp` が付く。`prompt_id` は 1 回のユーザー入力に紐づくイベントで共通。
- フェーズの境界はイベントから取れる:
  - Plan mode: `permission_mode_changed`（`to_mode=plan` / `trigger=shift_tab` で開始、`from_mode=plan` / `trigger=exit_plan_mode` で終了）
  - Skill: `skill_activated`（`skill_name=dev-workflow`, `invocation_trigger=nested-skill|user-slash`）。`api_request` にも `skill_name` が付き、Skill 内の API 時間を直接集計できる
  - スラッシュコマンド: `user_prompt` の `command_name`
  - hooks: `hook_execution_complete` の `hook_name`（`Stop`, `PreToolUse:Bash`, `UserPromptSubmit` …）と `total_duration_ms`。Stop hook は 0.1〜6.7 s の幅がある
- `api_request` に `duration_ms`・`ttft_ms`・`query_source`、`tool_result` に `duration_ms`・`success` が付く。
- Prometheus の全メトリクスに `session_id` ラベルが付く。`claude_code_active_time_seconds_total{type="user|cli"}`、`commit_count`、`pull_request_count`、`lines_of_code_count` もある。
- Tempo は空（traces 未有効）。traces (beta) は 1 プロンプト = 1 trace（root `claude_code.interaction`、子に `llm_request` / `tool` / `tool.blocked_on_user` / `tool.execution`、サブエージェントは Agent の `tool` span の下にネスト）。`claude_code.hook` span は organization の allowlist が要る detailed beta 限定なので、**hooks は traces を足しても Loki から取る**。

## 方針

Loki + Prometheus だけでセッション詳細ダッシュボードを作り（Phase 1）、traces でプロンプト内のウォーターフォールを足す（Phase 2）。概要ダッシュボードには、セッション一覧と改善に繋がる指標を足す（Phase 3）。
ADR-0008 の見直し条件「サブエージェントの所要時間をウォーターフォールで見たくなった場合」に Phase 2 が該当するので、ADR-0008 を追記する。

### Phase 1: セッション詳細ダッシュボード（設定変更なし）

新規 `environment/otel/dashboards/claude-code-session.json`（uid `claude-code-session`）。既存 provider がディレクトリ監視しているので、プロビジョニングの変更は要らない。

- 変数 `$session`: Prometheus `label_values(claude_code_session_count_total, session_id)`。Loki の label values API はインデックスラベルしか返さないため Prometheus から取る。
- 上段 stat（セッション全体）
  - 壁時計時間（最初〜最後のイベント）、プロンプト数、コスト、cache hit 率
  - 人間 / Claude の時間: `active_time{type="user"}` と `{type="cli"}`
  - 累積時間の内訳: API（`api_request` の `duration_ms` 合計）、ツール（`tool_result`）、hooks（`hook_execution_complete` の `total_duration_ms`）
- **プロンプト別ブレークダウン表**（本命）: 1 行 = 1 `prompt_id`
  - 列: 開始時刻、コマンド / 起動 Skill、Plan mode 中か、壁時計時間、API 時間、ツール時間、hook 時間（Stop hook は別列）、API 回数、ツール失敗数、コスト
  - 実装: LogQL instant クエリを `sum by (prompt_id)` で複数発行して `joinByField`（既存ダッシュボードと同じ手法）。壁時計時間は raw ログを `groupBy(prompt_id)` で Time の min/max を取り、`calculateField` で差分を出す。
  - 注記: 並列ツール・並列サブエージェントで累積時間が壁時計を超えうる。「累積」と明記する。
- **フェーズ帯（state timeline）**: Plan mode の on/off と、起動中 Skill を時間軸の帯で表示する。`permission_mode_changed`・`skill_activated`・`user_prompt` を時系列で取り、モードは次の変更まで値を保持（state timeline の既定動作）。
- **プロンプト別積み上げ棒**: API / ツール / hook / その他（壁時計 − 累積の残り。待ち・思考・人間の入力を含む）を横棒で並べ、どのターンが重いかを一目で見る。
- **イベントタイムライン表**: セッションの全イベントを `event_timestamp` 昇順（同時刻は `event_sequence`）で並べる。列: 時刻、イベント、名前（skill / tool / hook / command）、`duration_ms`、`prompt_id`。hook_registered / plugin_loaded など起動時ノイズは既定で除外。
- 概要ダッシュボードへの導線: 既存 `claude-code.json` に「セッション一覧」表を追加し（開始時刻、repo、プロンプト数、コスト、active time、PR 数）、`session_id` 列のデータリンクで `/d/claude-code-session?var-session=${__value.raw}&${__url_time_range}` に飛ぶ。

### Phase 2: traces (beta) でプロンプト内ウォーターフォール

- `ai-agents/settings/claude/settings.json` の `env` に `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`、`OTEL_TRACES_EXPORTER=otlp` を追加。エンドポイントは既存の `OTEL_EXPORTER_OTLP_ENDPOINT` を共有する。Tempo は LGTM に同梱済みでバックエンド変更は不要。
- セッション詳細ダッシュボードに Tempo パネルを追加: TraceQL で `claude_code.interaction` span を `session.id = "$session"` で検索し、行クリックでウォーターフォールを開く。属性名（resource か span か）は実データで確定させてから書く。
- traces でしか取れない指標を追加: **承認待ち時間**（`claude_code.tool.blocked_on_user` の `duration_ms`）と、サブエージェント内部の llm / tool 内訳。
- プロンプト別ブレークダウン表の `prompt_id` → trace を引く手段: `interaction` span に `prompt.id` 属性があるかを確認する。無ければ時刻範囲で検索するリンクで代用する。
- ADR-0008 に、traces を有効化したことと hook span は allowlist 制のため Loki 側で見る旨を追記する。

### 汎用性の制約（チーム共有）

このダッシュボード群は職場のチームにも共有する。特定の Skill 名・hook 名・repo 名・個人のワークフロー（dev-workflow など）をクエリやパネルに書き込まない。名前はすべて `by (skill_name)` などの集計キーとして扱い、誰の環境でもそのまま動くようにする。絞り込みが要るものは変数（`$repo`、`$skill`）にする。

### Phase 3a: Skill 改善に繋がる指標（新しい行「Skill 品質」）

どの Skill でも同じ物差しで比べられ、SKILL.md の改善アクションに直結するものに絞る。いずれも既存のイベントから取れる。

| 指標                                                                                                                             | ソース / 作り方                                                                                                       | 繋がる改善                                                                        |
| -------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| **起動経路の内訳**: Skill 別の `invocation_trigger`（`user-slash` / `claude-proactive` / `nested-skill`）比率                    | Loki `skill_activated`                                                                                                | 自動起動がほぼ無い Skill は description のトリガー語が弱い。description を直す    |
| **1 起動あたりのコスト・API 回数・API 時間**（p50 / p95）                                                                        | Loki `api_request` の `skill_name` で `sum by (skill_name)`、起動回数で割る                                           | 重い Skill は手順の削減、モデル指定（Haiku 化）、サブエージェント化を検討         |
| **Skill 実行中のツール失敗率**                                                                                                   | `skill_activated` と同じ `prompt_id` の `tool_result{success=false}` 件数 / 総数（Grafana の join で prompt_id 結合） | 失敗の多い手順（存在しないコマンド・パス）を SKILL.md から直す                    |
| **Skill 実行中の権限プロンプト数**                                                                                               | 同じ `prompt_id` の `tool_decision` で、ユーザー判断のもの（`source` の値は実データで確定）                           | Skill が使うコマンドを allowlist に入れる、または `allowed-tools` を Skill に足す |
| **Skill の連鎖**: 親 Skill → 子 Skill（`nested-skill`）の組と回数                                                                | 同じ `prompt_id` 内の `skill_activated` を `event_sequence` 順に並べる                                                | 想定どおり子 Skill を呼べているか（呼ばれない連携は指示の見直し）                 |
| **手戻りシグナル**: Skill 起動プロンプトの直後に `/rewind` が来た回数、または同じ Skill がセッション内で短時間に再起動された回数 | Loki `user_prompt{command_name="rewind"}` と `skill_activated` の時系列。誤検知が多ければ採用しない                   | 期待どおりに動かなかった Skill の候補抽出（`skill-observe` の記録対象を選ぶ）     |
| **改善前後の比較**: 上の指標を Skill 別に週次の時系列で表示                                                                      | 同上を range クエリで                                                                                                 | SKILL.md を変えた週の前後で、コスト・失敗率・自動起動率が動いたかを見る           |

起動 0 回の Skill（未使用 Skill）は、インストール済み Skill の一覧がテレメトリに無いため（`plugin_loaded` は `skill_path_count` の件数だけ）ダッシュボードでは出せない。必要なら別途スクリプトで突き合わせる。本計画のスコープ外。

### Phase 3b: その他の汎用指標（概要ダッシュボード）

| 指標                                                                                  | ソース                                                    | 繋がる改善                                             |
| ------------------------------------------------------------------------------------- | --------------------------------------------------------- | ------------------------------------------------------ |
| hook オーバーヘッド率: hook 累積時間 / セッション壁時計、hook 別の p50/p95            | Loki `hook_execution_complete`                            | 毎ターン走る hook（Stop など）の固定費削減             |
| TTFT p50/p95（モデル別）                                                              | Loki `api_request` の `ttft_ms`                           | 体感の遅さの切り分け（生成時間か待ちか）               |
| 人間 vs Claude の時間比                                                               | Prometheus `active_time{type}`                            | 承認・入力待ちが多いワークフローの特定                 |
| 権限プロンプトの頻度: ツール別の `tool_decision{source="user_*"}` 件数                | Loki `tool_decision`                                      | allowlist 追加（`fewer-permission-prompts`）の候補抽出 |
| 成果あたりコスト・時間: PR 1 本 / commit 1 本あたりのコストと active time             | Prometheus `pull_request_count` / `commit_count` / `cost` | 作業の型ごとのコスト把握                               |
| cache 断絶: `cache_creation_tokens` が急増したリクエスト（`/compact` や長い放置の後） | Loki `api_request`                                        | 休憩・compact の運用                                   |

`tool_decision` の `source` の実際の値は実データで確認してから使う。

## 実装結果（計画との差分）

実データとブラウザでの描画を確かめながら作った結果、次の点を計画から変えた。

- フェーズ帯（state timeline）はやめ、時間軸パネル（区間ごとの API・ツール・hook の累積時間を積み上げ棒）に、Plan mode の切替・Skill の起動・スラッシュコマンドを注釈（縦線）で重ねた。Loki のイベントから「次の変更まで値を保持する」状態列を作る変換が脆いため。
- プロンプト別の積み上げ棒はやめた。Agent ツールの時間はサブエージェントの API 時間を含み、並列ツールも重複するため、カテゴリを積むと誤解を招く。ブレークダウン表のゲージで代える。
- ブレークダウン表の「Plan mode 中か」列はやめた。`permission_mode_changed` の `prompt_id` は切替の直前のプロンプトを指し、対象プロンプトを正しく示せない。Plan mode は注釈とイベントタイムラインで見る。
- プロンプト別の壁時計時間は、LogQL の `` label_format ts_ms=`{{ __timestamp__ | unixEpochMillis }}` `` を `unwrap` して `max - min` で出した（Grafana 変換は不要だった）。
- Skill の連鎖と手戻りシグナルは見送った。親子や「直後の /rewind」は時系列の順序に依存し、LogQL の集計では表せない。
- 改善前後の比較は、直近 1 日の窓を 6 時間ごとに評価する時系列にした。step を 1d にすると Grafana の Loki クエリ分割と噛み合わず、ブラウザで空になる。
- cache 断絶は、Prometheus の `cacheCreation` / `cacheRead` トークンの増分を時系列の棒で出した。
- traces では、`session.id` が span 属性に付き `prompt_id` は付かないことを確認した。セッション詳細に、承認待ち時間（`blocked_on_user` の合計）、サブエージェント内の API 時間（`agent_id` のある `llm_request`、`query_source_safe` 別）、trace 一覧（Trace ID から Explore のウォーターフォールへ）を置いた。
- サブエージェント（レビューで追加）: `subagent_completed` は完了時に 1 件出て、`prompt_id`・`duration_ms`・`total_tool_uses`・`total_tokens`・`is_async` を持つ。traces を有効にするとイベントにも `trace_id` が付く。セッション詳細に 1 起動 1 行の一覧（開始 = 完了時刻 − 所要時間、trace へのリンク）を置き、ブレークダウン表にプロンプト別の Agent 数・Agent 時間、時間軸に起動（Agent の `tool_decision`）と完了の注釈を足した。起動元の Skill はイベントに無いので、`prompt_id` でブレークダウン表の行と突き合わせる。

## 対象ファイル

- `environment/otel/dashboards/claude-code-session.json` — 新規（Phase 1・2）
- `environment/otel/dashboards/claude-code.json` — セッション一覧表とリンク（Phase 1）、追加指標（Phase 3）
- `ai-agents/settings/claude/settings.json` — traces の env 2 行（Phase 2）
- `docs/adr/0008-use-grafana-lgtm-for-local-claude-code-telemetry.md` — traces 有効化を追記（Phase 2）
- `docs/plan/2026-10-11_claude-code-otel-session-drilldown.md` — 本プランを repo に保存
- `AGENTS.md` — telemetry 節にセッション詳細ダッシュボードを 1 行追記
- 触らない: `environment/otel/docker-compose.yml`、`provisioning/`、`scripts/agent-stats`

## 進め方

1. `dev-workflow` skill を起動し、ブランチ `feat/otel-session-drilldown` を切る（main に未コミット変更があるので巻き込まない）。
2. Phase 1: 各パネルのクエリを、Grafana の API（`/api/datasources/proxy/uid/loki|prometheus/...`）で実データに当てて結果を確かめてから JSON にする。保存すると provider が 10 秒で再読込する。
3. Phase 2: settings を変更し `mise run settings-copy` で配布。ユーザーが Claude Code を再起動した後、Plan mode → Skill 起動 → サブエージェント → Stop hook を通る短いセッションを回し、Tempo の span 属性名を確定させてからパネル化する。
4. Phase 3: 指標ごとにクエリを実データで確認しながら追加する。追加後、ダッシュボード JSON を `rg` で走査し、個別の Skill 名・hook 名・repo 名がハードコードされていないことを確認する。
5. ドキュメント更新 → `commit-and-draft-pr` でコミットとドラフト PR。Phase ごとにコミットを分ける。

## 検証

- 既存セッション `9c1ef0eb-…`（Plan mode → exit_plan_mode、Stop hook あり）を `$session` に選び、ブレークダウン表のプロンプト数と API 合計時間を、Loki の生クエリ結果（Python 集計）と照合する。
- 時間軸パネルの Plan mode 注釈が、`permission_mode_changed` の 2 イベントの時刻と一致する。
- 概要ダッシュボードのセッション一覧のリンクから、時間範囲を保ったまま詳細ダッシュボードに遷移できる。
- Phase 2: Tempo にトレースが入り、`claude_code.interaction` から Agent の `tool` span 配下にサブエージェントの span がネストして見える。`blocked_on_user` の値が AskUserQuestion の待ち時間と概ね一致する。
- `curl` で `GET /api/dashboards/uid/claude-code-session` がプロビジョニング済みとして返る。
- `mise run lint` と `mise run verify:changed`（prettier が JSON、markdownlint が md）。
