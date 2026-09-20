//go:build !darwin && !windows

package desktop

import (
	"os"
	"os/exec"
)

// NewEncoder on Linux/other: try VAAPI, else libx264, else synthetic.
func NewEncoder(width, height, fps int) (Encoder, error) {
	return NewEncoderBitrate(width, height, fps, 2500)
}

func NewEncoderBitrate(width, height, fps, bitrateK int) (Encoder, error) {
	if fps <= 0 {
		fps = 15
	}
	width &^= 1
	height &^= 1
	if _, err := lookPath("ffmpeg"); err != nil {
		return &syntheticEncoder{w: width, h: height}, nil
	}
	// Prefer software x264 for reliability; opt-in VAAPI via RE_ENCODER=vaapi.
	if os.Getenv("RE_ENCODER") == "vaapi" && probeFFmpegCodec("h264_vaapi") {
		if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "h264_vaapi"); err == nil {
			return enc, nil
		}
	}
	if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "libx264"); err == nil {
		return enc, nil
	}
	if probeFFmpegCodec("h264_vaapi") {
		if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "h264_vaapi"); err == nil {
			return enc, nil
		}
	}
	return &syntheticEncoder{w: width, h: height}, nil
}

func probeFFmpegCodec(name string) bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return indexBytes(out, []byte(name)) >= 0
}

func indexBytes(b, sub []byte) int {
outer:
	for i := 0; i+len(sub) <= len(b); i++ {
		for j := 0; j < len(sub); j++ {
			if b[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
