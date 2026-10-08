//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework Foundation -framework AppKit
#include "tray_ui_darwin.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func trayApplyAppIcon() {
	if len(trayIconColorPNG) == 0 {
		return
	}
	C.re_ui_set_app_icon_png((*C.uchar)(unsafe.Pointer(&trayIconColorPNG[0])), C.int(len(trayIconColorPNG)))
}

func trayShowStatusWindow() {
	title := C.CString(i18n.T("ui.status_title"))
	body := C.CString(statusReportText())
	closeBtn := C.CString(i18n.T("perm.btn_done"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	defer C.free(unsafe.Pointer(closeBtn))
	C.re_ui_show_text_window(title, body, closeBtn, 580)
}

func trayShowHelpWindow() {
	title := C.CString(i18n.T("ui.help_title"))
	body := C.CString(helpReportText())
	closeBtn := C.CString(i18n.T("perm.btn_done"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	defer C.free(unsafe.Pointer(closeBtn))
	C.re_ui_show_text_window(title, body, closeBtn, 760)
}

func trayShowAboutWindow() {
	title := C.CString(i18n.T("ui.about_title"))
	name := C.CString(i18n.T("ui.app_name"))
	ver := C.CString(aboutVersionText())
	git := C.CString(i18n.T("ui.github_url"))
	mail := C.CString(i18n.T("ui.contact_email"))
	check := C.CString(i18n.T("ui.check_update"))
	closeBtn := C.CString(i18n.T("perm.btn_done"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(ver))
	defer C.free(unsafe.Pointer(git))
	defer C.free(unsafe.Pointer(mail))
	defer C.free(unsafe.Pointer(check))
	defer C.free(unsafe.Pointer(closeBtn))
	var iconPtr *C.uchar
	iconLen := 0
	if len(trayIconColorPNG) > 0 {
		iconPtr = (*C.uchar)(unsafe.Pointer(&trayIconColorPNG[0]))
		iconLen = len(trayIconColorPNG)
	}
	C.re_ui_show_about_window(title, name, ver, iconPtr, C.int(iconLen), git, mail, check, closeBtn)
}

func trayShowConfigWindow() {
	relay, pub, manual, share := configFormDefaults()
	title := C.CString(i18n.T("ui.config_title"))
	rl := C.CString(i18n.T("ui.config_relay"))
	pl := C.CString(i18n.T("ui.config_public"))
	ml := C.CString(i18n.T("ui.config_manual"))
	mh := C.CString(i18n.T("ui.config_manual_hint"))
	sl := C.CString(i18n.T("ui.config_share"))
	sh := C.CString(i18n.T("ui.config_share_hint"))
	rv := C.CString(relay)
	pv := C.CString(pub)
	save := C.CString(i18n.T("ui.config_save"))
	cancel := C.CString(i18n.T("ui.config_cancel"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(rl))
	defer C.free(unsafe.Pointer(pl))
	defer C.free(unsafe.Pointer(ml))
	defer C.free(unsafe.Pointer(mh))
	defer C.free(unsafe.Pointer(sl))
	defer C.free(unsafe.Pointer(sh))
	defer C.free(unsafe.Pointer(rv))
	defer C.free(unsafe.Pointer(pv))
	defer C.free(unsafe.Pointer(save))
	defer C.free(unsafe.Pointer(cancel))
	man, shareI := 0, 0
	if manual {
		man = 1
	}
	if share {
		shareI = 1
	}
	C.re_ui_show_config_window(title, rl, pl, ml, mh, sl, sh, rv, pv, C.int(man), C.int(shareI), save, cancel)
}

//export reUIStatusText
func reUIStatusText() *C.char { return C.CString(statusReportText()) }

//export reUIHelpText
func reUIHelpText() *C.char { return C.CString(helpReportText()) }

//export reUIAboutVersion
func reUIAboutVersion() *C.char { return C.CString(aboutVersionText()) }

//export reUICheckUpdate
func reUICheckUpdate() *C.char {
	msg, url, newer := checkForUpdate()
	flag := "0"
	if newer {
		flag = "1"
	}
	return C.CString(fmt.Sprintf("%s|%s|%s", flag, msg, url))
}

//export reUIOpenURL
func reUIOpenURL(url *C.char) {
	openUpdateURL(C.GoString(url))
}

//export reUIConfigLoad
func reUIConfigLoad() *C.char {
	relay, pub, manual, share := configFormDefaults()
	m, s := "0", "0"
	if manual {
		m = "1"
	}
	if share {
		s = "1"
	}
	return C.CString(fmt.Sprintf("%s\x1f%s\x1f%s\x1f%s", relay, pub, m, s))
}

//export reUIConfigSave
func reUIConfigSave(relay, pub *C.char, manual, share C.int) *C.char {
	err := saveConfigFromUI(C.GoString(relay), C.GoString(pub), manual != 0, share != 0)
	if err != nil {
		return C.CString(err.Error())
	}
	return C.CString("")
}

// QR window refresh is owned by a Go goroutine so mint/write never runs via cgo
// entered from a GCD/AppKit thread (deadlocks with systray LockOSThread).

var (
	qrRefreshMu     sync.Mutex
	qrRefreshReq    chan struct{}
	qrRefreshCancel chan struct{}
	qrRefreshBusy   bool
)

func ensureQRRefreshLoop() {
	qrRefreshMu.Lock()
	defer qrRefreshMu.Unlock()
	if qrRefreshReq != nil {
		return
	}
	qrRefreshReq = make(chan struct{}, 1)
	qrRefreshCancel = make(chan struct{})
	req, cancel := qrRefreshReq, qrRefreshCancel
	go func() {
		for {
			select {
			case <-cancel:
				return
			case <-req:
				runQRMintAndPush()
			}
		}
	}()
}

func stopQRRefreshLoop() {
	qrRefreshMu.Lock()
	cancel := qrRefreshCancel
	qrRefreshReq = nil
	qrRefreshCancel = nil
	qrRefreshBusy = false
	qrRefreshMu.Unlock()
	if cancel != nil {
		close(cancel)
	}
}

func runQRMintAndPush() {
	qrRefreshMu.Lock()
	if qrRefreshBusy {
		qrRefreshMu.Unlock()
		return
	}
	qrRefreshBusy = true
	qrRefreshMu.Unlock()
	defer func() {
		qrRefreshMu.Lock()
		qrRefreshBusy = false
		qrRefreshMu.Unlock()
	}()

	a := getTrayAgent()
	if a == nil {
		pushQRUpdate(nil, "", 0, i18n.T("tray.not_running"))
		return
	}
	png, link, exp, err := encodePairQRPNG(a)
	if err != nil {
		pushQRUpdate(nil, "", 0, err.Error())
		return
	}
	_ = traySetClipboard(link)
	pushQRUpdate(png, link, exp, "")
}

func pushQRUpdate(png []byte, link string, exp int64, errMsg string) {
	var pngPtr *C.uchar
	if len(png) > 0 {
		pngPtr = (*C.uchar)(unsafe.Pointer(&png[0]))
	}
	linkC := C.CString(link)
	errC := C.CString(errMsg)
	defer C.free(unsafe.Pointer(linkC))
	defer C.free(unsafe.Pointer(errC))
	C.re_ui_qr_apply(pngPtr, C.int(len(png)), C.longlong(exp), linkC, errC)
	runtime.KeepAlive(png)
}

//export reUIRequestPairQRRefresh
func reUIRequestPairQRRefresh() {
	ensureQRRefreshLoop()
	qrRefreshMu.Lock()
	req := qrRefreshReq
	qrRefreshMu.Unlock()
	if req == nil {
		return
	}
	select {
	case req <- struct{}{}:
	default:
	}
}

//export reUIQRWindowClosed
func reUIQRWindowClosed() {
	stopQRRefreshLoop()
}

// trayCloseQRWindow dismisses the pairing QR after the phone connects successfully.
func trayCloseQRWindow() {
	C.re_ui_close_qr_window()
}

func trayShowQRWindow() error {
	a := getTrayAgent()
	if a == nil {
		return fmt.Errorf("%s", i18n.T("tray.not_running"))
	}
	png, link, exp, err := encodePairQRPNG(a)
	if err != nil {
		return err
	}
	title := C.CString(i18n.T("ui.qr_title"))
	expiresFmt := C.CString(i18n.T("ui.qr_expires_in"))
	refreshing := C.CString(i18n.T("ui.qr_refreshing"))
	btnRefresh := C.CString(i18n.T("ui.qr_refresh"))
	btnClose := C.CString(i18n.T("perm.btn_done"))
	linkC := C.CString(link)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(expiresFmt))
	defer C.free(unsafe.Pointer(refreshing))
	defer C.free(unsafe.Pointer(btnRefresh))
	defer C.free(unsafe.Pointer(btnClose))
	defer C.free(unsafe.Pointer(linkC))
	var pngPtr *C.uchar
	if len(png) > 0 {
		pngPtr = (*C.uchar)(unsafe.Pointer(&png[0]))
	}
	ensureQRRefreshLoop()
	C.re_ui_show_qr_window(title, pngPtr, C.int(len(png)), C.longlong(exp), linkC,
		expiresFmt, refreshing, btnRefresh, btnClose)
	runtime.KeepAlive(png)
	_ = traySetClipboard(link)
	return nil
}
