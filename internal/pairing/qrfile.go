package pairing

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/foqerhk/runeverything/internal/protocol"
	qrcode "github.com/skip2/go-qrcode"
)

// WriteQRPNG writes a pairing QR code PNG and returns the file path.
func WriteQRPNG(p *protocol.PairingPayload, dir string) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "runeverything-pair.png")
	if err := qrcode.WriteFile(string(raw), qrcode.Medium, 512, path); err != nil {
		return "", err
	}
	return path, nil
}
