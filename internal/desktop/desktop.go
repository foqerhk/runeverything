package desktop

import (
	"context"
	"image"
	"time"
)

// Frame is a captured desktop frame (RGBA).
type Frame struct {
	Img       *image.RGBA
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

// Encoder produces H.264 Annex-B NAL units from RGBA frames.
type Encoder interface {
	Encode(f Frame, keyframe bool) (annexB []byte, err error)
	Close() error
}

// Codec name on the wire.
const CodecH264 = "h264"
