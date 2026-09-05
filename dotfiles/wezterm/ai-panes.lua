local wezterm = require("wezterm")
local act = wezterm.action
local mux = wezterm.mux

-- 検知対象。bin/ai-panes.sh の AI_PANES_AGENTS と対応させること。
-- 値はステータスバーでの表示色（Catppuccin Mocha）。
-- nvim は AI CLI ではないが「どの workspace で動いているかを把握して飛ぶ」対象としては
-- 完全に同型なので、専用モジュールを作らずここに同居させる。
local TRACKED = {
	nvim = "#89b4fa",
	claude = "#cba6f7",
	codex = "#94e2d5",
	["cursor-agent"] = "#a6e3a1",
	copilot = "#f9e2af",
}

-- ステータスバーの並びを固定するための一覧。pairs() の走査順は不定なので、
-- これを使わないとリフレッシュのたびに並びが入れ替わる。
-- nvim は「その workspace の主ペイン」なので先頭に置く。
local TRACKED_ORDER = { "nvim", "claude", "codex", "cursor-agent", "copilot" }

-- ダッシュボードペインが起動直後に立てる user var。
-- pane_id を wezterm.GLOBAL に持つ方式と違い、ペインが消えれば目印も消えるので
-- 手動で閉じられても設定をリロードしても状態がずれない。
local DASHBOARD_VAR = "ai_panes_dashboard"
local DASHBOARD_CMD = wezterm.config_dir .. "/bin/ai-panes.sh"
local DASHBOARD_PERCENT = 18

local JUMP_URI_PATTERN = "^wezterm%-ai%-panes://jump/(%d+)$"

-- ダッシュボードが l キーで送ってくる user var。値は `<pane_id>:<連番>` で、
-- 同じペインへ連続でジャンプしても値が変わり user-var-changed が確実に発火する。
local JUMP_VAR = "ai_panes_jump"
local JUMP_VAR_PATTERN = "^(%d+):"

-- update-status は毎秒発火するので、全ペイン走査はこの間隔まで間引く。
local STATUS_THROTTLE_SECONDS = 3

local PROGRESS_PATH = os.getenv("AI_PANES_PROGRESS_FILE")
	or (os.getenv("HOME") or "") .. "/.local/state/wezterm-ai-panes/progress"
local PROGRESS_THROTTLE_SECONDS = 1

local function basename(path)
	if path == nil or path == "" then
		return nil
	end
	return path:gsub("/+$", ""):match("([^/]+)$")
end

-- .zshrc の nvim() は docker exec でコンテナ内の nvim を起動するので、ホスト側の
-- foreground プロセスは docker になり名前一致では絶対に拾えない。判定はコンテナ名
-- （nvim-dev）ではなく argv に素の nvim トークンが現れるかで行う。
--   `bash --login -c 'nvim "$@"'` → argv の 1 要素が `nvim "$@"` になるので拾える
--   `docker container exec -it nvim-dev bash --login` → コンテナに入っただけなので拾わない
-- この判定ルールは bin/ai-panes.sh の awk 側と対になっている。
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

local function count_tracked()
	local counts = {}
	for _, win in ipairs(mux.all_windows()) do
		for _, tab in ipairs(win:tabs()) do
			for _, pane in ipairs(tab:panes()) do
				local proc = process_of(pane)
				if proc then
					counts[proc] = (counts[proc] or 0) + 1
				end
			end
		end
	end
	return counts
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

local function collect_progress()
	local lines = {}
	for _, win in ipairs(mux.all_windows()) do
		for _, tab in ipairs(win:tabs()) do
			for _, pane in ipairs(tab:panes()) do
				local ok, progress = pcall(function()
					return pane:get_progress()
				end)
				local token = ok and progress_token(progress) or nil
				if token then
					table.insert(lines, string.format("%d\t%s", pane:pane_id(), token))
				end
			end
		end
	end
	table.sort(lines)
	return table.concat(lines, "\n")
end

local function open_progress_tmp()
	local tmp = PROGRESS_PATH .. ".tmp"
	local handle = io.open(tmp, "w")
	if handle then
		return handle, tmp
	end
	wezterm.run_child_process({ "mkdir", "-p", PROGRESS_PATH:match("^(.*)/[^/]*$") or "." })
	return io.open(tmp, "w"), tmp
end

-- ダッシュボードは wezterm cli list --format json から行を組むが、progress は
-- そこに含まれない。Lua からファイル経由で渡す。
local function write_progress()
	local now = os.time()
	local wrote_at = wezterm.GLOBAL.ai_panes_progress_at
	if wrote_at and (now - wrote_at) < PROGRESS_THROTTLE_SECONDS then
		return
	end
	wezterm.GLOBAL.ai_panes_progress_at = now

	local body = collect_progress()
	if body == wezterm.GLOBAL.ai_panes_progress_body then
		return
	end

	local handle, tmp = open_progress_tmp()
	if not handle then
		return
	end
	handle:write(body)
	handle:close()
	if os.rename(tmp, PROGRESS_PATH) then
		wezterm.GLOBAL.ai_panes_progress_body = body
	end
end

local function find_pane(pane_id)
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

-- タブバー左端に出す `nvim:2 claude:1` 形式のサマリ。
-- ダッシュボードペインはそのタブの中でしか見えないので、どのタブ / どの workspace に
-- いても総数だけは分かるようにする補完。1 本も動いていなければ何も出さない。
local function status_text()
	local now = os.time()
	local cached_at = wezterm.GLOBAL.ai_panes_status_at
	if cached_at and (now - cached_at) < STATUS_THROTTLE_SECONDS then
		return wezterm.GLOBAL.ai_panes_status_text or ""
	end

	local counts = count_tracked()

	local parts = {}
	for _, name in ipairs(TRACKED_ORDER) do
		if counts[name] then
			table.insert(parts, { Foreground = { Color = TRACKED[name] } })
			table.insert(parts, { Text = string.format(" %s:%d", name, counts[name]) })
		end
	end

	local text = ""
	if #parts > 0 then
		table.insert(parts, { Text = " " })
		text = wezterm.format(parts)
	end

	wezterm.GLOBAL.ai_panes_status_at = now
	wezterm.GLOBAL.ai_panes_status_text = text
	return text
end

local function find_dashboard(tab)
	for _, pane in ipairs(tab:panes()) do
		local ok, vars = pcall(function()
			return pane:get_user_vars()
		end)
		if ok and vars and vars[DASHBOARD_VAR] then
			return pane
		end
	end
	return nil
end

local function toggle_dashboard()
	return wezterm.action_callback(function(window, pane)
		local existing = find_dashboard(window:active_tab())
		if existing then
			-- CloseCurrentPane は perform_action に渡したペインではなく「いまアクティブな
			-- ペイン」を閉じてしまうので使えない。スクリプトはペイン直下のプロセスなので、
			-- Ctrl-C を送れば INT トラップが exit 0 し、WezTerm がそのペインを閉じる。
			existing:send_text("\003")
			return
		end

		window:perform_action(
			act.SplitPane({
				direction = "Left",
				size = { Percent = DASHBOARD_PERCENT },
				command = { args = { DASHBOARD_CMD } },
			}),
			pane
		)
	end)
end

return function(config)
	wezterm.on("update-status", function(window)
		write_progress()
		if not window:is_focused() then
			return
		end
		window:set_left_status(status_text())
	end)

	wezterm.on("open-uri", function(window, _, uri)
		local id = uri:match(JUMP_URI_PATTERN)
		if not id then
			return
		end
		local row = find_pane(tonumber(id))
		if row then
			jump_to(window, row)
		else
			window:toast_notification("WezTerm", "Pane #" .. id .. " is gone", nil, 2000)
		end
		return false
	end)

	wezterm.on("user-var-changed", function(window, _, name, value)
		if name ~= JUMP_VAR then
			return
		end
		local id = value:match(JUMP_VAR_PATTERN)
		if not id then
			return
		end
		local row = find_pane(tonumber(id))
		if row then
			jump_to(window, row)
		else
			window:toast_notification("WezTerm", "Pane #" .. id .. " is gone", nil, 2000)
		end
	end)

	table.insert(config.keys, { key = "A", mods = "CMD|SHIFT", action = toggle_dashboard() })
end
