//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "RunEverythingSetup is a Windows installer; build with GOOS=windows.")
	os.Exit(2)
}
