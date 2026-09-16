//go:build !darwin && !windows && !linux

package keepalive

import "fmt"

func platformStart() (func(), error) {
	return nil, fmt.Errorf("keepalive not implemented on this OS")
}
