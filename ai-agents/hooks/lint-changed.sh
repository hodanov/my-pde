#!/usr/bin/env bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# Stop hook: lint files changed in the working tree and surface issues.
# Non-blocking — it only reports; it never fails the turn.
set -u
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT=$(cat)
[ "$(jq -r '.stop_hook_active // false' <<<"$INPUT")" = "false" ] || exit 0
session_id=$(jq -r '.session_id // empty' <<<"$INPUT")

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0

files=$(
	{
		git diff --name-only --diff-filter=d
		git diff --name-only --cached --diff-filter=d
		git ls-files --others --exclude-standard
	} 2>/dev/null | sort -u
)
[ -n "$files" ] || exit 0

issues=""
go_dirs=""

append_issue() {
	issues="${issues}$1"$'\n'
}

while IFS= read -r f; do
	{ [ -n "$f" ] && [ -f "$f" ]; } || continue
	case "$f" in
	*.go)
		d=$(dirname "$f")
		case " $go_dirs " in
		*" $d "*) ;;
		*) go_dirs="$go_dirs $d" ;;
		esac
		;;
	*.lua)
		if command -v stylua >/dev/null 2>&1 && ! stylua --check "$f" >/dev/null 2>&1; then
			append_issue "- [stylua] $f"
		fi
		;;
	*.md)
		if command -v markdownlint-cli2 >/dev/null 2>&1 && ! markdownlint-cli2 --config "$SCRIPT_DIR/.markdownlint-cli2.yaml" "$f" >/dev/null 2>&1; then
			append_issue "- [markdownlint] $f"
		fi
		;;
	*.sh)
		if command -v shellcheck >/dev/null 2>&1 && ! shellcheck "$f" >/dev/null 2>&1; then
			append_issue "- [shellcheck] $f"
		fi
		;;
	.github/workflows/*.yml | .github/workflows/*.yaml)
		# actionlint bundles its inline `run:` scripts to shellcheck when present.
		if command -v actionlint >/dev/null 2>&1 && ! actionlint "$f" >/dev/null 2>&1; then
			append_issue "- [actionlint] $f"
		fi
		;;
	Dockerfile | Dockerfile.* | *.dockerfile | */Dockerfile | */Dockerfile.*)
		if command -v hadolint >/dev/null 2>&1 && ! hadolint "$f" >/dev/null 2>&1; then
			append_issue "- [hadolint] $f"
		fi
		;;
	esac
done <<EOF
$files
EOF

if command -v golangci-lint >/dev/null 2>&1; then
	for d in $go_dirs; do
		if ! golangci-lint run "$d" >/dev/null 2>&1; then
			append_issue "- [golangci-lint] $d"
		fi
	done
fi

last_report_file="${TMPDIR:-/tmp}/claude-lint-changed-${session_id}"

if [ -z "$issues" ]; then
	rm -f "$last_report_file"
	exit 0
fi

if [ -n "$session_id" ]; then
	printf '%s' "$issues" | cmp -s - "$last_report_file" && exit 0
	printf '%s' "$issues" >"$last_report_file"
fi

jq -n --arg ctx "lint issues on changed files (fix before marking the task done):
$issues" '{hookSpecificOutput:{hookEventName:"Stop",additionalContext:$ctx}}'
exit 0
