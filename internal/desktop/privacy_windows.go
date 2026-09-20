//go:build windows

package desktop

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowDisplayAffinity   = user32.NewProc("SetWindowDisplayAffinity")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procGetModuleHandleW     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	privacyClassOnce         uintptr
)

const (
	wsExTopmost      = 0x00000008
	wsExToolwindow   = 0x00000080
	wsExLayered      = 0x00080000
	wsPopup          = 0x80000000
	wsVisible        = 0x10000000
	swShow           = 5
	lwaAlpha         = 0x2
	hwndTopmost      = ^uintptr(0) // HWND_TOPMOST = -1
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	wdaExcludeCapture = 0x00000011
	smCXVirtual      = 78
	smCYVirtual      = 79
	smXVirtual       = 76
	smYVirtual       = 77
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type msgWin struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func startPrivacyBlankOS() (func(), error) {
	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		className, _ := syscall.UTF16PtrFromString("REPrivacyBlank")
		hInst, _, _ := procGetModuleHandleW.Call(0)
		if privacyClassOnce == 0 {
			wndProc := syscall.NewCallback(func(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
				if msg == 0x0010 { // WM_CLOSE
					return 0
				}
				r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
				return r
			})
			wc := wndClassExW{
				Size:      uint32(unsafe.Sizeof(wndClassExW{})),
				WndProc:   wndProc,
				Instance:  syscall.Handle(hInst),
				Background: 4, // COLOR_WINDOW stock brush index +1 → black via null; use 4=panel
				ClassName: className,
			}
			// Black brush
			gdi := syscall.NewLazyDLL("gdi32.dll")
			createSolidBrush := gdi.NewProc("CreateSolidBrush")
			br, _, _ := createSolidBrush.Call(0x00000000)
			wc.Background = syscall.Handle(br)
			procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
			privacyClassOnce = 1
		}

		vx, _, _ := procGetSystemMetrics.Call(smXVirtual)
		vy, _, _ := procGetSystemMetrics.Call(smYVirtual)
		vw, _, _ := procGetSystemMetrics.Call(smCXVirtual)
		vh, _, _ := procGetSystemMetrics.Call(smCYVirtual)
		if vw == 0 {
			vw, _, _ = procGetSystemMetrics.Call(0)
			vh, _, _ = procGetSystemMetrics.Call(1)
		}
		title, _ := syscall.UTF16PtrFromString("")
		hwnd, _, _ := procCreateWindowExW.Call(
			wsExTopmost|wsExToolwindow|wsExLayered,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(title)),
			wsPopup|wsVisible,
			vx, vy, vw, vh,
			0, 0, hInst, 0,
		)
		if hwnd == 0 {
			<-stopCh
			return
		}
		procSetLayeredWindowAttributes.Call(hwnd, 0, 255, lwaAlpha)
		procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeCapture)
		procSetWindowPos.Call(hwnd, hwndTopmost, vx, vy, vw, vh, swpNoActivate|swpShowWindow)
		procShowWindow.Call(hwnd, swShow)
		procUpdateWindow.Call(hwnd)

		go func() {
			<-stopCh
			procDestroyWindow.Call(hwnd)
			procPostQuitMessage.Call(0)
		}()

		var msg msgWin
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	return func() {
		close(stopCh)
		<-done
	}, nil
}
