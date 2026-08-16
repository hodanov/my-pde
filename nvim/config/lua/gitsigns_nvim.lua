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

		-- blame（レビュー中に「この行はいつ・なぜ入ったか」を辿る導線）
		-- <leader>gB: ファイル全体の blame をスクロール同期の縦分割で開く。
		--   窓内で s = そのコミットの詳細(メッセージ＋差分)を縦分割 / S = 新規タブ /
		--   r = そのコミット時点で blame をやり直す（履歴を遡って追える）。
		-- <leader>gl: カーソル行だけを float で。full = true でコミットメッセージ全文と
		--   ハンクまで出す。分割を増やさず 1 行だけ確認したいとき用。
		-- diff base（<leader>gb）とは無関係で、change_base の設定に影響されない。
		map("n", "<leader>gB", gs.blame, "Blame current file (scroll-bound split)")
		map("n", "<leader>gl", function()
			gs.blame_line({ full = true })
		end, "Blame current line (full)")
	end,
})

-- ----------------------------------------
-- diff base のトグル（レビュー用）
-- 既定の index（＝未コミット変更のみ）と、ベースブランチとの merge-base
-- （＝このブランチが加えた変更全体 / GitHub の PR diff と同じ 3 点 diff の基準）を切り替える。
-- コミット済みの変更にもサインが付くので、既存の ]c/[c・<leader>gp/<leader>gi が
-- そのままブランチ全体のレビュー導線として使える。
-- change_base は global = true でセッション全体に効かせたいので、バッファローカルな
-- on_attach の中ではなくモジュール末尾（グローバル）に置く。
-- ----------------------------------------
local gs = require("gitsigns")

-- ベースブランチとの分岐点を求める。origin/HEAD → origin/main → origin/master の順に試す。
-- 単に "origin/main" を base に渡すと分岐後に main へ入った他人のコミットまで差分に
-- 混ざるため、必ず merge-base（分岐点コミット）を解決してから渡す。
local function resolve_merge_base()
	for _, ref in ipairs({ "origin/HEAD", "origin/main", "origin/master" }) do
		local res = vim.system({ "git", "merge-base", "HEAD", ref }, { text = true }):wait()
		if res.code == 0 then
			local rev = vim.trim(res.stdout or "")
			if rev ~= "" then
				return rev, ref
			end
		end
	end
	return nil, nil
end

local review_base_on = false
vim.keymap.set("n", "<leader>gb", function()
	if review_base_on then
		gs.change_base(nil, true) -- 既定（index）へ戻す
		review_base_on = false
		vim.notify("gitsigns: diff base -> index", vim.log.levels.INFO)
		return
	end
	local rev, ref = resolve_merge_base()
	if not rev then
		vim.notify("gitsigns: base branch ref not found (git fetch origin?)", vim.log.levels.WARN)
		return
	end
	gs.change_base(rev, true) -- 全バッファ＋以後開くバッファにも適用
	review_base_on = true
	vim.notify(("gitsigns: diff base -> merge-base with %s (%s)"):format(ref, rev:sub(1, 8)), vim.log.levels.INFO)
end, { desc = "Toggle gitsigns diff base: index <-> merge-base with base branch" })

-- ----------------------------------------
-- 変更ハンクをリポジトリ横断で quickfix に集約する（レビューの入口）。
-- 対象は <leader>gb で設定中の diff base に従う:
--   既定(index)        → 未コミットの変更だけ
--   merge-base 切替後  → ブランチが加えた変更全体（＝ PR の差分）
-- ]c/[c はバッファローカルなので「まだ開いていない変更ファイル」へは到達できない。
-- 集約後は ]q / [q でファイルを跨いでハンクを順送りでき、
-- switchbuf の useopen により参照用に開いてある分割は潰れない。
-- gitsigns 側が quickfix ウィンドウまで開く（opts.open 既定 true）。
-- setqflist("all") はアタッチ済みバッファに限らず cwd のリポジトリも対象にするグローバルな
-- 操作なので、<leader>gb と同じくバッファローカルな on_attach ではなくここに置く。
-- キーは既存の <leader>gp / <leader>gi / <leader>gb に揃える（素の gq は組み込みの
-- フォーマット演算子なので潰さない）。
-- ----------------------------------------
vim.keymap.set("n", "<leader>gq", function()
	gs.setqflist("all")
end, { desc = "Send all changed hunks (current diff base) to quickfix" })
