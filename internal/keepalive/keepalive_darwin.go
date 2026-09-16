//go:build darwin

package keepalive

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func platformStart() (func(), error) {
	// -i idle, -s system (AC), -m disk. Do not assert -d (allow display sleep/lock).
	cmd := exec.Command("caffeinate", "-i", "-s", "-m", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("caffeinate: %w", err)
	}
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}, nil
}
