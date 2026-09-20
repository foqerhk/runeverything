package desktop

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
)

// ClipboardHub syncs text and optional PNG image bytes.
type ClipboardHub struct {
	mu       sync.Mutex
	last     string
	lastPNG  []byte
	seqText  uint64
	seqImage uint64
}

func NewClipboardHub() *ClipboardHub { return &ClipboardHub{} }

func (c *ClipboardHub) SetText(s string) {
	c.mu.Lock()
	c.last = s
	c.seqText++
	c.mu.Unlock()
	_ = writeClipboardText(s)
}

func (c *ClipboardHub) GetText() string {
	if t, err := readClipboardText(); err == nil && t != "" {
		return t
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

func (c *ClipboardHub) SetPNG(b []byte) {
	if len(b) == 0 {
		return
	}
	c.mu.Lock()
	c.lastPNG = append([]byte(nil), b...)
	c.seqImage++
	c.mu.Unlock()
	_ = writeClipboardPNG(b)
}

func (c *ClipboardHub) GetPNG() []byte {
	if b, err := readClipboardPNG(); err == nil && len(b) > 0 {
		return b
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.lastPNG...)
}

func (c *ClipboardHub) Snapshot() (text string, png []byte, textSeq, imgSeq uint64) {
	text = c.GetText()
	png = c.GetPNG()
	c.mu.Lock()
	defer c.mu.Unlock()
	return text, png, c.seqText, c.seqImage
}

func EncodePNGB64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func DecodePNGB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// File transfer staging under ~/.runeverything/xfer
func XferDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".runeverything", "xfer")
	return dir, os.MkdirAll(dir, 0o700)
}
