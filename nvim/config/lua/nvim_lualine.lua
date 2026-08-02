require("lualine").setup({
	options = {
		icons_enabled = false,
		theme = "auto",
		component_separators = { left = "", right = "" },
		section_separators = { left = "", right = "" },
		disabled_filetypes = {
			statusline = {},
			winbar = {},
		},
		ignore_focus = {},
		always_divide_middle = true,
		globalstatus = true, -- laststatus=3。分割時もステータスラインを画面最下部の1本に統一する
		refresh = {
			statusline = 1000,
			tabline = 1000,
			winbar = 1000,
		},
	},
	sections = {
		lualine_a = { "mode" },
		lualine_b = { "branch", "diff", "diagnostics" },
		lualine_c = { { "filename", path = 1 } },
		lualine_x = {
			-- 'busy' は Neovim 0.12 で追加されたバッファのビジー状態を示すオプション。
			-- LSP リクエスト中などに ◐ を表示する。
			{
				function()
					local ok, busy = pcall(function()
						return vim.o.busy
					end)
					return (ok and busy and busy > 0) and "◐" or ""
				end,
			},
			-- vim.ui.progress_status() も 0.12 で追加。LSP の進捗メッセージを統一表示する。
			{
				function()
					local ok, status = pcall(vim.ui.progress_status)
					return (ok and status) or ""
				end,
			},
			"encoding",
			"fileformat",
			"filetype",
		},
		lualine_y = { "progress" },
		lualine_z = { "location" },
	},
	inactive_sections = {
		lualine_a = {},
		lualine_b = {},
		lualine_c = { { "filename", path = 1 } },
		lualine_x = { "location" },
		lualine_y = {},
		lualine_z = {},
	},
	tabline = {},
	winbar = {},
	inactive_winbar = {},
	extensions = {},
})
