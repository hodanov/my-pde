local dir = os.getenv("NVIM_USAGE_DIR")
if not dir or dir == "" then
	return
end

local events_dir = dir .. "/events"
vim.fn.mkdir(events_dir, "p")
if vim.fn.isdirectory(events_dir) == 0 then
	vim.notify("usage_recorder: cannot create " .. events_dir, vim.log.levels.WARN)
	return
end

local pending = {}
local chunk = nil
local last_key_at = 0

local function flush_pending()
	if #pending == 0 then
		return
	end
	local f = io.open(events_dir .. "/" .. os.date("%Y-%m-%d") .. ".jsonl", "a")
	if not f then
		return
	end
	f:write(table.concat(pending, "\n"), "\n")
	f:close()
	pending = {}
end

local function record(event)
	local flush_threshold = 50
	table.insert(pending, vim.json.encode(event))
	if #pending >= flush_threshold then
		flush_pending()
	end
end

local function close_chunk()
	if chunk then
		record(chunk)
		chunk = nil
	end
end

local function mode_family(mode)
	local head = mode:sub(1, 1)
	if head == "n" then
		return "normal"
	end
	if head == "v" or head == "V" or head == "\22" then
		return "visual"
	end
	return nil
end

local function snapshot_keymaps()
	local seen, keymaps = {}, {}
	local function collect(mode, maps, buffer_local)
		for _, m in ipairs(maps) do
			local lhs = vim.fn.keytrans(m.lhsraw or m.lhs)
			local key = table.concat({ mode, lhs, tostring(buffer_local) }, "\0")
			if not seen[key] and not lhs:find("^<Plug>") and not lhs:find("^<SNR>") then
				seen[key] = true
				table.insert(keymaps, {
					mode = mode,
					lhs = lhs,
					desc = m.desc,
					rhs = m.rhs,
					buffer_local = buffer_local,
				})
			end
		end
	end
	for _, mode in ipairs({ "n", "x", "o" }) do
		collect(mode, vim.api.nvim_get_keymap(mode), false)
		for _, buf in ipairs(vim.api.nvim_list_bufs()) do
			if vim.api.nvim_buf_is_loaded(buf) then
				collect(mode, vim.api.nvim_buf_get_keymap(buf, mode), true)
			end
		end
	end
	local f = io.open(dir .. "/keymaps.json", "w")
	if f then
		f:write(vim.json.encode({ nvim = tostring(vim.version()), keymaps = keymaps }))
		f:close()
	end
end

vim.on_key(function(_, typed)
	local idle_split_ms = 5000
	local max_chunk_keys = 200
	if typed == "" then
		return
	end
	local family = mode_family(vim.api.nvim_get_mode().mode)
	if not family then
		return
	end
	local now = vim.uv.now()
	if chunk and now - last_key_at > idle_split_ms then
		close_chunk()
	end
	last_key_at = now
	if not chunk then
		chunk = { kind = "keys", ts = os.time(), family = family, ft = vim.bo.filetype, keys = {} }
	end
	table.insert(chunk.keys, vim.fn.keytrans(typed))
	if #chunk.keys >= max_chunk_keys then
		close_chunk()
	end
end, vim.api.nvim_create_namespace("usage_recorder"))

local group = vim.api.nvim_create_augroup("usage_recorder", { clear = true })

vim.api.nvim_create_autocmd("ModeChanged", {
	group = group,
	callback = function()
		if chunk and mode_family(vim.v.event.new_mode) ~= chunk.family then
			close_chunk()
		end
	end,
})

vim.api.nvim_create_autocmd("CmdlineLeave", {
	group = group,
	callback = function()
		if vim.v.event.abort then
			return
		end
		local cmdtype = vim.v.event.cmdtype
		if cmdtype == "/" or cmdtype == "?" then
			record({ kind = "search", ts = os.time() })
		elseif cmdtype == ":" then
			local ok, parsed = pcall(vim.api.nvim_parse_cmd, vim.fn.getcmdline(), {})
			if ok then
				record({ kind = "cmd", ts = os.time(), name = parsed.cmd })
			end
		end
	end,
})

vim.api.nvim_create_autocmd("VimLeavePre", {
	group = group,
	callback = function()
		close_chunk()
		flush_pending()
		snapshot_keymaps()
	end,
})
