# ADR-0008: Use Grafana LGTM as the local backend for Claude Code telemetry

- Status: Accepted (2026-10-08)

## Context

Claude Code の使い方（どのモデル・Skill・Agent を使ったか、サブエージェントの処理にどれだけ時間がかかったか）を計測し、改善に繋げたい。Claude Code 本体は OpenTelemetry (OTel) で metrics と events を OTLP 出力できる。出力先の受け口が要る。

既存の分析手段は `scripts/agent-stats`（`~/.claude/projects/**/*.jsonl` の事後バッチ集計）と permission-ledger hook で、transcript に残るものしか見えない。API レイテンシ、サブエージェントの `duration_ms`（`claude_code.subagent_completed`）、ツール単位の所要時間と失敗率（`claude_code.tool_result`）、継続的なコスト／トークンの時系列は OTel で初めて取れる。両者は補完関係で、OTel は agent-stats を置き換えない。

条件は次のとおり。

- まずは手元の macOS (arm64) + Docker で、すぐ分析できること。
- OTLP で受けるので、後から Datadog など別のバックエンドへ送り先だけ差し替えられること。
- Claude Code は Bedrock 経由で使っており、`DISABLE_TELEMETRY=1` を設定済み。OTel の出力は `CLAUDE_CODE_ENABLE_TELEMETRY=1` と `OTEL_*` で別に有効化する。
- プロジェクトの `.claude/settings.json` の `env` では OTel のエクスポータ変数が無視される。恒久設定は user settings（`ai-agents/settings/claude/settings.json` の配布先）に置くしかない。

## Options considered

| 案                                                 | 退けた理由                                                                                                                                                                                      |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OTel Collector で JSONL に書き、DuckDB で集計      | 最も軽量で SQL を書きやすいが、GUI のダッシュボードが無く、DuckDB のホスト側導入も要る。時系列を眺めて気付きを得る用途には LGTM の方が合う。events の集計だけが欲しくなったら後から追加できる。 |
| Aspire Dashboard                                   | 起動は最速で 3 シグナルとも見られるが、保持はメモリのみで、集計・長期比較が弱い。                                                                                                               |
| Datadog への直接送信                               | 検証の前に SaaS 契約・API キー・送信範囲の判断が要る。`OTEL_LOG_TOOL_DETAILS=1` で Bash コマンドやファイルパスがローカル外に出るため、まずローカルで中身を把握してからにする。                  |
| 独自に Prometheus・Loki・Tempo・Grafana を個別構築 | 構成要素が多く、「ささっと」の条件に合わない。`grafana/otel-lgtm` は同じ構成を 1 コンテナで提供する。                                                                                           |

## Decision

`grafana/otel-lgtm`（Grafana + Prometheus + Loki + Tempo + OTel Collector）を、`environment/otel/docker-compose.yml` でローカルに常駐させ、Claude Code から OTLP/HTTP (`localhost:4318`) へ metrics と events を送る。

- ポートは `127.0.0.1` にだけ公開する。ツール詳細を含むため LAN に出さない。
- 送信する変数は `ai-agents/settings/claude/settings.json` の `env` に常時 ON で置く。traces (beta) は有効化しない。
- `OTEL_LOG_TOOL_DETAILS=1` を有効にする。無効だと `skill.name`、`agent.name`、`agent_type` が `"custom"` に丸められ、Skill と Agent の実名で分析できないため。プロンプト本文 (`OTEL_LOG_USER_PROMPTS`) は取らない。
- metrics の temporality は `cumulative` に固定する。Claude Code の既定は `delta` だが、`otel-lgtm` の Collector 設定には deltatocumulative が無く、Prometheus の OTLP 受信は cumulative を前提にする。

ダッシュボードは `environment/otel/dashboards/` の JSON で管理する。`otel-lgtm` 同梱の provider は固定の 3 ファイルしか読まないため、`environment/otel/provisioning/claude-code-dashboards.yaml` を追加の provider として置く。

## Consequences

Skill・Agent・モデル・ツール別の利用状況とサブエージェントの所要時間を、手元の Grafana で見られる。

- `cost.usage` は Claude Code の概算で、Bedrock の請求額とは一致しない。Bedrock では `user.email` などのアカウント属性も付かない。
- ラベル値は変換しない。`model` はメトリクスでは Bedrock の ID（`global.anthropic.claude-sonnet-5-5`）、`api_request` イベントでは正規化済み（`claude-sonnet-5-5`）と、信号ごとに値が異なる。実装を増やさないため、パネルごとに出てきた値をそのまま使う。
- Bash コマンドとファイルパスが、`~/.local/state/claude-otel-lgtm` に平文で残る。送り先を SaaS に変える場合は、`OTEL_LOG_TOOL_DETAILS` を落とすかフィルタする設計に見直す。
- `settings-copy` は `~/.claude/settings.json` を上書き配布するので、OTel の変数もそこで配られる。環境変数は起動時にしか読まれず、反映には Claude Code の再起動が要る。
- コンテナが止まっていると、エクスポートは失敗するだけでデータは欠ける。欠損を許容する。
- Loki 3.7 は WAL のあるディスクの使用率が `-ingester.wal-disk-full-threshold`（既定 0.90）を超えると、すべての push を `Ingester is shutting down` で拒否する。`/data` は bind mount なので、判定に使われるのは Mac のホストボリュームの使用率になる。ホストは 90% 前後で推移するため、compose の `LOKI_EXTRA_ARGS` で閾値を 0.98 に上げる。0 にして無効化はせず、本当に満杯になる手前の保護は残す。`otel-lgtm` は Loki のログを捨てるので、拒否されていても `docker logs` には何も出ない。events だけが欠けるときは、Loki の `loki_ingester_wal_disk_usage_percent` を見る。
- `otel-lgtm` は開発・検証用途向けで、本番運用には向かない。個人のローカル分析に限って採用する。

見直す条件は次の 2 つ。

- 複数マシンやチームで集計したくなった場合。送り先を Collector 経由の共有バックエンドへ変える。
- サブエージェントの所要時間をウォーターフォールで見たくなった場合。`CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` と `OTEL_TRACES_EXPORTER=otlp` を足す。LGTM は Tempo を含むので、バックエンドの変更は要らない。

関連: `docs/plan/2026-10-08_claude-code-otel-local-observability.md`、`docs/plan/2026-08-22_agent-stats-observability-expansion.md`
