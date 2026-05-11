package launcher

import "strings"

// Tmux launches a script in a new tmux window.
type Tmux struct {
	run CommandRunner
}

// shellQuote returns a POSIX shell-safe single-quoted string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// args returns the command-line arguments for tmux new-window.
// Bash is intentionally invoked WITHOUT `-l` (login shell): some interactive
// AI CLIs (e.g. cursor-agent) detect the login-shell context and refuse to
// enter their TUI mode, causing the spawned window to exit immediately.
func (t *Tmux) args(cwd, scriptPath string) []string {
	shellCmd := "bash " + shellQuote(scriptPath)
	return []string{"new-window", "-c", cwd, shellCmd}
}

// Launch opens a new tmux window and runs the script.
func (t *Tmux) Launch(cwd, scriptPath string) error {
	return t.run("tmux", t.args(cwd, scriptPath)...)
}
