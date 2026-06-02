#!/bin/bash
# stdin から JSON を読み込む
INPUT=$(cat)

# フックイベント名で通知本文を切り替える
EVENT=$(echo "$INPUT" | jq -r '.hook_event_name // ""')
case "$EVENT" in
Notification)
	# 許可待ち・入力待ちなどは payload の message をそのまま出す
	MESSAGE=$(echo "$INPUT" | jq -r '.message // "アクションが必要です"')
	;;
*)
	# Stop（推論完了）など、それ以外は固定文言
	MESSAGE="推論が完了しました"
	;;
esac

# AppleScript の文字列を壊さないよう二重引用符を除去して通知
osascript -e "display notification \"${MESSAGE//\"/}\" with title \"Claude Code\"" 2>/dev/null || true
