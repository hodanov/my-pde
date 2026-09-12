# Plan: 承認した操作を記録し allow 昇格をバッチレビューする仕組み

## Background

勤務先で auto mode が禁止され、今後は `defaultMode: acceptEdits` が主になる。acceptEdits はファイル編集と cwd 内の `mkdir`/`touch`/`mv`/`cp` しか自動承認しないため、未許可の Bash・WebFetch・MCP・作業ディレクトリ外の Read/Edit でプロンプトが頻発する。一度承認した操作を今後は自動許可したいが、`ai-agents/settings/claude/settings.json` の allow を手で足すのは負担が大きい（現に `Bash(gofmt *)` を手で追記した差分があった）。

意図する成果: 「プロンプトが出た操作と、その承認/拒否」を事実として蓄積し、`/permission-review` 1 回で昇格候補を件数付きで一覧し、選んだものだけを allow のソースへ反映・配布できる状態にする。書き込みは必ず人間が選んでから行う（自動昇格は auto mode 相当のリスクを無審査で再現するため採らない）。

## Current structure

調査とスパイクで確定した事実:

- **transcript は拒否しか明示しない。** `~/.claude/projects/**/*.jsonl` の tool_result 行に `toolDenialKind: "user-rejected"`（ダイアログで拒否）/ `"permission-rule"`（deny ルールや hook の遮断）と `userFeedback` が付く。承認は通常の tool_result と区別できず、ルールによる自動許可・auto mode・sandbox 自動許可と見分けがつかない。過去の permissionMode は auto 731 / plan 315 / acceptEdits 268 で、過去データからは「プロンプトが出て承認した」事実を復元できない。
- **`PermissionRequest` hook** はダイアログ表示直前に発火し、`permission_suggestions`（`{type:"addRules", rules:[{toolName, ruleContent}], behavior, destination}` = 「don't ask again」で保存されるはずの正規ルール）を受け取る。ユーザーの選択結果は受け取らない。
- **スパイク結果（Claude Code 2.1.269、acceptEdits）**: Bash / WebFetch / 作業ディレクトリ外の Read / subagent 内の Bash の 5 件すべてで発火し、`async: true` も有効だった。複合コマンドはサブコマンドごとに提案が出る。subagent の呼び出しには `agent_id` / `agent_type` が付き、`transcript_path` は親セッションのまま（呼び出し本体は `<session>/subagents/agent-<agent_id>.jsonl` にある）。**ドキュメントの例と異なり入力に `tool_use_id` は無い。** `prompt_id` も transcript の `promptId` とは一致しない。ツール名と `tool_input` の一致なら、5 件とも transcript の tool_use に一意に対応した。
- **「Yes, and don't ask again」は各リポの `.claude/settings.local.json`（worktree は main checkout 側）に保存される。** 既に hodalog-hugo / my-pde / new-project / stable_diffusion_modal に計 90 件近く溜まっており、グローバル昇格の候補源になる。
- **休眠中の旧仕組み**: `ai-agents/hooks/permission-prompt-detect.sh`（PreToolUse でプロンプト発生を Python 近似判定）+ `permission-prompt-nudge.sh`（Stop でスキル起動）+ `ai-agents/skills/permission-prompt-tuner/`。近似判定ではなく PermissionRequest で実際のプロンプトを取れるため置き換える。
- **`scripts/agent-stats/`**（Go）は transcript スキーマ知識を `internal/parser` に閉じる方針。`rawContent` は tool_use の `id` と tool_result の `tool_use_id` を、`rawLine` は `toolDenialKind` をまだ読んでいない。
- **配布**: `mise run claude-settings-copy` が `ai-agents/settings/claude/` と `ai-agents/hooks/` を `~/.claude` へ `--force` コピーする。`~/.claude/settings.json` はコピーなので、編集対象は常にソース側。ai-agents プラグインは未インストールで、hook は settings.json の配線でのみ発火する。

## Design policy

- **記録は事実のみ、判断はレビュー時。** hook は観測専用（stdout なし、決定を返さない）。昇格の可否はスキル + 人間が決める。
- **ルール文字列は Claude Code に作らせる。** 候補キーは `permission_suggestions` の `toolName` + `ruleContent` をそのまま使い、Bash の正規化や許可判定マッチャーを自前で再実装しない。既存 allow との重複判定は `:*` → `*` を揃えた完全一致だけで足りる（提案はそもそも未許可の操作にしか出ない）。
- **照合キーは tool_input。** hook の入力に `tool_use_id` が無いため、ledger に `tool_input` を保存し、transcript の呼び出しと「ledger に残ったキーがすべて一致」で照合する。hook はファイル本文系のフィールド（`content` / `old_string` / `new_string` / `new_source` / `edits`）を落として保存するので、落とすキーの一覧は hook の 1 か所にだけある。コマンド文字列は ledger にも残るが、権限 600・リポジトリ外で、transcript と同程度の機微度に留まる（ユーザー確認済み）。
- **transcript スキーマ知識は `internal/parser` に置く。** 新しい読み取りもここに足し、分析側はスキーマを知らない。
- **スコープ判断は既存方針を踏襲。** 汎用・無害 → グローバル、リポ/パス依存 → local のまま。途中 `*`（`Bash(terraform -chdir=* show *)` 型）は採らない。

## Implementation steps

`main` からブランチを切って作業する。未コミットだった `README.md` の変更はユーザー自身のものなので含めない。`settings.json` の `Bash(gofmt *)` 追記は hook 配線のコミットに同梱する（ユーザー確認済み）。

### 0. スパイク（実機で確認済み）

使い捨ての `--settings` に PermissionRequest hook（stdin を scratch に追記するだけ、async と同期の 2 本）を配線し、`claude --permission-mode acceptEdits --settings <spike.json>` で対話実行した。結果は「Current structure」のとおりで、照合方式を `tool_use_id` から tool_input に切り替えた。

### 1. 旧 tuner の撤去

- 削除: `ai-agents/hooks/permission-prompt-detect.sh`, `ai-agents/hooks/permission-prompt-nudge.sh`, `ai-agents/skills/permission-prompt-tuner/`（`references/prompt-causes.md` の知見は手順 4 の審査基準へ移す）
- `.claude/rules/skill-authoring.md` の `permission-prompt-tuner` 例示、`.claude/rules/hook-authoring.md` の休眠 hook 例示を修正
- 配布済みの `~/.claude/hooks/permission-prompt-*.sh` と `~/.claude/skills/permission-prompt-tuner/` は `copy-entries.sh --force` では消えないため、ユーザー確認のうえ手で削除

### 2. 記録 hook `ai-agents/hooks/permission-ledger.sh`

- 入力から `{ts, session_id, prompt_id, agent_id, tool_name, cwd, transcript_path, permission_mode, tool_input（本文系フィールドを除去）, suggestions}` を 1 行 JSON で追記する。`jq` が無ければ何もしない
- 追記先: `${XDG_STATE_HOME:-$HOME/.local/state}/claude-permission-ledger/ledger.jsonl`（`umask 077` でディレクトリ 700 / ファイル 600）。リポジトリ外・セッション跨ぎで残る場所
- 配線: `ai-agents/settings/claude/settings.json` と `ai-agents/hooks/hooks.json` の両方に `PermissionRequest`（matcher 省略 = 全ツール、`async: true`）。既存 hook と同じ二重配線パターン

### 3. 分析 CLI `scripts/agent-stats/cmd/permission-audit/`

agent-stats モジュールに 2 本目のバイナリとして追加し、`internal/parser` を共有する。

- **`internal/parser`**: `rawContent` に `ID` と `ToolUseID`、`rawLine` に `ToolDenialKind` を追加。`ToolCallIndex`（tool_use id → `ToolCall{ID, Name, Input, Cwd, Timestamp, Outcome}`）を `toolcall.go` に追加する。`Outcome` は `executed` / `user-rejected` / `denied` / `no-result`。`toolDenialKind` が無い古い transcript は既存の `classifyToolError` の文言分類にフォールバックする。行の走査は既存の `appendLines` を `scanLines`（コールバック版）に切り出して共有する。tool_use と tool_result は同じファイルに書かれるため、索引はファイル単位で作ればよい
- **`internal/permission`**（新規）:
  - `ReadLedger`: 読み込み。二重配線による重複は「同じセッション・agent・prompt・ツール・入力で 10 秒以内」をまとめて落とす
  - `Audit`: 各エントリを、その transcript ファイル（subagent なら agent transcript）の中で「同じツール名・ledger 側のキーがすべて一致・記録時刻（秒精度なので 5 秒の猶予）以前で最も新しい未照合」の呼び出しに対応づけ、outcome を確定する（見つからなければ `pending`）。ルール単位に approved / rejected / pending 件数、セッション数、プロンプトが出たリポ（`repos`）、そのルールを既に local で許可しているリポ（`local_repos`）、最終日時、コマンド例 ≤ 3 を集計する
  - 提案なしの呼び出しは先頭コマンド・ドメイン・パスから下書きし `unsuggested` として別枠にする
  - 拒否履歴: 走査した transcript 全体の `user-rejected` を系統（Bash 先頭コマンド / WebFetch ドメイン / MCP ツール）別に数え、`family_rejections` として候補に付ける
  - 既存 allow / ask / deny と完全一致（`:*` 正規化後）する候補、および declined リストにある候補を除外
- **CLI**: `--settings <path>`（必須）、`--ledger`、`--declined`、`--dir`（既定 `~/.claude/projects`）、`--since`（既定 30 日）、`--json`、既定は table。local 取り込みは ledger と transcript の cwd から `git rev-parse --git-common-dir` で main checkout を解決し、各 `.claude/settings.local.json` の allow を読む
- **mise task** `permission-audit`（`--settings {{config_root}}/ai-agents/settings/claude/settings.json` を渡す）。`agent-stats:test` / `:lint` と CI は `./...` なので追加設定不要
- `scripts/agent-stats/README.md` に permission-audit の節を追加

### 4. レビュースキル `.claude/skills/permission-review/`

`ai-agents/**` を書き換えるため my-pde 専用ルートに置く（skill-authoring ルール）。審査基準は `references/review-criteria.md`（集計値の読み方、許可判定の前提、見送り・一般化・local 維持・昇格の判断基準）。

Workflow:

1. `mise run permission-audit -- --json` を実行
2. 各候補を references に照らして分類: 昇格 / 一般化して昇格 / local のまま / 見送り
3. 表で提示し、AskUserQuestion（multiSelect）で選んでもらう
4. 選ばれたルールを `ai-agents/settings/claude/settings.json` の allow に追記し `prettier --check`。昇格しなかったルールを declined リスト（ledger と同じ state ディレクトリの `declined.txt`）に追記し、次回以降は出さない。昇格済みで各リポの `settings.local.json` に残る重複は、ファイルごとに確認して削除
5. `deploy-ai-config` で配布

### 5. 初回ブートストラップ（手動運用、コード変更なし）

ledger は導入時点で空なので、初回は local 取り込みだけで `/permission-review` を回す。加えて組み込みの `/fewer-permission-prompts` を 1 回実行し、読み取り系コマンドの提案はプロジェクト設定ではなく `ai-agents/settings/claude/settings.json` へ反映させる。

## File changes

| パス                                                                               | 変更                                                           |
| ---------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| `ai-agents/hooks/permission-ledger.sh`                                             | 新規: PermissionRequest 記録 hook                              |
| `ai-agents/hooks/hooks.json`, `ai-agents/settings/claude/settings.json`            | PermissionRequest の配線                                       |
| `ai-agents/hooks/permission-prompt-{detect,nudge}.sh`                              | 削除                                                           |
| `ai-agents/skills/permission-prompt-tuner/`                                        | 削除                                                           |
| `scripts/agent-stats/internal/parser/parser.go`, `toolcall.go`, `toolcall_test.go` | transcript の ID / `toolDenialKind` 読み取りと `ToolCallIndex` |
| `scripts/agent-stats/internal/permission/`                                         | 新規: ledger 読み取り・入力照合・outcome 確定・候補集計        |
| `scripts/agent-stats/cmd/permission-audit/`                                        | 新規 CLI                                                       |
| `scripts/agent-stats/README.md`, `mise.toml`                                       | 説明と `permission-audit` task                                 |
| `.claude/skills/permission-review/SKILL.md`, `references/review-criteria.md`       | 新規スキル                                                     |
| `.claude/rules/skill-authoring.md`, `.claude/rules/hook-authoring.md`              | 旧 tuner への言及を更新                                        |

コミット単位: (1) 旧 tuner 撤去 + rules 更新 → (2) hook + 配線 → (3) parser + permission-audit + mise + README → (4) スキル + この plan。

## Risks and mitigations

- **transcript の保持期限（`cleanupPeriodDays`、既定 30 日）を過ぎると outcome が引けない。** `pending` として件数は残し、スキルは月 1 回以上の実行を前提にする。
- **同じ入力の呼び出しが 1 セッションに複数ある。** 記録時刻以前で最も新しい未照合の呼び出しに対応づける。プロンプト後にセッション許可で通った同一入力は、時刻が後なので取り違えない。
- **hook 入力のスキーマが CLI 更新で変わる**（今回 `tool_use_id` がドキュメントと食い違っていた）。照合できない行は `pending` に落ちて件数は残るので、`pending` の急増を異常のサインとして扱う。
- **提案文字列が広すぎる**（例: `Bash(curl *)`）。hook は記録だけで、昇格は必ずレビューを通す。審査基準に「広すぎる prefix」の具体例を持たせる。
- **一回限りの完全一致ルールが local から大量に候補に出る**（実データで 81 件中の多数）。初回レビューで declined に入れれば以後は出ない。
- **ledger にコマンド文字列が残る。** ファイル本文は落とし、権限 600・リポジトリ外に置く。transcript と同程度の機微度。

## Validation

- `mise run agent-stats:test` / `mise run agent-stats:lint`（transcript の outcome 判別〈`user-rejected` / `permission-rule` / 文言フォールバック〉、ledger の重複排除の時間窓、subagent transcript での照合、同一入力の直前選択、秒精度の猶予、本文を落とした入力の部分一致、`:*` 正規化込みの既存ルール除外、worktree サブディレクトリからの main checkout 解決、declined 除外。いずれもテーブル駆動・インライン JSONL）
- hook: `shellcheck` / `shfmt`、scratch の `XDG_STATE_HOME` でサンプル入力を流し 1 行追記・stdout 空・壊れた入力で exit 0・本文フィールド非保存・権限 700/600 を確認
- スパイクの実ペイロードを本番 hook に通して ledger を作り、`permission-audit` がスパイク通りの承認/拒否を集計すること
- `prettier --check` で settings.json / hooks.json、`markdownlint-cli2` で SKILL.md / references / plan / README
- E2E: `claude-settings-copy` 後、acceptEdits の新セッションで未許可コマンドを 1 回承認・1 回拒否 → `mise run permission-audit` に approved 1 / rejected 1 で出る → `/permission-review` で承認側を選ぶ → ソース settings.json に追記され、配布後の新セッションでそのコマンドがプロンプトなしで通る

## Open questions

- なし（スパイクで acceptEdits での発火を確認済み）。
