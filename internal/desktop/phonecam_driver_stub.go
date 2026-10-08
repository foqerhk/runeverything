//go:build !darwin && !windows

package desktop

import "fmt"

func phoneCamDriverStatus() PhoneCamDriverStatus { return PhoneCamDriverStatus{} }

func openPhoneCamDriverInstall() error {
	return fmt.Errorf("phone webcam driver install unsupported on this platform")
}
