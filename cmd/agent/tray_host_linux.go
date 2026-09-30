//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func trayHasDesktopSession() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func trayNotify(title, body string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", title, body).Run()
		return
	}
	trayAlert(title, body)
}

func trayAlert(title, body string) {
	if _, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command("zenity", "--info", "--title="+title, "--text="+body).Run()
		return
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		_ = exec.Command("kdialog", "--title", title, "--msgbox", body).Run()
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, body)
}

func trayOpenPath(path string) error {
	return exec.Command("xdg-open", path).Start()
}

func trayOpenFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

func traySetClipboard(text string) error {
	if _, err := exec.LookPath("wl-copy"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return fmt.Errorf("no clipboard tool (wl-copy/xclip/xsel)")
}

func trayAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "runeverything.desktop")
}

func trayAutostartEnabled() bool {
	_, err := os.Stat(trayAutostartPath())
	return err == nil
}

func trayEnableAutostart(exe string) error {
	path := trayAutostartPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=RunEverything
Comment=RunEverything Agent (tray)
Exec=%s tray
Icon=utilities-terminal
Terminal=false
X-GNOME-Autostart-enabled=true
`, exe)
	return os.WriteFile(path, []byte(body), 0o644)
}

func trayDisableAutostart() error {
	return os.Remove(trayAutostartPath())
}
