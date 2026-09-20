//go:build windows

package winhelper

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	wtsapi32                 = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSEnumerateSessions = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSFreeMemory        = wtsapi32.NewProc("WTSFreeMemory")
	procWTSQueryUserToken    = wtsapi32.NewProc("WTSQueryUserToken")

	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procWTSGetActiveConsole      = kernel32.NewProc("WTSGetActiveConsoleSessionId")
	procProcessIdToSessionId     = kernel32.NewProc("ProcessIdToSessionId")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
)

const (
	th32csSnapProcess = 0x00000002
	wtsActive         = 0
)

type wtsSessionEntry struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// ActiveSessionID returns a usable interactive session (prefer console, else first active).
func ActiveSessionID() (uint32, error) {
	id, _, _ := procWTSGetActiveConsole.Call()
	if id != 0xFFFFFFFF && sessionIsActive(uint32(id)) {
		return uint32(id), nil
	}
	sessions, err := enumerateSessions()
	if err != nil {
		return 0, err
	}
	for _, s := range sessions {
		if s.State == wtsActive && s.SessionID != 0 {
			return s.SessionID, nil
		}
	}
	return 0, fmt.Errorf("no active interactive session")
}

func sessionIsActive(id uint32) bool {
	sessions, err := enumerateSessions()
	if err != nil {
		return id != 0
	}
	for _, s := range sessions {
		if s.SessionID == id {
			return s.State == wtsActive
		}
	}
	return false
}

func enumerateSessions() ([]wtsSessionEntry, error) {
	var p *wtsSessionEntry
	var count uint32
	r, _, err := procWTSEnumerateSessions.Call(
		0, 0, 1,
		uintptr(unsafe.Pointer(&p)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r == 0 {
		return nil, err
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(p)))
	out := make([]wtsSessionEntry, 0, count)
	sizeof := unsafe.Sizeof(wtsSessionEntry{})
	for i := uint32(0); i < count; i++ {
		e := *(*wtsSessionEntry)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i)*sizeof))
		out = append(out, e)
	}
	return out, nil
}

// systemTokenForSession duplicates a SYSTEM token bound to sessionId (from winlogon.exe).
func systemTokenForSession(sessionID uint32) (windows.Token, error) {
	_ = enableDebugPrivilege()
	pid, err := findWinlogon(sessionID)
	if err != nil {
		return 0, err
	}
	hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		hProc, err = windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	}
	if err != nil {
		return 0, fmt.Errorf("OpenProcess winlogon: %w", err)
	}
	defer windows.CloseHandle(hProc)
	var tok windows.Token
	if err := windows.OpenProcessToken(hProc, windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ADJUST_SESSIONID, &tok); err != nil {
		return 0, fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer tok.Close()
	var dup windows.Token
	if err := windows.DuplicateTokenEx(tok, windows.MAXIMUM_ALLOWED, nil, windows.SecurityIdentification, windows.TokenPrimary, &dup); err != nil {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	sid := sessionID
	if err := windows.SetTokenInformation(dup, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sid)), uint32(unsafe.Sizeof(sid))); err != nil {
		dup.Close()
		return 0, fmt.Errorf("SetTokenInformation SessionId: %w", err)
	}
	return dup, nil
}

func enableDebugPrivilege() error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeDebugPrivilege"), &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}
	return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

func findWinlogon(sessionID uint32) (uint32, error) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %v", err)
	}
	defer procCloseHandle.Call(snap)
	var pe processEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	r, _, err := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&pe)))
	if r == 0 {
		return 0, fmt.Errorf("Process32First: %v", err)
	}
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if equalFoldASCII(name, "winlogon.exe") {
			var sid uint32
			procProcessIdToSessionId.Call(uintptr(pe.ProcessID), uintptr(unsafe.Pointer(&sid)))
			if sid == sessionID {
				return pe.ProcessID, nil
			}
		}
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&pe)))
		if r == 0 {
			break
		}
	}
	return 0, fmt.Errorf("winlogon.exe not found in session %d", sessionID)
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// StartSessionHelper launches winhelper.exe serve in the active session as SYSTEM.
// Uses schtasks /RU SYSTEM /IT because winlogon.exe is PPL on modern Windows
// and OpenProcess(winlogon) fails even for LocalSystem.
func StartSessionHelper(exepath string) (uint32, error) {
	logf := func(format string, args ...any) {
		_ = os.MkdirAll(`C:\Users\Public\rewin`, 0o755)
		f, err := os.OpenFile(`C:\Users\Public\rewin\helper_launch.log`, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, time.Now().Format(time.RFC3339)+" "+format+"\n", args...)
	}

	sessionID, err := ActiveSessionID()
	if err != nil {
		logf("ActiveSessionID: %v", err)
		return 0, err
	}
	logf("session=%d exe=%s", sessionID, exepath)

	task := "RunEverythingHelperSession"
	tr := fmt.Sprintf(`"%s" serve`, exepath)
	// /IT = interactive session; /RU SYSTEM = LocalSystem in that session.
	create := exec.Command("schtasks", "/Create", "/TN", task, "/TR", tr,
		"/SC", "ONCE", "/ST", "00:00", "/RU", "SYSTEM", "/RL", "HIGHEST", "/IT", "/F")
	out, err := create.CombinedOutput()
	logf("schtasks create: %s err=%v", strings.TrimSpace(string(out)), err)
	if err != nil {
		// Fallback: try CreateProcessAsUser via winlogon (may fail on PPL).
		if pid, ferr := startViaWinlogonToken(exepath, sessionID, logf); ferr == nil {
			return pid, nil
		} else {
			logf("winlogon fallback: %v", ferr)
		}
		return 0, fmt.Errorf("schtasks create: %w (%s)", err, out)
	}
	run := exec.Command("schtasks", "/Run", "/TN", task)
	out, err = run.CombinedOutput()
	logf("schtasks run: %s err=%v", strings.TrimSpace(string(out)), err)
	if err != nil {
		return 0, fmt.Errorf("schtasks run: %w (%s)", err, out)
	}
	return 1, nil // PID unknown via schtasks; pipe readiness is the signal
}

func startViaWinlogonToken(exepath string, sessionID uint32, logf func(string, ...any)) (uint32, error) {
	tok, err := systemTokenForSession(sessionID)
	if err != nil {
		return 0, err
	}
	defer tok.Close()

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_HIDE
	si.Desktop = windows.StringToUTF16Ptr(`winsta0\default`)
	var pi windows.ProcessInformation
	app, err := windows.UTF16PtrFromString(exepath)
	if err != nil {
		return 0, err
	}
	cmdLine, err := windows.UTF16PtrFromString(fmt.Sprintf(`"%s" serve`, exepath))
	if err != nil {
		return 0, err
	}
	err = windows.CreateProcessAsUser(
		tok, app, cmdLine, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW,
		nil, nil, &si, &pi,
	)
	if err != nil {
		return 0, err
	}
	logf("CreateProcessAsUser pid=%d", pi.ProcessId)
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return pi.ProcessId, nil
}

// EnsureSessionHelper keeps a session helper alive (best-effort).
func EnsureSessionHelper(exepath string) {
	go func() {
		var lastPID uint32
		for {
			if !pipeAlive() {
				pid, err := StartSessionHelper(exepath)
				if err == nil {
					lastPID = pid
					_ = lastPID
				}
			}
			time.Sleep(3 * time.Second)
		}
	}()
}

func pipeAlive() bool {
	return Available()
}

// ServiceExePath resolves the installed service binary path.
func ServiceExePath() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return os.Executable()
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return os.Executable()
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil || cfg.BinaryPathName == "" {
		return os.Executable()
	}
	return cfg.BinaryPathName, nil
}
