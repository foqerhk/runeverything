//go:build windows && !cgo

package desktop

import "fmt"

func tryNewDXGICapturer() (Capturer, error) {
	return nil, fmt.Errorf("dxgi requires cgo")
}
