#!/bin/bash
# stdin から JSON を読み込む
INPUT=$(cat)

# stop イベントの status で通知本文を切り替える
STATUS=$(echo "$INPUT" | jq -r '.status // ""')
case "$STATUS" in
aborted)
	MESSAGE="処理が中断されたよ"
	;;
error)
	MESSAGE="エラーで停止したよ"
	;;
*)
	MESSAGE="推論が完了したよ"
	;;
esac

# AppleScript の文字列を壊さないよう二重引用符を除去して通知
osascript -e "display notification \"${MESSAGE//\"/}\" with title \"Cursor\"" 2>/dev/null || true
