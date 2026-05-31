local wezterm = require("wezterm")

return function(config)
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
		local title = tab.active_pane.title
		title = title:gsub(".*[/\\]", "")

		local icon = " "

		if tab.is_active then
			return {
				{ Text = " " .. icon .. title .. " " },
			}
		else
			return {
				{ Foreground = { Color = "#6c7086" } },
				{ Text = " " .. icon .. title .. " " },
			}
		end
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
			opacity = 0.23,
			vertical_align = "Bottom",
			horizontal_align = "Right",
			repeat_x = "NoRepeat",
			repeat_y = "NoRepeat",
			width = "1247",
			height = "1795",
		},
	}
end
