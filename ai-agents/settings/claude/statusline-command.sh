#!/bin/sh
# Claude Code statusline.
# Shows: model | git branch | dir | context used% | (activity indicator, if any)
#
# LIMITATION (read this before assuming the activity indicator is complete):
# Claude Code does not expose a "N background tasks running" field to
# statusline scripts. The only signal available here is the session
# transcript file (transcript_path), which we scan for a tool_use entry
# that has no matching tool_result yet -- i.e. a tool call still in flight
# at the moment this script runs. That reliably catches a *foreground*
# subagent (Agent tool) or a long-running foreground tool call. It does NOT
# catch backgrounded Bash shells started with run_in_background (e.g.
# `gh run watch ...` run in the background): those return control (and a
# shell id) immediately, so the transcript shows them as "done" the instant
# they start, even though the shell keeps running. There is currently no
# way for an external statusline script to see "there's a background shell
# still going" -- only Claude Code's own UI knows that.

input=$(cat)

model=$(printf '%s' "$input" | jq -r '.model.display_name // "Claude"')
dir=$(printf '%s' "$input" | jq -r '.workspace.current_dir // .cwd // empty')
transcript=$(printf '%s' "$input" | jq -r '.transcript_path // empty')
used=$(printf '%s' "$input" | jq -r '.context_window.used_percentage // empty')

short_dir=""
if [ -n "$dir" ]; then
	base=$(basename "$dir")
	parent=$(basename "$(dirname "$dir")")
	if [ -n "$parent" ] && [ "$parent" != "/" ]; then
		short_dir="$parent/$base"
	else
		short_dir="$base"
	fi
fi

branch=""
if [ -n "$dir" ] && git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	branch=$(git -C "$dir" --no-optional-locks branch --show-current 2>/dev/null)
fi

# Look for an in-flight tool call (foreground subagent / long tool) in the
# tail of the transcript. See LIMITATION note above.
activity=""
if [ -n "$transcript" ] && [ -f "$transcript" ]; then
	activity=$(tail -n 300 "$transcript" 2>/dev/null |
		jq -cR 'fromjson? // empty' |
		jq -rs '
        def content: ((.message.content? // []) | if type == "array" then . else [] end);
        ([ .[] | select(.type=="assistant") | content[] | select(.type=="tool_use") | {id: .id, name: .name} ]) as $calls |
        ([ .[] | select(.type=="user") | content[] | select(.type=="tool_result") | .tool_use_id ]) as $done |
        ($calls | map(select(.id as $id | ($done | index($id)) == null))) as $pending |
        if ($pending | length) > 0 then
          (if ($pending | any(.name == "Agent" or .name == "Task")) then "subagent"
           else ($pending[0].name // "tool")
           end)
        else "" end
      ' 2>/dev/null)
fi

dim() { printf '\033[2m%s\033[0m' "$1"; }
warn() { printf '\033[33m%s\033[0m' "$1"; }

line=$(dim "$model")
[ -n "$branch" ] && line="$line $(dim "| git:$branch")"
[ -n "$short_dir" ] && line="$line $(dim "| $short_dir")"
[ -n "$used" ] && line="$line $(dim "| ctx:${used}%")"

if [ -n "$activity" ]; then
	if [ "$activity" = "subagent" ]; then
		line="$line $(warn '| ⏳ subagent running')"
	else
		line="$line $(warn "| ⏳ ${activity} running")"
	fi
fi

printf '%s' "$line"
