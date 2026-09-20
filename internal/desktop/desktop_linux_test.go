//go:build linux

package desktop

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLinuxCaptureEncodeInject(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		_ = os.Setenv("DISPLAY", ":10")
	}
	cap, err := NewCapturer()
	if err != nil {
		t.Fatalf("NewCapturer: %v", err)
	}
	defer cap.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := cap.Start(ctx, 640, 360)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var f Frame
	select {
	case f = <-ch:
	case <-ctx.Done():
		t.Fatal("timeout waiting for frame — is DISPLAY set to an active X session?")
	}
	if f.Img == nil || f.Img.Bounds().Dx() < 16 {
		t.Fatalf("bad frame %+v", f.Img)
	}
	t.Logf("frame %dx%d", f.Img.Bounds().Dx(), f.Img.Bounds().Dy())

	w, h := f.Img.Bounds().Dx()&^1, f.Img.Bounds().Dy()&^1
	enc, err := NewEncoder(w, h, 10)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	b, err := enc.Encode(f, true)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(b) < 8 {
		t.Fatalf("annexB too short %d", len(b))
	}
	t.Logf("annexB=%d", len(b))

	inj, err := NewInjector()
	if err != nil {
		t.Fatalf("NewInjector: %v", err)
	}
	if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
		di.SetScreenSize(f.Img.Bounds().Dx(), f.Img.Bounds().Dy())
	}
	if err := inj.Move(0.5, 0.5); err != nil {
		t.Fatalf("Move: %v", err)
	}
}
