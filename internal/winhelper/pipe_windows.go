//go:build windows

package winhelper

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

func winListenPipe(name string, _ interface{}) (net.Listener, error) {
	l, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;WD)(A;;GA;;;SY)(A;;GA;;;BA)",
		InputBufferSize:    1 << 20,
		OutputBufferSize:   1 << 20,
	})
	if err != nil {
		return nil, fmt.Errorf("ListenPipe(%s): %w", name, err)
	}
	return l, nil
}
