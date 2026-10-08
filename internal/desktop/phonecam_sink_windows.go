//go:build windows

package desktop

import (
	"fmt"
	"sync"
)

// Windows: keep a latest-frame buffer. A DirectShow / OBS virtual-cam bridge can
// be wired later; for now we accept frames so the protocol path works and return
// a clear device name for the client UI.
type windowsPhoneCamSink struct {
	mu     sync.Mutex
	last   []byte
	width  int
	height int
	closed bool
}

func startPhoneCamSinkImpl(width, height, fps int) (phoneCamSinkImpl, error) {
	_ = fps
	return &windowsPhoneCamSink{width: width, height: height}, nil
}

func (s *windowsPhoneCamSink) deviceName() string { return "KoKo Phone Camera" }

func (s *windowsPhoneCamSink) writeJPEG(jpeg []byte, width, height int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("phonecam: closed")
	}
	s.last = append(s.last[:0], jpeg...)
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
	return nil
}

func (s *windowsPhoneCamSink) writeH264(annexB []byte, width, height int, key bool) error {
	_ = key
	// No decoder on Windows stub yet — keep buffer so the path stays open.
	return s.writeJPEG(annexB, width, height)
}

func (s *windowsPhoneCamSink) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.last = nil
	return nil
}
