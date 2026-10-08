//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/winutil"
	"golang.org/x/sys/windows"
)

// Set via -ldflags
var version = "dev"

func main() {
	title := "RunEverything Setup"
	msg := fmt.Sprintf(
		"Install RunEverything Agent %s?\n\n"+
			"• Installs to %%LOCALAPPDATA%%\\RunEverything\\bin\n"+
			"• Adds Start Menu shortcut\n"+
			"• Starts with Windows (tray)\n"+
			"• No admin required",
		version,
	)
	if messageBox(title, msg, windows.MB_OKCANCEL|windows.MB_ICONINFORMATION) != 1 { // IDOK
		return
	}

	if err := install(); err != nil {
		messageBox(title, "Install failed:\n"+err.Error(), windows.MB_OK|windows.MB_ICONERROR)
		os.Exit(1)
	}

	exe := winutil.AgentPath()
	launch := messageBox(title,
		"Installed successfully.\n\nStart RunEverything now? (tray icon — use “Show pairing QR” to pair)",
		windows.MB_YESNO|windows.MB_ICONINFORMATION,
	)
	if launch == 6 { // IDYES
		cmd := exec.Command(exe, "tray")
		cmd.Dir = filepath.Dir(exe)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Start()
		time.Sleep(800 * time.Millisecond)
	}
	messageBox(title,
		"Done.\n\nLook for the tray icon near the clock.\nRight-click → Show pairing QR.",
		windows.MB_OK|windows.MB_ICONINFORMATION,
	)
}

func install() error {
	if err := winutil.EnsureInstallDir(); err != nil {
		return err
	}
	dest := winutil.AgentPath()
	data, err := embeddedAgent()
	if err != nil {
		return err
	}
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		// fallback copy
		if err2 := os.WriteFile(dest, data, 0o755); err2 != nil {
			return err2
		}
		_ = os.Remove(tmp)
	}

	_ = winutil.AddUserPath(winutil.InstallDir())
	_ = winutil.CreateStartMenuShortcut(dest)
	if err := winutil.RegisterLogonTask(dest); err != nil {
		// non-fatal: still usable via Start Menu
		fmt.Fprintf(os.Stderr, "autostart warning: %v\n", err)
	}
	_ = installPhoneCamDriverBundle()

	// default config
	home, err := identity.EnsureHome()
	if err == nil {
		cfgPath := filepath.Join(home, "config.json")
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			cfg := &identity.Config{}
			_ = identity.SaveConfig(cfg)
		}
	}
	return nil
}

// installPhoneCamDriverBundle copies the optional AkVirtualCamera installer next
// to the Agent so “用手机当摄像头 → 去设置” can run it locally with UAC.
func installPhoneCamDriverBundle() error {
	self, _ := os.Executable()
	cands := []string{
		filepath.Join(filepath.Dir(self), "akvirtualcamera-windows.exe"),
		filepath.Join(filepath.Dir(self), "akvirtualcamera", "akvirtualcamera-windows.exe"),
		os.Getenv("RE_AKVCAM_EXE"),
	}
	var src string
	for _, p := range cands {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			src = p
			break
		}
	}
	if src == "" {
		return nil
	}
	dstDir := filepath.Join(filepath.Dir(winutil.InstallDir()), "akvirtualcamera")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dstDir, "akvirtualcamera-windows.exe"), data, 0o755)
}

func messageBox(title, text string, flags uint32) int {
	t, _ := windows.UTF16PtrFromString(title)
	b, _ := windows.UTF16PtrFromString(text)
	r, _ := windows.MessageBox(0, b, t, flags)
	return int(r)
}
