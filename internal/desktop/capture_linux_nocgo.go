//go:build linux && !cgo

package desktop

import (
	"fmt"
	"os"
)

func NewCapturer() (Capturer, error) {
	if os.Getenv("RE_DESKTOP_FAKE") == "1" {
		return newFakeCapturer(1280, 720), nil
	}
	return nil, fmt.Errorf("desktop: linux capture requires CGO + libX11 (rebuild with CGO_ENABLED=1)")
}

// Referenced by monitors_linux.go fallback path.
type x11Capturer struct{ w, h int }

func newX11Capturer() (*x11Capturer, error) {
	return nil, fmt.Errorf("x11 unavailable without cgo")
}

func (c *x11Capturer) Size() (int, int) {
	if c == nil {
		return 0, 0
	}
	return c.w, c.h
}
