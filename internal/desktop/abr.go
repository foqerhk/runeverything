package desktop

import (
	"sync"
	"time"
)

// ABRConfig holds adaptive bitrate / FPS targets.
type ABRConfig struct {
	MinFPS      int
	MaxFPS      int
	MinWidth    int
	MaxWidth    int
	MinBitrateK int
	MaxBitrateK int
}

func DefaultABR() ABRConfig {
	return ABRConfig{
		MinFPS: 5, MaxFPS: 30,
		MinWidth: 640, MaxWidth: 1920,
		MinBitrateK: 300, MaxBitrateK: 8000,
	}
}

// ABRController adjusts encode params from client Stats feedback.
type ABRController struct {
	mu     sync.Mutex
	cfg    ABRConfig
	fps    int
	width  int
	height int
	bitrateK int
	wantKey  bool
	lastAdj  time.Time
}

func NewABR(cfg ABRConfig, width, height, fps, bitrateK int) *ABRController {
	if fps <= 0 {
		fps = 15
	}
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	return &ABRController{cfg: cfg, fps: fps, width: width, height: height, bitrateK: bitrateK}
}

func (a *ABRController) Snapshot() (w, h, fps, bitrateK int, forceKey bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fk := a.wantKey
	a.wantKey = false
	return a.width, a.height, a.fps, a.bitrateK, fk
}

func (a *ABRController) RequestKeyframe() {
	a.mu.Lock()
	a.wantKey = true
	a.mu.Unlock()
}

// OnStats updates targets. High RTT/loss → lower bitrate/FPS/resolution; healthy → ramp up.
func (a *ABRController) OnStats(rttMs int, lossPct float64, wantKey bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if wantKey {
		a.wantKey = true
	}
	now := time.Now()
	if now.Sub(a.lastAdj) < 500*time.Millisecond {
		return
	}
	a.lastAdj = now

	bad := rttMs > 180 || lossPct > 5
	worse := rttMs > 350 || lossPct > 12
	good := rttMs < 80 && lossPct < 1

	if worse {
		a.bitrateK = max(a.cfg.MinBitrateK, a.bitrateK*6/10)
		a.fps = max(a.cfg.MinFPS, a.fps-5)
		a.width = max(a.cfg.MinWidth, a.width*3/4)
		a.height = max(a.cfg.MinWidth*9/16, a.height*3/4)
		a.wantKey = true
	} else if bad {
		a.bitrateK = max(a.cfg.MinBitrateK, a.bitrateK*8/10)
		a.fps = max(a.cfg.MinFPS, a.fps-2)
	} else if good {
		a.bitrateK = min(a.cfg.MaxBitrateK, a.bitrateK*11/10+100)
		a.fps = min(a.cfg.MaxFPS, a.fps+1)
		if a.width < a.cfg.MaxWidth {
			a.width = min(a.cfg.MaxWidth, a.width+80)
			a.height = min(a.cfg.MaxWidth*9/16, a.height+45)
		}
	}
	a.width &^= 1
	a.height &^= 1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
