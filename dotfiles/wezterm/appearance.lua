local wezterm = require("wezterm")

local TAB_INACTIVE_FG = "#6c7086"
local TAB_ERROR_FG = "#f38ba8"

local PROGRESS_SLICES = {
	wezterm.nerdfonts.md_circle_slice_1,
	wezterm.nerdfonts.md_circle_slice_2,
	wezterm.nerdfonts.md_circle_slice_3,
	wezterm.nerdfonts.md_circle_slice_4,
	wezterm.nerdfonts.md_circle_slice_5,
	wezterm.nerdfonts.md_circle_slice_6,
	wezterm.nerdfonts.md_circle_slice_7,
	wezterm.nerdfonts.md_circle_slice_8,
}

local function progress_mark(progress)
	if progress == nil or progress == "None" then
		return nil
	end
	if progress == "Indeterminate" then
		return { glyph = wezterm.nerdfonts.md_circle_medium }
	end
	if type(progress) ~= "table" then
		return nil
	end
	if progress.Percentage ~= nil then
		local slot = math.floor(progress.Percentage / 12.5) + 1
		if slot < 1 then
			slot = 1
		end
		if slot > #PROGRESS_SLICES then
			slot = #PROGRESS_SLICES
		end
		return { glyph = PROGRESS_SLICES[slot] }
	end
	if progress.Error ~= nil then
		return { glyph = wezterm.nerdfonts.md_alert_circle_outline, is_error = true }
	end
	return nil
end

return function(config)
	-- 描画バックエンド: macOS では wgpu 経由で Metal を使う WebGpu を明示
	config.front_end = "WebGpu"

	config.native_macos_fullscreen_mode = false
	config.macos_fullscreen_extend_behind_notch = true

	-- カラースキーマ
	config.color_scheme = "Catppuccin Mocha"

	-- タブバー
	config.enable_tab_bar = true
	config.window_decorations = "RESIZE"
	config.window_frame = {
		inactive_titlebar_bg = "none",
		active_titlebar_bg = "none",
	}
	config.show_new_tab_button_in_tab_bar = false
	config.tab_max_width = 32

	config.colors = {
		tab_bar = {
			background = "#1e1e2e",

			active_tab = {
				bg_color = "#cba6f7",
				fg_color = "#1e1e2e",
				intensity = "Bold",
			},

			inactive_tab = {
				bg_color = "#313244",
				fg_color = "#cdd6f4",
			},

			inactive_tab_hover = {
				bg_color = "#45475a",
				fg_color = "#ffffff",
				italic = true,
			},

			new_tab = {
				bg_color = "#1e1e2e",
				fg_color = "#a6adc8",
			},
		},
	}

	-- タブタイトルのフォーマット
	wezterm.on("format-tab-title", function(tab)
		local title = tab.active_pane.title:gsub(".*[/\\]", "")
		local mark = progress_mark(tab.active_pane.progress)
		local elements = {}

		if mark then
			if mark.is_error then
				table.insert(elements, { Foreground = { Color = TAB_ERROR_FG } })
				table.insert(elements, { Text = " " .. mark.glyph })
				table.insert(elements, "ResetAttributes")
			else
				table.insert(elements, { Text = " " .. mark.glyph })
			end
		end

		if not tab.is_active then
			table.insert(elements, { Foreground = { Color = TAB_INACTIVE_FG } })
		end
		table.insert(elements, { Text = (mark and " " or "  ") .. title .. " " })

		return elements
	end)

	-- フォント
	config.font = wezterm.font_with_fallback({
		{ family = "Meslo LG L DZ for Powerline" },
		{ family = "Hiragino Maru Gothic ProN W4" },
	})

	-- 背景
	config.background = {
		{
			source = {
				Gradient = {
					-- colors = { "#1e1e2e", "#1e1e2e" },
					colors = { "#181825", "#181825" },
					-- colors = { "#11111b", "#11111b" },
					orientation = {
						Linear = { angle = -30.0 },
					},
				},
			},
			opacity = 1,
			horizontal_align = "Right",
			width = "100%",
			height = "100%",
		},
		{
			source = {
				File = os.getenv("HOME") .. "/Pictures/romancing_saga_rs_alkaizer.PNG",
			},
			opacity = 0.13,
			vertical_align = "Bottom",
			horizontal_align = "Right",
			repeat_x = "NoRepeat",
			repeat_y = "NoRepeat",
			width = "1247",
			height = "1795",
		},
	}
end
