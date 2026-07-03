#!/bin/bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# stdin から JSON を読み込む
INPUT=$(cat)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FILE_PATH=$(printf '%s' "$INPUT" | python3 "$SCRIPT_DIR/get_file_path.py")

case "$FILE_PATH" in
*.html | *.css | *.js | *.mjs | *.ts | *.json | *.yaml | *.yml)
	if ! prettier --write "$FILE_PATH" 2>&1; then
		echo "[prettier] fail: $FILE_PATH" >&2
		exit 1
	fi
	;;
esac
