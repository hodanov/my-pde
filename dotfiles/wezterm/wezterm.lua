local wezterm = require("wezterm")
local config = wezterm.config_builder()

require("appearance")(config)
-- keybindings が config.keys を代入するので、keys を追記するモジュールは必ずこの後
require("keybindings")(config)
require("workspaces")(config)
require("ai-panes")(config)

return config
