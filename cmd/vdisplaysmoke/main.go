//go:build darwin

// Command vdisplaysmoke exercises re-vdisplay + ListMonitors CGDirectDisplayID wiring.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
)

func main() {
	mode := "8k"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	id, w, h, err := desktop.EnsureVirtual(mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "EnsureVirtual: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("virtual cgID=%d logical=%dx%d\n", id, w, h)
	time.Sleep(500 * time.Millisecond)
	mons, err := desktop.ListMonitors()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListMonitors: %v\n", err)
		os.Exit(1)
	}
	found := false
	for _, m := range mons {
		fmt.Printf("  monitor id=%d name=%q %dx%d primary=%v virtual=%v\n",
			m.ID, m.Name, m.Width, m.Height, m.Primary, desktop.IsVirtualDisplay(m.ID))
		if m.ID == int(id) {
			found = true
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "virtual display %d not in ListMonitors\n", id)
		os.Exit(2)
	}
	if mode == "16k" {
		fw, fh, ok := desktop.VirtualFramebuffer(int(id))
		if !ok || fw < 15000 || fh < 8000 {
			fmt.Fprintf(os.Stderr, "16k fb unexpected: %dx%d ok=%v\n", fw, fh, ok)
			os.Exit(3)
		}
		fmt.Printf("16k desktop fb=%dx%d (encode ceiling ≤16K full-blood)\n", fw, fh)
	}
	_ = desktop.DestroyAllVirtual()
	fmt.Println("OK")
}
