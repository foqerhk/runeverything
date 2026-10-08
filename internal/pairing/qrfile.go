package pairing

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/foqerhk/runeverything/internal/protocol"
	qrcode "github.com/skip2/go-qrcode"
)

// EncodeQRPNG returns a PNG encoding of the pairing payload as a QR code.
func EncodeQRPNG(p *protocol.PairingPayload, size int) ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		size = 512
	}
	return qrcode.Encode(string(raw), qrcode.Medium, size)
}

// WriteQRPNG writes a pairing QR code PNG and returns the file path.
func WriteQRPNG(p *protocol.PairingPayload, dir string) (string, error) {
	png, err := EncodeQRPNG(p, 512)
	if err != nil {
		return "", err
	}
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "runeverything-pair.png")
	if err := os.WriteFile(path, png, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
