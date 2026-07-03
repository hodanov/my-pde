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

# .mdファイルじゃなければ何もしない
if [[ "$FILE_PATH" != *.md ]]; then
	exit 0
fi

rc=0

# markdownlint-cli2
if ! markdownlint-cli2 --config "$HOME/.claude/hooks/.markdownlint-cli2.yaml" --fix "$FILE_PATH" 2>&1; then
	echo "[markdownlint] fail: $FILE_PATH" >&2
	rc=1
fi

# prettier
if ! prettier --write "$FILE_PATH" 2>&1; then
	echo "[prettier:md] fail: $FILE_PATH" >&2
	rc=1
fi

exit "$rc"
