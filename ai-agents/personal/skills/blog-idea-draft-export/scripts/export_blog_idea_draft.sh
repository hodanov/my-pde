#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  export_blog_idea_draft.sh --slug <kebab-case> [--date <YYYY-MM-DD>] [--output-dir <dir>] [--force]

Behavior:
  - Output dir priority: --output-dir > BLOG_IDEA_DRAFT_EXPORT_DIR > interactive prompt (TTY only).
  - Markdown content is read from stdin.
  - File name: YYYY-MM-DD_<slug>.md
EOF
}

prompt_from_tty() {
	local __var_name="$1"
	local __prompt="$2"
	local __value=""

	if [[ ! -t 0 && ! -t 1 ]]; then
		return 1
	fi

	if read -r -p "$__prompt" __value </dev/tty; then
		printf -v "$__var_name" "%s" "$__value"
		return 0
	fi

	return 1
}

slug=""
draft_date=""
output_dir=""
force_overwrite="0"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--slug)
		slug="${2:-}"
		shift 2
		;;
	--date)
		draft_date="${2:-}"
		shift 2
		;;
	--output-dir)
		output_dir="${2:-}"
		shift 2
		;;
	--force)
		force_overwrite="1"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ -z "$slug" ]]; then
	echo "Error: --slug is required." >&2
	usage >&2
	exit 1
fi

if [[ ! "$slug" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
	echo "Error: slug must be kebab-case ASCII (e.g. my-post-title)." >&2
	exit 1
fi

if [[ -z "$draft_date" ]]; then
	draft_date="$(date +%F)"
fi

if [[ -z "$output_dir" ]]; then
	output_dir="${BLOG_IDEA_DRAFT_EXPORT_DIR:-}"
fi

if [[ -z "$output_dir" ]]; then
	default_dir="$HOME/workspace/hodalog-hugo/docs/idea"
	if prompt_from_tty user_dir "Output directory (default: ${default_dir}): "; then
		output_dir="${user_dir:-$default_dir}"
	else
		echo "Error: output directory is not set." >&2
		echo "Set BLOG_IDEA_DRAFT_EXPORT_DIR or pass --output-dir." >&2
		echo "In skill execution, ask the user for a directory and pass --output-dir." >&2
		exit 1
	fi
fi

mkdir -p "$output_dir"
target_file="${output_dir}/${draft_date}_${slug}.md"

if [[ -e "$target_file" && "$force_overwrite" != "1" ]]; then
	if prompt_from_tty answer "File exists: ${target_file}. Overwrite? [y/N]: "; then
		# shellcheck disable=SC2154
		case "$answer" in
		y | Y | yes | YES) ;;
		*)
			echo "Canceled."
			exit 1
			;;
		esac
	else
		echo "Error: target file already exists: ${target_file}" >&2
		echo "Use --force in non-interactive mode." >&2
		exit 1
	fi
fi

cat >"$target_file"

echo "$target_file"
