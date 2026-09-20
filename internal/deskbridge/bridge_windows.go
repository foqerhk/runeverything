//go:build windows

package deskbridge

import (
	"context"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/winhelper"
)

// NewCapturer uses LocalSystem helper when available, else in-process GDI.
func NewCapturer() (desktop.Capturer, error) {
	if winhelper.Available() {
		return &helperCapturer{maxW: 1280, maxH: 720}, nil
	}
	return desktop.NewCapturer()
}

// NewInjector uses helper when available.
func NewInjector() (desktop.Injector, error) {
	if winhelper.Available() {
		return &helperInjector{screenW: 1920, screenH: 1080}, nil
	}
	return desktop.NewInjector()
}

type helperCapturer struct {
	maxW, maxH int
	w, h       int
	cancel     context.CancelFunc
	mu         sync.Mutex
}

func (c *helperCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan desktop.Frame, error) {
	if maxW > 0 {
		c.maxW = maxW
	}
	if maxH > 0 {
		c.maxH = maxH
	}
	ctx, c.cancel = context.WithCancel(ctx)
	ch := make(chan desktop.Frame, 1)
	go func() {
		defer close(ch)
		t := time.NewTicker(66 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				img, err := helperCapture(c.maxW, c.maxH)
				if err != nil {
					continue
				}
				c.mu.Lock()
				c.w, c.h = img.Bounds().Dx(), img.Bounds().Dy()
				c.mu.Unlock()
				select {
				case ch <- desktop.Frame{Img: img, Timestamp: time.Now()}:
				default:
				}
			}
		}
	}()
	return ch, nil
}

func (c *helperCapturer) Size() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == 0 {
		return c.maxW, c.maxH
	}
	return c.w, c.h
}

func (c *helperCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func helperCapture(maxW, maxH int) (*image.RGBA, error) {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	resp, err := cli.Call(winhelper.Request{Op: "capture_rgba", MaxW: maxW, MaxH: maxH})
	if err != nil {
		return nil, err
	}
	if len(resp.RGBA) < 16 || resp.W <= 0 || resp.H <= 0 {
		return nil, fmt.Errorf("helper: empty frame")
	}
	img := image.NewRGBA(image.Rect(0, 0, resp.W, resp.H))
	if len(resp.RGBA) < len(img.Pix) {
		return nil, fmt.Errorf("helper: short rgba")
	}
	copy(img.Pix, resp.RGBA[:len(img.Pix)])
	return img, nil
}

type helperInjector struct {
	screenW, screenH int
	relative         bool
}

func (i *helperInjector) SetScreenSize(w, h int) {
	if w > 0 {
		i.screenW = w
	}
	if h > 0 {
		i.screenH = h
	}
}

func (i *helperInjector) SetRelativeMouse(on bool) { i.relative = on }

func (i *helperInjector) Move(x, y float64) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "move", X: x, Y: y})
	return err
}

func (i *helperInjector) MoveRelative(dx, dy float64) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "move_rel", X: dx, Y: dy})
	if err != nil {
		// older helper: approximate via absolute if screen size known
		ax := 0.5 + dx
		ay := 0.5 + dy
		if ax < 0 {
			ax = 0
		}
		if ay < 0 {
			ay = 0
		}
		if ax > 1 {
			ax = 1
		}
		if ay > 1 {
			ay = 1
		}
		_, err = cli.Call(winhelper.Request{Op: "move", X: ax, Y: ay})
	}
	return err
}

func (i *helperInjector) Button(buttons int, down bool) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "button", Buttons: buttons, Down: down})
	return err
}

func (i *helperInjector) Wheel(delta int) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "wheel", Delta: delta})
	return err
}

func (i *helperInjector) WheelH(delta int) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "wheel_h", Delta: delta})
	return err
}

func (i *helperInjector) Key(keyCode int, text string, down bool, modifiers int) error {
	cli, err := winhelper.Dial(2 * time.Second)
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Call(winhelper.Request{Op: "key", KeyCode: keyCode, Text: text, Down: down, Mods: modifiers})
	return err
}

func (i *helperInjector) Close() error { return nil }
