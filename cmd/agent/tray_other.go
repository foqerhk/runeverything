//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func cmdTray() {
	fmt.Fprintln(os.Stderr, i18n.T("tray.only_windows"))
	os.Exit(2)
}
