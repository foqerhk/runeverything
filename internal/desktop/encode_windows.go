//go:build windows

package desktop

import (
	"os/exec"
)

// NewEncoder on Windows: prefer MediaFoundation (h264_mf), else libx264, else synthetic.
func NewEncoder(width, height, fps int) (Encoder, error) {
	return NewEncoderBitrate(width, height, fps, 2500)
}

// NewEncoderBitrate creates an encoder with an explicit target bitrate (kbps).
func NewEncoderBitrate(width, height, fps, bitrateK int) (Encoder, error) {
	if fps <= 0 {
		fps = 15
	}
	width &^= 1
	height &^= 1
	if _, err := lookPath("ffmpeg"); err != nil {
		return &syntheticEncoder{w: width, h: height}, nil
	}
	// Probe h264_mf quickly; fall back to libx264.
	if probeFFmpegCodec("h264_mf") {
		if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "h264_mf"); err == nil {
			return enc, nil
		}
	}
	return newFFmpegEncoder(width, height, fps, bitrateK, "libx264")
}

func probeFFmpegCodec(name string) bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return containsBytes(out, []byte(name))
}

func containsBytes(b, sub []byte) bool {
	return len(sub) == 0 || (len(b) >= len(sub) && indexBytes(b, sub) >= 0)
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
