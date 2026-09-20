//go:build linux

package desktop

/*
#cgo LDFLAGS: -lX11 -lXtst
#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <X11/extensions/XTest.h>
#include <stdlib.h>

static Display *re_dpy = NULL;

static Display *re_open(void) {
	if (!re_dpy) re_dpy = XOpenDisplay(NULL);
	return re_dpy;
}

static int re_move(int x, int y) {
	Display *d = re_open();
	if (!d) return -1;
	XTestFakeMotionEvent(d, -1, x, y, CurrentTime);
	XFlush(d);
	return 0;
}

static int re_button(int btn, int down) {
	Display *d = re_open();
	if (!d) return -1;
	unsigned int b = 1;
	if (btn == 1) b = 3; // right
	if (btn == 2) b = 2; // middle
	XTestFakeButtonEvent(d, b, down ? True : False, CurrentTime);
	XFlush(d);
	return 0;
}

static int re_wheel(int delta) {
	Display *d = re_open();
	if (!d) return -1;
	unsigned int b = delta > 0 ? 4 : 5;
	int n = delta > 0 ? delta : -delta;
	if (n == 0) n = 1;
	for (int i = 0; i < n && i < 20; i++) {
		XTestFakeButtonEvent(d, b, True, CurrentTime);
		XTestFakeButtonEvent(d, b, False, CurrentTime);
	}
	XFlush(d);
	return 0;
}

static int re_key(int keycode, int down) {
	Display *d = re_open();
	if (!d) return -1;
	// keycode here treated as X keysym if high bits, else hardware keycode
	KeyCode kc;
	if (keycode > 0xff) {
		kc = XKeysymToKeycode(d, (KeySym)keycode);
	} else {
		kc = (KeyCode)keycode;
	}
	if (kc == 0) return -2;
	XTestFakeKeyEvent(d, kc, down ? True : False, CurrentTime);
	XFlush(d);
	return 0;
}
*/
import "C"

import (
	"fmt"
	"os/exec"
)

type x11Injector struct {
	screenW, screenH int
	relative         bool
}

func NewInjector() (Injector, error) {
	return &x11Injector{screenW: 1920, screenH: 1080}, nil
}

func (i *x11Injector) SetScreenSize(w, h int) {
	if w > 0 {
		i.screenW = w
	}
	if h > 0 {
		i.screenH = h
	}
}

func (i *x11Injector) SetRelativeMouse(on bool) { i.relative = on }

func (i *x11Injector) Move(x, y float64) error {
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	if y < 0 {
		y = 0
	}
	if y > 1 {
		y = 1
	}
	px := int(x * float64(i.screenW))
	py := int(y * float64(i.screenH))
	if m, ok := SelectedMonitorInfo(); ok && m.Width > 0 && m.Height > 0 {
		px = m.X + int(x*float64(m.Width))
		py = m.Y + int(y*float64(m.Height))
	}
	if C.re_move(C.int(px), C.int(py)) != 0 {
		return fmt.Errorf("desktop: X11 move failed")
	}
	return nil
}

func (i *x11Injector) MoveRelative(dx, dy float64) error {
	// XTest relative: get current via xdotool or approximate from last; use xdotool mousemove_relative
	cmd := exec.Command("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", int(dx)), fmt.Sprintf("%d", int(dy)))
	_ = cmd.Run()
	return nil
}

func (i *x11Injector) Button(buttons int, down bool) error {
	d := 0
	if down {
		d = 1
	}
	if buttons&1 != 0 && C.re_button(0, C.int(d)) != 0 {
		return fmt.Errorf("desktop: X11 button failed")
	}
	if buttons&2 != 0 && C.re_button(1, C.int(d)) != 0 {
		return fmt.Errorf("desktop: X11 button failed")
	}
	if buttons&4 != 0 && C.re_button(2, C.int(d)) != 0 {
		return fmt.Errorf("desktop: X11 button failed")
	}
	return nil
}

func (i *x11Injector) Wheel(delta int) error {
	if C.re_wheel(C.int(delta)) != 0 {
		return fmt.Errorf("desktop: X11 wheel failed")
	}
	return nil
}

func (i *x11Injector) WheelH(delta int) error {
	btn := "7"
	if delta > 0 {
		btn = "6"
	}
	n := delta
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	for nClick := 0; nClick < n && nClick < 20; nClick++ {
		_ = exec.Command("xdotool", "click", btn).Run()
	}
	return nil
}

func (i *x11Injector) Key(keyCode int, text string, down bool, modifiers int) error {
	if text != "" && down {
		// Best-effort IME commit via xdotool when present
		cmd := exec.Command("xdotool", "type", "--clearmodifiers", "--", text)
		_ = cmd.Run()
		return nil
	}
	d := 0
	if down {
		d = 1
	}
	if modifiers&1 != 0 {
		C.re_key(0xffe1, C.int(d)) // Shift_L keysym
	}
	if modifiers&2 != 0 {
		C.re_key(0xffe3, C.int(d)) // Control_L
	}
	if modifiers&4 != 0 {
		C.re_key(0xffe9, C.int(d)) // Alt_L
	}
	if modifiers&8 != 0 {
		C.re_key(0xffeb, C.int(d)) // Super_L
	}
	if C.re_key(C.int(keyCode), C.int(d)) != 0 {
		return fmt.Errorf("desktop: X11 key failed")
	}
	return nil
}

func (i *x11Injector) Close() error { return nil }

var _ Injector = (*x11Injector)(nil)
