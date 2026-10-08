//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation -framework AVFoundation -framework Foundation
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>
#include "perms_media_darwin.h"

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
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// HostPermissions summarizes macOS privacy grants needed for remote desktop.
type HostPermissions struct {
	ScreenRecording bool
	Accessibility   bool
	Microphone      bool
	Camera          bool
}

// CheckHostPermissions reports current TCC-related grants (no prompts).
func CheckHostPermissions() HostPermissions {
	return HostPermissions{
		ScreenRecording: C.re_screen_preflight() == 1,
		Accessibility:   C.re_ax_trusted(0) == 1,
		Microphone:      C.re_mic_authorized() == 1,
		Camera:          C.re_camera_authorized() == 1,
	}
}

// HostPermissionsEffective is in-process grant state plus “granted after
// restart” detection. macOS often keeps CGPreflightScreenCaptureAccess false
// in a long-lived process until that process is relaunched.
type HostPermissionsEffective struct {
	HostPermissions
	ScreenNeedsRestart bool
	AxNeedsRestart     bool
}

// CheckHostPermissionsEffective probes a fresh helper process when this
// process still reports denied, so the UI can show “restart to apply”.
func CheckHostPermissionsEffective() HostPermissionsEffective {
	p := CheckHostPermissions()
	out := HostPermissionsEffective{HostPermissions: p}
	if p.ScreenRecording && p.Accessibility {
		return out
	}
	fresh, ok := probeFreshPermissions()
	if !ok {
		return out
	}
	if !p.ScreenRecording && fresh.ScreenRecording {
		out.ScreenNeedsRestart = true
	}
	if !p.Accessibility && fresh.Accessibility {
		out.AxNeedsRestart = true
	}
	return out
}

func probeFreshPermissions() (HostPermissions, bool) {
	probeMu.Lock()
	if time.Since(probeAt) < 2*time.Second && probeOK {
		v := probeCache
		probeMu.Unlock()
		return v, true
	}
	probeMu.Unlock()

	exe, err := os.Executable()
	if err != nil || exe == "" {
		return HostPermissions{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "__re_perm_check")
	cmd.Env = append(os.Environ(), "RE_PERM_PROBE=1")
	out, err := cmd.Output()
	if err != nil {
		return HostPermissions{}, false
	}
	s := strings.TrimSpace(string(out))
	if len(s) < 2 {
		return HostPermissions{}, false
	}
	v := HostPermissions{
		ScreenRecording: s[0] == '1',
		Accessibility:   s[1] == '1',
	}
	probeMu.Lock()
	probeAt = time.Now()
	probeCache = v
	probeOK = true
	probeMu.Unlock()
	return v, true
}

var (
	probeMu    sync.Mutex
	probeAt    time.Time
	probeCache HostPermissions
	probeOK    bool
)

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

// RequestMicrophone shows the mic permission sheet when still NotDetermined.
// Returns true if already authorized. If previously denied, returns false
// (caller should open Privacy settings — OS will not re-prompt).
func RequestMicrophone() bool {
	C.re_mic_request()
	return C.re_mic_authorized() == 1
}

// RequestCamera shows the camera permission sheet when still NotDetermined.
func RequestCamera() bool {
	C.re_camera_request()
	return C.re_camera_authorized() == 1
}

// MicrophoneCanPrompt is true while TCC status is NotDetermined.
func MicrophoneCanPrompt() bool { return C.re_mic_can_prompt() == 1 }

// CameraCanPrompt is true while TCC status is NotDetermined.
func CameraCanPrompt() bool { return C.re_camera_can_prompt() == 1 }

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

// EnsureHostPermissions reports grants without prompting. Prefer the permissions
// panel so users can read explanations before opening System Settings.
func EnsureHostPermissions() HostPermissions {
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
