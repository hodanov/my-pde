#!/usr/bin/env bash
# WezTerm pane dashboard sink.
#
# ai-panes.lua collects the rows, renders the frame and pushes it into this pane
# with pane:inject_output(). This script owns none of that: it holds the pane
# open, marks itself so the Lua side can find it, and relays j / k / l back as a
# user var. Keeping the shell out of the rendering path is what lets one
# dashboard live in every tab without one refresh loop per tab.
set -eu

marker_var="ai_panes_dashboard"
key_var="ai_panes_key"
esc=$'\033'
seq=0
saved_stty=""

set_var() {
	printf '%s]1337;SetUserVar=%s=%s\a' "$esc" "$1" "$(printf '%s' "$2" | base64)"
}

cleanup() {
	set_var "$marker_var" 0
	printf '%s[?25h' "$esc"
	if [ -n "$saved_stty" ]; then
		stty "$saved_stty" </dev/tty 2>/dev/null
	fi
}

# 制御端末が無いとリダイレクト自体が失敗し、その診断はシェルが出すので
# stty の 2>/dev/null では消えない。ブロックごと束ねて捨てる。
{ saved_stty=$(stty -g </dev/tty); } 2>/dev/null || saved_stty=""
if [ -n "$saved_stty" ]; then
	# isig は落とさない。CMD+SHIFT+A の再押下が送る Ctrl-C で閉じられなくなるため。
	stty -echo -icanon min 1 time 0 </dev/tty
fi

trap 'cleanup; exit 0' INT TERM
trap cleanup EXIT

printf '%s[?25l' "$esc"
set_var "$marker_var" 1

while :; do
	key=""
	if ! IFS= read -rsn1 key </dev/tty; then
		sleep 1
		continue
	fi
	case "$key" in
	j | k) ;;
	# read -n1 は改行を行区切りとして食うので、Enter は rc=0 かつ空文字で返る。
	l | "") key=l ;;
	*) continue ;;
	esac
	seq=$((seq + 1))
	set_var "$key_var" "${key}:${seq}"
done
