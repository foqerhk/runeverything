package desktop

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// CameraDevice describes a capture device.
type CameraDev struct {
	ID   string
	Name string
}

// ListCameras enumerates cameras via ffmpeg device listing when possible.
func ListCameras() ([]CameraDev, error) {
	if _, err := lookPath("ffmpeg"); err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "darwin":
		return listCamerasAVFoundation()
	case "windows":
		return listCamerasDShow()
	case "linux":
		return listCamerasV4L2()
	default:
		return nil, fmt.Errorf("unsupported")
	}
}

func listCamerasAVFoundation() ([]CameraDev, error) {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()
	var devs []CameraDev
	inVideo := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "AVFoundation video devices") {
			inVideo = true
			continue
		}
		if strings.Contains(line, "AVFoundation audio devices") {
			break
		}
		if !inVideo {
			continue
		}
		// [AVFoundation ...] [0] FaceTime HD Camera
		if i := strings.Index(line, "] ["); i >= 0 {
			rest := line[i+3:]
			end := strings.Index(rest, "]")
			if end < 0 {
				continue
			}
			id := rest[:end]
			name := strings.TrimSpace(rest[end+1:])
			devs = append(devs, CameraDev{ID: id, Name: name})
		}
	}
	return devs, nil
}

func listCamerasDShow() ([]CameraDev, error) {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	out, _ := cmd.CombinedOutput()
	var devs []CameraDev
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "(video)") {
			continue
		}
		start := strings.Index(line, `"`)
		end := strings.LastIndex(line, `"`)
		if start < 0 || end <= start {
			continue
		}
		name := line[start+1 : end]
		devs = append(devs, CameraDev{ID: name, Name: name})
	}
	return devs, nil
}

func listCamerasV4L2() ([]CameraDev, error) {
	var devs []CameraDev
	entries, _ := os.ReadDir("/dev")
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "video") {
			devs = append(devs, CameraDev{ID: "/dev/" + name, Name: name})
		}
	}
	if len(devs) == 0 {
		devs = append(devs, CameraDev{ID: "/dev/video0", Name: "video0"})
	}
	return devs, nil
}

// CameraCapture streams MJPEG or H264 frames from a camera via ffmpeg.
type CameraCapture struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdout io.ReadCloser
	codec  string
	w, h   int
	cancel context.CancelFunc
}

func StartCamera(ctx context.Context, deviceID string, width, height, fps int, codec string) (*CameraCapture, error) {
	if _, err := lookPath("ffmpeg"); err != nil {
		return nil, err
	}
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 720
	}
	if fps <= 0 {
		fps = 15
	}
	if codec == "" {
		codec = "mjpeg"
	}
	if deviceID == "" {
		devs, _ := ListCameras()
		if len(devs) > 0 {
			deviceID = devs[0].ID
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	inArgs := cameraInputArgs(deviceID)
	var outArgs []string
	switch codec {
	case "h264":
		outArgs = []string{"-an", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-f", "h264", "pipe:1"}
	default:
		codec = "mjpeg"
		outArgs = []string{"-an", "-c:v", "mjpeg", "-q:v", "5", "-f", "mjpeg", "pipe:1"}
	}
	args := append([]string{"-loglevel", "error"}, inArgs...)
	args = append(args, "-s", fmt.Sprintf("%dx%d", width, height), "-r", itoa(fps))
	args = append(args, outArgs...)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &CameraCapture{cmd: cmd, stdout: stdout, codec: codec, w: width, h: height, cancel: cancel}, nil
}

func cameraInputArgs(deviceID string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"-f", "avfoundation", "-framerate", "30", "-i", deviceID + ":none"}
	case "windows":
		return []string{"-f", "dshow", "-i", "video=" + deviceID}
	default:
		return []string{"-f", "v4l2", "-i", deviceID}
	}
}

func (c *CameraCapture) Codec() string { return c.codec }
func (c *CameraCapture) Size() (int, int) { return c.w, c.h }

// ReadFrame reads one MJPEG frame or a chunk of H264.
func (c *CameraCapture) ReadFrame() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdout == nil {
		return nil, io.EOF
	}
	if c.codec == "mjpeg" {
		return readMJPEGFrame(c.stdout)
	}
	buf := make([]byte, 32*1024)
	n, err := c.stdout.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	return nil, err
}

func readMJPEGFrame(r io.Reader) ([]byte, error) {
	// Scan for JPEG SOI/EOI.
	var out []byte
	tmp := make([]byte, 4096)
	started := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, err := r.Read(tmp)
		if n > 0 {
			out = append(out, tmp[:n]...)
			if !started {
				if i := index2(out, 0xff, 0xd8); i >= 0 {
					out = out[i:]
					started = true
				} else if len(out) > 64*1024 {
					out = out[len(out)-2:]
				}
			}
			if started {
				if i := index2(out[2:], 0xff, 0xd9); i >= 0 {
					end := i + 2 + 2
					frame := append([]byte(nil), out[:end]...)
					return frame, nil
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("camera: mjpeg timeout")
}

func index2(b []byte, a0, a1 byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == a0 && b[i+1] == a1 {
			return i
		}
	}
	return -1
}

func (c *CameraCapture) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func EncodeB64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
