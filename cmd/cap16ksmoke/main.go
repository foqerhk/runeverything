package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
)

func main() {
	mode := "16k"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	id, w, h, err := desktop.EnsureVirtual(mode)
	if err != nil {
		panic(err)
	}
	fw, fh, ok := desktop.VirtualFramebuffer(int(id))
	fmt.Printf("virtual id=%d logical=%dx%d fb=%dx%d ok=%v\n", id, w, h, fw, fh, ok)
	desktop.SetSelectedMonitor(int(id))
	tw, th := fw, fh
	if tw <= 0 {
		tw, th = w, h
	}
	desktop.SetCaptureInlineHEVC(tw, th, 80000, 15)
	cap, err := desktop.NewCapturer()
	if err != nil {
		panic(err)
	}
	defer cap.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	ch, err := cap.Start(ctx, tw, th)
	if err != nil {
		panic(err)
	}
	select {
	case f, ok := <-ch:
		if !ok {
			panic("closed")
		}
		if f.Img != nil {
			b := f.Img.Bounds()
			fmt.Printf("OK rgba %dx%d\n", b.Dx(), b.Dy())
		} else {
			fmt.Printf("OK annexB len=%d size=? inline\n", len(f.AnnexB))
		}
	case <-ctx.Done():
		panic("timeout waiting frame")
	}
	_ = desktop.DestroyAllVirtual()
}
