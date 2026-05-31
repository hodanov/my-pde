local lint = require("lint")

lint.linters_by_ft = {
	markdown = { "markdownlint-cli2" },
	dockerfile = { "hadolint" },
	terraform = { "tflint" },
}

-- Run it when you open, save or exit insert mode (you can adjust the events to your liking).
local grp = vim.api.nvim_create_augroup("nvim-lint", { clear = true })
vim.api.nvim_create_autocmd({ "BufReadPost", "BufWritePost" }, {
	group = grp,
	callback = function()
		lint.try_lint() -- run linters_by_ft depending on filetype.
	end,
})
