//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>

static void re_cursor(double *x, double *y, int *visible) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
	*visible = 1;
}

static void re_display_bounds(int idx, double *x, double *y, double *w, double *h, int *ok) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	if (count == 0 || idx < 0 || (uint32_t)idx >= count) { *ok = 0; return; }
	CGDirectDisplayID ids[16];
	if (count > 16) count = 16;
	CGGetActiveDisplayList(count, ids, &count);
	CGRect r = CGDisplayBounds(ids[idx]);
	*x = r.origin.x; *y = r.origin.y; *w = r.size.width; *h = r.size.height;
	*ok = 1;
}

static int re_display_count(void) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	return (int)count;
}

static void re_type_utf8(const char *utf8) {
	if (!utf8) return;
	// UniChar buffer
	CFStringRef s = CFStringCreateWithCString(NULL, utf8, kCFStringEncodingUTF8);
	if (!s) return;
	CFIndex n = CFStringGetLength(s);
	if (n <= 0 || n > 64) { CFRelease(s); return; }
	UniChar buf[64];
	CFStringGetCharacters(s, CFRangeMake(0, n), buf);
	CFRelease(s);
	CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)0, true);
	CGEventKeyboardSetUnicodeString(keyDown, (UniCharCount)n, buf);
	CGEventPost(kCGHIDEventTap, keyDown);
	CFRelease(keyDown);
	CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)0, false);
	CGEventKeyboardSetUnicodeString(keyUp, (UniCharCount)n, buf);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyUp);
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type darwinCursor struct{}

func NewCursorReader() CursorReader { return &darwinCursor{} }

func (d *darwinCursor) Cursor() (CursorPos, error) {
	var x, y C.double
	var vis C.int
	C.re_cursor(&x, &y, &vis)
	// normalize against selected monitor bounds
	idx := SelectedMonitor()
	var ox, oy, w, h C.double
	var ok C.int
	C.re_display_bounds(C.int(idx), &ox, &oy, &w, &h, &ok)
	if ok == 0 || w <= 0 || h <= 0 {
		// fallback main display
		C.re_display_bounds(0, &ox, &oy, &w, &h, &ok)
	}
	if ok == 0 || w <= 0 || h <= 0 {
		return CursorPos{Visible: vis != 0}, fmt.Errorf("no display")
	}
	nx := (float64(x) - float64(ox)) / float64(w)
	ny := (float64(y) - float64(oy)) / float64(h)
	if nx < 0 {
		nx = 0
	}
	if nx > 1 {
		nx = 1
	}
	if ny < 0 {
		ny = 0
	}
	if ny > 1 {
		ny = 1
	}
	return CursorPos{X: nx, Y: ny, Visible: vis != 0}, nil
}

func listMonitorsCG() ([]Monitor, error) {
	n := int(C.re_display_count())
	if n <= 0 {
		n = 1
	}
	out := make([]Monitor, 0, n)
	for i := 0; i < n; i++ {
		var x, y, w, h C.double
		var ok C.int
		C.re_display_bounds(C.int(i), &x, &y, &w, &h, &ok)
		if ok == 0 {
			continue
		}
		out = append(out, Monitor{
			ID: i, Name: fmt.Sprintf("Display %d", i+1),
			Width: int(w), Height: int(h), X: int(x), Y: int(y),
			Primary: i == 0,
		})
	}
	if len(out) == 0 {
		return []Monitor{{ID: 0, Name: "Main", Width: 1920, Height: 1080, Primary: true}}, nil
	}
	return out, nil
}

func typeUTF8Darwin(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.re_type_utf8(cs)
}
