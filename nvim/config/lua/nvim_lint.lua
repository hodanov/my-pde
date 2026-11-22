local lint = require("lint")

-- .github/workflows/ 配下のファイルだけ複合 filetype "yaml.ghaction" にする。
-- こうすると通常の yaml (docker-compose.yml 等) には actionlint が走らず、
-- yamlls による汎用 YAML 検査 (yaml 成分) と actionlint (ghaction 成分) が両立する。
vim.filetype.add({
	pattern = {
		[".*/%.github/workflows/.*%.ya?ml"] = "yaml.ghaction",
	},
})

-- zizmor は既定の stdin 実行をやめてファイルパス渡しに上書きする。stdin 入力では
-- .github/zizmor.yml の `self-repository: disable` が効かず (--config 明示でも同じ)、
-- composite action を使う workflow 全部に誤検知が出る。stdin = false にすると
-- nvim-lint がファイル名を末尾に付けるため、mise run lint:zizmor と同じ呼び出しになる。
lint.linters.zizmor.stdin = false
lint.linters.zizmor.args = { "--format", "json-v1" }

lint.linters_by_ft = {
	markdown = { "markdownlint-cli2" },
	dockerfile = { "hadolint" },
	terraform = { "tflint" },
	sh = { "shellcheck" },
	ghaction = { "actionlint", "zizmor" },
}

-- Run it when you open, save or exit insert mode (you can adjust the events to your liking).
local grp = vim.api.nvim_create_augroup("nvim-lint", { clear = true })
vim.api.nvim_create_autocmd({ "BufReadPost", "BufWritePost" }, {
	group = grp,
	callback = function()
		lint.try_lint() -- run linters_by_ft depending on filetype.
	end,
})
