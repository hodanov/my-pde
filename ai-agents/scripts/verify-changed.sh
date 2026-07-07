#!/usr/bin/env bash
# Deterministic verification for changed files in this repository (my-pde):
# detect the change set, map each file to the repo's mise tasks and linters,
# run every applicable check, and print a compact [PASS]/[FAIL]/[SKIP]
# report. Exits 1 if any check fails.
#
# Usage: verify-changed.sh [file ...]
#   No args: verify the union of unstaged, staged, and untracked files
#   (same detection as the lint-changed.sh Stop hook).
#
# Missing tools degrade to SKIP. Humans run it via `mise run verify:changed`;
# the verify-runner subagent runs the script directly. This script is
# intentionally my-pde specific — in repositories without it, the subagent
# discovers verification commands from the project's config instead.

# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
# repo-local node toolchain (tombi / prettier / markdownlint-cli2, ...)
if [ -d "$repo_root/environment/tools/node/node_modules/.bin" ]; then
	export PATH="$repo_root/environment/tools/node/node_modules/.bin:$PATH"
fi

if [ "$#" -gt 0 ]; then
	files=$(printf '%s\n' "$@")
else
	files=$(
		{
			git diff --name-only --diff-filter=d
			git diff --name-only --cached --diff-filter=d
			git ls-files --others --exclude-standard
		} 2>/dev/null | sort -u
	)
fi

pass=0
fail=0
skip=0

# run_check <tool-for-command-v-guard> <command...>
run_check() {
	local tool=$1
	shift
	local label="$*"
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "[SKIP] $label ($tool not installed)"
		skip=$((skip + 1))
		return 0
	fi
	local out
	if out=$("$@" 2>&1); then
		echo "[PASS] $label"
		pass=$((pass + 1))
	else
		echo "[FAIL] $label"
		[ -n "$out" ] && printf '%s\n' "$out"
		fail=$((fail + 1))
	fi
}

go_apps=""
declare -a lua_files sh_files md_files fmt_files docker_files
run_tombi=0
run_actionlint=0
unmapped=""

while IFS= read -r f; do
	{ [ -n "$f" ] && [ -f "$f" ]; } || continue
	mapped=0
	# This scripts/<app> Go pattern is mirrored in .claude/hooks/test-changed.sh
	# (the report-only Stop hook). Keep the two in sync when the layout changes.
	case "$f" in
	scripts/*/*.go | scripts/*/go.mod | scripts/*/go.sum | scripts/*/testdata/*)
		app=${f#scripts/}
		app=${app%%/*}
		case " $go_apps " in
		*" $app "*) ;;
		*) go_apps="$go_apps $app" ;;
		esac
		mapped=1
		;;
	esac
	case "$f" in
	*.lua)
		lua_files+=("$f")
		mapped=1
		;;
	*.sh)
		sh_files+=("$f")
		mapped=1
		;;
	*.md)
		md_files+=("$f")
		mapped=1
		;;
	*.toml)
		run_tombi=1
		mapped=1
		;;
	.github/workflows/*.yml | .github/workflows/*.yaml)
		run_actionlint=1
		fmt_files+=("$f")
		mapped=1
		;;
	*.json | *.yml | *.yaml)
		fmt_files+=("$f")
		mapped=1
		;;
	*.dockerfile)
		docker_files+=("$f")
		mapped=1
		;;
	esac
	if [ "$mapped" -eq 0 ]; then
		unmapped="${unmapped}  - $f"$'\n'
	fi
done <<EOF
$files
EOF

# Tests first, then linters. Go apps under scripts/ ship <app>:test / <app>:lint
# mise tasks by repo convention (AGENTS.md).
for app in $go_apps; do
	run_check mise mise run "$app:test"
done
for app in $go_apps; do
	run_check mise mise run "$app:lint"
done
if [ "${#lua_files[@]}" -gt 0 ]; then
	run_check stylua stylua --check "${lua_files[@]}"
fi
if [ "${#sh_files[@]}" -gt 0 ]; then
	run_check shfmt shfmt -d "${sh_files[@]}"
	run_check shellcheck shellcheck "${sh_files[@]}"
fi
if [ "${#md_files[@]}" -gt 0 ]; then
	run_check markdownlint-cli2 markdownlint-cli2 "${md_files[@]}"
fi
if [ "$run_tombi" -eq 1 ]; then
	run_check tombi tombi lint
fi
if [ "${#fmt_files[@]}" -gt 0 ]; then
	run_check prettier prettier --check "${fmt_files[@]}"
fi
if [ "$run_actionlint" -eq 1 ]; then
	run_check actionlint actionlint
fi
if [ "${#docker_files[@]}" -gt 0 ]; then
	run_check hadolint hadolint "${docker_files[@]}"
fi

echo "----"
if [ -n "$unmapped" ]; then
	printf 'no check mapped (verify manually if needed):\n%s' "$unmapped"
fi
total=$((pass + fail + skip))
if [ "$total" -eq 0 ] && [ -z "$unmapped" ]; then
	echo "result: PASS (no changed files to verify)"
	exit 0
fi
if [ "$fail" -gt 0 ]; then
	echo "result: FAIL ($fail failed, $pass passed, $skip skipped)"
	exit 1
fi
echo "result: PASS ($pass passed, $skip skipped)"
