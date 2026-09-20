package desktop

import (
	"image"
	"sync"
)

// Monitor describes one display.
type Monitor struct {
	ID      int
	Name    string
	Width   int
	Height  int
	X       int
	Y       int
	Primary bool
}

// Lister enumerates monitors (optional; platforms may stub one virtual display).
type Lister interface {
	ListMonitors() ([]Monitor, error)
}

var (
	monitorMu   sync.Mutex
	selectedMon = 0
	hideCursor  bool
)

func SetSelectedMonitor(id int) { monitorMu.Lock(); selectedMon = id; monitorMu.Unlock() }
func SelectedMonitor() int     { monitorMu.Lock(); defer monitorMu.Unlock(); return selectedMon }

func SetHideCursor(hide bool) { monitorMu.Lock(); hideCursor = hide; monitorMu.Unlock() }
func HideCursor() bool        { monitorMu.Lock(); defer monitorMu.Unlock(); return hideCursor }

// SelectedMonitorInfo returns the currently selected display geometry.
func SelectedMonitorInfo() (Monitor, bool) {
	mons, err := ListMonitors()
	if err != nil || len(mons) == 0 {
		return Monitor{}, false
	}
	id := SelectedMonitor()
	for _, m := range mons {
		if m.ID == id {
			return m, true
		}
	}
	for _, m := range mons {
		if m.Primary {
			return m, true
		}
	}
	return mons[0], true
}

// CropRGBA extracts a sub-rectangle from src (src is full virtual desktop coords).
func CropRGBA(src *image.RGBA, x, y, w, h int) *image.RGBA {
	if src == nil || w <= 0 || h <= 0 {
		return src
	}
	b := src.Bounds()
	if x < b.Min.X {
		x = b.Min.X
	}
	if y < b.Min.Y {
		y = b.Min.Y
	}
	if x+w > b.Max.X {
		w = b.Max.X - x
	}
	if y+h > b.Max.Y {
		h = b.Max.Y - y
	}
	if w <= 0 || h <= 0 {
		return src
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for row := 0; row < h; row++ {
		srcOff := (y+row-b.Min.Y)*src.Stride + (x-b.Min.X)*4
		dstOff := row * out.Stride
		copy(out.Pix[dstOff:dstOff+w*4], src.Pix[srcOff:srcOff+w*4])
	}
	return out
}
