package launcher

// WezTerm launches a script in a new WezTerm tab.
type WezTerm struct {
	run CommandRunner
}

// args returns the command-line arguments for wezterm cli spawn.
// Bash is intentionally invoked WITHOUT `-l` (login shell): some interactive
// AI CLIs (e.g. cursor-agent) detect the login-shell context and refuse to
// enter their TUI mode, causing the spawned tab to exit immediately.
func (w *WezTerm) args(cwd, scriptPath string) []string {
	return []string{"cli", "spawn", "--cwd", cwd, "--", "bash", scriptPath}
}

// Launch opens a new WezTerm tab and runs the script.
func (w *WezTerm) Launch(cwd, scriptPath string) error {
	return w.run("wezterm", w.args(cwd, scriptPath)...)
}
