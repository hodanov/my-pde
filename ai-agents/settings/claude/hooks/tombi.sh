#!/bin/bash
# mise-managed tools (non-interactive contexts do not run mise activate)
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:$PATH"
# stdin から JSON を読み込む
INPUT=$(cat)

FILE_PATH=$(echo "$INPUT" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(data.get('tool_input', {}).get('file_path', ''))
")

if [[ "$FILE_PATH" != *.toml ]]; then
	exit 0
fi

if ! tombi format "$FILE_PATH" 2>&1; then
	echo "[tombi] fail: $FILE_PATH" >&2
	exit 1
fi
