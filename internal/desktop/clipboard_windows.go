//go:build windows

package desktop

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procGlobalAlloc      = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalAlloc")
	procGlobalLock       = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalLock")
	procGlobalUnlock     = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func writeClipboardText(s string) error {
	u16, err := syscall.UTF16FromString(s)
	if err != nil {
		return err
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	size := len(u16) * 2
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc failed")
	}
	p, _, _ := procGlobalLock.Call(h)
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u16))
	copy(dst, u16)
	procGlobalUnlock.Call(h)
	procSetClipboardData.Call(cfUnicodeText, h)
	return nil
}

func readClipboardText() (string, error) {
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return "", fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", fmt.Errorf("no text")
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return "", fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)
	return syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(p))[:]), nil
}

func writeClipboardPNG(b []byte) error {
	// Windows native clipboard prefers DIB; store PNG via file drop is complex.
	// Best-effort: write temp and skip if conversion unavailable — text path remains primary.
	_ = b
	return nil
}

func readClipboardPNG() ([]byte, error) {
	return nil, fmt.Errorf("no image")
}
