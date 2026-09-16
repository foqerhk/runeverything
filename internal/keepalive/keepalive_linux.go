//go:build linux

package keepalive

import (
	"fmt"
	"os/exec"
)

func platformStart() (func(), error) {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		return nil, fmt.Errorf("systemd-inhibit not found")
	}
	// Block idle + sleep; display may blank/lock (network usually stays up).
	cmd := exec.Command(
		"systemd-inhibit",
		"--what=idle:sleep",
		"--who=runeverything",
		"--why=Keep agent online for remote pairing",
		"--mode=block",
		"tail", "-f", "/dev/null",
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("systemd-inhibit: %w", err)
	}
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}, nil
}
