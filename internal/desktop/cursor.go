package desktop

// CursorPos is normalized cursor location on the selected display.
type CursorPos struct {
	X, Y    float64
	Visible bool
}

// CursorReader reads the OS cursor position.
type CursorReader interface {
	Cursor() (CursorPos, error)
}
