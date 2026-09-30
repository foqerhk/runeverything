//go:build !windows && !darwin && !linux

package main

import (
	"fmt"
	"os"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func cmdTray() {
	fmt.Fprintln(os.Stderr, i18n.T("tray.unsupported"))
	os.Exit(2)
}
