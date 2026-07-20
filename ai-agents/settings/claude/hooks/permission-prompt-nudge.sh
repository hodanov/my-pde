#!/usr/bin/env bash
# Stop hook: when this session hit commands that triggered a permission prompt
# (recorded by permission-prompt-detect.sh), block once so Claude runs the
# permission-prompt-tuner skill to record them and propose settings.json fixes.
# Non-destructive — it only nudges; the analysis/records are written by Claude.
set -u

command -v jq >/dev/null 2>&1 || exit 0

INPUT=$(cat)

SESSION_ID=$(printf '%s' "$INPUT" | jq -r '.session_id // ""')
STOP_ACTIVE=$(printf '%s' "$INPUT" | jq -r '.stop_hook_active // false')
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // ""')

# Loop guard: we are already in a hook-triggered continuation.
[ "$STOP_ACTIVE" = "true" ] && exit 0

[ -n "$SESSION_ID" ] || SESSION_ID="default"
buffer="${TMPDIR:-/tmp}/claude-permission-prompt-tuner/${SESSION_ID}.jsonl"
[ -s "$buffer" ] || exit 0

# Resolve where records live. Prefer SKILL_OBSERVE_HOME (the my-pde checkout) so
# prompt records land in the git-tracked skills tree regardless of the work repo;
# fall back to the current repo when the env var is unset (e.g. working in my-pde).
[ -n "$CWD" ] || CWD=$(pwd)
OBS_HOME=""
observe_home="${SKILL_OBSERVE_HOME:-}"
# shellcheck disable=SC2088  # intentionally matching/stripping a literal ~, not expanding
case "$observe_home" in
"~") observe_home="$HOME" ;;
"~/"*) observe_home="$HOME/${observe_home#"~/"}" ;;
esac
if [ -n "$observe_home" ] && [ -d "$observe_home/ai-agents/skills" ]; then
	OBS_HOME="$observe_home"
else
	ROOT=$(git -C "$CWD" rev-parse --show-toplevel 2>/dev/null || true)
	if [ -n "$ROOT" ] && [ -d "$ROOT/ai-agents/skills" ]; then
		OBS_HOME="$ROOT"
	fi
fi
[ -n "$OBS_HOME" ] || exit 0
records_dir="$OBS_HOME/ai-agents/skills/permission-prompt-tuner/records"

reason=$(
	cat <<REASON
このセッションで permission prompt が出た（自動許可されなかった）コマンドが検出されています。
permission-prompt-tuner スキルを実行して、記録と settings.json への提案を行ってください。

- 検出バッファ（1 行 1 コマンドの JSONL）: ${buffer}
- 記録先ディレクトリ（絶対パス。現在の作業リポジトリと異なっても必ずここに書く）: ${records_dir}
- 実行後、バッファ ${buffer} を空にする（同じプロンプトの再通知を防ぐため）。
- settings.json は編集せず、提案の提示に留める。
REASON
)

jq -n --arg r "$reason" '{decision: "block", reason: $r}'
exit 0
