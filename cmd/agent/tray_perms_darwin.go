//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework Foundation -framework AppKit
#include "tray_perms_panel_darwin.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
)

func trayShowPermissionsPanel() {
	markPermsUIShown()
	title := C.CString(i18n.T("perm.window_title"))
	sub := C.CString(i18n.T("perm.window_sub"))
	btnSet := C.CString(i18n.T("perm.btn_settings"))
	btnDone := C.CString(i18n.T("perm.btn_done"))
	btnDoneRestart := C.CString(i18n.T("perm.btn_done_restart"))
	hint := C.CString(i18n.T("perm.auto_refresh"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(sub))
	defer C.free(unsafe.Pointer(btnSet))
	defer C.free(unsafe.Pointer(btnDone))
	defer C.free(unsafe.Pointer(btnDoneRestart))
	defer C.free(unsafe.Pointer(hint))
	C.re_perms_panel_show(title, sub, btnSet, btnDone, btnDoneRestart, hint)
}

//export rePermsStatusJSON
func rePermsStatusJSON() *C.char {
	return C.CString(permsRowsJSON())
}

//export rePermsOpenID
func rePermsOpenID(cid *C.char) {
	id := C.GoString(cid)
	switch id {
	case "screen":
		// System permission sheet (may offer “Open System Settings”); do not jump to Settings ourselves.
		_ = desktop.RequestScreenRecording()
	case "accessibility":
		_ = desktop.RequestAccessibility()
	case "microphone":
		// Mic/camera use AVFoundation: prompt only while NotDetermined.
		// After Deny, OS won't re-prompt — open Privacy pane so the user can flip it.
		if desktop.MicrophoneCanPrompt() || desktop.CheckHostPermissions().Microphone {
			_ = desktop.RequestMicrophone()
		} else {
			_ = desktop.OpenPrivacySettings("microphone")
		}
	case "camera":
		if desktop.CameraCanPrompt() || desktop.CheckHostPermissions().Camera {
			_ = desktop.RequestCamera()
		} else {
			_ = desktop.OpenPrivacySettings("camera")
		}
	case "phonecam":
		// Optional virtual-webcam driver — runs the bundled local installer
		// (macOS Installer.app prompts for an admin password).
		openPhoneCamPermSettings()
	}
}

//export rePermsRelaunch
func rePermsRelaunch() {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return
	}
	// Delay start so this process can exit and release agent.lock first.
	script := fmt.Sprintf("sleep 0.6; exec %q tray", exe)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		os.Exit(0)
	}()
}
