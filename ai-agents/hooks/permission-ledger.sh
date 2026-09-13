#!/usr/bin/env bash
set -eu

command -v jq >/dev/null 2>&1 || exit 0

state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/claude-permission-ledger"
umask 077
mkdir -p "$state_dir"

jq -c 'select(.tool_name | IN("AskUserQuestion", "ExitPlanMode") | not) | {
	ts: (now | todate),
	session_id,
	prompt_id,
	agent_id,
	tool_name,
	cwd,
	transcript_path,
	permission_mode,
	tool_input: (.tool_input | if type == "object" then del(.content, .old_string, .new_string, .new_source, .edits) else . end),
	suggestions: (.permission_suggestions // [])
}' >>"$state_dir/ledger.jsonl" 2>/dev/null || true

exit 0
