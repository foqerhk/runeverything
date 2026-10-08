//go:build !darwin

package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func trayApplyAppIcon() {}

func trayCloseQRWindow() {}

func trayShowStatusWindow() {
	trayShowTextDialog(i18n.T("ui.status_title"), statusReportText())
}

func trayShowHelpWindow() {
	trayShowTextDialog(i18n.T("ui.help_title"), helpReportText())
}

func trayShowAboutWindow() {
	msg, url, newer := checkForUpdate()
	body := aboutVersionText() + "\n\n" + msg
	if newer && url != "" {
		body += "\n\n" + url
		openUpdateURL(url)
	}
	trayShowTextDialog(i18n.T("ui.about_title"), body)
}

func trayShowConfigWindow() {
	relay, pub, manual, share := configFormDefaults()
	// Best-effort: show current values; editing via CLI on non-macOS for now.
	body := fmt.Sprintf("%s\n%s\n%s=%v\n%s=%v\n\n%s",
		i18n.T("ui.config_relay")+": "+relay,
		i18n.T("ui.config_public")+": "+pub,
		i18n.T("ui.config_manual"), manual,
		i18n.T("ui.config_share"), share,
		i18n.T("ui.config_cli_hint"))
	_ = strconv.FormatBool(share)
	trayShowTextDialog(i18n.T("ui.config_title"), body)
}

func trayShowTextDialog(title, body string) {
	switch runtime.GOOS {
	case "linux":
		_ = exec.Command("zenity", "--info", "--title="+title, "--text="+body, "--no-wrap").Start()
	case "windows":
		trayAlert(title, body)
	default:
		trayAlert(title, body)
	}
}
