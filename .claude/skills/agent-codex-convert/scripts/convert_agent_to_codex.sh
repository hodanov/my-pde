#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  convert_agent_to_codex.sh [OPTIONS] FILE [FILE ...]

Convert Claude/Cursor agent markdown files to Codex CLI TOML format.

Options:
  --reasoning-effort <low|medium|high>  Model reasoning effort (default: medium)
  --output-dir <dir>                    Output directory (default: agents/codex)
  --force                               Overwrite without prompting
  --dry-run                             Print to stdout, don't write files
  -h, --help                            Show this help
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

# --- Argument parsing ---
reasoning_effort="medium"
output_dir="agents/codex"
force_overwrite=0
dry_run=0
files=()

while [[ $# -gt 0 ]]; do
	case "$1" in
	--reasoning-effort)
		reasoning_effort="${2:-}"
		shift 2
		;;
	--output-dir)
		output_dir="${2:-}"
		shift 2
		;;
	--force)
		force_overwrite=1
		shift
		;;
	--dry-run)
		dry_run=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	*)
		files+=("$1")
		shift
		;;
	esac
done

if [[ ${#files[@]} -eq 0 ]]; then
	echo "Error: at least one .md file is required." >&2
	usage >&2
	exit 1
fi

case "$reasoning_effort" in
low | medium | high) ;;
*)
	echo "Error: --reasoning-effort must be low, medium, or high." >&2
	exit 1
	;;
esac

# --- Main loop ---
success_count=0

for input_file in "${files[@]}"; do

	# Validate input
	if [[ ! -f "$input_file" ]]; then
		echo "Error: File not found: $input_file" >&2
		continue
	fi
	if [[ "$input_file" != *.md ]]; then
		echo "Error: Not a .md file: $input_file" >&2
		continue
	fi

	# Find frontmatter delimiters
	first_delim=0
	second_delim=0
	line_num=0
	while IFS= read -r line; do
		line_num=$((line_num + 1))
		if [[ "$line" == "---" ]]; then
			if [[ $first_delim -eq 0 ]]; then
				first_delim=$line_num
			elif [[ $second_delim -eq 0 ]]; then
				second_delim=$line_num
				break
			fi
		fi
	done <"$input_file"

	if [[ $first_delim -ne 1 || $second_delim -eq 0 ]]; then
		echo "Error: Invalid frontmatter in $input_file" >&2
		continue
	fi

	# Parse frontmatter
	name=""
	description=""
	tools=""
	model=""
	permission_mode=""
	memory=""
	max_turns=""
	background=""

	frontmatter_text=$(sed -n "2,$((second_delim - 1))p" "$input_file")
	while IFS= read -r fm_line; do
		[[ -z "$fm_line" || "$fm_line" == \#* ]] && continue

		key="${fm_line%%:*}"
		value="${fm_line#*: }"
		# Strip surrounding quotes
		value="${value#\"}"
		value="${value%\"}"

		# shellcheck disable=SC2034
		case "$key" in
		name) name="$value" ;;
		description) description="$value" ;;
		tools) tools="$value" ;;
		model) model="$value" ;;
		permissionMode) permission_mode="$value" ;;
		memory) memory="$value" ;;
		maxTurns) max_turns="$value" ;;
		background) background="$value" ;;
		*) echo "Warning: Unknown frontmatter key '$key' in $input_file" >&2 ;;
		esac
	done <<<"$frontmatter_text"

	if [[ -z "$name" ]]; then
		echo "Error: Missing 'name' in frontmatter: $input_file" >&2
		continue
	fi
	if [[ -z "$description" ]]; then
		echo "Error: Missing 'description' in frontmatter: $input_file" >&2
		continue
	fi

	# Extract body (everything after closing ---)
	body=$(tail -n +"$((second_delim + 1))" "$input_file")
	# Strip leading empty lines
	body=$(printf '%s' "$body" | sed '/./,$!d')

	# Build Constraints section
	constraints_section=""
	if [[ "$permission_mode" == "plan" || -n "$tools" ]]; then
		constraints_section="## Constraints"$'\n'$'\n'
		if [[ "$permission_mode" == "plan" ]]; then
			constraints_section+="- You are limited to READ-ONLY operations. Do not modify any files."$'\n'
		fi
		if [[ -n "$tools" ]]; then
			constraints_section+="- Equivalent tool restrictions: ${tools} only."$'\n'
		fi
	fi

	# Inject Constraints before the first ## heading in body
	if [[ -n "$constraints_section" ]]; then
		first_heading_line=$(printf '%s' "$body" | grep -n '^## ' | head -1 | cut -d: -f1)
		if [[ -n "$first_heading_line" ]]; then
			opening=$(printf '%s' "$body" | head -n "$((first_heading_line - 1))")
			rest=$(printf '%s' "$body" | tail -n +"$first_heading_line")
			body="${opening}"$'\n'$'\n'"${constraints_section}"$'\n'"${rest}"
		else
			body="${constraints_section}"$'\n'"${body}"
		fi
	fi

	# Map sandbox_mode
	sandbox_line=""
	if [[ "$permission_mode" == "plan" ]]; then
		sandbox_line='sandbox_mode = "read-only"'
	fi

	# Build header comments
	short_desc=$(printf '%s' "$description" | sed 's/\. .*//' | cut -c1-80)
	source_basename=$(basename "$input_file")

	# Build comment block
	comments="# ${name}: ${short_desc}"$'\n'
	comments+="# Claude/Cursor equivalent: agents/${source_basename}"

	if [[ -n "$memory" ]]; then
		comments+=$'\n'"# Note: Claude/Cursor version has memory=${memory} for cross-session learning."
		comments+=$'\n'"# Codex CLI does not support agent-level memory, so memory instructions are omitted."
	fi
	if [[ -n "$max_turns" ]]; then
		comments+=$'\n'"# Note: Claude/Cursor version has maxTurns=${max_turns}. Codex CLI does not support this."
	fi
	if [[ -n "$background" ]]; then
		comments+=$'\n'"# Note: Claude/Cursor version has background=${background}. Codex CLI does not support this."
	fi

	# Escape triple quotes in body
	safe_body=$(printf '%s' "$body" | sed 's/"""/""\\"/g')

	# Assemble TOML
	assemble_toml() {
		printf '%s\n' "$comments"
		printf '\n'
		printf 'model = "%s"\n' "gpt-5.3-codex"
		printf 'model_reasoning_effort = "%s"\n' "$reasoning_effort"
		if [[ -n "$sandbox_line" ]]; then
			printf '%s\n' "$sandbox_line"
		fi
		printf '\n'
		printf 'developer_instructions = """\n'
		printf '%s\n' "$safe_body"
		printf '"""\n'
	}

	# Output
	if [[ $dry_run -eq 1 ]]; then
		assemble_toml
		if [[ -n "$memory" ]]; then
			echo "" >&2
			echo "[REVIEW NEEDED] Source had memory=${memory}. Memory-related sections may need manual removal from developer_instructions." >&2
		fi
		success_count=$((success_count + 1))
		continue
	fi

	mkdir -p "$output_dir"
	target_file="${output_dir}/${name}.toml"

	if [[ -e "$target_file" && $force_overwrite -ne 1 ]]; then
		if prompt_from_tty answer "File exists: ${target_file}. Overwrite? [y/N]: "; then
			# shellcheck disable=SC2154
			case "$answer" in
			y | Y | yes | YES) ;;
			*)
				echo "Skipped: $target_file"
				continue
				;;
			esac
		else
			echo "Error: target file already exists: ${target_file}" >&2
			echo "Use --force in non-interactive mode." >&2
			continue
		fi
	fi

	assemble_toml >"$target_file"
	echo "Created: $target_file"

	if [[ -n "$memory" ]]; then
		echo "  WARNING: Source had memory=${memory}. Review developer_instructions for memory-related content to remove." >&2
	fi

	success_count=$((success_count + 1))
done

if [[ $success_count -eq 0 ]]; then
	echo "Error: No files were converted." >&2
	exit 1
fi
