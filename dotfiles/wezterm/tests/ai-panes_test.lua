local here = arg[0]:match("^(.*)/[^/]+$") or "."
package.path = here .. "/../?.lua;" .. package.path

local failures = 0

local function report(ok, label, detail)
	if ok then
		print("ok   - " .. label)
	else
		failures = failures + 1
		print("FAIL - " .. label)
		if detail then
			print("       " .. detail)
		end
	end
end

local function assert_eq(actual, expected, label)
	report(actual == expected, label, string.format("expected %q, got %q", tostring(expected), tostring(actual)))
end

local function assert_contains(haystack, needle, label)
	report(haystack:find(needle, 1, true) ~= nil, label, string.format("missing %q", needle))
end

local function assert_absent(haystack, needle, label)
	report(haystack:find(needle, 1, true) == nil, label, string.format("unexpected %q", needle))
end

local wezterm_stub = {
	config_dir = "/tmp/wezterm-config",
	GLOBAL = {},
	action = setmetatable({}, {
		__index = function(_, name)
			return function(args)
				return { action = name, args = args }
			end
		end,
	}),
}

function wezterm_stub.on() end

function wezterm_stub.action_callback(fn)
	return fn
end

function wezterm_stub.format(parts)
	local out = {}
	for _, part in ipairs(parts) do
		if part.Text then
			out[#out + 1] = part.Text
		end
	end
	return table.concat(out)
end

local mux_tree = {}

wezterm_stub.mux = {
	all_windows = function()
		return mux_tree
	end,
	get_pane = function()
		error("pane not found")
	end,
}

package.preload["wezterm"] = function()
	return wezterm_stub
end

local ai_panes = require("ai-panes")

local function fake_pane(spec)
	return {
		pane_id = function()
			return spec.id
		end,
		get_foreground_process_info = function()
			if spec.info == nil then
				error("no process info")
			end
			return spec.info
		end,
		get_foreground_process_name = function()
			if spec.name == nil then
				error("no process name")
			end
			return spec.name
		end,
		get_user_vars = function()
			return spec.vars or {}
		end,
		get_current_working_dir = function()
			return spec.cwd
		end,
		get_progress = function()
			if spec.progress == nil then
				error("get_progress is unavailable")
			end
			return spec.progress
		end,
		get_dimensions = function()
			return { cols = spec.cols or 26 }
		end,
	}
end

local function fake_window(workspace, panes)
	local tab = {
		tab_id = function()
			return 1
		end,
		panes = function()
			return panes
		end,
	}
	return {
		get_workspace = function()
			return workspace
		end,
		tabs = function()
			return { tab }
		end,
	}
end

local docker_nvim = fake_pane({
	id = 1,
	info = {
		name = "/usr/local/bin/docker",
		executable = "/usr/local/bin/docker",
		argv = {
			"docker",
			"container",
			"exec",
			"-it",
			"-w",
			"/Users/hodanov/workspace/hodalog-hugo",
			"nvim-dev",
			"bash",
			"--login",
			"-c",
			'nvim "$@"',
		},
	},
	name = "/usr/local/bin/docker",
	cwd = { file_path = "/Users/hodanov/workspace/hodalog-hugo" },
	progress = { Percentage = 42 },
})

local versioned_claude = fake_pane({
	id = 2,
	info = {
		name = "/Users/hodanov/.local/share/claude/versions/2.1.235",
		executable = "/Users/hodanov/.local/share/claude/versions/2.1.235",
		argv = { "claude" },
	},
	name = "/Users/hodanov/.local/share/claude/versions/2.1.235",
	cwd = "file://host/Users/hodanov/workspace/hodalog-hugo",
	progress = "Indeterminate",
})

local host_nvim = fake_pane({
	id = 3,
	info = { name = "/opt/homebrew/bin/nvim", executable = "/opt/homebrew/bin/nvim", argv = { "nvim" } },
	name = "/opt/homebrew/bin/nvim",
	cwd = { file_path = "/Users/hodanov/workspace/my-pde" },
})

local container_shell = fake_pane({
	id = 4,
	info = {
		name = "/usr/local/bin/docker",
		executable = "/usr/local/bin/docker",
		argv = { "docker", "container", "exec", "-it", "nvim-dev", "bash", "--login" },
	},
	name = "/usr/local/bin/docker",
	cwd = { file_path = "/Users/hodanov/workspace/my-pde" },
})

local sink = fake_pane({
	id = 5,
	info = { name = "/bin/bash", executable = "/bin/bash", argv = { "bash" } },
	name = "/bin/bash",
	vars = { ai_panes_dashboard = "1" },
	cwd = { file_path = "/Users/hodanov/workspace/my-pde" },
	cols = 26,
})

local codex_worktree = fake_pane({
	id = 6,
	info = { name = "/opt/homebrew/bin/codex", executable = "/opt/homebrew/bin/codex", argv = { "codex" } },
	name = "/opt/homebrew/bin/codex",
	cwd = { file_path = "/Users/hodanov/workspace/.worktrees/my-pde/lively-otter/" },
	progress = { Error = 2 },
})

local compose_noise = fake_pane({
	id = 7,
	info = {
		name = "/usr/local/bin/docker",
		executable = "/usr/local/bin/docker",
		argv = { "docker", "compose", "-f", "/Users/hodanov/workspace/my-pde/nvim.dockerfile", "build" },
	},
	name = "/usr/local/bin/docker",
	cwd = { file_path = "/Users/hodanov/workspace/my-pde" },
})

mux_tree = {
	fake_window("my-pde", { host_nvim, container_shell, sink, codex_worktree, compose_noise }),
	fake_window("blog", { docker_nvim, versioned_claude }),
}
wezterm_stub.mux.all_windows = function()
	return mux_tree
end

local rows = ai_panes.collect()

assert_eq(#rows, 4, "collect() keeps only tracked panes")
assert_eq(
	rows[1].ws .. "/" .. rows[1].proc .. "/" .. rows[1].pane_id,
	"blog/nvim/1",
	"docker 越しの nvim を先頭に並べる"
)
assert_eq(
	rows[2].ws .. "/" .. rows[2].proc .. "/" .. rows[2].pane_id,
	"blog/claude/2",
	"バージョン番号の claude を拾う"
)
assert_eq(
	rows[3].ws .. "/" .. rows[3].proc .. "/" .. rows[3].pane_id,
	"my-pde/nvim/3",
	"ホスト直の nvim を拾う"
)
assert_eq(
	rows[4].ws .. "/" .. rows[4].proc .. "/" .. rows[4].pane_id,
	"my-pde/codex/6",
	"種別順が pane_id より優先される"
)
assert_eq(rows[1].project, "hodalog-hugo", "cwd が Url オブジェクトでも project を取れる")
assert_eq(rows[2].project, "hodalog-hugo", "cwd が file:// 文字列でも project を取れる")
assert_eq(rows[4].project, "lively-otter", "末尾スラッシュを落として project を取る")

assert_eq(ai_panes.progress_token(nil), nil, "progress が nil なら進捗なし")
assert_eq(ai_panes.progress_token("None"), nil, "None は進捗なし")
assert_eq(ai_panes.progress_token("Indeterminate"), "busy", "Indeterminate は busy")
assert_eq(ai_panes.progress_token({ Percentage = 42 }), "42%", "Percentage はパーセント表記")
assert_eq(ai_panes.progress_token({ Error = 2 }), "err", "Error は err")
assert_eq(ai_panes.progress_token(42), nil, "想定外の型は進捗なし")

assert_eq(rows[1].progress, "42%", "collect() が Percentage を行に載せる")
assert_eq(rows[2].progress, "busy", "collect() が Indeterminate を行に載せる")
assert_eq(rows[3].progress, nil, "get_progress を持たない wezterm でも行は組める")
assert_eq(rows[4].progress, "err", "collect() が Error を行に載せる")

assert_eq(ai_panes.status_text(rows), " nvim:2 claude:1 codex:1 ", "status_text() が TRACKED_ORDER の順に並べる")
assert_eq(ai_panes.status_text({}), "", "1 本も無ければステータスは空")

local frame = ai_panes.render(rows, { cols = 26, here = "my-pde", selected = 3 })

assert_contains(frame, "\027[?25l", "カーソルを隠す")
assert_contains(frame, "\027[H\027[0J", "毎フレーム先頭から描き直す")
assert_contains(frame, "PANES", "見出しを出す")
assert_contains(frame, "▍blog", "workspace のグループ見出しを出す")
assert_contains(frame, "▍my-pde", "workspace ごとにグループ見出しを出す")
assert_contains(frame, "\027]8;;wezterm-ai-panes://jump/3\007", "行に OSC 8 のジャンプ先を張る")
assert_contains(frame, "▸", "選択カーソルを出す")
assert_contains(frame, "↳ lively-otter", "cwd が workspace 名と違う行だけ補足する")
assert_absent(frame, "↳ my-pde", "cwd が workspace 名と同じなら補足しない")
report(select(2, frame:gsub("●", "")) == 2, "現在の workspace の行だけ ● を出す")
report(select(2, frame:gsub("○", "")) == 2, "他の workspace の行は ○ を出す")
report(select(2, frame:gsub("▸", "")) == 1, "選択カーソルは 1 行だけ")

local reset = "\027[0m"
local green = "\027[38;2;166;227;161m"
local red = "\027[38;2;243;139;168m"

assert_contains(frame, "#1" .. reset .. " " .. green .. "42%" .. reset, "Percentage の行末にトークンを出す")
assert_contains(frame, "#2" .. reset .. " " .. green .. "busy" .. reset, "busy は green で出す")
assert_contains(frame, "#6" .. reset .. " " .. red .. "err" .. reset, "err は red で出す")
assert_contains(frame, "#3" .. reset .. "\027]8;;\007", "進捗の無い行にはトークンを足さない")
report(
	frame:find("busy" .. reset .. "\027]8;;\007", 1, true) ~= nil,
	"トークンは OSC 8 の内側に置いて行全体をクリック可能に保つ"
)

local narrow = ai_panes.render(rows, { cols = 10, here = "my-pde", selected = 3 })
assert_absent(narrow, "busy", "幅が足りない行はトークンを落とす")

local lf_only = frame:gsub("\r\n", "")
assert_absent(lf_only, "\n", "改行はすべて CRLF")

local empty_frame = ai_panes.render({}, { cols = 26, here = "my-pde", selected = nil })
assert_contains(empty_frame, "nothing running", "1 本も無いときの表示")

local wide = ai_panes.render(rows, { cols = 40, here = "my-pde", selected = 3 })
report(wide:find(string.rep("─", 38), 1, true) ~= nil, "区切り線が幅に追従する")

local function widest_visible_line(text)
	local widest = 0
	for line in (text .. "\r\n"):gmatch("(.-)\r\n") do
		local plain = line:gsub("\027%]8;;[^\007]*\007", ""):gsub("\027%[[%d;?]*[a-zA-Z]", "")
		local _, glyphs = plain:gsub("[^\128-\191]", "")
		widest = math.max(widest, glyphs)
	end
	return widest
end

for _, cols in ipairs({ 10, 14, 20, 26, 37 }) do
	local frame_at = ai_panes.render(rows, { cols = cols, here = "my-pde", selected = 3 })
	report(
		widest_visible_line(frame_at) <= cols,
		string.format("幅 %d でどの行も折り返さない", cols),
		string.format("widest=%d", widest_visible_line(frame_at))
	)
end

assert_eq(ai_panes.move_selection(rows, 3, 1), 6, "j で次の行へ進む")
assert_eq(ai_panes.move_selection(rows, 3, -1), 2, "k で前の行へ戻る")
assert_eq(ai_panes.move_selection(rows, 1, -1), 1, "先頭で止まる")
assert_eq(ai_panes.move_selection(rows, 6, 1), 6, "末尾で止まる")
assert_eq(ai_panes.move_selection({}, 3, 1), nil, "行が無ければ選択も無い")

assert_eq(ai_panes.resolve_selection(rows, 6), 6, "生きている選択はそのまま保つ")
assert_eq(ai_panes.resolve_selection(rows, 99), 1, "消えたペインを選んでいたら先頭へ戻す")
assert_eq(ai_panes.resolve_selection({}, 3), nil, "行が空なら選択を空にする")

if failures > 0 then
	print(string.format("%d test(s) failed", failures))
	os.exit(1)
end
print("all tests passed")
