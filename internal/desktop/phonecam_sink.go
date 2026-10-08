package desktop

import (
	"fmt"
	"strings"
	"sync"
)

// PhoneCamSink publishes phone-camera frames into a host virtual webcam
// (and a small preview window on Darwin). Platform files implement the body.
type PhoneCamSink struct {
	mu     sync.Mutex
	impl   phoneCamSinkImpl
	width  int
	height int
	fps    int
	device string
	closed bool
}

type phoneCamSinkImpl interface {
	writeJPEG(jpeg []byte, width, height int) error
	writeH264(annexB []byte, width, height int, key bool) error
	close() error
	deviceName() string
}

// StartPhoneCamSink prepares the host virtual webcam sink.
func StartPhoneCamSink(width, height, fps int) (*PhoneCamSink, error) {
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 720
	}
	if fps <= 0 {
		fps = 15
	}
	impl, err := startPhoneCamSinkImpl(width, height, fps)
	if err != nil {
		return nil, err
	}
	return &PhoneCamSink{
		impl:   impl,
		width:  width,
		height: height,
		fps:    fps,
		device: impl.deviceName(),
	}, nil
}

func (s *PhoneCamSink) DeviceName() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device
}

func (s *PhoneCamSink) WriteJPEG(jpeg []byte, width, height int) error {
	return s.write(jpeg, width, height, "mjpeg", true)
}

// WriteH264 publishes one Annex-B access unit (SPS/PPS prepended on keys).
func (s *PhoneCamSink) WriteH264(annexB []byte, width, height int, key bool) error {
	return s.write(annexB, width, height, "h264", key)
}

// WriteFrame routes by codec string ("mjpeg" / "h264").
func (s *PhoneCamSink) WriteFrame(codec string, data []byte, width, height int, key bool) error {
	return s.write(data, width, height, codec, key)
}

func (s *PhoneCamSink) write(data []byte, width, height int, codec string, key bool) error {
	if s == nil {
		return fmt.Errorf("phonecam: nil sink")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.impl == nil {
		return fmt.Errorf("phonecam: closed")
	}
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "avc":
		return s.impl.writeH264(data, s.width, s.height, key)
	default:
		return s.impl.writeJPEG(data, s.width, s.height)
	}
}

func (s *PhoneCamSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.impl != nil {
		err := s.impl.close()
		s.impl = nil
		return err
	}
	return nil
}
