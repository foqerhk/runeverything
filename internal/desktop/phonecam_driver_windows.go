//go:build windows

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func phoneCamDriverStatus() PhoneCamDriverStatus {
	prog := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
	}
	for _, root := range prog {
		if root == "" {
			continue
		}
		for _, rel := range []string{
			`AkVirtualCamera`,
			`AkVirtualCamera\AkVCamManager.exe`,
			`webcamoid\AkVirtualCamera`,
		} {
			p := filepath.Join(root, rel)
			if st, err := os.Stat(p); err == nil {
				_ = st
				return PhoneCamDriverStatus{Installed: true, Name: "AkVirtualCamera"}
			}
		}
	}
	obsRoots := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "obs-studio"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "obs-studio"),
	}
	for _, root := range obsRoots {
		if _, err := os.Stat(root); err == nil {
			return PhoneCamDriverStatus{Installed: true, Name: "OBS Virtual Camera"}
		}
	}
	_ = strings.Builder{}
	return PhoneCamDriverStatus{}
}

func openPhoneCamDriverInstall() error {
	installer, err := resolvePhoneCamInstaller()
	if err != nil {
		return fmt.Errorf("bundled AkVirtualCamera installer not found (expected next to Agent under akvirtualcamera\\)")
	}
	// UAC elevation — user enters admin password / consent in the system dialog.
	ps := fmt.Sprintf(
		`Start-Process -FilePath '%s' -Verb RunAs`,
		strings.ReplaceAll(installer, "'", "''"),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
