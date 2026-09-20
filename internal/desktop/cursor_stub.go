//go:build !darwin && !linux && !windows

package desktop

type stubCursor struct{}

func NewCursorReader() CursorReader { return &stubCursor{} }

func (s *stubCursor) Cursor() (CursorPos, error) {
	return CursorPos{X: 0.5, Y: 0.5, Visible: true}, nil
}
