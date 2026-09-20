//go:build windows

package windesk

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procOpenInputDesktop          = user32.NewProc("OpenInputDesktop")
	procOpenDesktopW              = user32.NewProc("OpenDesktopW")
	procCloseDesktop              = user32.NewProc("CloseDesktop")
	procSetThreadDesktop          = user32.NewProc("SetThreadDesktop")
	procGetThreadDesktop          = user32.NewProc("GetThreadDesktop")
	procGetUserObjectInformationW = user32.NewProc("GetUserObjectInformationW")
	procOpenWindowStationW        = user32.NewProc("OpenWindowStationW")
	procSetProcessWindowStation   = user32.NewProc("SetProcessWindowStation")
	procGetProcessWindowStation   = user32.NewProc("GetProcessWindowStation")
	procCloseWindowStation        = user32.NewProc("CloseWindowStation")
)

const (
	uoIObjectName = 2

	desktopReadObjects   = 0x0001
	desktopWriteObjects  = 0x0002
	desktopSwitchDesktop = 0x0100
	genericRead          = 0x80000000
	genericWrite         = 0x40000000
	genericAll           = 0x10000000
	desktopAllAccess     = desktopReadObjects | desktopWriteObjects | desktopSwitchDesktop | 0x0004 | 0x0008 | 0x0010 | 0x0020 | 0x0040

	// WINSTA_ALL_ACCESS
	winstaAllAccess = 0x37F
)

// Info describes the interactive input desktop.
type Info struct {
	Name   string
	Secure bool // Winlogon / UAC secure desktop
}

// QueryInput returns the current input desktop name without attaching.
func QueryInput() (Info, error) {
	_ = ensureWinSta0()
	h, _, err := procOpenInputDesktop.Call(0, 0, uintptr(desktopReadObjects|desktopSwitchDesktop|genericRead))
	if h == 0 {
		return Info{}, fmt.Errorf("OpenInputDesktop: %v", err)
	}
	defer procCloseDesktop.Call(h)
	name, err := desktopName(h)
	if err != nil {
		return Info{}, err
	}
	return Info{Name: name, Secure: isSecureName(name)}, nil
}

// AttachInput switches the calling thread onto the current input desktop.
func AttachInput() (info Info, restore func(), err error) {
	_ = ensureWinSta0()
	prev, _, _ := procGetThreadDesktop.Call(currentThreadID())

	h, _, oerr := procOpenInputDesktop.Call(0, 0, uintptr(genericAll))
	if h == 0 {
		h, _, oerr = procOpenInputDesktop.Call(0, 0, uintptr(desktopAllAccess|genericRead|genericWrite))
	}
	if h == 0 {
		h, info, err = openByCandidates()
		if err != nil {
			return Info{}, nil, fmt.Errorf("OpenInputDesktop: %v; fallback: %w", oerr, err)
		}
	} else {
		name, nerr := desktopName(h)
		if nerr != nil {
			procCloseDesktop.Call(h)
			return Info{}, nil, nerr
		}
		info = Info{Name: name, Secure: isSecureName(name)}
	}

	r, _, serr := procSetThreadDesktop.Call(h)
	if r == 0 {
		procCloseDesktop.Call(h)
		return Info{}, nil, fmt.Errorf("SetThreadDesktop(%s): %v", info.Name, serr)
	}

	restore = func() {
		if prev != 0 {
			procSetThreadDesktop.Call(prev)
		}
		procCloseDesktop.Call(h)
	}
	return info, restore, nil
}

func ensureWinSta0() error {
	name, _ := syscall.UTF16PtrFromString("WinSta0")
	hw, _, err := procOpenWindowStationW.Call(uintptr(unsafe.Pointer(name)), 0, winstaAllAccess)
	if hw == 0 {
		return fmt.Errorf("OpenWindowStation(WinSta0): %v", err)
	}
	r, _, err := procSetProcessWindowStation.Call(hw)
	if r == 0 {
		procCloseWindowStation.Call(hw)
		return fmt.Errorf("SetProcessWindowStation: %v", err)
	}
	// Intentionally keep hw open for process lifetime.
	return nil
}

func openByCandidates() (uintptr, Info, error) {
	var last error
	for _, name := range []string{"Default", "Winlogon"} {
		h, err := openDesktop(name)
		if err != nil {
			last = err
			continue
		}
		return h, Info{Name: name, Secure: isSecureName(name)}, nil
	}
	return 0, Info{}, last
}

func openDesktop(name string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	h, _, e := procOpenDesktopW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		0,
		uintptr(genericAll),
	)
	if h == 0 {
		h, _, e = procOpenDesktopW.Call(
			uintptr(unsafe.Pointer(p)),
			0,
			0,
			uintptr(desktopAllAccess|genericRead|genericWrite),
		)
	}
	if h == 0 {
		return 0, fmt.Errorf("OpenDesktop(%s): %v", name, e)
	}
	return h, nil
}

func desktopName(h uintptr) (string, error) {
	var needed uint32
	procGetUserObjectInformationW.Call(h, uoIObjectName, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		needed = 512
	}
	buf := make([]uint16, needed/2+2)
	r, _, err := procGetUserObjectInformationW.Call(
		h, uoIObjectName,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return "", fmt.Errorf("GetUserObjectInformation: %v", err)
	}
	return syscall.UTF16ToString(buf), nil
}

func isSecureName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "winlogon") || strings.Contains(n, "secure")
}

func currentThreadID() uintptr {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	p := k32.NewProc("GetCurrentThreadId")
	id, _, _ := p.Call()
	return id
}
