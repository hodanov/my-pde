#!/bin/bash
set -u

INPUT=$(cat)

TARGET=$(printf '%s' "$INPUT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
except Exception:
    print('')
    sys.exit(0)
for holder in (data, data.get('tool_input') or {}, data.get('params') or {}, data.get('event') or {}):
    if not isinstance(holder, dict):
        continue
    for key in ('file_path', 'path', 'command'):
        value = holder.get(key)
        if isinstance(value, str) and value:
            print(value)
            sys.exit(0)
print('')
")

[ -n "$TARGET" ] || exit 0

case "$TARGET" in
*.env.example* | *.env.sample* | *.env.template* | *credentials.example* | *credentials.sample* | *process.env* | *vim.env* | *os.environ*)
	exit 0
	;;
esac

case "$TARGET" in
*.env* | *id_rsa* | *id_ed25519* | *.pem* | *credentials* | *.netrc* | *.npmrc*)
	python3 -c "
import json, sys
target = sys.argv[1]
print(json.dumps({
    'permission': 'deny',
    'userMessage': '[guard-secret-read] blocked: ' + target,
    'agentMessage': '秘密情報を含みうるファイルの読み取りをブロックしました。中身が必要な場合はユーザーに確認してください。',
}, ensure_ascii=False))
" "$TARGET"
	exit 0
	;;
esac

exit 0
