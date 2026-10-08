//go:build !darwin

package desktop

import "fmt"

func NudgeDesktopSpace(delta int) error {
	return fmt.Errorf("space switch not supported on this OS")
}
