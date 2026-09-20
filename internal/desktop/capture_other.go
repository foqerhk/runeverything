//go:build !darwin && !linux && !windows

package desktop

import "os"

// NewCapturer for platforms without a native capturer yet.
func NewCapturer() (Capturer, error) {
	_ = os.Getenv("RE_DESKTOP_FAKE")
	return newFakeCapturer(1280, 720), nil
}
