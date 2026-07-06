#!/usr/bin/env bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# WorktreeCreate hook: place session/subagent worktrees under
# ${WORKTREE_BASE:-$HOME/workspace/.worktrees}/<repo>/<name>/ instead of
# .claude/worktrees/ inside the checkout, so every repo shares one layout and
# worktrees never pollute the working tree AI agents read.
# stdin: hook JSON (.name = worktree name, .cwd = launch directory; schema
# captured from a live run). stdout: the created path.
# Non-zero exit aborts worktree creation, so collisions never overwrite.
# The branch is a placeholder named after <name>; the session agent renames it
# to a proper slug with `git branch -m <slug>` (see AGENTS.md, Parallel Work).
set -euo pipefail

input=$(cat)
name=$(jq -r '.name // empty' <<<"$input")
cwd=$(jq -r '.cwd // empty' <<<"$input")

if [ -z "$name" ] || [ -z "$cwd" ]; then
	echo "worktree-create: missing name or cwd in hook input" >&2
	exit 1
fi

# cwd may be a subdirectory; fails (and aborts) when cwd is not a git repo.
source_path=$(git -C "$cwd" rev-parse --show-toplevel)
repo=$(basename "$source_path")
dest="${WORKTREE_BASE:-$HOME/workspace/.worktrees}/$repo/$name"

if [ -e "$dest" ]; then
	echo "worktree-create: destination already exists: $dest" >&2
	exit 1
fi
mkdir -p "$(dirname "$dest")"

# Branch from origin/HEAD for a clean tree matching the remote; fall back to
# local HEAD when there is no remote, the fetch fails, or origin/HEAD is unset
# (mirrors the default --worktree behavior).
base_ref="HEAD"
if git -C "$source_path" remote get-url origin >/dev/null 2>&1 &&
	git -C "$source_path" fetch origin >/dev/null 2>&1; then
	origin_head=$(git -C "$source_path" symbolic-ref --quiet refs/remotes/origin/HEAD || true)
	if [ -n "$origin_head" ]; then
		base_ref="$origin_head"
	fi
fi

# -b fails on an existing branch; keep stdout clean for the path contract.
git -C "$source_path" worktree add -b "$name" "$dest" "$base_ref" >&2
echo "$dest"
