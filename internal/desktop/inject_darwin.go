//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>

static void re_move(double x, double y) {
	CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void re_move_rel(double dx, double dy) {
	CGEventRef loc = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(loc);
	CFRelease(loc);
	re_move(p.x + dx, p.y + dy);
}

static void re_button(int btn, int down) {
	CGEventType typ;
	CGMouseButton mbtn = kCGMouseButtonLeft;
	if (btn == 1) { mbtn = kCGMouseButtonRight; }
	if (btn == 2) { mbtn = kCGMouseButtonCenter; }
	if (down) {
		if (btn == 0) typ = kCGEventLeftMouseDown;
		else if (btn == 1) typ = kCGEventRightMouseDown;
		else typ = kCGEventOtherMouseDown;
	} else {
		if (btn == 0) typ = kCGEventLeftMouseUp;
		else if (btn == 1) typ = kCGEventRightMouseUp;
		else typ = kCGEventOtherMouseUp;
	}
	CGEventRef loc = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(loc);
	CFRelease(loc);
	CGEventRef e = CGEventCreateMouseEvent(NULL, typ, p, mbtn);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void re_wheel(int delta) {
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, delta);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void re_wheel_h(int delta) {
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, 0, delta);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void re_key(int keycode, int down) {
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}
*/
import "C"

import (
	"fmt"
)

type darwinInjector struct {
	screenW, screenH int
	relative         bool
}

func NewInjector() (Injector, error) {
	return &darwinInjector{screenW: 1920, screenH: 1080}, nil
}

func (i *darwinInjector) SetScreenSize(w, h int) {
	if w > 0 {
		i.screenW = w
	}
	if h > 0 {
		i.screenH = h
	}
}

func (i *darwinInjector) SetRelativeMouse(on bool) { i.relative = on }

func (i *darwinInjector) Move(x, y float64) error {
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
	px := x * float64(i.screenW)
	py := y * float64(i.screenH)
	if m, ok := SelectedMonitorInfo(); ok && m.Width > 0 {
		px = float64(m.X) + x*float64(m.Width)
		py = float64(m.Y) + y*float64(m.Height)
	}
	C.re_move(C.double(px), C.double(py))
	return nil
}

func (i *darwinInjector) MoveRelative(dx, dy float64) error {
	C.re_move_rel(C.double(dx), C.double(dy))
	return nil
}

func (i *darwinInjector) Button(buttons int, down bool) error {
	d := 0
	if down {
		d = 1
	}
	if buttons&1 != 0 {
		C.re_button(0, C.int(d))
	}
	if buttons&2 != 0 {
		C.re_button(1, C.int(d))
	}
	if buttons&4 != 0 {
		C.re_button(2, C.int(d))
	}
	return nil
}

func (i *darwinInjector) Wheel(delta int) error {
	C.re_wheel(C.int(delta))
	return nil
}

func (i *darwinInjector) WheelH(delta int) error {
	C.re_wheel_h(C.int(delta))
	return nil
}

func (i *darwinInjector) Key(keyCode int, text string, down bool, modifiers int) error {
	// IME committed text: inject unicode on key-down only
	if text != "" && down {
		typeUTF8Darwin(text)
		return nil
	}
	pressMod := func(down bool) {
		d := 0
		if down {
			d = 1
		}
		if modifiers&1 != 0 { // shift
			C.re_key(56, C.int(d))
		}
		if modifiers&2 != 0 { // ctrl
			C.re_key(59, C.int(d))
		}
		if modifiers&4 != 0 { // opt/alt
			C.re_key(58, C.int(d))
		}
		if modifiers&8 != 0 { // cmd
			C.re_key(55, C.int(d))
		}
	}
	if down {
		pressMod(true)
	}
	d := 0
	if down {
		d = 1
	}
	if keyCode != 0 {
		C.re_key(C.int(keyCode), C.int(d))
	}
	if !down {
		pressMod(false)
	}
	return nil
}

func (i *darwinInjector) Close() error { return nil }

// compile-time check
var _ Injector = (*darwinInjector)(nil)

func init() {
	_ = fmt.Sprintf
}
