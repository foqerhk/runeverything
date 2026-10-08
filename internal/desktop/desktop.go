package desktop

import (
	"context"
	"image"
	"time"
)

// Frame is a captured desktop frame (RGBA), or a pre-encoded Annex-B unit
// (darwin SCK→VT inline path for 5K HEVC — avoids Go full-panel copies).
type Frame struct {
	Img       *image.RGBA
	AnnexB    []byte
	Keyframe  bool
	Timestamp time.Time
}

// Capturer grabs desktop frames.
type Capturer interface {
	Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error)
	Size() (w, h int)
	Close() error
}

// Injector injects mouse/keyboard into the local desktop.
type Injector interface {
	Move(x, y float64) error // normalized 0..1
	MoveRelative(dx, dy float64) error
	Button(buttons int, down bool) error
	Wheel(delta int) error
	WheelH(delta int) error
	Key(keyCode int, text string, down bool, modifiers int) error
	SetRelativeMouse(on bool)
	Close() error
}

// Encoder produces Annex-B NAL units (H.264 or HEVC) from RGBA frames.
type Encoder interface {
	Encode(f Frame, keyframe bool) (annexB []byte, err error)
	Close() error
}

// CodecNamer is optionally implemented by encoders that report the wire codec.
type CodecNamer interface {
	CodecName() string
}

// Codec name on the wire.
const (
	CodecH264 = "h264"
	CodecH265 = "h265"
)
