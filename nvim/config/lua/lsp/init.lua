-- ----------------------------------------
-- LSP configuration (Neovim 0.12 native style)
--
-- 0.11/0.12 で標準化されたデフォルトマッピングを優先採用し、
-- 同等のことをやっていた手動マッピングは削除している。
--
-- コアが提供するデフォルト (本ファイルでは設定しない):
--   K     : vim.lsp.buf.hover()                   カーソル下のシンボル (関数名 / 変数名 / 型名) の ドキュメントや型情報を float で表示
--   grn   : vim.lsp.buf.rename()                  カーソル下のシンボルを参照場所/定義元含めてリネーム
--   gra   : vim.lsp.buf.code_action()
--   grr   : vim.lsp.buf.references()              参照場所にジャンプ
--   gri   : vim.lsp.buf.implementation()          インターフェースの定義 から、そのインターフェースを 実装している型 にジャンプ (複数あればリスト表示)
--   grt   : vim.lsp.buf.type_definition()         (0.12 で追加)
--   grx   : vim.lsp.codelens.run()                (0.12 で追加)
--   ]d/[d : vim.diagnostic.jump({ count = ±1 })   (0.11 で追加, 0.12 で goto_prev/next を deprecated)
--
-- 参考資料
-- - https://github.com/neovim/nvim-lspconfig/tree/master/lua/lspconfig/configs
-- - https://zenn.dev/kawarimidoll/articles/b202e546bca344
-- ----------------------------------------

-- Go
vim.lsp.config("gopls", {
	settings = {
		gopls = {
			codelenses = {
				generate = true,
				test = true,
				tidy = true,
				upgrade_dependency = true,
				regenerate_cgo = true,
				run_govulncheck = true,
				gc_details = true,
			},
			-- gopls は hints を明示指定しないとインレイヒントを一切返さないため、
			-- エディタ側の有効化 (LspAttach) と合わせてここで表示対象を指定する。
			hints = {
				assignVariableTypes = true,
				compositeLiteralFields = true,
				compositeLiteralTypes = true,
				constantValues = true,
				functionTypeParameters = true,
				parameterNames = true,
				rangeVariableTypes = true,
			},
		},
	},
})
vim.lsp.enable("gopls")
vim.lsp.enable("golangci_lint_ls")

-- JavaScript, Node.js
vim.lsp.enable("ts_ls")
vim.lsp.enable("eslint")

-- Python
vim.lsp.enable("pyright")
vim.lsp.enable("ruff")

-- Terraform
vim.lsp.enable("terraformls")

-- Config files
vim.lsp.enable("yamlls")
vim.lsp.enable("tombi")
-- vim.lsp.enable("lemminx")

-- Global mappings.
-- See `:help vim.diagnostic.*` for documentation on any of the below functions
vim.keymap.set("n", "<space>e", vim.diagnostic.open_float)
vim.keymap.set("n", "<space>q", vim.diagnostic.setloclist)

-- Use LspAttach autocommand to only map the following keys
-- after the language server attaches to the current buffer
vim.api.nvim_create_autocmd("LspAttach", {
	group = vim.api.nvim_create_augroup("UserLspConfig", {}),
	callback = function(ev)
		-- Enable completion triggered by <c-x><c-o>
		vim.bo[ev.buf].omnifunc = "v:lua.vim.lsp.omnifunc"

		-- Buffer local mappings (デフォルトで提供されないものだけを設定する)
		local opts = { buffer = ev.buf }
		vim.keymap.set("n", "gD", vim.lsp.buf.declaration, opts)
		vim.keymap.set("n", "gd", vim.lsp.buf.definition, opts)
		vim.keymap.set("n", "<C-k>", vim.lsp.buf.signature_help, opts)
		vim.keymap.set("n", "<space>wa", vim.lsp.buf.add_workspace_folder, opts)
		vim.keymap.set("n", "<space>wr", vim.lsp.buf.remove_workspace_folder, opts)
		vim.keymap.set("n", "<space>wl", function()
			print(vim.inspect(vim.lsp.buf.list_workspace_folders()))
		end, opts)
		vim.keymap.set("n", "<space>f", function()
			vim.lsp.buf.format({ async = true })
		end, opts)

		-- Code lens を全 LSP で常時有効化する。
		-- vim.lsp.codelens.enable は内部で BufEnter/InsertLeave 等の refresh まで面倒を見るので、
		-- 自前の autocmd ループは不要 (0.12 で codelens.refresh は deprecated)。
		-- ノイズが目立つサーバーが出てきたら、ここで client.name を見て弾く方針に切り替える。
		local client = vim.lsp.get_client_by_id(ev.data.client_id)
		if client and client:supports_method("textDocument/codeLens") then
			vim.lsp.codelens.enable(true, { bufnr = ev.buf })
		end

		-- Inlay hint (vim.lsp.inlay_hint) を対応サーバーで常時有効化する。
		-- 変数の推論型・関数の引数名などをソースを書き換えずに仮想テキストで補足表示する。
		-- ノイズに感じる場合は下の <space>h でバッファ単位にトグルできる。
		if client and client:supports_method("textDocument/inlayHint") then
			vim.lsp.inlay_hint.enable(true, { bufnr = ev.buf })
			vim.keymap.set("n", "<space>h", function()
				vim.lsp.inlay_hint.enable(not vim.lsp.inlay_hint.is_enabled({ bufnr = 0 }), { bufnr = 0 })
			end, opts)
		end
	end,
})

-- Diagnostic 設定
-- - virtual_text は無効、float で詳細を確認する
-- - virtual_lines = { current_line = true } でカーソル行の診断だけを直下にインライン表示する
--   (0.11 で追加。全行に出さず現在行に限定するので「普段はクリーン、必要な行だけ詳細」を維持できる)
-- - ]d/[d でジャンプした際に on_jump コールバックから open_float を呼び、
--   DiagnosticRelatedInformation も含めてその場で確認できるようにする
--   (0.12 で `jump.float` は deprecated)
vim.diagnostic.config({
	virtual_text = false,
	virtual_lines = { current_line = true },
	jump = {
		on_jump = function(_, bufnr)
			vim.diagnostic.open_float({ bufnr = bufnr })
		end,
	},
})
