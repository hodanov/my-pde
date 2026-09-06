#!/usr/bin/env bash
# SessionStart hook (matcher=compact): context-anchor-save.sh が PreCompact で
# 退避したユーザー発話の原文を additionalContext として再注入する。
# 要約は「何をやったか」を残すが原文の指示・制約は言い換えで落ちるため、
# コンパクション直後の前提ずれをここで機械的に埋め戻す。
# 素材を渡すだけで判断はモデルに委ねる。情報提供専用なので常に exit 0。
set -u

command -v jq >/dev/null 2>&1 || exit 0

INPUT=$(cat)

SESSION_ID=$(printf '%s' "$INPUT" | jq -r '.session_id // ""' 2>/dev/null)
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // ""' 2>/dev/null)

state_dir="${TMPDIR:-/tmp}/claude-context-anchor"
[ -d "$state_dir" ] || exit 0

keys=""
[ -n "$SESSION_ID" ] && keys="$SESSION_ID"
if [ -n "$CWD" ]; then
	keys="$keys cwd-$(printf '%s' "$CWD" | cksum | cut -d' ' -f1)"
fi
[ -n "$keys" ] || keys="default"

first=""
recent=""
for key in $keys; do
	if [ -z "$first" ] && [ -s "$state_dir/$key.first.md" ]; then
		first=$(cat "$state_dir/$key.first.md" 2>/dev/null || true)
	fi
	if [ -z "$recent" ] && [ -s "$state_dir/$key.recent.md" ]; then
		recent=$(cat "$state_dir/$key.recent.md" 2>/dev/null || true)
	fi
done

{ [ -n "$first" ] || [ -n "$recent" ]; } || exit 0

ctx="コンパクション前のユーザー指示（原文）。要約より原文を優先し、ここに残る制約・禁止事項を守って作業を再開すること。"
[ -n "$first" ] && ctx="$ctx"$'\n\n'"## 最初の指示"$'\n\n'"$first"
[ -n "$recent" ] && ctx="$ctx"$'\n\n'"## 直近の指示"$'\n\n'"$recent"

jq -n --arg ctx "$ctx" \
	'{hookSpecificOutput:{hookEventName:"SessionStart",additionalContext:$ctx}}'
exit 0
