local wezterm = require("wezterm")
local act = wezterm.action
local mux = wezterm.mux

-- 検知対象。値はステータスバーとダッシュボードでの表示色（Catppuccin Mocha）。
-- nvim は AI CLI ではないが「どの workspace で動いているかを把握して飛ぶ」対象としては
-- 完全に同型なので、専用モジュールを作らずここに同居させる。
local TRACKED = {
	nvim = "#89b4fa",
	claude = "#cba6f7",
	codex = "#94e2d5",
	["cursor-agent"] = "#a6e3a1",
	copilot = "#f9e2af",
}

-- 並びを固定するための一覧。pairs() の走査順は不定なので、これを使わないと
-- リフレッシュのたびに並びが入れ替わる。nvim は「その workspace の主ペイン」なので先頭。
local TRACKED_ORDER = { "nvim", "claude", "codex", "cursor-agent", "copilot" }

local TRACKED_RANK = {}
for index, name in ipairs(TRACKED_ORDER) do
	TRACKED_RANK[name] = index
end

local MAUVE = "#cba6f7"
local GREEN = "#a6e3a1"
local RED = "#f38ba8"
local OVERLAY0 = "#6c7086"

local DASHBOARD_VAR = "ai_panes_dashboard"
local DASHBOARD_CMD = wezterm.config_dir .. "/bin/ai-panes.sh"
local DASHBOARD_FRACTION = 0.18

local KEY_VAR = "ai_panes_key"
local KEY_VAR_PATTERN = "^(%a):"

local JUMP_URI_PREFIX = "wezterm-ai-panes://jump/"
local JUMP_URI_PATTERN = "^wezterm%-ai%-panes://jump/(%d+)$"

local COLLECT_THROTTLE_SECONDS = 2

local ESC = "\027"
local RESET = ESC .. "[0m"

local state = { rows = {}, collected_at = 0, tab_id = {}, dashboards = {}, painted = {} }

local function basename(path)
	if path == nil or path == "" then
		return nil
	end
	return path:gsub("/+$", ""):match("([^/]+)$")
end

local function fg(hex)
	local r, g, b = hex:match("^#(%x%x)(%x%x)(%x%x)$")
	return string.format("%s[38;2;%d;%d;%dm", ESC, tonumber(r, 16), tonumber(g, 16), tonumber(b, 16))
end

-- .zshrc の nvim() は docker exec でコンテナ内の nvim を起動するので、ホスト側の
-- foreground プロセスは docker になり名前一致では絶対に拾えない。判定はコンテナ名
-- （nvim-dev）ではなく argv に素の nvim トークンが現れるかで行う。
--   `bash --login -c 'nvim "$@"'` → argv の 1 要素が `nvim "$@"` になるので拾える
--   `docker container exec -it nvim-dev bash --login` → コンテナに入っただけなので拾わない
local function argv_runs_nvim(argv)
	if argv == nil then
		return false
	end
	for _, token in ipairs(argv) do
		if token == "nvim" or token:match("^nvim%s") then
			return true
		end
	end
	return false
end

-- 実行ファイルの実パスだけでは判定できない。claude は ~/.local/bin/claude が
-- ~/.local/share/claude/versions/<version> への symlink で、get_foreground_process_name()
-- は解決後の実パスを返すため basename がバージョン番号（例 2.1.235）になる。
-- LocalProcessInfo の name / argv[0] も候補に入れて、どれか 1 つでも一致すれば採用する。
local function tracked_name(candidate)
	local name = basename(candidate)
	if name and TRACKED[name] then
		return name
	end
	return nil
end

local function process_of(pane)
	local info = nil

	local ok_info, got = pcall(function()
		return pane:get_foreground_process_info()
	end)
	if ok_info and got then
		info = got
		local hit = tracked_name(info.name)
			or (info.argv and tracked_name(info.argv[1]))
			or tracked_name(info.executable)
		if hit then
			return hit
		end
	end

	local ok_name, proc = pcall(function()
		return pane:get_foreground_process_name()
	end)
	if ok_name then
		local hit = tracked_name(proc)
		if hit then
			return hit
		end
	end

	-- 名前一致が空振りしたときだけ docker ラッパーを疑う。誤検知の面を狭めるため、
	-- foreground が docker のときに限って argv を舐める。
	if info then
		local front = basename(info.name) or basename(info.argv and info.argv[1])
		if front == "docker" and argv_runs_nvim(info.argv) then
			return "nvim"
		end
	end
	return nil
end

local function is_dashboard(pane)
	if state.dashboards[pane:pane_id()] then
		return true
	end
	local ok, vars = pcall(function()
		return pane:get_user_vars()
	end)
	if ok and vars and vars[DASHBOARD_VAR] == "1" then
		state.dashboards[pane:pane_id()] = true
		return true
	end
	return false
end

local function project_of(pane)
	local ok, cwd = pcall(function()
		return pane:get_current_working_dir()
	end)
	if not ok or cwd == nil then
		return nil
	end
	if type(cwd) == "string" then
		return basename((cwd:gsub("^file://[^/]*", "")))
	end
	local ok_path, path = pcall(function()
		return cwd.file_path or cwd.path
	end)
	if not ok_path then
		return nil
	end
	return basename(path)
end

local function progress_token(progress)
	if progress == nil or progress == "None" then
		return nil
	end
	if progress == "Indeterminate" then
		return "busy"
	end
	if type(progress) ~= "table" then
		return nil
	end
	if progress.Percentage ~= nil then
		return string.format("%d%%", progress.Percentage)
	end
	if progress.Error ~= nil then
		return "err"
	end
	return nil
end

local function progress_of(pane)
	local ok, progress = pcall(function()
		return pane:get_progress()
	end)
	if not ok then
		return nil
	end
	return progress_token(progress)
end

local function collect()
	local rows = {}
	for _, win in ipairs(mux.all_windows()) do
		local workspace = win:get_workspace()
		for _, tab in ipairs(win:tabs()) do
			for _, pane in ipairs(tab:panes()) do
				if not is_dashboard(pane) then
					local proc = process_of(pane)
					if proc then
						rows[#rows + 1] = {
							ws = workspace,
							proc = proc,
							pane_id = pane:pane_id(),
							project = project_of(pane),
							progress = progress_of(pane),
						}
					end
				end
			end
		end
	end

	table.sort(rows, function(a, b)
		if a.ws ~= b.ws then
			return a.ws < b.ws
		end
		if a.proc ~= b.proc then
			return TRACKED_RANK[a.proc] < TRACKED_RANK[b.proc]
		end
		return a.pane_id < b.pane_id
	end)
	return rows
end

local function status_text(rows)
	local counts = {}
	for _, row in ipairs(rows) do
		counts[row.proc] = (counts[row.proc] or 0) + 1
	end

	local parts = {}
	for _, name in ipairs(TRACKED_ORDER) do
		if counts[name] then
			parts[#parts + 1] = { Foreground = { Color = TRACKED[name] } }
			parts[#parts + 1] = { Text = string.format(" %s:%d", name, counts[name]) }
		end
	end
	if #parts == 0 then
		return ""
	end
	parts[#parts + 1] = { Text = " " }
	return wezterm.format(parts)
end

local function index_of(rows, pane_id)
	for index, row in ipairs(rows) do
		if row.pane_id == pane_id then
			return index
		end
	end
	return nil
end

local function resolve_selection(rows, selected)
	if #rows == 0 then
		return nil
	end
	if selected and index_of(rows, selected) then
		return selected
	end
	return rows[1].pane_id
end

local function move_selection(rows, selected, delta)
	if #rows == 0 then
		return nil
	end
	local index = index_of(rows, selected) or 1
	index = math.max(1, math.min(#rows, index + delta))
	return rows[index].pane_id
end

local function pad(text, preferred, available)
	local width = math.max(1, math.min(preferred, available))
	return string.format("%-" .. width .. "." .. width .. "s", text)
end

local function clip(text, available)
	if available < 1 then
		return ""
	end
	return text:sub(1, available)
end

local function render(rows, ctx)
	local lines = {
		" " .. fg(MAUVE) .. clip("PANES", ctx.cols - 1) .. RESET,
		" " .. fg(OVERLAY0) .. string.rep("─", math.max(0, ctx.cols - 2)) .. RESET,
	}

	if #rows == 0 then
		lines[#lines + 1] = " " .. fg(OVERLAY0) .. clip("nothing running", ctx.cols - 1) .. RESET
	end

	local group = nil
	for _, row in ipairs(rows) do
		if row.ws ~= group then
			lines[#lines + 1] = " " .. fg(MAUVE) .. "▍" .. clip(row.ws, ctx.cols - 2) .. RESET
			group = row.ws
		end

		local link_open = ESC .. "]8;;" .. JUMP_URI_PREFIX .. row.pane_id .. "\007"
		local link_close = ESC .. "]8;;\007"
		local marker = row.ws == ctx.here and (fg(GREEN) .. "●") or (fg(OVERLAY0) .. "○")
		local cursor = row.pane_id == ctx.selected and (fg(MAUVE) .. "▸" .. RESET) or " "

		local id_text = "#" .. row.pane_id
		local room = ctx.cols - 5 - #id_text
		local token = row.progress
		if token and room - (#token + 1) < 1 then
			token = nil
		end
		local progress_cell = token and (" " .. fg(token == "err" and RED or GREEN) .. token .. RESET) or ""

		lines[#lines + 1] = string.format(
			" %s %s%s%s %s%s%s%s%s%s%s%s",
			cursor,
			link_open,
			marker,
			RESET,
			fg(TRACKED[row.proc]),
			pad(row.proc, 13, room - (token and #token + 1 or 0)),
			RESET,
			fg(OVERLAY0),
			id_text,
			RESET,
			progress_cell,
			link_close
		)

		if row.project and row.project ~= row.ws then
			lines[#lines + 1] = string.format(
				"     %s%s↳ %s%s%s",
				link_open,
				fg(OVERLAY0),
				pad(row.project, 16, ctx.cols - 7),
				RESET,
				link_close
			)
		end
	end

	lines[#lines + 1] = ""
	for _, hint in ipairs({ "j/k move", "l/Enter jump", "click jump", "CMD+SHIFT+A close" }) do
		lines[#lines + 1] = " " .. fg(OVERLAY0) .. clip(hint, ctx.cols - 1) .. RESET
	end

	return ESC .. "[?25l" .. ESC .. "[H" .. ESC .. "[0J" .. table.concat(lines, "\r\n") .. "\r\n"
end

local function find_pane(pane_id)
	if pane_id == nil then
		return nil
	end
	local ok, pane = pcall(mux.get_pane, pane_id)
	if not ok or not pane then
		return nil
	end
	local win = pane:window()
	if not win then
		return nil
	end
	return { workspace = win:get_workspace(), pane_id = pane_id, pane = pane }
end

-- 先にペインを activate してから workspace を切り替える。
-- MuxPane:activate() は自身をタブ内で、そのタブを mux window 内で active にするので、
-- SwitchToWorkspace が完了した時点で目的のペインが前面に出る。
local function jump_to(window, row)
	local ok = pcall(function()
		row.pane:activate()
	end)
	if not ok then
		window:toast_notification("WezTerm", "Pane #" .. row.pane_id .. " is gone", nil, 2000)
		return
	end
	if window:active_workspace() ~= row.workspace then
		window:perform_action(act.SwitchToWorkspace({ name = row.workspace }), window:active_pane())
	end
end

local function jump_by_id(window, pane_id)
	if pane_id == nil then
		return
	end
	local row = find_pane(pane_id)
	if row then
		jump_to(window, row)
	else
		window:toast_notification("WezTerm", "Pane #" .. pane_id .. " is gone", nil, 2000)
	end
end

local function find_dashboard(tab)
	for _, pane in ipairs(tab:panes()) do
		if is_dashboard(pane) then
			return pane
		end
	end
	return nil
end

local function ensure_dashboard(tab, focus)
	local existing = find_dashboard(tab)
	if existing then
		if focus then
			pcall(function()
				existing:activate()
			end)
		end
		return existing
	end

	local before = tab:active_pane()
	local ok, dash = pcall(function()
		return before:split({
			direction = "Left",
			size = DASHBOARD_FRACTION,
			top_level = true,
			args = { DASHBOARD_CMD },
		})
	end)
	if not ok or not dash then
		return nil
	end

	state.dashboards[dash:pane_id()] = true
	if not focus then
		pcall(function()
			before:activate()
		end)
	end
	return dash
end

local function paint(window, tab)
	local dash = find_dashboard(tab)
	if not dash then
		return
	end

	local selected = resolve_selection(state.rows, wezterm.GLOBAL.ai_panes_selected)
	wezterm.GLOBAL.ai_panes_selected = selected

	local ok, dims = pcall(function()
		return dash:get_dimensions()
	end)
	local frame = render(state.rows, {
		cols = ok and dims.cols or 26,
		here = window:active_workspace(),
		selected = selected,
	})

	local id = dash:pane_id()
	if state.painted[id] == frame then
		return
	end
	local painted = pcall(function()
		dash:inject_output(frame)
	end)
	if painted then
		state.painted[id] = frame
	end
end

local function refresh(force)
	local now = os.time()
	if force or (now - state.collected_at) >= COLLECT_THROTTLE_SECONDS then
		state.rows = collect()
		state.collected_at = now
	end
end

local function close_all()
	for _, win in ipairs(mux.all_windows()) do
		for _, tab in ipairs(win:tabs()) do
			local dash = find_dashboard(tab)
			if dash then
				-- CloseCurrentPane は perform_action に渡したペインではなく「いまアクティブな
				-- ペイン」を閉じてしまうので使えない。sink はペイン直下のプロセスなので、
				-- Ctrl-C を送れば INT トラップが exit 0 し、WezTerm がそのペインを閉じる。
				pcall(function()
					dash:send_text("\003")
				end)
			end
		end
	end
end

local function toggle_dashboard()
	return wezterm.action_callback(function(window, _pane)
		if wezterm.GLOBAL.ai_panes_on then
			wezterm.GLOBAL.ai_panes_on = false
			close_all()
			state.dashboards = {}
			state.painted = {}
			return
		end

		wezterm.GLOBAL.ai_panes_on = true
		local tab = window:active_tab()
		ensure_dashboard(tab, true)
		refresh(true)
		paint(window, tab)
	end)
end

local M = {
	collect = collect,
	status_text = status_text,
	render = render,
	move_selection = move_selection,
	resolve_selection = resolve_selection,
	process_of = process_of,
	progress_token = progress_token,
}

return setmetatable(M, {
	__call = function(_, config)
		wezterm.on("update-status", function(window)
			local tab = window:active_tab()
			local seen = window:window_id()
			local switched = state.tab_id[seen] ~= tab:tab_id()
			state.tab_id[seen] = tab:tab_id()

			refresh(switched)
			window:set_left_status(status_text(state.rows))

			if wezterm.GLOBAL.ai_panes_on then
				ensure_dashboard(tab, false)
				paint(window, tab)
			end
		end)

		wezterm.on("open-uri", function(window, _, uri)
			local id = uri:match(JUMP_URI_PATTERN)
			if not id then
				return
			end
			jump_by_id(window, tonumber(id))
			return false
		end)

		wezterm.on("user-var-changed", function(window, pane, name, value)
			if name == DASHBOARD_VAR then
				if value == "1" then
					state.dashboards[pane:pane_id()] = true
				end
				return
			end
			if name ~= KEY_VAR then
				return
			end

			local key = value:match(KEY_VAR_PATTERN)
			if key == "l" then
				jump_by_id(window, wezterm.GLOBAL.ai_panes_selected)
			elseif key == "j" or key == "k" then
				refresh(false)
				wezterm.GLOBAL.ai_panes_selected =
					move_selection(state.rows, wezterm.GLOBAL.ai_panes_selected, key == "j" and 1 or -1)
				paint(window, window:active_tab())
			end
		end)

		table.insert(config.keys, { key = "A", mods = "CMD|SHIFT", action = toggle_dashboard() })
	end,
})
