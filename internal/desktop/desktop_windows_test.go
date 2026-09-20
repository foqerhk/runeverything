//go:build windows

package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsCaptureEncodeInject(t *testing.T) {
	outDir := os.Getenv("RE_TEST_OUT")
	if outDir == "" {
		outDir = os.TempDir()
	}
	_ = os.MkdirAll(outDir, 0o755)
	logPath := filepath.Join(outDir, "desktop_smoke.log")
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		t.Log(line)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
	}

	cap, err := NewCapturer()
	if err != nil {
		logf("FAIL NewCapturer: %v", err)
		t.Fatalf("NewCapturer: %v", err)
	}
	defer cap.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ch, err := cap.Start(ctx, 640, 360)
	if err != nil {
		logf("FAIL Start: %v", err)
		t.Fatalf("Start: %v", err)
	}
	var f Frame
	select {
	case f = <-ch:
	case <-ctx.Done():
		logf("FAIL timeout waiting for frame")
		t.Fatal("timeout waiting for frame")
	}
	if f.Img == nil || f.Img.Bounds().Dx() < 16 {
		logf("FAIL bad frame")
		t.Fatalf("bad frame %+v", f.Img)
	}
	logf("FRAME_OK %dx%d", f.Img.Bounds().Dx(), f.Img.Bounds().Dy())

	w, h := f.Img.Bounds().Dx()&^1, f.Img.Bounds().Dy()&^1
	enc, err := NewEncoder(w, h, 10)
	if err != nil {
		logf("FAIL NewEncoder: %v", err)
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	b, err := enc.Encode(f, true)
	if err != nil {
		logf("FAIL Encode: %v", err)
		t.Fatalf("Encode: %v", err)
	}
	if len(b) < 8 {
		logf("FAIL annexB too short %d", len(b))
		t.Fatalf("annexB too short %d", len(b))
	}
	logf("ENCODE_OK annexB=%d", len(b))

	inj, err := NewInjector()
	if err != nil {
		logf("FAIL NewInjector: %v", err)
		t.Fatalf("NewInjector: %v", err)
	}
	if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
		di.SetScreenSize(f.Img.Bounds().Dx(), f.Img.Bounds().Dy())
	}
	if err := inj.Move(0.5, 0.5); err != nil {
		logf("FAIL Move: %v", err)
		t.Fatalf("Move: %v", err)
	}
	logf("INJECT_MOVE_OK")

	mons, err := ListMonitors()
	if err != nil {
		logf("MONITORS_ERR %v", err)
	} else {
		logf("MONITORS %d", len(mons))
	}
	cur, err := NewCursorReader().Cursor()
	if err != nil {
		logf("CURSOR_ERR %v", err)
	} else {
		logf("CURSOR x=%.3f y=%.3f visible=%v", cur.X, cur.Y, cur.Visible)
	}
	logf("PASS")
}
