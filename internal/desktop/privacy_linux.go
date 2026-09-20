//go:build linux

package desktop

import (
	"os"
	"os/exec"
	"time"
)

func startPrivacyBlankOS() (func(), error) {
	// Best-effort: black fullscreen via xsetroot + unclutter, or python/tk if present.
	// Prefer a dedicated overlay with xdotool/xwininfo when available.
	if os.Getenv("DISPLAY") == "" {
		return privacyUnsupported()
	}
	// Use ffmpeg lavfi color as a borderless mpv window if mpv exists.
	if _, err := exec.LookPath("mpv"); err == nil {
		cmd := exec.Command("mpv", "--no-osc", "--no-input-default-bindings", "--cursor-autohide=always",
			"--fullscreen", "--ontop", "--really-quiet",
			"av://lavfi:color=c=black:s=1920x1080:r=1")
		if err := cmd.Start(); err == nil {
			return func() {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
				}
			}, nil
		}
	}
	// Fallback: dim via xrandr brightness (restored on stop).
	_ = exec.Command("xrandr", "--output", "eDP-1", "--brightness", "0.01").Run()
	_ = exec.Command("xrandr", "--output", "HDMI-1", "--brightness", "0.01").Run()
	_ = exec.Command("xrandr", "--output", "DP-1", "--brightness", "0.01").Run()
	return func() {
		_ = exec.Command("xrandr", "--output", "eDP-1", "--brightness", "1").Run()
		_ = exec.Command("xrandr", "--output", "HDMI-1", "--brightness", "1").Run()
		_ = exec.Command("xrandr", "--output", "DP-1", "--brightness", "1").Run()
		time.Sleep(20 * time.Millisecond)
	}, nil
}
