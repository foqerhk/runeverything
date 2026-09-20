package desktop

import (
	"context"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ffmpegCapturer streams RGBA frames from ffmpeg screen grab (ddagrab/gdigrab/avfoundation/x11grab).
type ffmpegCapturer struct {
	w, h   int
	fps    int
	args   []string
	cmd    *exec.Cmd
	stdout io.ReadCloser
	cancel context.CancelFunc
	mu     sync.Mutex
}

// tryFFmpegCapturer builds a platform screen-grab ffmpeg capturer when ffmpeg is available.
func tryFFmpegCapturer(w, h, fps int, args []string) (Capturer, error) {
	if _, err := lookPath("ffmpeg"); err != nil {
		return nil, err
	}
	if fps <= 0 {
		fps = 15
	}
	w &^= 1
	h &^= 1
	if w < 2 || h < 2 {
		return nil, fmt.Errorf("desktop: invalid ffmpeg capture size")
	}
	return &ffmpegCapturer{w: w, h: h, fps: fps, args: args}, nil
}

func (c *ffmpegCapturer) Start(ctx context.Context, maxW, maxH int) (<-chan Frame, error) {
	ctx, c.cancel = context.WithCancel(ctx)
	tw, th := c.w, c.h
	if maxW > 0 && maxH > 0 && (c.w > maxW || c.h > maxH) {
		img := scaleRGBA(image.NewRGBA(image.Rect(0, 0, c.w, c.h)), maxW, maxH)
		tw, th = img.Bounds().Dx(), img.Bounds().Dy()
		tw &^= 1
		th &^= 1
	}
	args := append([]string{}, c.args...)
	// Scale + force rgba rawvideo out.
	vf := fmt.Sprintf("scale=%d:%d:flags=fast_bilinear,format=rgba", tw, th)
	args = append(args,
		"-an", "-vf", vf,
		"-f", "rawvideo", "-pix_fmt", "rgba",
		"pipe:1",
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-loglevel", "error", "-fflags", "nobuffer", "-flags", "low_delay"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cmd = cmd
	c.stdout = stdout
	c.w, c.h = tw, th
	c.mu.Unlock()

	ch := make(chan Frame, 1)
	frameBytes := tw * th * 4
	go func() {
		defer close(ch)
		defer func() {
			c.mu.Lock()
			if c.cmd != nil && c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			c.mu.Unlock()
		}()
		buf := make([]byte, frameBytes)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if _, err := io.ReadFull(stdout, buf); err != nil {
				return
			}
			img := image.NewRGBA(image.Rect(0, 0, tw, th))
			copy(img.Pix, buf)
			select {
			case ch <- Frame{Img: img, Timestamp: time.Now()}:
			default:
			}
		}
	}()
	return ch, nil
}

func (c *ffmpegCapturer) Size() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w, c.h
}

func (c *ffmpegCapturer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func itoaFPS(n int) string { return strconv.Itoa(n) }
