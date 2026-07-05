#!/usr/bin/env bash
# Deterministic verification for changed files, usable in any git repo: detect
# the change set, map each file to verification commands — preferring the
# repo's mise tasks when they exist — run every applicable check, and print a
# compact [PASS]/[FAIL]/[SKIP] report. Exits 1 if any check fails.
#
# Usage: verify-changed.sh [file ...]
#   No args: verify the union of unstaged, staged, and untracked files
#   (same detection as the lint-changed.sh Stop hook).
#   Runs against the repository containing the current working directory.
#
# Missing tools degrade to SKIP, so this works without mise or the node-based
# linters. `mise run agents-copy` symlinks it to ~/.local/bin/verify-changed;
# the verify-runner subagent runs it from there in repos that do not ship
# their own copy. Humans can run it via `mise run verify:changed`.

# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
# repo-local node toolchains (tombi / prettier / markdownlint-cli2, ...)
for d in "$repo_root/environment/tools/node/node_modules/.bin" "$repo_root/node_modules/.bin"; do
	if [ -d "$d" ]; then
		PATH="$d:$PATH"
	fi
done
export PATH

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

mise_tasks=""
if command -v mise >/dev/null 2>&1; then
	mise_tasks=$(mise tasks ls 2>/dev/null | awk '{print $1}')
fi

has_mise_task() {
	printf '%s\n' "$mise_tasks" | grep -qx "$1"
}

pass=0
fail=0
skip=0

# run_check [-C dir] <tool-for-command-v-guard> <command...>
run_check() {
	local dir=.
	if [ "$1" = "-C" ]; then
		dir=$2
		shift 2
	fi
	local tool=$1
	shift
	local label="$*"
	if [ "$dir" != "." ]; then
		label="($dir) $*"
	fi
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "[SKIP] $label ($tool not installed)"
		skip=$((skip + 1))
		return 0
	fi
	local out
	if out=$(cd "$dir" && "$@" 2>&1); then
		echo "[PASS] $label"
		pass=$((pass + 1))
	else
		echo "[FAIL] $label"
		[ -n "$out" ] && printf '%s\n' "$out"
		fail=$((fail + 1))
	fi
}

# nearest_go_mod <repo-relative path>: echo the closest ancestor dir holding
# a go.mod, or nothing if the file is outside any Go module.
nearest_go_mod() {
	local d
	d=$(dirname "$1")
	while :; do
		if [ -f "$d/go.mod" ]; then
			echo "$d"
			return 0
		fi
		if [ "$d" = "." ]; then
			return 0
		fi
		d=$(dirname "$d")
	done
}

go_mod_dirs=""
declare -a lua_files sh_files md_files fmt_files docker_files
run_tombi=0
run_actionlint=0
unmapped=""

while IFS= read -r f; do
	{ [ -n "$f" ] && [ -f "$f" ]; } || continue
	mapped=0
	case "$f" in
	*.go | go.mod | */go.mod | go.sum | */go.sum | *testdata/*)
		mod_dir=$(nearest_go_mod "$f")
		if [ -n "$mod_dir" ]; then
			case " $go_mod_dirs " in
			*" $mod_dir "*) ;;
			*) go_mod_dirs="$go_mod_dirs $mod_dir" ;;
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
	Dockerfile | */Dockerfile | Dockerfile.* | */Dockerfile.* | *.dockerfile)
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

# Tests first, then linters. Per Go module: prefer the repo's mise tasks
# (named <module dir basename>:test / :lint), else plain go tooling.
for d in $go_mod_dirs; do
	app=$(basename "$d")
	if has_mise_task "$app:test"; then
		run_check mise mise run "$app:test"
	else
		run_check -C "$d" go go test ./...
	fi
done
for d in $go_mod_dirs; do
	app=$(basename "$d")
	if has_mise_task "$app:lint"; then
		run_check mise mise run "$app:lint"
	else
		run_check -C "$d" golangci-lint golangci-lint run
	fi
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
