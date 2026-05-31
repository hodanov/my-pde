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
		markdown = { "markdownlint-cli2" },
		python = { "ruff_fix", "ruff_format", "ruff_organize_imports" },
		terraform = { "terraform_fmt" },
		["terraform-vars"] = { "terraform_fmt" },
		yaml = { "prettierd", "prettier", stop_after_first = true },
	},
})
