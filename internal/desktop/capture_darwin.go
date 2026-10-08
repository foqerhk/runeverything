//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreGraphics -framework CoreFoundation -framework Foundation -framework CoreVideo -framework AVFoundation -framework CoreMedia -framework VideoToolbox
#include <stdlib.h>

typedef struct {
	unsigned char *data;
	int w;
	int h;
	int stride;
	int err;
} re_sck_shot;

typedef struct {
	unsigned char *data;
	int len;
	int keyframe;
	int w;
	int h;
	int err;
} re_sck_nal;

void re_sck_capture(re_sck_shot *out);
void re_sck_capture_display(re_sck_shot *out, int display_index);
void re_sck_capture_display_ex(re_sck_shot *out, int display_index, int show_cursor);
int re_sck_main_size(int *w, int *h);
void re_sck_request_access(void);
int re_sck_stream_start(int display_index, int show_cursor, int *w, int *h);
int re_sck_stream_start_ex(int display_index, int show_cursor, int maxW, int maxH,
	int bitrateK, int fps, int hevc, int *w, int *h);
int re_sck_stream_poll(re_sck_shot *out);
int re_sck_stream_poll_nal(re_sck_nal *out);
void re_sck_stream_request_keyframe(void);
int re_sck_stream_inline_hevc(void);
void re_sck_stream_stop(void);
*/
import "C"

import (
	"context"
	"fmt"
	"image"
	"log"
	"os"
	"sync"
	"time"
	"unsafe"
)

var (
	captureInlineMu   sync.Mutex
	captureInlineHEVC bool
	captureInlineW    int
	captureInlineH    int
	captureInlineBR   int
	captureInlineFPS  int
)

// SetCaptureInlineHEVC asks the next SCK stream start to VT-encode HEVC in the
// SCStream callback (CVPixelBuffer → Annex-B, no Go 5K RGBA copies).
func SetCaptureInlineHEVC(maxW, maxH, bitrateK, fps int) {
	captureInlineMu.Lock()
	captureInlineHEVC = true
	captureInlineW, captureInlineH = maxW, maxH
	captureInlineBR = bitrateK
	captureInlineFPS = fps
	captureInlineMu.Unlock()
}

func ClearCaptureInlineHEVC() {
	captureInlineMu.Lock()
	captureInlineHEVC = false
	captureInlineMu.Unlock()
}

func takeCaptureInlineHEVC() (on bool, maxW, maxH, br, fps int) {
	captureInlineMu.Lock()
	on = captureInlineHEVC
	maxW, maxH = captureInlineW, captureInlineH
	br, fps = captureInlineBR, captureInlineFPS
	captureInlineHEVC = false
	captureInlineMu.Unlock()
	return
}

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
	inlineOn, inlineW, inlineH, inlineBR, inlineFPS := takeCaptureInlineHEVC()
	mw, mh := w, h
	if inlineOn {
		if inlineW > 0 {
			mw = inlineW
		}
		if inlineH > 0 {
			mh = inlineH
		}
	} else if fw, fh, ok := VirtualFramebuffer(SelectedMonitor()); ok && fw > mw && fh > mh {
		// Non-inline: still target virtual FB pixels (16K full-blood).
		mw, mh = fw, fh
	}
	var sw, sh C.int
	hevcFlag := 0
	if inlineOn {
		hevcFlag = 1
	}
	st := C.re_sck_stream_start_ex(C.int(SelectedMonitor()), C.int(show),
		C.int(mw), C.int(mh), C.int(inlineBR), C.int(inlineFPS), C.int(hevcFlag), &sw, &sh)
	if st == 0 {
		if sw > 0 {
			w = int(sw)
		}
		if sh > 0 {
			h = int(sh)
		}
		usedInline := inlineOn && C.re_sck_stream_inline_hevc() != 0
		if inlineOn && !usedInline {
			log.Printf("desktop: inline VT unavailable at %dx%d — SCK pixel + Go encode", w, h)
		}
		return &sckStreamCapturer{w: w, h: h, inlineHEVC: usedInline}, nil
	}
	log.Printf("desktop: SCK stream start failed st=%d target=%dx%d inline=%v", int(st), mw, mh, inlineOn)
	ClearCaptureInlineHEVC()
	// Do not fall through to ffmpeg with CGDirectDisplayID as avfoundation index —
	// that silently opens the wrong display. Surface the SCK error instead.
	if mw >= 3840 || mh >= 2160 {
		return nil, fmt.Errorf("SCK capture %dx%d failed (st=%d)", mw, mh, int(st))
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
	w, h        int
	inlineHEVC  bool
	cancel      context.CancelFunc
}

func (c *sckStreamCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	ctx, c.cancel = context.WithCancel(ctx)
	ch := make(chan Frame, 2)
	go func() {
		defer close(ch)
		defer C.re_sck_stream_stop()
		var lastImage *image.RGBA
		var lastImageEmit time.Time
		emit := func(frame Frame) bool {
			select {
			case <-ctx.Done():
				return false
			case ch <- frame:
			default:
				select {
				case <-ch:
				default:
				}
				select {
				case ch <- frame:
				default:
				}
			}
			return true
		}
		pollHz := 30
		if maxW > 8192 || maxH > 8192 {
			pollHz = 2
		} else if maxW >= 7680 || maxH >= 4320 {
			pollHz = 8
		}
		t := time.NewTicker(time.Second / time.Duration(pollHz))
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if c.inlineHEVC || C.re_sck_stream_inline_hevc() != 0 {
					var nal C.re_sck_nal
					if C.re_sck_stream_poll_nal(&nal) != 0 || nal.data == nil || nal.len <= 0 {
						continue
					}
					b := C.GoBytes(unsafe.Pointer(nal.data), nal.len)
					C.free(unsafe.Pointer(nal.data))
					if nal.w > 0 {
						c.w = int(nal.w)
					}
					if nal.h > 0 {
						c.h = int(nal.h)
					}
					fr := Frame{AnnexB: b, Keyframe: nal.keyframe != 0, Timestamp: time.Now()}
					select {
					case <-ctx.Done():
						return
					case ch <- fr:
					default:
						select {
						case <-ch:
						default:
						}
						select {
						case ch <- fr:
						default:
						}
					}
					continue
				}
				img, err := pollSCKStreamMax(maxW, maxH)
				if err != nil {
					// ScreenCaptureKit is damage-driven and may emit nothing on a
					// static desktop. Replay the last pixel frame at 1fps so an
					// OPEN/keyframe request can resize/rebuild the decoder instead
					// of waiting forever for unrelated screen damage.
					if lastImage != nil && time.Since(lastImageEmit) >= time.Second {
						lastImageEmit = time.Now()
						if !emit(Frame{Img: lastImage, Timestamp: lastImageEmit}) {
							return
						}
					}
					continue
				}
				if maxW > 0 && maxH > 0 {
					img = scaleRGBA(img, maxW, maxH)
				}
				c.w, c.h = img.Bounds().Dx(), img.Bounds().Dy()
				lastImage = img
				lastImageEmit = time.Now()
				if !emit(Frame{Img: img, Timestamp: lastImageEmit}) {
					return
				}
			}
		}
	}()
	return ch, nil
}

func (c *sckStreamCapturer) RequestKeyframe() {
	C.re_sck_stream_request_keyframe()
}

func (c *sckStreamCapturer) Size() (int, int) { return c.w, c.h }
func (c *sckStreamCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	C.re_sck_stream_stop()
	return nil
}

func pollSCKStream() (*image.RGBA, error) { return pollSCKStreamMax(0, 0) }

// pollSCKStreamMax copies out of the SCK buffer already stepped down toward maxW/maxH
// so a 5K secondary never materializes a full 56MB Go RGBA (was pegging encode at ~1fps).
func pollSCKStreamMax(maxW, maxH int) (*image.RGBA, error) {
	var shot C.re_sck_shot
	if C.re_sck_stream_poll(&shot) != 0 || shot.data == nil {
		return nil, fmt.Errorf("no frame")
	}
	defer C.free(unsafe.Pointer(shot.data))
	width, height, str := int(shot.w), int(shot.h), int(shot.stride)
	step := 1
	if maxW > 0 && maxH > 0 {
		for width/step > maxW*2 || height/step > maxH*2 {
			step *= 2
		}
	}
	dw, dh := width/step, height/step
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, dw, dh))
	src := unsafe.Slice((*byte)(unsafe.Pointer(shot.data)), str*height)
	for y := 0; y < dh; y++ {
		sy := y * step
		srcOff := sy * str
		dstOff := y * img.Stride
		if step == 1 {
			copy(img.Pix[dstOff:dstOff+dw*4], src[srcOff:srcOff+dw*4])
			continue
		}
		for x := 0; x < dw; x++ {
			si := srcOff + x*step*4
			di := dstOff + x*4
			img.Pix[di] = src[si]
			img.Pix[di+1] = src[si+1]
			img.Pix[di+2] = src[si+2]
			img.Pix[di+3] = src[si+3]
		}
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
					for img.Bounds().Dx() > maxW*2 || img.Bounds().Dy() > maxH*2 {
						img = subsampleHalfRGBA(img)
					}
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
