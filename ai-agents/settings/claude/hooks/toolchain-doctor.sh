#!/usr/bin/env bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# SessionStart hook: report expected dev tools missing on PATH so the deployed
# formatter/lint hooks (which `command -v <tool> || exit 0` and then silently
# skip) don't no-op unnoticed. Fast & read-only: only `command -v` checks.
# Prints nothing when everything is present, keeping the session context clean.
set -u

# tool -> hook(s) that silently no-op when the tool is missing.
tools="goimports golangci-lint shfmt shellcheck prettier stylua markdownlint-cli2 tombi"

hook_for() {
	case "$1" in
	goimports) echo "goimports.sh" ;;
	golangci-lint) echo "lint-changed.sh (Go)" ;;
	shfmt) echo "shfmt.sh" ;;
	shellcheck) echo "lint-changed.sh (shell)" ;;
	prettier) echo "prettier.sh, markdown-format.sh" ;;
	stylua) echo "stylua.sh, lint-changed.sh (Lua)" ;;
	markdownlint-cli2) echo "lint-changed.sh (Markdown)" ;;
	tombi) echo "tombi.sh" ;;
	*) echo "formatter/lint hook" ;;
	esac
}

missing=""
for t in $tools; do
	command -v "$t" >/dev/null 2>&1 || missing="${missing} $t"
done

[ -n "$missing" ] || exit 0

echo "toolchain-doctor: the following dev tools are missing on PATH."
echo "Their formatter/lint hooks will silently no-op until they are installed:"
for t in $missing; do
	printf '  - %-18s -> %s\n' "$t" "$(hook_for "$t")"
done
echo "Fix: run \`mise install\` at the repo root (installs pinned tools from mise.toml)."
