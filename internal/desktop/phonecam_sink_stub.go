//go:build !darwin && !windows

package desktop

import "fmt"

func startPhoneCamSinkImpl(width, height, fps int) (phoneCamSinkImpl, error) {
	return nil, fmt.Errorf("phonecam: virtual webcam unsupported on this platform")
}
