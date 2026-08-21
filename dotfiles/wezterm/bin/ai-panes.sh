#!/usr/bin/env bash
# ai-panes.lua はこのスクリプトをペインのプログラムとして直接起動する。ログインシェルを
# 経由しないため PATH は WezTerm GUI プロセスのもの（/usr/bin:/bin:...）になり、
# Homebrew の jq / wezterm も mise の shim も入っていない。明示的に足す。
export PATH="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims:/opt/homebrew/bin:/usr/local/bin:$PATH"
# WezTerm 本体は Lua から起動されたときも WEZTERM_EXECUTABLE で所在が分かる
if [ -n "${WEZTERM_EXECUTABLE:-}" ]; then
	export PATH="${WEZTERM_EXECUTABLE%/*}:$PATH"
fi
# WezTerm pane dashboard: list the AI CLIs and the nvim instances running across
# every workspace and pane.
# Rendered inside the thin left pane that ai-panes.lua spawns (CMD+SHIFT+A).
#
# `wezterm cli list` knows every pane's workspace/cwd/tty but not its foreground
# process, so the process is resolved by joining that list with `ps` on the tty.
#
#   ai-panes.sh          run the refresh loop (default)
#   ai-panes.sh --once   render a single frame and exit
#   ai-panes.sh --json   dump the collected rows as JSON
set -uo pipefail

interval="${AI_PANES_INTERVAL:-2}"
self_pane="${WEZTERM_PANE:--1}"
# Keep this list in sync with TRACKED in dotfiles/wezterm/ai-panes.lua.
targets="${AI_PANES_AGENTS:-nvim claude codex cursor-agent copilot}"

# Catppuccin Mocha, matching appearance.lua and the zsh prompt.
esc=$'\033'
reset="${esc}[0m"
mauve="${esc}[38;2;203;166;247m"
blue="${esc}[38;2;137;180;250m"
teal="${esc}[38;2;148;226;213m"
green="${esc}[38;2;166;227;161m"
yellow="${esc}[38;2;249;226;175m"
overlay0="${esc}[38;2;108;112;134m"

# 依存が欠けたときに無言の空ペインにならないよう、不足分を名前で返す。
missing_deps() {
	local cmd out=""
	for cmd in wezterm jq ps awk; do
		command -v "$cmd" >/dev/null 2>&1 || out="${out} ${cmd}"
	done
	printf '%s' "$out"
}

proc_color() {
	case "$1" in
	nvim) printf '%s' "$blue" ;;
	claude) printf '%s' "$mauve" ;;
	codex) printf '%s' "$teal" ;;
	cursor-agent) printf '%s' "$green" ;;
	*) printf '%s' "$yellow" ;;
	esac
}

# tty -> process name. Matching on the command name alone (rather than on the
# foreground process group) keeps an agent listed while it shells out.
#
# nvim is the exception: .zshrc wraps it in `docker exec`, so the host-side command
# name is `docker`. It is recognised by a bare `nvim` token anywhere in the command
# line rather than by the container name, so that merely entering the container
# (`docker container exec -it nvim-dev bash --login`) is not reported as an editor.
# Same rule as argv_runs_nvim() in dotfiles/wezterm/ai-panes.lua.
procs_by_tty() {
	ps -Ao tty=,command= | awk -v names="$targets" '
		BEGIN {
			n = split(names, list, " ")
			for (i = 1; i <= n; i++) {
				want[list[i]] = 1
			}
		}
		# ttyless processes carry huge argv (Docker Desktop et al) and can never
		# match a pane, so drop them before looking at the command line.
		$1 == "??" { next }
		{
			m = split($2, seg, "/")
			cmd = seg[m]
			if (cmd in want) {
				print "/dev/" $1 "\t" cmd
				next
			}
			if (cmd == "docker" && $0 ~ /(^| )nvim( |$)/) {
				print "/dev/" $1 "\tnvim"
			}
		}
	'
}

# Rows of { ws, pane, proc, project }, sorted by workspace then by the order of
# $targets, excluding this pane.
collect() {
	local panes
	panes=$(wezterm cli list --format json 2>/dev/null) || return 1
	[ -n "$panes" ] || return 1

	printf '%s' "$panes" | jq -c \
		--arg procs "$(procs_by_tty)" \
		--arg targets "$targets" \
		--arg self "$self_pane" '
		($procs | split("\n") | map(select(length > 0) | split("\t")
			| { tty: .[0], proc: .[1] })) as $procs
		| ($targets | split(" ")) as $order
		| [ .[]
			| select((.pane_id | tostring) != $self)
			| . as $p
			| ($procs | map(select(.tty == $p.tty_name)) | first) as $hit
			| select($hit != null)
			| { ws: $p.workspace,
			    pane: $p.pane_id,
			    proc: $hit.proc,
			    project: (($p.cwd // "") | sub("^file://[^/]*"; "")
			              | sub("/$"; "") | sub(".*/"; "")) } ]
		| sort_by(.ws, (.proc as $n | $order | index($n)), .pane)'
}

workspace_of_self() {
	wezterm cli list --format json 2>/dev/null |
		jq -r --arg p "$self_pane" '.[] | select((.pane_id | tostring) == $p) | .workspace'
}

# render() の出力は $(...) で受けるため stdout はパイプになる。tput は stdout を
# 見て幅を決めるので、制御端末から直接引く。
term_cols() {
	local size=""
	# 制御端末が無いとリダイレクト自体が失敗し、その診断はシェルが出すので
	# stty の 2>/dev/null では消えない。ブロックごと束ねて捨てる。
	{ size=$(stty size </dev/tty); } 2>/dev/null
	if [ -n "$size" ]; then
		printf '%s' "${size#* }"
		return
	fi
	printf '%s' "${COLUMNS:-26}"
}

rule() {
	local i out=""
	for ((i = 0; i < $1; i++)); do
		out="${out}─"
	done
	printf '%s' "$out"
}

render() {
	local cols json count prev_ws marker missing
	cols=$(term_cols)

	printf ' %sPANES%s\n' "$mauve" "$reset"
	printf ' %s%s%s\n' "$overlay0" "$(rule $((cols - 2)))" "$reset"

	missing=$(missing_deps)
	if [ -n "$missing" ]; then
		printf ' %snot on PATH:%s%s\n' "$yellow" "$missing" "$reset"
		return 0
	fi

	if ! json=$(collect); then
		printf ' %swezterm cli unavailable%s\n' "$yellow" "$reset"
		return 0
	fi
	count=$(printf '%s' "$json" | jq 'length')

	if [ "$count" -eq 0 ]; then
		printf ' %snothing running%s\n' "$overlay0" "$reset"
	else
		prev_ws=""
		printf '%s' "$json" |
			jq -r '.[] | [.ws, .proc, .project, (.pane | tostring)] | @tsv' |
			while IFS=$'\t' read -r ws proc project pane; do
				if [ "$ws" != "$prev_ws" ]; then
					printf ' %s▍%s%s\n' "$mauve" "$ws" "$reset"
					prev_ws="$ws"
				fi
				if [ "$ws" = "$here" ]; then
					marker="${green}●"
				else
					marker="${overlay0}○"
				fi
				printf '   %s%s %s%-13.13s%s%s#%s%s\n' \
					"$marker" "$reset" "$(proc_color "$proc")" "$proc" \
					"$reset" "$overlay0" "$pane" "$reset"
				# cwd がワークスペース名と食い違うときだけ実際の場所を補足する
				if [ -n "$project" ] && [ "$project" != "$ws" ]; then
					printf '     %s↳ %-16.16s%s\n' "$overlay0" "$project" "$reset"
				fi
			done
	fi

	printf '\n %sCMD+a  jump%s\n' "$overlay0" "$reset"
}

cleanup() {
	printf '%s' "${esc}[?25h${esc}[?1049l"
}

loop() {
	trap 'cleanup; exit 0' INT TERM
	trap cleanup EXIT
	# alternate screen so the refreshes never touch the pane's scrollback
	printf '%s' "${esc}[?1049h${esc}[?25l"
	# Marker read back by ai-panes.lua so CMD+SHIFT+A can find and close this pane.
	printf '\033]1337;SetUserVar=ai_panes_dashboard=%s\a' "$(printf '1' | base64)"
	local frame
	while :; do
		frame=$(render)
		# [H + [0J redraws with far less flicker than a full [2J clear
		printf '%s%s\n' "${esc}[H${esc}[0J" "$frame"
		sleep "$interval"
	done
}

here=$(workspace_of_self)

case "${1:-}" in
--json) collect ;;
--once) render ;;
*) loop ;;
esac
