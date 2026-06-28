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

# Locate the repository and its skills directory.
[ -n "$CWD" ] || CWD=$(pwd)
ROOT=$(git -C "$CWD" rev-parse --show-toplevel 2>/dev/null) || exit 0
SKILLS_DIR="$ROOT/ai-agents/skills"
[ -d "$SKILLS_DIR" ] || exit 0

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
ai-agents/skills/<スキル名>/observations/$(date +%Y-%m-%d)_NNN_obs.md を作成してください。

- 形式は ai-agents/skills/skill-observe/SKILL.md の6フィールド（タスク / スキル / 結果 /
  問題 / フィードバック / コンテキスト）に従う。
- NNN は当日連番。同日の既存ファイルと重複しないよう採番する。
- 当日同スキルの observation が既にあればスキップしてよい。
- ユーザーへの確認は不要。淡々と記録し、作成したファイルを1行で報告して終了する。
- markdownlint を通すため、既存 observation ファイルの体裁に合わせる。
REASON
)

jq -n --arg r "$reason" '{decision: "block", reason: $r}'
exit 0
