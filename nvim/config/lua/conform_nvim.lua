require("conform").setup({
	format_on_save = {
		timeout_ms = 500,
		lsp_format = "fallback",
	},
	formatters_by_ft = {
		css = { "prettierd", "prettier", stop_after_first = true },
		go = { "goimports" },
		html = { "prettierd", "prettier", stop_after_first = true },
		javascript = { "prettierd", "prettier", stop_after_first = true },
		json = { "prettierd", "prettier", stop_after_first = true },
		lua = { "stylua" },
		-- CI (`mise run lint:format`) と AI フック (markdown-format.sh) は
		-- markdownlint --fix → prettier --write の順で両方を掛ける。保存時整形も
		-- 同じ順序・同じ構成にして、エディタ経由の編集だけが prettier --check に
		-- 落ちる状態を作らない。stop_after_first は付けない（付けると
		-- markdownlint だけで打ち切られ現状と変わらないため）。
		markdown = { "markdownlint-cli2", "prettierd" },
		python = { "ruff_fix", "ruff_format", "ruff_organize_imports" },
		sh = { "shfmt" },
		terraform = { "terraform_fmt" },
		["terraform-vars"] = { "terraform_fmt" },
		yaml = { "prettierd", "prettier", stop_after_first = true },
	},
	formatters = {
		shfmt = {
			args = { "-filename", "$FILENAME" },
		},
	},
})

vim.keymap.set({ "n", "v" }, "<space>f", function()
	require("conform").format({ async = true, lsp_format = "fallback" })
end, { desc = "Format buffer or range (conform)" })
