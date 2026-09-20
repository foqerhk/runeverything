package desktop

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AudioCapture streams compressed or PCM audio chunks via ffmpeg.
type AudioCapture struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	codec  string // opus|pcm16
	rate   int
}

func StartAudioCapture(ctx context.Context, sampleRate int) (*AudioCapture, error) {
	if os.Getenv("RE_AUDIO") == "0" {
		return nil, nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if _, err := lookPath("ffmpeg"); err != nil {
		return nil, err
	}
	preferOpus := os.Getenv("RE_AUDIO_PCM") != "1"
	for _, in := range audioInputArgs(sampleRate) {
		var args []string
		codec := "pcm16"
		if preferOpus && probeFFmpegEncoder("libopus") {
			args = append([]string{}, in...)
			args = append(args, "-c:a", "libopus", "-b:a", "64k", "-frame_duration", "20", "-application", "lowdelay", "-f", "opus", "pipe:1")
			codec = "opus"
		} else {
			args = append([]string{}, in...)
			args = append(args, "-f", "s16le", "pipe:1")
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-loglevel", "error"}, args...)...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			continue
		}
		if err := cmd.Start(); err != nil {
			continue
		}
		return &AudioCapture{cmd: cmd, stdout: stdout, codec: codec, rate: sampleRate}, nil
	}
	return nil, os.ErrNotExist
}

func (a *AudioCapture) Codec() string {
	if a == nil || a.codec == "" {
		return "pcm16"
	}
	return a.codec
}

func (a *AudioCapture) SampleRate() int {
	if a == nil || a.rate <= 0 {
		return 48000
	}
	return a.rate
}

func audioInputArgs(sampleRate int) [][]string {
	ar := itoa(sampleRate)
	switch runtime.GOOS {
	case "darwin":
		return [][]string{
			{"-f", "avfoundation", "-i", ":0", "-ac", "1", "-ar", ar},
			{"-f", "avfoundation", "-i", ":1", "-ac", "1", "-ar", ar},
		}
	case "linux":
		return [][]string{
			{"-f", "pulse", "-i", "default", "-ac", "1", "-ar", ar},
			{"-f", "pulse", "-i", "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor", "-ac", "1", "-ar", ar},
			{"-f", "alsa", "-i", "default", "-ac", "1", "-ar", ar},
		}
	case "windows":
		devs := windowsAudioDevices()
		out := make([][]string, 0, len(devs)+3)
		for _, d := range devs {
			out = append(out, []string{"-f", "dshow", "-i", "audio=" + d, "-ac", "1", "-ar", ar})
		}
		out = append(out,
			[]string{"-f", "dshow", "-i", "audio=virtual-audio-capturer", "-ac", "1", "-ar", ar},
			[]string{"-f", "dshow", "-i", "audio=Stereo Mix", "-ac", "1", "-ar", ar},
			[]string{"-f", "wasapi", "-i", "default", "-ac", "1", "-ar", ar},
		)
		return out
	default:
		return nil
	}
}

func windowsAudioDevices() []string {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	out, _ := cmd.CombinedOutput()
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "(audio)") {
			continue
		}
		start := strings.Index(line, `"`)
		end := strings.LastIndex(line, `"`)
		if start < 0 || end <= start {
			continue
		}
		name := line[start+1 : end]
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func probeFFmpegEncoder(name string) bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), name)
}

func (a *AudioCapture) ReadPCM(n int) ([]byte, error) {
	if a == nil || a.stdout == nil {
		return nil, io.EOF
	}
	if a.codec == "opus" {
		// Read a bounded chunk of opus container bytes.
		if n < 256 {
			n = 256
		}
		if n > 4096 {
			n = 4096
		}
		buf := make([]byte, n)
		nr, err := a.stdout.Read(buf)
		if nr > 0 {
			return buf[:nr], nil
		}
		return nil, err
	}
	buf := make([]byte, n)
	nr, err := io.ReadFull(a.stdout, buf)
	if nr > 0 {
		return buf[:nr], nil
	}
	return nil, err
}

func (a *AudioCapture) Close() error {
	if a == nil {
		return nil
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
	return nil
}

func EncodeAudioPCM16(pcm []byte) string {
	return base64.StdEncoding.EncodeToString(pcm)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// AudioPlayer is a persistent ffplay/ffmpeg sink for inbound remote audio.
type AudioPlayer struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	codec  string
	rate   int
	closed bool
}

func StartAudioPlayer(ctx context.Context, sampleRate int, codec string) (*AudioPlayer, error) {
	if os.Getenv("RE_AUDIO") == "0" {
		return nil, nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if codec == "" {
		codec = "pcm16"
	}
	var cmd *exec.Cmd
	if codec == "opus" {
		if _, err := lookPath("ffplay"); err == nil {
			cmd = exec.CommandContext(ctx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "error",
				"-f", "opus", "-i", "pipe:0")
		} else if _, err := lookPath("ffmpeg"); err == nil {
			args := append([]string{"-loglevel", "error", "-f", "opus", "-i", "pipe:0"}, audioOutArgs()...)
			cmd = exec.CommandContext(ctx, "ffmpeg", args...)
		} else {
			return nil, fmt.Errorf("ffplay/ffmpeg not found")
		}
	} else if _, err := lookPath("ffplay"); err == nil {
		cmd = exec.CommandContext(ctx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "error",
			"-f", "s16le", "-ar", itoa(sampleRate), "-ac", "1", "-i", "pipe:0")
	} else if _, err := lookPath("ffmpeg"); err == nil {
		args := append([]string{"-loglevel", "error", "-f", "s16le", "-ar", itoa(sampleRate), "-ac", "1", "-i", "pipe:0"}, audioOutArgs()...)
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
	} else {
		return nil, fmt.Errorf("ffplay/ffmpeg not found")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &AudioPlayer{cmd: cmd, stdin: stdin, codec: codec, rate: sampleRate}, nil
}

func audioOutArgs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"-f", "coreaudio", "default"}
	case "linux":
		return []string{"-f", "pulse", "default"}
	case "windows":
		return []string{"-f", "wasapi", "default"}
	default:
		return []string{"-f", "null", "-"}
	}
}

func (p *AudioPlayer) Write(b []byte) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.stdin == nil {
		return io.ErrClosedPipe
	}
	_, err := p.stdin.Write(b)
	return err
}

func (p *AudioPlayer) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}

// PlayPCM16 plays a short buffer (legacy one-shot). Prefer StartAudioPlayer for streams.
func PlayPCM16(sampleRate int, pcm []byte) error {
	if len(pcm) == 0 || os.Getenv("RE_AUDIO") == "0" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := StartAudioPlayer(ctx, sampleRate, "pcm16")
	if err != nil {
		return err
	}
	_ = p.Write(pcm)
	return p.Close()
}

const AudioPumpInterval = 20 * time.Millisecond
