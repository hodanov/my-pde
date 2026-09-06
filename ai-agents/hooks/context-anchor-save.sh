#!/usr/bin/env bash
# PreCompact hook: transcript からユーザー発話の原文（先頭 1 件 + 直近 5 件）を
# 抽出し、セッション別の退避ファイルへ保存する。コンパクションで会話履歴が要約に
# 置き換わると原文の指示・制約が言い換えられて落ちるため、再開時に
# context-anchor-restore.sh (SessionStart, matcher=compact) が読み戻す。
# PreCompact は additionalContext を返せないので保存と注入を 2 本に分ける。
# exit 2 はコンパクション自体をブロックしてしまうため、どの失敗パスでも exit 0。
set -u

command -v jq >/dev/null 2>&1 || exit 0

INPUT=$(cat)

TRANSCRIPT=$(printf '%s' "$INPUT" | jq -r '.transcript_path // ""' 2>/dev/null)
SESSION_ID=$(printf '%s' "$INPUT" | jq -r '.session_id // ""' 2>/dev/null)
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // ""' 2>/dev/null)
TRIGGER=$(printf '%s' "$INPUT" | jq -r '.trigger // "unknown"' 2>/dev/null)

[ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ] || exit 0

# tool_result / スラッシュコマンドの展開・bash 出力は指示ではないので落とす。
messages=$(
	jq -s -c '
		[ .[]
			| select((.type // "") == "user")
			| select((.isMeta // false) == false)
			| .message.content
			| if type == "string" then .
				elif type == "array" then ([ .[] | select((.type // "") == "text") | .text // "" ] | join("\n"))
				else "" end
			| select(type == "string")
			| select(test("^\\s*<(command-name|command-message|command-args|local-command-stdout|local-command-stderr|bash-input|bash-stdout|bash-stderr|system-reminder)") | not)
			| select(test("\\S"))
		]' "$TRANSCRIPT" 2>/dev/null
) || exit 0
[ -n "$messages" ] || exit 0

render() {
	printf '%s' "$messages" | jq -r --argjson from "$1" --argjson count "$2" '
		(if $from == 0 then .[0:1] else (.[1:] | .[-$count:]) end)
		| map(.[0:1200])
		| select(length > 0)
		| map("- " + (gsub("\n"; "\n  ")))
		| join("\n")
		| .[0:2000]' 2>/dev/null
}

first=$(render 0 1)
recent=$(render 1 5)
{ [ -n "$first" ] || [ -n "$recent" ]; } || exit 0

state_dir="${TMPDIR:-/tmp}/claude-context-anchor"
umask 077
mkdir -p "$state_dir" 2>/dev/null || exit 0
chmod 700 "$state_dir" 2>/dev/null || true

# session_id が再開側で変わる実装だった場合の空振りを避けるため、cwd を鍵にした
# 控えも同時に書き、restore 側で二段構えに引く。
keys=""
[ -n "$SESSION_ID" ] && keys="$SESSION_ID"
if [ -n "$CWD" ]; then
	keys="$keys cwd-$(printf '%s' "$CWD" | cksum | cut -d' ' -f1)"
fi
[ -n "$keys" ] || keys="default"

for key in $keys; do
	# 先頭の指示は最初のコンパクション時だけ書き、以降は上書きしない
	# （複数回コンパクションが走ると transcript から消えうるため）。
	if [ -n "$first" ] && [ ! -s "$state_dir/$key.first.md" ]; then
		printf '%s\n' "$first" >"$state_dir/$key.first.md" 2>/dev/null || continue
		chmod 600 "$state_dir/$key.first.md" 2>/dev/null || true
	fi
	if [ -n "$recent" ]; then
		{
			printf '%s\n' "$recent"
			printf '\n(saved at %s, trigger=%s)\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$TRIGGER"
		} >"$state_dir/$key.recent.md" 2>/dev/null || continue
		chmod 600 "$state_dir/$key.recent.md" 2>/dev/null || true
	fi
done

exit 0
