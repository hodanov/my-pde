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
		-- bash/sh を保存時に整形。shfmt は nvim イメージ同梱（go-tools.txt 経由）。
		-- 追加引数なし＝shfmt 既定スタイルなので、CLI 側の `shfmt -w` フックと整形結果が一致する。
		sh = { "shfmt" },
		terraform = { "terraform_fmt" },
		["terraform-vars"] = { "terraform_fmt" },
		yaml = { "prettierd", "prettier", stop_after_first = true },
	},
	formatters = {
		-- conform 同梱の shfmt 定義は expandtab なバッファに `-i <shiftwidth>` を渡す。
		-- init.lua は expandtab=true / shiftwidth=4 なのでスペース 4 で整形されてしまい、
		-- CLI 側（`mise run lint:shell` の shfmt -d、hooks の shfmt -w＝いずれも引数なし）の
		-- タブ整形と食い違う。引数を既定に戻して整形設定を shfmt 自身へ委ねる
		-- （shfmt はフラグ未指定時に .editorconfig を読むので、そのリポジトリの流儀にも従う）。
		shfmt = {
			args = { "-filename", "$FILENAME" },
		},
	},
})

-- 手動フォーマット。保存時 (format_on_save) と同じ conform のチェーンを通す。
-- conform にフォーマッタが無い filetype では lsp_format = "fallback" で LSP 整形に委譲する。
vim.keymap.set({ "n", "v" }, "<space>f", function()
	require("conform").format({ async = true, lsp_format = "fallback" })
end, { desc = "Format buffer or range (conform)" })
