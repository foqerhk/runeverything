//go:build windows && cgo

package desktop

/*
#cgo LDFLAGS: -ld3d11 -ldxgi -luuid -lole32
#include <stdint.h>
#include <stdlib.h>

int re_dxgi_open(int output_index, int *w, int *h);
int re_dxgi_capture(uint8_t **rgba, int *w, int *h, int *stride);
void re_dxgi_free(uint8_t *p);
void re_dxgi_close(void);
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

type dxgiCapturer struct {
	w, h   int
	cancel context.CancelFunc
}

func tryNewDXGICapturer() (Capturer, error) {
	mode := os.Getenv("RE_CAPTURE")
	if mode == "gdi" || mode == "gdigrab" || mode == "ffmpeg" {
		return nil, fmt.Errorf("dxgi disabled by RE_CAPTURE=%s", mode)
	}
	idx := SelectedMonitor()
	var w, h C.int
	if rc := C.re_dxgi_open(C.int(idx), &w, &h); rc != 0 || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("dxgi open failed: %d", int(rc))
	}
	return &dxgiCapturer{w: int(w), h: int(h)}, nil
}

func (c *dxgiCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	ctx, c.cancel = context.WithCancel(ctx)
	ch := make(chan Frame, 1)
	go func() {
		defer close(ch)
		t := time.NewTicker(time.Second / 30)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				img, err := captureDXGIOnce()
				if err != nil || img == nil {
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

func (c *dxgiCapturer) Size() (int, int) { return c.w, c.h }
func (c *dxgiCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	C.re_dxgi_close()
	return nil
}

func captureDXGIOnce() (*image.RGBA, error) {
	var p *C.uint8_t
	var w, h, stride C.int
	if rc := C.re_dxgi_capture(&p, &w, &h, &stride); rc != 0 || p == nil {
		return nil, fmt.Errorf("dxgi capture %d", int(rc))
	}
	defer C.re_dxgi_free(p)
	width, height, str := int(w), int(h), int(stride)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(unsafe.Pointer(p)), str*height)
	for y := 0; y < height; y++ {
		copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*str:y*str+width*4])
	}
	return img, nil
}
