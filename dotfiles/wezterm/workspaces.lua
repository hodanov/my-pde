local wezterm = require("wezterm")
local mux = wezterm.mux

local HOME = os.getenv("HOME")

-- ワークスペース定義
-- tabs に split を指定するとペイン分割される
local workspace_defs = {
	{
		name = "blog",
		cwd = HOME .. "/workspace/hodalog-hugo",
		tabs = {
			{ title = "nvim" },
			{ title = "ai-cli" },
			{ title = "ops" },
		},
	},
	{
		name = "stable-diffusion",
		cwd = HOME .. "/workspace/stable_diffusion_modal",
		tabs = {
			{ title = "nvim+ops", split = { direction = "Bottom", size = 0.20 } },
		},
	},
	{
		name = "my-pde",
		cwd = HOME .. "/workspace/my-pde",
		tabs = {
			{ title = "nvim" },
			{ title = "ai-cli" },
			{ title = "ops" },
		},
	},
	{
		name = "new-project",
		cwd = HOME .. "/workspace/new-project",
		tabs = {
			{ title = "nvim" },
			{ title = "ai-cli" },
			{ title = "ops" },
		},
	},
}

local default_workspace = "blog"

local function workspace_exists(name)
	for _, win in ipairs(mux.all_windows()) do
		if win:get_workspace() == name then
			return true
		end
	end
	return false
end

local function setup_workspace(def)
	if workspace_exists(def.name) then
		return
	end

	local first_tab = def.tabs[1]
	local t, p, w = mux.spawn_window({
		workspace = def.name,
		cwd = def.cwd,
		args = { "zsh", "-lc", "$SHELL" },
	})
	t:set_title(first_tab.title)

	if first_tab.split then
		p:split({
			direction = first_tab.split.direction,
			size = first_tab.split.size,
			args = { "zsh", "-lc", "$SHELL" },
		})
	end

	for i = 2, #def.tabs do
		local tab_def = def.tabs[i]
		local new_tab, new_pane = w:spawn_tab({
			cwd = def.cwd,
			args = { "zsh", "-lc", "$SHELL" },
		})
		new_tab:set_title(tab_def.title)

		if tab_def.split then
			new_pane:split({
				direction = tab_def.split.direction,
				size = tab_def.split.size,
				args = { "zsh", "-lc", "$SHELL" },
			})
		end
	end
end

local function setup_all()
	for _, def in ipairs(workspace_defs) do
		setup_workspace(def)
	end
end

return function(config)
	-- ステータスバーにワークスペース名を表示
	wezterm.on("update-status", function(window)
		window:set_right_status(window:active_workspace())
	end)

	-- 起動時にワークスペースを作成
	wezterm.on("gui-startup", function()
		setup_all()
		mux.set_active_workspace(default_workspace)
	end)

	-- CMD+SHIFT+R でワークスペースを追加生成（既存はスキップ）
	wezterm.on("setup-project-layouts", function()
		setup_all()
	end)
	table.insert(config.keys, {
		key = "R",
		mods = "CMD|SHIFT",
		action = wezterm.action.EmitEvent("setup-project-layouts"),
	})
end
