local ts = require("nvim-treesitter")

ts.setup({
	-- install_dir = vim.fn.stdpath("data") .. "/site",
})

-- Incremental selection は Neovim 0.12 のコア機能 (v_an / v_in / v_]n / v_[n) に任せる。
-- コアは LSP `textDocument/selectionRange` が使えればそれを優先し、なければ treesitter
-- ノードベースの実装にフォールバックする。
-- nvim-treesitter 新 main では `incremental_selection` モジュールが廃止されているため、
-- ここでプラグイン側のキー (gnn / grn / grm / grc 等) を再現する設定は入れない。

ts.install({ "go", "python", "markdown", "markdown_inline", "terraform", "hcl" })

vim.api.nvim_create_autocmd("FileType", {
	pattern = { "go", "python", "markdown", "markdown_inline", "terraform", "hcl" },
	callback = function()
		-- highlight
		vim.treesitter.start()
		-- folds
		-- vim.wo.foldexpr = "v:lua.vim.treesitter.foldexpr()"
		-- vim.wo.foldmethod = "expr"
		-- indent
		-- vim.bo.indentexpr = "v:lua.require'nvim-treesitter'.indentexpr()"
	end,
})
