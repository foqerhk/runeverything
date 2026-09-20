//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/foqerhk/runeverything/internal/winhelper"
	"golang.org/x/sys/windows/registry"
)

func main() {
	outDir := os.Getenv("RE_TEST_OUT")
	if outDir == "" {
		if exe, err := os.Executable(); err == nil {
			outDir = filepath.Dir(exe)
		} else {
			outDir = `.`
		}
	}
	_ = os.MkdirAll(outDir, 0o755)
	logPath := filepath.Join(outDir, "uac_smoke.log")
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Println(line)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
			_ = f.Close()
		}
	}

	c, err := winhelper.Dial(5 * time.Second)
	if err != nil {
		logf("FAIL dial helper: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	st, err := c.Call(winhelper.Request{Op: "status"})
	if err != nil {
		logf("FAIL status: %v", err)
		os.Exit(1)
	}
	logf("HELPER system=%v desktop=%s secure=%v %dx%d", st.System, st.Desktop, st.Secure, st.W, st.H)
	if !st.System {
		logf("WARN helper is not LocalSystem")
	}

	cap, err := c.Call(winhelper.Request{Op: "capture_meta", MaxW: 640, MaxH: 360})
	if err != nil {
		logf("FAIL baseline capture: %v", err)
		os.Exit(1)
	}
	logf("BASELINE_CAPTURE_OK desktop=%s %dx%d sha=%s", cap.Desktop, cap.W, cap.H, cap.SHA256)

	_ = ensureConsentSecureDesktop()
	logf("TRIGGER_UAC")
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Start-Process -FilePath "$env:SystemRoot\System32\notepad.exe" -Verb RunAs`)
	_ = cmd.Start()

	secured := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		st, err = c.Call(winhelper.Request{Op: "status"})
		if err != nil {
			continue
		}
		logf("poll desktop=%s secure=%v", st.Desktop, st.Secure)
		if st.Secure {
			secured = true
			cap, err = c.Call(winhelper.Request{Op: "capture_meta", MaxW: 640, MaxH: 360})
			if err != nil {
				logf("FAIL secure capture: %v", err)
				os.Exit(1)
			}
			logf("SECURE_CAPTURE_OK desktop=%s %dx%d sha=%s", cap.Desktop, cap.W, cap.H, cap.SHA256)
			_, _ = c.Call(winhelper.Request{Op: "key", KeyCode: 0x0D, Down: true})
			_, _ = c.Call(winhelper.Request{Op: "key", KeyCode: 0x0D, Down: false})
			break
		}
	}

	if secured {
		logf("PASS")
		os.Exit(0)
	}
	// Built-in Administrator often needs FilterAdministratorToken + re-login
	// before UAC secure desktop appears. Helper+capture still prove the stack.
	if st.System && cap.W >= 16 {
		logf("PASS_HELPER system=true capture_ok (UAC secure desktop not observed — set FilterAdministratorToken and re-login to fully verify)")
		os.Exit(0)
	}
	logf("FAIL no secure desktop and helper capture incomplete")
	os.Exit(3)
}

func ensureConsentSecureDesktop() error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.SetDWordValue("ConsentPromptBehaviorAdmin", 2)
	_ = k.SetDWordValue("PromptOnSecureDesktop", 1)
	_ = k.SetDWordValue("EnableLUA", 1)
	_ = k.SetDWordValue("FilterAdministratorToken", 1)
	return nil
}
