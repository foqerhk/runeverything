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

// RequestScreenRecording triggers the system Screen Recording prompt when possible.
func RequestScreenRecording() bool {
	_ = C.re_screen_request()
	return C.re_screen_preflight() == 1
}

// RequestAccessibility triggers the Accessibility trust prompt when possible.
func RequestAccessibility() bool {
	_ = C.re_ax_trusted(1)
	return C.re_ax_trusted(0) == 1
}

// OpenPrivacySettings opens the matching macOS Privacy & Security pane.
func OpenPrivacySettings(kind string) error {
	url := ""
	switch kind {
	case "screen", "screen_recording":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
	case "accessibility", "ax":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	case "microphone", "mic":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone"
	case "camera":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_Camera"
	default:
		url = "x-apple.systempreferences:com.apple.preference.security"
	}
	return exec.Command("open", url).Start()
}

// EnsureHostPermissions requests Screen Recording + Accessibility when missing,
// and optionally opens System Settings (unless RE_OPEN_PRIVACY=0).
func EnsureHostPermissions() HostPermissions {
	p := CheckHostPermissions()
	if !p.ScreenRecording {
		_ = RequestScreenRecording()
		p.ScreenRecording = C.re_screen_preflight() == 1
	}
	if !p.Accessibility {
		_ = RequestAccessibility()
		p.Accessibility = C.re_ax_trusted(0) == 1
	}
	permsOnce.Do(func() {
		if os.Getenv("RE_OPEN_PRIVACY") == "0" {
			return
		}
		if !p.ScreenRecording {
			_ = OpenPrivacySettings("screen")
		}
		if !p.Accessibility {
			_ = OpenPrivacySettings("accessibility")
		}
	})
	return CheckHostPermissions()
}

// Missing returns keys for still-denied required grants.
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
