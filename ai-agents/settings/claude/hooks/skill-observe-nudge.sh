#!/usr/bin/env bash
# Stop hook: detect repository skills used this session that have no observation
# yet, and block once so Claude auto-records them (Observe phase automation).
# Non-destructive — it only nudges; the observation content is written by Claude.
set -u

command -v jq >/dev/null 2>&1 || exit 0

INPUT=$(cat)

TRANSCRIPT=$(printf '%s' "$INPUT" | jq -r '.transcript_path // ""')
SESSION_ID=$(printf '%s' "$INPUT" | jq -r '.session_id // ""')
STOP_ACTIVE=$(printf '%s' "$INPUT" | jq -r '.stop_hook_active // false')
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // ""')

# Loop guard 1: we are already in a hook-triggered continuation.
[ "$STOP_ACTIVE" = "true" ] && exit 0

[ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ] || exit 0

# Resolve where observations live. Prefer SKILL_OBSERVE_HOME (the my-pde checkout)
# so skill usage in ANY repo is captured into the git-tracked observations tree;
# fall back to the current repo when the env var is unset (e.g. working in my-pde).
[ -n "$CWD" ] || CWD=$(pwd)
OBS_HOME=""
# Expand a leading ~ ourselves; settings.json env values are not shell-expanded,
# so SKILL_OBSERVE_HOME="~/workspace/my-pde" stays portable across machines.
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
SKILLS_DIR="$OBS_HOME/ai-agents/skills"

# Skills extracted from this session's transcript (Skill tool invocations).
used=$(
	jq -r 'select(.message.content != null)
		| .message.content[]?
		| select(.type == "tool_use" and .name == "Skill")
		| .input.skill // empty' "$TRANSCRIPT" 2>/dev/null | sort -u
)
[ -n "$used" ] || exit 0

# Session state: skills already nudged in this session (loop guard 2).
state_dir="${TMPDIR:-/tmp}/claude-skill-observe-nudge"
mkdir -p "$state_dir" 2>/dev/null || exit 0
[ -n "$SESSION_ID" ] || SESSION_ID="default"
state_file="$state_dir/$SESSION_ID"
[ -f "$state_file" ] || : >"$state_file"

pending=""
while IFS= read -r skill; do
	[ -n "$skill" ] || continue
	# Only repository skills, excluding the improvement-loop skills themselves.
	[ -f "$SKILLS_DIR/$skill/SKILL.md" ] || continue
	case "$skill" in
	skill-observe | skill-improve) continue ;;
	esac
	# Skip if already nudged this session.
	if grep -qxF "$skill" "$state_file" 2>/dev/null; then
		continue
	fi
	pending="${pending}${skill}"$'\n'
	printf '%s\n' "$skill" >>"$state_file"
done <<EOF
$used
EOF

[ -n "$pending" ] || exit 0

# Build a comma-separated list for the reason text.
list=$(printf '%s' "$pending" | sed '/^$/d' | paste -sd ',' -)

reason=$(
	cat <<REASON
今セッションで次のリポジトリスキルを使用したが observation が未記録です: ${list}

各スキルについて、直近の使用結果を会話文脈から success / partial / failure で判定し、
${SKILLS_DIR}/<スキル名>/observations/$(date +%Y-%m-%d)_NNN_obs.md を作成してください
（上記は絶対パス。現在の作業リポジトリと異なっても必ずこのパスに書く）。

- 形式は ${SKILLS_DIR}/skill-observe/SKILL.md の6フィールド（タスク / スキル / 結果 /
  問題 / フィードバック / コンテキスト）に従う。
- 「コンテキスト」には実際の作業リポジトリ（cwd: ${CWD}）を必ず記録する。
- NNN は当日連番。同日の既存ファイルと重複しないよう採番する。
- 当日同スキルの observation が既にあればスキップしてよい。
- ユーザーへの確認は不要。淡々と記録し、作成したファイルを1行で報告して終了する。
- markdownlint を通すため、既存 observation ファイルの体裁に合わせる。
REASON
)

jq -n --arg r "$reason" '{decision: "block", reason: $r}'
exit 0
