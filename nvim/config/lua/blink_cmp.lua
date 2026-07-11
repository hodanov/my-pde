require("blink.cmp").setup({
	---@module 'blink.cmp'
	---@type blink.cmp.Config
	--
	-- 'default' (recommended) for mappings similar to built-in completions (C-y to accept)
	-- 'super-tab' for mappings similar to vscode (tab to accept)
	-- 'enter' for enter to accept
	-- 'none' for no mappings
	--
	-- All presets have the following mappings:
	-- C-space: Open menu or open docs if already open
	-- C-n/C-p or Up/Down: Select next/previous item
	-- C-e: Hide menu
	-- C-k: Toggle signature help (if signature.enabled = true)
	--
	-- See :h blink-cmp-config-keymap for defining your own keymap
	keymap = {
		preset = "default",
		["<Tab>"] = false,
		["<S-Tab>"] = false,
		-- preset = "super-tab",
		-- ["<CR>"] = { "accept", "fallback" },
	},

	appearance = {
		-- 'mono' (default) for 'Nerd Font Mono' or 'normal' for 'Nerd Font'
		-- Adjusts spacing to ensure icons are aligned
		nerd_font_variant = "mono",
	},

	-- (Default) Only show the documentation popup when manually triggered
	completion = { documentation = { auto_show = true } },

	-- Default list of enabled providers defined so that you can extend it
	-- elsewhere in your config, without redefining it, due to `opts_extend`
	sources = {
		default = { "lazydev", "lsp", "path", "snippets", "buffer" },
		providers = {
			-- Neovim 設定 (Lua) 編集時に vim API 等を上位に出すため lazydev を高スコアで足す。
			lazydev = {
				name = "LazyDev",
				module = "lazydev.integrations.blink",
				score_offset = 100,
			},
		},
	},

	-- コマンドライン (: / 検索) 補完。auto_show = true は打鍵ごとにメニューが出て煩わしかったので
	-- false にし、Tab を押したときだけメニューを表示する。
	-- keymap preset "cmdline" は Tab で挿入/確定、単一候補は自動確定する定番マッピング。
	cmdline = {
		enabled = true,
		keymap = { preset = "cmdline" },
		sources = { "buffer", "cmdline" },
		completion = {
			menu = { auto_show = false },
		},
	},

	-- (Default) Rust fuzzy matcher for typo resistance and significantly better performance
	-- You may use a lua implementation instead by using `implementation = "lua"` or fallback to the lua implementation,
	-- when the Rust fuzzy matcher is not available, by using `implementation = "prefer_rust"`
	--
	-- See the fuzzy documentation for more information
	fuzzy = { implementation = "prefer_rust_with_warning" },
})
