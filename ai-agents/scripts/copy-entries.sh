#!/usr/bin/env bash
set -eu

usage() {
	echo "Usage: $0 [--force] <mode> <src> <dest>"
	echo "  --force: overwrite existing entries without prompting"
	echo "  mode: skills | agents | settings"
	exit 1
}

force=0
if [ "${1:-}" = "--force" ]; then
	force=1
	shift
fi

[ $# -eq 3 ] || usage

mode="$1"
src="$2"
dest="$3"

if [ ! -d "$src" ]; then
	echo "Source directory not found: $src"
	exit 1
fi

if [ -L "$dest" ]; then
	echo "Destination is a symlink. Remove it before copying: $dest"
	exit 1
fi

mkdir -p "$dest"

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

case "$mode" in
skills)
	find "$src" -mindepth 1 -maxdepth 1 -type d -print0 >"$tmp"
	;;
agents)
	find "$src" -maxdepth 1 -name '*.md' -type f -print0 >"$tmp"
	;;
settings)
	find "$src" -type f -print0 >"$tmp"
	;;
*)
	echo "Unknown mode: $mode"
	exit 1
	;;
esac

if [ ! -s "$tmp" ]; then
	echo "No entries found in $src"
	exit 0
fi

# Build duplicate list
dup_found=0
dup_list=""
while IFS= read -r -d '' entry; do
	if [ "$mode" = "settings" ]; then
		label="${entry#"$src"/}"
	else
		label=$(basename "$entry")
	fi
	if [ -e "$dest/$label" ]; then
		dup_found=1
		dup_list="${dup_list}  ${label}"$'\n'
	fi
done <"$tmp"

overwrite=$force
if [ "$dup_found" -eq 1 ] && [ "$force" -ne 1 ]; then
	echo "The following entries already exist in $dest:"
	printf "%s" "$dup_list"
	printf "Overwrite? [y/N] "
	read -r ans
	# shellcheck disable=SC2249
	case "$ans" in
	y | Y) overwrite=1 ;;
	esac
fi

# Copy entries
while IFS= read -r -d '' entry; do
	if [ "$mode" = "settings" ]; then
		label="${entry#"$src"/}"
	else
		label=$(basename "$entry")
	fi
	target="$dest/$label"

	if [ -e "$target" ] && [ "$overwrite" -ne 1 ]; then
		echo "Skip $label (already exists)"
		continue
	fi

	case "$mode" in
	skills)
		rm -rf "$target"
		cp -R "$entry" "$target"
		;;
	agents)
		cp "$entry" "$target"
		;;
	settings)
		mkdir -p "$(dirname "$target")"
		cp -p "$entry" "$target"
		;;
	esac
	echo "Installed $label"
done <"$tmp"

echo "Done."
