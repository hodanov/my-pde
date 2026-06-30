#!/bin/bash
# stdin から JSON を読み込む
INPUT=$(cat)

FILE_PATH=$(echo "$INPUT" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(data.get('tool_input', {}).get('file_path', ''))
")

case "$FILE_PATH" in
*.html | *.css | *.js | *.mjs | *.ts | *.json | *.yaml | *.yml)
	if ! prettier --write "$FILE_PATH" 2>&1; then
		echo "[prettier] fail: $FILE_PATH" >&2
		exit 1
	fi
	;;
esac
