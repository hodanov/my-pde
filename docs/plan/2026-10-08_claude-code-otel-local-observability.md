# Plan: Claude Code の OpenTelemetry 計測をローカル Grafana で分析する

## Background

Claude Code の使い方（どのモデル／Skill／Agent を使ったか、サブエージェントの処理にどれだけ時間がかかったか）を計測し、分析して改善に繋げたい。
Claude Code 本体の OpenTelemetry (OTel) 出力を有効化し、まずはローカルの Grafana スタックで見る。OTLP で受けるので、後から Datadog 等へ送り先だけ差し替えられる。

方針（バックエンド選定の理由と却下案は [ADR-0008](../adr/0008-use-grafana-lgtm-for-local-claude-code-telemetry.md)）:

- 可視化: `grafana/otel-lgtm`（Grafana + Prometheus + Loki + Tempo + Collector の 1 コンテナ）
- 初回は metrics + events(logs) のみ。traces (beta) は後回し
- 有効化は `ai-agents/settings/claude/settings.json` の `env` に常時 ON

## Current structure

- 設定の置き場: `ai-agents/settings/claude/settings.json`。`env` ブロックは既にある（`CLAUDE_CODE_USE_BEDROCK=1`、`AWS_PROFILE`、`DISABLE_TELEMETRY=1` など）。
- 配布: `mise run settings-copy`（`cp -p` 上書き、マージなし）。反映には Claude Code の再起動が必要（OTel の env は起動時に一度だけ読まれる）。
- プロジェクトの `.claude/settings.json` / `settings.local.json` の `env` では OTEL\_\* と `CLAUDE_CODE_ENABLE_TELEMETRY` は無視される。user settings（= 上記の配布先）か shell でのみ有効。
- 既存の `environment/docker/docker-compose.yml` は nvim 専用。別ディレクトリ `environment/otel/` に新設する。
- 既存の分析手段: `scripts/agent-stats`（transcript の事後バッチ集計）、permission-ledger hook。OTel は置き換えではなく補完（連続取得、API レイテンシ、`tool_decision`、サブエージェント所要時間など）。
- Claude の Bash サンドボックスからは Docker ソケットに触れない。コンテナの起動はユーザーが手元の端末で行う。
- sandbox の `network.allowedDomains` に localhost 許可は無いが、OTel エクスポートは Claude Code 本体プロセスから出るので不要の見込み（Validation で確認）。

## Design policy

### 知りたいことと取得元

| 知りたいこと                     | シグナル                                                               | 主な属性                                                 |
| -------------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------- |
| 使ったモデル別のトークン／コスト | metric `claude_code.token.usage` / `claude_code.cost.usage`            | `model`, `type`, `query_source`(main/subagent/auxiliary) |
| Skill の利用                     | event `claude_code.skill_activated`、上記 metric の `skill.name`       | `skill.name`, `invocation_trigger`, `skill.source`       |
| Agent の利用                     | metric の `agent.name`、event `claude_code.subagent_completed`         | `agent_type`, `model`, `total_tool_uses`, `is_async`     |
| サブエージェントの所要時間       | event `claude_code.subagent_completed` の `duration_ms`（traces 不要） | `agent_type` で p50/p95                                  |
| ツール別の所要時間・失敗率       | event `claude_code.tool_result`                                        | `tool_name`, `duration_ms`, `success`                    |
| API レイテンシ・エラー           | event `claude_code.api_request` / `api_error`                          | `model`, `duration_ms`, `cache_*_tokens`                 |

`skill.name` / `agent.name` / `agent_type` は `OTEL_LOG_TOOL_DETAILS=1` が無いと `"custom"` に丸められる。実名で見るために必須。

### settings.json の `env` に追加する変数

| 変数                                                | 値                      | 理由                                                                                                                      |
| --------------------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `CLAUDE_CODE_ENABLE_TELEMETRY`                      | `1`                     | OTel 全体の有効化                                                                                                         |
| `OTEL_METRICS_EXPORTER` / `OTEL_LOGS_EXPORTER`      | `otlp`                  | metrics と events を送る（traces は未設定＝送らない）                                                                     |
| `OTEL_EXPORTER_OTLP_PROTOCOL`                       | `http/protobuf`         | 4318 で受ける。プロキシ越しでも扱いやすい                                                                                 |
| `OTEL_EXPORTER_OTLP_ENDPOINT`                       | `http://localhost:4318` | ローカルの LGTM                                                                                                           |
| `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` | `cumulative`            | 既定は `delta`。`otel-lgtm` の Collector 設定に deltatocumulative が無く、Prometheus の OTLP 受信は cumulative 前提のため |
| `OTEL_LOG_TOOL_DETAILS`                             | `1`                     | Skill / Agent の実名を出す。Bash コマンドやファイルパスもローカルに残る                                                   |
| `OTEL_METRICS_INCLUDE_REPOSITORY`                   | `true`                  | リポジトリ別に切れるようにする                                                                                            |
| `OTEL_METRICS_INCLUDE_ENTRYPOINT`                   | `true`                  | cli と sdk（`claude -p` 経由のツール）を区別する                                                                          |
| `OTEL_METRICS_INCLUDE_VERSION`                      | `true`                  | バージョン更新前後の比較用                                                                                                |
| `OTEL_METRIC_EXPORT_INTERVAL`                       | `15000`                 | 既定 60s だと確認が遅い。ローカル用に短縮                                                                                 |

`OTEL_LOG_USER_PROMPTS` は未設定（プロンプト本文は取らない）。`OTEL_LOG_TOOL_CONTENT` / `OTEL_LOG_RAW_API_BODIES` も使わない。

### バックエンド

`environment/otel/docker-compose.yml` を新設（既存 compose とは分離）。

- image: `grafana/otel-lgtm` をバージョンタグで固定
- ports: `127.0.0.1:3000:3000`（Grafana）、`127.0.0.1:4317:4317`、`127.0.0.1:4318:4318`。LAN に公開しない（ツール詳細が入るため）
- 永続化: `${HOME}/.local/state/claude-otel-lgtm:/data`（既存 compose の bind mount 流儀、permission-ledger の `~/.local/state/` 規約に合わせる）
- `restart: unless-stopped`

### ダッシュボード

`environment/otel/dashboards/` の JSON を repo 管理する。`otel-lgtm` 同梱の provider は固定の 3 ファイルしか読まないため、`environment/otel/provisioning/claude-code-dashboards.yaml` を追加の provider として `provisioning/dashboards/` にマウントし、`dashboards/` を監視させる。パネルは、名前ごとに 1 行の表（値の列は gauge 表示）を基本にする。Skill・サブエージェント・ツール・hook を名前で見分けるのが目的で、棒グラフ（bar gauge）は値が 1 つだと名前を隠し、Loki の instant 結果も 1 行しか描けないため使わない。

- 上段の stat: コスト合計、セッション数、cache hit 率（cacheRead / (input + cacheRead + cacheCreation)）、API エラー数
- モデル別 コスト、Skill 別 コスト、Agent 別 コスト、モデル別 トークン（`type` 別）: Prometheus
- Skill 別 起動回数（起動契機つき）: Loki の `skill_activated`
- サブエージェント別 実行回数・p50・p95: `subagent_completed`
- ツール別 呼び出し回数・失敗回数・p95: `tool_result`
- hook 別 実行回数・平均・合計: `hook_execution_complete` の `total_duration_ms`。`Stop` hook のように毎ターン走るものが固定費になっていないかを見る
- モデル別 API リクエスト数・p50・p95: `api_request`

Loki の表は、クエリごとの結果を `joinByField` でラベル列にそろえて並べる。回数と ms のように単位が違う列が同じ表にあるため、gauge の最大値は `fieldMinMax` で列ごとに取る。

PromQL / LogQL の実際のメトリクス名・ラベル名（`.` → `_`、単位サフィックスなど）は推測で書かず、Grafana Explore で実データを見て確定させてからパネル化する。

ラベル値の変換・正規化はしない。スモークテストでは `model` がメトリクスで `global.anthropic.claude-sonnet-5-5`（Bedrock ID）、`api_request` イベントで `claude-sonnet-5-5` と信号ごとに異なり、`query_source` もメトリクス `main` に対しイベント `sdk` と異なった。各パネルは出てきた値をそのまま使い、信号をまたぐ突き合わせや同一条件での絞り込みは前提にしない。

### 分析から改善へ

- `ai-agents/skills/` の一覧と `skill_activated` を突き合わせ、起動されない Skill を整理する（`skill-improve` サイクルへ接続）。
- サブエージェント別の p95 と `total_tool_uses` から、モデルを Haiku に下げる／指示を絞る候補を探す。
- Opus のコスト比率、cache hit 率の低いセッションの特徴を見る。
- ツール失敗率の高いものを、permission / hook / sandbox 設定の見直しに繋げる。

## Implementation steps

1. 作業ブランチ `feat/claude-otel-local-observability` を切る。
2. `environment/otel/` に compose とプロビジョニング定義を追加し、`docker compose -f environment/otel/docker-compose.yml config` で構文確認。ユーザーが `up -d` する。
3. settings 変更前に、shell で console exporter を使うスモークテストを行い、`DISABLE_TELEMETRY=1` と共存して OTel が出るかを確認する。
4. `settings.json` の `env` に上表の変数を追記し、`mise run settings-copy` で配布して Claude Code を再起動する。
5. Skill と subagent（Explore 等）を使う短いセッションを回し、Grafana Explore でメトリクス名・LogQL の属性名を確定してからダッシュボード JSON を作る。
6. AGENTS.md、ADR-0008、本 plan を更新する。
7. `commit-and-draft-pr` skill でコミットし、ドラフト PR を作る。

## File changes

- `ai-agents/settings/claude/settings.json` — `env` に OTel 変数を追加
- `environment/otel/docker-compose.yml` — 新規
- `environment/otel/provisioning/claude-code-dashboards.yaml` — 新規
- `environment/otel/dashboards/claude-code.json` — 新規（実データ確認後）
- `AGENTS.md` — Project Structure / Commands に追記
- `docs/adr/0008-use-grafana-lgtm-for-local-claude-code-telemetry.md` — 新規
- `docs/plan/2026-10-08_claude-code-otel-local-observability.md` — 本ファイル
- 触らない: `.claude/settings.json`、`environment/docker/docker-compose.yml`、`scripts/agent-stats`

## Risks and mitigations

| リスク                                                             | 対応                                                                                                                                                                            |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| delta temporality のまま送ると Prometheus に入らない               | `cumulative` を指定。Prometheus で `claude_code_*` が見えるか確認（公式ドキュメントの要約は「delta のままで動く」と読めるが、LGTM の Collector 設定とは食い違うので実機で判断） |
| `DISABLE_TELEMETRY=1` が OTel に効くか未確定                       | ドキュメント上は Anthropic 向け運用テレメトリの設定で別物。Bedrock では既定で無効なので、効いてしまう場合はこの行を外せばよい。手順 3 で確認                                    |
| Bedrock 利用のため `user.email` 等が付かない                       | 単独利用なので問題なし（`user.id` / `session.id` は付く）。`cost.usage` は概算で Bedrock 請求額とは一致しない                                                                   |
| `model` ラベルの値が Bedrock ID / `[1m]` 付きになる                | 確認済み（メトリクスは Bedrock ID、イベントは正規化済み）。実装を増やさないため変換せず、パネルごとに出てきた値をそのまま表示する                                               |
| ツール詳細（Bash コマンド・パス）が平文でローカル保存される        | 127.0.0.1 バインド、プロンプト本文は取らない。将来 SaaS に送る場合は詳細を落とす設計に見直す                                                                                    |
| コンテナ停止中に Claude Code が遅くなる／エラーを吐く              | コンテナを止めて `claude -p "hi"` を実行して確認。問題なら常駐運用に寄せる                                                                                                      |
| metrics の `session.id` ラベルで series が増える                   | ローカルでは許容。重くなったら `OTEL_METRICS_INCLUDE_SESSION_ID=false`                                                                                                          |
| `settings-copy` は上書き配布で、他の未コミット変更も一緒に配られる | 配布前に `git diff` を確認。`~/.claude/settings.json` のローカル専用編集は消える点に注意                                                                                        |
| `otel-lgtm` は開発用途向け（本番非推奨、メモリ 1〜2GB）            | 個人のローカル分析用途なので許容                                                                                                                                                |
| `/data` が永続化先として効かない                                   | 起動後に `docker exec claude-otel-lgtm ls /data` で中身を確認し、再作成してもデータが残ることを見る                                                                             |

## Validation

1. スモークテスト（バックエンド不要）: `CLAUDE_CODE_ENABLE_TELEMETRY=1 OTEL_METRICS_EXPORTER=console OTEL_LOGS_EXPORTER=console OTEL_METRIC_EXPORT_INTERVAL=1000 claude -p "hi"` でメトリクスと events が標準出力に出る。`DISABLE_TELEMETRY=1` 下でも出ることを確認。
2. 受信確認: `docker compose -f environment/otel/docker-compose.yml up -d` 後、`curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:4318/v1/metrics` が応答する。Grafana (`http://localhost:3000`) が開き、追加 provider のダッシュボードフォルダが見える。
3. エンドツーエンド: settings 反映・再起動後、Skill と subagent を使うセッションを実行し、Prometheus に `claude_code_session_count` 系、Loki に `skill_activated` と `subagent_completed` が出る。`claude --debug-file <path>` で `[3P telemetry]` エラーが無いことも見る。
4. 値の突き合わせ: 同じ日のモデル別トークン合計と `scripts/agent-stats` の出力を比較する。サブエージェントの `duration_ms` を transcript のタイムスタンプと 1 件照合する。
5. 障害時挙動: コンテナ停止中に `claude -p "hi"` が詰まらず終了する。
6. リポジトリ検証: `mise run lint` と `mise run verify:changed`（prettier が compose / JSON を、markdownlint が md を検査）。

## Open questions

- Grafana のホスト側ポート 3000 は他の開発サーバと衝突しないか（衝突するなら変更）。
- 1 週間ほど運用した後、traces (beta) を足すか。足す場合は `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` と `OTEL_TRACES_EXPORTER=otlp` を追加するだけ。
- プロンプト本文 (`OTEL_LOG_USER_PROMPTS`) を取るか。プロンプトの書き方の改善には有用だが、業務内容が入るため当面は取らない。
- permission-ledger の承認／拒否集計を `tool_decision` で置き換えられるか（本計画のスコープ外、運用後に評価）。
