//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func trayHasDesktopSession() bool { return true }

func trayNotify(title, body string) {
	script := fmt.Sprintf(`display notification %s with title %s`, appleScriptString(body), appleScriptString(title))
	_ = exec.Command("osascript", "-e", script).Run()
}

func trayAlert(title, body string) {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button 1`, appleScriptString(body), appleScriptString(title))
	_ = exec.Command("osascript", "-e", script).Run()
}

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func trayOpenPath(path string) error {
	return exec.Command("open", path).Start()
}

func trayOpenFolder(path string) error {
	return exec.Command("open", path).Start()
}

func traySetClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func trayAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.foqerhk.runeverything.plist")
}

func trayAutostartEnabled() bool {
	_, err := os.Stat(trayAutostartPath())
	return err == nil
}

func trayEnableAutostart(exe string) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.foqerhk.runeverything</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>tray</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/runeverything-tray.log</string>
  <key>StandardErrorPath</key><string>/tmp/runeverything-tray.log</string>
</dict>
</plist>
`, exe)
	path := trayAutostartPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	return exec.Command("launchctl", "load", path).Run()
}

func trayDisableAutostart() error {
	path := trayAutostartPath()
	_ = exec.Command("launchctl", "unload", path).Run()
	return os.Remove(path)
}
