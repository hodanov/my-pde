require("gitsigns").setup({
	on_attach = function(bufnr)
		local gs = require("gitsigns")
		local function map(mode, l, r, desc)
			vim.keymap.set(mode, l, r, { buffer = bufnr, desc = desc })
		end

		-- ハンク移動（diff モード時はネイティブの ]c/[c に委譲）
		map("n", "]c", function()
			if vim.wo.diff then
				vim.cmd.normal({ "]c", bang = true })
			else
				gs.nav_hunk("next")
			end
		end, "Next hunk")
		map("n", "[c", function()
			if vim.wo.diff then
				vim.cmd.normal({ "[c", bang = true })
			else
				gs.nav_hunk("prev")
			end
		end, "Prev hunk")

		-- プレビュー（<leader> = <space>。<space>h を避けて <leader>g 系に）
		map("n", "<leader>gp", gs.preview_hunk, "Preview hunk")
		map("n", "<leader>gi", gs.preview_hunk_inline, "Preview hunk inline")
	end,
})
