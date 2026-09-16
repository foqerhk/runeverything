//go:build windows

package keepalive

import (
	"fmt"
	"syscall"
)

const (
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	esAwayModeRequired = 0x00000040
)

func platformStart() (func(), error) {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("SetThreadExecutionState")
	r1, _, err := proc.Call(uintptr(esContinuous | esSystemRequired | esAwayModeRequired))
	if r1 == 0 {
		return nil, fmt.Errorf("SetThreadExecutionState: %v", err)
	}
	return func() {
		_, _, _ = proc.Call(uintptr(esContinuous))
	}, nil
}
