//go:build darwin

package desktop

import (
	"image"
	"testing"
)

func TestVideoToolboxEncodeSmoke(t *testing.T) {
	enc, err := newVideoToolboxEncoder(320, 180, 10)
	if err != nil {
		t.Fatalf("vt create: %v", err)
	}
	defer enc.Close()
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 20
		img.Pix[i+1] = 40
		img.Pix[i+2] = 200
		img.Pix[i+3] = 255
	}
	b, err := enc.Encode(Frame{Img: img}, true)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(b) < 8 {
		t.Fatalf("annexB too short: %d", len(b))
	}
	// start code
	if !(b[0] == 0 && b[1] == 0 && (b[2] == 1 || (b[2] == 0 && b[3] == 1))) {
		t.Fatalf("missing annex-B start code: %x", b[:8])
	}
	t.Logf("vt annexB bytes=%d", len(b))
}

func TestCGCaptureSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	img, err := captureSCK()
	if err != nil {
		t.Skipf("screen capture unavailable (grant Screen Recording?): %v", err)
	}
	if img.Bounds().Dx() < 16 || img.Bounds().Dy() < 16 {
		t.Fatalf("tiny frame %v", img.Bounds())
	}
}
