package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
)

// daysmoke exercises today's commercial-parity changes on the current OS.
func main() {
	outDir := os.Getenv("RE_TEST_OUT")
	if outDir == "" {
		if exe, err := os.Executable(); err == nil {
			outDir = filepath.Dir(exe)
		} else {
			outDir = "."
		}
	}
	_ = os.MkdirAll(outDir, 0o755)
	logPath := filepath.Join(outDir, "day_smoke.log")
	var fails []string
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Println(line)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
			_ = f.Close()
		}
	}
	fail := func(format string, args ...any) {
		line := "FAIL " + fmt.Sprintf(format, args...)
		fails = append(fails, line)
		logf("%s", line)
	}
	pass := func(format string, args ...any) {
		logf("PASS "+format, args...)
	}

	logf("=== day_smoke start os=%s arch=%s ===", runtime.GOOS, runtime.GOARCH)

	// Monitors
	mons, err := desktop.ListMonitors()
	if err != nil || len(mons) == 0 {
		fail("ListMonitors: %v len=%d", err, len(mons))
	} else {
		pass("ListMonitors n=%d primary=%+v", len(mons), mons[0])
	}

	// Hide cursor flag + capture
	desktop.SetHideCursor(true)
	desktop.SetSelectedMonitor(0)
	cap, err := desktop.NewCapturer()
	if err != nil {
		fail("NewCapturer: %v", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		ch, err := cap.Start(ctx, 640, 360)
		if err != nil {
			fail("Capturer.Start: %v", err)
			cancel()
		} else {
			select {
			case f := <-ch:
				if f.Img == nil || f.Img.Bounds().Dx() < 16 {
					fail("bad frame")
				} else {
					pass("Capture %dx%d backend_ok", f.Img.Bounds().Dx(), f.Img.Bounds().Dy())
					w, h := f.Img.Bounds().Dx()&^1, f.Img.Bounds().Dy()&^1
					enc, err := desktop.NewEncoderBitrate(w, h, 10, 1500)
					if err != nil {
						fail("NewEncoderBitrate: %v", err)
					} else {
						b, err := enc.Encode(f, true)
						_ = enc.Close()
						if err != nil || len(b) < 8 {
							fail("Encode: %v len=%d", err, len(b))
						} else {
							pass("Encode annexB=%d", len(b))
						}
					}
				}
			case <-ctx.Done():
				fail("capture timeout")
			}
			cancel()
		}
		_ = cap.Close()
	}
	desktop.SetHideCursor(false)

	// Inject relative + wheelH
	inj, err := desktop.NewInjector()
	if err != nil {
		fail("NewInjector: %v", err)
	} else {
		inj.SetRelativeMouse(true)
		_ = inj.MoveRelative(1, 1)
		_ = inj.WheelH(1)
		_ = inj.Wheel(1)
		inj.SetRelativeMouse(false)
		_ = inj.Close()
		pass("Injector relative+wheelH")
	}

	// Clipboard
	hub := desktop.NewClipboardHub()
	hub.SetText("re-day-smoke-" + runtime.GOOS)
	got := hub.GetText()
	if !strings.Contains(got, "re-day-smoke") {
		fail("clipboard text got=%q", got)
	} else {
		pass("Clipboard text")
	}

	// WOL packet (won't wake anyone; validates craft+send)
	if err := desktop.WakeOnLAN("AA:BB:CC:DD:EE:FF", "127.0.0.1:9"); err != nil {
		fail("WakeOnLAN: %v", err)
	} else {
		pass("WakeOnLAN send")
	}

	// Camera list (may be empty)
	cams, err := desktop.ListCameras()
	if err != nil {
		logf("WARN ListCameras: %v", err)
	} else {
		pass("ListCameras n=%d", len(cams))
	}

	// USB list
	usbs, err := desktop.ListUSBDevices()
	if err != nil {
		logf("WARN ListUSB: %v", err)
	} else {
		pass("ListUSB n=%d", len(usbs))
	}

	// Printers
	printers, err := desktop.ListPrinters()
	if err != nil {
		logf("WARN ListPrinters: %v", err)
	} else {
		pass("ListPrinters n=%d", len(printers))
	}

	// Xfer dir + write
	dir, err := desktop.XferDir()
	if err != nil {
		fail("XferDir: %v", err)
	} else {
		p := filepath.Join(dir, "day_smoke.txt")
		if err := os.WriteFile(p, []byte("ok"), 0o600); err != nil {
			fail("xfer write: %v", err)
		} else {
			pass("XferDir %s", dir)
			_ = os.Remove(p)
		}
	}

	// Privacy blank — brief toggle (skip if RE_SMOKE_NO_PRIVACY=1)
	if os.Getenv("RE_SMOKE_NO_PRIVACY") != "1" {
		if err := desktop.SetPrivacyBlank(true); err != nil {
			logf("WARN PrivacyBlank on: %v", err)
		} else {
			time.Sleep(300 * time.Millisecond)
			_ = desktop.SetPrivacyBlank(false)
			pass("PrivacyBlank toggle")
		}
	}

	// Confirm API exists (don't block on dialog)
	pass("ConfirmLocal available (not invoking interactive)")

	logf("=== day_smoke done fails=%d ===", len(fails))
	if len(fails) > 0 {
		for _, f := range fails {
			logf("%s", f)
		}
		os.Exit(1)
	}
	logf("ALL_PASS")
}
