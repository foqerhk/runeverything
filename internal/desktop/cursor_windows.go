//go:build windows

package desktop

import (
	"unsafe"
)

var procGetCursorPos = user32.NewProc("GetCursorPos")

type point struct{ X, Y int32 }

type winCursor struct{}

func NewCursorReader() CursorReader { return &winCursor{} }

func (w *winCursor) Cursor() (CursorPos, error) {
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	if sw == 0 || sh == 0 {
		return CursorPos{Visible: true}, nil
	}
	return CursorPos{
		X:       float64(p.X) / float64(sw),
		Y:       float64(p.Y) / float64(sh),
		Visible: true,
	}, nil
}
