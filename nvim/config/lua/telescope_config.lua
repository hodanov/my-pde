-- ----------
-- telescope.nvim
-- ----------
local builtin = require("telescope.builtin")
-- vim.keymap.set("n", "<Leader>ff", builtin.find_files, { desc = "Find Files" })
-- vim.keymap.set("n", "<Leader>fg", builtin.live_grep, { desc = "Live Grep" })
vim.keymap.set("n", "<Leader>ff", function()
	builtin.find_files({
		hidden = true, -- .git, .github, .configなどを含む
		no_ignore = true, -- .gitignoreを無視しない
		no_ignore_parent = true,
	})
end, { desc = "Find Files (include hidden)" })

vim.keymap.set("n", "<Leader>fg", function()
	builtin.live_grep({
		additional_args = function()
			return { "--hidden" } -- grepでも隠しファイルを対象に
		end,
	})
end, { desc = "Live Grep (include hidden)" })

vim.keymap.set("n", "<leader>fb", builtin.buffers, { desc = "Telescope buffers" })
vim.keymap.set("n", "<leader>fh", builtin.help_tags, { desc = "Telescope help tags" })

-- LSP シンボル / 診断ピッカー (LSP アタッチ済みバッファで有効)
vim.keymap.set("n", "<leader>fs", builtin.lsp_document_symbols, { desc = "LSP document symbols (current file)" })
vim.keymap.set("n", "<leader>fS", builtin.lsp_dynamic_workspace_symbols, { desc = "LSP workspace symbols (project)" })
vim.keymap.set("n", "<leader>fd", function()
	builtin.diagnostics({ bufnr = 0 })
end, { desc = "Diagnostics (current buffer)" })
vim.keymap.set("n", "<leader>fD", builtin.diagnostics, { desc = "Diagnostics (all buffers)" })

-- ----------
-- telescope-file-browser.nvim
-- ----------
local telescope = require("telescope")
telescope.setup({
	defaults = {
		sorting_strategy = "ascending", -- 上から順番に表示
		file_sorter = require("telescope.sorters").get_fuzzy_file, -- アルファベット順ベース
		layout_config = {
			prompt_position = "top", -- 検索窓を上に
		},
		find_files = { hidden = true },
	},
	pickers = {
		find_files = {
			hidden = true,
			no_ignore = true,
			no_ignore_parent = true,
		},
	},
	extensions = {
		file_browser = {
			sorting_strategy = "ascending",
			layout_config = {
				prompt_position = "top",
			},
			grouped = true, -- ディレクトリを上にまとめる
			hidden = true,
		},
	},
})
telescope.load_extension("file_browser")
-- vim.keymap.set("n", "<space>fo", ":Telescope file_browser<CR>", { desc = "Telescope File Browser" })
vim.keymap.set("n", "<space>fo", function()
	require("telescope").extensions.file_browser.file_browser({
		path = "%:p:h", -- 現在開いてるファイルのディレクトリ
		select_buffer = true, -- バッファのパスを基準に
		hidden = true,
	})
end, { desc = "Telescope File Browser (current file dir)" })
