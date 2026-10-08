//go:build darwin

package idemirror

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework CoreGraphics
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>

static int re_ide_trusted(void) {
	return AXIsProcessTrusted() ? 1 : 0;
}

// Bring the app (and the window whose title contains hint) to the front.
// Returns -1 when the app is not running, 1 when a matching window was raised, 0 otherwise.
static int re_ide_raise(int pid, const char *hint) {
	@autoreleasepool {
		NSRunningApplication *app = pid > 0 ? [NSRunningApplication runningApplicationWithProcessIdentifier:pid] : nil;
		if (!app) return -1;
		AXUIElementRef ax = AXUIElementCreateApplication(app.processIdentifier);
		int raised = 0;
		if (hint && hint[0]) {
			NSString *needle = [NSString stringWithUTF8String:hint];
			CFArrayRef wins = NULL;
			if (AXUIElementCopyAttributeValue(ax, kAXWindowsAttribute, (CFTypeRef *)&wins) == kAXErrorSuccess && wins) {
				for (CFIndex i = 0; i < CFArrayGetCount(wins) && !raised; i++) {
					AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(wins, i);
					CFTypeRef title = NULL;
					if (AXUIElementCopyAttributeValue(w, kAXTitleAttribute, &title) == kAXErrorSuccess && title) {
						if (CFGetTypeID(title) == CFStringGetTypeID() && [(__bridge NSString *)title containsString:needle]) {
							AXUIElementSetAttributeValue(w, kAXMainAttribute, kCFBooleanTrue);
							AXUIElementPerformAction(w, kAXRaiseAction);
							raised = 1;
						}
						CFRelease(title);
					}
				}
				CFRelease(wins);
			}
		}
		AXUIElementSetAttributeValue(ax, kAXFrontmostAttribute, kCFBooleanTrue);
		CFRelease(ax);
		[app activateWithOptions:NSApplicationActivateAllWindows];
		return raised;
	}
}

static int re_ide_frontmost_pid(void) {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		return app ? app.processIdentifier : 0;
	}
}

static char *re_ide_clip_get(void) {
	@autoreleasepool {
		NSString *s = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
		if (!s) return NULL;
		return strdup([s UTF8String]);
	}
}

static void re_ide_clip_set(const char *text) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		[pb setString:[NSString stringWithUTF8String:text] forType:NSPasteboardTypeString];
	}
}

static NSString *re_ax_string(AXUIElementRef e, CFStringRef attr) {
	CFTypeRef v = NULL;
	if (AXUIElementCopyAttributeValue(e, attr, &v) != kAXErrorSuccess || !v) return nil;
	if (CFGetTypeID(v) != CFStringGetTypeID()) { CFRelease(v); return nil; }
	return (__bridge_transfer NSString *)v;
}

// Visible label of a button: its title/description, else the text of its children.
static NSString *re_ax_label(AXUIElementRef e) {
	NSString *t = re_ax_string(e, kAXTitleAttribute);
	if (t.length == 0) t = re_ax_string(e, kAXDescriptionAttribute);
	if (t.length == 0) {
		CFArrayRef kids = NULL;
		if (AXUIElementCopyAttributeValue(e, kAXChildrenAttribute, (CFTypeRef *)&kids) == kAXErrorSuccess && kids) {
			NSMutableString *acc = [NSMutableString string];
			for (CFIndex i = 0; i < CFArrayGetCount(kids) && i < 8; i++) {
				AXUIElementRef k = (AXUIElementRef)CFArrayGetValueAtIndex(kids, i);
				NSString *v = re_ax_string(k, kAXValueAttribute);
				if (v.length == 0) v = re_ax_string(k, kAXTitleAttribute);
				if (v.length) [acc appendString:v];
			}
			CFRelease(kids);
			t = acc;
		}
	}
	return [t stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
}

static int re_ax_pressable(AXUIElementRef e) {
	CFArrayRef names = NULL;
	if (AXUIElementCopyActionNames(e, &names) != kAXErrorSuccess || !names) return 0;
	int ok = CFArrayContainsValue(names, CFRangeMake(0, CFArrayGetCount(names)), kAXPressAction) ? 1 : 0;
	CFRelease(names);
	return ok;
}

// Electron exposes many controls (e.g. Cursor's review bar) as pressable AXGroups
// wrapping a static text, not as AXButtons.
static int re_ax_press_walk(AXUIElementRef e, NSString *want, int depth, int *budget) {
	if (depth > 90 || (*budget)-- <= 0) return 0;
	NSString *role = re_ax_string(e, kAXRoleAttribute);
	BOOL control = [role isEqualToString:(NSString *)kAXButtonRole] || [role isEqualToString:@"AXLink"] ||
		[role isEqualToString:(NSString *)kAXGroupRole];
	if (control && re_ax_pressable(e) && [re_ax_label(e) isEqualToString:want]) {
		return AXUIElementPerformAction(e, kAXPressAction) == kAXErrorSuccess ? 1 : 0;
	}
	CFArrayRef kids = NULL;
	int hit = 0;
	if (AXUIElementCopyAttributeValue(e, kAXChildrenAttribute, (CFTypeRef *)&kids) == kAXErrorSuccess && kids) {
		for (CFIndex i = 0; i < CFArrayGetCount(kids) && !hit; i++) {
			hit = re_ax_press_walk((AXUIElementRef)CFArrayGetValueAtIndex(kids, i), want, depth + 1, budget);
		}
		CFRelease(kids);
	}
	return hit;
}

// Press the first button labelled title in the app's focused window (Electron needs
// AXManualAccessibility to expose its DOM). Returns 1 when pressed.
static int re_ide_press(int pid, const char *title) {
	@autoreleasepool {
		AXUIElementRef app = AXUIElementCreateApplication(pid);
		AXUIElementSetAttributeValue(app, CFSTR("AXManualAccessibility"), kCFBooleanTrue);
		NSString *want = [NSString stringWithUTF8String:title];
		int hit = 0;
		for (int attempt = 0; attempt < 3 && !hit; attempt++) {
			if (attempt > 0) usleep(250000);
			CFTypeRef win = NULL;
			if (AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute, &win) == kAXErrorSuccess && win) {
				int budget = 60000;
				hit = re_ax_press_walk((AXUIElementRef)win, want, 0, &budget);
				CFRelease(win);
			}
		}
		CFRelease(app);
		return hit;
	}
}

static void re_ide_key(int keycode, int cmd) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	CGEventRef down = CGEventCreateKeyboardEvent(src, (CGKeyCode)keycode, true);
	CGEventRef up = CGEventCreateKeyboardEvent(src, (CGKeyCode)keycode, false);
	CGEventFlags f = 0;
	if (cmd & 1) f |= kCGEventFlagMaskCommand;
	if (cmd & 2) f |= kCGEventFlagMaskShift;
	CGEventSetFlags(down, f);
	CGEventSetFlags(up, f);
	CGEventPost(kCGHIDEventTap, down);
	usleep(15000);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	if (src) CFRelease(src);
}
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"
)

const (
	keyV      = 9
	keyReturn = 36
)

type darwinDesk struct{}

func newDesk() desk { return darwinDesk{} }

func (darwinDesk) Trusted() bool { return C.re_ide_trusted() == 1 }

func (darwinDesk) Raise(pid int, titleHint string) error {
	h := C.CString(titleHint)
	defer C.free(unsafe.Pointer(h))
	if C.re_ide_raise(C.int(pid), h) < 0 {
		return errors.New("app not running")
	}
	return nil
}

func (darwinDesk) Frontmost() int { return int(C.re_ide_frontmost_pid()) }

func (darwinDesk) PressButton(pid int, title string) bool {
	t := C.CString(title)
	defer C.free(unsafe.Pointer(t))
	return C.re_ide_press(C.int(pid), t) == 1
}

// Paste pastes text into the focused field without submitting.
func (d darwinDesk) Paste(text string) {
	withClipboard(text, func() { C.re_ide_key(keyV, 1) })
}

func withClipboard(text string, f func()) {
	var prev *string
	if p := C.re_ide_clip_get(); p != nil {
		s := C.GoString(p)
		C.free(unsafe.Pointer(p))
		prev = &s
	}
	t := C.CString(text)
	C.re_ide_clip_set(t)
	C.free(unsafe.Pointer(t))
	time.Sleep(60 * time.Millisecond)
	f()
	time.Sleep(400 * time.Millisecond)
	if prev != nil {
		r := C.CString(*prev)
		C.re_ide_clip_set(r)
		C.free(unsafe.Pointer(r))
	}
}

// PasteAndSubmit pastes text into the focused field and presses Return,
// restoring the previous clipboard text afterwards.
func (darwinDesk) PasteAndSubmit(text string) {
	withClipboard(text, func() {
		C.re_ide_key(keyV, 1)
		time.Sleep(250 * time.Millisecond)
		C.re_ide_key(keyReturn, 0)
	})
}
