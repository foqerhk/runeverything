//go:build linux

package desktop

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <stdlib.h>

static int re_x_cursor(double *nx, double *ny, int *root_w, int *root_h) {
	Display *d = XOpenDisplay(NULL);
	if (!d) return -1;
	int screen = DefaultScreen(d);
	Window root = RootWindow(d, screen);
	*root_w = DisplayWidth(d, screen);
	*root_h = DisplayHeight(d, screen);
	Window rr, cr;
	int rx, ry, wx, wy;
	unsigned int mask;
	if (!XQueryPointer(d, root, &rr, &cr, &rx, &ry, &wx, &wy, &mask)) {
		XCloseDisplay(d);
		return -2;
	}
	*nx = (double)rx / (double)(*root_w);
	*ny = (double)ry / (double)(*root_h);
	XCloseDisplay(d);
	return 0;
}
*/
import "C"

type linuxCursor struct{}

func NewCursorReader() CursorReader { return &linuxCursor{} }

func (l *linuxCursor) Cursor() (CursorPos, error) {
	var nx, ny C.double
	var w, h C.int
	if C.re_x_cursor(&nx, &ny, &w, &h) != 0 {
		return CursorPos{Visible: true}, nil
	}
	x, y := float64(nx), float64(ny)
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	if y < 0 {
		y = 0
	}
	if y > 1 {
		y = 1
	}
	return CursorPos{X: x, Y: y, Visible: true}, nil
}
