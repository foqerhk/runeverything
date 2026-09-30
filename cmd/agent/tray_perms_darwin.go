//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework Foundation -framework AppKit
#include "tray_perms_panel_darwin.h"
#include <stdlib.h>
*/
import "C"
import (
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
	hint := C.CString(i18n.T("perm.auto_refresh"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(sub))
	defer C.free(unsafe.Pointer(btnSet))
	defer C.free(unsafe.Pointer(btnDone))
	defer C.free(unsafe.Pointer(hint))
	C.re_perms_panel_show(title, sub, btnSet, btnDone, hint)
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
		_ = desktop.RequestScreenRecording()
		_ = desktop.OpenPrivacySettings("screen")
	case "accessibility":
		_ = desktop.RequestAccessibility()
		_ = desktop.OpenPrivacySettings("accessibility")
	case "microphone", "camera":
		_ = desktop.OpenPrivacySettings(id)
	}
}
