//go:build darwin

package desktop

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Darwin sink: spawn re-phonecam helper which shows a preview window and
// publishes frames into the "KoKo Phone Camera" virtual webcam (CMIO / AkVCam).
type darwinPhoneCamSink struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	device string
}

func startPhoneCamSinkImpl(width, height, fps int) (phoneCamSinkImpl, error) {
	if phoneCamVisibleDeviceName() == "" && !phoneCamExtensionActive() {
		// DAL-only installs are invisible to browsers on macOS 14.1+; require Camera Extension.
		if _, err := os.Stat("/Applications/AkVirtualCameraCX.app"); err == nil {
			return nil, fmt.Errorf("phonecam: Camera Extension not active — open AkVirtualCameraCX → Install extension, allow it in System Settings, then retry")
		}
		return nil, fmt.Errorf("phonecam: virtual camera not visible to apps — enable「用手机当摄像头」in permissions (Camera Extension required on this macOS)")
	}
	// Ensure virtual device exists even when profiler has not listed it yet.
	ensureKoKoPhoneCamDevice()
	bin, err := resolvePhoneCamBin()
	if err != nil {
		return nil, err
	}
	// Browsers typically open the first landscape format (1280x720). Streaming the
	// phone's native portrait size at full RGB32 saturated the pipe (~50MB/s) and
	// stalled the agent — keep a fixed, browser-friendly output canvas instead.
	_ = width
	_ = height
	outW, outH, outFPS := 1280, 720, fps
	if outFPS <= 0 {
		outFPS = 10
	}
	if outFPS > 15 {
		outFPS = 15
	}
	cmd := exec.Command(bin, "--width", itoa(outW), "--height", itoa(outH), "--fps", itoa(outFPS), "--no-preview")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("phonecam: start helper: %w", err)
	}
	s := &darwinPhoneCamSink{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		device: "KoKo Phone Camera",
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "READY ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "READY "))
			if name != "" {
				s.device = name
			}
			go s.drainStdout()
			return s, nil
		}
		if strings.HasPrefix(line, "ERROR ") {
			_ = s.close()
			return nil, fmt.Errorf("phonecam: %s", strings.TrimPrefix(line, "ERROR "))
		}
	}
	go s.drainStdout()
	return s, nil
}

func (s *darwinPhoneCamSink) drainStdout() {
	for {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "READY ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "READY "))
			if name != "" {
				s.mu.Lock()
				s.device = name
				s.mu.Unlock()
			}
		}
	}
}

func (s *darwinPhoneCamSink) deviceName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device
}

func (s *darwinPhoneCamSink) writeJPEG(jpeg []byte, width, height int) error {
	return s.writeFrame("PCAM", jpeg, width, height, false)
}

func (s *darwinPhoneCamSink) writeH264(annexB []byte, width, height int, key bool) error {
	return s.writeFrame("PCAH", annexB, width, height, key)
}

func (s *darwinPhoneCamSink) writeFrame(magic string, payload []byte, width, height int, key bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin == nil {
		return fmt.Errorf("phonecam: no stdin")
	}
	// Frame header: magic + w + h + len (u32le) + payload.
	// For PCAH (H264), width high bit marks keyframe so the helper can wait for IDR.
	var hdr [16]byte
	copy(hdr[0:4], []byte(magic))
	w := uint32(width)
	if magic == "PCAH" && key {
		w |= 1 << 31
	}
	binary.LittleEndian.PutUint32(hdr[4:8], w)
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(height))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(len(payload)))
	if _, err := s.stdin.Write(hdr[:]); err != nil {
		return err
	}
	_, err := s.stdin.Write(payload)
	return err
}

func (s *darwinPhoneCamSink) close() error {
	s.mu.Lock()
	stdin := s.stdin
	cmd := s.cmd
	s.stdin = nil
	s.cmd = nil
	s.mu.Unlock()
	if stdin != nil {
		var hdr [16]byte
		copy(hdr[0:4], []byte("PCAM"))
		_, _ = stdin.Write(hdr[:])
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(800 * time.Millisecond):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	return nil
}

func resolvePhoneCamBin() (string, error) {
	if p := strings.TrimSpace(os.Getenv("RE_PHONECAM_BIN")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("RE_PHONECAM_BIN not found: %s", p)
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "re-phonecam")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	for _, cand := range []string{
		"/Applications/RunEverything.app/Contents/MacOS/re-phonecam",
		"/tmp/re-phonecam",
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf("phonecam: re-phonecam helper not found (build scripts/build-re-phonecam.sh)")
}
