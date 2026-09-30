//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

static int re_screen_preflight(void) {
	if (CGPreflightScreenCaptureAccess()) return 1;
	return 0;
}

static int re_screen_request(void) {
	if (CGRequestScreenCaptureAccess()) return 1;
	return 0;
}

static int re_ax_trusted(int prompt) {
	CFDictionaryRef opts = NULL;
	if (prompt) {
		const void *keys[] = { kAXTrustedCheckOptionPrompt };
		const void *vals[] = { kCFBooleanTrue };
		opts = CFDictionaryCreate(kCFAllocatorDefault, keys, vals, 1,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	}
	Boolean ok = AXIsProcessTrustedWithOptions(opts);
	if (opts) CFRelease(opts);
	return ok ? 1 : 0;
}
*/
import "C"
import (
	"os"
	"os/exec"
	"sync"
)

// HostPermissions summarizes macOS privacy grants needed for remote desktop.
type HostPermissions struct {
	ScreenRecording bool
	Accessibility   bool
}

var permsOnce sync.Once

// CheckHostPermissions reports current TCC-related grants (no prompts).
func CheckHostPermissions() HostPermissions {
	return HostPermissions{
		ScreenRecording: C.re_screen_preflight() == 1,
		Accessibility:   C.re_ax_trusted(0) == 1,
	}
}

// EnsureHostPermissions requests Screen Recording + Accessibility when missing,
// logs guidance, and optionally opens System Settings (unless RE_OPEN_PRIVACY=0).
func EnsureHostPermissions() HostPermissions {
	p := CheckHostPermissions()
	if !p.ScreenRecording {
		_ = C.re_screen_request()
		p.ScreenRecording = C.re_screen_preflight() == 1
	}
	if !p.Accessibility {
		_ = C.re_ax_trusted(1)
		p.Accessibility = C.re_ax_trusted(0) == 1
	}
	permsOnce.Do(func() {
		if os.Getenv("RE_OPEN_PRIVACY") == "0" {
			return
		}
		if !p.ScreenRecording {
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture").Start()
		}
		if !p.Accessibility {
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Start()
		}
	})
	return CheckHostPermissions()
}

// MissingPermissionHints returns human-facing keys for still-denied grants.
func (p HostPermissions) Missing() []string {
	var out []string
	if !p.ScreenRecording {
		out = append(out, "screen")
	}
	if !p.Accessibility {
		out = append(out, "accessibility")
	}
	return out
}
