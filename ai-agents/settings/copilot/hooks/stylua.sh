#!/bin/bash
# stdin から JSON を読み込む
INPUT=$(cat)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FILE_PATH=$(printf '%s' "$INPUT" | python3 "$SCRIPT_DIR/get_file_path.py")

if [[ "$FILE_PATH" != *.lua ]]; then
	exit 0
fi

if ! stylua "$FILE_PATH" 2>&1; then
	echo "[stylua] fail: $FILE_PATH" >&2
	exit 1
fi
