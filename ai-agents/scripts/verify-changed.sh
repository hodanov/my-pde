#!/usr/bin/env bash
# Deterministic verification for changed files: detect the change set, map each
# file to the repo's mise tasks / linters, run every applicable check, and print
# a compact [PASS]/[FAIL]/[SKIP] report. Exits 1 if any check fails.
#
# Usage: verify-changed.sh [file ...]
#   No args: verify the union of unstaged, staged, and untracked files
#   (same detection as the lint-changed.sh Stop hook).
#
# Consumed by the verify-runner subagent via `mise run verify:changed`;
# humans can run it the same way.

# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
# tombi / prettier / markdownlint-cli2 live in the node toolchain, not on PATH
export PATH="$repo_root/environment/tools/node/node_modules/.bin:$PATH"

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
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "[SKIP] $* ($tool not installed)"
		skip=$((skip + 1))
		return 0
	fi
	local out
	if out=$("$@" 2>&1); then
		echo "[PASS] $*"
		pass=$((pass + 1))
	else
		echo "[FAIL] $*"
		[ -n "$out" ] && printf '%s\n' "$out"
		fail=$((fail + 1))
	fi
}

go_apps=""
declare -a lua_files sh_files md_files fmt_files
run_tombi=0
run_actionlint=0
run_hadolint=0
unmapped=""

while IFS= read -r f; do
	{ [ -n "$f" ] && [ -f "$f" ]; } || continue
	mapped=0
	case "$f" in
	scripts/*/*)
		app=${f#scripts/}
		app=${app%%/*}
		if [ -f "scripts/$app/go.mod" ]; then
			case " $go_apps " in
			*" $app "*) ;;
			*) go_apps="$go_apps $app" ;;
			esac
			mapped=1
		fi
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
	environment/docker/nvim.dockerfile)
		run_hadolint=1
		mapped=1
		;;
	esac
	if [ "$mapped" -eq 0 ]; then
		unmapped="${unmapped}  - $f"$'\n'
	fi
done <<EOF
$files
EOF

# Tests first, then linters.
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
if [ "$run_hadolint" -eq 1 ]; then
	run_check hadolint hadolint environment/docker/nvim.dockerfile
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
