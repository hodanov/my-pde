-- ----------------------------------------
-- Disable unused providers.
-- ----------------------------------------
vim.g.loaded_node_provider = 0
vim.g.loaded_perl_provider = 0
vim.g.loaded_python3_provider = 0
vim.g.loaded_ruby_provider = 0

-- ----------------------------------------
-- Key bind and other setting.
-- ----------------------------------------
vim.opt.encoding = "utf-8" -- Prevent garbled characters
vim.opt.fileencoding = "utf-8" -- Setting for handling multi byte characters
vim.scriptencoding = "utf-8" -- Setting for handling multi byte characters
vim.opt.number = true -- Add row number
vim.opt.title = true -- Add a filename to each tabs
vim.opt.cursorline = true -- Add cursor line
vim.opt.tabstop = 4 -- Insert spaces when the tab key is pressed
vim.opt.shiftwidth = 4 -- Change the number of spaces inserted for indentation
-- vim.opt.softtabstop = 4 -- Make spaces feel like real tabs
vim.opt.expandtab = true -- Convert tabs to spaces
vim.opt.smartindent = true -- Add a new line with autoindent
vim.opt.colorcolumn = "120" -- Add a color on 80'th column
vim.opt.hlsearch = true -- Highlight searched characters
vim.opt.incsearch = true -- Highlight when inputting chars
vim.opt.wildmenu = true -- Show completion suggestions at command line mode
vim.opt.conceallevel = 0 -- Show double quotations in json file and so on.
vim.g.mapleader = " " -- Set a space key to a leader.
vim.opt.mouse = "" -- Don't use a mouse.
vim.opt.signcolumn = "yes" -- Always show signcolumn to prevent rattling.
vim.opt.foldlevelstart = 99 -- Open files fully expanded; folds (treesitter foldexpr) are closed manually when needed.
vim.opt.updatetime = 300 -- Fire CursorHold sooner (default 4000ms) for LSP document highlight. Kept >250ms to avoid frequent swap writes.

-- ----------------------------------------
-- Neovim 0.12 で追加された UI オプション
-- ----------------------------------------
vim.opt.pumborder = "rounded" -- Add a rounded border to the popup menu (completion).
vim.opt.pummaxwidth = 60 -- Cap the popup menu width to keep long doc lines readable.
vim.opt.winborder = "rounded" -- Default border for floating windows (hover, signature help, etc.).

-- ----------------------------------------
-- Copy to the system clipboard.
-- ----------------------------------------
local has_osc52, osc52 = pcall(require, "vim.ui.clipboard.osc52")
if has_osc52 then
	local osc52_copy_plus = osc52.copy("+")
	local osc52_copy_star = osc52.copy("*")
	local osc52_yank_group = vim.api.nvim_create_augroup("auto_copy_yank_to_osc52", { clear = true })
	vim.api.nvim_create_autocmd("TextYankPost", {
		group = osc52_yank_group,
		callback = function()
			if vim.v.event.operator ~= "y" then
				return
			end
			if vim.v.event.regname ~= "" and vim.v.event.regname ~= "+" and vim.v.event.regname ~= "*" then
				return
			end
			local yanked_text = vim.fn.getreg('"', 1, true)
			local yanked_regtype = vim.fn.getregtype('"')
			osc52_copy_plus(yanked_text, yanked_regtype)
			osc52_copy_star(yanked_text, yanked_regtype)
		end,
	})
elseif vim.fn.has("clipboard") == 1 then
	vim.opt.clipboard = "unnamedplus"
end

-- ----------------------------------------
-- Highlight yanked text (ヤンク範囲を一瞬フラッシュして視覚フィードバックする)
-- ----------------------------------------
local hl_yank_group = vim.api.nvim_create_augroup("highlight_yank", { clear = true })
vim.api.nvim_create_autocmd("TextYankPost", {
	group = hl_yank_group,
	callback = function()
		-- 0.12 では vim.hl.on_yank が現行 API。将来 (0.13+) は hl_op へ移行し
		-- on_yank が deprecated 化するため、存在すれば hl_op を優先する。
		if vim.hl and vim.hl.hl_op then
			vim.hl.hl_op({ higroup = "IncSearch", timeout = 200 })
		else
			vim.hl.on_yank({ higroup = "IncSearch", timeout = 200 })
		end
	end,
})

-- ----------------------------------------
-- Remember a history of undo/redo.
-- ----------------------------------------
if vim.fn.has("persistent_undo") == 1 then
	local undo_path = vim.fn.expand("~/.local/state/nvim/undo")
	vim.cmd("set undodir=" .. undo_path)
	vim.opt.undofile = true
end

-- ----------------------------------------
-- Settings for indent each files.
-- ----------------------------------------
vim.api.nvim_create_augroup("html_css_js_and_others_indent", { clear = true })
vim.api.nvim_create_autocmd({ "BufNewFile", "BufRead" }, {
	group = "html_css_js_and_others_indent",
	pattern = { "*.yml", "*.yaml", "*.tmpl", "*json" },
	command = "set tabstop=2 shiftwidth=2",
})
vim.api.nvim_create_autocmd({ "BufNewFile", "BufRead" }, {
	group = "html_css_js_and_others_indent",
	pattern = { "*.html", "*.css", "*.js", "*.ts", "*.php" },
	command = "set tabstop=2 shiftwidth=2",
})
vim.api.nvim_create_autocmd({ "BufNewFile", "BufRead" }, {
	group = "html_css_js_and_others_indent",
	pattern = "*.go",
	command = "set noexpandtab tabstop=8 shiftwidth=8",
})

-- ----------------------------------------
-- Open init.vim and 'source' it.
-- ----------------------------------------
vim.api.nvim_set_keymap("n", "<Leader>.", ":vs ~/.config/nvim/init.lua<CR>", { noremap = true, silent = true })
vim.api.nvim_set_keymap("n", "<Leader>s", ":source ~/.config/nvim/init.lua<CR>", { noremap = true, silent = true })

-- ----------------------------------------
-- Clear highlighted characters.
-- ----------------------------------------
vim.api.nvim_set_keymap("n", "<C-[><C-[>", ":nohlsearch<CR>", { noremap = true, silent = true })

-- ----------------------------------------
-- vimshell setting.
-- ----------------------------------------
if vim.fn.has("nvim") == 1 then
	vim.api.nvim_set_keymap("n", "<Leader>-", ":split term://bash<CR>", { noremap = true, silent = true })
	vim.api.nvim_set_keymap("n", "<Leader>l", ":vsplit term://bash<CR>", { noremap = true, silent = true })
else
	vim.api.nvim_set_keymap(
		"n",
		"<Leader>-",
		":below terminal ++close ++rows=13 bash<CR>",
		{ noremap = true, silent = true }
	)
	vim.api.nvim_set_keymap("n", "<Leader>l", ":vertical terminal ++close bash<CR>", { noremap = true, silent = true })
end

-- ----------------------------------------
-- indent_guides setting.
-- ----------------------------------------
vim.g.indent_guides_enable_on_vim_startup = 1
vim.g.indent_guides_start_level = 2
vim.g.indent_guides_guide_size = 1

-- ----------------------------------------
-- Undo tree viewer (Neovim 0.12 builtin).
-- ----------------------------------------
vim.api.nvim_set_keymap("n", "<Leader>u", ":Undotree<CR>", { noremap = true, silent = true })

-- ----------------------------------------
-- fern.vim setting.
-- ----------------------------------------
vim.api.nvim_set_keymap(
	"n",
	"<Leader>o",
	":Fern . -drawer -reveal=% -width=30 -toggle<CR>",
	{ noremap = true, silent = true }
)
vim.api.nvim_set_var("fern#default_hidden", 1)

-- ----------------------------------------
-- lazy.nvim setting.
-- ----------------------------------------
require("lazy_nvim")

-- ----------------------------------------
-- Setting transparent background.
-- ----------------------------------------
vim.cmd([[
  highlight Normal guibg=none
  highlight NonText guibg=none
  highlight Normal ctermbg=none
  highlight NonText ctermbg=none
  highlight NormalNC guibg=none
  highlight NormalSB guibg=none
]])

-- ----------------------------------------
-- lsp setting.
-- ----------------------------------------
require("lsp")

-- ----------------------------------------
-- ai_bridge setting.
-- ----------------------------------------
require("ai_bridge")

-- ----------------------------------------
-- textlint setting.
-- ----------------------------------------
local textlint = require("textlint_nvim")
textlint.setup({
	cmd = "textlint",
	filetypes = { "markdown", "text", "plaintext" },
	debounce = 500,
})

vim.keymap.set("n", "<leader>tl", textlint.lint, { desc = "Run textlint" })
vim.keymap.set("n", "<leader>tc", textlint.clear, { desc = "Clear textlint diagnostics" })
