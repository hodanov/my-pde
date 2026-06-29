// Package launchd generates and installs the launchd plist that runs ai-bridge.
package launchd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ai-bridge.daemon</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>daemon</string>
    </array>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>{{.Path}}</string>
        <key>AI_BRIDGE_CLI</key>
        <string>{{.CLI}}</string>
        <key>AI_BRIDGE_LAUNCHER</key>
        <string>{{.Launcher}}</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/tmp/ai-bridge-daemon.log</string>

    <key>StandardErrorPath</key>
    <string>/tmp/ai-bridge-daemon.log</string>
</dict>
</plist>
`

// PlistData holds the values used to generate a launchd plist.
type PlistData struct {
	BinaryPath string
	Path       string
	CLI        string
	Launcher   string
}

// generatePlist creates a launchd plist XML string.
func generatePlist(binaryPath, path, cli, launcherName string) (string, error) {
	tmpl, parseErr := template.New("plist").Parse(plistTemplate)
	if parseErr != nil {
		return "", fmt.Errorf("parse plist template: %w", parseErr)
	}

	var buf bytes.Buffer
	data := PlistData{
		BinaryPath: binaryPath,
		Path:       path,
		CLI:        cli,
		Launcher:   launcherName,
	}
	if executeErr := tmpl.Execute(&buf, data); executeErr != nil {
		return "", fmt.Errorf("execute plist template: %w", executeErr)
	}
	return buf.String(), nil
}

const plistName = "com.ai-bridge.daemon.plist"

// LaunchctlRunner executes launchctl commands.
type LaunchctlRunner func(args ...string) error

// defaultLaunchctl runs launchctl via os/exec.
func defaultLaunchctl(args ...string) error {
	return exec.Command("launchctl", args...).Run()
}

// Install writes the plist to ~/Library/LaunchAgents/ and loads it via launchctl.
func Install(binaryPath, cli, launcherName string) error {
	home, homeDirErr := os.UserHomeDir()
	if homeDirErr != nil {
		return fmt.Errorf("cannot determine home directory: %w", homeDirErr)
	}
	dstDir := filepath.Join(home, "Library", "LaunchAgents")
	return installTo(binaryPath, os.Getenv("PATH"), cli, launcherName, dstDir, defaultLaunchctl)
}

// installTo writes the plist to dstDir and loads it via the provided runner.
func installTo(binaryPath, path, cli, launcherName, dstDir string, launchctl LaunchctlRunner) error {
	content, genPlistErr := generatePlist(binaryPath, path, cli, launcherName)
	if genPlistErr != nil {
		return genPlistErr
	}

	if mkdirErr := os.MkdirAll(dstDir, 0o755); mkdirErr != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", mkdirErr)
	}

	dst := filepath.Join(dstDir, plistName)
	if writeErr := os.WriteFile(dst, []byte(content), 0o644); writeErr != nil {
		return fmt.Errorf("write plist: %w", writeErr)
	}
	fmt.Printf("Installed: %s\n", dst)

	// Unload existing (ignore error if not loaded).
	_ = launchctl("unload", dst)

	if loadErr := launchctl("load", dst); loadErr != nil {
		return fmt.Errorf("launchctl load: %w", loadErr)
	}
	fmt.Println("Loaded: com.ai-bridge.daemon")
	return nil
}
