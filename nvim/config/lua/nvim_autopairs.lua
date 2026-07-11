-- ----------
-- nvim-autopairs
-- 入力時に対応する閉じ記号を自動挿入し、ペアの内側で Enter を押すと
-- インデント付きで展開する。全 filetype で有効。
-- ----------
require("nvim-autopairs").setup({
	-- treesitter を使い、文字列/コメント内での不要なペア挿入を避ける。
	-- treesitter は lazy=false で常時ロード済みなので有効化して問題ない。
	check_ts = true,
})
