package desktop

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ffmpegEncoder keeps a persistent ffmpeg process for low-latency H.264.
type ffmpegEncoder struct {
	mu        sync.Mutex
	w, h, fps int
	bitrateK  int
	codec     string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	outCh     chan []byte
	errCh     chan error
	closed    bool
}

func newFFmpegEncoder(width, height, fps, bitrateK int, codec string) (*ffmpegEncoder, error) {
	if fps <= 0 {
		fps = 15
	}
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	width &^= 1
	height &^= 1
	e := &ffmpegEncoder{w: width, h: height, fps: fps, bitrateK: bitrateK, codec: codec}
	if err := e.start(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *ffmpegEncoder) buildArgs() []string {
	size := fmt.Sprintf("%dx%d", e.w, e.h)
	br := fmt.Sprintf("%dk", e.bitrateK)
	maxr := fmt.Sprintf("%dk", e.bitrateK*2)
	commonIn := []string{
		"-loglevel", "error",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-f", "rawvideo", "-pix_fmt", "rgba",
		"-s", size, "-r", strconv.Itoa(e.fps),
		"-i", "pipe:0",
		"-an",
	}
	switch e.codec {
	case "h264_videotoolbox":
		return append(commonIn,
			"-c:v", "h264_videotoolbox", "-b:v", br, "-realtime", "1", "-bf", "0",
			"-pix_fmt", "yuv420p", "-f", "h264", "pipe:1")
	case "h264_mf":
		return append(commonIn,
			"-c:v", "h264_mf", "-b:v", br, "-bf", "0",
			"-pix_fmt", "yuv420p", "-f", "h264", "pipe:1")
	case "h264_vaapi":
		return []string{
			"-loglevel", "error",
			"-vaapi_device", "/dev/dri/renderD128",
			"-fflags", "nobuffer", "-flags", "low_delay",
			"-f", "rawvideo", "-pix_fmt", "rgba",
			"-s", size, "-r", strconv.Itoa(e.fps),
			"-i", "pipe:0",
			"-vf", "format=nv12,hwupload",
			"-an", "-c:v", "h264_vaapi", "-b:v", br, "-bf", "0",
			"-f", "h264", "pipe:1",
		}
	default: // libx264
		return append(commonIn,
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-b:v", br, "-maxrate", maxr, "-bufsize", maxr,
			"-g", strconv.Itoa(max(e.fps, 1)), "-bf", "0", "-x264-params", "scenecut=0:keyint="+strconv.Itoa(max(e.fps, 1)),
			"-pix_fmt", "yuv420p", "-f", "h264", "-flush_packets", "1", "pipe:1")
	}
}

func (e *ffmpegEncoder) start() error {
	cmd := exec.Command("ffmpeg", e.buildArgs()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return err
	}
	e.cmd = cmd
	e.stdin = stdin
	e.stdout = stdout
	e.outCh = make(chan []byte, 4)
	e.errCh = make(chan error, 1)
	e.closed = false
	go e.readLoop()
	return nil
}

func (e *ffmpegEncoder) readLoop() {
	buf := make([]byte, 0, 512*1024)
	tmp := make([]byte, 64*1024)
	for {
		n, err := e.stdout.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				au, rest, ok := popH264AU(buf)
				if !ok {
					break
				}
				buf = rest
				select {
				case e.outCh <- au:
				default:
					// drop if consumer is slow
				}
			}
		}
		if err != nil {
			select {
			case e.errCh <- err:
			default:
			}
			return
		}
	}
}

func popH264AU(buf []byte) (au, rest []byte, ok bool) {
	if len(buf) < 8 {
		return nil, buf, false
	}
	starts := findStartCodes(buf)
	if len(starts) < 2 {
		// If buffer is large, emit whole buffer as one AU once we see at least one start code.
		if len(starts) == 1 && len(buf) > 32*1024 {
			return buf, nil, true
		}
		return nil, buf, false
	}
	return buf[starts[0]:starts[1]], buf[starts[1]:], true
}

func findStartCodes(b []byte) []int {
	var out []int
	for i := 0; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 {
			if b[i+2] == 1 {
				out = append(out, i)
				i += 2
			} else if b[i+2] == 0 && b[i+3] == 1 {
				out = append(out, i)
				i += 3
			}
		}
	}
	return out
}

func (e *ffmpegEncoder) Encode(f Frame, keyframe bool) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if f.Img == nil {
		return nil, fmt.Errorf("desktop: nil frame")
	}
	w, h := f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
	if e.closed || e.stdin == nil || w != e.w || h != e.h {
		_ = e.closeLocked()
		e.w, e.h = w&^1, h&^1
		if err := e.start(); err != nil {
			return nil, err
		}
	}
	// Pack tightly: ffmpeg expects contiguous rgba (stride == w*4).
	pix := f.Img.Pix
	need := e.w * e.h * 4
	if f.Img.Stride != e.w*4 || len(pix) < need || f.Img.Rect.Min.X != 0 || f.Img.Rect.Min.Y != 0 {
		packed := make([]byte, need)
		for y := 0; y < e.h; y++ {
			srcOff := y * f.Img.Stride
			copy(packed[y*e.w*4:(y+1)*e.w*4], pix[srcOff:srcOff+e.w*4])
		}
		pix = packed
	} else if len(pix) > need {
		pix = pix[:need]
	}
	if _, err := e.stdin.Write(pix); err != nil {
		_ = e.closeLocked()
		return nil, fmt.Errorf("ffmpeg write: %w", err)
	}
	// Some builds of libx264 buffer one frame; nudge with a duplicate write once.
	if keyframe {
		_, _ = e.stdin.Write(pix)
	}
	_ = keyframe
	deadline := 4 * time.Second
	select {
	case au := <-e.outCh:
		return au, nil
	case err := <-e.errCh:
		_ = e.closeLocked()
		return nil, fmt.Errorf("ffmpeg: %w", err)
	case <-time.After(deadline):
		// last resort: emit whatever we buffered as one AU
		select {
		case au := <-e.outCh:
			return au, nil
		default:
			return nil, fmt.Errorf("ffmpeg: encode timeout")
		}
	}
}

func (e *ffmpegEncoder) Reconfigure(width, height, fps, bitrateK int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	width &^= 1
	height &^= 1
	if fps <= 0 {
		fps = e.fps
	}
	if bitrateK <= 0 {
		bitrateK = e.bitrateK
	}
	if width == e.w && height == e.h && fps == e.fps && bitrateK == e.bitrateK && !e.closed {
		return nil
	}
	_ = e.closeLocked()
	e.w, e.h, e.fps, e.bitrateK = width, height, fps, bitrateK
	return e.start()
}

func (e *ffmpegEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closeLocked()
}

func (e *ffmpegEncoder) closeLocked() error {
	e.closed = true
	if e.stdin != nil {
		_ = e.stdin.Close()
		e.stdin = nil
	}
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_, _ = e.cmd.Process.Wait()
	}
	e.cmd = nil
	e.stdout = nil
	return nil
}

type syntheticEncoder struct {
	w, h int
}

func (e *syntheticEncoder) Encode(f Frame, keyframe bool) ([]byte, error) {
	return append([]byte(nil), syntheticH264Black...), nil
}

func (e *syntheticEncoder) Close() error { return nil }

var syntheticH264Black = []byte{
	0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x0a, 0xf8, 0x41, 0xa2,
	0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80,
	0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x2a, 0xff, 0xfe,
	0xf6, 0xf0, 0x00, 0x00,
}

// Reconfigurer is optionally implemented by encoders that can hot-change params.
type Reconfigurer interface {
	Reconfigure(width, height, fps, bitrateK int) error
}
