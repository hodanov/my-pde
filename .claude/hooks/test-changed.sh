#!/usr/bin/env bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# Stop hook (my-pde only, not distributed): run tests for changed Go apps and
# surface failures. Non-blocking & report-only — it mirrors lint-changed.sh and
# is the test-side counterpart lint-changed lacks. Opt out with
# TEST_CHANGED_DISABLE=1. Exits fast when no Go app under scripts/ changed.
#
# The scripts/<app> path pattern is shared with ai-agents/scripts/verify-changed.sh
# (the human `mise run verify:changed` / verify-runner entry point). Keep the two
# in sync when the repo layout changes.
set -u

[ "${TEST_CHANGED_DISABLE:-0}" = "1" ] && exit 0
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0

files=$(
	{
		git diff --name-only --diff-filter=d
		git diff --name-only --cached --diff-filter=d
		git ls-files --others --exclude-standard
	} 2>/dev/null | sort -u
)
[ -n "$files" ] || exit 0

go_apps=""

while IFS= read -r f; do
	{ [ -n "$f" ] && [ -f "$f" ]; } || continue
	case "$f" in
	scripts/*/*.go | scripts/*/go.mod | scripts/*/go.sum | scripts/*/testdata/*)
		app=${f#scripts/}
		app=${app%%/*}
		case " $go_apps " in
		*" $app "*) ;;
		*) go_apps="$go_apps $app" ;;
		esac
		;;
	esac
done <<EOF
$files
EOF

{ [ -n "$go_apps" ] && command -v go >/dev/null 2>&1; } || exit 0

failures=""
for app in $go_apps; do
	(cd "scripts/$app" && go test -timeout=60s ./...) >/dev/null 2>&1 ||
		failures="${failures}- [go test] scripts/$app"$'\n'
done

if [ -n "$failures" ]; then
	printf 'test failures on changed Go apps (fix before marking the task done):\n%s' "$failures"
fi
exit 0
