package desktop

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ffmpegEncoder keeps a persistent ffmpeg process for low-latency H.264/HEVC.
type ffmpegEncoder struct {
	mu        sync.Mutex
	w, h, fps int
	bitrateK  int
	codec     string
	bin       string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	outCh     chan []byte
	errCh     chan error
	closed    bool
	primed    bool // libx265 often withholds the first AU until a second input frame
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
	bin, err := lookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	e := &ffmpegEncoder{w: width, h: height, fps: fps, bitrateK: bitrateK, codec: codec, bin: bin}
	if err := e.start(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *ffmpegEncoder) CodecName() string {
	switch e.codec {
	case "hevc_videotoolbox", "libx265":
		return CodecH265
	default:
		return CodecH264
	}
}

func (e *ffmpegEncoder) hevc() bool {
	return e.codec == "hevc_videotoolbox" || e.codec == "libx265"
}

func (e *ffmpegEncoder) buildArgs() []string {
	size := fmt.Sprintf("%dx%d", e.w, e.h)
	br := fmt.Sprintf("%dk", e.bitrateK)
	maxr := fmt.Sprintf("%dk", e.bitrateK*2)
	gop := max(e.fps, 1)
	if e.w > 8192 || e.h > 8192 {
		// 16K libx265 is multi-second/frame — keep GOP short so IDRs stay frequent.
		gop = max(gop, 2)
	}
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
	case "hevc_videotoolbox":
		return append(commonIn,
			"-c:v", "hevc_videotoolbox", "-b:v", br, "-realtime", "1", "-bf", "0",
			"-pix_fmt", "yuv420p", "-tag:v", "hvc1", "-f", "hevc", "pipe:1")
	case "libx265":
		// Full-blood 16K: Apple VT refuses >8192; libx265 ultrafast is the LAN path.
		// Cap vbv tightly so the first IDR stays assemblable over REUDP (~1.2KB parts).
		brK := e.bitrateK
		if (e.w > 8192 || e.h > 8192) && brK > 12000 {
			brK = 12000
		}
		br = fmt.Sprintf("%dk", brK)
		maxr = fmt.Sprintf("%dk", brK*3/2)
		buf := fmt.Sprintf("%dk", brK)
		x265 := fmt.Sprintf("log-level=error:keyint=%d:min-keyint=%d:scenecut=0:repeat-headers=1:frame-threads=1:bframes=0:rc-lookahead=0:vbv-maxrate=%d:vbv-bufsize=%d", gop, gop, brK, brK)
		return append(commonIn,
			"-c:v", "libx265", "-preset", "ultrafast", "-tune", "zerolatency",
			"-b:v", br, "-maxrate", maxr, "-bufsize", buf,
			"-g", strconv.Itoa(gop), "-bf", "0",
			"-x265-params", x265,
			"-pix_fmt", "yuv420p",
			"-muxdelay", "0", "-muxpreload", "0",
			"-f", "hevc", "-flush_packets", "1", "pipe:1")
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
			"-g", strconv.Itoa(gop), "-bf", "0", "-x264-params", "scenecut=0:keyint="+strconv.Itoa(gop),
			"-pix_fmt", "yuv420p", "-f", "h264", "-flush_packets", "1", "pipe:1")
	}
}

func (e *ffmpegEncoder) start() error {
	bin := e.bin
	if bin == "" {
		var err error
		bin, err = lookPath("ffmpeg")
		if err != nil {
			return err
		}
		e.bin = bin
	}
	cmd := exec.Command(bin, e.buildArgs()...)
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
	e.outCh = make(chan []byte, 8)
	e.errCh = make(chan error, 1)
	e.closed = false
	e.primed = false
	go e.readLoop()
	return nil
}

func (e *ffmpegEncoder) readLoop() {
	type chunk struct {
		b   []byte
		err error
	}
	hevc := e.hevc()
	ch := make(chan chunk, 8)
	go func() {
		tmp := make([]byte, 256*1024)
		for {
			n, err := e.stdout.Read(tmp)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, tmp[:n])
				ch <- chunk{b: cp}
			}
			if err != nil {
				ch <- chunk{err: err}
				return
			}
		}
	}()

	idleMs := 40 * time.Millisecond
	if e.w > 8192 || e.h > 8192 {
		idleMs = 120 * time.Millisecond
	}
	var buf []byte
	var idleC <-chan time.Time
	var idle *time.Timer
	stopIdle := func() {
		if idle == nil {
			return
		}
		if !idle.Stop() {
			select {
			case <-idleC:
			default:
			}
		}
		idle, idleC = nil, nil
	}
	armIdle := func() {
		stopIdle()
		idle = time.NewTimer(idleMs)
		idleC = idle.C
	}
	emit := func(force bool) {
		for {
			au, rest, ok := popAnnexBAU(buf, hevc, force)
			if !ok {
				break
			}
			buf = rest
			// Blocking send — dropping a fat 16K IDR leaves iOS stuck on 1512p.
			select {
			case e.outCh <- au:
			case err := <-e.errCh:
				// Surface prior error; re-queue for Encode.
				select {
				case e.errCh <- err:
				default:
				}
				return
			}
		}
	}

	for {
		select {
		case c := <-ch:
			if len(c.b) > 0 {
				buf = append(buf, c.b...)
				emit(false)
				armIdle()
			}
			if c.err != nil {
				stopIdle()
				emit(true)
				select {
				case e.errCh <- c.err:
				default:
				}
				return
			}
		case <-idleC:
			idle, idleC = nil, nil
			emit(true)
		}
	}
}

// popAnnexBAU returns one access unit: leading parameter-set NALs + first VCL NAL.
// requireComplete=false only emits when a following start code delimits the VCL
// (prevents the classic 64KiB truncate on the first stdout Read of a fat IDR).
// requireComplete=true also emits a trailing VCL (EOF / idle flush).
func popAnnexBAU(buf []byte, hevc bool, requireComplete bool) (au, rest []byte, ok bool) {
	if len(buf) < 8 {
		return nil, buf, false
	}
	starts := findStartCodes(buf)
	if len(starts) == 0 {
		return nil, buf, false
	}
	vclIdx := -1
	for i, sc := range starts {
		nalOff := startCodeNALOffset(buf, sc)
		if nalOff < 0 || nalOff >= len(buf) {
			continue
		}
		if annexBIsVCL(buf[nalOff], hevc) {
			vclIdx = i
			break
		}
	}
	if vclIdx < 0 {
		return nil, buf, false
	}
	if vclIdx+1 < len(starts) {
		end := starts[vclIdx+1]
		return buf[starts[0]:end], buf[end:], true
	}
	if !requireComplete {
		return nil, buf, false
	}
	if len(buf)-starts[vclIdx] < 16 {
		return nil, buf, false
	}
	return buf[starts[0]:], nil, true
}

func startCodeNALOffset(buf []byte, sc int) int {
	if sc+4 <= len(buf) && buf[sc] == 0 && buf[sc+1] == 0 && buf[sc+2] == 0 && buf[sc+3] == 1 {
		return sc + 4
	}
	if sc+3 <= len(buf) && buf[sc] == 0 && buf[sc+1] == 0 && buf[sc+2] == 1 {
		return sc + 3
	}
	return -1
}

func annexBIsVCL(nalHeader byte, hevc bool) bool {
	if hevc {
		nt := int((nalHeader >> 1) & 0x3F)
		// Trails / TSA / STSA / RADL / RASL / BLA / IDR / CRA
		return nt <= 21
	}
	nt := nalHeader & 0x1F
	return nt == 1 || nt == 5
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
	// libx264/libx265 often buffer one frame before emitting; duplicate nudge
	// primes the pipeline (critical for first 16K AU with stdin kept open).
	if (keyframe || !e.primed) && (e.codec == "libx264" || e.codec == "libx265" || e.codec == "") {
		_, _ = e.stdin.Write(pix)
	}
	deadline := 4 * time.Second
	if e.w > 8192 || e.h > 8192 || e.codec == "libx265" {
		// 16K libx265 ultrafast is ~2–4s/frame on Apple Silicon; duplicate prime ≈2×.
		deadline = 45 * time.Second
	}
	select {
	case au := <-e.outCh:
		e.primed = true
		return au, nil
	case err := <-e.errCh:
		_ = e.closeLocked()
		return nil, fmt.Errorf("ffmpeg: %w", err)
	case <-time.After(deadline):
		// last resort: emit whatever we buffered as one AU
		select {
		case au := <-e.outCh:
			e.primed = true
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
