//go:build linux && !cgo

package desktop

import "fmt"

func NewInjector() (Injector, error) {
	return nil, fmt.Errorf("desktop: linux inject requires CGO + libXtst")
}
