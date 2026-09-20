package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
)

// Standalone Windows desktop smoke — writes results next to the exe (or RE_TEST_OUT).
func main() {
	outDir := os.Getenv("RE_TEST_OUT")
	if outDir == "" {
		if exe, err := os.Executable(); err == nil {
			outDir = filepath.Dir(exe)
		} else {
			outDir = `.`
		}
	}
	_ = os.MkdirAll(outDir, 0o755)
	logPath := filepath.Join(outDir, "desktop_smoke.log")
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Println(line)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
			_ = f.Close()
		}
	}

	cap, err := desktop.NewCapturer()
	if err != nil {
		logf("FAIL NewCapturer: %v", err)
		os.Exit(1)
	}
	defer cap.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := cap.Start(ctx, 640, 360)
	if err != nil {
		logf("FAIL Start: %v", err)
		os.Exit(1)
	}
	var f desktop.Frame
	select {
	case f = <-ch:
	case <-ctx.Done():
		logf("FAIL timeout waiting for frame")
		os.Exit(2)
	}
	if f.Img == nil || f.Img.Bounds().Dx() < 16 {
		logf("FAIL bad frame")
		os.Exit(1)
	}
	logf("FRAME_OK %dx%d", f.Img.Bounds().Dx(), f.Img.Bounds().Dy())

	w, h := f.Img.Bounds().Dx()&^1, f.Img.Bounds().Dy()&^1
	enc, err := desktop.NewEncoder(w, h, 10)
	if err != nil {
		logf("FAIL NewEncoder: %v", err)
		os.Exit(1)
	}
	defer enc.Close()
	b, err := enc.Encode(f, true)
	if err != nil {
		logf("FAIL Encode: %v", err)
		os.Exit(1)
	}
	logf("ENCODE_OK annexB=%d", len(b))

	inj, err := desktop.NewInjector()
	if err != nil {
		logf("FAIL NewInjector: %v", err)
		os.Exit(1)
	}
	if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
		di.SetScreenSize(f.Img.Bounds().Dx(), f.Img.Bounds().Dy())
	}
	if err := inj.Move(0.5, 0.5); err != nil {
		logf("FAIL Move: %v", err)
		os.Exit(1)
	}
	logf("INJECT_MOVE_OK")
	mons, err := desktop.ListMonitors()
	if err != nil {
		logf("MONITORS_ERR %v", err)
	} else {
		logf("MONITORS %d %+v", len(mons), mons)
	}
	cur, err := desktop.NewCursorReader().Cursor()
	if err != nil {
		logf("CURSOR_ERR %v", err)
	} else {
		logf("CURSOR x=%.3f y=%.3f visible=%v", cur.X, cur.Y, cur.Visible)
	}
	logf("PASS")
}
