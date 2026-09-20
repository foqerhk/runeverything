//go:build windows

package desktop

import (
	"context"
	"fmt"
	"image"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procEnumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
)

const (
	smCXScreen         = 0
	smCYScreen         = 1
	smXVIRTUALSCREEN   = 76
	smYVIRTUALSCREEN   = 77
	smCXVIRTUALSCREEN  = 78
	smCYVIRTUALSCREEN  = 79
	srcCopy            = 0x00CC0020
	dibRGB             = 0
	monitorInfoPrimary = 0x00000001
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type rectWin struct {
	Left, Top, Right, Bottom int32
}

type monitorInfoW struct {
	CbSize    uint32
	RcMonitor rectWin
	RcWork    rectWin
	DwFlags   uint32
}

func NewCapturer() (Capturer, error) {
	if os.Getenv("RE_DESKTOP_FAKE") == "1" {
		return newFakeCapturer(1280, 720), nil
	}
	m, ok := SelectedMonitorInfo()
	if !ok {
		w, _, _ := procGetSystemMetrics.Call(smCXScreen)
		h, _, _ := procGetSystemMetrics.Call(smCYScreen)
		if w == 0 || h == 0 {
			return nil, fmt.Errorf("desktop: GetSystemMetrics failed")
		}
		m = Monitor{Width: int(w), Height: int(h), Primary: true}
	}
	// Prefer native DXGI Desktop Duplication.
	if c, err := tryNewDXGICapturer(); err == nil {
		return c, nil
	}
	fps := 15
	if _, err := lookPath("ffmpeg"); err == nil {
		idx := SelectedMonitor()
		drawMouse := "1"
		if HideCursor() {
			drawMouse = "0"
		}
		if os.Getenv("RE_CAPTURE") == "dda" {
			dda := []string{"-f", "ddagrab", "-framerate", itoaFPS(fps), "-output_idx", itoaFPS(idx), "-i", "desktop"}
			if c, err := tryFFmpegCapturer(m.Width, m.Height, fps, dda); err == nil {
				return c, nil
			}
		}
		gdi := []string{
			"-f", "gdigrab", "-framerate", itoaFPS(fps),
			"-offset_x", itoaFPS(m.X), "-offset_y", itoaFPS(m.Y),
			"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
			"-draw_mouse", drawMouse, "-i", "desktop",
		}
		if c, err := tryFFmpegCapturer(m.Width, m.Height, fps, gdi); err == nil {
			return c, nil
		}
	}
	return &gdiCapturer{w: m.Width, h: m.Height}, nil
}

type gdiCapturer struct {
	w, h   int
	cancel context.CancelFunc
}

func (c *gdiCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	ctx, c.cancel = context.WithCancel(ctx)
	ch := make(chan Frame, 1)
	go func() {
		defer close(ch)
		t := time.NewTicker(66 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				img, err := captureSelected(maxW, maxH)
				if err != nil {
					continue
				}
				c.w, c.h = img.Bounds().Dx(), img.Bounds().Dy()
				select {
				case ch <- Frame{Img: img, Timestamp: time.Now()}:
				default:
				}
			}
		}
	}()
	return ch, nil
}

func (c *gdiCapturer) Size() (int, int) { return c.w, c.h }
func (c *gdiCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func captureSelected(maxW, maxH int) (*image.RGBA, error) {
	m, ok := SelectedMonitorInfo()
	if !ok {
		w, _, _ := procGetSystemMetrics.Call(smCXScreen)
		h, _, _ := procGetSystemMetrics.Call(smCYScreen)
		img, err := captureGDI(0, 0, int(w), int(h))
		if err != nil {
			return nil, err
		}
		if maxW > 0 && maxH > 0 {
			img = scaleRGBA(img, maxW, maxH)
		}
		return img, nil
	}
	img, err := captureGDI(m.X, m.Y, m.Width, m.Height)
	if err != nil {
		return nil, err
	}
	if maxW > 0 && maxH > 0 {
		img = scaleRGBA(img, maxW, maxH)
	}
	return img, nil
}

func captureGDI(x, y, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid capture size")
	}
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, hdc)
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	if mem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)
	procSelectObject.Call(mem, bmp)
	ret, _, _ := procBitBlt.Call(mem, 0, 0, uintptr(width), uintptr(height), hdc, uintptr(x), uintptr(y), srcCopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	bi := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    int32(width),
		Height:   -int32(height), // top-down
		Planes:   1,
		BitCount: 32,
	}
	buf := make([]byte, width*height*4)
	ret, _, _ = procGetDIBits.Call(mem, bmp, 0, uintptr(height),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGB)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// BGRA -> RGBA
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i+0] = buf[i+2]
		img.Pix[i+1] = buf[i+1]
		img.Pix[i+2] = buf[i+0]
		img.Pix[i+3] = 255
	}
	return img, nil
}

func ListMonitors() ([]Monitor, error) {
	var out []Monitor
	cb := syscall.NewCallback(func(hmonitor uintptr, hdc uintptr, lprc uintptr, lparam uintptr) uintptr {
		var mi monitorInfoW
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		r, _, _ := procGetMonitorInfoW.Call(hmonitor, uintptr(unsafe.Pointer(&mi)))
		if r == 0 {
			return 1
		}
		m := Monitor{
			ID:      len(out),
			Name:    fmt.Sprintf("Display %d", len(out)+1),
			X:       int(mi.RcMonitor.Left),
			Y:       int(mi.RcMonitor.Top),
			Width:   int(mi.RcMonitor.Right - mi.RcMonitor.Left),
			Height:  int(mi.RcMonitor.Bottom - mi.RcMonitor.Top),
			Primary: mi.DwFlags&monitorInfoPrimary != 0,
		}
		out = append(out, m)
		return 1
	})
	r, _, _ := procEnumDisplayMonitors.Call(0, 0, cb, 0)
	if r == 0 || len(out) == 0 {
		w, _, _ := procGetSystemMetrics.Call(smCXScreen)
		h, _, _ := procGetSystemMetrics.Call(smCYScreen)
		return []Monitor{{ID: 0, Name: "Primary", Width: int(w), Height: int(h), Primary: true}}, nil
	}
	return out, nil
}

// CaptureOnce grabs a single desktop frame (optionally scaled). Used by the
// SYSTEM helper so it can BitBlt after attaching to Winlogon/UAC desktop.
func CaptureOnce(maxW, maxH int) (*image.RGBA, error) {
	return captureSelected(maxW, maxH)
}
