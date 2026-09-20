//go:build linux

package desktop

import (
	"os/exec"
	"strings"
)

func writeClipboardText(s string) error {
	// Prefer xclip, then xsel
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(s)
	if err := cmd.Run(); err == nil {
		return nil
	}
	cmd = exec.Command("xsel", "--clipboard", "--input")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

func readClipboardText() (string, error) {
	out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
	if err == nil {
		return string(out), nil
	}
	out, err = exec.Command("xsel", "--clipboard", "--output").Output()
	return string(out), err
}

func writeClipboardPNG(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png")
	cmd.Stdin = strings.NewReader(string(b))
	return cmd.Run()
}

func readClipboardPNG() ([]byte, error) {
	out, err := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o").Output()
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return out, nil
}
