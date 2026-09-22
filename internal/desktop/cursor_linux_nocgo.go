//go:build linux && !cgo

package desktop

type linuxCursor struct{}

func NewCursorReader() CursorReader { return &linuxCursor{} }

func (l *linuxCursor) Cursor() (CursorPos, error) {
	return CursorPos{Visible: true}, nil
}
