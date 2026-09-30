//go:build linux

package main

import (
	"fmt"
	"strings"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func trayShowPermissionsPanel() {
	markPermsUIShown()
	rows := collectPermRows()
	var b strings.Builder
	b.WriteString(i18n.T("perm.window_sub"))
	b.WriteString("\n\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "• %s — %s\n  %s\n\n", r.Title, r.Status, r.Desc)
	}
	trayAlert(i18n.T("perm.window_title"), b.String())
}
