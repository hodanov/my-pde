# Plan: ai-agents 向け 汎用 hooks/skills 提案ルーチンの追加

## Context（なぜやるか）

`routines/daily-neovim-trend-scan.json` を実運用したところ、「毎回1件の改善 Issue を起票 →
採用/不採用をラベルで管理 → 採用分は weekly PR bot が実装」という自律改善ループがうまく回っている。
この成功パターンを **開発効率（DevX）の向上** という別領域に横展開したい。

具体的には、参照ブログ
[Steering Claude Code: skills, hooks, rules, subagents, and more](https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more)
の考え方を下敷きに、`ai-agents/` に **新規追加すると良さそうな汎用的な hooks / skills** を
毎週1件ずつ提案する Routine を新設する。出力は Issue 起票のみ（コード変更や PR はしない）。

ユーザー決定事項:

- 実行頻度: **毎週**（hooks/skills のネタは nvim トレンドより変化が遅く母数も小さいため、週次で質を担保）
- 提案スコープ: **hooks と skills のみ**（subagents/rules/CLAUDE.md は対象外）

## 設計方針

既存 `daily-neovim-trend-scan.json` と同じ「冪等な単発提案」パターンを踏襲する:

- 提案済み Open Issue と 不採用 Close 済み Issue を両方取得し、どちらとも重複しない提案を毎回1件だけ起票。
- ラベルで管理。採用された Issue に `adopted` を付ければ、既存の
  `weekly-adopted-issue-pr-bot.json` がそのまま実装→ドラフト PR まで回す（パイプライン再利用）。
- read-only ツール + `gh`(Bash) のみ。コード変更・PR はしない。

### スコープの土台（agent が参照すべき既存構造）

- skills: `ai-agents/skills/<name>/SKILL.md`（frontmatter: `name` / `description` / 任意で
  `argument-hint` `disable-model-invocation` `metadata.version`、本文は `# /<name> スキル` →
  `## Goal` / `## Workflow` / `## Notes`）。既存18スキルと重複させない。
- hooks: `ai-agents/settings/{claude,cursor,copilot}/hooks/*.sh` ＋
  `settings/claude/settings.json`（`PostToolUse` / `Stop` / `Notification` などのイベント・matcher 配線）。
  既存 hook（goimports / markdown-format / shfmt / prettier / stylua / tombi / notify-macos）と重複させない。
- デプロイ経路: ルート `ai-agents/Makefile` と `ai-agents/scripts/copy-entries.sh`
  （skills/agents/settings を `~/.{codex,claude,cursor,copilot}` へコピー）。新規 hook は
  3エディタ分の配線と Makefile/copy 経路への影響まで Issue 本文で触れさせる。

## 追加するファイル

### 1. `routines/weekly-devx-skills-hooks-scan.json`（新規）

既存ルーチンと同一スキーマ。確定値:

- `name`: `"Weekly DevX Skills/Hooks Scan"`
- `trigger_id`: 起票時は未確定。**`/schedule`(Create) で払い出された ID を後から記録**する
  （README の運用どおり。PR にはプレースホルダで出し、apply 後に実 ID で更新）。
- `enabled`: `true`
- `cron_expression`: `"0 22 * * 3"`（水 22:00 UTC = 木 07:00 JST）。
  既存の daily nvim(`0 23 * * *`) と weekly PR bot(`0 23 * * 6`) を避け、時刻も 22:00 にずらして衝突回避。
- `schedule_note`: `"水 22:00 UTC = 毎週木曜 7:00 JST"`
- `job_config`: 既存と同じ `model: claude-opus-4-8` / `repository` / `environment_id: env_01H6ttcff6EJXggu8Xqmx5gN`
- `allowed_tools`: `["Bash", "Read", "Grep", "Glob", "WebSearch", "WebFetch"]`（nvim scan と同じ read-only 構成）
- `prompt`（1行=配列1要素）に以下の主旨を記述:
  1. 役割: my-pde の `ai-agents/` に追加する汎用 hooks/skills を自律提案するエージェント。
     判断軸は参照ブログ（hook=決定論的/自動強制、skill=会話で見える手続き）。
  2. リポジトリ構成: 上記「スコープの土台」の3点（skills 配置・hooks 配置と settings.json 配線・
     Makefile/copy-entries デプロイ経路）を明記。
  3. 重複回避: `gh issue list --state open --label "scan:ai-agents"` と
     `gh issue list --state closed --label "scan:ai-agents" --label "rejected"` を両方取得。
     さらに `ai-agents/skills/` と `settings/*/hooks/` を読み、既存実物とも重複させない。
  4. 調査: WebSearch/WebFetch で Claude Code の skills/hooks ベストプラクティス・有用例を調べつつ、
     ブログのフレームワークで「hook にすべきか skill にすべきか」を判断。
  5. 提案は**汎用的・再利用可能な hook または skill を1つだけ**選ぶ（このリポジトリ固有の一発ネタは避ける）。
  6. ラベル `scan:ai-agents` が無ければ作成し、Issue を1件だけ起票。本文に:
     (a) 何を・なぜ / (b) hook か skill か＋その機構を選ぶ理由（ブログ準拠） /
     (c) 出典URL / (d) **どこにどう置くか**（skill なら `ai-agents/skills/<name>/SKILL.md` の
     frontmatter 雛形、hook なら `settings/{claude,cursor,copilot}/hooks/<name>.sh` ＋
     settings.json の配線 ＋ Makefile/copy-entries への影響）/ (e) リスク・留意点。
  7. コード変更・PR はしない。Issue 起票のみ。重複しない新提案が見つからなければ起票せず終了可。
  - 制約: 探索を広げすぎない。Issue 最大1件。Open 提案済み・Close 済み rejected と重複しない。
    機構は hooks と skills のみ（subagents/rules は提案しない）。

### 2. `routines/README.md`（更新）

- 「現在の定義」表に新ルーチン行を追加。
- **既存の表のズレを修正**: 1行目が `weekly-neovim-trend-scan.json` /「毎週土曜」のままだが、
  実体は `daily-neovim-trend-scan.json` / `0 23 * * *`（毎日）。実ファイルに合わせて修正する。
- 修正後の表（3行）:
  | ファイル | 名前 | スケジュール | 役割 |
  | `daily-neovim-trend-scan.json` | Daily Neovim Trend Scan | 毎日 8:00 JST (`0 23 * * *`) | Neovim 動向を調べ改善を Issue 起票（最大1件） |
  | `weekly-devx-skills-hooks-scan.json` | Weekly DevX Skills/Hooks Scan | 毎週木 7:00 JST (`0 22 * * 3`) | ai-agents 向け汎用 hooks/skills を Issue 起票（最大1件） |
  | `weekly-adopted-issue-pr-bot.json` | Weekly Adopted-Issue PR Bot | 毎週日 8:00 JST (`0 23 * * 6`) | `adopted` Issue を実装しドラフト PR を作成 |

## 採用フローとの接続（再利用）

既存 `weekly-adopted-issue-pr-bot.json` は `adopted` ラベルの Issue を汎用的に実装する。
この新ルーチンが起票した Issue に手動で `adopted` を付ければ、そのまま実装→ドラフト PR まで自動で回る。
新ルーチン側の追加配線は不要（ラベル運用だけで連結）。

## 検証方法

クラウド Routine 自体はローカル実行できないため、リポジトリ側で以下を確認する:

1. **JSON 妥当性**: `cat routines/weekly-devx-skills-hooks-scan.json | jq .` でパース成功を確認。
   既存2ファイルとキー構成（`name`/`trigger_id`/`enabled`/`cron_expression`/`schedule_note`/
   `job_config`/`prompt`）が一致していること。
2. **cron 衝突確認**: 既存の `0 23 * * *`・`0 23 * * 6` と時刻/曜日が重複しないこと（`0 22 * * 3`）。
3. **Markdown**: README 更新後、`markdownlint-cli2` が通ること（`ai-agents/.markdownlint-cli2.yaml` 準拠）。
4. **apply（手動・ユーザー操作）**: PR マージ後に `/schedule`(Create) で Routine を作成し、
   払い出された `trigger_id` を JSON に記録して追従コミット。初回は claude.ai 上で手動 Run して
   `scan:ai-agents` ラベル付き Issue が1件・重複なしで起票されることを確認する。

## スコープ外

- subagents / rules / CLAUDE.md(agents.xml) の提案（今回は hooks/skills のみ）。
- routines を CI から自動 apply する仕組み（README の既知 TODO のまま）。
- 実際の hook/skill の実装（採用後に PR bot or 手動で対応）。
