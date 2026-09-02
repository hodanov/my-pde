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

interval="${AI_PANES_INTERVAL:-1}"
self_pane="${WEZTERM_PANE:--1}"
# Keep this list in sync with TRACKED in dotfiles/wezterm/ai-panes.lua.
targets="${AI_PANES_AGENTS:-nvim claude codex cursor-agent copilot}"
jump_uri_prefix="wezterm-ai-panes://jump/"
jump_var="ai_panes_jump"
jump_seq=0
saved_stty=""

row_ws=()
row_proc=()
row_project=()
row_pane=()
count=0
idx=0
selected=""
found_idx=-1
here=""
notice=""
rows_serial=""
frame=""
separator=""
cols=0
cols_stale=1

# Catppuccin Mocha, matching appearance.lua and the zsh prompt.
esc=$'\033'
bel=$'\a'
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

# tty -> process name. Matching on the command name alone (rather than on the
# foreground process group) keeps an agent listed while it shells out.
#
# nvim is the exception: .zshrc wraps it in `docker exec`, so the host-side command
# name is `docker`. It is recognised by a bare `nvim` token anywhere in the command
# line rather than by the container name, so that merely entering the container
# (`docker container exec -it nvim-dev bash --login`) is not reported as an editor.
# Same rule as argv_runs_nvim() in dotfiles/wezterm/ai-panes.lua.
procs_by_tty() {
	ps -ao tty=,command= | awk -v names="$targets" '
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

IFS= read -r -d '' collect_filter <<'JQ'
	($procs | split("\n") | map(select(length > 0) | split("\t")
		| { tty: .[0], proc: .[1] })) as $procs
	| ($targets | split(" ")) as $order
	| . as $all
	| ([ $all[] | select((.pane_id | tostring) == $self) | .workspace ] | first // "") as $here
	| [ $all[]
		| select((.pane_id | tostring) != $self)
		| . as $p
		| ($procs | map(select(.tty == $p.tty_name)) | first) as $hit
		| select($hit != null)
		| { ws: $p.workspace,
		    pane: $p.pane_id,
		    proc: $hit.proc,
		    project: (($p.cwd // "") | sub("^file://[^/]*"; "")
		              | sub("/$"; "") | sub(".*/"; "")) } ]
	| sort_by(.ws, (.proc as $n | $order | index($n)), .pane) as $rows
	| if $shape == "tsv"
	  then ($here), ($rows[] | [.ws, .proc, .project, (.pane | tostring)] | @tsv)
	  else ($rows | tojson)
	  end
JQ

collect() {
	local panes
	panes=$(wezterm cli list --format json 2>/dev/null) || return 1
	[ -n "$panes" ] || return 1

	printf '%s' "$panes" | jq -r \
		--arg procs "$(procs_by_tty)" \
		--arg targets "$targets" \
		--arg self "$self_pane" \
		--arg shape "$1" \
		"$collect_filter"
}

collect_tsv() {
	collect tsv
}

collect_json() {
	collect json
}

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

resize_if_stale() {
	local i out=""
	[ "$cols_stale" -eq 1 ] || return
	cols=$(term_cols)
	for ((i = 0; i < cols - 2; i++)); do
		out="${out}─"
	done
	separator=$out
	cols_stale=0
}

refresh_rows() {
	local deps out body ws proc project pane

	row_ws=()
	row_proc=()
	row_project=()
	row_pane=()
	count=0
	rows_serial=""

	deps=$(missing_deps)
	if [ -n "$deps" ]; then
		notice="not on PATH:${deps}"
		return
	fi
	if ! out=$(collect_tsv); then
		notice="wezterm cli unavailable"
		return
	fi
	notice=""

	here=${out%%$'\n'*}
	if [ "$here" = "$out" ]; then
		body=""
	else
		body=${out#*$'\n'}
	fi
	[ -n "$body" ] || return

	while IFS=$'\t' read -r ws proc project pane; do
		row_ws[count]=$ws
		row_proc[count]=$proc
		row_project[count]=$project
		row_pane[count]=$pane
		count=$((count + 1))
	done <<<"$body"
	rows_serial=$body
}

index_of_pane() {
	local i
	for ((i = 0; i < count; i++)); do
		if [ "${row_pane[$i]}" = "$1" ]; then
			found_idx=$i
			return
		fi
	done
	found_idx=-1
}

render_into_frame() {
	local i line ws proc project pane marker cursor color link_open link_close prev_ws=""

	printf -v frame ' %sPANES%s\n' "$mauve" "$reset"
	printf -v line ' %s%s%s\n' "$overlay0" "$separator" "$reset"
	frame+=$line

	if [ -n "$notice" ]; then
		printf -v line ' %s%s%s\n' "$yellow" "$notice" "$reset"
		frame+=$line
	elif [ "$count" -eq 0 ]; then
		printf -v line ' %snothing running%s\n' "$overlay0" "$reset"
		frame+=$line
	else
		for ((i = 0; i < count; i++)); do
			ws=${row_ws[$i]}
			proc=${row_proc[$i]}
			project=${row_project[$i]}
			pane=${row_pane[$i]}
			link_open="${esc}]8;;${jump_uri_prefix}${pane}${bel}"
			link_close="${esc}]8;;${bel}"
			if [ "$ws" != "$prev_ws" ]; then
				printf -v line ' %s▍%s%s\n' "$mauve" "$ws" "$reset"
				frame+=$line
				prev_ws=$ws
			fi
			if [ "$ws" = "$here" ]; then
				marker="${green}●"
			else
				marker="${overlay0}○"
			fi
			if [ "$i" -eq "$idx" ]; then
				cursor="${mauve}▸${reset}"
			else
				cursor=" "
			fi
			case "$proc" in
			nvim) color=$blue ;;
			claude) color=$mauve ;;
			codex) color=$teal ;;
			cursor-agent) color=$green ;;
			*) color=$yellow ;;
			esac
			printf -v line ' %s %s%s%s %s%-13.13s%s%s#%s%s%s\n' \
				"$cursor" "$link_open" "$marker" "$reset" "$color" "$proc" \
				"$reset" "$overlay0" "$pane" "$reset" "$link_close"
			frame+=$line
			# cwd がワークスペース名と食い違うときだけ実際の場所を補足する
			if [ -n "$project" ] && [ "$project" != "$ws" ]; then
				printf -v line '     %s%s↳ %-16.16s%s%s\n' "$link_open" "$overlay0" "$project" "$reset" "$link_close"
				frame+=$line
			fi
		done
	fi

	printf -v line '\n %sj/k move  l/Enter jump%s\n' "$overlay0" "$reset"
	frame+=$line
	printf -v line ' %sclick jump%s\n' "$overlay0" "$reset"
	frame+=$line
}

move_down() {
	if [ "$idx" -lt $((count - 1)) ]; then
		idx=$((idx + 1))
		selected=${row_pane[$idx]}
	fi
}

move_up() {
	if [ "$idx" -gt 0 ]; then
		idx=$((idx - 1))
		selected=${row_pane[$idx]}
	fi
}

resync_selection() {
	if [ "$count" -eq 0 ]; then
		idx=0
		selected=""
		return
	fi
	if [ -n "$selected" ]; then
		index_of_pane "$selected"
		if [ "$found_idx" -ge 0 ]; then
			idx=$found_idx
		fi
	fi
	if [ "$idx" -ge "$count" ]; then
		idx=$((count - 1))
	fi
	selected=${row_pane[$idx]}
}

emit_jump() {
	jump_seq=$((jump_seq + 1))
	printf '\033]1337;SetUserVar=%s=%s\a' "$jump_var" \
		"$(printf '%s:%s' "$1" "$jump_seq" | base64)"
}

cleanup() {
	printf '%s' "${esc}[?25h${esc}[?1049l"
	if [ -n "$saved_stty" ]; then
		stty "$saved_stty" </dev/tty 2>/dev/null
	fi
}

loop() {
	local key last_second=0 stale=1 painted="" frame_key="" prev_frame_key=""

	{ saved_stty=$(stty -g </dev/tty); } 2>/dev/null || saved_stty=""
	if [ -n "$saved_stty" ]; then
		# isig は落とさない。CMD+SHIFT+A の再押下が送る Ctrl-C で閉じられなくなるため。
		stty -echo -icanon min 1 time 0 </dev/tty
	fi

	trap 'cleanup; exit 0' INT TERM
	trap cleanup EXIT
	# alternate screen so the refreshes never touch the pane's scrollback
	printf '%s' "${esc}[?1049h${esc}[?25l"
	# Marker read back by ai-panes.lua so CMD+SHIFT+A can find and close this pane.
	printf '\033]1337;SetUserVar=ai_panes_dashboard=%s\a' "$(printf '1' | base64)"

	while :; do
		# read はキーが来るたび早期に返るので、タイムアウトだけを頼りにすると
		# 操作している間ずっと更新が止まる。秒単位の締め切りを保険に併走させる。
		if [ "$stale" -eq 1 ] || [ $((SECONDS - last_second)) -ge $((interval + 1)) ]; then
			refresh_rows
			stale=0
			cols_stale=1
			last_second=$SECONDS
		fi

		resync_selection
		resize_if_stale

		frame_key="$count|$idx|$notice|$cols|$here|$rows_serial"
		if [ "$frame_key" != "$prev_frame_key" ]; then
			render_into_frame
			prev_frame_key=$frame_key
			if [ "$frame" != "$painted" ]; then
				# [H + [0J redraws with far less flicker than a full [2J clear
				printf '%s%s' "${esc}[H${esc}[0J" "$frame"
				painted=$frame
			fi
		fi

		key=""
		if IFS= read -rsn1 -t "$interval" key </dev/tty; then
			case "$key" in
			j) move_down ;;
			k) move_up ;;
			# read -n1 は改行を行区切りとして食うので、Enter は rc=0 かつ空文字で返る。
			# タイムアウトは rc!=0 なので取り違えない。
			l | "")
				if [ -n "$selected" ]; then
					emit_jump "$selected"
				fi
				;;
			esac
		else
			stale=1
		fi
	done
}

render_once() {
	refresh_rows
	idx=0
	resize_if_stale
	render_into_frame
	printf '%s' "$frame"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	case "${1:-}" in
	--json) collect_json ;;
	--once) render_once ;;
	*) loop ;;
	esac
fi
