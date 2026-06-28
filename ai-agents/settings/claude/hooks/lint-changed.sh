#!/usr/bin/env bash
# Stop hook: lint files changed in the working tree and surface issues.
# Non-blocking — it only reports; it never fails the turn.
set -u

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
		if command -v markdownlint-cli2 >/dev/null 2>&1 && ! markdownlint-cli2 "$f" >/dev/null 2>&1; then
			append_issue "- [markdownlint] $f"
		fi
		;;
	*.sh)
		if command -v shellcheck >/dev/null 2>&1 && ! shellcheck "$f" >/dev/null 2>&1; then
			append_issue "- [shellcheck] $f"
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

if [ -n "$issues" ]; then
	printf 'lint issues on changed files (fix before marking the task done):\n%s' "$issues"
fi
exit 0
