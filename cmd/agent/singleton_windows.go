//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procCloseHandle  = kernel32.NewProc("CloseHandle")
	procOpenProcess  = kernel32.NewProc("OpenProcess")
	procTerminate    = kernel32.NewProc("TerminateProcess")
)

const errorAlreadyExists = 183

// acquireAgentLock uses a named mutex + pid file so only one agent runs.
func acquireAgentLock() (release func(), err error) {
	home, err := identity.HomeDir()
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(home, 0o700)
	lockPath := filepath.Join(home, "agent.lock")

	name, err := syscall.UTF16PtrFromString("Local\\RunEverythingAgent")
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(4 * time.Second)
	var handle uintptr
	for {
		r, _, lastErr := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(name)))
		handle = r
		if handle == 0 {
			return nil, lastErr
		}
		if errno, ok := lastErr.(syscall.Errno); ok && errno == errorAlreadyExists {
			_ = procCloseHandle.Call(handle)
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("%s", i18n.T("singleton.busy"))
			}
			_ = stopOtherAgentWin(lockPath)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		break
	}

	_ = os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600)
	return func() {
		_ = procCloseHandle.Call(handle)
	}, nil
}

func stopOtherAgentWin(lockPath string) error {
	b, err := os.ReadFile(lockPath)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return nil
	}
	const processTerminate = 0x0001
	h, _, _ := procOpenProcess.Call(processTerminate, 0, uintptr(pid))
	if h == 0 {
		return nil
	}
	defer procCloseHandle.Call(h)
	_, _, _ = procTerminate.Call(h, 0)
	return nil
}
