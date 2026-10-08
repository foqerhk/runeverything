//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/foqerhk/runeverything/internal/winutil"
)

func trayHasDesktopSession() bool { return true }

func trayNotify(title, body string) {
	winutil.NotifyBalloon(title, body)
}

func trayAlert(title, body string) {
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('%s','%s','OK','Information') | Out-Null`,
		escapePS(body), escapePS(title),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}

func trayOpenPath(path string) error {
	return winutil.OpenFile(path)
}

func trayOpenFolder(path string) error {
	return winutil.OpenFolder(path)
}

func traySetClipboard(text string) error {
	return winutil.SetClipboardText(text)
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
