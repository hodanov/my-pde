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
vim.opt.inccommand = "split" -- :substitute の置換結果を入力中にライブプレビュー（下部スプリットに before/after 一覧）
vim.opt.ignorecase = true -- 小文字のみの検索パターンは大文字小文字を無視する
vim.opt.smartcase = true -- ただし大文字が1文字でも含まれる場合は大小を区別する（ignorecase と併用時のみ有効）
-- :grep / :lgrep を ripgrep バックエンドにし、結果を quickfix に集約する。
-- rg は .gitignore を尊重し高速。--vimgrep で file:line:col:text を出力する。
-- :grep <pat> → :copen → :cdo s/old/new/gc | update でプロジェクト横断の一括置換の基点になる。
if vim.fn.executable("rg") == 1 then
	vim.opt.grepprg = "rg --vimgrep --smart-case --hidden --glob '!.git'"
	vim.opt.grepformat = "%f:%l:%c:%m"
	vim.keymap.set("n", "co", ":copen<CR>", { noremap = true, silent = true, desc = "Open quickfix list" })
end
vim.opt.wildmenu = true -- Show completion suggestions at command line mode
vim.opt.conceallevel = 0 -- Show double quotations in json file and so on.
vim.g.mapleader = " " -- Set a space key to a leader.
vim.opt.mouse = "" -- Don't use a mouse.
vim.opt.signcolumn = "yes" -- Always show signcolumn to prevent rattling.
vim.opt.foldlevelstart = 99 -- Open files fully expanded; folds (treesitter foldexpr) are closed manually when needed.
vim.opt.updatetime = 300 -- Fire CursorHold sooner (default 4000ms) for LSP document highlight. Kept >250ms to avoid frequent swap writes.
vim.opt.splitbelow = true -- Open horizontal splits (:split) below the current window.
vim.opt.splitright = true -- Open vertical splits (:vsplit) to the right of the current window.
-- 分割の開閉で周囲ウィンドウの表示テキストが上下にズレないよう、画面上の見た目位置を保つ。
-- 既定は "cursor"（カーソルの相対位置を保つ＝ビューポートはスクロールしうる）。
-- quickfix(:copen) / ターミナル分割 / gitsigns プレビュー等の開閉時に読んでいた行を見失うのを防ぐ。
vim.opt.splitkeep = "screen"
-- quickfix (:cc / :cn / :cp や quickfix ウィンドウ上の <CR>) から飛ぶ際、対象ファイルを
-- 既に表示しているウィンドウが同一タブ内にあればそこへジャンプする。
-- 既定の "uselast" だけだと他ウィンドウを見ずに「直前に使ったウィンドウ」へ読み込むため、
-- 突き合わせ中の分割が差し替わり、同じファイルが 2 枚並ぶことがある。
-- 未表示時のフォールバックとして uselast を残したいので、代入せず append する。
vim.opt.switchbuf:append("useopen")
vim.opt.scrolloff = 8 -- カーソルの上下に常に 8 行の文脈を確保し、画面端への張り付きを防ぐ
vim.opt.sidescrolloff = 8 -- nowrap 時、カーソルの左右に常に 8 桁の文脈を確保する
-- jumplist (<C-o>/<C-i>) / changelist (g;/g,) / alternate-file (<C-^>) / マークジャンプで
-- 戻った際に、カーソル行だけでなく「カーソル行と topline の距離」= 元の画面スクロール位置まで復元する。
-- gd / grr / telescope / quickfix から飛んで戻る往復で、毎回 zz を打ち直す手間を消す。
-- 既定は "clean" (未ロードバッファを jumplist から除く) のため、上書きせず append する。
vim.opt.jumpoptions:append("view")
vim.opt.confirm = true -- 未保存バッファを破棄しうる :q / :e 等でエラーにせず、保存/破棄/キャンセルの確認ダイアログを出す
-- diff の行内アライメントを精緻化する（gitsigns インラインプレビュー / :diffthis 双方に効く）。
-- 既定値を壊さないよう append で足す（重複指定は無視される）。internal は既定で有効。
vim.opt.diffopt:append("linematch:60") -- 変更ハンク内で似た行同士を対応付け直し、本当に変わった行だけを着色（Neovim 0.9+）
vim.opt.diffopt:append("algorithm:histogram") -- 内蔵 diff のアルゴリズムを histogram にし、並べ替えを含む差分でも直感的なマッチにする

-- ----------------------------------------
-- 外部変更ファイルの自動リロード (autoread + :checktime トリガ)
-- ----------------------------------------
vim.opt.autoread = true -- ディスク上で更新されたファイルをバッファへ読み直す
local autoread_group = vim.api.nvim_create_augroup("auto_reload_on_external_change", { clear = true })
-- CursorHoldI を含めない: 挿入モード中のリロード/競合ダイアログは入力を中断してしまう
vim.api.nvim_create_autocmd({ "FocusGained", "BufEnter", "CursorHold" }, {
	group = autoread_group,
	callback = function()
		-- コマンドライン入力中やバッファ種別が特殊なものは対象外
		if vim.fn.mode() == "c" or vim.bo.buftype ~= "" then
			return
		end
		local name = vim.api.nvim_buf_get_name(0)
		if name == "" then
			return
		end
		-- 置換中で一瞬消えているファイルをリロードすると、バッファが
		-- 新規ファイル扱い (BF_NEW) になり以後の :w が E13 で落ちる。
		-- 実在し、書き込みが 500ms 以上静止しているときだけ checktime する。
		local st = vim.uv.fs_stat(name)
		if not st then
			return
		end
		local sec, usec = vim.uv.gettimeofday()
		local age = (sec - st.mtime.sec) + (usec * 1e-6 - st.mtime.nsec * 1e-9)
		if age < 0.5 then
			return
		end
		vim.cmd("checktime %")
	end,
})
-- 外部変更を検知したら軽く通知（無言の差し替えを避ける）
vim.api.nvim_create_autocmd("FileChangedShellPost", {
	group = autoread_group,
	callback = function()
		vim.notify("External change detected on disk", vim.log.levels.INFO)
	end,
})

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
-- Restore the last cursor position when reopening a file.
-- BufReadPost は shada から `"` マーク (バッファを最後に離れた位置) が復元された後に
-- 発火するので、この契機で参照する。永続 undo と合わせて中断→再開の摩擦を減らす。
-- ----------------------------------------
local last_pos_group = vim.api.nvim_create_augroup("restore_last_cursor_pos", { clear = true })
vim.api.nvim_create_autocmd("BufReadPost", {
	group = last_pos_group,
	callback = function(args)
		local buf = args.buf
		-- コミットメッセージ等は常に先頭で開くのが自然なので除外する。
		local ft = vim.bo[buf].filetype
		if ft == "gitcommit" or ft == "gitrebase" then
			return
		end
		local mark = vim.api.nvim_buf_get_mark(buf, '"')
		local line = mark[1]
		if line > 0 and line <= vim.api.nvim_buf_line_count(buf) then
			-- 範囲外行や折り畳み等での失敗を握りつぶして安全に復帰する。
			pcall(vim.api.nvim_win_set_cursor, 0, mark)
		end
	end,
})

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
-- 散文系 filetype の折り返しを読みやすくする（ドキュメント編集の主用途向け）
-- linebreak: 単語境界で折り返す / breakindent: 継続行を字下げに揃える
-- breakindentopt=list:-1: Markdown 箇条書きの折り返しをぶら下げ字下げにする
-- ----------------------------------------
vim.api.nvim_create_augroup("prose_wrap", { clear = true })
vim.api.nvim_create_autocmd("FileType", {
	group = "prose_wrap",
	pattern = { "markdown", "text", "plaintext", "gitcommit" },
	callback = function()
		vim.opt_local.linebreak = true -- 単語の途中で折り返さない
		vim.opt_local.breakindent = true -- 折り返し継続行を元のインデントに揃える
		vim.opt_local.breakindentopt = "list:-1" -- 箇条書き/番号リストの継続行をぶら下げ字下げにする
	end,
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
-- 内蔵ターミナルの UX 整備
-- ----------------------------------------
local term_group = vim.api.nvim_create_augroup("terminal_ux", { clear = true })
vim.api.nvim_create_autocmd("TermOpen", {
	group = term_group,
	callback = function()
		vim.opt_local.number = false -- シェル出力に行番号を出さない
		vim.opt_local.relativenumber = false
		vim.opt_local.signcolumn = "yes" -- サイン列を消して出力の折り返しずれを防ぐ
		vim.cmd("startinsert") -- 開いた瞬間に端末操作モードへ入る
	end,
})
-- terminal-mode からの脱出を簡略化（<Esc><Esc> を <C-\><C-n> の代替に）
vim.keymap.set("t", "<Esc><Esc>", [[<C-\><C-n>]], { desc = "Exit terminal mode" })

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
