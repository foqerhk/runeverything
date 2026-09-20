//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreGraphics -framework CoreFoundation -framework Foundation -framework CoreVideo -framework AVFoundation -framework CoreMedia
#include <stdlib.h>

typedef struct {
	unsigned char *data;
	int w;
	int h;
	int stride;
	int err;
} re_sck_shot;

void re_sck_capture(re_sck_shot *out);
void re_sck_capture_display(re_sck_shot *out, int display_index);
void re_sck_capture_display_ex(re_sck_shot *out, int display_index, int show_cursor);
int re_sck_main_size(int *w, int *h);
void re_sck_request_access(void);
int re_sck_stream_start(int display_index, int show_cursor, int *w, int *h);
int re_sck_stream_poll(re_sck_shot *out);
void re_sck_stream_stop(void);
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

// NewCapturer prefers continuous SCStream, then ffmpeg, then one-shot SCK ticker.
func NewCapturer() (Capturer, error) {
	if os.Getenv("RE_DESKTOP_FAKE") == "1" {
		return newFakeCapturer(1280, 720), nil
	}
	C.re_sck_request_access()
	m, ok := SelectedMonitorInfo()
	w, h := 1280, 720
	if ok {
		w, h = m.Width, m.Height
	} else {
		var cw, ch C.int
		C.re_sck_main_size(&cw, &ch)
		if cw > 0 && ch > 0 {
			w, h = int(cw), int(ch)
		}
	}
	show := 1
	if HideCursor() {
		show = 0
	}
	var sw, sh C.int
	if C.re_sck_stream_start(C.int(SelectedMonitor()), C.int(show), &sw, &sh) == 0 {
		if sw > 0 {
			w = int(sw)
		}
		if sh > 0 {
			h = int(sh)
		}
		return &sckStreamCapturer{w: w, h: h}, nil
	}
	fps := 15
	if _, err := lookPath("ffmpeg"); err == nil {
		idx := SelectedMonitor()
		cur := "1"
		if HideCursor() {
			cur = "0"
		}
		av := []string{"-f", "avfoundation", "-framerate", itoaFPS(fps),
			"-capture_cursor", cur, "-i", fmt.Sprintf("%d:none", idx+1)}
		if c, err := tryFFmpegCapturer(w, h, fps, av); err == nil {
			return c, nil
		}
	}
	return newSCKCapturer()
}

type sckStreamCapturer struct {
	w, h   int
	cancel context.CancelFunc
}

func (c *sckStreamCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	ctx, c.cancel = context.WithCancel(ctx)
	ch := make(chan Frame, 1)
	go func() {
		defer close(ch)
		defer C.re_sck_stream_stop()
		t := time.NewTicker(time.Second / 30)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				img, err := pollSCKStream()
				if err != nil {
					continue
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

func (c *sckStreamCapturer) Size() (int, int) { return c.w, c.h }
func (c *sckStreamCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	C.re_sck_stream_stop()
	return nil
}

func pollSCKStream() (*image.RGBA, error) {
	var shot C.re_sck_shot
	if C.re_sck_stream_poll(&shot) != 0 || shot.data == nil {
		return nil, fmt.Errorf("no frame")
	}
	defer C.free(unsafe.Pointer(shot.data))
	width, height, str := int(shot.w), int(shot.h), int(shot.stride)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(unsafe.Pointer(shot.data)), str*height)
	for y := 0; y < height; y++ {
		copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*str:y*str+width*4])
	}
	return img, nil
}

type sckCapturer struct {
	w, h   int
	cancel context.CancelFunc
}

func newSCKCapturer() (*sckCapturer, error) {
	var w, h C.int
	C.re_sck_main_size(&w, &h)
	if w <= 0 || h <= 0 {
		w, h = 1280, 720
	}
	return &sckCapturer{w: int(w), h: int(h)}, nil
}

func (c *sckCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
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
				img, err := captureSCKSelected()
				if err != nil {
					continue
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

func (c *sckCapturer) Size() (int, int) { return c.w, c.h }
func (c *sckCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func captureSCK() (*image.RGBA, error) { return captureSCKAt(0) }

func captureSCKSelected() (*image.RGBA, error) {
	idx := SelectedMonitor()
	if idx < 0 {
		idx = 0
	}
	return captureSCKAt(idx)
}

func captureSCKAt(displayIndex int) (*image.RGBA, error) {
	var shot C.re_sck_shot
	show := C.int(1)
	if HideCursor() {
		show = 0
	}
	C.re_sck_capture_display_ex(&shot, C.int(displayIndex), show)
	if shot.err != 0 || shot.data == nil {
		return nil, fmt.Errorf("desktop: ScreenCaptureKit failed (%d) — enable Screen Recording for this app", int(shot.err))
	}
	defer C.free(unsafe.Pointer(shot.data))
	width, height, str := int(shot.w), int(shot.h), int(shot.stride)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(unsafe.Pointer(shot.data)), str*height)
	for y := 0; y < height; y++ {
		copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*str:y*str+width*4])
	}
	return img, nil
}
