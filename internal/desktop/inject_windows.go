//go:build windows

package desktop

import (
	"fmt"
)

var (
	procSetCursorPos   = user32.NewProc("SetCursorPos")
	procMouseEvent     = user32.NewProc("mouse_event")
	procKeybdEvent     = user32.NewProc("keybd_event")
	procGetSystemMetrics2 = user32.NewProc("GetSystemMetrics")
)

const (
	mouseEventMove     = 0x0001
	mouseEventLeftDown = 0x0002
	mouseEventLeftUp   = 0x0004
	mouseEventRightDown = 0x0008
	mouseEventRightUp  = 0x0010
	mouseEventMiddleDown = 0x0020
	mouseEventMiddleUp = 0x0040
	mouseEventWheel    = 0x0800
	keyEventKeyUp      = 0x0002
)

type winInjector struct {
	screenW, screenH int
	relative         bool
}

func NewInjector() (Injector, error) {
	w, _, _ := procGetSystemMetrics2.Call(0)
	h, _, _ := procGetSystemMetrics2.Call(1)
	if w == 0 {
		w = 1920
	}
	if h == 0 {
		h = 1080
	}
	return &winInjector{screenW: int(w), screenH: int(h)}, nil
}

func (i *winInjector) SetScreenSize(w, h int) {
	if w > 0 {
		i.screenW = w
	}
	if h > 0 {
		i.screenH = h
	}
}

func (i *winInjector) SetRelativeMouse(on bool) { i.relative = on }

func (i *winInjector) Move(x, y float64) error {
	var px, py int
	if m, ok := SelectedMonitorInfo(); ok && m.Width > 0 && m.Height > 0 {
		px = m.X + int(x*float64(m.Width))
		py = m.Y + int(y*float64(m.Height))
	} else {
		px = int(x * float64(i.screenW))
		py = int(y * float64(i.screenH))
	}
	r, _, err := procSetCursorPos.Call(uintptr(px), uintptr(py))
	if r == 0 {
		return fmt.Errorf("SetCursorPos: %v", err)
	}
	return nil
}

func (i *winInjector) MoveRelative(dx, dy float64) error {
	// mouse_event MOVE relative when not ABSOLUTE
	procMouseEvent.Call(mouseEventMove, uintptr(int32(dx)), uintptr(int32(dy)), 0, 0)
	return nil
}

func (i *winInjector) Button(buttons int, down bool) error {
	var flags uintptr
	if buttons&1 != 0 {
		if down {
			flags |= mouseEventLeftDown
		} else {
			flags |= mouseEventLeftUp
		}
	}
	if buttons&2 != 0 {
		if down {
			flags |= mouseEventRightDown
		} else {
			flags |= mouseEventRightUp
		}
	}
	if buttons&4 != 0 {
		if down {
			flags |= mouseEventMiddleDown
		} else {
			flags |= mouseEventMiddleUp
		}
	}
	if flags == 0 {
		return nil
	}
	procMouseEvent.Call(flags, 0, 0, 0, 0)
	return nil
}

func (i *winInjector) Wheel(delta int) error {
	procMouseEvent.Call(mouseEventWheel, 0, 0, uintptr(int32(delta*120)), 0)
	return nil
}

func (i *winInjector) WheelH(delta int) error {
	const mouseEventHWHeel = 0x01000
	procMouseEvent.Call(mouseEventHWHeel, 0, 0, uintptr(int32(delta*120)), 0)
	return nil
}

func (i *winInjector) Key(keyCode int, text string, down bool, modifiers int) error {
	if text != "" && down {
		// UTF-16 SendInput would be ideal; fallback: VkKeyScanA for ASCII
		for _, r := range text {
			if r < 128 {
				vk := byte(r)
				if r >= 'a' && r <= 'z' {
					vk = byte(r - 32)
				}
				procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
				procKeybdEvent.Call(uintptr(vk), 0, keyEventKeyUp, 0)
			}
		}
		return nil
	}
	applyMods := func(down bool) {
		flag := uintptr(0)
		if !down {
			flag = keyEventKeyUp
		}
		if modifiers&1 != 0 {
			procKeybdEvent.Call(0x10, 0, flag, 0)
		}
		if modifiers&2 != 0 {
			procKeybdEvent.Call(0x11, 0, flag, 0)
		}
		if modifiers&4 != 0 {
			procKeybdEvent.Call(0x12, 0, flag, 0)
		}
		if modifiers&8 != 0 {
			procKeybdEvent.Call(0x5B, 0, flag, 0)
		}
	}
	if down {
		applyMods(true)
	}
	flag := uintptr(0)
	if !down {
		flag = keyEventKeyUp
	}
	if keyCode != 0 {
		procKeybdEvent.Call(uintptr(keyCode&0xff), 0, flag, 0)
	}
	if !down {
		applyMods(false)
	}
	return nil
}

func (i *winInjector) Close() error { return nil }

var _ Injector = (*winInjector)(nil)
