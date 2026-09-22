//go:build linux && cgo

package desktop

/*
#cgo LDFLAGS: -lX11
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

static int re_x11_capture(unsigned char **out, int *w, int *h, int *stride) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return -1;
	int screen = DefaultScreen(dpy);
	Window root = RootWindow(dpy, screen);
	int width = DisplayWidth(dpy, screen);
	int height = DisplayHeight(dpy, screen);
	XImage *img = XGetImage(dpy, root, 0, 0, width, height, AllPlanes, ZPixmap);
	if (!img) { XCloseDisplay(dpy); return -2; }

	size_t bpr = (size_t)width * 4;
	unsigned char *buf = (unsigned char *)malloc(bpr * (size_t)height);
	if (!buf) { XDestroyImage(img); XCloseDisplay(dpy); return -3; }

	for (int y = 0; y < height; y++) {
		for (int x = 0; x < width; x++) {
			unsigned long p = XGetPixel(img, x, y);
			size_t i = (size_t)y * bpr + (size_t)x * 4;
			buf[i+0] = (p >> 16) & 0xff; // R
			buf[i+1] = (p >> 8) & 0xff;  // G
			buf[i+2] = p & 0xff;         // B
			buf[i+3] = 255;
		}
	}
	XDestroyImage(img);
	XCloseDisplay(dpy);
	*out = buf;
	*w = width;
	*h = height;
	*stride = (int)bpr;
	return 0;
}

static int re_x11_size(int *w, int *h) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return -1;
	int screen = DefaultScreen(dpy);
	*w = DisplayWidth(dpy, screen);
	*h = DisplayHeight(dpy, screen);
	XCloseDisplay(dpy);
	return 0;
}
*/
import "C"

import (
	"context"
	"fmt"
	"image"
	"os"
	"time"
	"unsafe"
)

// NewCapturer prefers X11 root capture when DISPLAY is set.
func NewCapturer() (Capturer, error) {
	if os.Getenv("RE_DESKTOP_FAKE") == "1" {
		return newFakeCapturer(1280, 720), nil
	}
	if os.Getenv("DISPLAY") == "" {
		for _, d := range []string{":0", ":10", ":11"} {
			_ = os.Setenv("DISPLAY", d)
			if _, err := captureX11(); err == nil {
				break
			}
		}
	}
	m, ok := SelectedMonitorInfo()
	w, h := 1920, 1080
	x, y := 0, 0
	if ok {
		w, h, x, y = m.Width, m.Height, m.X, m.Y
	} else if c, err := newX11Capturer(); err == nil {
		w, h = c.Size()
	}
	fps := 15
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		disp = ":0.0"
	}
	if _, err := lookPath("ffmpeg"); err == nil {
		args := []string{
			"-f", "x11grab", "-framerate", itoaFPS(fps),
			"-video_size", fmt.Sprintf("%dx%d", w, h),
			"-i", fmt.Sprintf("%s+%d,%d", disp, x, y),
		}
		if c, err := tryFFmpegCapturer(w, h, fps, args); err == nil {
			return c, nil
		}
	}
	return newX11Capturer()
}

type x11Capturer struct {
	w, h   int
	cancel context.CancelFunc
}

func newX11Capturer() (*x11Capturer, error) {
	var w, h C.int
	if C.re_x11_size(&w, &h) != 0 || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("desktop: cannot open X11 display (set DISPLAY, grant access)")
	}
	return &x11Capturer{w: int(w), h: int(h)}, nil
}

func (c *x11Capturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
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
				img, err := captureX11()
				if err != nil {
					continue
				}
				if m, ok := SelectedMonitorInfo(); ok && m.Width > 0 && m.Height > 0 {
					img = CropRGBA(img, m.X, m.Y, m.Width, m.Height)
				}
				if maxW > 0 && maxH > 0 {
					img = scaleRGBA(img, maxW, maxH)
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

func (c *x11Capturer) Size() (int, int) { return c.w, c.h }

func (c *x11Capturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func captureX11() (*image.RGBA, error) {
	var out *C.uchar
	var w, h, stride C.int
	if rc := C.re_x11_capture(&out, &w, &h, &stride); rc != 0 {
		return nil, fmt.Errorf("desktop: X11 capture failed (%d)", int(rc))
	}
	defer C.free(unsafe.Pointer(out))
	width, height, str := int(w), int(h), int(stride)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(unsafe.Pointer(out)), str*height)
	for y := 0; y < height; y++ {
		copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*str:y*str+width*4])
	}
	return img, nil
}
