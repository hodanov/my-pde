#!/bin/bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# stdin から JSON を読み込む
INPUT=$(cat)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FILE_PATH=$(printf '%s' "$INPUT" | python3 "$SCRIPT_DIR/get_file_path.py")

# .goファイルじゃなければ何もしない
if [[ "$FILE_PATH" != *.go ]]; then
	exit 0
fi

if ! goimports -w "$FILE_PATH" 2>&1; then
	echo "[goimports] fail: $FILE_PATH" >&2
	exit 1
fi
