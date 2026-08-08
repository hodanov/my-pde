---
name: hook-scaffold
description: >-
  新しいフックの雛形（スクリプト + 各 CLI の配線）を本リポジトリの規約どおりに生成する。
  イベント選定（PreToolUse / PostToolUse / Stop / SessionStart 等）とブロッキング可否、
  claude / cursor / copilot のどこまで配線するかを対話的に決め、
  ai-agents/settings/<cli>/hooks/<name>.sh と settings.json / hooks.json への配線まで行う。
  新しいフックを追加したいときに `/hook-scaffold <フック名> [用途の一言]` で呼び出す。
disable-model-invocation: true
argument-hint: "<フック名> [用途の一言]"
metadata:
  version: 1
---

# /hook-scaffold スキル

## Goal

新規フックの追加を、スクリプト雛形の生成から 3 CLI（claude / cursor / copilot）分の配線・検証まで、
本リポジトリの規約どおりに一貫した手順で完了させる。
`skill-scaffold` が担う「スキルの新規作成」に対する、hook 側の対（つい）を埋める。

## Workflow

### Step 1: 引数パース

`$ARGUMENTS` を以下のように解釈する:

- **第1引数**: 生成するフック名（以降 `HOOK_NAME`。ファイル名は `HOOK_NAME.sh`）
- **第2引数以降**: そのフックの用途の一言（自由記述、省略可）

引数なしの場合は対話でフック名と用途を尋ねる。`HOOK_NAME` は既存に倣い lowercase + ハイフンにする。

### Step 2: 衝突チェックと既存重複の確認

`ai-agents/settings/*/hooks/HOOK_NAME.sh` が **存在しないこと** を確認する。存在する場合は作成せず中断し、既存スクリプトの修正を案内する。

あわせて既存フックを読み、同種の処理（formatter / lint / guard / notify）が既にないか確認する。
formatter 系は 1 ファイル種別 1 スクリプトで揃っているので、対象拡張子が既存とかぶる場合は新規追加ではなく既存への追記を提案する。

### Step 3: イベント選定とブロッキング可否の確認

「何をきっかけに走らせたいか」からイベントを選ぶ。現在このリポジトリが配線しているのは以下だけで、他イベントを使う場合は公式ドキュメント（<https://code.claude.com/docs/en/hooks>）で最新のスキーマを確認する。

| 用途                   | claude のイベント                                 | 現行の配線                |
| ---------------------- | ------------------------------------------------- | ------------------------- |
| 起動時の環境チェック   | `SessionStart`                                    | `toolchain-doctor.sh`     |
| Bash 実行前の遮断      | `PreToolUse`（matcher `Bash`）                    | `guard-dangerous-bash.sh` |
| ファイル編集後の整形   | `PostToolUse`（matcher `Write\|Edit\|MultiEdit`） | formatter 6 本            |
| 応答終了時のまとめ処理 | `Stop`                                            | `lint-changed.sh` ほか    |
| 通知                   | `Notification`                                    | `notify-macos.sh`         |
| worktree 作成時        | `WorktreeCreate`                                  | `worktree-create.sh`      |

選定時に必ず確認すること:

- **そのイベントは exit 2 でブロックできるか**。`PreToolUse` はブロックできるが `PostToolUse` はできない。ブロック不可のイベントに強制ロジックを載せると「黙って効かないフック」になる。
- **終了コードの意味**: `0` = 正常（stdout が JSON なら構造化出力として解釈される）、`2` = ブロッキング（stderr がメッセージ）、それ以外 = 非ブロッキングエラー。
- **`hookSpecificOutput` の形はイベントごとに違う**。使う場合は転記済みの記憶に頼らず、必ず上記ドキュメントで確認する。

report-only（報告のみで止めない）で足りるなら、`exit 0` + stderr への出力に留めるのが既定。

### Step 4: 3 CLI 展開の要否判断

**既定は claude のみ**。以下の基準で横展開を判断する。

- **展開する**: `PostToolUse`（Write/Edit）相当のファイル編集フック。cursor の `afterFileEdit`、copilot の `postToolUse` に同じ意味で載る（既存 formatter 6 本がこの形）。
- **展開しない**: `Stop` / `SessionStart` / `WorktreeCreate` / macOS 通知など Claude 固有機構に依存するもの。既存の `lint-changed` / `guard-dangerous-bash` / `toolchain-doctor` / `notify-macos` は claude 専用。
- cursor CLI（cursor-agent）は定義しても一部イベントしか実際に飛ばさないという報告がある。迷ったら claude だけに配線し、必要になってから広げる。

### Step 5: スクリプト生成

`ai-agents/settings/claude/hooks/HOOK_NAME.sh` を既存フック準拠で作る。定型は次のとおり
（インデントは既存フックに合わせてタブにする。以下は markdownlint の hard-tab 検査を避けるためスペース表記にしてあるので、書き出し後に `shfmt -w` を通して揃える）:

```bash
#!/bin/bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# stdin から JSON を読み込む
INPUT=$(cat)

FILE_PATH=$(echo "$INPUT" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(data.get('tool_input', {}).get('file_path', ''))
")

# 対象外なら何もせず抜ける
if [[ "$FILE_PATH" != *.EXT ]]; then
  exit 0
fi

if ! COMMAND "$FILE_PATH" 2>&1; then
  echo "[HOOK_NAME] fail: $FILE_PATH" >&2
  exit 1
fi
```

Step 4 で cursor / copilot にも展開する場合は、同じスクリプトを両ディレクトリにも置き、
**ファイルパス抽出の 1 行だけ** `get_file_path.py` 経由に差し替える（これが CLI 間の唯一の差分）:

```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT=$(cat)

FILE_PATH=$(printf '%s' "$INPUT" | python3 "$SCRIPT_DIR/get_file_path.py")
```

### Step 6: 配線

選んだ CLI ごとに配線する。JSON は既存エントリの末尾に追記する形にし、並び順の慣習（formatter は既存の並びに続ける）に従う。

- **claude**: `ai-agents/settings/claude/settings.json` の `hooks.<Event>` に
  `{"type": "command", "command": "~/.claude/hooks/HOOK_NAME.sh"}` を追記する。
  `PostToolUse` なら既存の `matcher: "Write|Edit|MultiEdit"` グループの `hooks` 配列に足す（新しいグループを作らない）。
- **cursor**: `ai-agents/settings/cursor/hooks.json` の `hooks.afterFileEdit` に
  `{"command": "~/.cursor/hooks/HOOK_NAME.sh"}` を追記する（`type` は不要）。
- **copilot**: `ai-agents/settings/copilot/hooks/hooks.json` の `hooks.postToolUse` に
  `{"type": "command", "bash": "~/.copilot/hooks/HOOK_NAME.sh", "timeoutSec": 10}` を追記する
  （キーは `command` ではなく **`bash`**、`timeoutSec` は必須）。

編集後、触った JSON すべてを `jq . <file>` にかけてパースできることを必ず確認する。

### Step 7: 配布経路の確認

`mise.toml` の `claude-settings-copy` / `cursor-settings-copy` / `copilot-hooks-copy` は
`ai-agents/scripts/copy-entries.sh` でディレクトリ単位に配るため、
**既存ディレクトリにファイルを 1 本足すだけなら mise / `copy-entries.sh` の変更は不要**。
新しいディレクトリ階層を導入する場合のみタスク追加を検討する。この前提自体を毎回確認する。

### Step 8: 検証

以下を実行し、エラーを潰してから完了とする。

1. `chmod +x` されていること（既存フックに合わせる）
2. `shfmt -d <script>` と `shellcheck <script>`（`mise run lint:shell` で一括でも可）
3. `jq .` で配線先 JSON がパースできること
4. サンプル入力での単体実行: `echo '{"tool_input":{"file_path":"/tmp/x.sh"}}' | ./HOOK_NAME.sh`
5. デプロイは自動実行せず、`mise run settings-copy` で 3 CLI へ配布される旨を案内する
6. 発火確認は `claude --debug` 等で行う旨を案内する

## Notes

- **既存スクリプトの上書き防止が最重要**。Step 2 の非存在確認を必ず先に行い、衝突時は作成せず中断する。
- **イベント一覧や JSON スキーマを SKILL.md に転記しない**。Claude Code のフック仕様は変化が速いため、
  本文には「このリポジトリ固有の配線規約」だけを書き、詳細は <https://code.claude.com/docs/en/hooks> を参照させる。
- **既定は claude 専用**。3 CLI への横展開はファイル編集系フックに限る保守的な運用にする（Step 4）。
- Step 6 の JSON 編集を誤ると設定全体が読めなくなるため、`jq` での妥当性確認を飛ばさない。
- 誤爆でフックが生成されるのを防ぐため `disable-model-invocation: true` を付けている。明示呼び出し専用。
- 本スキルは「新規フックの立ち上げ」専任。既存フック定義の静的検査は `agents-lint` 系の役割で、そちらには踏み込まない。
