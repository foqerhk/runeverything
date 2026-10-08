//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdint.h>

static void re_cursor(double *x, double *y, int *visible) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
	*visible = 1;
}

static int re_display_count(void) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	return (int)count;
}

// Fill ids[0..*n) with active CGDirectDisplayIDs. *n is capacity in/out.
static void re_display_ids(uint32_t *ids, int *n) {
	if (!ids || !n || *n <= 0) { if (n) *n = 0; return; }
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	if (count == 0) { *n = 0; return; }
	if (count > (uint32_t)*n) count = (uint32_t)*n;
	CGGetActiveDisplayList(count, ids, &count);
	*n = (int)count;
}

static void re_display_bounds_id(uint32_t id, double *x, double *y, double *w, double *h, int *ok) {
	if (id == 0 || id == kCGNullDirectDisplay) { *ok = 0; return; }
	CGRect r = CGDisplayBounds(id);
	if (r.size.width <= 0 || r.size.height <= 0) { *ok = 0; return; }
	*x = r.origin.x; *y = r.origin.y; *w = r.size.width; *h = r.size.height;
	*ok = 1;
}

static uint32_t re_main_display_id(void) {
	return CGMainDisplayID();
}

static void re_type_utf8(const char *utf8) {
	if (!utf8) return;
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
	// SelectedMonitor is CGDirectDisplayID on Darwin.
	id := SelectedMonitor()
	var ox, oy, w, h C.double
	var ok C.int
	if id > 0 {
		C.re_display_bounds_id(C.uint32_t(id), &ox, &oy, &w, &h, &ok)
	}
	if ok == 0 || w <= 0 || h <= 0 {
		C.re_display_bounds_id(C.re_main_display_id(), &ox, &oy, &w, &h, &ok)
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
	if n > 32 {
		n = 32
	}
	ids := make([]C.uint32_t, n)
	cn := C.int(n)
	C.re_display_ids(&ids[0], &cn)
	n = int(cn)
	out := make([]Monitor, 0, n)
	mainID := uint32(C.re_main_display_id())
	vmap := VirtualDisplayIDs()
	for i := 0; i < n; i++ {
		id := uint32(ids[i])
		var x, y, w, h C.double
		var ok C.int
		C.re_display_bounds_id(C.uint32_t(id), &x, &y, &w, &h, &ok)
		if ok == 0 {
			continue
		}
		name := fmt.Sprintf("Display %d", i+1)
		pw, ph := int(w), int(h)
		if mode, ok := vmap[id]; ok {
			name = fmt.Sprintf("Virtual %s", mode)
			// Report framebuffer pixels (not HiDPI logical) so OPEN/encode can
			// request full 8K/16K — CGDisplayBounds is points for these profiles.
			if fw, fh, fok := VirtualFramebuffer(int(id)); fok && fw > 0 && fh > 0 {
				pw, ph = fw, fh
			}
		}
		out = append(out, Monitor{
			ID:      int(id),
			Name:    name,
			Width:   pw,
			Height:  ph,
			X:       int(x),
			Y:       int(y),
			Primary: id == mainID,
		})
	}
	if len(out) == 0 {
		return []Monitor{{ID: int(mainID), Name: "Main", Width: 1920, Height: 1080, Primary: true}}, nil
	}
	return out, nil
}

func typeUTF8Darwin(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.re_type_utf8(cs)
}
