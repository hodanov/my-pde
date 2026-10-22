package domain

import (
	"fmt"
	"strings"
)

// BuildScript returns the contents of a bash script that runs the AI CLI with
// the given prompt and then deletes itself. The prompt is shell-quoted and
// passed after "--" so that arbitrary user input can neither break out of the
// command nor be parsed as a CLI option. scriptPath is the path the script will
// be written to, used for the self-deleting rm line.
func BuildScript(cli, prompt, scriptPath string) string {
	return fmt.Sprintf("#!/bin/bash\n%s -- %s\nrm -f %s\n", cli, ShellQuote(prompt), ShellQuote(scriptPath))
}

// ShellQuote returns a POSIX shell-safe single-quoted string.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
