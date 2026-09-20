//go:build !darwin && !linux && !windows

package desktop

func writeClipboardText(s string) error { return nil }
func readClipboardText() (string, error) { return "", nil }
func writeClipboardPNG(b []byte) error  { return nil }
func readClipboardPNG() ([]byte, error) { return nil, nil }
