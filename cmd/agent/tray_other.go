//go:build !windows

package main

import (
	"fmt"
	"os"
)

func cmdTray() {
	fmt.Fprintln(os.Stderr, "tray mode is only available on Windows; use: runeverything run")
	os.Exit(2)
}
