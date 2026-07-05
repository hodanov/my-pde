local lint = require("lint")

-- .github/workflows/ 配下のファイルだけ複合 filetype "yaml.ghaction" にする。
-- こうすると通常の yaml (docker-compose.yml 等) には actionlint が走らず、
-- yamlls による汎用 YAML 検査 (yaml 成分) と actionlint (ghaction 成分) が両立する。
vim.filetype.add({
	pattern = {
		[".*/%.github/workflows/.*%.ya?ml"] = "yaml.ghaction",
	},
})

lint.linters_by_ft = {
	markdown = { "markdownlint-cli2" },
	dockerfile = { "hadolint" },
	terraform = { "tflint" },
	-- GitHub Actions ワークフロー専用。複合 filetype の "ghaction" 成分を key にすることで
	-- 通常の yaml には作用させない。actionlint 未インストール時は nvim-lint が黙ってスキップする。
	ghaction = { "actionlint" },
}

-- Run it when you open, save or exit insert mode (you can adjust the events to your liking).
local grp = vim.api.nvim_create_augroup("nvim-lint", { clear = true })
vim.api.nvim_create_autocmd({ "BufReadPost", "BufWritePost" }, {
	group = grp,
	callback = function()
		lint.try_lint() -- run linters_by_ft depending on filetype.
	end,
})
