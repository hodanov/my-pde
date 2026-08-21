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

-- ステータスバーとピッカーの並びを固定するための一覧。pairs() の走査順は不定なので、
-- これを使わないとリフレッシュのたびに並びが入れ替わる。
-- nvim は「その workspace の主ペイン」なので先頭に置く。
local TRACKED_ORDER = { "nvim", "claude", "codex", "cursor-agent", "copilot" }

-- ピッカーのソート用に TRACKED_ORDER を逆引きできるようにしておく。
local TRACKED_RANK = {}
for i, name in ipairs(TRACKED_ORDER) do
	TRACKED_RANK[name] = i
end

-- ピッカーの行はフィールドを空白で並べただけだと workspace 名と CLI 名が地続きに
-- 読めてしまうので、境目には可視のセパレータを挟む。
-- ラベルに色は付けない。InputSelector は選択行を Reverse 属性で描くので、前景色を
-- 指定するとその色が選択時の背景になり、行の背景がフィールドごとにまだらになる。
-- 色無しにしておけば CMD+s のワークスペース選択と同じ均一な背景が乗る。
local SEP = " / "

-- 対象が 1 本も無いときにピッカーへ出すプレースホルダの id。
-- トースト通知は macOS の通知設定次第で無音になり「何も起きない」ように見えるため、
-- 空でもセレクタ自体は開いて理由を出す。
local NO_PANES_ID = "__none__"

-- ダッシュボードペインが起動直後に立てる user var。
-- pane_id を wezterm.GLOBAL に持つ方式と違い、ペインが消えれば目印も消えるので
-- 手動で閉じられても設定をリロードしても状態がずれない。
local DASHBOARD_VAR = "ai_panes_dashboard"
local DASHBOARD_CMD = wezterm.config_dir .. "/bin/ai-panes.sh"
local DASHBOARD_PERCENT = 18

-- update-status は毎秒発火するので、全ペイン走査はこの間隔まで間引く。
local STATUS_THROTTLE_SECONDS = 3

local function basename(path)
	if path == nil or path == "" then
		return nil
	end
	return path:gsub("/+$", ""):match("([^/]+)$")
end

-- 20240127 以降 get_current_working_dir() は Url オブジェクトを返す。
-- 文字列を返す旧版とも両対応にしておく。
local function pane_cwd(pane)
	local ok, cwd = pcall(function()
		return pane:get_current_working_dir()
	end)
	if not ok or cwd == nil then
		return nil
	end
	if type(cwd) == "string" then
		return (cwd:gsub("^file://[^/]*", ""))
	end
	return cwd.file_path
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
local function process_of(pane)
	local candidates = {}
	local info = nil

	local ok_info, got = pcall(function()
		return pane:get_foreground_process_info()
	end)
	if ok_info and got then
		info = got
		table.insert(candidates, info.name)
		if info.argv then
			table.insert(candidates, info.argv[1])
		end
		table.insert(candidates, info.executable)
	end

	local ok_name, proc = pcall(function()
		return pane:get_foreground_process_name()
	end)
	if ok_name then
		table.insert(candidates, proc)
	end

	for _, candidate in ipairs(candidates) do
		local name = basename(candidate)
		if name and TRACKED[name] then
			return name
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

-- mux 配下の全 window / tab / pane を走査して追跡対象が動いているペインを集める。
-- この wezterm には mux.get_pane() が無いので、後でジャンプできるよう
-- MuxPane オブジェクトそのものを行に持たせる。
local function collect()
	local rows = {}
	for _, win in ipairs(mux.all_windows()) do
		local workspace = win:get_workspace()
		for _, tab in ipairs(win:tabs()) do
			for _, pane in ipairs(tab:panes()) do
				local proc = process_of(pane)
				if proc then
					table.insert(rows, {
						workspace = workspace,
						proc = proc,
						project = basename(pane_cwd(pane)) or workspace,
						tab_title = tab:get_title(),
						pane_id = pane:pane_id(),
						pane = pane,
					})
				end
			end
		end
	end
	table.sort(rows, function(a, b)
		if a.workspace ~= b.workspace then
			return a.workspace < b.workspace
		end
		if a.proc ~= b.proc then
			return (TRACKED_RANK[a.proc] or math.huge) < (TRACKED_RANK[b.proc] or math.huge)
		end
		return a.pane_id < b.pane_id
	end)
	return rows
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

local function select_pane()
	return wezterm.action_callback(function(window, pane)
		local rows = collect()

		local choices = {}
		if #rows == 0 then
			table.insert(choices, { id = NO_PANES_ID, label = "no tracked panes" })
		end
		for _, row in ipairs(rows) do
			local label = row.workspace .. SEP .. row.proc
			-- タブ名は無いことがある。空のまま並べると区切りだけが浮くので出さない。
			-- プロセス名と同じタブ名（`my-pde / nvim / nvim`）も冗長なだけなので出さない。
			if row.tab_title and row.tab_title ~= "" and row.tab_title ~= row.proc then
				label = label .. SEP .. row.tab_title
			end

			table.insert(choices, {
				id = tostring(row.pane_id),
				label = label .. string.format(" #%d", row.pane_id),
			})
		end

		window:perform_action(
			act.InputSelector({
				title = "Jump to pane",
				fuzzy = true,
				fuzzy_description = "Pane > ",
				choices = choices,
				action = wezterm.action_callback(function(w, _, id)
					if not id or id == NO_PANES_ID then
						return
					end
					for _, row in ipairs(rows) do
						if tostring(row.pane_id) == id then
							jump_to(w, row)
							return
						end
					end
				end),
			}),
			pane
		)
	end)
end

-- タブバー左端に出す `nvim:2 claude:1` 形式のサマリ。
-- ダッシュボードペインはそのタブの中でしか見えないので、どのタブ / どの workspace に
-- いても総数だけは分かるようにする補完。1 本も動いていなければ何も出さない。
--
-- キャッシュはスカラー 2 本で持つ。wezterm.GLOBAL に入れたテーブルはネストした
-- 書き換えが保持されないことがあるため、テーブルを持たせない。
local function status_text()
	local now = os.time()
	local cached_at = wezterm.GLOBAL.ai_panes_status_at
	if cached_at and (now - cached_at) < STATUS_THROTTLE_SECONDS then
		return wezterm.GLOBAL.ai_panes_status_text or ""
	end

	local counts = {}
	for _, row in ipairs(collect()) do
		counts[row.proc] = (counts[row.proc] or 0) + 1
	end

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
		-- 分割直後は新しいペインにフォーカスが移るので、作業中のペインへ戻す
		pane:activate()
	end)
end

return function(config)
	-- workspaces.lua が update-right-status / set_right_status を持っているので、
	-- こちらは update-status / set_left_status を使う。両イベントは併存して発火し、
	-- 書き込むスロットも別なので workspaces.lua を変更せずに共存できる。
	wezterm.on("update-status", function(window)
		window:set_left_status(status_text())
	end)

	table.insert(config.keys, { key = "a", mods = "CMD", action = select_pane() })
	table.insert(config.keys, { key = "A", mods = "CMD|SHIFT", action = toggle_dashboard() })
end
