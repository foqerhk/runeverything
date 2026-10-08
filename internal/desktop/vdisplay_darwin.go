//go:build darwin

package desktop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Virtual display helper (re-vdisplay) IPC manager.
// The helper process retains CGVirtualDisplay objects; killing it drops the displays.

type vdisplayResp struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	DisplayID uint32 `json:"display_id,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	FBWidth   int    `json:"fb_width,omitempty"`
	FBHeight  int    `json:"fb_height,omitempty"`
	Reused    bool   `json:"reused,omitempty"`
	API       bool   `json:"api,omitempty"`
	Displays  []struct {
		DisplayID uint32 `json:"display_id"`
		Mode      string `json:"mode"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		FBWidth   int    `json:"fb_width"`
		FBHeight  int    `json:"fb_height"`
	} `json:"displays,omitempty"`
}

type vdisplayMgr struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	mode    string // active profile, if any
	cgID    uint32
	width   int
	height  int
	fbW     int
	fbH     int
	binPath string
}

var (
	vdOnce sync.Once
	vdInst *vdisplayMgr
)

func vdisplay() *vdisplayMgr {
	vdOnce.Do(func() { vdInst = &vdisplayMgr{} })
	return vdInst
}

// VDisplayAvailable reports whether the helper binary exists (API probed on first call).
func VDisplayAvailable() bool {
	_, err := resolveVDisplayBin()
	return err == nil
}

func resolveVDisplayBin() (string, error) {
	if p := strings.TrimSpace(os.Getenv("RE_VDISPLAY_BIN")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("RE_VDISPLAY_BIN not found: %s", p)
	}
	// Same bundle as Agent: .../RunEverything.app/Contents/MacOS/re-vdisplay
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		cand := filepath.Join(filepath.Dir(exe), "re-vdisplay")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	// Dev fallbacks
	for _, cand := range []string{
		"/tmp/re-vdisplay",
		filepath.Join("bin", "re-vdisplay"),
		filepath.Join("cmd", "re-vdisplay", "re-vdisplay"),
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf("re-vdisplay helper not found (set RE_VDISPLAY_BIN or install app bundle)")
}

func (m *vdisplayMgr) ensureHelperLocked() error {
	if m.cmd != nil && m.cmd.Process != nil {
		// Cheap liveness: ping
		if _, err := m.roundTripLocked(map[string]any{"cmd": "ping"}, 2*time.Second); err == nil {
			return nil
		}
		m.killLocked()
	}
	bin, err := resolveVDisplayBin()
	if err != nil {
		return err
	}
	m.binPath = bin
	cmd := exec.Command(bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start re-vdisplay: %w", err)
	}
	m.cmd = cmd
	m.stdin = stdin
	m.stdout = bufio.NewReader(stdout)
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
			m.stdin = nil
			m.stdout = nil
			m.cgID = 0
			m.mode = ""
		}
		m.mu.Unlock()
	}()
	resp, err := m.roundTripLocked(map[string]any{"cmd": "ping"}, 3*time.Second)
	if err != nil {
		m.killLocked()
		return err
	}
	if !resp.OK {
		m.killLocked()
		return fmt.Errorf("re-vdisplay ping: %s", resp.Error)
	}
	if !resp.API {
		m.killLocked()
		return fmt.Errorf("CGVirtualDisplay API unavailable")
	}
	return nil
}

func (m *vdisplayMgr) killLocked() {
	if m.stdin != nil {
		_, _ = io.WriteString(m.stdin, "{\"cmd\":\"quit\"}\n")
		_ = m.stdin.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
	m.cmd = nil
	m.stdin = nil
	m.stdout = nil
	m.cgID = 0
	m.mode = ""
	m.width, m.height = 0, 0
	m.fbW, m.fbH = 0, 0
}

func (m *vdisplayMgr) roundTripLocked(req map[string]any, timeout time.Duration) (*vdisplayResp, error) {
	if m.stdin == nil || m.stdout == nil {
		return nil, fmt.Errorf("helper not running")
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := m.stdin.Write(b); err != nil {
		return nil, err
	}
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := m.stdout.ReadString('\n')
		ch <- result{line, err}
	}()
	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("helper timeout")
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		var resp vdisplayResp
		if err := json.Unmarshal([]byte(strings.TrimSpace(r.line)), &resp); err != nil {
			return nil, fmt.Errorf("helper json: %w (%q)", err, strings.TrimSpace(r.line))
		}
		return &resp, nil
	}
}

// EnsureVirtual creates or reuses a virtual display for mode (5k|8k|16k).
func EnsureVirtual(mode string) (cgID uint32, w, h int, err error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "5k", "8k", "16k":
	default:
		return 0, 0, 0, fmt.Errorf("unsupported mode %q (use 5k|8k|16k)", mode)
	}
	m := vdisplay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode == mode && m.cgID != 0 {
		return m.cgID, m.width, m.height, nil
	}
	if err := m.ensureHelperLocked(); err != nil {
		return 0, 0, 0, err
	}
	resp, err := m.roundTripLocked(map[string]any{"cmd": "create", "mode": mode}, 15*time.Second)
	if err != nil {
		return 0, 0, 0, err
	}
	if !resp.OK {
		return 0, 0, 0, fmt.Errorf("create: %s", resp.Error)
	}
	m.mode = mode
	m.cgID = resp.DisplayID
	m.width = resp.Width
	m.height = resp.Height
	m.fbW = resp.FBWidth
	m.fbH = resp.FBHeight
	log.Printf("vdisplay: ready mode=%s cgID=%d logical=%dx%d fb=%dx%d reused=%v",
		mode, m.cgID, m.width, m.height, m.fbW, m.fbH, resp.Reused)
	return m.cgID, m.width, m.height, nil
}

// DestroyVirtual removes one virtual display by CGDirectDisplayID.
func DestroyVirtual(cgID uint32) error {
	m := vdisplay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureHelperLocked(); err != nil {
		return err
	}
	resp, err := m.roundTripLocked(map[string]any{"cmd": "destroy", "display_id": cgID}, 5*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("destroy: %s", resp.Error)
	}
	if m.cgID == cgID {
		m.cgID = 0
		m.mode = ""
	}
	return nil
}

// DestroyAllVirtual tears down helper-owned virtual displays.
func DestroyAllVirtual() error {
	m := vdisplay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil {
		return nil
	}
	_, _ = m.roundTripLocked(map[string]any{"cmd": "destroy_all"}, 5*time.Second)
	m.killLocked()
	return nil
}

// VirtualDisplayIDs returns CGDirectDisplayIDs currently owned by the helper.
func VirtualDisplayIDs() map[uint32]string {
	m := vdisplay()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[uint32]string{}
	if m.cgID != 0 && m.mode != "" {
		out[m.cgID] = m.mode
	}
	return out
}

// VirtualFramebuffer returns framebuffer pixel size for a helper-owned display.
func VirtualFramebuffer(id int) (fbW, fbH int, ok bool) {
	m := vdisplay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if id > 0 && m.cgID == uint32(id) && m.fbW > 0 {
		return m.fbW, m.fbH, true
	}
	return 0, 0, false
}

// IsVirtualDisplay reports whether id is a helper-owned virtual display.
func IsVirtualDisplay(id int) bool {
	if id <= 0 {
		return false
	}
	_, ok := VirtualDisplayIDs()[uint32(id)]
	return ok
}

// StartVirtualFromEnv creates a virtual display when RE_VDISPLAY=5k|8k|16k.
// Returns without error when unset/off.
func StartVirtualFromEnv() error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RE_VDISPLAY")))
	if mode == "" || mode == "0" || mode == "off" || mode == "false" || mode == "no" {
		return nil
	}
	_, _, _, err := EnsureVirtual(mode)
	return err
}
