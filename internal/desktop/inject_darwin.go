//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <dlfcn.h>
#include <math.h>
#include <unistd.h>
#include <sys/sysctl.h>

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

// Pixel continuous scroll — line units are ignored by many modern AppKit/WebKit views.
static void re_wheel_px(int dx, int dy) {
	if (dx == 0 && dy == 0) return;
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 2, dy, dx);
	if (!e) return;
	CGEventSetIntegerValueField(e, kCGScrollWheelEventIsContinuous, 1);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void re_wheel(int delta) { re_wheel_px(0, delta); }
static void re_wheel_h(int delta) { re_wheel_px(delta, 0); }

static void re_key(int keycode, int down) {
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

// Arrow/shortcut keys must carry modifier flags on the same event (Spaces, Mission Control).
static void re_key_flags(int keycode, int down, int modifiers) {
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
	CGEventFlags f = 0;
	if (modifiers & 1) f |= kCGEventFlagMaskShift;
	if (modifiers & 2) f |= kCGEventFlagMaskControl;
	if (modifiers & 4) f |= kCGEventFlagMaskAlternate;
	if (modifiers & 8) f |= kCGEventFlagMaskCommand;
	if (f) CGEventSetFlags(e, f);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void *re_sl_sym(const char *a, const char *b) {
	static void *h;
	if (!h) {
		h = dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/SkyLight", RTLD_LAZY);
		if (!h) {
			h = dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/Versions/A/SkyLight", RTLD_LAZY);
		}
		if (!h) h = RTLD_DEFAULT;
	}
	void *s = dlsym(h, a);
	if (!s && b) s = dlsym(h, b);
	if (!s) s = dlsym(RTLD_DEFAULT, a);
	if (!s && b) s = dlsym(RTLD_DEFAULT, b);
	return s;
}

static uint64_t re_cf_u64(CFDictionaryRef d, CFStringRef k) {
	if (!d || !k) return 0;
	CFTypeRef v = CFDictionaryGetValue(d, k);
	if (!v || CFGetTypeID(v) != CFNumberGetTypeID()) return 0;
	int64_t n = 0;
	CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, &n);
	return (uint64_t)n;
}

static uint64_t re_space_id(CFDictionaryRef sp) {
	uint64_t id = re_cf_u64(sp, CFSTR("id64"));
	if (!id) id = re_cf_u64(sp, CFSTR("ManagedSpaceID"));
	if (!id) id = re_cf_u64(sp, CFSTR("id"));
	return id;
}

// Undocumented CGEvent gesture fields (same indices Dock / trackpad use).
static const CGEventField kRE_CGSEventTypeField = (CGEventField)55;
static const CGEventField kRE_CGEventGestureHIDType = (CGEventField)110;
static const CGEventField kRE_CGEventGestureSwipeMotion = (CGEventField)123;
static const CGEventField kRE_CGEventGestureSwipeProgress = (CGEventField)124;
static const CGEventField kRE_CGEventGestureSwipeVelocityX = (CGEventField)129;
static const CGEventField kRE_CGEventGestureSwipeVelocityY = (CGEventField)130;
static const CGEventField kRE_CGEventGesturePhase = (CGEventField)132;
static const uint32_t kRE_IOHIDEventTypeDockSwipe = 23;
static const int kRE_CGSEventDockControl = 30;
static const int kRE_CGSGesturePhaseBegan = 1;
static const int kRE_CGSGesturePhaseChanged = 2;
static const int kRE_CGSGesturePhaseEnded = 4;
static const int kRE_CGGestureMotionHorizontal = 1;

// Post one Dock-swipe phase. sign>0 → next Space (finger left), sign<0 → previous.
static int re_post_dock_swipe(int phase, double sign, double progress, double velocity) {
	CGEventRef ev = CGEventCreate(NULL);
	if (!ev) return -1;
	CGEventSetIntegerValueField(ev, kRE_CGSEventTypeField, kRE_CGSEventDockControl);
	CGEventSetIntegerValueField(ev, kRE_CGEventGestureHIDType, kRE_IOHIDEventTypeDockSwipe);
	CGEventSetIntegerValueField(ev, kRE_CGEventGesturePhase, phase);
	CGEventSetIntegerValueField(ev, kRE_CGEventGestureSwipeMotion, kRE_CGGestureMotionHorizontal);
	CGEventSetDoubleValueField(ev, kRE_CGEventGestureSwipeProgress, sign * progress);
	if (phase == kRE_CGSGesturePhaseEnded) {
		CGEventSetDoubleValueField(ev, kRE_CGEventGestureSwipeVelocityX, sign * velocity);
		CGEventSetDoubleValueField(ev, kRE_CGEventGestureSwipeVelocityY, 0);
	}
	CGEventPost(kCGSessionEventTap, ev);
	CFRelease(ev);
	return 0;
}

// Animated trackpad-like Space switch: progressive Dock swipe (not hard CGS set).
// Moderate end velocity keeps the system slide transition (high velocity = instant cut).
static int re_space_dock_swipe(int delta) {
	if (delta == 0) return -1;
	double sign = delta > 0 ? 1.0 : -1.0;
	static const double steps[] = {0.12, 0.28, 0.48, 0.68, 0.88};
	if (re_post_dock_swipe(kRE_CGSGesturePhaseBegan, sign, 0.0, 0) != 0) return -20;
	usleep(12000);
	for (int i = 0; i < (int)(sizeof(steps) / sizeof(steps[0])); i++) {
		if (re_post_dock_swipe(kRE_CGSGesturePhaseChanged, sign, steps[i], 0) != 0) return -21;
		usleep(28000);
	}
	// ~3 keeps the slide animation; ±400+ is what InstantSpaceSwitcher uses to skip it.
	if (re_post_dock_swipe(kRE_CGSGesturePhaseEnded, sign, 1.0, 3.0) != 0) return -22;
	return 0;
}

// Hard Space switch (no animation) — fallback if Dock swipe is ignored.
static int re_space_hard_set(int delta) {
	if (delta == 0) return -1;
	typedef int (*conn_fn)(void);
	typedef CFArrayRef (*copy_fn)(int);
	typedef void (*set_fn)(int, CFStringRef, uint64_t);
	conn_fn conn = (conn_fn)re_sl_sym("SLSMainConnectionID", "CGSMainConnectionID");
	copy_fn copy = (copy_fn)re_sl_sym("SLSCopyManagedDisplaySpaces", "CGSCopyManagedDisplaySpaces");
	set_fn setsp = (set_fn)re_sl_sym("SLSManagedDisplaySetCurrentSpace", "CGSManagedDisplaySetCurrentSpace");
	if (!conn || !copy || !setsp) return -2;
	int cid = conn();
	CFArrayRef displays = copy(cid);
	if (!displays || CFGetTypeID(displays) != CFArrayGetTypeID()) return -3;
	CFIndex nd = CFArrayGetCount(displays);
	if (nd < 1) {
		CFRelease(displays);
		return -4;
	}
	CFDictionaryRef disp = CFArrayGetValueAtIndex(displays, 0);
	if (!disp || CFGetTypeID(disp) != CFDictionaryGetTypeID()) {
		CFRelease(displays);
		return -5;
	}
	CFStringRef uuid = CFDictionaryGetValue(disp, CFSTR("Display Identifier"));
	if (!uuid || CFGetTypeID(uuid) != CFStringGetTypeID()) {
		CFRelease(displays);
		return -6;
	}
	CFArrayRef spaces = CFDictionaryGetValue(disp, CFSTR("Spaces"));
	if (!spaces || CFGetTypeID(spaces) != CFArrayGetTypeID()) {
		CFRelease(displays);
		return -7;
	}
	CFIndex ns = CFArrayGetCount(spaces);
	if (ns < 2) {
		CFRelease(displays);
		return -8;
	}
	CFDictionaryRef cur = CFDictionaryGetValue(disp, CFSTR("Current Space"));
	uint64_t curID = re_space_id(cur);
	CFIndex idx = -1;
	for (CFIndex i = 0; i < ns; i++) {
		CFDictionaryRef sp = CFArrayGetValueAtIndex(spaces, i);
		if (!sp || CFGetTypeID(sp) != CFDictionaryGetTypeID()) continue;
		if (re_space_id(sp) == curID) {
			idx = i;
			break;
		}
	}
	if (idx < 0) {
		CFRelease(displays);
		return -9;
	}
	CFIndex nxt = idx + (CFIndex)delta;
	if (nxt < 0) nxt = 0;
	if (nxt >= ns) nxt = ns - 1;
	if (nxt == idx) {
		CFRelease(displays);
		return -10;
	}
	CFDictionaryRef dest = CFArrayGetValueAtIndex(spaces, nxt);
	uint64_t destID = re_space_id(dest);
	if (!destID) {
		CFRelease(displays);
		return -11;
	}
	CFRetain(uuid);
	setsp(cid, uuid, destID);
	CFRelease(uuid);
	CFRelease(displays);
	return 0;
}

static int re_space_nudge(int delta) {
	if (delta == 0) return -1;
	// Prefer Dock trackpad swipe (animated). Fall back to hard CGS set.
	if (re_space_dock_swipe(delta) == 0) return 0;
	return re_space_hard_set(delta);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
)

type darwinInjector struct {
	screenW, screenH int
	relative         bool
}

var (
	spaceNudgeMu   sync.Mutex
	lastSpaceNudge time.Time
)

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
		C.re_key_flags(C.int(keyCode), C.int(d), C.int(modifiers))
	}
	if !down {
		pressMod(false)
	}
	return nil
}

func (i *darwinInjector) Close() error { return nil }

// NudgeDesktopSpace switches Mission Control spaces (desktop ↔ fullscreen app)
// the same way a trackpad three-finger swipe does.
//
// Rapid successive nudges (common on WSS·Relay with HOL delay) must NOT start a
// second Dock animated swipe while the first slide is still on-screen — macOS
// leaves Spaces half-switched and input feels "stuck". Use hard-set then.
func NudgeDesktopSpace(delta int) error {
	spaceNudgeMu.Lock()
	defer spaceNudgeMu.Unlock()
	rapid := !lastSpaceNudge.IsZero() && time.Since(lastSpaceNudge) < 500*time.Millisecond
	lastSpaceNudge = time.Now()

	if rapid {
		st := int(C.re_space_hard_set(C.int(delta)))
		if st != 0 {
			return fmt.Errorf("space hard-set delta=%d st=%d (rapid)", delta, st)
		}
		return nil
	}

	// Prefer animated Dock swipe (trackpad-like). Fall back to hard CGS set.
	st := int(C.re_space_dock_swipe(C.int(delta)))
	if st == 0 {
		// Let Mission Control finish the slide before another nudge is allowed
		// to take the animated path (mutex held → serializes concurrent callers).
		time.Sleep(350 * time.Millisecond)
		lastSpaceNudge = time.Now()
		return nil
	}
	st2 := int(C.re_space_hard_set(C.int(delta)))
	if st2 != 0 {
		return fmt.Errorf("space nudge delta=%d dock=%d hard=%d", delta, st, st2)
	}
	fmt.Printf("runeverything: space dock-swipe failed st=%d — used hard-set\n", st)
	return nil
}

// compile-time check
var _ Injector = (*darwinInjector)(nil)

func init() {
	_ = fmt.Sprintf
}
