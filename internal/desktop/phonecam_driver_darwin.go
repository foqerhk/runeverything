//go:build darwin

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func phoneCamDriverStatus() PhoneCamDriverStatus {
	if name := phoneCamVisibleDeviceName(); name != "" {
		return PhoneCamDriverStatus{Installed: true, Name: name}
	}
	// Camera Extension can be activated before system_profiler enumerates a
	// virtual device (device appears after AkVCamManager add-device / first use).
	if phoneCamExtensionActive() {
		return PhoneCamDriverStatus{Installed: true, Name: "KoKo Phone Camera (Camera Extension)"}
	}
	cands := []string{
		"/Applications/AkVirtualCameraCX.app",
		"/Library/CoreMediaIO/Plug-Ins/DAL/AkVirtualCamera.plugin",
		"/Applications/AkVirtualCamera/AkVirtualCamera.plugin",
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return PhoneCamDriverStatus{Installed: false, Name: "AkVirtualCamera (需在 AkVirtualCameraCX 中 Install extension)"}
		}
	}
	return PhoneCamDriverStatus{}
}

func phoneCamExtensionActive() bool {
	out, err := exec.Command("systemextensionsctl", "list").Output()
	if err != nil {
		return false
	}
	s := string(out)
	// Prefer our bundle id; fall back to CMIO category + activated.
	if strings.Contains(s, "com.foqerhk.runeverything.vcamCX.Extension") &&
		strings.Contains(s, "[activated enabled]") {
		return true
	}
	return strings.Contains(s, "com.apple.system_extension.cmio") &&
		strings.Contains(s, "vcamCX.Extension") &&
		strings.Contains(s, "activated")
}

func phoneCamVisibleDeviceName() string {
	out, err := exec.Command("system_profiler", "SPCameraDataType", "-detailLevel", "mini").Output()
	if err != nil {
		return ""
	}
	s := string(out)
	for _, needle := range []string{"KoKo Phone Camera", "AkVirtualCamera", "OBS Virtual Camera", "Virtual Camera"} {
		if strings.Contains(s, needle) {
			return needle
		}
	}
	return ""
}

func openPhoneCamDriverInstall() error {
	payload, err := resolvePhoneCamMacPayload()
	if err != nil {
		return err
	}
	plugin := filepath.Join(payload, "AkVirtualCamera.plugin")
	cx := filepath.Join(payload, "AkVirtualCameraCX.app")
	if _, err := os.Stat(plugin); err != nil {
		return fmt.Errorf("bundled AkVirtualCamera.plugin missing under %s", payload)
	}
	if _, err := os.Stat(cx); err != nil {
		return fmt.Errorf("bundled AkVirtualCameraCX.app missing under %s", payload)
	}

	// If CX is already installed, bring it forward and tell the user what to do next.
	// Always open the /Applications copy by path — never Launch Services name lookup,
	// which can resolve to an unsigned/stale copy under the source tree.
	if _, err := os.Stat("/Applications/AkVirtualCameraCX.app"); err == nil {
		_ = exec.Command("open", "/Applications/AkVirtualCameraCX.app").Start()
		_ = exec.Command("osascript", "-e",
			`display dialog "已打开 AkVirtualCameraCX。

请点击窗口里的「Install extension」，再到系统设置允许摄像头扩展。

完成后在网页/会议软件里选择摄像头「KoKo Phone Camera」。" buttons {"好"} default button 1 with title "用手机当摄像头"`).Start()
		go func() {
			time.Sleep(500 * time.Millisecond)
			ensureKoKoPhoneCamDevice()
		}()
		return nil
	}

	sh, err := writePhoneCamInstallShell(plugin, cx)
	if err != nil {
		return err
	}
	scpt, err := writePhoneCamInstallAppleScript(sh)
	if err != nil {
		return err
	}

	// Run asynchronously but surface AppleScript errors in a dialog (script itself shows them).
	cmd := exec.Command("osascript", scpt)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
		time.Sleep(500 * time.Millisecond)
		ensureKoKoPhoneCamDevice()
	}()
	return nil
}

func writePhoneCamInstallShell(plugin, cx string) (string, error) {
	path := filepath.Join(os.TempDir(), "re-install-akvcam.sh")
	body := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
PLUGIN=%q
CX=%q
mkdir -p /Applications/AkVirtualCamera /Library/CoreMediaIO/Plug-Ins/DAL
rm -rf /Applications/AkVirtualCamera/AkVirtualCamera.plugin \
       /Applications/AkVirtualCameraCX.app \
       /Library/CoreMediaIO/Plug-Ins/DAL/AkVirtualCamera.plugin
/usr/bin/ditto "$PLUGIN" /Applications/AkVirtualCamera/AkVirtualCamera.plugin
/usr/bin/ditto "$CX" /Applications/AkVirtualCameraCX.app
/bin/ln -sf /Applications/AkVirtualCamera/AkVirtualCamera.plugin \
  /Library/CoreMediaIO/Plug-Ins/DAL/AkVirtualCamera.plugin
/usr/bin/xattr -cr /Applications/AkVirtualCameraCX.app /Applications/AkVirtualCamera || true
/bin/chmod a+x /Applications/AkVirtualCamera/AkVirtualCamera.plugin/Contents/Resources/AkVCamManager \
  /Applications/AkVirtualCamera/AkVirtualCamera.plugin/Contents/Resources/AkVCamAssistant || true
`, plugin, cx)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func writePhoneCamInstallAppleScript(shellPath string) (string, error) {
	path := filepath.Join(os.TempDir(), "re-install-akvcam.applescript")
	body := fmt.Sprintf(`
set sh to %s
try
	do shell script ("/bin/bash " & quoted form of sh) with administrator privileges
	do shell script "open /Applications/AkVirtualCameraCX.app"
	display dialog "安装完成。

请在打开的 AkVirtualCameraCX 窗口点击「Install extension」，再到系统设置里允许摄像头扩展。

然后在网页/会议软件的摄像头列表中选择「KoKo Phone Camera」。" buttons {"好"} default button 1 with title "用手机当摄像头"
on error errMsg number errNum
	display dialog ("安装失败 (" & errNum & "): " & errMsg) buttons {"好"} default button 1 with icon stop with title "用手机当摄像头"
end try
`, appleScriptQuote(shellPath))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func ensureKoKoPhoneCamDevice() {
	mgr := "/Applications/AkVirtualCamera/AkVirtualCamera.plugin/Contents/Resources/AkVCamManager"
	if st, err := os.Stat(mgr); err != nil || st.IsDir() {
		return
	}
	// Sandboxed Camera Extension cannot attach POSIX shm from AkVCamAssistant;
	// sockets keeps frames on the TCP MessageClient path (needs network.client).
	_ = exec.Command(mgr, "set-data-mode", "sockets").Run()
	_ = exec.Command(mgr, "add-device", "-i", "KoKoPhoneCam0", "KoKo Phone Camera").Run()
	// add-format DEVICE FORMAT WIDTH HEIGHT FPS
	// (-i is format index, not device id — wrong flags left the device with zero formats
	// so AVFoundation/browsers never listed "KoKo Phone Camera".)
	// RGB32 maps to CMIO 32ARGB and is what browsers reliably enumerate.
	out, _ := exec.Command(mgr, "formats", "KoKoPhoneCam0").CombinedOutput()
	s := string(out)
	// Prefer RGB32@30 as format 0 — browsers often bind the first format; RGB24-first
	// plus an RGB32 producer historically yielded a blank preview.
	if !strings.Contains(s, "RGB32 1280x720 30") {
		_ = exec.Command(mgr, "add-format", "KoKoPhoneCam0", "RGB32", "1280", "720", "30").Run()
	}
	_ = exec.Command(mgr, "update").Run()
}

func resolvePhoneCamMacPayload() (string, error) {
	var roots []string
	if exe, err := os.Executable(); err == nil {
		exe, _ := filepath.EvalSymlinks(exe)
		dir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(dir, "..", "Resources", "akvirtualcamera", "macos-payload"),
			filepath.Join(dir, "akvirtualcamera", "macos-payload"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots,
			filepath.Join(wd, "packaging", "akvirtualcamera", "macos-payload"),
			filepath.Join(wd, "..", "packaging", "akvirtualcamera", "macos-payload"),
			filepath.Join(wd, "..", "..", "packaging", "akvirtualcamera", "macos-payload"),
		)
	}
	for _, root := range roots {
		p := filepath.Clean(root)
		if st, err := os.Stat(filepath.Join(p, "AkVirtualCameraCX.app")); err == nil && st.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func appleScriptQuote(s string) string {
	out := make([]byte, 0, len(s)+8)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"':
			out = append(out, '\\', c)
		default:
			out = append(out, c)
		}
	}
	out = append(out, '"')
	return string(out)
}
