package desktop

import (
	"context"
	"fmt"
	"image"
	"os/exec"
	"time"
)

type fakeCapturer struct {
	w, h   int
	ch     chan Frame
	cancel context.CancelFunc
}

func newFakeCapturer(w, h int) *fakeCapturer {
	return &fakeCapturer{w: w, h: h}
}

func (c *fakeCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	if maxW > 0 && maxW < c.w {
		c.w = maxW
	}
	if maxH > 0 && maxH < c.h {
		c.h = maxH
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.ch = make(chan Frame, 2)
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		defer close(c.ch)
		n := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
				x0 := (n * 8) % c.w
				for y := 0; y < c.h; y++ {
					for x := 0; x < c.w; x++ {
						v := uint8(40)
						if x >= x0 && x < x0+40 {
							v = 200
						}
						i := y*img.Stride + x*4
						img.Pix[i] = v
						img.Pix[i+1] = v / 2
						img.Pix[i+2] = 80
						img.Pix[i+3] = 255
					}
				}
				n++
				select {
				case c.ch <- Frame{Img: img, Timestamp: time.Now()}:
				default:
				}
			}
		}
	}()
	return c.ch, nil
}

func (c *fakeCapturer) Size() (int, int) { return c.w, c.h }
func (c *fakeCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func scaleRGBA(src *image.RGBA, maxW, maxH int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= maxW && sh <= maxH {
		return src
	}
	scale := float64(maxW) / float64(sw)
	if sy := float64(maxH) / float64(sh); sy < scale {
		scale = sy
	}
	dw := int(float64(sw) * scale)
	dh := int(float64(sh) * scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	return scaleRGBAExact(src, dw, dh)
}

func scaleRGBAExact(src *image.RGBA, dw, dh int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			sy := y * sh / dh
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// ScaleExact resizes an RGBA frame to exact dimensions (used by ABR).
func ScaleExact(src *image.RGBA, dw, dh int) *image.RGBA {
	if src == nil || dw <= 0 || dh <= 0 {
		return src
	}
	if src.Bounds().Dx() == dw && src.Bounds().Dy() == dh {
		return src
	}
	return scaleRGBAExact(src, dw, dh)
}

func lookPath(bin string) (string, error) {
	p, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", bin, err)
	}
	return p, nil
}
