return {
	-- File manager
	"lambdalisue/fern.vim",
	-- Fuzzy finder
	{
		"nvim-telescope/telescope-file-browser.nvim",
		dependencies = {
			"nvim-lua/plenary.nvim",
			"nvim-telescope/telescope.nvim",
		},
		config = function()
			require("telescope_config")
		end,
	},
	{ -- colorscheme
		"catppuccin/nvim",
		config = function()
			vim.cmd("colorscheme catppuccin-mocha")
		end,
	},
	{ -- Appearance
		"lukas-reineke/indent-blankline.nvim",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
	},
	{ -- Appearance
		"nvim-lualine/lualine.nvim",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
		config = function()
			require("nvim_lualine")
		end,
	},
	{ -- Appearance
		"lewis6991/gitsigns.nvim",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
		config = function()
			require("gitsigns_nvim")
		end,
	},
	{ -- Appearance
		"nvim-treesitter/nvim-treesitter",
		lazy = false,
		build = ":TSUpdate",
		config = function()
			require("nvim_treesitter")
		end,
	},
	{ -- Configurations for Nvim LSP
		"neovim/nvim-lspconfig",
		lazy = false,
	},
	{ -- Formatt runner
		"stevearc/conform.nvim",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
		config = function()
			require("conform_nvim")
		end,
	},
	{ -- Lint runner
		"mfussenegger/nvim-lint",
		config = function()
			require("nvim_lint")
		end,
	},
	{ -- Auto completion
		"saghen/blink.cmp",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
		version = "1.*",
		config = function()
			require("blink_cmp")
		end,
	},
	{ -- Debug Adapter Protocol
		"mfussenegger/nvim-dap",
		lazy = true,
		event = { "BufRead", "BufNewFile" },
		dependencies = {
			{
				"leoluz/nvim-dap-go",
				lazy = true,
				event = { "BufRead", "BufNewFile" },
			},
			{
				"mfussenegger/nvim-dap-python",
				lazy = true,
				event = { "BufRead", "BufNewFile" },
			},
			{
				"theHamsta/nvim-dap-virtual-text",
				lazy = true,
				event = { "BufRead", "BufNewFile" },
			},
		},
		config = function()
			require("nvim_dap")
		end,
	},
}
