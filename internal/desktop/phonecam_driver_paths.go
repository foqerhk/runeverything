package desktop

import (
	"os"
	"path/filepath"
	"runtime"
)

// phoneCamInstallerCandidates returns local installer paths to try, in order.
// Prefer the copy bundled next to the Agent / inside the .app Resources.
func phoneCamInstallerCandidates() []string {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		// Prefer the Camera Extension payload (required on macOS 14.1+ / browsers).
		// Legacy .pkg retained as last-resort fallback.
		names = []string{
			"macos-payload/install.sh",
			"akvirtualcamera-mac-arm64.pkg",
			"akvirtualcamera-mac-x64.pkg",
		}
	case "windows":
		names = []string{"akvirtualcamera-windows.exe"}
	default:
		return nil
	}

	var roots []string
	if exe, err := os.Executable(); err == nil {
		exe, _ := filepath.EvalSymlinks(exe)
		dir := filepath.Dir(exe)
		// macOS app: Contents/MacOS/RunEverything → Contents/Resources/akvirtualcamera
		roots = append(roots,
			filepath.Join(dir, "akvirtualcamera"),
			filepath.Join(dir, "..", "Resources", "akvirtualcamera"),
			filepath.Join(dir, "Resources", "akvirtualcamera"),
		)
		// Windows install dir often %LOCALAPPDATA%\RunEverything\bin
		roots = append(roots, filepath.Join(dir, "..", "akvirtualcamera"))
	}
	// Dev / source-tree fallbacks (run from repo root or cmd/agent).
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots,
			filepath.Join(wd, "packaging", "akvirtualcamera"),
			filepath.Join(wd, "..", "packaging", "akvirtualcamera"),
			filepath.Join(wd, "..", "..", "packaging", "akvirtualcamera"),
		)
	}

	var out []string
	seen := map[string]bool{}
	for _, root := range roots {
		for _, name := range names {
			p := filepath.Clean(filepath.Join(root, name))
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func resolvePhoneCamInstaller() (string, error) {
	for _, p := range phoneCamInstallerCandidates() {
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() && st.Size() > 0 {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
