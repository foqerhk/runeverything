// Gesture inject self-test matching KoKo「更多 → 手势说明」Agent-side mappings.
//go:build darwin

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
)

func step(name string, fn func() error) {
	fmt.Printf("== %s\n", name)
	if err := fn(); err != nil {
		fmt.Printf("FAIL %s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("OK  %s\n", name)
	time.Sleep(250 * time.Millisecond)
}

func main() {
	inj, err := desktop.NewInjector()
	if err != nil {
		fmt.Fprintf(os.Stderr, "injector: %v\n", err)
		os.Exit(1)
	}
	defer inj.Close()

	step("1 tap = left click", func() error {
		_ = inj.Move(0.5, 0.5)
		_ = inj.Button(1, true)
		return inj.Button(1, false)
	})
	step("2 double-tap = double-click", func() error {
		_ = inj.Move(0.5, 0.45)
		for i := 0; i < 2; i++ {
			_ = inj.Button(1, true)
			_ = inj.Button(1, false)
			time.Sleep(80 * time.Millisecond)
		}
		return nil
	})
	step("3 slide = move cursor", func() error { return inj.MoveRelative(40, 20) })
	step("4 hold-drag = select", func() error {
		_ = inj.Move(0.4, 0.5)
		_ = inj.Button(1, true)
		_ = inj.MoveRelative(80, 0)
		return inj.Button(1, false)
	})
	step("5 two-finger scroll = wheel", func() error {
		_ = inj.Wheel(40)
		return inj.WheelH(-20)
	})
	step("6 two-finger long-press = right click", func() error {
		_ = inj.Move(0.55, 0.55)
		_ = inj.Button(2, true)
		return inj.Button(2, false)
	})
	step("7 three-finger swipe Spaces +1", func() error { return desktop.NudgeDesktopSpace(1) })
	step("8 three-finger swipe Spaces -1", func() error { return desktop.NudgeDesktopSpace(-1) })
	fmt.Println("ALL GESTURE INJECT STEPS OK")
}
