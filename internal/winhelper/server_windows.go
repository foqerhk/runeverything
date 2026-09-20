//go:build windows

package winhelper

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/windesk"
	"golang.org/x/sys/windows"
)

// ServePipe listens on the helper named pipe forever.
func ServePipe() error {
	l, err := winListenPipe(PipeName, nil)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Minute))
	in := bufio.NewScanner(c)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(c)
	var mu sync.Mutex
	write := func(resp Response) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(resp)
		_, _ = out.Write(append(b, '\n'))
		_ = out.Flush()
	}
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			write(Response{OK: false, Error: "bad json"})
			continue
		}
		write(dispatch(req))
	}
}

func dispatch(req Request) Response {
	switch strings.ToLower(req.Op) {
	case "ping":
		return Response{OK: true, V: 1, System: isLocalSystem()}
	case "status":
		return withDesktop(func(info windesk.Info) Response {
			w, h := screenMetrics()
			return Response{OK: true, V: 1, Desktop: info.Name, Secure: info.Secure, W: w, H: h, System: isLocalSystem()}
		})
	case "capture", "capture_meta", "capture_rgba":
		return withDesktop(func(info windesk.Info) Response {
			img, err := desktop.CaptureOnce(req.MaxW, req.MaxH)
			if err != nil {
				return Response{OK: false, Error: err.Error(), Desktop: info.Name, Secure: info.Secure, System: isLocalSystem()}
			}
			sum := sha256.Sum256(img.Pix)
			resp := Response{
				OK:      true,
				Desktop: info.Name,
				Secure:  info.Secure,
				W:       img.Bounds().Dx(),
				H:       img.Bounds().Dy(),
				SHA256:  hex.EncodeToString(sum[:8]),
				System:  isLocalSystem(),
			}
			if strings.ToLower(req.Op) == "capture_rgba" {
				resp.RGBA = append([]byte(nil), img.Pix...)
			}
			return resp
		})
	case "wheel":
		return withDesktop(func(info windesk.Info) Response {
			inj, err := desktop.NewInjector()
			if err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			defer inj.Close()
			if err := inj.Wheel(req.Delta); err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			return Response{OK: true, Desktop: info.Name, Secure: info.Secure}
		})
	case "move":
		return withDesktop(func(info windesk.Info) Response {
			inj, err := desktop.NewInjector()
			if err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			defer inj.Close()
			if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
				w, h := screenMetrics()
				di.SetScreenSize(w, h)
			}
			if err := inj.Move(req.X, req.Y); err != nil {
				return Response{OK: false, Error: err.Error(), Desktop: info.Name, Secure: info.Secure}
			}
			return Response{OK: true, Desktop: info.Name, Secure: info.Secure}
		})
	case "button":
		return withDesktop(func(info windesk.Info) Response {
			inj, err := desktop.NewInjector()
			if err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			defer inj.Close()
			if err := inj.Button(req.Buttons, req.Down); err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			return Response{OK: true, Desktop: info.Name, Secure: info.Secure}
		})
	case "key":
		return withDesktop(func(info windesk.Info) Response {
			inj, err := desktop.NewInjector()
			if err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			defer inj.Close()
			if err := inj.Key(req.KeyCode, req.Text, req.Down, req.Mods); err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			return Response{OK: true, Desktop: info.Name, Secure: info.Secure}
		})
	default:
		return Response{OK: false, Error: "unknown op"}
	}
}

func withDesktop(fn func(windesk.Info) Response) Response {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	info, restore, err := windesk.AttachInput()
	if err != nil {
		if qi, qerr := windesk.QueryInput(); qerr == nil {
			info = qi
		} else {
			info = windesk.Info{Name: "unknown"}
		}
		resp := fn(info)
		if !resp.OK && resp.Error == "" {
			resp.Error = fmt.Sprintf("attach: %v", err)
		}
		return resp
	}
	defer restore()
	return fn(info)
}

func screenMetrics() (int, int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	get := user32.NewProc("GetSystemMetrics")
	w, _, _ := get.Call(0)
	h, _, _ := get.Call(1)
	return int(w), int(h)
}

func isLocalSystem() bool {
	u := strings.ToUpper(os.Getenv("USERNAME"))
	if u == "SYSTEM" || u == "LOCALSYSTEM" {
		return true
	}
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer tok.Close()
	uinfo, err := tok.GetTokenUser()
	if err != nil {
		return false
	}
	return uinfo.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
}
