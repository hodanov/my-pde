local failures = {}

local function expect(ok, message)
	if not ok then
		table.insert(failures, message)
	end
end

local function read_lockfile()
	local tests_dir = vim.fs.dirname(debug.getinfo(1, "S").source:sub(2))
	local path = vim.fs.joinpath(vim.fs.dirname(tests_dir), "config", "lazy-lock.json")
	local ok, lines = pcall(vim.fn.readfile, path)
	if not ok then
		expect(false, "cannot read " .. path)
		return {}
	end
	return vim.json.decode(table.concat(lines, "\n"))
end

local function expect_plugins_match_lockfile()
	local lock = read_lockfile()
	local git = require("lazy.manage.git")
	for _, plugin in ipairs(require("lazy").plugins()) do
		local locked = lock[plugin.name]
		if not plugin._.installed then
			expect(false, plugin.name .. ": not installed")
		elseif not locked then
			expect(false, plugin.name .. ": missing from lazy-lock.json")
		else
			local head = (git.info(plugin.dir) or {}).commit
			expect(head == locked.commit, ("%s: HEAD %s != locked %s"):format(plugin.name, head, locked.commit))
		end
	end
end

local function expect_all_plugins_load_cleanly()
	local errors = {}
	local notify = vim.notify
	vim.notify = function(msg, level, opts)
		if level == vim.log.levels.ERROR then
			table.insert(errors, msg)
		end
		return notify(msg, level, opts)
	end
	local names = vim.tbl_map(function(plugin)
		return plugin.name
	end, require("lazy").plugins())
	require("lazy").load({ plugins = names })
	vim.wait(100)
	vim.notify = notify
	for _, err in ipairs(errors) do
		expect(false, "error while loading plugins: " .. err)
	end
	expect(vim.v.errmsg == "", "v:errmsg after loading plugins: " .. vim.v.errmsg)
end

local function expect_blink_rust_fuzzy()
	local fuzzy = require("blink.cmp.fuzzy")
	local ready = vim.wait(30 * 1000, function()
		return fuzzy.implementation_type == "rust"
	end)
	expect(ready, "blink.cmp fuzzy implementation is " .. fuzzy.implementation_type .. ", want rust")
end

local function expect_treesitter_highlights()
	local autocmds = vim.api.nvim_get_autocmds({ group = "treesitter_start", event = "FileType" })
	expect(#autocmds > 0, "no FileType autocmds in group treesitter_start")
	for _, autocmd in ipairs(autocmds) do
		local buf = vim.api.nvim_create_buf(false, true)
		vim.api.nvim_set_current_buf(buf)
		local ok, err = pcall(function()
			vim.bo[buf].filetype = autocmd.pattern
		end)
		expect(ok, autocmd.pattern .. ": " .. tostring(err))
		expect(
			vim.treesitter.highlighter.active[buf] ~= nil,
			autocmd.pattern .. ": treesitter highlighter is not active"
		)
	end
	local locks = vim.fn.glob(vim.fn.expand("~/.cache/tree-sitter/lock/*"), false, true)
	expect(#locks == 0, "stale tree-sitter locks: " .. table.concat(locks, ", "))
end

local function expect_blink_binary_installed()
	local lib_path = require("blink.cmp.fuzzy.download.files").lib_path
	expect(vim.uv.fs_stat(lib_path) ~= nil, "blink.cmp prebuilt binary is missing: " .. lib_path)
end

expect(vim.v.errmsg == "", "v:errmsg at startup: " .. vim.v.errmsg)
for _, check in ipairs({
	expect_plugins_match_lockfile,
	expect_blink_binary_installed,
	expect_all_plugins_load_cleanly,
	expect_blink_rust_fuzzy,
	expect_treesitter_highlights,
}) do
	local ok, err = xpcall(check, debug.traceback)
	expect(ok, tostring(err))
end

if #failures > 0 then
	io.stderr:write("smoke test failed:\n  " .. table.concat(failures, "\n  ") .. "\n")
	vim.cmd("cquit 1")
end
io.stdout:write("smoke test passed\n")
vim.cmd("qall!")
