package main

import (
	"fmt"
	"image"

	"github.com/foqerhk/runeverything/internal/desktop"
)

func try(w, h int, hevc bool) {
	enc, err := desktop.NewEncoderBitrateCodec(w, h, 5, 80000, hevc)
	if err != nil {
		fmt.Printf("CREATE FAIL %dx%d hevc=%v: %v\n", w, h, hevc, err)
		return
	}
	defer enc.Close()
	name := "h264"
	if n, ok := enc.(desktop.CodecNamer); ok {
		name = n.CodecName()
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 30
		img.Pix[i+1] = 60
		img.Pix[i+2] = 180
		img.Pix[i+3] = 255
	}
	b, err := enc.Encode(desktop.Frame{Img: img}, true)
	if err != nil {
		fmt.Printf("ENCODE FAIL %dx%d codec=%s: %v\n", w, h, name, err)
		return
	}
	fmt.Printf("OK %dx%d codec=%s annexB=%d\n", w, h, name, len(b))
}

func main() {
	for _, s := range [][2]int{{8192, 4320}, {15360, 8640}} {
		try(s[0], s[1], true)
	}
}
