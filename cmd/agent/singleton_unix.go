//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
)

// acquireAgentLock ensures only one agent process (run or tray) is active.
// Other live agent processes are terminated first so a new tray/run can take over.
func acquireAgentLock() (release func(), err error) {
	home, err := identity.HomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "agent.lock")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}

	stopAllOtherAgents()

	var f *os.File
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		_ = f.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s", i18n.T("singleton.busy"))
		}
		stopAllOtherAgents()
		time.Sleep(150 * time.Millisecond)
	}

	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Sync()

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func stopAllOtherAgents() {
	self := os.Getpid()
	for _, pid := range listAgentPIDs() {
		if pid == self || pid <= 0 {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Signal(syscall.SIGTERM)
	}
}

func listAgentPIDs() []int {
	// Exact match on process name (avoids killing relays named differently).
	out, err := exec.Command("pgrep", "-x", "runeverything").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
